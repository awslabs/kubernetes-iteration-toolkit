package securitygroup

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/operator/pkg/awsprovider"
	"github.com/awslabs/kit/operator/pkg/kubeprovider"
	cpinstances "github.com/awslabs/kit/operator/pkg/utils/instances"
	"knative.dev/pkg/ptr"
)

type Provider struct {
	ec2api    *awsprovider.EC2
	instances *cpinstances.Provider
}

func New(ec2api *awsprovider.EC2, client *kubeprovider.Client) *Provider {
	return &Provider{ec2api: ec2api, instances: cpinstances.New(client)}
}

func (p *Provider) For(ctx context.Context, clusterName string) (string, error) {
	instanceID, err := p.instances.ControlPlaneInstancesFor(ctx, clusterName)
	if err != nil {
		return "", fmt.Errorf("getting control plane instances for %v, %w", clusterName, err)
	}
	return p.getSecurityGroupFor(ctx, instanceID[0])
}

func (p *Provider) getSecurityGroupFor(ctx context.Context, instanceID string) (string, error) {
	output, err := p.ec2api.DescribeInstancesWithContext(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []*string{ptr.String(instanceID)},
	})
	if err != nil {
		return "", fmt.Errorf("describing ec2 instance id %s, %w", instanceID, err)
	}
	if len(output.Reservations) != 1 {
		return "", fmt.Errorf("unknown reservations count for %s", instanceID)
	}
	if len(output.Reservations[0].Instances) != 1 {
		return "", fmt.Errorf("missing desired instance id %s", instanceID)
	}
	if len(output.Reservations[0].Instances[0].SecurityGroups) != 1 {
		return "", fmt.Errorf("expected one security group for instance %s, found %d", instanceID,
			len(output.Reservations[0].Instances[0].SecurityGroups))
	}
	return *output.Reservations[0].Instances[0].SecurityGroups[0].GroupId, nil
}
