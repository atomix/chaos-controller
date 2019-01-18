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
	"github.com/atomix/chaos-controller/pkg/apis/chaos/v1alpha1"
	"k8s.io/api/core/v1"
	"math/rand"
)

type CrashMonkey struct {
	context Context
	config  *v1alpha1.CrashMonkey
}

type CrashContainerMonkey struct {
	*CrashMonkey
}

func (m *CrashContainerMonkey) run(pods []v1.Pod, stop <-chan struct{}) {
	// Choose a random node to kill.
	pod := pods[rand.Intn(len(pods))]
	container := pod.Spec.Containers[0]

	log.Info("Killing container", "pod", pod.Name, "namespace", pod.Namespace, "container", container.Name)

	_, err := m.context.exec(pod, &container, "bash", "-c", "/sbin/killall5")
	if err != nil {
		m.context.log.Error(err, "Failed to kill container", "pod", pod.Name, "container", container.Name, "namespace", pod.Namespace)
	}

	<-stop
}

type CrashPodMonkey struct {
	*CrashMonkey
}

func (m *CrashPodMonkey) run(pods []v1.Pod, stop <-chan struct{}) {
	// Choose a random node to kill.
	pod := pods[rand.Intn(len(pods))]

	log.Info("Killing pod", "pod", pod.Name, "namespace", pod.Namespace)
	err := m.context.client.Delete(context.TODO(), &pod)
	if err != nil {
		m.context.log.Error(err, "Failed to kill pod", "pod", pod.Name, "namespace", pod.Namespace)
	}

	<-stop
}
