package controller

import (
	"context"
	"fmt"
	"github.com/atomix/chaos-controller/pkg/apis/chaos/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sync"
	"time"
)

var log = logf.Log.WithName("controller_chaosmonkey")

// Add creates a new ChaosMonkey Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func AddController(mgr manager.Manager) error {
	ch := New(mgr.GetClient(), mgr.GetScheme(), mgr.GetConfig())
	err := mgr.Add(ch)
	if err != nil {
		return err
	}

	r := &ReconcileChaosMonkey{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: mgr.GetConfig(),
		chaos:  ch,
	}

	// Create a new controller
	c, err := controller.New("chaos", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource ChaosMonkey
	err = c.Watch(&source.Kind{Type: &v1alpha1.ChaosMonkey{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

var _ reconcile.Reconciler = &ReconcileChaosMonkey{}

// ReconcileChaosMonkey reconciles a ChaosMonkey object
type ReconcileChaosMonkey struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
	chaos  *ChaosController
}

// Reconcile reads that state of the cluster for a ChaosMonkey object and makes changes based on the state read
// and what is in the ChaosMonkey.Spec
func (r *ReconcileChaosMonkey) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("namespace", request.Namespace, "name", request.Name)
	reqLogger.Info("Reconciling ChaosMonkey")

	// Fetch the ChaosMonkey instance
	instance := &v1alpha1.ChaosMonkey{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			monkey := r.chaos.RemoveMonkey(request.NamespacedName)
			if monkey != nil {
				monkey.Stop()
			}
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	v1alpha1.SetDefaults(instance)

	monkey := r.chaos.GetOrCreateMonkey(request.NamespacedName, instance)
	if !monkey.Started {
		err := monkey.Start()
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, err
}

type Context struct {
	client  client.Client
	scheme  *runtime.Scheme
	kubecli kubernetes.Interface
	config  *rest.Config
	log     logr.Logger
}

// new returns a new Context with the given logger
func (c Context) new(log logr.Logger) Context {
	return Context{c.client, c.scheme, c.kubecli, c.config, log}
}

var _ manager.Runnable = &ChaosController{}

// New returns a new ChaosController for managing chaos monkeys running in the cluster.
func New(client runtimeclient.Client, scheme *runtime.Scheme, config *rest.Config) *ChaosController {
	kubecli := kubernetes.NewForConfigOrDie(config)
	ctx := Context{client, scheme, kubecli, config, log}
	return &ChaosController{
		context: ctx,
		monkeys: make(map[string]*MonkeyController),
	}
}

// Chaos controller manages all chaos monkeys running in the cluster.
type ChaosController struct {
	context Context
	mu      sync.Mutex
	monkeys map[string]*MonkeyController
	Started bool
	stopped bool
}

// Start starts the chaos controller. Added monkeys are run until the stop channel is closed.
func (c *ChaosController) Start(stop <-chan struct{}) error {
	c.mu.Lock()

	defer utilruntime.HandleCrash()
	defer c.Stop()

	log.Info("Starting chaos controller")

	c.Started = true
	c.mu.Unlock()

	<-stop

	log.Info("Stopping monkeys")
	for _, monkey := range c.monkeys {
		monkey.Stop()
	}

	return nil
}

// Stop stops the chaos controller.
func (c *ChaosController) Stop() {
	c.stopped = true
}

// getName returns a namespaced string monkey name.
func getName(name types.NamespacedName) string {
	return fmt.Sprintf("%s-%s", name.Namespace, name.Name)
}

// GetMonkey returns a named chaos MonkeyController if one exists, otherwise nil.
func (c *ChaosController) RemoveMonkey(name types.NamespacedName) *MonkeyController {
	m := c.monkeys[getName(name)]
	delete(c.monkeys, getName(name))
	return m
}

// GetOrCreateMonkey returns a named multiton chaos MonkeyController, creating one if one does not already exist.
func (c *ChaosController) GetOrCreateMonkey(name types.NamespacedName, monkey *v1alpha1.ChaosMonkey) *MonkeyController {
	m := c.monkeys[getName(name)]
	if m == nil {
		m = c.newMonkey(monkey)
		c.monkeys[getName(name)] = m
	}
	return m
}

// newMonkey returns a new MonkeyController for the given ChaosMonkey configuration.
func (c *ChaosController) newMonkey(monkey *v1alpha1.ChaosMonkey) *MonkeyController {
	return &MonkeyController{
		monkey: monkey,
		context: Context{
			client:  c.context.client,
			scheme:  c.context.scheme,
			kubecli: c.context.kubecli,
			config:  c.context.config,
			log:     log.WithValues("namespace", monkey.Namespace, "monkey", monkey.Name),
		},
		selector: func() ([]v1.Pod, error) {
			return c.selectPods(monkey, monkey.Spec.Selector)
		},
		rate:    time.Duration(*monkey.Spec.RateSeconds * int64(time.Second)),
		period:  time.Duration(*monkey.Spec.PeriodSeconds * int64(time.Second)),
		jitter:  *monkey.Spec.Jitter,
		stopped: make(chan struct{}),
	}
}

// selectPods selects a slice of pods using configured label and field selectors.
func (c *ChaosController) selectPods(monkey *v1alpha1.ChaosMonkey, selector *v1alpha1.MonkeySelector) ([]v1.Pod, error) {
	listOptions := runtimeclient.ListOptions{
		Namespace:     monkey.Namespace,
		LabelSelector: c.newLabelSelector(monkey, selector),
		FieldSelector: c.newFieldSelector(selector),
	}

	// Get a list of pods in the current cluster.
	pods := &v1.PodList{}
	err := c.context.client.List(context.TODO(), &listOptions, pods)
	if err != nil {
		return nil, err
	}
	return pods.Items, nil
}

// newLabelSelector returns a new label selector derived from the given MonkeySelector.
func (c *ChaosController) newLabelSelector(monkey *v1alpha1.ChaosMonkey, selector *v1alpha1.MonkeySelector) labels.Selector {
	labelSelector := labels.NewSelector()
	if selector != nil && selector.LabelSelector != nil {
		for label, value := range selector.MatchLabels {
			r, err := labels.NewRequirement(label, selection.Equals, []string{value})
			if err == nil {
				labelSelector = labelSelector.Add(*r)
			}
		}

		for _, requirement := range selector.MatchExpressions {
			var operator selection.Operator
			switch requirement.Operator {
			case metav1.LabelSelectorOpIn:
				operator = selection.In
			case metav1.LabelSelectorOpNotIn:
				operator = selection.NotIn
			case metav1.LabelSelectorOpExists:
				operator = selection.Exists
			case metav1.LabelSelectorOpDoesNotExist:
				operator = selection.DoesNotExist
			}

			r, err := labels.NewRequirement(requirement.Key, operator, requirement.Values)
			if err == nil {
				labelSelector = labelSelector.Add(*r)
			}
		}
	}
	return labelSelector
}

// newFieldSelector returns a new field selector derived from the given MonkeySelector.
func (c *ChaosController) newFieldSelector(selector *v1alpha1.MonkeySelector) fields.Selector {
	if selector != nil && selector.PodSelector != nil {
		podNames := map[string]string{}
		for _, name := range selector.MatchPods {
			podNames["metadata.name"] = name
		}
		return fields.SelectorFromSet(podNames)
	}
	return nil
}

// MonkeyController manages the lifecycle of a single ChaosMonkey.
type MonkeyController struct {
	monkey   *v1alpha1.ChaosMonkey
	selector func() ([]v1.Pod, error)
	Started  bool
	context  Context
	stopped  chan struct{}
	mu       sync.Mutex
	rate     time.Duration
	period   time.Duration
	jitter   float64
}

// Start starts the chaos monkey and workers.
func (m *MonkeyController) Start() error {
	m.mu.Lock()

	defer utilruntime.HandleCrash()

	if m.period == 0 {
		m.period = 1 * time.Minute
	}

	m.context.log.Info("Starting monkey")
	m.setRunning(true)

	go func() {
		// wait.Until will immediately trigger the monkey, so we need to wait for the configured rate first.
		t := time.NewTimer(m.rate)
		<-t.C

		// Run the monkey every rate for the configured period until the monkey is stopped.
		wait.JitterUntil(func() {
			var wg wait.Group
			defer wg.Wait()

			m.context.log.Info("Running monkey")
			handler := m.newHandler(m.monkey)

			pods, err := m.selector()
			if err != nil {
				m.context.log.Error(err, "Failed to select pods")
			} else if len(pods) == 0 {
				m.context.log.Info("No pods selected")
			} else {
				err = handler.create(pods)
				if err != nil {
					m.context.log.Error(err, "Failed to create tasks")
				}
			}

			t := time.NewTimer(m.period)
			select {
			case <-m.stopped:
				m.context.log.Info("Monkey stopped")
				err = handler.delete(pods)
				if err != nil {
					m.context.log.Error(err, "Failed to stop tasks")
				}
			case <-t.C:
				m.context.log.Info("Monkey complete")
				err = handler.delete(pods)
				if err != nil {
					m.context.log.Error(err, "Failed to stop tasks")
				}
			}
		}, m.rate, m.jitter, true, m.stopped)
	}()

	m.Started = true
	m.mu.Unlock()

	return nil
}

// newHandler returns a new MonkeyHandler for the given ChaosMonkey configuration.
func (c *MonkeyController) newHandler(monkey *v1alpha1.ChaosMonkey) MonkeyHandler {
	ctx := c.context.new(c.context.log.WithValues("monkey", monkey.Name))
	if monkey.Spec.Crash != nil {
		return &CrashMonkey{
			context: ctx,
			monkey:  monkey,
			time:    time.Now(),
		}
	} else if monkey.Spec.Partition != nil {
		return &PartitionMonkey{
			context: ctx,
			monkey:  monkey,
			time:    time.Now(),
		}
	} else if monkey.Spec.Stress != nil {
		return &StressMonkey{
			context: ctx,
			monkey:  monkey,
			time:    time.Now(),
		}
	} else {
		return &NilMonkey{}
	}
}

// Stop stops the chaos monkey and workers.
func (m *MonkeyController) Stop() {
	m.context.log.Info("Stopping monkey")
	close(m.stopped)
}

// setRunning updates the ChaosMonkey status to indicate that the monkey is currently running.
func (m *MonkeyController) setRunning(running bool) {
	m.monkey.Status.Running = running
	err := m.context.client.Status().Update(context.TODO(), m.monkey)
	if err != nil {
		m.context.log.Error(err, "Failed to update monkey status")
	}
}

// MonkeyHandler provides a runnable interface for chaos monkey implementations.
type MonkeyHandler interface {
	create([]v1.Pod) error
	delete([]v1.Pod) error
}

// NilMonkey is an invalid chaos monkey handler.
type NilMonkey struct{}

func (m *NilMonkey) create(pods []v1.Pod) error {
	return nil
}

func (m *NilMonkey) delete(pods []v1.Pod) error {
	return nil
}

func getLabels(monkey *v1alpha1.ChaosMonkey) map[string]string {
	return map[string]string{
		"app":    "chaos-controller",
		"monkey": monkey.Name,
	}
}
