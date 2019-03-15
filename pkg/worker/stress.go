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
	"docker.io/go-docker/api/types/container"
	"docker.io/go-docker/api/types/filters"
	"docker.io/go-docker/api/types/network"
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
	"strings"
)

// addStressController adds a Stress resource controller to the given controller
func addStressController(mgr manager.Manager) error {
	r := &ReconcileStress{
		client: mgr.GetClient(),
		scheme: mgr.GetScheme(),
		config: mgr.GetConfig(),
		kube:   kubernetes.NewForConfigOrDie(mgr.GetConfig()),
	}

	c, err := controller.New("stress", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to Crash resource
	err = c.Watch(&source.Kind{Type: &v1alpha1.Stress{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}
	return err
}

var _ reconcile.Reconciler = &ReconcileStress{}

// ReconcileNetworkPartition reconciles a Crash object
type ReconcileStress struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client client.Client
	scheme *runtime.Scheme
	config *rest.Config
	kube   kubernetes.Interface
}

// Reconcile reads that state of the cluster for a Stress object and makes changes based on the state read
// and what is in the ChaosMonkey.Spec
func (r *ReconcileStress) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	logger := log.WithValues("namespace", request.Namespace, "name", request.Name)
	logger.Info("Reconciling Stress")

	// Fetch the Stress instance
	instance := &v1alpha1.Stress{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			if err := r.cancel(request.NamespacedName); err != nil {
				return reconcile.Result{}, err
			}
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

	// If the stress has been started, start the stressers.
	if instance.Status.Phase == v1alpha1.PhaseStarted {
		err = r.stress(instance)
	} else if instance.Status.Phase == v1alpha1.PhaseStopped {
		err = r.destress(instance)
	}
	return reconcile.Result{}, err
}

// setStatus sets the given Stress Phase to the given phase.
func (r *ReconcileStress) setStatus(stress *v1alpha1.Stress, phase v1alpha1.Phase) error {
	stress.Status.Phase = phase
	return r.client.Status().Update(context.TODO(), stress)
}

// setStarted sets the given Stress Phase to Started.
func (r *ReconcileStress) setStarted(stress *v1alpha1.Stress) error {
	return r.setStatus(stress, v1alpha1.PhaseStarted)
}

// setRunning sets the given Stress Phase to Running.
func (r *ReconcileStress) setRunning(stress *v1alpha1.Stress) error {
	return r.setStatus(stress, v1alpha1.PhaseRunning)
}

// setRunning sets the given Stress Phase to Complete.
func (r *ReconcileStress) setComplete(stress *v1alpha1.Stress) error {
	return r.setStatus(stress, v1alpha1.PhaseComplete)
}

// getPodName returns the namespaced name of the pod to which to apply the given Stress.
func (r *ReconcileStress) getPodName(stress *v1alpha1.Stress) types.NamespacedName {
	return types.NamespacedName{
		Namespace: stress.Namespace,
		Name:      stress.Spec.PodName,
	}
}

// getLocalPod returns the Pod to which the given Stress refers if the pod is
// running on the local node, otherwise nil.
func (r *ReconcileStress) getLocalPod(stress *v1alpha1.Stress) (*v1.Pod, error) {
	// Get the pod to determine whether the pod is running on this node
	pod := &v1.Pod{}
	err := r.client.Get(context.TODO(), r.getPodName(stress), pod)
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

// stress executes the stress function on the local node.
func (r *ReconcileStress) stress(stress *v1alpha1.Stress) error {
	logger := log.WithValues("namespace", stress.Namespace, "name", stress.Name, "pod", stress.Spec.PodName)

	if stress.Spec.IO != nil {
		logger.Info("Stressing I/O")
		if err := r.stressIo(stress); err != nil {
			logger.Error(err, "Failed to stress I/O")
			return err
		}
	}
	if stress.Spec.CPU != nil {
		logger.Info("Stressing CPU")
		if err := r.stressCpu(stress); err != nil {
			logger.Error(err, "Failed to stress CPU")
			return err
		}
	}
	if stress.Spec.Memory != nil {
		logger.Info("Stressing memory")
		if err := r.stressMemory(stress); err != nil {
			logger.Error(err, "Failed to stress memory")
			return err
		}
	}
	if stress.Spec.HDD != nil {
		logger.Info("Stressing HDD")
		if err := r.stressHdd(stress); err != nil {
			logger.Error(err, "Failed to stress HDD")
			return err
		}
	}
	if stress.Spec.Network != nil {
		logger.Info("Stressing network")
		if err := r.stressNetwork(stress); err != nil {
			logger.Error(err, "Failed to stress network")
			return err
		}
	}
	return r.setRunning(stress)
}

// stressIo creates a stress container spinning on I/O.
func (r *ReconcileStress) stressIo(stress *v1alpha1.Stress) error {
	args := []string{"--io", fmt.Sprintf("%d", *stress.Spec.IO.Workers)}
	return r.execStress(stress, args)
}

// stressCpu creates a stress container spinning on CPU.
func (r *ReconcileStress) stressCpu(stress *v1alpha1.Stress) error {
	args := []string{"--cpu", fmt.Sprintf("%d", *stress.Spec.CPU.Workers)}
	return r.execStress(stress, args)
}

// stressMemory creates a stress container spinning on allocating/deallocating memory.
func (r *ReconcileStress) stressMemory(stress *v1alpha1.Stress) error {
	args := []string{"--vm", fmt.Sprintf("%d", *stress.Spec.Memory.Workers)}
	return r.execStress(stress, args)
}

// stressHdd creates a stress container spinning on HDD.
func (r *ReconcileStress) stressHdd(stress *v1alpha1.Stress) error {
	args := []string{"--hdd", fmt.Sprintf("%d", *stress.Spec.HDD.Workers)}
	return r.execStress(stress, args)
}

// stressNetwork creates traffic control rules for the stressed pod's containers on the host.
func (r *ReconcileStress) stressNetwork(stress *v1alpha1.Stress) error {
	ifaces, err := getInterfaces(r.getPodName(stress))
	if err != nil {
		return err
	}

	for _, iface := range ifaces {
		var args []string
		if stress.Spec.Network != nil {
			args := []string{"tc", "qdisc", "add", "dev", iface, "root", "netem", "delay"}
			args = append(args, fmt.Sprintf("%dms", *stress.Spec.Network.LatencyMilliseconds))
			if stress.Spec.Network.Jitter != nil {
				args = append(args, fmt.Sprintf("%dms", int(*stress.Spec.Network.Jitter*float64(*stress.Spec.Network.LatencyMilliseconds))))
				if stress.Spec.Network.Correlation != nil {
					args = append(args, fmt.Sprintf("%d%%", int(*stress.Spec.Network.Correlation*100)))
				}
			}
			if stress.Spec.Network.Distribution != nil {
				args = append(args, "distribution")
				args = append(args, string(*stress.Spec.Network.Distribution))
			}
		}

		_, err := execCommand("bash", "-c", strings.Join(args, " "))
		if err != nil {
			return err
		}
	}
	return nil
}

// execStress creates and runs a stress container using the given command.
func (r *ReconcileStress) execStress(stress *v1alpha1.Stress, command []string) error {
	cli, err := docker.NewEnvClient()
	if err != nil {
		return err
	}

	containers, err := cli.ContainerList(context.Background(), dockertypes.ContainerListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("%s=%s", "io.kubernetes.pod.name", stress.Spec.PodName)),
			filters.Arg("label", fmt.Sprintf("%s=%s", "io.kubernetes.pod.namespace", stress.Namespace)),
		),
	})
	if err != nil {
		return err
	}

	var pauseContainer *dockertypes.Container
	for _, c := range containers {
		if strings.Contains(c.Image, "k8s.gcr.io/pause") {
			pauseContainer = &c
			break
		}
	}

	if pauseContainer == nil {
		return nil
	}

	config := &container.Config{
		Image: "progrium/stress:latest",
		Cmd:   command,
		Labels: map[string]string{
			"io.atomix.chaos.stress.name":      stress.Name,
			"io.atomix.chaos.stress.namespace": stress.Namespace,
		},
	}

	hostConfig := &container.HostConfig{
		NetworkMode: "host",
		Privileged:  true,
		PidMode:     container.PidMode(fmt.Sprintf("container:%s", pauseContainer.ID)),
	}

	networkingConfig := &network.NetworkingConfig{}

	create, err := cli.ContainerCreate(context.Background(), config, hostConfig, networkingConfig, stress.Name)
	if err != nil {
		return err
	}

	return cli.ContainerStart(context.Background(), create.ID, dockertypes.ContainerStartOptions{})
}

