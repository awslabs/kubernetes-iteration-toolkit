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
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/imageprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/object"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/patch"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (c *Controller) reconcileKCMCloudConfig(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	configMap, err := object.GenerateConfigMap(cloudConfig, struct{ ClusterName, ConfigMapName, Namespace string }{
		ClusterName:   controlPlane.ClusterName(),
		ConfigMapName: CloudConfigMapName(controlPlane.ClusterName()),
		Namespace:     controlPlane.Namespace,
	})
	if err != nil {
		return fmt.Errorf("generating cloud config, %w", err)
	}
	return c.kubeClient.EnsurePatch(ctx, &v1.ConfigMap{}, object.WithOwner(controlPlane, configMap))
}

func (c *Controller) reconcileKCM(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (err error) {
	kcmPodSpec := kcmPodSpecFor(controlPlane)
	if controlPlane.Spec.Master.ControllerManager != nil {
		kcmPodSpec, err = patch.PodSpec(&kcmPodSpec, controlPlane.Spec.Master.ControllerManager.Spec)
		if err != nil {
			return fmt.Errorf("patch KCM pod spec, %w", err)
		}
	}
	return c.kubeClient.EnsurePatch(ctx, &appsv1.DaemonSet{},
		object.WithOwner(controlPlane, &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      controllerManagerName(controlPlane.ClusterName()),
				Namespace: controlPlane.Namespace,
				Labels:    kcmLabels(controlPlane.ClusterName()),
			},
			Spec: appsv1.DaemonSetSpec{
				UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.RollingUpdateDaemonSetStrategyType},
				Selector:       &metav1.LabelSelector{MatchLabels: kcmLabels(controlPlane.ClusterName())},
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: kcmLabels(controlPlane.ClusterName())},
					Spec:       kcmPodSpec,
				},
			},
		}),
	)
}

func controllerManagerName(clusterName string) string {
	return fmt.Sprintf("%s-controller-manager", clusterName)
}

func kcmLabels(clustername string) map[string]string {
	return map[string]string{
		object.AppNameLabelKey:      "kube-controller-manager",
		object.ControlPlaneLabelKey: clustername,
	}
}

