package substrate

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"go.uber.org/zap"
)

var (
	azs            = []string{"us-west-2a", "us-west-2b", "us-west-2c"}
	privateSubnets = []string{"10.1.0.0/24", "10.2.0.0/24", "10.3.0.0/24"}
	publicSubnets  = []string{"10.100.0.0/24", "10.101.0.0/24", "10.102.0.0/24"}
)

type subnet struct {
	ec2api *EC2
}

func (s *subnet) resourceName() string {
	return "subnet"
}

func (s *subnet) Provision(ctx context.Context, substrate *v1alpha1.Substrate) error {
	identifier := ""
	// for all AZs, provision a pivate and a public subnet
	vpc, err := getVPC(ctx, s.ec2api, identifier)
	if err != nil {
		return fmt.Errorf("getting VPC %w", err)
	}
	if vpc == nil {
		// return fmt.Errorf("vpc does not exist %v, %w", identifier, errors.WaitingForSubResources)
	}
	for _, az := range azs {
		for _, privateSubnet := range privateSubnets {
			if _, err := s.create(ctx, *vpc.VpcId, az, privateSubnet, identifier); err != nil {
				return fmt.Errorf("provisioning private subnet, %w", err)
			}
		}
		for _, publicSubnet := range publicSubnets {
			subnetID, err := s.create(ctx, *vpc.VpcId, az, publicSubnet, identifier)
			if err != nil {
				return fmt.Errorf("provisioning public subnet, %w", err)
			}
			if err := s.markSubnetPublic(ctx, subnetID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *subnet) Deprovision(ctx context.Context, substrate *v1alpha1.Substrate) error {
	identifier := ""
	subnets, err := getSubnets(ctx, s.ec2api, identifier)
	if err != nil {
		return err
	}
	if len(subnets) != 0 {
		if err := s.deleteSubnets(ctx, subnets); err != nil {
			return err
		}
		zap.S().Infof("Successfully deleted Subnets for %v", identifier)
	}
	return nil
}

func (s *subnet) create(ctx context.Context, vpcID, az, subnetCIDR, identifier string) (string, error) {
	output, err := s.ec2api.CreateSubnetWithContext(ctx, &ec2.CreateSubnetInput{
		AvailabilityZone:  aws.String(az),
		CidrBlock:         aws.String(subnetCIDR),
		VpcId:             aws.String(vpcID),
		TagSpecifications: generateEC2Tags(s.resourceName(), identifier),
	})
	if err != nil {
		return "", fmt.Errorf("creating subnet, %w", err)
	}
	return *output.Subnet.SubnetId, nil
}

func (s *subnet) markSubnetPublic(ctx context.Context, subnetID string) error {
	attributeInput := &ec2.ModifySubnetAttributeInput{
		MapPublicIpOnLaunch: &ec2.AttributeBooleanValue{
			Value: aws.Bool(true),
		},
		SubnetId: aws.String(subnetID),
	}
	if _, err := s.ec2api.ModifySubnetAttribute(attributeInput); err != nil {
		return fmt.Errorf("modifying subnet attributes, %w", err)
	}
	return nil
}

func (s *subnet) deleteSubnets(ctx context.Context, subnets []*ec2.Subnet) error {
	for _, subnet := range subnets {
		if _, err := s.ec2api.DeleteSubnetWithContext(ctx, &ec2.DeleteSubnetInput{
			SubnetId: subnet.SubnetId,
		}); err != nil {
			return fmt.Errorf("deleting subnet, %w", err)
		}
	}
	return nil
}

func getSubnets(ctx context.Context, ec2api *EC2, clusterName string) ([]*ec2.Subnet, error) {
	input := &ec2.DescribeSubnetsInput{
		Filters: ec2FilterFor(clusterName),
	}
	output, err := ec2api.DescribeSubnetsWithContext(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("describing subnet, %w", err)
	}
	return output.Subnets, nil
}

func getPrivateSubnetIDs(ctx context.Context, ec2api *EC2, clusterName string) ([]string, error) {
	subnets, err := getSubnets(ctx, ec2api, clusterName)
	if err != nil {
		return nil, err
	}
	result := []string{}
	for _, subnet := range subnets {
		if aws.BoolValue(subnet.MapPublicIpOnLaunch) {
			continue
		}
		result = append(result, *subnet.SubnetId)
	}
	return result, nil
}

func getPublicSubnetIDs(ctx context.Context, ec2api *EC2, clusterName string) ([]string, error) {
	subnets, err := getSubnets(ctx, ec2api, clusterName)
	if err != nil {
		return nil, err
	}
	result := []string{}
	for _, subnet := range subnets {
		if aws.BoolValue(subnet.MapPublicIpOnLaunch) {
			result = append(result, *subnet.SubnetId)
		}
	}
	return result, nil
}
