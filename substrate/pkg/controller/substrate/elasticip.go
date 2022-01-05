package substrate

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"go.uber.org/zap"
)

type elasticIP struct {
	ec2Client *ec2.EC2
}

func (e *elasticIP) resourceName() string {
	return "elastic-ip"
}

func (e *elasticIP) Create(ctx context.Context, substrate *v1alpha1.Substrate) error {
	eip, err := e.get(ctx, substrate.Name)
	if err != nil {
		return fmt.Errorf("getting elastic IP, %w", err)
	}
	if eip == nil || aws.StringValue(eip.AllocationId) == "" {
		output, err := e.ec2Client.AllocateAddressWithContext(ctx, &ec2.AllocateAddressInput{
			TagSpecifications: generateEC2Tags(e.resourceName(), substrate.Name),
		})
		if err != nil {
			return fmt.Errorf("allocating elastic IP for %v, %w", substrate.Name, err)
		}
		zap.S().Infof("Successfully created elastic-ip %v for cluster %v", *output.AllocationId, substrate.Name)
		substrate.Status.ElasticIPID = output.AllocationId
		return nil
	}
	substrate.Status.ElasticIPID = eip.AllocationId
	return nil
}

func (e *elasticIP) Delete(ctx context.Context, substrate *v1alpha1.Substrate) error {
	eip, err := e.get(ctx, substrate.Name)
	if err != nil {
		return fmt.Errorf("getting elastic IP, %w", err)
	}
	if eip == nil || aws.StringValue(eip.AllocationId) == "" {
		return nil
	}
	if _, err := e.ec2Client.ReleaseAddressWithContext(ctx, &ec2.ReleaseAddressInput{
		AllocationId: eip.AllocationId,
	}); err != nil {
		return fmt.Errorf("failed to release elastic IP, %w", err)
	}
	zap.S().Infof("Successfully deleted elastic-ip %v for cluster %v", *eip.AllocationId, substrate.Name)
	substrate.Status.ElasticIPID = nil
	return nil
}

func (e *elasticIP) get(ctx context.Context, identifier string) (*ec2.Address, error) {
	output, err := e.ec2Client.DescribeAddressesWithContext(ctx, &ec2.DescribeAddressesInput{
		Filters: ec2FilterFor(identifier),
	})
	if err != nil {
		return nil, fmt.Errorf("describing elastic-ip, %w", err)
	}
	if len(output.Addresses) == 0 {
		return nil, nil
	}
	if len(output.Addresses) > 1 {
		return nil, fmt.Errorf("expected to find one elastic-ip, but found %d", len(output.Addresses))
	}
	return output.Addresses[0], nil
}
