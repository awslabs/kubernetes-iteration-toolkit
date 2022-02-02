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

	"github.com/awslabs/kit/operator/pkg/kubeprovider"
	"github.com/awslabs/kit/operator/pkg/utils/imageprovider"
	"github.com/awslabs/kit/operator/pkg/utils/object"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuberuntime "k8s.io/apimachinery/pkg/runtime"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"knative.dev/pkg/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Controller struct {
	KubeClient *kubeprovider.Client
}

func New(KubeClient *kubeprovider.Client) *Controller {
	return &Controller{KubeClient: KubeClient}
}

func (c *Controller) EnsureConfig(ctx context.Context, name, ns, instanceRole, accountID string) error {
	configMap, err := Config(ctx, name, ns, instanceRole, accountID)
	if err != nil {
		return err
	}
	return c.KubeClient.EnsurePatch(ctx, &v1.ConfigMap{}, configMap)
}

func Config(ctx context.Context, name, ns, instanceRole, accountID string) (*v1.ConfigMap, error) {
	configMapBytes, err := ParseTemplate(authenticatorConfig, struct{ Name, ClusterName, Namespace, KitNodeRole, AWSAccountID, PrivateDNS, SessionName string }{
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
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), configMapBytes, configMap); err != nil {
		return nil, fmt.Errorf("decoding authenticator config map, %w", err)
	}
	return configMap, nil
}

func (c *Controller) EnsureDaemonSet(ctx context.Context, obj client.Object, nodeSelector map[string]string) error {
	return c.KubeClient.EnsurePatch(ctx, &appsv1.DaemonSet{}, object.WithOwner(obj, &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      AuthenticatorDaemonSetName(obj.GetName()),
			Namespace: obj.GetNamespace(),
			Labels:    authenticatorLabels(),
		},
		Spec: appsv1.DaemonSetSpec{
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.RollingUpdateDaemonSetStrategyType},
			Selector: &metav1.LabelSelector{
				MatchLabels: authenticatorLabels(),
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: authenticatorLabels()},
				Spec: PodSpec(obj, func(spec v1.PodSpec) v1.PodSpec {
					spec.NodeSelector = nodeSelector
					spec.Volumes = append(spec.Volumes, v1.Volume{
						Name: "config",
						VolumeSource: v1.VolumeSource{
							ConfigMap: &v1.ConfigMapVolumeSource{
								LocalObjectReference: v1.LocalObjectReference{Name: AuthenticatorConfigMapName(obj.GetName())},
							},
						},
					})
					return spec
				}),
			},
		},
	}))
}

type Options func(v1.PodSpec) v1.PodSpec

func PodSpec(obj client.Object, opts ...Options) v1.PodSpec {
	spec := podSpec(obj)
	for _, opt := range opts {
		spec = opt(spec)
	}
	return spec
}

func podSpec(obj client.Object) v1.PodSpec {
	return v1.PodSpec{
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
				"--master=https://localhost/",
				"--config=/etc/aws-iam-authenticator/config.yaml",
				"--state-dir=/var/aws-iam-authenticator/state/",
				"--generate-kubeconfig=/var/aws-iam-authenticator/kubeconfig/kubeconfig.yaml",
			},
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
	}
}

func authenticatorLabels() map[string]string {
	return map[string]string{"component": "aws-iam-authenticator"}
}

var (
	authenticatorConfig = `
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

func AuthenticatorDaemonSetName(clusterName string) string {
	return fmt.Sprintf("%s-authenticator", clusterName)
}

func AuthenticatorConfigMapName(clusterName string) string {
	return fmt.Sprintf("%s-auth-config", clusterName)
}
