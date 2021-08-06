package runtime

import (
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
)

var doOnce sync.Once
var scheme *runtime.Scheme

type NScheme struct {
	*runtime.Scheme
}

func Scheme() *runtime.Scheme {
	doOnce.Do(func() {
		scheme = runtime.NewScheme()
	})
	return scheme
}