// destress stops stress containers and restores traffic to the given Stress's pod.
func (r *ReconcileStress) destress(stress *v1alpha1.Stress) error {
	if err := r.destressContainers(stress); err != nil {
		return err
	}
	if err := r.destressNetwork(stress); err != nil {
		return err
	}
	return r.setComplete(stress)
}

// destressContainers stops stress containers for the given Stress.
func (r *ReconcileStress) destressContainers(stress *v1alpha1.Stress) error {
	return r.cancelContainers(types.NamespacedName{
		Namespace: stress.Namespace,
		Name:      stress.Name,
	})
}

// destressNetwork restores traffic control for the given Stress pod.
func (r *ReconcileStress) destressNetwork(stress *v1alpha1.Stress) error {
	ifaces, err := getInterfaces(r.getPodName(stress))
	if err != nil {
		return err
	}

	for _, iface := range ifaces {
		var args []string
		if stress.Spec.Network != nil {
			args := []string{"tc", "qdisc", "del", "dev", iface}
			args = append(args, fmt.Sprintf("%dms", *stress.Spec.Network.LatencyMilliseconds))
			if stress.Spec.Network.Jitter != nil {
				args = append(args, fmt.Sprintf("%dms", int(*stress.Spec.Network.Jitter*float64(*stress.Spec.Network.LatencyMilliseconds))))
				if stress.Spec.Network.Correlation != nil {
					args = append(args, fmt.Sprintf("%d%%", int(*stress.Spec.Network.Correlation*100)))
				}
			}
			if stress.Spec.Network.Distribution != nil {
				args = append(args, "distribution")
				args = append(args, string(*stress.Spec.Network.Distribution))
			}
		}

		_, err := execCommand("bash", "-c", strings.Join(args, " "))
		if err != nil {
			return err
		}
	}
	return nil
}

// cancel stops stress containers created by the given Stress.
func (r *ReconcileStress) cancel(name types.NamespacedName) error {
	return r.cancelContainers(name)
}

// cancelContainers stops stress containers created by the given Stress.
func (r *ReconcileStress) cancelContainers(name types.NamespacedName) error {
	cli, err := docker.NewEnvClient()
	if err != nil {
		return err
	}

	containers, err := cli.ContainerList(context.Background(), dockertypes.ContainerListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("%s=%s", "io.atomix.chaos.stress.name", name.Name)),
			filters.Arg("label", fmt.Sprintf("%s=%s", "io.atomix.chaos.stress.namespace", name.Namespace)),
		),
	})
	if err != nil {
		return err
	}

	if len(containers) == 0 {
		return nil
	}

	for _, c := range containers {
		if err = cli.ContainerStop(context.Background(), c.ID, nil); err != nil {
			return err
		}
	}
	return nil
}
