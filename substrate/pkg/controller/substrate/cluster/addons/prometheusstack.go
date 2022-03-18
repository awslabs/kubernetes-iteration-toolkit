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
	"bytes"
	"context"
	"fmt"
	"html/template"

	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/utils/helm"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/utils/kubectl"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type PrometheusStack struct{}

func (p *PrometheusStack) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if !substrate.Status.IsReady() {
		return reconcile.Result{Requeue: true}, nil
	}
	if err := helm.NewClient(*substrate.Status.Cluster.KubeConfig).Apply(ctx, &helm.Chart{
		Namespace:       "monitoring",
		Name:            "kube-prometheus-stack",
		Repository:      "https://github.com/prometheus-community/helm-charts/releases/download/kube-prometheus-stack-34.0.0/",
		Version:         "34.0.0",
		CreateNamespace: true,
		Values: map[string]interface{}{
			"coreDns":               map[string]interface{}{"enabled": false},
			"kubeProxy":             map[string]interface{}{"enabled": false},
			"kubeEtcd":              map[string]interface{}{"enabled": false},
			"alertmanager":          map[string]interface{}{"enabled": false},
			"kubeScheduler":         map[string]interface{}{"enabled": false},
			"kubeApiServer":         map[string]interface{}{"enabled": false},
			"kubeStateMetrics":      map[string]interface{}{"enabled": false},
			"kubeControllerManager": map[string]interface{}{"enabled": false},
			"prometheus":            map[string]interface{}{"serviceMonitor": map[string]interface{}{"selfMonitor": false}},
			"prometheusOperator":    map[string]interface{}{"serviceMonitor": map[string]interface{}{"selfMonitor": false}},
		},
	}); err != nil {
		return reconcile.Result{}, fmt.Errorf("applying chart, %w", err)
	}
	// configure podmonitors
	client, err := kubectl.NewClient(*substrate.Status.Cluster.KubeConfig)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("initializing client, %w", err)
	}
	for _, name := range []string{"apiserver", "etcd"} {
		var buf bytes.Buffer
		tmpl := template.Must(template.New("Text").Parse(podMonitorTemplate))
		err := tmpl.Execute(&buf, struct{ ComponentName string }{ComponentName: name})
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("error when executing template, %w", err)
		}
		if err := client.ApplyYAML(ctx, buf.Bytes()); err != nil {
			return reconcile.Result{}, fmt.Errorf("applying pod monitors, %w", err)
		}
	}
	return reconcile.Result{}, nil
}

func (p *PrometheusStack) Delete(_ context.Context, _ *v1alpha1.Substrate) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

var podMonitorTemplate = `
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: {{ .ComponentName }}-pods
  namespace: monitoring
  labels:
    release: kube-prometheus-stack
spec:
  jobLabel: {{ .ComponentName }}-guest
  namespaceSelector:
    matchNames:
    - default
  selector:
    matchLabels:
      kit.k8s.sh/app: {{ .ComponentName }}
  podMetricsEndpoints:
  - port: metrics
`
