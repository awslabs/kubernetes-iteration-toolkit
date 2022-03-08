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

package cluster

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kit/substrate/pkg/utils/discovery"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type Instance struct {
	EC2 *ec2.EC2
}

func (i *Instance) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if len(substrate.Status.Infrastructure.PublicSubnetIDs) == 0 || substrate.Status.Cluster.LaunchTemplateVersion == nil {
		return reconcile.Result{Requeue: true}, nil
	}
	instancesOutput, err := i.EC2.DescribeInstancesWithContext(ctx, &ec2.DescribeInstancesInput{Filters: discovery.Filters(substrate)})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("describing instances, %w", err)
	}
	for _, reservation := range instancesOutput.Reservations {
		for _, instance := range reservation.Instances {
			if aws.StringValue(instance.State.Name) == ec2.InstanceStateNameRunning || aws.StringValue(instance.State.Name) == ec2.InstanceStateNamePending {
				for _, tag := range instance.Tags {
					if aws.StringValue(tag.Key) == "aws:ec2launchtemplate:version" && aws.StringValue(tag.Value) == aws.StringValue(substrate.Status.Cluster.LaunchTemplateVersion) {
						logging.FromContext(ctx).Infof("Found instance %s", aws.StringValue(instance.InstanceId))
						return reconcile.Result{}, nil
					}
				}
			}
		}
	}
	overrides := []*ec2.FleetLaunchTemplateOverridesRequest{}
	for _, subnet := range substrate.Status.Infrastructure.PublicSubnetIDs {
		overrides = append(overrides, &ec2.FleetLaunchTemplateOverridesRequest{SubnetId: aws.String(subnet)})
	}
	createFleetOutput, err := i.EC2.CreateFleetWithContext(ctx, &ec2.CreateFleetInput{
		Type: aws.String(ec2.FleetTypeInstant),
		LaunchTemplateConfigs: []*ec2.FleetLaunchTemplateConfigRequest{{
			Overrides: overrides,
			LaunchTemplateSpecification: &ec2.FleetLaunchTemplateSpecificationRequest{
				LaunchTemplateName: discovery.Name(substrate),
				Version:            substrate.Status.Cluster.LaunchTemplateVersion,
			}},
		},
		TargetCapacitySpecification: &ec2.TargetCapacitySpecificationRequest{
			DefaultTargetCapacityType: aws.String(ec2.DefaultTargetCapacityTypeOnDemand),
			TotalTargetCapacity:       aws.Int64(1),
		},
		TagSpecifications: []*ec2.TagSpecification{{
			ResourceType: aws.String(ec2.ResourceTypeInstance),
			Tags:         discovery.Tags(substrate, discovery.Name(substrate)),
		}},
		OnDemandOptions: &ec2.OnDemandOptionsRequest{AllocationStrategy: aws.String(ec2.FleetOnDemandAllocationStrategyLowestPrice)},
	})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("creating fleet, %w", err)
	}
	for _, err := range createFleetOutput.Errors {
		if strings.Contains(aws.StringValue(err.ErrorMessage), "Invalid IAM Instance Profile name") {
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, fmt.Errorf("creating fleet %v", aws.StringValue(err.ErrorMessage))
	}
	logging.FromContext(ctx).Infof("Created instance %s", aws.StringValue(createFleetOutput.Instances[0].InstanceIds[0]))

	if err := i.delete(ctx, substrate, func(instance *ec2.Instance) bool {
		if aws.StringValue(instance.InstanceId) == aws.StringValue(createFleetOutput.Instances[0].InstanceIds[0]) {
			return false
		}
		return aws.StringValue(instance.State.Name) == ec2.InstanceStateNameRunning ||
			aws.StringValue(instance.State.Name) == ec2.InstanceStateNamePending
	}); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (i *Instance) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	return reconcile.Result{}, i.delete(ctx, substrate, func(instance *ec2.Instance) bool {
		return aws.StringValue(instance.State.Name) != ec2.InstanceStateNameShuttingDown &&
			aws.StringValue(instance.State.Name) != ec2.InstanceStateNameTerminated
	})
}

func (i *Instance) delete(ctx context.Context, substrate *v1alpha1.Substrate, predicate func(*ec2.Instance) bool) error {
	instancesOutput, err := i.EC2.DescribeInstancesWithContext(ctx, &ec2.DescribeInstancesInput{Filters: discovery.Filters(substrate)})
	if err != nil {
		return fmt.Errorf("describing instances, %w", err)
	}
	instances := []*string{}
	for _, reservation := range instancesOutput.Reservations {
		for _, instance := range reservation.Instances {
			if predicate(instance) {
				instances = append(instances, instance.InstanceId)

			}
		}
	}
	if len(instances) == 0 {
		return nil
	}
	if _, err := i.EC2.TerminateInstancesWithContext(ctx, &ec2.TerminateInstancesInput{InstanceIds: instances}); err != nil {
		return fmt.Errorf("terminating instances, %w", err)
	}
	logging.FromContext(ctx).Infof("Deleted instances %v", aws.StringValueSlice(instances))
	return nil
}
