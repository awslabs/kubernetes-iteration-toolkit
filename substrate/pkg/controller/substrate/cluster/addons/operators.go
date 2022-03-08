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

	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kit/substrate/pkg/controller/substrate/cluster"
	"github.com/awslabs/kit/substrate/pkg/utils/discovery"
	"github.com/awslabs/kit/substrate/pkg/utils/json"
	"go.uber.org/multierr"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type HelmCharts struct {
}

type chart struct {
	namespace string
	name      string
	location  string
	values    json.Value
}

func (h *HelmCharts) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if !substrate.IsReady() {
		return reconcile.Result{Requeue: true}, nil
	}
	charts := []*chart{
		{
			namespace: "kube-system",
			name:      "aws-vpc-cni",
			location:  "https://aws.github.io/eks-charts/aws-vpc-cni-1.1.13.tgz",
		},
		{
			namespace: "kit",
			name:      "kit-operator",
			location:  "https://github.com/awslabs/kubernetes-iteration-toolkit/releases/download/kit-operator-0.0.5/kit-operator-0.0.5.tgz",
		},
		{
			namespace: "karpenter",
			name:      "karpenter",
			location:  "https://charts.karpenter.sh/karpenter-0.5.5.tgz",
			values: json.Value{
				"controller": json.Value{
					"clusterName": substrate.Name, "clusterEndpoint": fmt.Sprintf("https://%s:8443", *substrate.Status.Cluster.Address),
					"resources": json.Value{
						"requests": json.Value{
							"cpu": "100m",
						},
					},
				},
				"aws": json.Value{
					"defaultInstanceProfile": discovery.Name(substrate, cluster.TenantControlPlaneNodeRole),
				},
			},
		},
		{
			namespace: "kube-system",
			name:      "aws-ebs-csi-driver",
			location:  "https://github.com/kubernetes-sigs/aws-ebs-csi-driver/releases/download/helm-chart-aws-ebs-csi-driver-2.6.3/aws-ebs-csi-driver-2.6.3.tgz",
			values: json.Value{
				"controller": json.Value{
					"replicaCount": "1",
				},
			},
		},
		{
			namespace: "kube-system",
			name:      "aws-load-balancer-controller",
			location:  "https://aws.github.io/eks-charts/aws-load-balancer-controller-1.4.0.tgz",
			values: json.Value{
				"clusterName":  substrate.Name,
				"replicaCount": "1",
			},
		},
	}
	errs := make([]error, len(charts))
	workqueue.ParallelizeUntil(ctx, len(charts), len(charts), func(i int) {
		errs[i] = h.ensure(charts[i], substrate)
	})
	if err := multierr.Combine(errs...); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (h *HelmCharts) Delete(_ context.Context, _ *v1alpha1.Substrate) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

func (h *HelmCharts) ensure(chart *chart, substrate *v1alpha1.Substrate) error {
	// Get the chart from the repository
	charts, err := new(getter.HTTPGetter).Get(chart.location)
	if err != nil {
		return fmt.Errorf("getting chart, %w", err)
	}
	// Load archive file in memory and return *chart.Chart
	desiredChart, err := loader.LoadArchive(charts)
	if err != nil {
		return fmt.Errorf("loading chart archive, %w", err)
	}
	actionConfig := new(action.Configuration)
	if err := actionConfig.Init(&genericclioptions.ConfigFlags{
		KubeConfig: substrate.Status.Cluster.KubeConfig, Namespace: &chart.namespace}, chart.namespace, "", discardDebugLogs); err != nil {
		return fmt.Errorf("init helm action config, %w", err)
	}
	// check history for the releaseName, if release is not found install else upgrade
	histClient := action.NewHistory(actionConfig)
	histClient.Max = 1
	if _, err := histClient.Run(chart.name); err == driver.ErrReleaseNotFound {
		installClient := action.NewInstall(actionConfig)
		installClient.Namespace = chart.namespace
		installClient.ReleaseName = chart.name
		installClient.CreateNamespace = true
		if _, err := installClient.Run(desiredChart, chart.values); err != nil {
			return fmt.Errorf("installing chart: %w", err)
		}
		return nil
	}
	upgradeClient := action.NewUpgrade(actionConfig)
	upgradeClient.Namespace = chart.namespace
	if _, err := upgradeClient.Run(chart.name, desiredChart, chart.values); err != nil {
		return fmt.Errorf("upgrading chart: %w", err)
	}
	return nil
}

func discardDebugLogs(_ string, v ...interface{}) {
	// discard for logs from helm library
}
