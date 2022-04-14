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

package iamauthenticator

import (
	"bytes"
	"context"
	"fmt"
	"html/template"

	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/imageprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/object"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"knative.dev/pkg/ptr"
)

func Config(ctx context.Context, name, ns, instanceRole, accountID string) (*v1.ConfigMap, error) {
	configMapBytes, err := parseTemplate(config, struct{ Name, ClusterName, Namespace, KitNodeRole, AWSAccountID, PrivateDNS, SessionName string }{
		Name:         AuthenticatorConfigMapName(name),
		ClusterName:  name,
		Namespace:    ns,
		KitNodeRole:  instanceRole,
		AWSAccountID: accountID,
		PrivateDNS:   "{{EC2PrivateDNSName}}",
		SessionName:  "{{SessionNameRaw}}",
	})
	if err != nil {
		return nil, fmt.Errorf("generating authenticator config, %w", err)
	}
	configMap := &v1.ConfigMap{}
	if err := runtime.DecodeInto(scheme.Codecs.UniversalDecoder(), configMapBytes, configMap); err != nil {
		return nil, fmt.Errorf("decoding authenticator config map, %w", err)
	}
	return configMap, nil
}

type Options func(v1.PodTemplateSpec) v1.PodTemplateSpec

func PodSpec(opts ...Options) v1.PodTemplateSpec {
	podTemplateSpec := v1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Name: "aws-iam-authenticator", Labels: Labels},
		Spec: v1.PodSpec{
			HostNetwork: true,
			Tolerations: []v1.Toleration{{Operator: v1.TolerationOpExists}},
			InitContainers: []v1.Container{{
				Name:  "chown",
				Image: imageprovider.BusyBox(),
				Command: []string{"sh", "-c",
					"touch /var/aws-iam-authenticator/kubeconfig/kubeconfig.yaml && chown -R 10000:10000 /var/aws-iam-authenticator/state/ && chown -R 10000:10000 /var/aws-iam-authenticator/kubeconfig && ls -lrt /var/aws-iam-authenticator",
				},
				SecurityContext: &v1.SecurityContext{AllowPrivilegeEscalation: ptr.Bool(true)},
				VolumeMounts: []v1.VolumeMount{{
					Name:      "state",
					MountPath: "/var/aws-iam-authenticator/state/",
				}, {
					Name:      "kubeconfig",
					MountPath: "/var/aws-iam-authenticator/kubeconfig/",
				}},
			}},
			Containers: []v1.Container{{
				Name:  "aws-iam-authenticator",
				Image: imageprovider.AWSIamAuthenticator(),
				Args: []string{
					"server",
					"--address=0.0.0.0",
					"--master=https://localhost/",
					"--config=/etc/aws-iam-authenticator/config.yaml",
					"--state-dir=/var/aws-iam-authenticator/state/",
					"--generate-kubeconfig=/var/aws-iam-authenticator/kubeconfig/kubeconfig.yaml",
				},
				Ports: []v1.ContainerPort{{
					ContainerPort: 21362,
					Name:          "metrics",
				}},
				SecurityContext: &v1.SecurityContext{AllowPrivilegeEscalation: ptr.Bool(true)},
				VolumeMounts: []v1.VolumeMount{{
					Name:      "config",
					MountPath: "/etc/aws-iam-authenticator/",
				}, {
					Name:      "state",
					MountPath: "/var/aws-iam-authenticator/state/",
				}, {
					Name:      "kubeconfig",
					MountPath: "/var/aws-iam-authenticator/kubeconfig/",
				}},
			}},
			Volumes: []v1.Volume{{
				Name: "kubeconfig",
				VolumeSource: v1.VolumeSource{
					HostPath: &v1.HostPathVolumeSource{
						Path: "/var/aws-iam-authenticator/kubeconfig/",
					},
				},
			}, {
				Name: "state",
				VolumeSource: v1.VolumeSource{
					HostPath: &v1.HostPathVolumeSource{
						Path: "/var/aws-iam-authenticator/state/",
					},
				},
			}},
		},
	}
	for _, opt := range opts {
		podTemplateSpec = opt(podTemplateSpec)
	}
	return podTemplateSpec
}

var (
	config = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .Name }}
  namespace: {{ .Namespace }}
data:
  config.yaml: |
    clusterID: {{ .ClusterName }}
    server:
      mapRoles:
        - groups:
          - system:bootstrappers
          - system:nodes
          rolearn: arn:aws:iam::{{ .AWSAccountID }}:role/{{ .KitNodeRole }}
          username: system:node:{{ .PrivateDNS}}
      # List of Account IDs to whitelist for authentication
      mapAccounts:
        - {{ .AWSAccountID }}
`
)

// TODO move this to util. parseTemplate validates and parses passed as argument template
func parseTemplate(strtmpl string, obj interface{}) ([]byte, error) {
	var buf bytes.Buffer
	if err := template.Must(template.New("Text").Parse(strtmpl)).Execute(&buf, obj); err != nil {
		return nil, fmt.Errorf("error when executing template, %w", err)
	}
	return buf.Bytes(), nil
}

func AuthenticatorConfigMapName(clusterName string) string {
	return fmt.Sprintf("%s-auth-config", clusterName)
}

var Labels = map[string]string{object.AppNameLabelKey: "aws-iam-authenticator"}
