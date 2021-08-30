package securitygroup

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/operator/pkg/awsprovider"
	"github.com/awslabs/kit/operator/pkg/controllers/master"
	"github.com/awslabs/kit/operator/pkg/kubeprovider"
	v1 "k8s.io/api/core/v1"
	"knative.dev/pkg/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Provider struct {
	ec2api     *awsprovider.EC2
	kubeClient *kubeprovider.Client
}

func New(ec2api *awsprovider.EC2, client *kubeprovider.Client) *Provider {
	return &Provider{ec2api: ec2api, kubeClient: client}
}

func (p *Provider) For(ctx context.Context, clusterName string) (string, error) {
	instanceID, err := p.controlPlaneInstancesFor(ctx, clusterName)
	if err != nil {
		return "", err
	}
	return p.getSecurityGroupFor(ctx, instanceID)
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

func (p *Provider) controlPlaneInstancesFor(ctx context.Context, clusterName string) (string, error) {
	nodes, err := p.nodesWithLabelFor(ctx, clusterName)
	if err != nil {
		return "", err
	}
	return parseInstanceID(nodes.Items[0].Spec.ProviderID)
}

func (p *Provider) nodesWithLabelFor(ctx context.Context, clusterName string) (*v1.NodeList, error) {
	nodes := &v1.NodeList{}
	if err := p.kubeClient.List(ctx, nodes, client.MatchingLabels(master.APIServerLabels(clusterName))); err != nil {
		return nil, fmt.Errorf("getting kube nodes for cluster %v, %w", clusterName, err)
	}
	return nodes, nil
}

func parseInstanceID(providerID string) (string, error) {
	if !strings.HasPrefix(providerID, "aws:///") {
		return "", fmt.Errorf("incorrect format for provider ID, %s", providerID)
	}
	values := strings.Split(strings.TrimPrefix(providerID, "aws:///"), "/")
	if len(values) != 2 {
		return "", fmt.Errorf("parsing provider ID, %s", providerID)
	}
	return values[1], nil
}
