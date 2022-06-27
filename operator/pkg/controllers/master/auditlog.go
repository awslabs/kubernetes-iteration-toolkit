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
	"fmt"

	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/object"
	v1 "k8s.io/api/core/v1"
)

func (c *Controller) reconcileAuditLogConfig(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	providerConfig := auditLogConfig
	configMap, err := object.GenerateConfigMap(providerConfig, struct{ ConfigMapName, Namespace string }{
		ConfigMapName: AuditLogConfigName(controlPlane.ClusterName()),
		Namespace:     controlPlane.Namespace,
	})
	if err != nil {
		return fmt.Errorf("creating audit log config, %w", err)
	}
	return c.kubeClient.EnsurePatch(ctx, &v1.ConfigMap{}, object.WithOwner(controlPlane, configMap))
}

func AuditLogConfigName(clusterName string) string {
	return fmt.Sprintf("%s-audit-log-config", clusterName)
}

var (
	auditLogConfig = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .ConfigMapName }}
  namespace: {{ .Namespace }}
data:
  audit-policy.yaml: |
    apiVersion: audit.k8s.io/v1
    kind: Policy
    rules:
      - level: Metadata
`
)
