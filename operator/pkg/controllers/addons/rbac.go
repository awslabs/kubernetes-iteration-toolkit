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

package addons

import (
	"context"

	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/kubeprovider"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type RBAC struct {
	kubeClient *kubeprovider.Client
}

// RBACController configures the required RBAC permissions in the cluster for monitoring
func RBACController(kubeClient *kubeprovider.Client) *RBAC {
	return &RBAC{kubeClient: kubeClient}
}

func (r *RBAC) Reconcile(ctx context.Context, _ *v1alpha1.ControlPlane) error {
	for _, fn := range []func(context.Context) error{
		r.clusterRole,
		r.clusterRoleBinding,
	} {
		if err := fn(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r *RBAC) Finalize(_ context.Context, _ *v1alpha1.ControlPlane) (err error) {
	return nil
}

func (r *RBAC) clusterRole(ctx context.Context) error {
	return r.kubeClient.EnsureCreate(ctx, &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "system:prometheus-monitoring",
		},
		Rules: []rbacv1.PolicyRule{{
			NonResourceURLs: []string{"/metrics"},
			Verbs:           []string{"get"},
		}},
	})
}

func (r *RBAC) clusterRoleBinding(ctx context.Context) error {
	return r.kubeClient.EnsureCreate(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "system:prometheus-monitoring",
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     "system:prometheus-monitoring",
		},
		Subjects: []rbacv1.Subject{{
			APIGroup: rbacv1.GroupName,
			Kind:     rbacv1.GroupKind,
			Name:     "system:monitoring",
		}},
	})
}
