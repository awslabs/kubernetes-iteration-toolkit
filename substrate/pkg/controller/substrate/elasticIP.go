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
	ec2api *EC2
}

func (e *elasticIP) resourceName() string {
	return "elastic-ip"
}

func (e *elasticIP) Provision(ctx context.Context, substrate *v1alpha1.Substrate) error {
	identifier := ""
	output, err := e.ec2api.AllocateAddressWithContext(ctx, &ec2.AllocateAddressInput{
		TagSpecifications: generateEC2Tags(e.resourceName(), identifier),
	})
	if err != nil {
		return err
	}
	zap.S().Infof("Successfully created elastic-ip %v for cluster %v", *output.AllocationId, identifier)
	return nil
}

func (e *elasticIP) Deprovision(ctx context.Context, substrate *v1alpha1.Substrate) error {
	identifier := ""
	eip, err := getElasticIP(ctx, e.ec2api, identifier)
	if err != nil {
		return fmt.Errorf("getting elastic IP, %w", err)
	}
	if eip == nil || aws.StringValue(eip.AllocationId) == "" {
		return nil
	}
	if _, err := e.ec2api.ReleaseAddressWithContext(ctx, &ec2.ReleaseAddressInput{
		AllocationId: eip.AllocationId,
	}); err != nil {
		return fmt.Errorf("failed to release elastic IP, %w", err)
	}
	zap.S().Infof("Successfully deleted elastic-ip %v for cluster %v", *eip.AllocationId, identifier)
	return nil
}

func getElasticIP(ctx context.Context, ec2api *EC2, identifier string) (*ec2.Address, error) {
	output, err := ec2api.DescribeAddressesWithContext(ctx, &ec2.DescribeAddressesInput{
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
