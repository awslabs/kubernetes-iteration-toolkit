package substrate

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
)

type routeTableAssociation struct {
	ec2api *EC2
}

func (r *routeTableAssociation) Provision(ctx context.Context, substrate *v1alpha1.Substrate) error {
	// identifier := ""
	// routeTables, err := getRouteTables(ctx, r.ec2api, identifier)
	// if err != nil {
	// 	return fmt.Errorf("getting route tables for %v, %w", identifier, err)
	// }
	if substrate.Status.PrivateRouteTableID == nil && substrate.Status.PublicRouteTableID == nil {
		return fmt.Errorf("route tables not found")
	}
	// associate private route table and private subnets
	if err := r.associateSubnetsToTable(ctx, *substrate.Status.PrivateRouteTableID, substrate.Status.PrivateSubnetIDs); err != nil {
		return fmt.Errorf("associating private route table and subnets, %w", err)
	}
	// associate public route table and public subnets
	if err := r.associateSubnetsToTable(ctx, *substrate.Status.PublicRouteTableID, substrate.Status.PublicSubnetIDs); err != nil {
		return fmt.Errorf("associating public route table and subnets, %w", err)
	}

	// for _, routeTable := range routeTables {
	// 	subnetIDs := []string{}
	// 	switch tableName(routeTable.Tags) {
	// 	case privateTableName(identifier):
	// 		subnetIDs, err = privateSubnetIDs(ctx, r.ec2api, identifier)
	// 	case publicTableName(identifier):
	// 		subnetIDs, err = publicSubnetIDs(ctx, r.ec2api, identifier)
	// 	}
	// 	remaining := []string{}
	// 	for _, subnet := range subnetIDs {
	// 		if !isSubnetAssociated(routeTable.Associations, subnet) {
	// 			remaining = append(remaining, subnet)
	// 		}
	// 	}
	// 	if err := r.associateSubnetsToTable(ctx, *routeTable.RouteTableId, remaining); err != nil {
	// 		return err
	// 	}
	// }
	return nil
}

func (r *routeTableAssociation) associateSubnetsToTable(ctx context.Context, tableID string, subnets []string) error {
	for _, subnet := range subnets {
		_, err := r.ec2api.AssociateRouteTableWithContext(ctx, &ec2.AssociateRouteTableInput{
			RouteTableId: aws.String(tableID),
			SubnetId:     aws.String(subnet),
		})
		// TODO handle error when subnet is already associated and continue
		if err != nil {
			return fmt.Errorf("associate route table %v subnet ID %v, %w", tableID, subnet, err)
		}
	}
	return nil
}

func (r *routeTableAssociation) Deprovision(ctx context.Context, _ *v1alpha1.Substrate) error {
	return nil
}

// func tableName(tags []*ec2.Tag) string {
// 	for _, tag := range tags {
// 		if aws.StringValue(tag.Key) == "Name" {
// 			return aws.StringValue(tag.Value)
// 		}
// 	}
// 	return ""
// }

// func isSubnetAssociated(associations []*ec2.RouteTableAssociation, subnetID string) bool {
// 	for _, association := range associations {
// 		if aws.StringValue(association.SubnetId) == subnetID {
// 			return true
// 		}
// 	}
// 	return false
// }
