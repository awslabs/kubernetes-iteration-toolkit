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

package etcd

import (
	"context"

	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/kubeprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/keypairs"

	"go.uber.org/zap"
)

const (
	etcdRootCACommonName = "etcd/ca"
)

type Controller struct {
	kubeClient *kubeprovider.Client
	keypairs   *keypairs.Provider
}

type reconciler func(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (err error)

func New(kubeclient *kubeprovider.Client) *Controller {
	return &Controller{kubeClient: kubeclient, keypairs: keypairs.Reconciler(kubeclient)}
}

func (c *Controller) Reconcile(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (err error) {
	for _, reconcile := range []reconciler{
		c.reconcileService,
		c.reconcileSecrets,
		c.reconcileBootstrapConfig,
		c.reconcileStatefulSet,
		c.reconcilePersistentVolumeClaims,
	} {
		if err := reconcile(ctx, controlPlane); err != nil {
			return err
		}
	}
	zap.S().Infof("[%v] etcd reconciled", controlPlane.ClusterName())
	return nil
}

func (c *Controller) Finalize(_ context.Context, _ *v1alpha1.ControlPlane) (err error) {
	return nil
}
