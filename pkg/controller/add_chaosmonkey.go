package controller

import (
	"github.com/atomix/chaos-operator/pkg/controller/chaosmonkey"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, chaosmonkey.Add)
}
