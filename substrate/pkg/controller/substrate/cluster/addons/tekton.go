package addons

import (
	"context"
	"fmt"

	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kit/substrate/pkg/utils/kubectl"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

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
	return reconcile.Result{}, nil
}

func (t *Tekton) Delete(_ context.Context, _ *v1alpha1.Substrate) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
