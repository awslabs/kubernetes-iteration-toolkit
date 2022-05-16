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
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/utils/helm"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type AWSVPCCNI struct{}

func (a *AWSVPCCNI) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if !substrate.Status.IsReady() {
		return reconcile.Result{Requeue: true}, nil
	}
	if err := helm.NewClient(*substrate.Status.Cluster.KubeConfig).Apply(ctx, &helm.Chart{
		Namespace:  "kube-system",
		Name:       "aws-vpc-cni",
		Repository: "https://aws.github.io/eks-charts",
		Version:    "1.1.16",
	}); err != nil {
		return reconcile.Result{}, fmt.Errorf("applying chart, %w", err)
	}
	return reconcile.Result{}, nil
}

func (a *AWSVPCCNI) Delete(_ context.Context, _ *v1alpha1.Substrate) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
