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
	"strings"
)

type StressMonkey struct {
	context  Context
	config   *v1alpha1.StressMonkey
}

func (m *StressMonkey) run(pods []v1.Pod, stop <-chan struct{}) {
	switch m.config.StressStrategy.Type {
	case v1alpha1.StressRandom:
		m.stressRandom(pods, stop)
	case v1alpha1.StressAll:
		m.stressAll(pods, stop)
	}
}

func (m *StressMonkey) stressRandom(pods []v1.Pod, stop <-chan struct{}) {
	// Choose a random node to kill.
	pod := pods[rand.Intn(len(pods))]

	m.stressPod(pod)

	<-stop

	m.destressPod(pod)
}

func (m *StressMonkey) stressAll(pods []v1.Pod, stop <-chan struct{}) {
	for _, pod := range pods {
		m.stressPod(pod)
	}

	<-stop

	for _, pod := range pods {
		m.destressPod(pod)
	}
}

func (m *StressMonkey) stressPod(pod v1.Pod) {
	if m.config.IO != nil || m.config.CPU != nil || m.config.Memory != nil || m.config.HDD != nil {
		// Build a 'stress' command and arguments.
		command := []string{"bash", "-c"}

		stress := []string{"stress"}
		if m.config.IO != nil {
			stress = append(stress, "--io")
			stress = append(stress, fmt.Sprintf("%d", *m.config.IO.Workers))
		}
		if m.config.CPU != nil {
			stress = append(stress, "--cpu")
			stress = append(stress, fmt.Sprintf("%d", *m.config.CPU.Workers))
		}
		if m.config.Memory != nil {
			stress = append(stress, "--vm")
			stress = append(stress, fmt.Sprintf("%d", *m.config.Memory.Workers))
		}
		if m.config.HDD != nil {
			stress = append(stress, "--hdd")
			stress = append(stress, fmt.Sprintf("%d", *m.config.HDD.Workers))
		}

		stress = append(stress, ">")
		stress = append(stress, "/dev/null")
		stress = append(stress, "2>")
		stress = append(stress, "/dev/null")
		stress = append(stress, "&")

		command = append(command, strings.Join(stress, " "))

		container := pod.Spec.Containers[0]
		log.Info("Stressing container", "pod", pod.Name, "namespace", pod.Namespace, "container", container.Name)

		// Execute the command inside the Atomix container.
		_, err := m.context.exec(pod, &container, command...)
		if err != nil {
			m.context.log.Error(err, "Failed to stress container", "pod", pod.Name, "namespace", pod.Namespace, "container", container.Name)
		}
	}

	// If network stress is configured, build a 'tc' command to inject latency into the network.
	if m.config.Network != nil {
		command := []string{"bash", "-c", "tc", "qdisc", "add", "dev", "eth0", "root", "netem", "delay"}
		command = append(command, fmt.Sprintf("%dms", *m.config.Network.LatencyMilliseconds))
		if m.config.Network.Jitter != nil {
			command = append(command, fmt.Sprintf("%dms", int(*m.config.Network.Jitter*float64(*m.config.Network.LatencyMilliseconds))))
			if m.config.Network.Correlation != nil {
				command = append(command, fmt.Sprintf("%d%%", int(*m.config.Network.Correlation*100)))
			}
		}
		if m.config.Network.Distribution != nil {
			command = append(command, "distribution")
			command = append(command, string(*m.config.Network.Distribution))
		}

		container := pod.Spec.Containers[0]
		log.Info("Slowing container network", "pod", pod.Name, "namespace", pod.Namespace, "container", container.Name)

		// Execute the command inside the Atomix container.
		_, err := m.context.exec(pod, &container, command...)
		if err != nil {
			m.context.log.Error(err, "Failed to slow container network", "pod", pod.Name, "namespace", pod.Namespace, "container", container.Name)
		}
	}
}

func (m *StressMonkey) destressPod(pod v1.Pod) {
	// If the 'stress' utility options are enabled, kill the 'stress' process.
	if m.config.IO != nil || m.config.CPU != nil || m.config.Memory != nil || m.config.HDD != nil {
		command := []string{"bash", "-c", "pkill -f stress"}

		container := pod.Spec.Containers[0]
		log.Info("Destressing container", "pod", pod.Name, "namespace", pod.Namespace, "container", container.Name)

		_, err := m.context.exec(pod, &container, command...)
		if err != nil {
			m.context.log.Error(err, "Failed to destress container", "pod", pod.Name, "namespace", pod.Namespace, "container", container.Name)
		}
	}

	// If the network stress options are enabled, delete the 'tc' rule injecting latency.
	if m.config.Network != nil {
		command := []string{"bash", "-c", "tc qdisc del dev eth0 root"}

		container := pod.Spec.Containers[0]
		log.Info("Restoring container network", "pod", pod.Name, "namespace", pod.Namespace, "container", container.Name)

		_, err := m.context.exec(pod, &container, command...)
		if err != nil {
			m.context.log.Error(err, "Failed to restore container network", "pod", pod.Name, "namespace", pod.Namespace, "container", container.Name)
		}
	}
}
