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

package substrate

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kit/substrate/pkg/utils/discovery"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type autoScalingGroup struct {
	AutoScaling *autoscaling.AutoScaling
}

func (a *autoScalingGroup) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if len(substrate.Status.PublicSubnetIDs) == 0 {
		return reconcile.Result{Requeue: true}, nil
	}
	if _, err := a.AutoScaling.CreateAutoScalingGroupWithContext(ctx, &autoscaling.CreateAutoScalingGroupInput{
		AutoScalingGroupName: aws.String(autoScalingGroupName(substrate.Name)),
		DesiredCapacity:      aws.Int64(1), MaxSize: aws.Int64(1), MinSize: aws.Int64(1),
		LaunchTemplate:    &autoscaling.LaunchTemplateSpecification{LaunchTemplateName: aws.String(launchTemplateName(substrate.Name)), Version: aws.String("$Latest")},
		VPCZoneIdentifier: aws.String(strings.Join(substrate.Status.PublicSubnetIDs, ",")),
		Tags: []*autoscaling.Tag{{
			Key:               aws.String(discovery.OwnerTagKey),
			Value:             aws.String(substrate.Name),
			PropagateAtLaunch: aws.Bool(true),
		}, {
			Key:               aws.String("Name"),
			Value:             aws.String(autoScalingGroupName(substrate.Name)),
			PropagateAtLaunch: aws.Bool(true),
		}},
	}); err != nil {
		if strings.Contains(err.Error(), "Invalid IAM Instance Profile name") { // Instance Profile can take several seconds to propagate
			return reconcile.Result{Requeue: true}, nil
		}
		if err.(awserr.Error).Code() != autoscaling.ErrCodeAlreadyExistsFault {
			return reconcile.Result{}, fmt.Errorf("creating autoscaling group, %w", err)
		}
		logging.FromContext(ctx).Infof("Found auto scaling group %s", autoScalingGroupName(substrate.Name))
	} else {
		logging.FromContext(ctx).Infof("Created auto scaling group %s", autoScalingGroupName(substrate.Name))
	}
	return reconcile.Result{}, nil
}

func (a *autoScalingGroup) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	autoscalingGroupsOutput, err := a.AutoScaling.DescribeAutoScalingGroupsWithContext(ctx, &autoscaling.DescribeAutoScalingGroupsInput{
		AutoScalingGroupNames: aws.StringSlice([]string{autoScalingGroupName(substrate.Name)}),
	})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("describing auto scaling groups, %w", err)
	}
	for _, group := range autoscalingGroupsOutput.AutoScalingGroups {
		if aws.StringValue(group.Status) == "Delete in progress" {
			continue
		}
		if _, err := a.AutoScaling.DeleteAutoScalingGroupWithContext(ctx, &autoscaling.DeleteAutoScalingGroupInput{
			AutoScalingGroupName: group.AutoScalingGroupName,
			ForceDelete:          aws.Bool(true),
		}); err != nil {
			return reconcile.Result{}, fmt.Errorf("deleting autoscaling group, %w", err)
		}
		logging.FromContext(ctx).Infof("Deleted auto scaling group %s", autoScalingGroupName(substrate.Name))
	}
	return reconcile.Result{}, nil
}

func autoScalingGroupName(identifier string) string {
	return fmt.Sprintf("substrate-nodes-for-%s", identifier)
}
