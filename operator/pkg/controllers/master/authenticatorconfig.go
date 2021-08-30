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
	"github.com/awslabs/kit/operator/pkg/awsprovider"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// reconcileAuthenticatorConfig creates required configs for aws-iam-authenticator and stores them as secret in api server
func (c *Controller) reconcileAuthenticatorConfig(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	awsAccountID, err := awsprovider.AccountID(c.session)
	if err != nil {
		return err
	}
	configBytes, err := ParseTemplate(authenticatorConfig, struct{ ClusterName, Group, AWSAccountID string }{
		ClusterName:  controlPlane.ClusterName(),
		Group:        v1alpha1.SchemeGroupVersion.Group,
		AWSAccountID: awsAccountID,
	})
	if err != nil {
		return fmt.Errorf("generating authenticator config, %w", err)
	}
	return c.kubeClient.EnsurePatch(ctx, &v1.ConfigMap{}, &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "aws-iam-authenticator",
			Namespace: controlPlane.Namespace,
			Labels:    authenticatorLabel(),
		},
		Data: map[string]string{
			"config.yaml": string(configBytes),
		},
	})
}

func authenticatorLabel() map[string]string {
	return map[string]string{
		"k8s-app": "aws-iam-authenticator",
	}
}

var (
	authenticatorConfig = `clusterID: {{ .ClusterName }}.{{ .Group }}
server:
	mapRoles:
	- groups:
		- system:bootstrappers
		- system:nodes
		rolearn: arn:aws:iam::{{ .AWSAccountID }}:role/KitNodeRole-{{ .ClusterName }}-cluster
		username: system:node:{{EC2PrivateDNSName}}
	# List of Account IDs to whitelist for authentication
	mapAccounts:
	- {{ .AWSAccountID }}`
)

// TODO move this to util. ParseTemplate validates and parses passed as argument template
func ParseTemplate(strtmpl string, obj interface{}) ([]byte, error) {
	var buf bytes.Buffer
	tmpl, err := template.New("template").Parse(strtmpl)
	if err != nil {
		return nil, fmt.Errorf("error when parsing template, %w", err)
	}
	err = tmpl.Execute(&buf, obj)
	if err != nil {
		return nil, fmt.Errorf("error when executing template, %w", err)
	}
	return buf.Bytes(), nil
}

// c.kubeClient.EnsurePatch(ctx, &appsv1.DaemonSet{}, &appsv1.DaemonSet{
// 	ObjectMeta: metav1.ObjectMeta{
// 		Name:      "coredns",
// 		Namespace: kubeSystem,
// 		Labels:    coreDNSLabels(),
// 	},
// })
