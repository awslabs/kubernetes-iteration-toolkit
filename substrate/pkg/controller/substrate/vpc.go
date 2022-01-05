package substrate

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"knative.dev/pkg/logging"
)

type vpc struct {
	ec2Client *ec2.EC2
}

func (v *vpc) resourceName() string {
	return "vpc"
}

func (v *vpc) Create(ctx context.Context, substrate *v1alpha1.Substrate) error {
	vpc, err := v.getVPC(ctx, substrate.Name)
	if err != nil {
		return err
	}
	// vpc doesn't exist create VPC
	vpcID := ""
	if vpc == nil || *vpc.VpcId == "" {
		result, err := v.ec2Client.CreateVpc(&ec2.CreateVpcInput{
			CidrBlock:         aws.String(substrate.Spec.VPC.CIDR),
			TagSpecifications: generateEC2Tags(v.resourceName(), substrate.Name),
		})
		if err != nil {
			return fmt.Errorf("creating VPC, %w", err)
		}
		logging.FromContext(ctx).Infof("Created VPC %v ID %v", substrate.Name, *result.Vpc.VpcId)
		vpcID = *result.Vpc.VpcId
	} else {
		vpcID = *vpc.VpcId
	}
	substrate.Status.VPCID = aws.String(vpcID)
	return err
}

func (v *vpc) Delete(ctx context.Context, substrate *v1alpha1.Substrate) error {
	vpc, err := v.getVPC(ctx, substrate.Name)
	if err != nil {
		return err
	}
	// vpc doesn't exist, return
	if vpc == nil || *vpc.VpcId == "" {
		return nil
	}
	if _, err := v.ec2Client.DeleteVpcWithContext(ctx, &ec2.DeleteVpcInput{
		VpcId: vpc.VpcId,
	}); err != nil {
		return fmt.Errorf("deleting vpc, %w", err)
	}
	return nil
}

func (v *vpc) getVPC(ctx context.Context, identifier string) (*ec2.Vpc, error) {
	input := &ec2.DescribeVpcsInput{
		Filters: ec2FilterFor(identifier),
	}
	output, err := v.ec2Client.DescribeVpcsWithContext(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("describing VPC for %v, err: %w", identifier, err)
	}
	// Check if VPC doesn't exist return no error
	if len(output.Vpcs) == 0 {
		return nil, nil
	}
	if len(output.Vpcs) > 1 {
		return nil, fmt.Errorf("expected to find one VPC, but found %v", len(output.Vpcs))
	}
	return output.Vpcs[0], nil
}
