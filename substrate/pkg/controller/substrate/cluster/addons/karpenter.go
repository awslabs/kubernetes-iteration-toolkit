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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/controller/substrate/cluster"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/utils/discovery"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/utils/helm"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/utils/kubectl"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Karpenter struct {
	EC2 *ec2.EC2
}

var provisioner = `
apiVersion: karpenter.sh/v1alpha5
kind: Provisioner
metadata:
  name: default
spec:
  requirements:
  - key: kit.k8s.sh/app
    operator: Exists
  - key: kit.k8s.sh/control-plane-name
    operator: Exists
  kubeletConfiguration:
    clusterDNS:
      - "10.96.0.10"
  ttlSecondsAfterEmpty: 30
  provider:
    instanceProfile: %[1]s-tenant-controlplane-node-role
    tags:
      kit.aws/substrate: %[1]s
    subnetSelector:
      karpenter.sh/discovery: %[1]s
    securityGroupSelector:
      karpenter.sh/discovery: %[1]s
`

func (k *Karpenter) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if !substrate.Status.IsReady() {
		return reconcile.Result{Requeue: true}, nil
	}
	if err := helm.NewClient(*substrate.Status.Cluster.KubeConfig).Apply(ctx, &helm.Chart{
		Namespace:       "karpenter",
		Name:            "karpenter",
		Repository:      "https://charts.karpenter.sh",
		Version:         "0.7.3",
		CreateNamespace: true,
		Values: map[string]interface{}{
			"clusterName":     substrate.Name,
			"clusterEndpoint": fmt.Sprintf("https://%s:8443", *substrate.Status.Cluster.APIServerAddress),
			"controller": map[string]interface{}{
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
						"cpu":    "100m",
						"memory": "500Mi",
					},
				},
			},
			"aws": map[string]interface{}{
				"defaultInstanceProfile": discovery.Name(substrate, cluster.TenantControlPlaneNodeRole),
			},
		},
	}); err != nil {
		return reconcile.Result{}, fmt.Errorf("applying chart, %w", err)
	}
	client, err := kubectl.NewClient(*substrate.Status.Cluster.KubeConfig)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("initializing client, %w", err)
	}
	// Tag EC2 Resources
	if _, err := k.EC2.CreateTagsWithContext(ctx, &ec2.CreateTagsInput{
		Resources: aws.StringSlice(append(
			substrate.Status.Infrastructure.PublicSubnetIDs,
			aws.StringValue(substrate.Status.Infrastructure.SecurityGroupID),
		)),
		Tags: []*ec2.Tag{{Key: aws.String("karpenter.sh/discovery"), Value: aws.String(substrate.Name)}},
	}); err != nil {
		return reconcile.Result{}, fmt.Errorf("tagging resources, %w", err)
	}
	logging.FromContext(ctx).Debugf("Tagged subnets and security groups with %s=%s", "karpenter.sh/discovery", substrate.Name)
	// Apply Provisioner
	if err := client.ApplyYAML(ctx, []byte(fmt.Sprintf(provisioner, substrate.Name))); err != nil {
		return reconcile.Result{}, fmt.Errorf("applying provisioner, %w", err)
	}
	logging.FromContext(ctx).Debug("Applied default provisioner")
	return reconcile.Result{}, nil
}

func (k *Karpenter) Delete(_ context.Context, _ *v1alpha1.Substrate) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
