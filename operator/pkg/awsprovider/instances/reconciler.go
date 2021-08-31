/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package instances

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/operator/pkg/apis/dataplane/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/awsprovider"
	"github.com/awslabs/kit/operator/pkg/awsprovider/launchtemplate"
	"github.com/awslabs/kit/operator/pkg/errors"
	"github.com/awslabs/kit/operator/pkg/kubeprovider"
	cpinstances "github.com/awslabs/kit/operator/pkg/utils/instances"
	"go.uber.org/zap"

	"knative.dev/pkg/ptr"
)

type Controller struct {
	ec2api      *awsprovider.EC2
	autoscaling *awsprovider.AutoScaling
	instances   *cpinstances.Provider
}

// NewController returns a controller for managing LaunchTemplates in AWS
func NewController(ec2api *awsprovider.EC2, autoscaling *awsprovider.AutoScaling, client *kubeprovider.Client) *Controller {
	return &Controller{ec2api: ec2api, autoscaling: autoscaling, instances: cpinstances.New(client)}
}

func (c *Controller) Reconcile(ctx context.Context, dataplane *v1alpha1.DataPlane) error {
	asg, err := c.getAutoScalingGroup(ctx, AutoScalingGroupNameFor(dataplane.Spec.ClusterName))
	if err != nil {
		return fmt.Errorf("getting auto scaling group for %v, %w", dataplane.Spec.ClusterName, err)
	}
	if asg == nil {
		if err := c.createAutoScalingGroup(ctx, dataplane); err != nil {
			return fmt.Errorf("creating auto scaling group for %v, %w", dataplane.Spec.ClusterName, err)
		}
		return nil
	}
	if ptr.Int64Value(asg.DesiredCapacity) != int64(dataplane.Spec.NodeCount) {
		return c.setDesiredCapacity(ctx, dataplane)
	}
	return nil
}

func (c *Controller) Finalize(ctx context.Context, dataplane *v1alpha1.DataPlane) error {
	if _, err := c.autoscaling.DeleteAutoScalingGroupWithContext(ctx, &autoscaling.DeleteAutoScalingGroupInput{
		AutoScalingGroupName: ptr.String(AutoScalingGroupNameFor(dataplane.Spec.ClusterName)),
	}); err != nil {
		return fmt.Errorf("deleting auto scaling group, %w", err)
	}
	return nil
}

func (c *Controller) createAutoScalingGroup(ctx context.Context, dataplane *v1alpha1.DataPlane) error {
	// TODO improve subnet detection for these worker nodes
	privateSubnets, err := c.getPrivateSubnetsFor(ctx, dataplane.Spec.ClusterName)
	if err != nil {
		return err
	}
	if len(privateSubnets) == 0 {
		return fmt.Errorf("failed to find private subnets for dataplane")
	}
	if _, err := c.autoscaling.CreateAutoScalingGroupWithContext(ctx, &autoscaling.CreateAutoScalingGroupInput{
		AutoScalingGroupName: ptr.String(AutoScalingGroupNameFor(dataplane.Spec.ClusterName)),
		DesiredCapacity:      ptr.Int64(int64(dataplane.Spec.NodeCount)),
		MaxSize:              ptr.Int64(int64(1000)),
		MinSize:              ptr.Int64(int64(0)),
		LaunchTemplate: &autoscaling.LaunchTemplateSpecification{
			LaunchTemplateName: ptr.String(launchtemplate.TemplateName(dataplane.Spec.ClusterName)),
		},
		VPCZoneIdentifier: ptr.String(strings.Join(privateSubnets, ",")),
		Tags:              generateAutoScalingTags(dataplane.Spec.ClusterName),
	}); err != nil {
		return fmt.Errorf("creating autoscaling group, %w", err)
	}
	return nil
}

