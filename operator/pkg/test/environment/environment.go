/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package environment

import (
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/awslabs/kit/operator/pkg/controllers"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

type Environment struct {
	Client client.Client
	envtest.Environment
}

func New() *Environment {
	return &Environment{
		Environment: envtest.Environment{
			CRDDirectoryPaths: []string{crdFilePath()},
		},
	}
}

func (e *Environment) Start(scheme *apiruntime.Scheme) (err error) {
	// Environment
	if _, err = e.Environment.Start(); err != nil {
		return fmt.Errorf("starting environment, %w", err)
	}
	manager := controllers.NewManagerOrDie(e.Config, controllerruntime.Options{Scheme: scheme})
	go func() {
		err = manager.Start(controllerruntime.SetupSignalHandler())
	}()
	<-manager.Elected()
	e.Client = &FakeKubeClient{manager.GetClient()}
	return
}

func (e *Environment) Stop() error {
	return e.Environment.Stop()
}

func crdFilePath() string {
	_, file, _, _ := runtime.Caller(0)
	p := filepath.Join(filepath.Dir(file), "..", "..", "..")
	return filepath.Join(p, "config/control-plane-crd.yaml")
}
