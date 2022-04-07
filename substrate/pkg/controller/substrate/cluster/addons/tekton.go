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

package addons

import (
	"context"
	"fmt"

	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/utils/kubectl"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Tekton struct {
}

func (t *Tekton) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if !substrate.Status.IsReady() {
		return reconcile.Result{Requeue: true}, nil
	}
	client, err := kubectl.NewClient(*substrate.Status.Cluster.KubeConfig)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("initializing client, %w", err)
	}
	for _, file := range []string{
		pipeline,
		triggers,
		dashboard,
	} {
		if err := client.ApplyYAML(ctx, []byte(file)); err != nil {
			return reconcile.Result{}, fmt.Errorf("applying tekton, %w", err)
		}
	}
	return reconcile.Result{}, nil
}

func (t *Tekton) Delete(_ context.Context, _ *v1alpha1.Substrate) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
