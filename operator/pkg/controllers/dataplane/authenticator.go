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

package dataplane

import (
	"context"
	"fmt"

	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/dataplane/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/awsprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/awsprovider/iam"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/components/iamauthenticator"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/controllers/master"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/kubeprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/object"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Authenticator struct {
	kubeClient    *kubeprovider.Client
	cloudProvider awsprovider.AccountMetadata
}

// Reconcile creates required configs for aws-iam-authenticator and stores them as secret in api server
func (a *Authenticator) Reconcile(ctx context.Context, dataplane *v1alpha1.DataPlane) error {
	accountID, err := a.cloudProvider.ID()
	if err != nil {
		return fmt.Errorf("getting provider account ID, %w", err)
	}
	nodeRole := dataplane.Spec.InstanceProfile
	if nodeRole == "" {
		nodeRole = iam.KitNodeRoleNameFor(dataplane.Spec.ClusterName)
	}
	configMap, err := iamauthenticator.Config(ctx, dataplane.Spec.ClusterName, dataplane.Namespace, nodeRole, accountID)
	if err != nil {
		return fmt.Errorf("getting config for authenticator, %w", err)
	}
	if err := a.kubeClient.EnsurePatch(ctx, &v1.ConfigMap{}, object.WithOwner(dataplane, configMap)); err != nil {
		return fmt.Errorf("creating config map for authenticator, %w", err)
	}
	return a.ensureDaemonSet(ctx, dataplane)
}

func (a *Authenticator) Finalize(ctx context.Context, dataplane *v1alpha1.DataPlane) error {
	//TODO
	return nil
}

func (a *Authenticator) ensureDaemonSet(ctx context.Context, dataplane *v1alpha1.DataPlane) error {
	return a.kubeClient.EnsurePatch(ctx, &appsv1.DaemonSet{}, object.WithOwner(dataplane, &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-authenticator", dataplane.Spec.ClusterName),
			Namespace: dataplane.Namespace,
			Labels:    iamauthenticator.Labels(),
		},
		Spec: appsv1.DaemonSetSpec{
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{Type: appsv1.RollingUpdateDaemonSetStrategyType},
			Selector:       &metav1.LabelSelector{MatchLabels: iamauthenticator.Labels()},
			Template: iamauthenticator.PodSpec(func(template v1.PodTemplateSpec) v1.PodTemplateSpec {
				template.Spec.NodeSelector = master.APIServerLabels(dataplane.Spec.ClusterName)
				template.Spec.Volumes = append(template.Spec.Volumes, v1.Volume{Name: "config",
					VolumeSource: v1.VolumeSource{ConfigMap: &v1.ConfigMapVolumeSource{
						LocalObjectReference: v1.LocalObjectReference{Name: iamauthenticator.ConfigMapName(dataplane.Spec.ClusterName)},
					}},
				})
				return template
			}),
		},
	}))
}
