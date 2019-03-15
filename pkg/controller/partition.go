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

package controller

import (
	"context"
	"fmt"
	"github.com/atomix/chaos-controller/pkg/apis/chaos/v1alpha1"
	"github.com/atomix/chaos-controller/pkg/util"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"
)

// PartitionMonkey manages NetworkPartition resources in the cluster.
type PartitionMonkey struct {
	context Context
	monkey  *v1alpha1.ChaosMonkey
	time    time.Time
}

// getHash returns a unique-ish hash for the monkey.
func (m *PartitionMonkey) getHash() string {
	return util.ComputeHash(m.time)
}

// getPartitionName returns a unique-ish name for the monkey.
func (m *PartitionMonkey) getPartitionName(local v1.Pod, remote v1.Pod) string {
	return fmt.Sprintf("%s-%s", m.monkey.Name, util.ComputeHash(local.Name, remote.Name, m.time))
}

// getNamespacedName returns a NamespacedName for the monkey.
func (m *PartitionMonkey) getNamespacedName(local v1.Pod, remote v1.Pod) types.NamespacedName {
	return types.NamespacedName{
		Namespace: m.monkey.Namespace,
		Name:      m.getPartitionName(local, remote),
	}
}

// create selects pods and creates NetworkPartition resources for the monkey.
func (m *PartitionMonkey) create(pods []v1.Pod) error {
	switch m.monkey.Spec.Partition.PartitionStrategy.Type {
	case v1alpha1.PartitionIsolate:
		return m.createIsolate(pods)
	case v1alpha1.PartitionBridge:
		return m.createBridge(pods)
	case v1alpha1.PartitionHalves:
		return m.createHalves(pods)
	default:
		return nil
	}
}

// createIsolate selects a random pod and creates a network partition between the pod
// and all other pods in the set.
func (m *PartitionMonkey) createIsolate(pods []v1.Pod) error {
	// Select a random pod to isolate.
	local := selectRandomPod(pods)

	// Iterate through all pods and isolate the local pod from all remote pods.
	return m.bipartitionAll(pods, []v1.Pod{local})
}

// createBridge splits the set of pods into two partitions, selecting a random pod
// to maintain communication with both partitions.
func (m *PartitionMonkey) createBridge(pods []v1.Pod) error {
	// Choose a random pod to isolate.
	bridge := selectRandomPod(pods)

	log.Info("Bridging pod", "pod", bridge.Name, "namespace", bridge.Namespace)

	// Split the rest of the nodes into two halves.
	leftPods, rightPods := []v1.Pod{}, []v1.Pod{}
	for i, pod := range pods {
		if pod.Name != bridge.Name {
			if i%2 == 0 {
				leftPods = append(leftPods, pod)
			} else {
				rightPods = append(rightPods, pod)
			}
		}
	}

	// Iterate through both the left and right partitions and partition the nodes from each other.
	return m.bipartitionAll(leftPods, rightPods)
}

// createHalves splits the set of pods into two partitions, cutting off communication
// across the partitions.
func (m *PartitionMonkey) createHalves(pods []v1.Pod) error {
	// Split the set of pods into two halves.
	leftPods, rightPods := []v1.Pod{}, []v1.Pod{}
	for i, pod := range pods {
		if i%2 == 0 {
			leftPods = append(leftPods, pod)
		} else {
			rightPods = append(rightPods, pod)
		}
	}

	log.Info("Partitioning cluster into halves")
	return m.bipartitionAll(leftPods, rightPods)
}

// bipartitionAll creates a partition on each of the left and right pods from each other.
func (m *PartitionMonkey) bipartitionAll(leftPods []v1.Pod, rightPods []v1.Pod) error {
	for _, leftPod := range leftPods {
		for _, rightPod := range rightPods {
			if leftPod.Name != rightPod.Name {
				if err := m.bipartition(leftPod, rightPod); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// bipartition creates a partition on both the left and the right pod.
func (m *PartitionMonkey) bipartition(left v1.Pod, right v1.Pod) error {
	// Create a partition on the left pod from the right pod.
	if err := m.createPartition(left, right); err != nil {
		return err
	}

	// Create a partition on the right pod from the left pod.
	if err := m.createPartition(right, left); err != nil {
		return err
	}
	return nil
}

// createPartition creates a NetworkPartition on the local pod from the remote pod.
func (m *PartitionMonkey) createPartition(local v1.Pod, remote v1.Pod) error {
	// Create and label the NetworkPartition.
	partition := &v1alpha1.NetworkPartition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.getPartitionName(local, remote),
			Namespace: m.monkey.Namespace,
			Labels:    getLabels(m.monkey, m.time),
		},
		Spec: v1alpha1.NetworkPartitionSpec{
			PodName:    local.Name,
			SourceName: remote.Name,
		},
	}

	// Set the ChaosMonkey as the owner of the NetworkPartition and create the resource.
	if err := controllerutil.SetControllerReference(m.monkey, partition, m.context.scheme); err != nil {
		return err
	}
	return m.context.client.Create(context.TODO(), partition)
}

// delete stops all partitions for the selected pods.
// delete does not delete the created NetworkPartition resources. Instead, the resource
// Phase is updated to PhaseStopped.
func (m *PartitionMonkey) delete(pods []v1.Pod) error {
	// Create list options from the labels assigned to partitions by this monkey.
	listOptions := client.ListOptions{
		Namespace:     m.monkey.Namespace,
		LabelSelector: labels.SelectorFromSet(getLabels(m.monkey, m.time)),
	}

	// Load the list of partitions created by this monkey.
	partitions := &v1alpha1.NetworkPartitionList{}
	if err := m.context.client.List(context.TODO(), &listOptions, partitions); err != nil {
		return err
	}

	// Iterate through partitions and update the phase to PhaseStopped.
	for _, partition := range partitions.Items {
		partition.Status.Phase = v1alpha1.PhaseStopped
		if err := m.context.client.Status().Update(context.TODO(), &partition); err != nil {
			return err
		}
	}
	return nil
}
