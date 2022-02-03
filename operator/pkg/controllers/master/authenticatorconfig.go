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
	"github.com/awslabs/kit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/awsprovider/iam"
	"github.com/awslabs/kit/operator/pkg/utils/imageprovider"
	"github.com/awslabs/kit/operator/pkg/utils/object"
	"k8s.io/apimachinery/pkg/util/intstr"
	"knative.dev/pkg/ptr"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// reconcileAuthenticatorConfig creates required configs for aws-iam-authenticator and stores them as secret in api server
func (c *Controller) reconcileAuthenticatorConfig(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	awsAccountID, err := c.cloudProvider.ID()
	if err != nil {
		return fmt.Errorf("getting AWS account ID, %w", err)
	}
	configMap, err := object.GenerateConfigMap(authenticatorConfig, struct{ Name, ClusterName, Namespace, Group, KitNodeRole, AWSAccountID, PrivateDNS, SessionName string }{
		Name:         AuthenticatorConfigMapName(controlPlane.ClusterName()),
		ClusterName:  controlPlane.ClusterName(),
		Namespace:    controlPlane.Namespace,
		Group:        v1alpha1.SchemeGroupVersion.Group,
		KitNodeRole:  iam.KitNodeRoleNameFor(controlPlane.ClusterName()),
		AWSAccountID: awsAccountID,
		PrivateDNS:   "{{EC2PrivateDNSName}}",
		SessionName:  "{{SessionNameRaw}}",
	})
	if err != nil {
		return fmt.Errorf("generating authenticator config, %w", err)
	}
	return c.kubeClient.EnsurePatch(ctx, &v1.ConfigMap{}, configMap)
}

func (c *Controller) reconcileAuthenticatorDaemonSet(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	return c.kubeClient.EnsurePatch(ctx, &appsv1.DaemonSet{},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      AuthenticatorDaemonSetName(controlPlane.ClusterName()),
				Namespace: controlPlane.Namespace,
				Labels:    authenticatorLabels(),
			},
			Spec: appsv1.DaemonSetSpec{
				UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.RollingUpdateDaemonSetStrategyType},
				Selector: &metav1.LabelSelector{
					MatchLabels: authenticatorLabels(),
				},
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: authenticatorLabels()},
					Spec: v1.PodSpec{
						HostNetwork:  true,
						NodeSelector: APIServerLabels(controlPlane.ClusterName()),
						Tolerations:  []v1.Toleration{{Operator: v1.TolerationOpExists}},
						InitContainers: []v1.Container{{
							Name:  "chown",
							Image: imageprovider.BusyBox(),
							Command: []string{
								"sh",
								"-c",
								"chown -R 10000:10000 /var/aws-iam-authenticator/state/ && chown -R 10000:10000 /var/aws-iam-authenticator/kubeconfig && ls -lrt /var/",
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
							LivenessProbe: &v1.Probe{
								Handler: v1.Handler{
									HTTPGet: &v1.HTTPGetAction{
										Host:   "127.0.0.1",
										Scheme: v1.URISchemeHTTP,
										Path:   "/healthz",
										Port:   intstr.FromInt(21363),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       5,
								TimeoutSeconds:      5,
								FailureThreshold:    5,
							},
						}},
						Volumes: []v1.Volume{{
							Name: "config",
							VolumeSource: v1.VolumeSource{
								ConfigMap: &v1.ConfigMapVolumeSource{
									LocalObjectReference: v1.LocalObjectReference{Name: AuthenticatorConfigMapName(controlPlane.ClusterName())},
								},
							},
						}, {
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
				},
			},
		},
	)
}

func authenticatorLabels() map[string]string {
	return map[string]string{
		"k8s-app": "aws-iam-authenticator",
	}
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
    clusterID: {{ .ClusterName }}.{{ .Group }}
    server:
      mapRoles:
        - groups:
          - system:bootstrappers
          - system:nodes
          rolearn: arn:aws:iam::{{ .AWSAccountID }}:role/{{ .KitNodeRole }}
          username: system:node:{{ .PrivateDNS}}
        - groups:
          - system:bootstrappers
          - system:nodes
          rolearn: arn:aws:iam::{{ .AWSAccountID }}:role/KitletNodeRole
          username: system:node:{{ .SessionName }}
      # List of Account IDs to whitelist for authentication
      mapAccounts:
        - {{ .AWSAccountID }}
`
)

func AuthenticatorDaemonSetName(clusterName string) string {
	return fmt.Sprintf("%s-authenticator", clusterName)
}

func AuthenticatorConfigMapName(clusterName string) string {
	return fmt.Sprintf("%s-auth-config", clusterName)
}
