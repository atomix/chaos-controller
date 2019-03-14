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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"math/rand"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"
)

type PartitionMonkey struct {
	context Context
	monkey  *v1alpha1.ChaosMonkey
	time    time.Time
}

func (m *PartitionMonkey) getHash() string {
	return util.ComputeHash(m.time)
}

func (m *PartitionMonkey) getPartitionName(local v1.Pod, remote v1.Pod) string {
	return fmt.Sprintf("%s-%s", m.monkey.Name, util.ComputeHash(local.Name, remote.Name, m.time))
}

func (m *PartitionMonkey) getNamespacedName(local v1.Pod, remote v1.Pod) types.NamespacedName {
	return types.NamespacedName{
		Namespace: m.monkey.Namespace,
		Name:      m.getPartitionName(local, remote),
	}
}

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

func (m *PartitionMonkey) createIsolate(pods []v1.Pod) error {
	// Select a random pod to isolate.
	local := pods[rand.Intn(len(pods))]

	// Iterate through all pods and isolate the local pod from all remote pods.
	for _, remote := range pods {
		if remote.Name != local.Name {
			err := m.createPartition(local, remote)
			if err != nil {
				return err
			}

			err = m.createPartition(remote, local)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *PartitionMonkey) createBridge(pods []v1.Pod) error {
	// Choose a random pod to isolate.
	bridgeIdx := rand.Intn(len(pods))
	bridge := pods[bridgeIdx]

	log.Info("Bridging pod", "pod", bridge.Name, "namespace", bridge.Namespace)

	// Split the rest of the nodes into two halves.
	leftPods, rightPods := []v1.Pod{}, []v1.Pod{}
	for i, pod := range pods {
		if i != bridgeIdx {
			if i%2 == 0 {
				leftPods = append(leftPods, pod)
			} else {
				rightPods = append(rightPods, pod)
			}
		}
	}

	// Iterate through both the left and right partitions and partition the nodes from each other.
	for _, leftPod := range leftPods {
		for _, rightPod := range rightPods {
			// Create a partition on the left pod from the right pod.
			err := m.createPartition(leftPod, rightPod)
			if err != nil {
				return err
			}

			// Create a partition on the right pod from the left pod.
			err = m.createPartition(rightPod, leftPod)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

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

	// Iterate through both the left and right partitions and partition the nodes from each other.
	for _, leftPod := range leftPods {
		for _, rightPod := range rightPods {
			// Create a partition on the left pod from the right pod.
			err := m.createPartition(leftPod, rightPod)
			if err != nil {
				return err
			}

			// Create a partition on the right pod from the left pod.
			err = m.createPartition(rightPod, leftPod)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *PartitionMonkey) createPartition(local v1.Pod, remote v1.Pod) error {
	partition := &v1alpha1.NetworkPartition{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.getPartitionName(local, remote),
			Namespace: m.monkey.Namespace,
			Labels:    getLabels(m.monkey),
		},
		Spec: v1alpha1.NetworkPartitionSpec{
			PodName:    local.Name,
			SourceName: remote.Name,
		},
	}
	if err := controllerutil.SetControllerReference(m.monkey, partition, m.context.scheme); err != nil {
		return err
	}
	return m.context.client.Create(context.TODO(), partition)
}

func (m *PartitionMonkey) delete(pods []v1.Pod) error {
	// Iterate through all pods and ensure partitions are deleted.
	for _, local := range pods {
		for _, remote := range pods {
			partition := &v1alpha1.NetworkPartition{}
			err := m.context.client.Get(context.TODO(), m.getNamespacedName(local, remote), partition)
			if err != nil {
				if !errors.IsNotFound(err) {
					return err
				}
				return nil
			}

			partition.Status.Phase = v1alpha1.PhaseStopped
			err = m.context.client.Status().Update(context.TODO(), partition)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
