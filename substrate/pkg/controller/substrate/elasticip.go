package substrate

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"knative.dev/pkg/logging"
)

type elasticIPForNatGW struct {
	*elasticIP
}

type elasticIPForAPIServer struct {
	*elasticIP
}

type elasticIP struct {
	ec2Client *ec2.EC2
}

func (e *elasticIP) resourceName() string {
	return "elastic-ip"
}

func (e *elasticIPForNatGW) Create(ctx context.Context, substrate *v1alpha1.Substrate) (err error) {
	substrate.Status.ElasticIPIDForNAT, _, err = e.create(ctx, substrate.Name, fmt.Sprintf("natgwip-for-%s", substrate.Name))
	return
}

func (e *elasticIPForAPIServer) Create(ctx context.Context, substrate *v1alpha1.Substrate) (err error) {
	substrate.Status.ElasticIPIDForAPIServer, substrate.Status.ElasticIPForAPIServer, err = e.create(
		ctx, substrate.Name, fmt.Sprintf("apiserverip-for-%s", substrate.Name))
	return
}

func (e *elasticIP) create(ctx context.Context, substrateName, resourceName string) (*string, *string, error) {
	eips, err := e.get(ctx, substrateName)
	if err != nil {
		return nil, nil, fmt.Errorf("getting elastic IP, %w", err)
	}
	for _, eip := range eips {
		if isExpectedAddress(eip, resourceName) {
			return eip.AllocationId, eip.PublicIp, nil
		}
	}
	output, err := e.ec2Client.AllocateAddressWithContext(ctx, &ec2.AllocateAddressInput{
		TagSpecifications: generateEC2TagsWithName(e.resourceName(), substrateName, resourceName),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("allocating elastic IP for %v, %w", substrateName, err)
	}
	logging.FromContext(ctx).Infof("Successfully created elastic-ip %v for %v", *output.AllocationId, resourceName)
	return output.AllocationId, output.PublicIp, nil
}

func (e *elasticIP) Delete(ctx context.Context, substrate *v1alpha1.Substrate) error {
	eips, err := e.get(ctx, substrate.Name)
	if err != nil {
		return fmt.Errorf("getting elastic IP, %w", err)
	}
	for _, eip := range eips {
		if _, err := e.ec2Client.ReleaseAddressWithContext(ctx, &ec2.ReleaseAddressInput{
			AllocationId: eip.AllocationId,
		}); err != nil {
			return fmt.Errorf("failed to release elastic IP, %w", err)
		}
		logging.FromContext(ctx).Infof("Successfully deleted elastic-ip %v for %v", *eip.AllocationId, substrate.Name)
	}
	substrate.Status.ElasticIPIDForNAT = nil
	substrate.Status.ElasticIPIDForAPIServer = nil
	substrate.Status.ElasticIPForAPIServer = nil
	return nil
}

func (e *elasticIP) get(ctx context.Context, identifier string) ([]*ec2.Address, error) {
	output, err := e.ec2Client.DescribeAddressesWithContext(ctx, &ec2.DescribeAddressesInput{
		Filters: ec2FilterFor(identifier),
	})
	if err != nil {
		return nil, fmt.Errorf("describing elastic-ip, %w", err)
	}
	if len(output.Addresses) == 0 {
		return nil, nil
	}
	return output.Addresses, nil
}

func isExpectedAddress(address *ec2.Address, expectedName string) bool {
	for _, tag := range address.Tags {
		if aws.StringValue(tag.Key) == "Name" && aws.StringValue(tag.Value) == expectedName {
			return true
		}
	}
	return false
}
