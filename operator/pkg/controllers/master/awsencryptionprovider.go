package master

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/awslabs/kit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/utils/functional"
	"github.com/awslabs/kit/operator/pkg/utils/object"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (c *Controller) reconcileEncryptionProviderConfig(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	providerConfig := defaultProviderConfig
	if controlPlane.Spec.Master.KMSKeyARN != nil {
		providerConfig = awsEncryptionConfig
	}
	configMap, err := object.GenerateConfigMap(providerConfig, struct{ ConfigMapName, Namespace string }{
		ConfigMapName: EncryptionProviderConfigMapName(controlPlane.ClusterName()),
		Namespace:     controlPlane.Namespace,
	})
	if err != nil {
		return fmt.Errorf("generating provider config, %w", err)
	}
	return c.kubeClient.EnsurePatch(ctx, &v1.ConfigMap{}, object.WithOwner(controlPlane, configMap))
}

func (c *Controller) reconcileEncryptionProvider(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (err error) {
	if controlPlane.Spec.Master.KMSKeyARN == nil {
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
						Containers: []v1.Container{{
							Name:    "aws-encryption-provider",
							Image:   "TODO",
							Command: []string{"/aws-encryption-provider"},
							Args: []string{
								"--key=" + aws.StringValue(controlPlane.Spec.Master.KMSKeyARN),
								"--region=" + c.cloudProvider.Region(),
								"--listen=/var/run/kmsplugin/socket.sock",
							},
							Ports: []v1.ContainerPort{{ContainerPort: 8080}},
							LivenessProbe: &v1.Probe{
								Handler: v1.Handler{
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
		},
		),
	)
}

func EncryptionProviderConfigMapName(clusterName string) string {
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
  provider.config: |
	apiVersion: apiserver.config.k8s.io/v1
	kind: EncryptionConfiguration
	resources:
	- resources:
		- secrets
		providers:
		- identity: {}
`
	awsEncryptionConfig = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .ConfigMapName }}
  namespace: {{ .Namespace }}
data:
  provider.config: |
	apiVersion: apiserver.config.k8s.io/v1
	kind: EncryptionConfiguration
	resources:
	- resources:
		- secrets
		providers:
		- kms:
			name: aws-encryption-provider
			endpoint: unix:///var/run/kmsplugin/socket.sock
			cachesize: 50000
			timeout: 3s
		- identity: {}
`
)