func (c *Controller) getAutoScalingGroup(ctx context.Context, groupName string) (*autoscaling.Group, error) {
	output, err := c.autoscaling.DescribeAutoScalingGroupsWithContext(ctx, &autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: aws.StringSlice([]string{groupName}),
	})
	if err != nil {
		return nil, fmt.Errorf("getting autoscaling group, %w", err)
	}
	if len(output.AutoScalingGroups) == 0 {
		return nil, nil
	}
	if len(output.AutoScalingGroups) > 1 {
		return nil, fmt.Errorf("expected asg count one found asgs %d", len(output.AutoScalingGroups))
	}
	return output.AutoScalingGroups[0], nil
}

func (c *Controller) setDesiredCapacity(ctx context.Context, dataplane *v1alpha1.DataPlane) error {
	if _, err := c.autoscaling.SetDesiredCapacityWithContext(ctx, &autoscaling.SetDesiredCapacityInput{
		AutoScalingGroupName: ptr.String(AutoScalingGroupNameFor(dataplane.Spec.ClusterName)),
		DesiredCapacity:      ptr.Int64(int64(dataplane.Spec.NodeCount)),
	}); err != nil {
		return fmt.Errorf("setting desired asg capacity, %w", err)
	}
	return nil
}

func (c *Controller) getPrivateSubnetsFor(ctx context.Context, clusterName string) ([]string, error) {
	instanceIDs, err := c.instances.ControlPlaneInstancesFor(ctx, clusterName)
	if err != nil {
		return nil, err
	}
	zap.S().Infof("instanceIDs from API server are %v", instanceIDs)
	// TODO hardcoded for now
	desiredCPInstanceCount := 3
	// We want to wait for all the control plane nodes to be running
	if len(instanceIDs) != desiredCPInstanceCount {
		return nil, fmt.Errorf("waiting for control plane instances %w", errors.WaitingForSubResources)
	}
	subnetIDs, err := c.getSubnetIDsFor(ctx, instanceIDs)
	if err != nil {
		return nil, fmt.Errorf("getting subnet for %s, %w", clusterName, err)
	}
	return c.filterPrivateSubnets(ctx, subnetIDs)
}

func (c *Controller) filterPrivateSubnets(ctx context.Context, ids []*string) ([]string, error) {
	output, err := c.ec2api.DescribeSubnetsWithContext(ctx, &ec2.DescribeSubnetsInput{
		SubnetIds: ids,
	})
	if err != nil {
		return nil, fmt.Errorf("describing subnet, %w", err)
	}
	result := []string{}
	for _, subnet := range output.Subnets {
		if ptr.BoolValue(subnet.MapPublicIpOnLaunch) {
			continue
		}
		result = append(result, *subnet.SubnetId)
	}
	return result, nil
}

func (c *Controller) getSubnetIDsFor(ctx context.Context, instanceIDs []string) ([]*string, error) {
	requestIds := []*string{}
	for _, instanceID := range instanceIDs {
		requestIds = append(requestIds, ptr.String(instanceID))
	}
	output, err := c.ec2api.DescribeInstancesWithContext(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: requestIds,
	})
	if err != nil {
		return nil, fmt.Errorf("describing ec2 instance ids, %w", err)
	}
	temp := map[*string]struct{}{}
	result := []*string{}
	for _, reservation := range output.Reservations {
		for _, instance := range reservation.Instances {
			if _, ok := temp[instance.SubnetId]; !ok {
				result = append(result, instance.SubnetId)
				temp[instance.SubnetId] = struct{}{}
			}
		}
	}
	return result, nil
}

func AutoScalingGroupNameFor(clusterName string) string {
	return fmt.Sprintf("kit-%s-cluster-dataplane", clusterName)
}

func generateAutoScalingTags(clusterName string) []*autoscaling.Tag {
	return []*autoscaling.Tag{{
		Key:               ptr.String(fmt.Sprintf("kubernetes.io/cluster/%s", clusterName)),
		Value:             ptr.String("owned"),
		PropagateAtLaunch: aws.Bool(true),
	}, {
		Key:               aws.String("Name"),
		Value:             aws.String("auto-scaling-group"),
		PropagateAtLaunch: aws.Bool(true),
	}}
}
