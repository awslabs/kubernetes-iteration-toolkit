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
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/utils/helm"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type AWSLoadBalancer struct {
	EC2 *ec2.EC2
}

func (l *AWSLoadBalancer) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if !substrate.Status.IsReady() {
		return reconcile.Result{Requeue: true}, nil
	}
	if err := helm.NewClient(*substrate.Status.Cluster.KubeConfig).Apply(ctx, &helm.Chart{
		Namespace:  "kube-system",
		Name:       "aws-load-balancer-controller",
		Repository: "https://aws.github.io/eks-charts",
		Version:    "1.4.0",
		Values: map[string]interface{}{
			"clusterName":  substrate.Name,
			"replicaCount": "1",
		},
	}); err != nil {
		return reconcile.Result{}, fmt.Errorf("applying chart, %w", err)
	}
	if _, err := l.EC2.CreateTagsWithContext(ctx, &ec2.CreateTagsInput{
		Resources: aws.StringSlice(substrate.Status.Infrastructure.PublicSubnetIDs),
		Tags:      []*ec2.Tag{{Key: aws.String("kubernetes.io/role/elb"), Value: aws.String("1")}},
	}); err != nil {
		return reconcile.Result{}, fmt.Errorf("tagging resources, %w", err)
	}
	logging.FromContext(ctx).Debug("Tagged subnets with %s=%s", "kubernetes.io/role/elb", "1")
	return reconcile.Result{}, nil
}

func (l *AWSLoadBalancer) Delete(_ context.Context, _ *v1alpha1.Substrate) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
