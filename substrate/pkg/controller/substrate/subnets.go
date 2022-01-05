package substrate

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"knative.dev/pkg/logging"
)

var (
	azs            = []string{"us-west-2a", "us-west-2b", "us-west-2c"}
	privateSubnets = []string{"10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"}
	publicSubnets  = []string{"10.0.100.0/24", "10.0.101.0/24", "10.0.102.0/24"}
)

type subnet struct {
	ec2Client *ec2.EC2
}

func (s *subnet) resourceName() string {
	return "subnet"
}

func (s *subnet) Create(ctx context.Context, substrate *v1alpha1.Substrate) error {
	if substrate.Status.VPCID == nil {
		return fmt.Errorf("vpc ID not found for %v", substrate.Name)
	}
	subnets, err := s.getSubnets(ctx, substrate.Name)
	if err != nil {
		return err
	}
	for i, az := range azs {
		subnetID := ""
		// check is private subnet exists in this AZ
		subnet, ok := subnetExists(subnets, fmt.Sprintf("%s-private-%d", substrate.Name, i), az)
		if !ok {
			// create private subnet in this AZ
			subnetID, err = s.create(ctx, substrate, az, privateSubnets[i], fmt.Sprintf("%s-private-%d", substrate.Name, i))
			if err != nil {
				return fmt.Errorf("provisioning private subnet, %w", err)
			}
		} else {
			subnetID = *subnet.SubnetId
		}
		substrate.Status.PrivateSubnetIDs = append(substrate.Status.PrivateSubnetIDs, subnetID)

		// check is public subnet exists in this AZ
		subnet, ok = subnetExists(subnets, fmt.Sprintf("%s-public-%d", substrate.Name, i), az)
		if !ok {
			// create public subnet in this AZ
			subnetID, err = s.create(ctx, substrate, az, publicSubnets[i], fmt.Sprintf("%s-public-%d", substrate.Name, i))
			if err != nil {
				return fmt.Errorf("provisioning public subnet, %w", err)
			}
			if err := s.markSubnetPublic(ctx, subnetID); err != nil {
				return err
			}
		} else {
			subnetID = *subnet.SubnetId
		}
		substrate.Status.PublicSubnetIDs = append(substrate.Status.PublicSubnetIDs, subnetID)
	}
	return nil
}

func subnetExists(existingSubnets []*ec2.Subnet, name, az string) (*ec2.Subnet, bool) {
	for _, existingSubnet := range existingSubnets {
		if strings.EqualFold(aws.StringValue(existingSubnet.AvailabilityZone), az) &&
			strings.EqualFold(subnetName(existingSubnet), name) {
			return existingSubnet, true
		}
	}
	return nil, false
}

func subnetName(subnet *ec2.Subnet) string {
	for _, tag := range subnet.Tags {
		if aws.StringValue(tag.Key) == "Name" {
			return aws.StringValue(tag.Value)
		}
	}
	return ""
}

func (s *subnet) Delete(ctx context.Context, substrate *v1alpha1.Substrate) error {
	subnets, err := s.getSubnets(ctx, substrate.Name)
	if err != nil {
		return err
	}
	if len(subnets) != 0 {
		if err := s.deleteSubnets(ctx, subnets); err != nil {
			return err
		}
		logging.FromContext(ctx).Infof("Successfully deleted Subnets for %v", substrate.Name)
	}
	substrate.Status.PrivateSubnetIDs = nil
	substrate.Status.PublicSubnetIDs = nil
	return nil
}

func (s *subnet) create(ctx context.Context, substrate *v1alpha1.Substrate, az, subnetCIDR, subnetName string) (string, error) {
	output, err := s.ec2Client.CreateSubnetWithContext(ctx, &ec2.CreateSubnetInput{
		AvailabilityZone:  aws.String(az),
		CidrBlock:         aws.String(subnetCIDR),
		VpcId:             aws.String(*substrate.Status.VPCID),
		TagSpecifications: generateEC2TagsWithName(s.resourceName(), substrate.Name, subnetName),
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
	if _, err := s.ec2Client.ModifySubnetAttribute(attributeInput); err != nil {
		return fmt.Errorf("modifying subnet attributes, %w", err)
	}
	return nil
}

func (s *subnet) deleteSubnets(ctx context.Context, subnets []*ec2.Subnet) error {
	for _, subnet := range subnets {
		if _, err := s.ec2Client.DeleteSubnetWithContext(ctx, &ec2.DeleteSubnetInput{
			SubnetId: subnet.SubnetId,
		}); err != nil {
			return fmt.Errorf("deleting subnet, %w", err)
		}
	}
	return nil
}

func (s *subnet) getSubnets(ctx context.Context, identifier string) ([]*ec2.Subnet, error) {
	input := &ec2.DescribeSubnetsInput{
		Filters: ec2FilterFor(identifier),
	}
	output, err := s.ec2Client.DescribeSubnetsWithContext(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("describing subnet, %w", err)
	}
	return output.Subnets, nil
}

// UNUSED
// func (s *subnet) privateSubnetIDs(ctx context.Context, identifier string) ([]string, error) {
// 	subnets, err := s.getSubnets(ctx, identifier)
// 	if err != nil {
// 		return nil, err
// 	}
// 	result := []string{}
// 	for _, subnet := range subnets {
// 		if aws.BoolValue(subnet.MapPublicIpOnLaunch) {
// 			continue
// 		}
// 		result = append(result, *subnet.SubnetId)
// 	}
// 	return result, nil
// }

func (s *subnet) publicSubnetIDs(ctx context.Context, identifier string) ([]string, error) {
	subnets, err := s.getSubnets(ctx, identifier)
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
