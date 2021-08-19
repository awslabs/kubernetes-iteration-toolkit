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

package environment

import (
	"context"
	"strings"

	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type FakeKubeClient struct {
	client.Client
}

func (f *FakeKubeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	// some of the master resources depend on the LB Hostname
	if strings.HasSuffix(key.Name, "-controlplane-endpoint") {
		if err := f.Client.Get(ctx, key, obj); err != nil {
			return err
		}
		svc := obj.(*v1.Service)
		svc.Status.LoadBalancer.Ingress = []v1.LoadBalancerIngress{{Hostname: "elb-endpoint"}}
		return nil
	}
	return f.Client.Get(ctx, key, obj)
}
