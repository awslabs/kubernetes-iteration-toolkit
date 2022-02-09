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
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/bootstraptoken/node"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type RBAC struct {
}

func (r *RBAC) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if !substrate.IsReady() {
		return reconcile.Result{Requeue: true}, nil
	}
	client, err := kubeconfig.ClientSetFromFile(*substrate.Status.Cluster.KubeConfig)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("creating Kube client from admin config, %w", err)
	}
	// Create RBAC rules that makes the bootstrap tokens able to post CSRs
	if err := node.AllowBootstrapTokensToPostCSRs(client); err != nil {
		return reconcile.Result{}, fmt.Errorf("bootstrap tokens to post CSRs, %w", err)
	}
	// Create RBAC rules that makes the bootstrap tokens able to get their CSRs approved automatically
	if err := node.AutoApproveNodeBootstrapTokens(client); err != nil {
		return reconcile.Result{}, fmt.Errorf("node bootstrap tokens, %w", err)
	}
	// Create/update RBAC rules that makes the nodes to rotate certificates and get their CSRs approved automatically
	if err := node.AutoApproveNodeCertificateRotation(client); err != nil {
		return reconcile.Result{}, fmt.Errorf("node certs rotation, %w", err)
	}
	return reconcile.Result{}, nil
}

func (r *RBAC) Delete(_ context.Context, _ *v1alpha1.Substrate) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
