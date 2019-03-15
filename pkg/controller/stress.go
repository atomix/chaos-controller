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

type StressMonkey struct {
	context Context
	monkey  *v1alpha1.ChaosMonkey
	time    time.Time
}

func (m *StressMonkey) getHash() string {
	return util.ComputeHash(m.time)
}

func (m *StressMonkey) getStressName(pod v1.Pod) string {
	return fmt.Sprintf("%s-%s", m.monkey.Name, util.ComputeHash(pod.Name, m.time))
}

func (m *StressMonkey) getNamespacedName(pod v1.Pod) types.NamespacedName {
	return types.NamespacedName{
		Namespace: m.monkey.Namespace,
		Name:      m.getStressName(pod),
	}
}

func (m *StressMonkey) create(pods []v1.Pod) error {
	var selected []v1.Pod
	if m.monkey.Spec.Stress.StressStrategy.Type == v1alpha1.StressRandom {
		selected = []v1.Pod{pods[rand.Intn(len(pods))]}
	} else {
		selected = pods
	}

	for _, pod := range selected {
		stress := &v1alpha1.Stress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      m.getStressName(pod),
				Namespace: pod.Namespace,
				Labels:    getLabels(m.monkey, m.time),
			},
			Spec: v1alpha1.StressSpec{
				PodName: pod.Name,
				IO:      m.monkey.Spec.Stress.IO,
				CPU:     m.monkey.Spec.Stress.CPU,
				Memory:  m.monkey.Spec.Stress.Memory,
				HDD:     m.monkey.Spec.Stress.HDD,
				Network: m.monkey.Spec.Stress.Network,
			},
		}
		if err := controllerutil.SetControllerReference(m.monkey, stress, m.context.scheme); err != nil {
			return err
		}
		return m.context.client.Create(context.TODO(), stress)
	}
	return nil
}

func (m *StressMonkey) delete(pods []v1.Pod) error {
	// Create list options from the labels assigned to stresses by this monkey.
	listOptions := client.ListOptions{
		Namespace:     m.monkey.Namespace,
		LabelSelector: labels.SelectorFromSet(getLabels(m.monkey, m.time)),
	}

	// Load the list of stresses created by this monkey.
	stresses := &v1alpha1.NetworkPartitionList{}
	if err := m.context.client.List(context.TODO(), &listOptions, stresses); err != nil {
		return err
	}

	// Iterate through stresses and update the phase to PhaseStopped.
	for _, stress := range stresses.Items {
		stress.Status.Phase = v1alpha1.PhaseStopped
		if err := m.context.client.Status().Update(context.TODO(), &stress); err != nil {
			return err
		}
	}
	return nil
}
