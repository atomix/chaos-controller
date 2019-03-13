// NOTE: Boilerplate only.  Ignore this file.

// Package v1alpha1 contains API Schema definitions for the chaos v1alpha1 API group
// +k8s:deepcopy-gen=package,register
// +groupName=chaos.atomix.io
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/runtime/scheme"
)

func AddChaosToScheme(s *runtime.Scheme) error {
	groupVersion := schema.GroupVersion{Group: "chaos.atomix.io", Version: "v1alpha1"}
	schemeBuilder := &scheme.Builder{GroupVersion: groupVersion}
	schemeBuilder.Register(&ChaosMonkey{}, &ChaosMonkeyList{})
	return schemeBuilder.AddToScheme(s)
}

func AddFunctionsToScheme(s *runtime.Scheme) error {
	groupVersion := schema.GroupVersion{Group: "chaos.atomix.io", Version: "v1alpha1"}
	schemeBuilder := &scheme.Builder{GroupVersion: groupVersion}
	schemeBuilder.Register(&Crash{}, &CrashList{})
	schemeBuilder.Register(&NetworkPartition{}, &NetworkPartitionList{})
	schemeBuilder.Register(&Stress{}, &StressList{})
	return schemeBuilder.AddToScheme(s)
}
