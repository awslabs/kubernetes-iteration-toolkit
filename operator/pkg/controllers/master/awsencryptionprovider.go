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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/functional"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/imageprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/object"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (c *Controller) reconcileEncryptionProviderConfig(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	providerConfig := defaultProviderConfig
	if controlPlane.Spec.Master.KMSKeyID != nil {
		providerConfig = encryptionEnabledConfig
	}
	configMap, err := object.GenerateConfigMap(providerConfig, struct{ ConfigMapName, Namespace string }{
		ConfigMapName: EncryptionProviderConfigName(controlPlane.ClusterName()),
		Namespace:     controlPlane.Namespace,
	})
	if err != nil {
		return fmt.Errorf("generating provider config, %w", err)
	}
	return c.kubeClient.EnsurePatch(ctx, &v1.ConfigMap{}, object.WithOwner(controlPlane, configMap))
}

func (c *Controller) reconcileEncryptionProvider(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (err error) {
	if controlPlane.Spec.Master.KMSKeyID == nil {
		return nil
	}
	hostPathDirectoryOrCreate := v1.HostPathDirectoryOrCreate
	return c.kubeClient.EnsurePatch(ctx, &appsv1.DaemonSet{},
		object.WithOwner(controlPlane, &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-aws-encryption-provider", controlPlane.ClusterName()),
				Namespace: controlPlane.Namespace,
				Labels:    providerLabels(controlPlane.ClusterName()),
			},
			Spec: appsv1.DaemonSetSpec{
				UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.RollingUpdateDaemonSetStrategyType},
				Selector:       &metav1.LabelSelector{MatchLabels: providerLabels(controlPlane.ClusterName())},
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: providerLabels(controlPlane.ClusterName())},
					Spec: v1.PodSpec{
						PriorityClassName: "system-node-critical",
						Tolerations:       []v1.Toleration{{Operator: v1.TolerationOpExists}},
						NodeSelector:      nodeSelector(controlPlane.ClusterName(), controlPlane.Spec.ColocateAPIServerWithEtcd),
						Containers: []v1.Container{{
							Name:    "aws-encryption-provider",
							Image:   imageprovider.AWSEncryptionProvider(),
							Command: []string{"/aws-encryption-provider"},
							Args: []string{
								"--key=" + aws.StringValue(controlPlane.Spec.Master.KMSKeyID),
								"--listen=/var/run/kmsplugin/socket.sock",
							},
							Ports: []v1.ContainerPort{{ContainerPort: 8080}},
							LivenessProbe: &v1.Probe{
								ProbeHandler: v1.ProbeHandler{
									HTTPGet: &v1.HTTPGetAction{
										Scheme: v1.URISchemeHTTP,
										Path:   "/healthz",
										Port:   intstr.FromInt(8080),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       5,
								TimeoutSeconds:      5,
								FailureThreshold:    5,
							},
							VolumeMounts: []v1.VolumeMount{{
								Name:      "var-run-kmsplugin",
								MountPath: "/var/run/kmsplugin",
							}},
						}},
						Volumes: []v1.Volume{{
							Name: "var-run-kmsplugin",
							VolumeSource: v1.VolumeSource{
								HostPath: &v1.HostPathVolumeSource{
									Path: "/var/run/kmsplugin",
									Type: &hostPathDirectoryOrCreate,
								},
							},
						}},
					},
				},
			},
		}),
	)
}

func EncryptionProviderConfigName(clusterName string) string {
	return fmt.Sprintf("%s-encryption-provider-config", clusterName)
}

func providerLabels(clustername string) map[string]string {
	return functional.UnionStringMaps(labelsFor(clustername), map[string]string{"component": "aws-encryption-provider"})
}

var (
	defaultProviderConfig = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .ConfigMapName }}
  namespace: {{ .Namespace }}
data:
  encryption-configuration.yaml: |
    apiVersion: apiserver.config.k8s.io/v1
    kind: EncryptionConfiguration
    resources:
      - resources:
        - secrets
        providers:
        - identity: {}
`
	encryptionEnabledConfig = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .ConfigMapName }}
  namespace: {{ .Namespace }}
data:
  encryption-configuration.yaml: |
    apiVersion: apiserver.config.k8s.io/v1
    kind: EncryptionConfiguration
    resources:
      - resources:
        - secrets
        providers:
        - kms:
            name: aws-encryption-provider
            endpoint: unix:///var/run/kmsplugin/socket.sock
            cachesize: 10000
            timeout: 3s
        - identity: {}
`
)
