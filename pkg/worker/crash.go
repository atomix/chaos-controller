/*
 * Copyright (c) 2019-present Open Networking Foundation
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

package worker

import (
	"context"
	"docker.io/go-docker"
	dockertypes "docker.io/go-docker/api/types"
	"docker.io/go-docker/api/types/filters"
	"fmt"
	"github.com/atomix/chaos-controller/pkg/apis/chaos/v1alpha1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// addCrashController adds a Crash resource controller to the given controller
func addCrashController(mgr manager.Manager) error {
	r := &ReconcileCrash{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: mgr.GetConfig(),
		kube:   kubernetes.NewForConfigOrDie(mgr.GetConfig()),
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
	return err
}

var _ reconcile.Reconciler = &ReconcileCrash{}

// ReconcileCrash reconciles a Crash object
type ReconcileCrash struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
	kube   kubernetes.Interface
}

// Reconcile reads that state of the cluster for a ChaosMonkey object and makes changes based on the state read
// and what is in the ChaosMonkey.Spec
func (r *ReconcileCrash) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("namespace", request.Namespace, "name", request.Name)
	logger.Info("Reconciling Crash")

	// Fetch the ChaosMonkey instance
	instance := &v1alpha1.Crash{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	// If the partition has not yet been started, update the status.
	if instance.Status.Phase == "" {
		if err := r.setStarted(instance); err != nil {
			return reconcile.Result{}, err
		}
	}

	// If the status is running, execute the crash
	if instance.Status.Phase == v1alpha1.PhaseStarted {
		err = r.crash(instance)
	}
	return reconcile.Result{}, err
}

func (r *ReconcileCrash) crash(crash *v1alpha1.Crash) error {
	switch crash.Spec.CrashStrategy.Type {
	case v1alpha1.CrashPod:
		log.Info("CrashPod")
		return r.crashPod(crash)
	case v1alpha1.CrashContainer:
		log.Info("CrashContainer")
		return r.crashContainer(crash)
	default:
		return nil
	}
}

func (r *ReconcileCrash) setStarted(crash *v1alpha1.Crash) error {
	crash.Status.Phase = v1alpha1.PhaseStarted
	return r.client.Status().Update(context.TODO(), crash)
}

func (r *ReconcileCrash) setRunning(crash *v1alpha1.Crash) error {
	crash.Status.Phase = v1alpha1.PhaseRunning
	return r.client.Status().Update(context.TODO(), crash)
}

func (r *ReconcileCrash) setComplete(crash *v1alpha1.Crash) error {
	crash.Status.Phase = v1alpha1.PhaseComplete
	return r.client.Status().Update(context.TODO(), crash)
}

func (r *ReconcileCrash) getLocalPod(crash *v1alpha1.Crash) (*v1.Pod, error) {
	// Get the pod to determine whether the pod is running on this node
	pod := &v1.Pod{}
	err := r.client.Get(context.TODO(), types.NamespacedName{crash.Namespace, crash.Spec.PodName}, pod)
	if err != nil {
		return nil, err
	}

	// Compare the pod's node name to the local node name
	nodeName := pod.Spec.NodeName
	if nodeName != os.Getenv("NODE_NAME") {
		return nil, nil
	}
	return pod, nil
}

func (r *ReconcileCrash) crashPod(crash *v1alpha1.Crash) error {
	logger := log.WithValues("namespace", crash.Namespace, "name", crash.Name, "pod", crash.Spec.PodName)

	// Check if the pod belongs to the local node and load the pod
	pod, err := r.getLocalPod(crash)
	if err != nil {
		logger.Info("Not local pod")
		return err
	}

	// Update the crash status to running
	err = r.setRunning(crash)
	if err != nil {
		return err
	}

	// Delete the pod
	logger.Info("Deleting pod")
	err = r.client.Delete(context.TODO(), pod)
	if err != nil {
		return err
	}

	// Update the crash status to complete
	err = r.setComplete(crash)
	if err != nil {
		return err
	}
	return nil
}

func (r *ReconcileCrash) crashContainer(crash *v1alpha1.Crash) error {
	logger := log.WithValues("namespace", crash.Namespace, "name", crash.Name, "pod", crash.Spec.PodName)

	// Check if the pod belongs to the local node and load the pod
	_, err := r.getLocalPod(crash)
	if err != nil {
		logger.Info("Not local pod")
		return err
	}

	// Update the crash status to running
	err = r.setRunning(crash)
	if err != nil {
		return err
	}

	// Create a new Docker client
	cli, err := docker.NewEnvClient()
	if err != nil {
		logger.Error(err, "Cannot connect to Docker daemon")
		return err
	}

	// Find the Docker container for the pod
	containers, err := cli.ContainerList(context.Background(), dockertypes.ContainerListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("%s=%s", "io.kubernetes.pod.name", crash.Spec.PodName)),
			filters.Arg("label", fmt.Sprintf("%s=%s", "io.kubernetes.pod.namespace", crash.Namespace)),
		),
	})
	if err != nil {
		logger.Error(err, "Could not locate pod containers")
		return err
	}

	// If no containers matching the pod were found, skip it
	if len(containers) == 0 {
		return nil
	}

	logger.Info("Killing pod containers")
	for _, c := range containers {
		err = cli.ContainerKill(context.Background(), c.ID, "KILL")
		if err != nil {
			return err
		}
	}

	// Update the crash status to complete
	err = r.setComplete(crash)
	if err != nil {
		return err
	}
	return nil
}