func kcmPodSpecFor(controlPlane *v1alpha1.ControlPlane) v1.PodSpec {
	hostPathDirectoryOrCreate := v1.HostPathDirectoryOrCreate
	return kcmPodSpecForVersion(controlPlane.Spec.KubernetesVersion, &v1.PodSpec{
		TerminationGracePeriodSeconds: aws.Int64(1),
		HostNetwork:                   true,
		DNSPolicy:                     v1.DNSClusterFirstWithHostNet,
		PriorityClassName:             "system-node-critical",
		Tolerations:                   []v1.Toleration{{Operator: v1.TolerationOpExists}},
		NodeSelector:                  nodeSelector(controlPlane.ClusterName()),
		Containers: []v1.Container{{
			Name:    "controller-manager",
			Image:   imageprovider.KubeControllerManager(controlPlane.Spec.KubernetesVersion),
			Command: []string{"kube-controller-manager"},
			Resources: v1.ResourceRequirements{
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU: resource.MustParse("1"),
				},
			},
			Args: []string{
				"--authentication-kubeconfig=/etc/kubernetes/config/kcm/controller-manager.conf",
				"--authorization-kubeconfig=/etc/kubernetes/config/kcm/controller-manager.conf",
				"--bind-address=127.0.0.1",
				"--client-ca-file=/etc/kubernetes/pki/ca/ca.crt",
				"--cluster-signing-cert-file=/etc/kubernetes/pki/ca/ca.crt",
				"--cluster-signing-key-file=/etc/kubernetes/pki/ca/ca.key",
				"--controllers=*", // TODO add -csrsigning once this is closed - https://github.com/awslabs/kubernetes-iteration-toolkit/issues/105
				"--kubeconfig=/etc/kubernetes/config/kcm/controller-manager.conf",
				"--leader-elect=true",
				"--requestheader-client-ca-file=/etc/kubernetes/pki/proxy-ca/front-proxy-ca.crt",
				"--root-ca-file=/etc/kubernetes/pki/ca/ca.crt",
				"--service-account-private-key-file=/etc/kubernetes/pki/sa/sa.key",
				"--use-service-account-credentials=true",
				"--cloud-provider=aws",
				"--cloud-config=/etc/kubernetes/cloud-config/aws.config",
				"--horizontal-pod-autoscaler-use-rest-clients=true",
				"--feature-gates=RotateKubeletServerCertificate=true",
				"--logtostderr=true",
				"--v=2",
			},
			VolumeMounts: []v1.VolumeMount{{
				Name:      "ca-certs",
				MountPath: "/etc/ssl/certs",
				ReadOnly:  true,
			}, {
				Name:      "client-ca-file",
				MountPath: "/etc/kubernetes/pki/ca",
				ReadOnly:  true,
			}, {
				Name:      "front-proxy-ca",
				MountPath: "/etc/kubernetes/pki/proxy-ca",
				ReadOnly:  true,
			}, {
				Name:      "service-account",
				MountPath: "/etc/kubernetes/pki/sa",
				ReadOnly:  true,
			}, {
				Name:      "kcm-config",
				MountPath: "/etc/kubernetes/config/kcm",
				ReadOnly:  true,
			}, {
				Name:      "cloud-config",
				MountPath: "/etc/kubernetes/cloud-config",
				ReadOnly:  true,
			}},
			LivenessProbe: &v1.Probe{
				ProbeHandler: v1.ProbeHandler{
					HTTPGet: &v1.HTTPGetAction{
						Host:   "127.0.0.1",
						Scheme: kcmHealthCheckSchemeForVersion(controlPlane.Spec.KubernetesVersion),
						Path:   "/healthz",
						Port:   kcmHealthCheckPortForVersion(controlPlane.Spec.KubernetesVersion),
					},
				},
				InitialDelaySeconds: 10,
				PeriodSeconds:       5,
				TimeoutSeconds:      5,
				FailureThreshold:    5,
			},
		}},
		Volumes: []v1.Volume{{
			Name: "ca-certs",
			VolumeSource: v1.VolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: "/etc/ssl/certs",
					Type: &hostPathDirectoryOrCreate,
				},
			},
		}, {
			Name: "client-ca-file",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  RootCASecretNameFor(controlPlane.ClusterName()),
					DefaultMode: aws.Int32(0400),
					Items: []v1.KeyToPath{{
						Key:  "public",
						Path: "ca.crt",
					}, {
						Key:  "private",
						Path: "ca.key",
					}},
				},
			},
		}, {
			Name: "front-proxy-ca",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  FrontProxyCASecretNameFor(controlPlane.ClusterName()),
					DefaultMode: aws.Int32(0400),
					Items: []v1.KeyToPath{{
						Key:  "public",
						Path: "front-proxy-ca.crt",
					}},
				},
			},
		}, {
			Name: "service-account",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  SAKeyPairSecretNameFor(controlPlane.ClusterName()),
					DefaultMode: aws.Int32(0400),
					Items: []v1.KeyToPath{{
						Key:  "public",
						Path: "sa.pub",
					}, {
						Key:  "private",
						Path: "sa.key",
					}},
				},
			},
		}, {
			Name: "kcm-config",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  KubeControllerManagerSecretNameFor(controlPlane.ClusterName()),
					DefaultMode: aws.Int32(0400),
					Items: []v1.KeyToPath{{
						Key:  "config",
						Path: "controller-manager.conf",
					}},
				},
			},
		}, {
			Name: "cloud-config",
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{Name: CloudConfigMapName(controlPlane.ClusterName())},
				},
			},
		}},
	})
}

func CloudConfigMapName(clusterName string) string {
	return fmt.Sprintf("%s-cloud-config", clusterName)
}

var (
	cloudConfig = `
apiVersion: v1
kind: ConfigMap
metadata:
  name: {{ .ConfigMapName }}
  namespace: {{ .Namespace }}
data:
  aws.config: |
    [Global]
    KubernetesClusterID={{ .ClusterName}}
`
)

var (
	disabledFlagsForKube122 = map[string]struct{}{"--horizontal-pod-autoscaler-use-rest-clients": {}}
)

func kcmPodSpecForVersion(version string, defaultSpec *v1.PodSpec) v1.PodSpec {
	switch version {
	case "1.22":
		args := []string{}
		for _, arg := range defaultSpec.Containers[0].Args {
			if _, skip := disabledFlagsForKube122[strings.Split(arg, "=")[0]]; skip {
				continue
			}
			args = append(args, arg)
		}
		defaultSpec.Containers[0].Args = args
	}
	return *defaultSpec
}

func kcmHealthCheckPortForVersion(version string) intstr.IntOrString {
	switch version {
	case "1.22":
		return intstr.FromInt(10257)
	}
	return intstr.FromInt(10252)
}

func kcmHealthCheckSchemeForVersion(version string) v1.URIScheme {
	switch version {
	case "1.22":
		return v1.URISchemeHTTPS
	}
	return v1.URISchemeHTTP
}
