package substrate

import (
	"context"
	"fmt"

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

func (n *natGateway) Provision(ctx context.Context, substrate *v1alpha1.Substrate) error {
	identifier := ""
	// 1. Get elastic IP ID
	elasticIP, err := getElasticIP(ctx, n.ec2api, identifier)
	if err != nil {
		return err
	}
	if elasticIP == nil || aws.StringValue(elasticIP.AllocationId) == "" {
		return fmt.Errorf("elastic IP not found")
	}
	// 2. Get a private subnet ID
	publicSubnets, err := getPublicSubnetIDs(ctx, n.ec2api, identifier)
	if err != nil {
		return fmt.Errorf("getting private subnets, %w", err)
	}
	if len(publicSubnets) == 0 {
		return fmt.Errorf("public subnets not found")
	}
	// 3. Create NAT Gateway
	output, err := n.ec2api.CreateNatGatewayWithContext(ctx, &ec2.CreateNatGatewayInput{
		AllocationId:      elasticIP.AllocationId,
		SubnetId:          aws.String(publicSubnets[0]),
		TagSpecifications: generateEC2Tags(n.resourceName(), identifier),
	})
	if err != nil {
		return err
	}
	zap.S().Infof("Successfully created nat-gateway %v for cluster %v", *output.NatGateway.NatGatewayId, identifier)
	// 3. Wait for the NAT Gateway to be available
	// There are scenarios where after creating a NAT gateway, describe NAT GW
	// call doesn't return the NatGateway ID we just created. In such cases, we
	// end up creating multiple gateways, in the end only one becomes available
	// and others end up in the failed state.
	zap.S().Infof("Waiting for nat-gateway %v to be available for cluster %v", *output.NatGateway.NatGatewayId, identifier)
	if err := n.ec2api.WaitUntilNatGatewayAvailableWithContext(ctx, &ec2.DescribeNatGatewaysInput{
		NatGatewayIds: []*string{output.NatGateway.NatGatewayId},
	}); err != nil {
		return err
	}
	zap.S().Infof("Nat-gateway %v is available for cluster %v", *output.NatGateway.NatGatewayId, identifier)
	return nil
}

func (n *natGateway) Deprovision(ctx context.Context, substrate *v1alpha1.Substrate) error {
	identifier := ""
	natGW, err := n.getNatGateway(ctx, identifier)
	if err != nil {
		return err
	}
	if natGW == nil || *natGW.NatGatewayId == "" {
		return nil
	}
	if _, err := n.ec2api.DeleteNatGatewayWithContext(ctx, &ec2.DeleteNatGatewayInput{
		NatGatewayId: aws.String(*natGW.NatGatewayId),
	}); err != nil {
		return err
	}
	zap.S().Infof("Successfully deleted nat-gateway %v for cluster %v", *natGW.NatGatewayId, identifier)
	return nil
}

func (n *natGateway) getNatGateway(ctx context.Context, identifier string) (*ec2.NatGateway, error) {
	return getNatGateway(ctx, n.ec2api, identifier)
}

func getNatGateway(ctx context.Context, ec2api *EC2, identifier string) (*ec2.NatGateway, error) {
	output, err := ec2api.DescribeNatGatewaysWithContext(ctx, &ec2.DescribeNatGatewaysInput{
		Filter: ec2FilterFor(identifier),
	})
	if err != nil {
		return nil, fmt.Errorf("describing nat-gateway, %w", err)
	}
	if len(output.NatGateways) == 0 {
		return nil, nil
	}
	var result *ec2.NatGateway
	for _, natgw := range output.NatGateways {
		if aws.StringValue(natgw.State) == "deleting" || aws.StringValue(natgw.State) == "deleted" ||
			aws.StringValue(natgw.State) == "failed" {
			continue
		}
		if result != nil {
			return nil, fmt.Errorf("expected to find one nat-gateway, but found %d", len(output.NatGateways))
		}
		result = natgw
	}
	return result, nil
}
