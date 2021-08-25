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

package master

import (
	"context"

	"github.com/awslabs/kit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/kubeprovider"
	"github.com/awslabs/kit/operator/pkg/utils/keypairs"
	"github.com/awslabs/kit/operator/pkg/utils/kubeconfigs"
	"github.com/awslabs/kit/operator/pkg/utils/object"
	"github.com/awslabs/kit/operator/pkg/utils/patch"
	"go.uber.org/zap"
)

type Controller struct {
	kubeClient  *kubeprovider.Client
	keypairs    *keypairs.Provider
	kubeConfigs *kubeconfigs.Provider
}

func New(kubeclient *kubeprovider.Client) *Controller {
	return &Controller{
		kubeClient:  kubeclient,
		keypairs:    keypairs.Reconciler(kubeclient),
		kubeConfigs: kubeconfigs.Reconciler(kubeclient),
	}
}

type reconciler func(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (err error)

func (c *Controller) Reconcile(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	for _, reconcile := range []reconciler{
		c.reconcileEndpoint,
		c.reconcileCertificates,
		c.reconcileKubeConfigs,
		c.reconcileSAKeyPair,
		c.reconcileApiServer,
		c.reconcileKCM,
		c.reconcileScheduler,
	} {
		if err := reconcile(ctx, controlPlane); err != nil {
			return err
		}
	}
	zap.S().Infof("[%v] control plane reconciled", controlPlane.ClusterName())
	return nil
}

// Karpenter only created nodes for API server pods, as KCM and scheduler pods
// are configured with pod afinity. So the control plane nodes for a cluster
// will have 2 labels cluster name and clustername-apiserver
func nodeSelector(clusterName string) map[string]string {
	return patch.UnionStringMaps(apiServerLabels(clusterName),
		map[string]string{object.ControlPlaneLabelKey: clusterName})
}
