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

type CrashMonkey struct {
	context Context
	monkey  *v1alpha1.ChaosMonkey
	time    time.Time
}

func (m *CrashMonkey) getHash() string {
	return util.ComputeHash(m.time)
}

func (m *CrashMonkey) getCrashName(pod v1.Pod) string {
	return fmt.Sprintf("%s-%s", m.monkey.Name, util.ComputeHash(pod.Name, m.time))
}

func (m *CrashMonkey) getNamespacedName(pod v1.Pod) types.NamespacedName {
	return types.NamespacedName{
		Namespace: m.monkey.Namespace,
		Name:      m.getCrashName(pod),
	}
}

func (m *CrashMonkey) create(pods []v1.Pod) error {
	pod := pods[rand.Intn(len(pods))]
	crash := &v1alpha1.Crash{
		ObjectMeta: metav1.ObjectMeta{
			Name:      m.getCrashName(pod),
			Namespace: pod.Namespace,
			Labels:    getLabels(m.monkey),
		},
		Spec: v1alpha1.CrashSpec{
			PodName:       pod.Name,
			CrashStrategy: m.monkey.Spec.Crash.CrashStrategy,
		},
	}
	if err := controllerutil.SetControllerReference(m.monkey, crash, m.context.scheme); err != nil {
		return err
	}
	return m.context.client.Create(context.TODO(), crash)
}

func (m *CrashMonkey) delete(pods []v1.Pod) error {
	for _, pod := range pods {
		crash := &v1alpha1.Crash{}
		err := m.context.client.Get(context.TODO(), m.getNamespacedName(pod), crash)
		if err != nil {
			if !errors.IsNotFound(err) {
				return err
			}
			return nil
		}

		if crash.Status.Phase != v1alpha1.PhaseComplete {
			crash.Status.Phase = v1alpha1.PhaseStopped
			err = m.context.client.Status().Update(context.TODO(), crash)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
