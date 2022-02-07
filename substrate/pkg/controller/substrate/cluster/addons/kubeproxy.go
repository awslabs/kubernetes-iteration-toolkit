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

	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kit/substrate/pkg/controller/substrate/cluster"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/addons/proxy"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type KubeProxy struct {
}

func (k *KubeProxy) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if !substrate.IsReady() {
		return reconcile.Result{Requeue: true}, nil
	}
	client, err := kubeconfig.ClientSetFromFile(*substrate.Status.Cluster.KubeConfig)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("creating Kube client from admin config, %w", err)
	}
	config := cluster.DefaultClusterConfig(substrate)
	if err := proxy.EnsureProxyAddon(&config.ClusterConfiguration, &config.LocalAPIEndpoint, client); err != nil {
		return reconcile.Result{Requeue: true}, fmt.Errorf("deploying kube-proxy addon, %w", err)
	}
	return reconcile.Result{}, nil
}

func (k *KubeProxy) Delete(_ context.Context, _ *v1alpha1.Substrate) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
