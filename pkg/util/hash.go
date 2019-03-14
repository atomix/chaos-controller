package util

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"hash"
	"hash/fnv"
	"k8s.io/apimachinery/pkg/util/rand"
)

func ComputeHash(objects ...interface{}) string {
	hasher := fnv.New32a()
	deepHashObject(hasher, objects)
	return rand.SafeEncodeString(fmt.Sprint(hasher.Sum32()))
}

func deepHashObject(hasher hash.Hash, objects ...interface{}) {
	hasher.Reset()
	printer := spew.ConfigState{
		Indent:         " ",
		SortKeys:       true,
		DisableMethods: true,
		SpewKeys:       true,
	}
	for _, obj := range objects {
		printer.Fprintf(hasher, "%#v", obj)
	}
}
