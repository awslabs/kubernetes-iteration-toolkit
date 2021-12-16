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

package substrate

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type routeTable struct {
	ec2Client *ec2.EC2
}

func (r *routeTable) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if substrate.Status.VPCID == nil {
		return reconcile.Result{Requeue: true}, nil
	}
	publicRouteTable, err := r.ensure(ctx, substrate, publicTableName(substrate.Name))
	if err != nil {
		return reconcile.Result{}, err
	}
	substrate.Status.PublicRouteTableID = publicRouteTable.RouteTableId
	privateRouteTable, err := r.ensure(ctx, substrate, privateTableName(substrate.Name))
	if err != nil {
		return reconcile.Result{}, err
	}
	substrate.Status.PrivateRouteTableID = privateRouteTable.RouteTableId
	return reconcile.Result{}, nil
}

func (r *routeTable) ensure(ctx context.Context, substrate *v1alpha1.Substrate, name string) (*ec2.RouteTable, error) {
	describeRouteTablesOutput, err := r.ec2Client.DescribeRouteTablesWithContext(ctx, &ec2.DescribeRouteTablesInput{Filters: filtersFor(substrate.Name, name)})
	if err != nil {
		return nil, fmt.Errorf("describing route tables, %w", err)
	}
	if len(describeRouteTablesOutput.RouteTables) > 0 {
		logging.FromContext(ctx).Infof("Found route table %s", name)
		return describeRouteTablesOutput.RouteTables[0], nil
	}
	createRouteTableOutput, err := r.ec2Client.CreateRouteTableWithContext(ctx, &ec2.CreateRouteTableInput{
		VpcId:             substrate.Status.VPCID,
		TagSpecifications: tagsFor(ec2.ResourceTypeRouteTable, substrate.Name, name),
	})
	if err != nil {
		return nil, fmt.Errorf("creating route table, %w", err)
	}
	logging.FromContext(ctx).Infof("Created route table %s", name)
	return createRouteTableOutput.RouteTable, nil
}

func (r *routeTable) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	describeRouteTablesOutput, err := r.ec2Client.DescribeRouteTablesWithContext(ctx, &ec2.DescribeRouteTablesInput{Filters: filtersFor(substrate.Name)})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("describing route tables, %w", err)
	}
	if len(describeRouteTablesOutput.RouteTables) == 0 {
		return reconcile.Result{}, nil
	}
	for _, routeTable := range describeRouteTablesOutput.RouteTables {
		if _, err := r.ec2Client.DeleteRouteTableWithContext(ctx, &ec2.DeleteRouteTableInput{RouteTableId: routeTable.RouteTableId}); err != nil {
			if err.(awserr.Error).Code() == "DependencyViolation" {
				return reconcile.Result{Requeue: true}, nil
			}
			return reconcile.Result{}, fmt.Errorf("deleting route table, %w", err)
		}
		logging.FromContext(ctx).Infof("Deleted route table %s", aws.StringValue(routeTable.RouteTableId))
	}
	return reconcile.Result{}, nil
}

func privateTableName(identifier string) string {
	return fmt.Sprintf("%s-%s", identifier, "private")
}

func publicTableName(identifier string) string {
	return fmt.Sprintf("%s-%s", identifier, "public")
}
