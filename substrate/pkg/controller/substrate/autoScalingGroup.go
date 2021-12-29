package substrate

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"go.uber.org/zap"
)

type autoScalingGroup struct {
	ec2api         *EC2
	autoscalingAPI *AutoScaling
}

func (a *autoScalingGroup) Provision(ctx context.Context, substrate *v1alpha1.Substrate) error {
	existingASG, err := a.getAutoScalingGroup(ctx, substrate.Name)
	if err != nil {
		return fmt.Errorf("getting autoscaling groups, %w", err)
	}
	// If doesn't match or doesn't exists
	if existingASG == nil {
		if err := a.createAutoScalingGroup(ctx, substrate); err != nil {
			return fmt.Errorf("creating autoscaling groups, %w", err)
		}
		zap.S().Infof("Successfully created autoscaling group %v", scalingGroupName(substrate.Name))
		return nil
	}
	zap.S().Debugf("Successfully discovered autoscaling group %v", scalingGroupName(substrate.Name))
	return nil
}

func (a *autoScalingGroup) Deprovision(ctx context.Context, substrate *v1alpha1.Substrate) error {
	existingASG, err := a.getAutoScalingGroup(ctx, substrate.Name)
	if err != nil {
		return fmt.Errorf("getting autoscaling groups, %w", err)
	}
	if existingASG != nil && aws.StringValue(existingASG.Status) != "Delete in progress" {
		if _, err := a.autoscalingAPI.DeleteAutoScalingGroupWithContext(ctx, &autoscaling.DeleteAutoScalingGroupInput{
			AutoScalingGroupName: existingASG.AutoScalingGroupName,
			ForceDelete:          aws.Bool(true),
		}); err != nil {
			return fmt.Errorf("deleting autoscaling group, %w", err)
		}
		zap.S().Infof("Successfully deleted auto-scaling-group %v", *existingASG.AutoScalingGroupName)
	}
	return nil
}

func (a *autoScalingGroup) createAutoScalingGroup(ctx context.Context, substrate *v1alpha1.Substrate) error {
	publicSubnets, err := publicSubnetIDs(ctx, a.ec2api, substrate.Name)
	if err != nil {
		return err
	}
	if len(publicSubnets) == 0 {
		return fmt.Errorf("public subnets not found")
	}
	input := &autoscaling.CreateAutoScalingGroupInput{
		AutoScalingGroupName: aws.String(scalingGroupName(substrate.Name)),
		DesiredCapacity:      aws.Int64(1),
		MaxSize:              aws.Int64(1),
		MinSize:              aws.Int64(1),
		LaunchTemplate: &autoscaling.LaunchTemplateSpecification{
			LaunchTemplateName: aws.String(launchTemplateName(substrate.Name)),
		},
		VPCZoneIdentifier: aws.String(strings.Join(publicSubnets, ",")),
		Tags:              generateAutoScalingTags(substrate.Name, scalingGroupName(substrate.Name)),
	}
	if _, err := a.autoscalingAPI.CreateAutoScalingGroup(input); err != nil {
		return fmt.Errorf("creating autoscaling group, %w", err)
	}
	return nil
}

func (a *autoScalingGroup) getAutoScalingGroup(ctx context.Context, identifier string) (*autoscaling.Group, error) {
	output, err := a.autoscalingAPI.DescribeAutoScalingGroupsWithContext(ctx, &autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: aws.StringSlice([]string{scalingGroupName(identifier)}),
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

func scalingGroupName(identifier string) string {
	return fmt.Sprintf("substrate-nodes-for-%s", identifier)
}
