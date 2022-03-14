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

	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/object"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func (c *Controller) reconcileService(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	return c.kubeClient.EnsureCreate(ctx, object.WithOwner(controlPlane, &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceNameFor(controlPlane.ClusterName()),
			Namespace: controlPlane.Namespace,
			Labels:    labelsFor(controlPlane.ClusterName()),
		},
		Spec: v1.ServiceSpec{
			ClusterIP: v1.ClusterIPNone,
			Selector:  labelsFor(controlPlane.ClusterName()),
			Ports: []v1.ServicePort{{
				Port:       2380,
				Name:       serverPortNameFor(controlPlane.ClusterName()),
				TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: 2380},
				Protocol:   "TCP",
			}, {
				Port:       2379,
				Name:       clientPortNameFor(controlPlane.ClusterName()),
				TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: 2379},
				Protocol:   "TCP",
			}},
		},
	}))
}

func serverPortNameFor(clusterName string) string {
	return fmt.Sprintf("etcd-server-ssl-%s", clusterName)
}

func clientPortNameFor(clusterName string) string {
	return fmt.Sprintf("etcd-client-ssl-%s", clusterName)
}

func ServiceNameFor(clusterName string) string {
	return fmt.Sprintf("%s-etcd", clusterName)
}

func labelsFor(clusterName string) map[string]string {
	return map[string]string{
		object.AppNameLabelKey: ServiceNameFor(clusterName),
	}
}
