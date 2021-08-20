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
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	schedulerImage = "public.ecr.aws/eks-distro/kubernetes/kube-scheduler:v1.20.7-eks-1-20-4"
)

func (c *Controller) reconcileScheduler(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	return c.kubeClient.EnsurePatch(ctx, &appsv1.Deployment{}, object.WithOwner(controlPlane, schedulerDeploymentSpec(controlPlane)))
}

func schedulerDeploymentSpec(controlPlane *v1alpha1.ControlPlane) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SchedulerDeploymentName(controlPlane.ClusterName()),
			Namespace: controlPlane.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: schedulerLabels(controlPlane.ClusterName()),
			},
			Replicas: aws.Int32(3),
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: schedulerLabels(controlPlane.ClusterName()),
				},
				Spec: *schedulerPodSpecFor(controlPlane),
			},
		},
	}
}

func SchedulerDeploymentName(clusterName string) string {
	return fmt.Sprintf("%s-scheduler", clusterName)
}

func schedulerLabels(clustername string) map[string]string {
	return map[string]string{
		object.AppNameLabelKey: SchedulerDeploymentName(clustername),
	}
}

func schedulerPodSpecFor(controlPlane *v1alpha1.ControlPlane) *v1.PodSpec {
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
				MatchLabels: schedulerLabels(controlPlane.ClusterName()),
			},
		}, {
			MaxSkew:           int32(1),
			TopologyKey:       "kubernetes.io/hostname",
			WhenUnsatisfiable: v1.DoNotSchedule,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: schedulerLabels(controlPlane.ClusterName()),
			},
		}},
		Affinity: &v1.Affinity{PodAffinity: &v1.PodAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{{
				LabelSelector: &metav1.LabelSelector{MatchLabels: apiServerLabels(controlPlane.ClusterName())},
				TopologyKey:   "kubernetes.io/hostname",
			}},
		}},
		Containers: []v1.Container{{
			Name:    "scheduler",
			Image:   schedulerImage,
			Command: []string{"kube-scheduler"},
			Resources: v1.ResourceRequirements{
				Requests: map[v1.ResourceName]resource.Quantity{
					v1.ResourceCPU: resource.MustParse("1"),
				},
			},
			Args: []string{
				"--authentication-kubeconfig=/etc/kubernetes/config/scheduler/scheduler.conf",
				"--authorization-kubeconfig=/etc/kubernetes/config/scheduler/scheduler.conf",
				"--bind-address=127.0.0.1",
				"--kubeconfig=/etc/kubernetes/config/scheduler/scheduler.conf",
				"--leader-elect=true",
				"--port=0",
			},
			VolumeMounts: []v1.VolumeMount{{
				Name:      "ca-certs",
				MountPath: "/etc/ssl/certs",
				ReadOnly:  true,
			}, {
				Name:      "scheduler-config",
				MountPath: "/etc/kubernetes/config/scheduler",
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
			Name: "scheduler-config",
			VolumeSource: v1.VolumeSource{
				Secret: &v1.SecretVolumeSource{
					SecretName:  KubeSchedulerSecretNameFor(controlPlane.ClusterName()),
					DefaultMode: aws.Int32(0400),
					Items: []v1.KeyToPath{{
						Key:  "config",
						Path: "scheduler.conf",
					}},
				},
			},
		}},
	}
}
