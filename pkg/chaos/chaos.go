/*
 * Copyright 2019 Open Networking Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package chaos

import (
	"context"
	"fmt"
	"github.com/atomix/chaos-controller/pkg/apis/chaos/v1alpha1"
	"github.com/go-logr/logr"
	"k8s.io/api/core/v1"
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
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sync"
	"time"
)

var log = logf.Log.WithName("chaos_controller")

var _ manager.Runnable = &ChaosController{}

// Add creates a new ChaosMonkey Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func AddControllers(mgr manager.Manager) error {
	err := addCrashController(mgr)
	if err != nil {
		return err
	}

	err = addPartitionController(mgr)
	if err != nil {
		return err
	}

	err = addStressController(mgr)
	if err != nil {
		return err
	}
	r := &ReconcileCrash{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: mgr.GetConfig(),
	}

	c, err := controller.New("crash", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Crash resource
	err = c.Watch(&source.Kind{Type: &v1alpha1.Crash{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

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
func (c *ChaosController) GetMonkey(name types.NamespacedName) *MonkeyController {
	return c.monkeys[getName(name)]
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
		client: c.context.client,
		logger: log.WithValues("namespace", monkey.Namespace, "monkey", monkey.Name),
		selector: func() ([]v1.Pod, error) {
			return c.selectPods(monkey, monkey.Spec.Selector)
		},
		rate:    time.Duration(*monkey.Spec.RateSeconds * int64(time.Second)),
		period:  time.Duration(*monkey.Spec.PeriodSeconds * int64(time.Second)),
		jitter:  *monkey.Spec.Jitter,
		handler: c.newHandler(monkey),
		stopped: make(chan struct{}),
	}
}

// newHandler returns a new MonkeyHandler for the given ChaosMonkey configuration.
func (c *ChaosController) newHandler(monkey *v1alpha1.ChaosMonkey) MonkeyHandler {
	ctx := c.context.new(c.context.log.WithValues("monkey", monkey.Name))
	if monkey.Spec.Crash != nil {
		return &CrashMonkey{
			context: ctx,
			monkey:  monkey,
		}
	} else if monkey.Spec.Partition != nil {
		return &PartitionMonkey{
			context: ctx,
			monkey:  monkey,
		}
	} else if monkey.Spec.Stress != nil {
		return &StressMonkey{
			context: ctx,
			monkey:  monkey,
		}
	} else {
		return &NilMonkey{}
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
	if selector != nil {
		if selector.LabelSelector != nil {
			for label, value := range selector.MatchLabels {
				r, err := labels.NewRequirement(label, selection.Equals, []string{value})
				if err == nil {
					labelSelector.Add(*r)
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
					labelSelector.Add(*r)
				}
			}
		}
	}
	return labelSelector
}

// newFieldSelector returns a new field selector derived from the given MonkeySelector.
func (c *ChaosController) newFieldSelector(selector *v1alpha1.MonkeySelector) fields.Selector {
	if selector.PodSelector != nil {
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
	client   runtimeclient.Client
	logger   logr.Logger
	selector func() ([]v1.Pod, error)
	Started  bool
	handler  MonkeyHandler
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

	// Start the SharedIndexInformer factories to begin populating the SharedIndexInformer caches
	m.logger.Info("Starting monkey")

	if m.period == 0 {
		m.period = 1 * time.Minute
	}

	m.logger.Info("Starting worker")

	go func() {
		// wait.Until will immediately trigger the monkey, so we need to wait for the configured rate first.
		t := time.NewTimer(m.rate)
		<-t.C

		// Run the monkey every rate for the configured period until the monkey is stopped.
		wait.JitterUntil(func() {
			var wg wait.Group
			defer wg.Wait()

			stop := make(chan struct{})
			pods, err := m.selector()
			wg.StartWithChannel(stop, func(stop <-chan struct{}) {
				if err != nil {
					m.logger.Error(err, "Failed to select pods")
				} else if len(pods) == 0 {
					m.logger.Info("No pods selected")
				} else {
					m.setRunning(true)
					err = m.handler.create(pods)
					if err != nil {
						m.logger.Error(err, "Failed to create workers")
					}
				}
			})

			t := time.NewTimer(m.period)
			for {
				select {
				case <-m.stopped:
					m.logger.Info("Monkey stopped")
					err = m.handler.delete(pods)
					if err != nil {
						m.logger.Error(err, "Failed to stop workers")
					}
					m.setRunning(false)
					stop <- struct{}{}
					return
				case <-t.C:
					m.logger.Info("Monkey period expired")
					err = m.handler.delete(pods)
					if err != nil {
						m.logger.Error(err, "Failed to stop workers")
					}
					m.setRunning(false)
					stop <- struct{}{}
					return
				}
			}
		}, m.rate, m.jitter, true, m.stopped)
	}()

	m.Started = true
	m.mu.Unlock()

	return nil
}

// Stop stops the chaos monkey and workers.
func (m *MonkeyController) Stop() {
	close(m.stopped)
}

// setRunning updates the ChaosMonkey status to indicate that the monkey is currently running.
func (m *MonkeyController) setRunning(running bool) {
	m.monkey.Status.Running = running
	err := m.client.Update(context.TODO(), m.monkey)
	if err != nil {
		m.logger.Error(err, "Failed to update monkey status")
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
		"app": "chaos-controller",
		"monkey": monkey.Name,
	}
}
