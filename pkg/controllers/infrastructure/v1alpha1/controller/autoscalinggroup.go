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

package controller

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/prateekgogia/kit/pkg/apis/infrastructure/v1alpha1"
	"github.com/prateekgogia/kit/pkg/awsprovider"
	"github.com/prateekgogia/kit/pkg/controllers"
	"github.com/prateekgogia/kit/pkg/errors"
	"github.com/prateekgogia/kit/pkg/status"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	MasterInstanceASGName = "master-instance-auto-scaling-group-cluster-%s"
	ETCDInstanceASGName   = "etcd-instance-auto-scaling-group-cluster-%s"
)

type autoScalingGroup struct {
	ec2api         *awsprovider.EC2
	autoscalingAPI *awsprovider.AutoScaling
}

// NewAutoScalingGroupController returns a controller for managing autoScalingGroup in AWS
func NewAutoScalingGroupController(ec2api *awsprovider.EC2, autoscalingAPI *awsprovider.AutoScaling) *autoScalingGroup {
	return &autoScalingGroup{ec2api: ec2api, autoscalingAPI: autoscalingAPI}
}

// Name returns the name of the controller
func (a *autoScalingGroup) Name() string {
	return "auto-scaling-group"
}

// For returns the resource this controller is for.
func (a *autoScalingGroup) For() controllers.Object {
	return &v1alpha1.AutoScalingGroup{}
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (a *autoScalingGroup) Reconcile(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	asgObj := object.(*v1alpha1.AutoScalingGroup)
	existingASG, err := a.getAutoScalingGroup(ctx, asgObj.Name)
	if err != nil {
		return nil, fmt.Errorf("getting autoscaling groups, %w", err)
	}
	// If doesn't match or doesn't exists
	if existingASG == nil {
		if err := a.createAutoScalingGroup(ctx, asgObj); err != nil {
			return nil, err
		}
		zap.S().Infof("Successfully created autoscaling group %v for cluster %v", asgObj.Name, asgObj.Spec.ClusterName)
	} else {
		zap.S().Debugf("Successfully discovered autoscaling group %v for cluster %v", asgObj.Name, asgObj.Spec.ClusterName)
	}
	return status.Created, nil
}

// Finalize deletes the resource from AWS
func (a *autoScalingGroup) Finalize(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	asgObj := object.(*v1alpha1.AutoScalingGroup)
	existingASG, err := a.getAutoScalingGroup(ctx, asgObj.Name)
	if err != nil {
		return nil, fmt.Errorf("getting autoscaling groups, %w", err)
	}
	if existingASG != nil &&
		aws.StringValue(existingASG.Status) != "Delete in progress" {
		if _, err := a.autoscalingAPI.DeleteAutoScalingGroupWithContext(ctx, &autoscaling.DeleteAutoScalingGroupInput{
			AutoScalingGroupName: existingASG.AutoScalingGroupName,
			ForceDelete:          aws.Bool(true),
		}); err != nil {
			return nil, err
		}
		zap.S().Infof("Successfully deleted auto-scaling-group %v", *existingASG.AutoScalingGroupName)
	}
	return status.Terminated, nil
}

func (a *autoScalingGroup) createAutoScalingGroup(ctx context.Context, asg *v1alpha1.AutoScalingGroup) error {
	// zones :=
	privateSubnets, err := getPrivateSubnetIDs(ctx, a.ec2api, asg.Spec.ClusterName)
	if err != nil {
		return err
	}
	if len(privateSubnets) == 0 {
		return fmt.Errorf("waiting for private subnets, %w", errors.WaitingForSubResources)
	}
	input := &autoscaling.CreateAutoScalingGroupInput{
		AutoScalingGroupName: aws.String(asg.Name),
		// AvailabilityZones:    aws.StringSlice(zones),
		DesiredCapacity: aws.Int64(int64(asg.Spec.InstanceCount)),
		MaxSize:         aws.Int64(4),
		MinSize:         aws.Int64(1),
		LaunchTemplate: &autoscaling.LaunchTemplateSpecification{
			LaunchTemplateName: aws.String(asg.Name),
		},
		VPCZoneIdentifier: aws.String(strings.Join(privateSubnets, ",")),
		Tags:              generateAutoScalingTags(asg.Name, asg.Spec.ClusterName),
	}
	if _, err := a.autoscalingAPI.CreateAutoScalingGroup(input); err != nil {
		return fmt.Errorf("creating autoscaling group, %w", err)
	}
	return nil
}

func (a *autoScalingGroup) getAutoScalingGroup(ctx context.Context, groupName string) (*autoscaling.Group, error) {
	output, err := a.autoscalingAPI.DescribeAutoScalingGroupsWithContext(ctx, &autoscaling.DescribeAutoScalingGroupsInput{
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
