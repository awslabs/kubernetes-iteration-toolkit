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
	"bytes"
	"context"
	"fmt"
	"html/template"

	"github.com/awslabs/kit/operator/pkg/apis/controlplane/v1alpha1"

	v1 "k8s.io/api/core/v1"
	kuberuntime "k8s.io/apimachinery/pkg/runtime"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
)

// reconcileAuthenticatorConfig creates required configs for aws-iam-authenticator and stores them as secret in api server
func (c *Controller) reconcileAuthenticatorConfig(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	awsAccountID, err := c.cloudProvider.ID()
	if err != nil {
		return fmt.Errorf("getting AWS account ID, %w", err)
	}
	configMapBytes, err := ParseTemplate(authenticatorConfig, struct{ ClusterName, Namespace, Group, AWSAccountID, PrivateDNS string }{
		ClusterName:  controlPlane.ClusterName(),
		Namespace:    controlPlane.Namespace,
		Group:        v1alpha1.SchemeGroupVersion.Group,
		AWSAccountID: awsAccountID,
		PrivateDNS:   "{{EC2PrivateDNSName}}",
	})
	if err != nil {
		return fmt.Errorf("generating authenticator config, %w", err)
	}
	configMap := &v1.ConfigMap{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), configMapBytes, configMap); err != nil {
		return fmt.Errorf("decoding authenticator config map, %w", err)
	}
	return c.kubeClient.EnsurePatch(ctx, &v1.ConfigMap{}, configMap)
}

var (
	authenticatorConfig = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: aws-iam-authenticator
  namespace: {{ .Namespace }}
data:
  config.yaml: |
    clusterID: {{ .ClusterName }}.{{ .Group }}
    server:
      mapRoles:
        - groups:
          - system:bootstrappers
          - system:nodes
          rolearn: arn:aws:iam::{{ .AWSAccountID }}:role/KitNodeRole-{{ .ClusterName }}-cluster
          username: system:node:{{ .PrivateDNS}}
      # List of Account IDs to whitelist for authentication
      mapAccounts:
        - {{ .AWSAccountID }}
`
)

// TODO move this to util. ParseTemplate validates and parses passed as argument template
func ParseTemplate(strtmpl string, obj interface{}) ([]byte, error) {
	var buf bytes.Buffer
	tmpl := template.Must(template.New("Text").Parse(strtmpl))
	err := tmpl.Execute(&buf, obj)
	if err != nil {
		return nil, fmt.Errorf("error when executing template, %w", err)
	}
	return buf.Bytes(), nil
}
