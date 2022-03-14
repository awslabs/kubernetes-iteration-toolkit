package addons

import (
	"context"
	"fmt"

	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/utils/helm"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type KITOperator struct {
}

func (l *KITOperator) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if !substrate.Status.IsReady() {
		return reconcile.Result{Requeue: true}, nil
	}
	if err := helm.NewClient(*substrate.Status.Cluster.KubeConfig).Apply(ctx, &helm.Chart{
		Namespace:       "kit",
		Name:            "kit-operator",
		Repository:      "https://github.com/awslabs/kubernetes-iteration-toolkit/releases/download/kit-operator-0.0.8",
		Version:         "0.0.8",
		CreateNamespace: true,
	}); err != nil {
		return reconcile.Result{}, fmt.Errorf("applying chart, %w", err)
	}
	return reconcile.Result{}, nil
}

func (l *KITOperator) Delete(_ context.Context, _ *v1alpha1.Substrate) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
