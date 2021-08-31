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
	"github.com/awslabs/kit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/utils/object"
	"github.com/awslabs/kit/operator/pkg/utils/patch"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	controllerManagerImage = "public.ecr.aws/eks-distro/kubernetes/kube-controller-manager:v1.20.7-eks-1-20-4"
)

func (c *Controller) reconcileKCM(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	return c.kubeClient.EnsurePatch(ctx, &appsv1.Deployment{}, object.WithOwner(controlPlane, kcmDeploymentSpec(controlPlane)))
}

func kcmDeploymentSpec(controlPlane *v1alpha1.ControlPlane) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      KCMDeploymentName(controlPlane.ClusterName()),
			Namespace: controlPlane.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: kcmLabels(controlPlane.ClusterName()),
			},
			Replicas: aws.Int32(3),
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: kcmLabels(controlPlane.ClusterName()),
				},
				Spec: *kcmPodSpecFor(controlPlane),
			},
		},
	}
}

func KCMDeploymentName(clusterName string) string {
	return fmt.Sprintf("%s-controller-manager", clusterName)
}

func kcmLabels(clustername string) map[string]string {
	return patch.UnionStringMaps(labelsFor(clustername), map[string]string{"component": "kube-controller-manager"})
}

func kcmPodSpecFor(controlPlane *v1alpha1.ControlPlane) *v1.PodSpec {
	hostPathDirectoryOrCreate := v1.HostPathDirectoryOrCreate
	return &v1.PodSpec{
		TerminationGracePeriodSeconds: aws.Int64(1),
		HostNetwork:                   true,
		DNSPolicy:                     v1.DNSClusterFirstWithHostNet,
		PriorityClassName:             "system-node-critical",
		NodeSelector:                  nodeSelector(controlPlane.ClusterName()),
		TopologySpreadConstraints: []v1.TopologySpreadConstraint{{
			MaxSkew:           int32(1),
			TopologyKey:       "topology.kubernetes.io/zone",
			WhenUnsatisfiable: v1.DoNotSchedule,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: kcmLabels(controlPlane.ClusterName()),
			},
		}, {
			MaxSkew:           int32(1),
			TopologyKey:       "kubernetes.io/hostname",
			WhenUnsatisfiable: v1.DoNotSchedule,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: kcmLabels(controlPlane.ClusterName()),
			},
		}},
		Affinity: &v1.Affinity{PodAffinity: &v1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{{
				LabelSelector: &metav1.LabelSelector{MatchLabels: APIServerLabels(controlPlane.ClusterName())},
				TopologyKey:   "kubernetes.io/hostname",
			}},
		}},
		Containers: []v1.Container{{
			Name:    "controller-manager",
			Image:   controllerManagerImage,
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
				"--cluster-name=kubernetes",
				"--cluster-signing-cert-file=/etc/kubernetes/pki/ca/ca.crt",
				"--cluster-signing-key-file=/etc/kubernetes/pki/ca/ca.key",
				"--controllers=*,bootstrapsigner,tokencleaner",
				"--kubeconfig=/etc/kubernetes/config/kcm/controller-manager.conf",
				"--leader-elect=true",
				"--port=0",
				"--requestheader-client-ca-file=/etc/kubernetes/pki/proxy-ca/front-proxy-ca.crt",
				"--root-ca-file=/etc/kubernetes/pki/ca/ca.crt",
				"--service-account-private-key-file=/etc/kubernetes/pki/sa/sa.key",
				"--use-service-account-credentials=true",
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
			}},
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
		}},
	}
}
