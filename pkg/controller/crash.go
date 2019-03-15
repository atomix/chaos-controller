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
	"math/rand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"time"
)

// CrashMonkey manages Crash resources in the cluster.
type CrashMonkey struct {
	context Context
	monkey  *v1alpha1.ChaosMonkey
	time    time.Time
}

// getHash returns a unique-ish hash for the monkey.
func (m *CrashMonkey) getHash() string {
	return util.ComputeHash(m.time)
}

// getCrashName returns a unique-ish name for the monkey.
func (m *CrashMonkey) getCrashName(pod v1.Pod) string {
	return fmt.Sprintf("%s-%s", m.monkey.Name, util.ComputeHash(pod.Name, m.time))
}

// getNamespacedName returns a NamespacedName for the monkey.
func (m *CrashMonkey) getNamespacedName(pod v1.Pod) types.NamespacedName {
	return types.NamespacedName{
		Namespace: m.monkey.Namespace,
		Name:      m.getCrashName(pod),
	}
}

// selectRandom selects a random pod from the given slice.
func selectRandom(pods []v1.Pod) v1.Pod {
	return pods[rand.Intn(len(pods))]
}

// create selects pods and creates Crash resources for the monkey.
func (m *CrashMonkey) create(pods []v1.Pod) error {
	// Select a random pod to crash.
	pod := selectRandom(pods)

	// Create a Crash resource for the selected pod.
	crash := &v1alpha1.Crash{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.getCrashName(pod),
			Namespace: pod.Namespace,
			Labels:    getLabels(m.monkey, m.time),
		},
		Spec: v1alpha1.CrashSpec{
			PodName:       pod.Name,
			CrashStrategy: m.monkey.Spec.Crash.CrashStrategy,
		},
	}

	// Set the ChaosMonkey resource as the owner of the Crash.
	if err := controllerutil.SetControllerReference(m.monkey, crash, m.context.scheme); err != nil {
		return err
	}
	return m.context.client.Create(context.TODO(), crash)
}

// delete stops all crashes for the selected pods.
// delete does not delete the created Crash resources. Instead, the resource Phase is updated
// to PhaseStopped.
func (m *CrashMonkey) delete(pods []v1.Pod) error {
	// Create list options from the labels assigned to crashes by this monkey.
	listOptions := client.ListOptions{
		Namespace:     m.monkey.Namespace,
		LabelSelector: labels.SelectorFromSet(getLabels(m.monkey, m.time)),
	}

	// Load the list of crashes created by this monkey.
	crashes := &v1alpha1.CrashList{}
	if err := m.context.client.List(context.TODO(), &listOptions, crashes); err != nil {
		return err
	}

	// Iterate through crashes and update the phase to PhaseStopped.
	for _, crash := range crashes.Items {
		crash.Status.Phase = v1alpha1.PhaseStopped
		if err := m.context.client.Status().Update(context.TODO(), &crash); err != nil {
			return err
		}
	}
	return nil
}
