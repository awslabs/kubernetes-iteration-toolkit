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

package controller

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/prateekgogia/kit/pkg/apis/infrastructure/v1alpha1"
	"github.com/prateekgogia/kit/pkg/awsprovider"
	"github.com/prateekgogia/kit/pkg/controllers"
	"github.com/prateekgogia/kit/pkg/kiterr"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	desiredRouteTableCount = 2
)

type routeTable struct {
	ec2api *awsprovider.EC2
}

// NewRouteTableController returns a controller for managing route tables in AWS
func NewRouteTableController(ec2api *awsprovider.EC2) *routeTable {
	return &routeTable{ec2api: ec2api}
}

// Name returns the name of the controller
func (r *routeTable) Name() string {
	return "route-table"
}

// For returns the resource this controller is for.
func (r *routeTable) For() controllers.Object {
	return &v1alpha1.ControlPlane{}
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (r *routeTable) Reconcile(ctx context.Context, object controllers.Object) (reconcile.Result, error) {
	controlPlane := object.(*v1alpha1.ControlPlane)
	routeTables, err := r.getRouteTables(ctx, controlPlane.Name)
	if err != nil {
		return resourceReconcileFailed, fmt.Errorf("getting route tables, %w", err)
	}
	if len(routeTables) < desiredRouteTableCount {
		if err := r.createRouteTables(ctx, controlPlane); err != nil {
			return resourceReconcileFailed, err
		}
	} else {
		zap.S().Debugf("Successfully discovered route-tables for cluster %v", controlPlane.Name)
	}
	return resourceReconcileSucceeded, nil
}

// Finalize deletes the resource from AWS
func (r *routeTable) Finalize(ctx context.Context, object controllers.Object) (reconcile.Result, error) {
	controlPlane := object.(*v1alpha1.ControlPlane)
	if err := r.deleteRouteTable(ctx, controlPlane.Name); err != nil {
		return resourceReconcileFailed, err
	}
	return resourceReconcileSucceeded, nil
}

func (r *routeTable) createRouteTables(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	// Verify VPCID exists
	if controlPlane.Status.Infrastructure.VPCID == "" {
		return fmt.Errorf("vpc does not exist %w", kiterr.WaitingForSubResources)
	}
	// Verify private subnets exists
	if len(controlPlane.Status.Infrastructure.PrivateSubnets) != len(privateSubnetCIDRs) {
		return fmt.Errorf("private subnets do not exist %w", kiterr.WaitingForSubResources)
	}
	// Verify public subnets exists
	if len(controlPlane.Status.Infrastructure.PublicSubnets) != len(publicSubnetCIDRs) {
		return fmt.Errorf("public subnets do not exist %w", kiterr.WaitingForSubResources)
	}
	// Verify internet gateway exists
	if controlPlane.Status.Infrastructure.InternetGatewayID == "" {
		return fmt.Errorf("internet-gateway does not exist %w", kiterr.WaitingForSubResources)
	}
	// We need to get nat gateway which is active else we might add a route to
	// GW which is pending and GW might end up in the failed state in few
	// minutes.
	// TODO At some point in the reconciler here we will check the GWs added to
	// the routes, if they are still active, until then we need to wait for an
	// active NAT GW.
	natGW, err := getNatGateway(ctx, r.ec2api, controlPlane.Name)
	if err != nil || natGW == nil || aws.StringValue(natGW.State) != "available" {
		return fmt.Errorf("nat-gateway does not exist %w", kiterr.WaitingForSubResources)
	}
	if err := r.createRoutesForPrivateSubnets(ctx, controlPlane, *natGW.NatGatewayId); err != nil {
		return err
	}
	zap.S().Infof("Successfully created route table for private subnets for cluster %v", controlPlane.Name)
	if err := r.createRoutesForPublicSubnets(ctx, controlPlane); err != nil {
		return err
	}
	zap.S().Infof("Successfully created route table for public subnets for cluster %v", controlPlane.Name)
	return nil
}

func (r *routeTable) createRoutesForPrivateSubnets(ctx context.Context, controlPlane *v1alpha1.ControlPlane, natgw string) error {
	vpcID := controlPlane.Status.Infrastructure.VPCID
	natGWID := natgw
	subnets := controlPlane.Status.Infrastructure.PrivateSubnets
	routeTableID, err := r.createTableAndAssociateSubnets(ctx, vpcID, controlPlane.Name, subnets)
	if err != nil {
		return err
	}
	if _, err := r.ec2api.CreateRouteWithContext(ctx, &ec2.CreateRouteInput{
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		NatGatewayId:         aws.String(natGWID),
		RouteTableId:         aws.String(routeTableID),
	}); err != nil {
		return fmt.Errorf("adding route to the table, %w", err)
	}
	return nil
}

func (r *routeTable) createRoutesForPublicSubnets(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	vpcID := controlPlane.Status.Infrastructure.VPCID
	igwID := controlPlane.Status.Infrastructure.InternetGatewayID
	subnets := controlPlane.Status.Infrastructure.PublicSubnets
	routeTableID, err := r.createTableAndAssociateSubnets(ctx, vpcID, controlPlane.Name, subnets)
	if err != nil {
		return err
	}
	if _, err := r.ec2api.CreateRouteWithContext(ctx, &ec2.CreateRouteInput{
		DestinationCidrBlock: aws.String("0.0.0.0/0"),
		GatewayId:            aws.String(igwID),
		RouteTableId:         aws.String(routeTableID),
	}); err != nil {
		return fmt.Errorf("adding route to the table, %w", err)
	}
	return nil
}

func (r *routeTable) createTableAndAssociateSubnets(ctx context.Context, vpcID, clusterName string, subnets []string) (string, error) {
	routeTable, err := r.ec2api.CreateRouteTableWithContext(ctx, &ec2.CreateRouteTableInput{
		VpcId:             aws.String(vpcID),
		TagSpecifications: generateEC2Tags(r.Name(), clusterName),
	})
	if err != nil {
		return "", err
	}
	for _, subnet := range subnets {
		_, err := r.ec2api.AssociateRouteTableWithContext(ctx, &ec2.AssociateRouteTableInput{
			RouteTableId: aws.String(*routeTable.RouteTable.RouteTableId),
			SubnetId:     aws.String(subnet),
		})
		if err != nil {
			return "", err
		}
	}
	return *routeTable.RouteTable.RouteTableId, nil
}

func (r *routeTable) deleteRouteTable(ctx context.Context, clusterName string) error {
	routeTables, err := r.getRouteTables(ctx, clusterName)
	if err != nil {
		return err
	}
	if len(routeTables) != 0 {
		for _, routeTable := range routeTables {
			if _, err := r.ec2api.DeleteRouteTableWithContext(ctx, &ec2.DeleteRouteTableInput{
				RouteTableId: routeTable.RouteTableId,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *routeTable) getRouteTables(ctx context.Context, clusterName string) ([]*ec2.RouteTable, error) {
	output, err := r.ec2api.DescribeRouteTablesWithContext(ctx, &ec2.DescribeRouteTablesInput{
		Filters: generateEC2Filter(clusterName),
	})
	if err != nil {
		return nil, fmt.Errorf("describing route tables, %w", err)
	}
	if output == nil || len(output.RouteTables) == 0 {
		return nil, nil
	}
	if len(output.RouteTables) > desiredRouteTableCount {
		return nil, fmt.Errorf("expected to find two route tables, but found %d", len(output.RouteTables))
	}
	return output.RouteTables, nil
}
