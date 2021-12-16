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
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type routeTable struct {
	ec2api EC2
}

// NewRouteTableController returns a controller for managing route tables in AWS
func NewRouteTableController(ec2api *EC2) *routeTable {
	return &routeTable{ec2api: ec2api}
}

// Name returns the name of the controller
func (r *routeTable) resourceName() string {
	return "route-table"
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (r *routeTable) Reconcile(ctx context.Context, substrate *v1alpha1.Substrate) (*reconcile.Result, error) {
	identifier := ""
	// routeTables, err := r.getRouteTables(ctx, identifier)
	// if err != nil {
	// 	return nil, fmt.Errorf("getting route tables, %w", err)
	// }
	// for _, routeTable := range routeTables {
	// 	if containsTable(routeTable.Tags, identifier) {
	// 		if err := r.reconcileTableAssociations(ctx, routeTable, tableObj); err != nil {
	// 			return nil, fmt.Errorf("associate subnets to tables, %w", err)
	// 		}
	// 		zap.S().Debugf("Successfully discovered and subnets associated with route-table %v", tableName(identifier))
	// 		return status.Created, nil
	// 	}
	// }
	result, err := r.createRouteTables(ctx, tableObj)
	if err != nil {
		return result, err
	}
	zap.S().Infof("Successfully created route table %v for cluster %v", tableObj.Name, identifier)
	return status.Created, nil
}

// Finalize deletes the resource from AWS
func (r *routeTable) Finalize(ctx context.Context, substrate *v1alpha1.Substrate) (*reconcile.Result, error) {
	tableObj := object.(*v1alpha1.RouteTable)
	if err := r.deleteRouteTable(ctx, identifier); err != nil {
		return nil, err
	}
	return status.Created, nil
}

func (r *routeTable) createRouteTables(ctx context.Context, tableObj *v1alpha1.RouteTable, identifier string) error {
	// Verify VPCID exists
	vpc, err := getVPC(ctx, r.ec2api, identifier)
	if err != nil {
		return fmt.Errorf("getting VPC %w", err)
	}
	if vpc == nil {
		return fmt.Errorf("vpc does not exist %w", errors.WaitingForSubResources)
	}

	for _, tableName := range []string{privateTableName(identifier), publicTableName(identifier)} {
		routeInput := &ec2.CreateRouteInput{DestinationCidrBlock: aws.String("0.0.0.0/0")}
		routeTableID, err := r.createTable(ctx, *vpc.VpcId, tableObj.Name, identifier)
		if err != nil {
			return err
		}
		if isPrivateTable(tableName) {
			natGW, err := getNatGateway(ctx, r.ec2api, identifier)
			if err != nil || natGW == nil || aws.StringValue(natGW.State) != "available" {
				return fmt.Errorf("nat-gateway does not exist")
			}
			routeInput.NatGatewayId = natGW.NatGatewayId
		} else {
			igw, err := getInternetGateway(ctx, r.ec2api, identifier)
			if err != nil {
				return fmt.Errorf("getting internet-gateway %w", err)
			}
			if igw == nil {
				return fmt.Errorf("internet-gateway does not exist")
			}
			routeInput.GatewayId = igw.InternetGatewayId
		}
		routeInput.RouteTableId = aws.String(routeTableID)
		if _, err := r.ec2api.CreateRouteWithContext(ctx, routeInput); err != nil {
			return fmt.Errorf("adding route to the table, %w", err)
		}
	}
	return nil
}

// func (r *routeTable) createTableWithRoute(ctx context.Context, vpcID string, tableObj *v1alpha1.RouteTable) (*reconcile.Result, error) {
// 	routeInput := &ec2.CreateRouteInput{DestinationCidrBlock: aws.String("0.0.0.0/0")}
// 	if tableObj.Spec.ForPrivateSubnets {
// 		// We need to get nat gateway which is available else we might add a route to
// 		// GW which is pending and GW might end up in the failed state in few
// 		// minutes.
// 		// TODO At some point in the reconciler here we will check the GWs added to
// 		// the routes, if they are still active, until then we need to wait for an
// 		// active NAT GW.
// 		natGW, err := getNatGateway(ctx, r.ec2api, identifier)
// 		if err != nil || natGW == nil || aws.StringValue(natGW.State) != "available" {
// 			return status.Waiting, fmt.Errorf("waiting for nat-gateway, %w", errors.WaitingForSubResources)
// 		}
// 		routeInput.NatGatewayId = natGW.NatGatewayId
// 	} else {
// 		igw, err := getInternetGateway(ctx, r.ec2api, identifier)
// 		if err != nil {
// 			return nil, fmt.Errorf("getting internet-gateway %w", err)
// 		}
// 		if igw == nil {
// 			return status.Waiting, fmt.Errorf("waiting for internet-gateway, %w", errors.WaitingForSubResources)
// 		}
// 		routeInput.GatewayId = igw.InternetGatewayId
// 	}
// 	routeTableID, err := r.createTable(ctx, vpcID, tableObj.Name, identifier)
// 	if err != nil {
// 		return nil, err
// 	}
// 	routeInput.RouteTableId = aws.String(routeTableID)
// 	if _, err := r.ec2api.CreateRouteWithContext(ctx, routeInput); err != nil {
// 		return nil, fmt.Errorf("adding route to the table, %w", err)
// 	}
// 	return status.Created, nil
// }

func (r *routeTable) reconcileTableAssociations(ctx context.Context, routeTable *ec2.RouteTable, desiredObj *v1alpha1.RouteTable) error {
	var subnets []string
	var err error
	if desiredObj.Spec.ForPrivateSubnets {
		subnets, err = getPrivateSubnetIDs(ctx, r.ec2api, desiredObj.Spec.ClusterName)
		if err != nil {
			return fmt.Errorf("waiting for private subnets %w", errors.WaitingForSubResources)
		}
	} else {
		subnets, err = getPublicSubnetIDs(ctx, r.ec2api, desiredObj.Spec.ClusterName)
		if err != nil {
			return fmt.Errorf("waiting for public subnets %w", errors.WaitingForSubResources)
		}
	}
	remaining := []string{}
	for _, subnet := range subnets {
		if !isSubnetAssociated(routeTable.Associations, subnet) {
			remaining = append(remaining, subnet)
		}
	}
	if err := r.associateSubnetsToTable(ctx, *routeTable.RouteTableId, remaining); err != nil {
		return err
	}
	return nil
}

func (r *routeTable) createTable(ctx context.Context, vpcID, identifier string) (string, error) {
	routeTable, err := r.ec2api.CreateRouteTableWithContext(ctx, &ec2.CreateRouteTableInput{
		VpcId:             aws.String(vpcID),
		TagSpecifications: generateEC2TagsWithName(r.resourceName(), identifier, tableName(identifier)),
	})
	if err != nil {
		return "", err
	}
	return *routeTable.RouteTable.RouteTableId, nil
}

func (r *routeTable) associateSubnetsToTable(ctx context.Context, tableID string, subnets []string) error {
	for _, subnet := range subnets {
		_, err := r.ec2api.AssociateRouteTableWithContext(ctx, &ec2.AssociateRouteTableInput{
			RouteTableId: aws.String(tableID),
			SubnetId:     aws.String(subnet),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *routeTable) deleteRouteTable(ctx context.Context, identifier string) error {
	routeTables, err := r.getRouteTables(ctx, identifier)
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

// get all the route tables with the given identifier
func (r *routeTable) getRouteTables(ctx context.Context, identifier string) ([]*ec2.RouteTable, error) {
	output, err := r.ec2api.DescribeRouteTablesWithContext(ctx, &ec2.DescribeRouteTablesInput{
		Filters: ec2FilterFor(identifier),
	})
	if err != nil {
		return nil, fmt.Errorf("describing route tables, %w", err)
	}
	if output == nil || len(output.RouteTables) == 0 {
		return nil, nil
	}
	return output.RouteTables, nil
}

func containsTable(tags []*ec2.Tag, identifier string) bool {
	for _, tag := range tags {
		if aws.StringValue(tag.Key) == "Name" &&
			(aws.StringValue(tag.Value) == privateTableName(identifier) ||
				aws.StringValue(tag.Value) == publicTableName(identifier)) {
			return true
		}
	}
	return false
}

func isSubnetAssociated(associations []*ec2.RouteTableAssociation, subnetID string) bool {
	for _, association := range associations {
		if aws.StringValue(association.SubnetId) == subnetID {
			return true
		}
	}
	return false
}

func privateTableName(identifier string) string {
	return fmt.Sprintf("%s-%s", identifier, "private")
}

func publicTableName(identifier string) string {
	return fmt.Sprintf("%s-%s", identifier, "public")
}
