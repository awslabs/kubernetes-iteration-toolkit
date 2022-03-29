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
	"fmt"

	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/utils/kubectl"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var rbac = `
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  labels:
    app.kubernetes.io/component: dashboard
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-dashboard
  name: tekton-pipelines-executor
rules:
- apiGroups: ["kit.k8s.sh"]
  resources: ["controlplanes", "dataplanes"]
  verbs: ["get", "list", "watch", "create", "update", "delete", "patch"]
- apiGroups: [""]
  resources: ["serviceaccounts", "secrets", "namespaces", "nodes"]
  verbs: ["get", "list", "watch", "create", "update", "delete", "patch"]
- apiGroups: ["apiextensions.k8s.io"]
  resources: ["customresourcedefinitions"]
  verbs: ["get", "list", "watch", "create", "update", "delete", "patch"]
- apiGroups: ["rbac.authorization.k8s.io"]
  resources: ["clusterroles", "clusterrolebindings"]
  verbs: ["get", "list", "watch", "create", "update", "delete", "patch"]
- apiGroups: ["apps"]
  resources: ["daemonsets"]
  verbs: ["get", "list", "watch", "create", "update", "delete", "patch"]
- apiGroups: ["karpenter.sh"]
  resources: ["provisioners"]
  verbs: ["get"]
- apiGroups: ["certificates.k8s.io"]
  resources: ["certificatesigningrequests", "certificatesigningrequests/approval"]
  verbs: ["get", "list", "watch", "create", "update", "delete", "patch"]
- apiGroups: ["certificates.k8s.io"]
  resources: ["signers"]
  verbs: ["approve"]
---
apiVersion: v1
kind: ServiceAccount
metadata:
  labels:
    app.kubernetes.io/component: dashboard
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-dashboard
  name: tekton-pipelines-executor
  namespace: tekton-pipelines
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  labels:
    app.kubernetes.io/component: dashboard
    app.kubernetes.io/instance: default
    app.kubernetes.io/part-of: tekton-dashboard
  name: tekton-pipelines-executor
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: tekton-pipelines-executor
subjects:
- kind: ServiceAccount
  name: tekton-pipelines-executor
  namespace: tekton-pipelines
`

type Tekton struct {
}

func (t *Tekton) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if !substrate.Status.IsReady() {
		return reconcile.Result{Requeue: true}, nil
	}
	client, err := kubectl.NewClient(*substrate.Status.Cluster.KubeConfig)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("initializing client, %w", err)
	}
	for _, file := range []string{
		"https://storage.googleapis.com/tekton-releases/pipeline/previous/v0.33.2/release.yaml",
		"https://storage.googleapis.com/tekton-releases/triggers/previous/v0.19.0/release.yaml",
		"https://github.com/tektoncd/dashboard/releases/download/v0.24.1/tekton-dashboard-release.yaml",
	} {
		if err := client.Apply(ctx, file); err != nil {
			return reconcile.Result{}, fmt.Errorf("applying tekton, %w", err)
		}
	}
	if err := client.ApplyYAML(ctx, []byte(rbac)); err != nil {
		return reconcile.Result{}, fmt.Errorf("applying tekton dashboard RBAC for KIT resources, %w", err)
	}
	return reconcile.Result{}, nil
}

func (t *Tekton) Delete(_ context.Context, _ *v1alpha1.Substrate) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
