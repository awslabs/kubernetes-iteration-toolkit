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

package etcd

import (
	"context"
	"fmt"

	"github.com/awslabs/kit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/utils/object"
	"github.com/awslabs/kit/operator/pkg/utils/patch"

	"github.com/aws/aws-sdk-go/aws"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c *Controller) reconcileStatefulSet(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	// Generate the default pod spec for the given control plane, if user has
	// provided custom config for the etcd pod spec, patch this user
	// provided config to the default spec
	etcdSpec, err := patch.PodSpec(podSpecFor(controlPlane), controlPlane.Spec.Etcd.Spec)
	if err != nil {
		return fmt.Errorf("failed to patch pod spec, %w", err)
	}
	return c.kubeClient.EnsurePatch(ctx, &appsv1.StatefulSet{}, object.WithOwner(controlPlane, &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceNameFor(controlPlane.ClusterName()),
			Namespace: controlPlane.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labelsFor(controlPlane.ClusterName()),
			},
			PodManagementPolicy: appsv1.ParallelPodManagement,
			ServiceName:         ServiceNameFor(controlPlane.ClusterName()),
			Replicas:            aws.Int32(int32(controlPlane.Spec.Etcd.Replicas)),
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labelsFor(controlPlane.ClusterName()),
				},
				Spec: etcdSpec,
			},
			VolumeClaimTemplates: []v1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{
					Name: "etcd-data",
				},
				Spec: v1.PersistentVolumeClaimSpec{
					AccessModes:      []v1.PersistentVolumeAccessMode{"ReadWriteOnce"},
					StorageClassName: &controlPlane.Spec.Etcd.StorageSpec.StorageClassName,
					Resources: v1.ResourceRequirements{
						Requests: v1.ResourceList{"storage": controlPlane.Spec.Etcd.StorageSpec.Size},
					},
				},
			}},
		},
	}))
}
