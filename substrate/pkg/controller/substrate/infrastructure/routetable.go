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

package infrastructure

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kit/substrate/pkg/utils/discovery"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type RouteTable struct {
	EC2 *ec2.EC2
}

func (r *RouteTable) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if substrate.Status.Infrastructure.VPCID == nil {
		return reconcile.Result{Requeue: true}, nil
	}
	publicRouteTable, err := r.ensure(ctx, substrate, discovery.Name(substrate, "public"))
	if err != nil {
		return reconcile.Result{}, err
	}
	substrate.Status.Infrastructure.PublicRouteTableID = publicRouteTable.RouteTableId
	privateRouteTable, err := r.ensure(ctx, substrate, discovery.Name(substrate, "private"))
	if err != nil {
		return reconcile.Result{}, err
	}
	substrate.Status.Infrastructure.PrivateRouteTableID = privateRouteTable.RouteTableId
	return reconcile.Result{}, nil
}

func (r *RouteTable) ensure(ctx context.Context, substrate *v1alpha1.Substrate, name *string) (*ec2.RouteTable, error) {
	describeRouteTablesOutput, err := r.EC2.DescribeRouteTablesWithContext(ctx, &ec2.DescribeRouteTablesInput{Filters: discovery.Filters(substrate, discovery.Name(substrate))})
	if err != nil {
		return nil, fmt.Errorf("describing route tables, %w", err)
	}
	if len(describeRouteTablesOutput.RouteTables) > 0 {
		logging.FromContext(ctx).Debugf("Found route table %s", aws.StringValue(name))
		return describeRouteTablesOutput.RouteTables[0], nil
	}
	createRouteTableOutput, err := r.EC2.CreateRouteTableWithContext(ctx, &ec2.CreateRouteTableInput{
		VpcId: substrate.Status.Infrastructure.VPCID,
		TagSpecifications: []*ec2.TagSpecification{{
			ResourceType: aws.String(ec2.ResourceTypeRouteTable),
			Tags:         discovery.Tags(substrate, discovery.Name(substrate)),
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("creating route table, %w", err)
	}
	logging.FromContext(ctx).Infof("Created route table %s", aws.StringValue(name))
	return createRouteTableOutput.RouteTable, nil
}

func (r *RouteTable) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	describeRouteTablesOutput, err := r.EC2.DescribeRouteTablesWithContext(ctx, &ec2.DescribeRouteTablesInput{Filters: discovery.Filters(substrate)})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("describing route tables, %w", err)
	}
	if len(describeRouteTablesOutput.RouteTables) == 0 {
		return reconcile.Result{}, nil
	}
	for _, routeTable := range describeRouteTablesOutput.RouteTables {
		if _, err := r.EC2.DeleteRouteTableWithContext(ctx, &ec2.DeleteRouteTableInput{RouteTableId: routeTable.RouteTableId}); err != nil {
			if err.(awserr.Error).Code() == "DependencyViolation" {
				return reconcile.Result{Requeue: true}, nil
			}
			return reconcile.Result{}, fmt.Errorf("deleting route table, %w", err)
		}
		logging.FromContext(ctx).Infof("Deleted route table %s", aws.StringValue(routeTable.RouteTableId))
	}
	return reconcile.Result{}, nil
}
