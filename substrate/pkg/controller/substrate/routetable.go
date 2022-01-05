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
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"go.uber.org/zap"
)

type routeTable struct {
	ec2api *EC2
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
// else create the resource and then sync status with the substrate.Status
func (r *routeTable) Create(ctx context.Context, substrate *v1alpha1.Substrate) error {
	// Verify VPCID exists
	if substrate.Status.VPCID == nil ||
		substrate.Status.InternetGatewayID == nil ||
		substrate.Status.NatGatewayID == nil {
		return fmt.Errorf("vpc / IGW / NATGW ID not found for %v", substrate.Name)
	}
	routeTables, err := getRouteTables(ctx, r.ec2api, substrate.Name)
	if err != nil {
		return fmt.Errorf("getting route tables %w", err)
	}
	// TODO create only the missing route table
	if len(routeTables) < 2 {
		if err := r.createRouteTables(ctx, substrate); err != nil {
			return err
		}
		zap.S().Infof("Successfully created route table for cluster %v", substrate.Name)
		return nil
	}
	substrate.Status.PrivateRouteTableID = aws.String(parseTableID(routeTables, privateTableName(substrate.Name)))
	substrate.Status.PublicRouteTableID = aws.String(parseTableID(routeTables, publicTableName(substrate.Name)))
	return nil
}

// Finalize deletes the resource from AWS
func (r *routeTable) Delete(ctx context.Context, substrate *v1alpha1.Substrate) error {
	if err := r.deleteRouteTable(ctx, substrate); err != nil {
		return err
	}
	substrate.Status.PrivateRouteTableID = nil
	substrate.Status.PublicRouteTableID = nil
	return nil
}

func (r *routeTable) createRouteTables(ctx context.Context, substrate *v1alpha1.Substrate) error {
	// create private route table
	id, err := r.createTableWithRoute(ctx, *substrate.Status.VPCID, substrate.Name,
		privateTableName(substrate.Name), &ec2.CreateRouteInput{
			DestinationCidrBlock: aws.String("0.0.0.0/0"),
			NatGatewayId:         substrate.Status.NatGatewayID,
		})
	if err != nil {
		return fmt.Errorf("creating private route table, %w", err)
	}
	substrate.Status.PrivateRouteTableID = aws.String(id)
	// create public route table
	id, err = r.createTableWithRoute(ctx, *substrate.Status.VPCID, substrate.Name,
		publicTableName(substrate.Name), &ec2.CreateRouteInput{
			DestinationCidrBlock: aws.String("0.0.0.0/0"),
			GatewayId:            substrate.Status.InternetGatewayID,
		})
	if err != nil {
		return fmt.Errorf("creating public route table, %w", err)
	}
	substrate.Status.PublicRouteTableID = aws.String(id)
	return nil
}

func (r *routeTable) createTableWithRoute(ctx context.Context, vpcID, identifier, tableName string, route *ec2.CreateRouteInput) (string, error) {
	routeTable, err := r.ec2api.CreateRouteTableWithContext(ctx, &ec2.CreateRouteTableInput{
		VpcId:             aws.String(vpcID),
		TagSpecifications: generateEC2TagsWithName(r.resourceName(), identifier, tableName),
	})
	if err != nil {
		return "", fmt.Errorf("creating route table, %w", err)
	}
	route.RouteTableId = routeTable.RouteTable.RouteTableId
	if _, err := r.ec2api.CreateRouteWithContext(ctx, route); err != nil {
		return *routeTable.RouteTable.RouteTableId, fmt.Errorf("adding route to the table, %w", err)
	}
	return *routeTable.RouteTable.RouteTableId, nil
}

func (r *routeTable) deleteRouteTable(ctx context.Context, substrate *v1alpha1.Substrate) error {
	routeTableIDS, err := getRouteTableIDs(ctx, r.ec2api, substrate.Name)
	if err != nil {
		return fmt.Errorf("getting route tables for %v, %w", substrate.Name, err)
	}
	for _, routeTableID := range routeTableIDS {
		if _, err := r.ec2api.DeleteRouteTableWithContext(ctx, &ec2.DeleteRouteTableInput{
			RouteTableId: aws.String(routeTableID),
		}); err != nil {
			return fmt.Errorf("deleting route table %w", err)
		}
	}
	return nil
}
func getRouteTableIDs(ctx context.Context, ec2api *EC2, identifier string) ([]string, error) {
	routeTables, err := getRouteTables(ctx, ec2api, identifier)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, table := range routeTables {
		ids = append(ids, *table.RouteTableId)
	}
	return ids, nil
}

// get all the route tables with the given identifier
func getRouteTables(ctx context.Context, ec2api *EC2, identifier string) ([]*ec2.RouteTable, error) {
	output, err := ec2api.DescribeRouteTablesWithContext(ctx, &ec2.DescribeRouteTablesInput{
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

func parseTableID(tables []*ec2.RouteTable, name string) string {
	for _, table := range tables {
		if strings.EqualFold(tableName(table), name) {
			return *table.RouteTableId
		}
	}
	return ""
}

func tableName(table *ec2.RouteTable) string {
	for _, tag := range table.Tags {
		if *tag.Key == "Name" {
			return *tag.Value
		}
	}
	return ""
}

func privateTableName(identifier string) string {
	return fmt.Sprintf("%s-%s", identifier, "private")
}

func publicTableName(identifier string) string {
	return fmt.Sprintf("%s-%s", identifier, "public")
}
