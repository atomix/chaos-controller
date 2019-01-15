package controller

import (
	"context"
	"github.com/atomix/chaos-operator/pkg/apis/chaos/v1alpha1"
	"github.com/atomix/chaos-operator/pkg/chaos"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller_chaosmonkey")

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, AddController)
}

// AddToManagerFuncs is a list of functions to add all Controllers to the Manager
var AddToManagerFuncs []func(manager.Manager) error

// AddToManager adds all Controllers to the Manager
func AddToManager(m manager.Manager) error {
	for _, f := range AddToManagerFuncs {
		if err := f(m); err != nil {
			return err
		}
	}
	return nil
}

// Add creates a new ChaosMonkey Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func AddController(mgr manager.Manager) error {
	ch := chaos.New(mgr.GetClient(), mgr.GetScheme(), mgr.GetConfig())
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
	c, err := controller.New("chaosmonkey-controller", mgr, controller.Options{Reconciler: r})
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
	chaos  *chaos.ChaosController
}

// Reconcile reads that state of the cluster for a ChaosMonkey object and makes changes based on the state read
// and what is in the ChaosMonkey.Spec
func (r *ReconcileChaosMonkey) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ChaosMonkey")

	// Fetch the ChaosMonkey instance
	instance := &v1alpha1.ChaosMonkey{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			monkey := r.chaos.GetMonkey(request.NamespacedName)
			if monkey != nil {
				monkey.Stop()
			}
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	monkey := r.chaos.GetOrCreateMonkey(request.NamespacedName, instance)
	if !monkey.Started {
		err := monkey.Start()
		if err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, err
}
