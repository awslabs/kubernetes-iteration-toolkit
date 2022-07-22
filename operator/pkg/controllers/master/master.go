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

	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/awsprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/kubeprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/functional"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/keypairs"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/kubeconfigs"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/object"
	"go.uber.org/zap"
)

type Controller struct {
	kubeClient    *kubeprovider.Client
	keypairs      *keypairs.Provider
	kubeConfigs   *kubeconfigs.Provider
	iamController controlplane.Controller
	cloudProvider awsprovider.AccountMetadata
}

func New(kubeclient *kubeprovider.Client, account awsprovider.AccountMetadata, iamController controlplane.Controller) *Controller {
	return &Controller{
		kubeClient:    kubeclient,
		keypairs:      keypairs.Reconciler(kubeclient),
		kubeConfigs:   kubeconfigs.Reconciler(kubeclient),
		iamController: iamController,
		cloudProvider: account,
	}
}

type reconciler func(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (err error)

func (c *Controller) Reconcile(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	for _, reconcile := range []reconciler{
		c.reconcileEndpoint,
		c.reconcileCertificates,
		c.reconcileKubeConfigs,
		c.reconcileSAKeyPair,
		c.reconcileAuditLogConfig,
		c.reconcileApiServer,
		c.reconcileKCMCloudConfig,
		c.reconcileKCM,
		c.reconcileScheduler,
		c.reconcileAuthenticator,
		c.iamController.Reconcile,
		c.reconcileEncryptionProviderConfig,
		c.reconcileEncryptionProvider,
		c.reconcilePodmonitors,
	} {
		if err := reconcile(ctx, controlPlane); err != nil {
			return err
		}
	}
	zap.S().Infof("[%v] control plane reconciled", controlPlane.ClusterName())
	return nil
}

func (c *Controller) Finalize(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	return c.iamController.Finalize(ctx, controlPlane)
}

// Karpenter only created nodes for API server pods, as KCM and scheduler pods
// are configured with pod afinity. So the control plane nodes for a cluster
// will have 2 labels cluster name and clustername-apiserver
func nodeSelector(clusterName string) map[string]string {
	return functional.UnionStringMaps(APIServerLabels(clusterName),
		map[string]string{object.ControlPlaneLabelKey: clusterName})
}
