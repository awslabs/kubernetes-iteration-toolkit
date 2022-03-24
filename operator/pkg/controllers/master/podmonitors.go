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

	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/object"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// reconcilePodmonitors patches the required cert volumes by Prometheus to scrape guest cluster metrics
func (c *Controller) reconcilePodmonitors(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	// create pod monitor for API server and etcd pods
	for _, spec := range []monitoringv1.PodMonitorSpec{
		apiServerPodMonitorFor(controlPlane),
		etcdPodMonitorFor(controlPlane),
	} {
		if err := c.kubeClient.EnsureCreate(ctx, object.WithOwner(controlPlane, &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      spec.JobLabel,
				Namespace: controlPlane.Namespace,
				Labels:    map[string]string{"release": "kube-prometheus-stack"},
			},
			Spec: spec,
		})); err != nil {
			return fmt.Errorf("ensuring podmonitor for %s, %w", spec.JobLabel, err)
		}
	}
	return nil
}

func apiServerPodMonitorFor(controlPlane *v1alpha1.ControlPlane) monitoringv1.PodMonitorSpec {
	return monitoringv1.PodMonitorSpec{
		JobLabel:          fmt.Sprintf("%s-apiserver", controlPlane.ClusterName()),
		NamespaceSelector: monitoringv1.NamespaceSelector{MatchNames: []string{controlPlane.Namespace}},
		Selector:          metav1.LabelSelector{MatchLabels: map[string]string{object.AppNameLabelKey: "apiserver"}},
		PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{{
			Port: "https", Scheme: "https",
			TLSConfig: &monitoringv1.PodMetricsEndpointTLSConfig{
				SafeTLSConfig: monitoringv1.SafeTLSConfig{
					ServerName: "kubernetes",
					CA: monitoringv1.SecretOrConfigMap{Secret: &v1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{Name: RootCASecretNameFor(controlPlane.ClusterName())},
						Key:                  "public",
					}},
					Cert: monitoringv1.SecretOrConfigMap{Secret: &v1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{Name: PrometheusClientCertsFor(controlPlane.ClusterName())},
						Key:                  "public",
					}},
					KeySecret: &v1.SecretKeySelector{
						LocalObjectReference: v1.LocalObjectReference{Name: PrometheusClientCertsFor(controlPlane.ClusterName())},
						Key:                  "private",
					},
				},
			},
		}},
	}
}

func etcdPodMonitorFor(controlPlane *v1alpha1.ControlPlane) monitoringv1.PodMonitorSpec {
	return monitoringv1.PodMonitorSpec{
		JobLabel:            fmt.Sprintf("%s-etcd", controlPlane.ClusterName()),
		NamespaceSelector:   monitoringv1.NamespaceSelector{MatchNames: []string{controlPlane.Namespace}},
		Selector:            metav1.LabelSelector{MatchLabels: map[string]string{object.AppNameLabelKey: "etcd"}},
		PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{{Port: "metrics"}},
	}
}
