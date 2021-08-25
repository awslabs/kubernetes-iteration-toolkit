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

	"github.com/awslabs/kit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/controllers/master"
	"github.com/awslabs/kit/operator/pkg/errors"
	"github.com/awslabs/kit/operator/pkg/kubeprovider"
	"github.com/awslabs/kit/operator/pkg/utils/keypairs"
	"github.com/awslabs/kit/operator/pkg/utils/object"
	"github.com/awslabs/kit/operator/pkg/utils/scheme"
	"github.com/awslabs/kit/operator/pkg/utils/secrets"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Controller struct {
	kubeClient *kubeprovider.Client
	keypairs   *keypairs.Provider
}

func New(kubeClient *kubeprovider.Client) *Controller {
	return &Controller{
		kubeClient: kubeClient,
		keypairs:   keypairs.Reconciler(kubeClient),
	}
}

type reconciler func(context.Context, *Controller, *v1alpha1.ControlPlane) (err error)

func (c *Controller) Reconcile(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	newClusterClient, err := c.createKubeClient(ctx, object.NamespacedName(
		controlPlane.ClusterName(), controlPlane.Namespace))
	if err != nil {
		return err
	}
	// reconcile addons to the new cluster
	newCluster := &Controller{kubeClient: newClusterClient}
	for _, reconcile := range []reconciler{
		newCluster.kubeConfigForKubeProxy,
		newCluster.daemonsetForKubeProxy,
	} {
		if err := reconcile(ctx, c, controlPlane); err != nil {
			return err
		}
	}
	zap.S().Infof("[%v] Addons reconciled", controlPlane.ClusterName())
	return nil
}

// createKubeClient returns a kubeClient for the new cluster created from the
// admin config stored in management cluster
func (c *Controller) createKubeClient(ctx context.Context, nn types.NamespacedName) (*kubeprovider.Client, error) {
	// Get the admin kube config stored in a secret in the management cluster
	adminSecret, err := c.keypairs.GetSecretFromServer(ctx, object.NamespacedName(
		master.KubeAdminSecretNameFor(nn.Name), nn.Namespace))
	if err != nil {
		return nil, err
	}
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(adminSecret.Data[secrets.SecretConfigKey])
	if err != nil {
		return nil, fmt.Errorf("creating rest config for new cluster, %w", err)
	}
	newClient, err := client.New(restConfig, client.Options{Scheme: scheme.KitCluster})
	if err != nil {
		if errors.IsDNSLookUpNoSuchHost(err) {
			return nil, fmt.Errorf("%v control plane endpoint not ready, lookup failed, %w", nn.Name, errors.WaitingForSubResources)
		}
		if errors.IsNetIOTimeOut(err) {
			// This happens 1-2 times, but if it happens more we would want to know in the logs
			zap.S().Warnf("Creating kubeclient, net i/o timed out for control plane %s endpoint", nn.Name)
			return nil, fmt.Errorf("net i/o timeout for %v control plane endpoint, %w", nn.Name, errors.WaitingForSubResources)
		}
		return nil, fmt.Errorf("creating kubeclient for new cluster, %w", err)
	}
	return kubeprovider.New(newClient), nil
}
