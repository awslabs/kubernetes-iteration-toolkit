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
	"github.com/awslabs/kit/operator/pkg/utils/imageprovider"
	"github.com/awslabs/kit/operator/pkg/utils/object"
	"github.com/awslabs/kit/operator/pkg/utils/patch"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Controller) reconcileScheduler(ctx context.Context, controlPlane *v1alpha1.ControlPlane) (err error) {
	schedulerPodSpec := schedulerPodSpecFor(controlPlane)
	if controlPlane.Spec.Master.Scheduler != nil {
		schedulerPodSpec, err = patch.PodSpec(&schedulerPodSpec, controlPlane.Spec.Master.Scheduler.Spec)
		if err != nil {
			return fmt.Errorf("patch scheduler pod spec, %w", err)
		}
	}
	return c.kubeClient.EnsurePatch(ctx, &appsv1.DaemonSet{},
		object.WithOwner(controlPlane, &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      SchedulerDaemonSetName(controlPlane.ClusterName()),
				Namespace: controlPlane.Namespace,
				Labels:    schedulerLabels(controlPlane.ClusterName()),
			},
			Spec: appsv1.DaemonSetSpec{
				UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.RollingUpdateDaemonSetStrategyType},
				Selector:       &metav1.LabelSelector{MatchLabels: schedulerLabels(controlPlane.ClusterName())},
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: schedulerLabels(controlPlane.ClusterName())},
					Spec:       schedulerPodSpec,
				},
			},
		}),
	)
}

func SchedulerDaemonSetName(clusterName string) string {
	return fmt.Sprintf("%s-scheduler", clusterName)
}

func schedulerLabels(clustername string) map[string]string {
	return map[string]string{
		object.AppNameLabelKey: SchedulerDaemonSetName(clustername),
	}
}

func schedulerPodSpecFor(controlPlane *v1alpha1.ControlPlane) v1.PodSpec {
	hostPathDirectoryOrCreate := v1.HostPathDirectoryOrCreate
	return v1.PodSpec{
		TerminationGracePeriodSeconds: aws.Int64(1),
		HostNetwork:                   true,
		DNSPolicy:                     v1.DNSClusterFirstWithHostNet,
		PriorityClassName:             "system-node-critical",
		Tolerations:                   []v1.Toleration{{Operator: v1.TolerationOpExists}},
		NodeSelector:                  nodeSelector(controlPlane.ClusterName()),
		Containers: []v1.Container{{
			Name:    "scheduler",
			Image:   imageprovider.KubeScheduler(controlPlane.Spec.KubernetesVersion),
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
