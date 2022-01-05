package substrate

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"go.uber.org/zap"
)

type natGateway struct {
	ec2api *EC2
}

func (n *natGateway) resourceName() string {
	return "natgateway"
}

func (n *natGateway) Create(ctx context.Context, substrate *v1alpha1.Substrate) error {
	// Get elastic IP ID
	if substrate.Status.ElasticIPID == nil {
		return fmt.Errorf("elastic IP allocation ID not found for %v", substrate.Name)
	}
	if len(substrate.Status.PublicSubnetIDs) == 0 {
		return fmt.Errorf("public subnets not found for %v", substrate.Name)
	}
	// Get existing NAT GW
	natGW, err := n.getActiveNatGateway(ctx, substrate.Name)
	if err != nil {
		return err
	}
	if natGW == nil || *natGW.NatGatewayId == "" {
		// Create NAT Gateway
		output, err := n.ec2api.CreateNatGatewayWithContext(ctx, &ec2.CreateNatGatewayInput{
			AllocationId:      substrate.Status.ElasticIPID,
			SubnetId:          aws.String(substrate.Status.PublicSubnetIDs[0]),
			TagSpecifications: generateEC2Tags(n.resourceName(), substrate.Name),
		})
		if err != nil {
			return fmt.Errorf("creating NAT GW for %v", substrate.Name)
		}
		zap.S().Infof("Successfully created nat-gateway %v for cluster %v", *output.NatGateway.NatGatewayId, substrate.Name)
		// Wait for the NAT Gateway to be available
		// If we don't wait and add this GW ID to routes in a route table,
		// it fails saying no NAT GW exists with this ID, although its in pending state.
		func() {
			zap.S().Infof("Waiting for nat-gateway %v to be available for cluster %v", *output.NatGateway.NatGatewayId, substrate.Name)
			if err := n.ec2api.WaitUntilNatGatewayAvailableWithContext(ctx, &ec2.DescribeNatGatewaysInput{
				NatGatewayIds: []*string{output.NatGateway.NatGatewayId},
			}); err != nil {
				zap.S().Errorf("waiting for NAT GW %s to be available for %v", *output.NatGateway.NatGatewayId, substrate.Name)
			}
			zap.S().Infof("nat-gateway %v is available for cluster %v", *output.NatGateway.NatGatewayId, substrate.Name)
		}()
		substrate.Status.NatGatewayID = output.NatGateway.NatGatewayId
		return nil
	}
	substrate.Status.NatGatewayID = natGW.NatGatewayId
	return nil
}

func (n *natGateway) Delete(ctx context.Context, substrate *v1alpha1.Substrate) error {
	natGW, err := n.getActiveNatGateway(ctx, substrate.Name)
	if err != nil {
		return err
	}
	if natGW == nil || *natGW.NatGatewayId == "" {
		return nil
	}
	if _, err := n.ec2api.DeleteNatGatewayWithContext(ctx, &ec2.DeleteNatGatewayInput{
		NatGatewayId: natGW.NatGatewayId,
	}); err != nil {
		return fmt.Errorf("creating NAT GW %s for %v", *natGW.NatGatewayId, substrate.Name)
	}
	// We need to wait for NAT GW to be deleted and SDK doesn't have a wait method for it
	maxTry := 20
	for maxTry > 0 {
		time.Sleep(5 * time.Second)
		deleted, err := isNatGWDeleted(ctx, n.ec2api, substrate.Name)
		if err != nil {
			return err
		}
		if deleted {
			zap.S().Infof("Successfully deleted nat-gateway %v for cluster %v", *natGW.NatGatewayId, substrate.Name)
			return nil
		}
		maxTry--
		fmt.Println("Waiting for NAT GW to be deleted")
	}
	return fmt.Errorf("timed out while waiting for NAT GW to be deleted")
}

func (n *natGateway) getActiveNatGateway(ctx context.Context, identifier string) (*ec2.NatGateway, error) {
	natGWs, err := getNatGateway(ctx, n.ec2api, identifier)
	if err != nil {
		return nil, err
	}
	var result *ec2.NatGateway
	for _, natgw := range natGWs {
		if aws.StringValue(natgw.State) == "deleting" || aws.StringValue(natgw.State) == "deleted" ||
			aws.StringValue(natgw.State) == "failed" {
			continue
		}
		if result != nil {
			return nil, fmt.Errorf("expected to find one nat-gateway, but found %d", len(natGWs))
		}
		result = natgw
	}
	return result, nil
}

// When we get NAT GWs from EC2 there can be multiple NAT GWs returned with deleted state,
// So we need to make sure all of them are in deleted state when cleaning up.
func isNatGWDeleted(ctx context.Context, ec2api *EC2, identifier string) (bool, error) {
	natGWs, err := getNatGateway(ctx, ec2api, identifier)
	if err != nil {
		return false, err
	}
	var result bool = true
	for _, natgw := range natGWs {
		if aws.StringValue(natgw.State) == "deleted" {
			continue
		}
		result = false
	}
	return result, nil
}

func getNatGateway(ctx context.Context, ec2api *EC2, identifier string) ([]*ec2.NatGateway, error) {
	output, err := ec2api.DescribeNatGatewaysWithContext(ctx, &ec2.DescribeNatGatewaysInput{
		Filter: ec2FilterFor(identifier),
	})
	if err != nil {
		return nil, fmt.Errorf("describing nat-gateway, %w", err)
	}
	return output.NatGateways, nil
}
