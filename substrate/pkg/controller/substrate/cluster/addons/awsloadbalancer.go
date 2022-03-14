package addons

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kit/substrate/pkg/utils/helm"
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
