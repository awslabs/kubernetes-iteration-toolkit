package substrate

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"knative.dev/pkg/logging"
)

type securityGroup struct {
	ec2Client *ec2.EC2
}

func (s *securityGroup) resourceName() string {
	return "security-group"
}

func (s *securityGroup) Create(ctx context.Context, substrate *v1alpha1.Substrate) error {
	if substrate.Status.VPCID == nil {
		return fmt.Errorf("vpc ID not found for %v", substrate.Name)
	}
	existingGroup, err := s.getSecurityGroups(ctx, substrate.Name)
	if err != nil {
		return err
	}
	if existingGroup == nil {
		output, err := s.createFor(ctx, substrate)
		if err != nil {
			return fmt.Errorf("creating group, %w", err)
		}
		substrate.Status.SecurityGroupID = output.GroupId
	} else {
		substrate.Status.SecurityGroupID = existingGroup.GroupId
	}
	// group already has permissions configured
	if existingGroup != nil && len(existingGroup.IpPermissions) != 0 {
		return nil
	}
	// add ingress rules
	if err := s.addIngressRuleFor(ctx, *substrate.Status.SecurityGroupID); err != nil {
		return fmt.Errorf("adding ingress rule, %w", err)
	}
	logging.FromContext(ctx).Infof("Successfully added ingress rules for security group %v", groupName(substrate.Name))
	return nil
}

// Finalize deletes the resource from AWS
func (s *securityGroup) Delete(ctx context.Context, substrate *v1alpha1.Substrate) error {
	existingGroup, err := s.getSecurityGroups(ctx, substrate.Name)
	if err != nil {
		return err
	}
	if existingGroup == nil {
		return nil
	}
	if _, err := s.ec2Client.DeleteSecurityGroupWithContext(ctx, &ec2.DeleteSecurityGroupInput{
		GroupId: existingGroup.GroupId,
	}); err != nil {
		return fmt.Errorf("deleting security group, %w", err)
	}
	substrate.Status.SecurityGroupID = nil
	return nil
}

func (s *securityGroup) createFor(ctx context.Context, substrate *v1alpha1.Substrate) (*ec2.CreateSecurityGroupOutput, error) {
	result, err := s.ec2Client.CreateSecurityGroupWithContext(ctx, &ec2.CreateSecurityGroupInput{
		Description:       aws.String(fmt.Sprintf("Substrate node to allow access to substrate cluster endpoint for %s", substrate.Name)),
		GroupName:         aws.String(groupName(substrate.Name)),
		VpcId:             aws.String(*substrate.Status.VPCID),
		TagSpecifications: generateEC2Tags(s.resourceName(), substrate.Name),
	})
	if err != nil {
		return nil, fmt.Errorf("creating security group for master %w", err)
	}
	return result, nil
}

func (s *securityGroup) addIngressRuleFor(ctx context.Context, groupID string) error {
	if _, err := s.ec2Client.AuthorizeSecurityGroupIngressWithContext(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: aws.String(groupID),
		IpPermissions: []*ec2.IpPermission{{
			FromPort:   aws.Int64(443),
			ToPort:     aws.Int64(443),
			IpProtocol: aws.String("tcp"),
			IpRanges:   []*ec2.IpRange{{CidrIp: aws.String("0.0.0.0/0")}},
		}},
	}); err != nil {
		return err
	}
	return nil
}

func (s *securityGroup) getSecurityGroups(ctx context.Context, identifier string) (*ec2.SecurityGroup, error) {
	output, err := s.ec2Client.DescribeSecurityGroups(
		&ec2.DescribeSecurityGroupsInput{
			Filters: ec2FilterFor(identifier),
		},
	)
	if err != nil {
		return nil, err
	}
	if len(output.SecurityGroups) == 0 {
		return nil, nil
	}
	if len(output.SecurityGroups) != 1 {
		return nil, fmt.Errorf("expected to find 1 security group but found %d", len(output.SecurityGroups))
	}
	return output.SecurityGroups[0], nil
}

func groupName(identifier string) string {
	return fmt.Sprintf("substrate-group-for-%s", identifier)
}
