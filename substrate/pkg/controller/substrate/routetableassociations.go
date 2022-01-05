package substrate

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
)

type routeTableAssociation struct {
	ec2Client  *ec2.EC2
	routeTable *routeTable
}

func (r *routeTableAssociation) Create(ctx context.Context, substrate *v1alpha1.Substrate) error {
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
	return nil
}

func (r *routeTableAssociation) associateSubnetsToTable(ctx context.Context, tableID string, subnets []string) error {
	for _, subnet := range subnets {
		_, err := r.ec2Client.AssociateRouteTableWithContext(ctx, &ec2.AssociateRouteTableInput{
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

// Before deleting the route tables we first need to disassociate routes from
// it. If we try to delete route table before removing routes from it, we get
// the DependencyViolation: routeTable has dependencies and cannot be deleted.
func (r *routeTableAssociation) Delete(ctx context.Context, substrate *v1alpha1.Substrate) error {
	// Since the substrate status is not populated with the table IDs we need to get it directly from AWS API
	tables, err := r.routeTable.getRouteTables(ctx, substrate.Name)
	if err != nil {
		return err
	}
	for _, table := range tables {
		for _, association := range table.Associations {
			if _, err := r.ec2Client.DisassociateRouteTableWithContext(ctx, &ec2.DisassociateRouteTableInput{
				AssociationId: association.RouteTableAssociationId,
			}); err != nil {
				return fmt.Errorf("disassociating private route table and subnets, %w", err)
			}
		}
	}
	return nil
}
