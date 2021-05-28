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
	return &v1alpha1.ControlPlane{}
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (a *autoScalingGroup) Reconcile(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	controlPlane := object.(*v1alpha1.ControlPlane)
	asgNames := a.desiredAutoScalingGroups(controlPlane.Name)
	asgs, err := a.getAutoScalingGroup(ctx, asgNames)
	if err != nil {
		return nil, fmt.Errorf("getting autoscaling groups, %w", err)
	}
	for _, asgName := range asgNames {
		if a.existingASGMatchesDesrired(asgs, asgName) {
			zap.S().Debugf("Successfully discovered autoscaling group %v for cluster %v", asgName, controlPlane.Name)
			continue
		}
		if err := a.createAutoScalingGroup(ctx, asgName, controlPlane.Name); err != nil {
			return nil, err
		}
		zap.S().Infof("Successfully created autoscaling group %v for cluster %v", asgName, controlPlane.Name)
	}
	return status.Created, nil
}

// Finalize deletes the resource from AWS
func (a *autoScalingGroup) Finalize(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	controlPlane := object.(*v1alpha1.ControlPlane)
	asgNames := a.desiredAutoScalingGroups(controlPlane.Name)
	existingASGs, err := a.getAutoScalingGroup(ctx, asgNames)
	if err != nil {
		return nil, fmt.Errorf("getting auto-scaling-groups, %w", err)
	}
	for _, asg := range existingASGs {
		if asg.Status == aws.String("Delete in progress") {
			continue
		}
		if _, err := a.autoscalingAPI.DeleteAutoScalingGroupWithContext(ctx, &autoscaling.DeleteAutoScalingGroupInput{
			AutoScalingGroupName: asg.AutoScalingGroupName,
			ForceDelete:          aws.Bool(true),
		}); err != nil {
			return nil, err
		}
		zap.S().Infof("Successfully deleted auto-scaling-group %v", *asg.AutoScalingGroupName)
	}
	return status.Terminated, nil
}

func (a *autoScalingGroup) createAutoScalingGroup(ctx context.Context, name, clusterName string) error {
	zones := availabilityZonesForRegion(*a.ec2api.Config.Region)
	privateSubnets, err := getPrivateSubnetIDs(ctx, a.ec2api, clusterName)
	if err != nil {
		return err
	}
	subnets := strings.Join(privateSubnets, ",")
	launchTemplateName := a.desiredLaunchTemplateName(name, clusterName)
	if len(zones) == 0 || len(privateSubnets) == 0 || launchTemplateName == "" {
		return fmt.Errorf("failed to get zones/subnets/launch template")
	}
	input := &autoscaling.CreateAutoScalingGroupInput{
		AutoScalingGroupName: aws.String(name),
		// AvailabilityZones:    aws.StringSlice(zones),
		DesiredCapacity: aws.Int64(1),
		MaxSize:         aws.Int64(3),
		MinSize:         aws.Int64(1),
		LaunchTemplate: &autoscaling.LaunchTemplateSpecification{
			LaunchTemplateName: aws.String(launchTemplateName),
		},
		VPCZoneIdentifier: aws.String(subnets),
		Tags:              generateAutoScalingTags(name, clusterName),
	}
	if _, err := a.autoscalingAPI.CreateAutoScalingGroup(input); err != nil {
		return fmt.Errorf("creating autoscaling group, %w", err)
	}
	return nil
}

func (a *autoScalingGroup) desiredAutoScalingGroups(clusterName string) []string {
	return []string{
		fmt.Sprintf(MasterInstanceASGName, clusterName),
		fmt.Sprintf(ETCDInstanceASGName, clusterName),
	}
}

func (a *autoScalingGroup) desiredLaunchTemplateName(name, clusterName string) string {
	switch name {
	case fmt.Sprintf(MasterInstanceASGName, clusterName):
		return fmt.Sprintf(MasterInstanceLaunchTemplateName, clusterName)
	case fmt.Sprintf(ETCDInstanceASGName, clusterName):
		return fmt.Sprintf(ETCDInstanceLaunchTemplateName, clusterName)
	}
	return ""
}

func (a *autoScalingGroup) getAutoScalingGroup(ctx context.Context, groupNames []string) ([]*autoscaling.Group, error) {
	output, err := a.autoscalingAPI.DescribeAutoScalingGroupsWithContext(ctx, &autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: aws.StringSlice(groupNames),
	})
	if err != nil {
		return nil, fmt.Errorf("getting autoscaling group, %w", err)
	}
	if len(output.AutoScalingGroups) == 0 {
		return nil, nil
	}
	return output.AutoScalingGroups, nil
}

func (a *autoScalingGroup) existingASGMatchesDesrired(groups []*autoscaling.Group, asgName string) bool {
	for _, group := range groups {
		if *group.AutoScalingGroupName == asgName {
			return true
		}
	}
	return false
}
