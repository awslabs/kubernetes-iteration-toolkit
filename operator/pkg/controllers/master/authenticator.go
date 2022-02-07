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
	"github.com/awslabs/kit/operator/pkg/components/iamauthenticator"
	"github.com/awslabs/kit/operator/pkg/utils/object"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// reconcileAuthenticator creates required configs for aws-iam-authenticator and stores them as secret in api server
func (c *Controller) reconcileAuthenticator(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	accountID, err := c.cloudProvider.ID()
	if err != nil {
		return fmt.Errorf("getting provider account ID, %w", err)
	}
	configMap, err := iamauthenticator.Config(ctx, controlPlane.ClusterName(), controlPlane.Namespace,
		iam.KitNodeRoleNameFor(controlPlane.ClusterName()), accountID)
	if err != nil {
		return fmt.Errorf("getting config for authenticator, %w", err)
	}
	if err := c.kubeClient.EnsurePatch(ctx, &v1.ConfigMap{}, configMap); err != nil {
		return fmt.Errorf("creating config map for authenticator, %w", err)
	}
	return c.ensureDaemonSet(ctx, controlPlane)
}

func (c *Controller) ensureDaemonSet(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	return c.kubeClient.EnsurePatch(ctx, &appsv1.DaemonSet{}, object.WithOwner(controlPlane, &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-authenticator", controlPlane.ClusterName()),
			Namespace: controlPlane.Namespace,
			Labels:    iamauthenticator.Labels(),
		},
		Spec: appsv1.DaemonSetSpec{
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.RollingUpdateDaemonSetStrategyType},
			Selector:       &metav1.LabelSelector{MatchLabels: iamauthenticator.Labels()},
			Template: iamauthenticator.PodSpec(func(spec v1.PodSpec) v1.PodSpec {
				spec.NodeSelector = APIServerLabels(controlPlane.ClusterName())
				spec.Volumes = append(spec.Volumes, v1.Volume{Name: "config",
					VolumeSource: v1.VolumeSource{ConfigMap: &v1.ConfigMapVolumeSource{
						LocalObjectReference: v1.LocalObjectReference{Name: iamauthenticator.AuthenticatorConfigMapName(controlPlane.ClusterName())},
					}},
				})
				return spec
			}),
		},
	}))
}
