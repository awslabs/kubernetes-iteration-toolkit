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

package controller

import (
	"context"
	"fmt"
	"path"

	"github.com/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	clusterinfophase "k8s.io/kubernetes/cmd/kubeadm/app/phases/bootstraptoken/clusterinfo"
	nodebootstraptokenphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/bootstraptoken/node"
	kubeconfigutil "k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"
)

func (c *controlPlane) bootstrapMasterNodes(ctx context.Context, clusterName, masterNode string, config *kubeadm.InitConfiguration) error {
	// Create the default node bootstrap token
	kubeconfigAdminPath := path.Join("/tmp", clusterName, masterNode, "etc/kubernetes/admin.conf")
	client, err := kubeconfigutil.ClientSetFromFile(kubeconfigAdminPath)
	if err != nil {
		return fmt.Errorf("creating Kube client from admin config, %w", err)
	}

	zap.S().Info("Created the client for master")
	if err := nodebootstraptokenphase.UpdateOrCreateTokens(client, false, config.BootstrapTokens); err != nil {
		return fmt.Errorf("error updating or creating token, %w", err)
	}
	zap.S().Info("UpdateOrCreateTokens")

	// Create RBAC rules that makes the bootstrap tokens able to get nodes
	if err := nodebootstraptokenphase.AllowBoostrapTokensToGetNodes(client); err != nil {
		return fmt.Errorf("error allowing bootstrap tokens to get Nodes, %w", err)
	}
	// Create RBAC rules that makes the bootstrap tokens able to post CSRs
	if err := nodebootstraptokenphase.AllowBootstrapTokensToPostCSRs(client); err != nil {
		return fmt.Errorf("error allowing bootstrap tokens to post CSRs, %w", err)
	}
	// Create RBAC rules that makes the bootstrap tokens able to get their CSRs approved automatically
	if err := nodebootstraptokenphase.AutoApproveNodeBootstrapTokens(client); err != nil {
		return fmt.Errorf("error auto-approving node bootstrap tokens, %w", err)
	}
	// Create/update RBAC rules that makes the nodes to rotate certificates and get their CSRs approved automatically
	if err := nodebootstraptokenphase.AutoApproveNodeCertificateRotation(client); err != nil {
		return fmt.Errorf("err AutoApproveNodeCertificateRotation, %w", err)
	}
	// Create the cluster-info ConfigMap with the associated RBAC rules
	if err := clusterinfophase.CreateBootstrapConfigMapIfNotExists(client, kubeconfigAdminPath); err != nil {
		return errors.Wrap(err, "error creating bootstrap ConfigMap")
	}
	if err := clusterinfophase.CreateClusterInfoRBACRules(client); err != nil {
		return errors.Wrap(err, "error creating clusterinfo RBAC rules")
	}
	return nil
}
