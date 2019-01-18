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
	"fmt"
	"github.com/atomix/chaos-controller/pkg/apis/chaos/v1alpha1"
	"k8s.io/api/core/v1"
	"math/rand"
)

type PartitionMonkey struct {
	context  Context
	config   *v1alpha1.PartitionMonkey
}

type PartitionIsolateMonkey struct {
	*PartitionMonkey
}

func (m *PartitionIsolateMonkey) run(pods []v1.Pod, stop <-chan struct{}) {
	// Choose a random pod to isolate.
	local := pods[rand.Intn(len(pods))]
	container := local.Spec.Containers[0]

	log.Info("Isolating pod", "pod", local.Name, "namespace", local.Namespace)

	// Iterate through each of the remote pods and add a rule to the routing table to drop packets.
	for _, remote := range pods {
		if remote.Status.PodIP != "" {
			_, err := m.context.exec(local, &container, "bash", "-c", fmt.Sprintf("iptables -A INPUT -s %s -j DROP -w", remote.Status.PodIP))
			if err != nil {
				m.context.log.Error(err, "Failed to isolate pod", "pod", local.Name, "namespace", local.Namespace)
			}
		}
	}

	// Wait for the monkey to be stopped.
	<-stop

	log.Info("Healing isolate partition", "pod", local.Name, "namespace", local.Namespace)

	// Iterate through each of the remote pods and remove the routing table rule.
	for _, remote := range pods {
		if remote.Status.PodIP != "" {
			_, err := m.context.exec(local, &container, "bash", "-c", fmt.Sprintf("iptables -D INPUT -s %s -j DROP -w", remote.Status.PodIP))
			if err != nil {
				m.context.log.Error(err, "Failed to isolate pod", "pod", local.Name, "namespace", local.Namespace)
			}
		}
	}
}

type PartitionBridgeMonkey struct {
	*PartitionMonkey
}

func (m *PartitionBridgeMonkey) run(pods []v1.Pod, stop <-chan struct{}) {
	// Choose a random pod to isolate.
	bridgeIdx := rand.Intn(len(pods))
	bridge := pods[bridgeIdx]

	// Split the rest of the nodes into two halves.
	left, right := []v1.Pod{}, []v1.Pod{}
	for i, pod := range pods {
		if i != bridgeIdx {
			if i%2 == 0 {
				left = append(left, pod)
			} else {
				right = append(right, pod)
			}
		}
	}

	log.Info("Bridging pod", "pod", bridge.Name, "namespace", bridge.Namespace)

	// Iterate through both the left and right partitions and partition the nodes from each other.
	for _, leftPod := range left {
		for _, rightPod := range right {
			if leftPod.Status.PodIP != "" {
				_, err := m.context.exec(rightPod, &rightPod.Spec.Containers[0], "bash", "-c", fmt.Sprintf("iptables -A INPUT -s %s -j DROP -w", leftPod.Status.PodIP))
				if err != nil {
					m.context.log.Error(err, "Failed to partition pod", "pod", rightPod.Name, "namespace", rightPod.Namespace)
				}
			}
			if rightPod.Status.PodIP != "" {
				_, err := m.context.exec(leftPod, &leftPod.Spec.Containers[0], "bash", "-c", fmt.Sprintf("iptables -A INPUT -s %s -j DROP -w", rightPod.Status.PodIP))
				if err != nil {
					m.context.log.Error(err, "Failed to partition pod", "pod", leftPod.Name, "namespace", leftPod.Namespace)
				}
			}
		}
	}

	// Wait for the monkey to be stopped.
	<-stop

	log.Info("Healing bridge partition", "pod", bridge.Name, "namespace", bridge.Namespace)

	// Iterate through both the left and right partitions and restore the routing tables.
	for _, leftPod := range left {
		for _, rightPod := range right {
			if leftPod.Status.PodIP != "" {
				_, err := m.context.exec(rightPod, &rightPod.Spec.Containers[0], "bash", "-c", fmt.Sprintf("iptables -D INPUT -s %s -j DROP -w", leftPod.Status.PodIP))
				if err != nil {
					m.context.log.Error(err, "Failed to partition pod", "pod", rightPod.Name, "namespace", rightPod.Namespace)
				}
			}
			if rightPod.Status.PodIP != "" {
				_, err := m.context.exec(leftPod, &leftPod.Spec.Containers[0], "bash", "-c", fmt.Sprintf("iptables -D INPUT -s %s -j DROP -w", rightPod.Status.PodIP))
				if err != nil {
					m.context.log.Error(err, "Failed to partition pod", "pod", leftPod.Name, "namespace", leftPod.Namespace)
				}
			}
		}
	}
}
