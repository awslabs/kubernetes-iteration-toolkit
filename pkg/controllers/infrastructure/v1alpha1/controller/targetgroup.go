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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/awsprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/controllers"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/errors"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/status"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type TargetGroup struct {
	ec2api *awsprovider.EC2
	elbv2  *awsprovider.ELBV2
}

// NewTargetGroupController returns a controller for managing TargetGroup in AWS
func NewTargetGroupController(ec2api *awsprovider.EC2, elbv2 *awsprovider.ELBV2) *TargetGroup {
	return &TargetGroup{ec2api: ec2api, elbv2: elbv2}
}

// Name returns the name of the controller
func (t *TargetGroup) Name() string {
	return "targetgroup"
}

// For returns the resource this controller is for.
func (t *TargetGroup) For() controllers.Object {
	return &v1alpha1.TargetGroup{}
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (t *TargetGroup) Reconcile(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	tgObj := object.(*v1alpha1.TargetGroup)
	vpc, err := getVPC(ctx, t.ec2api, tgObj.Spec.ClusterName)
	if err != nil {
		return status.Waiting, fmt.Errorf("getting VPC %w", err)
	}
	if vpc == nil {
		return status.Waiting, fmt.Errorf("vpc does not exist %w", errors.WaitingForSubResources)
	}
	_, err = t.getTargetGroup(ctx, tgObj.Name)
	if err != nil && errors.IsTargetGroupNotExists(err) {
		_, err := t.createTargetGroup(ctx, tgObj, *vpc.VpcId)
		if err != nil {
			return nil, err
		}
		zap.S().Infof("Successfully created target group %v for cluster %v", tgObj.Name, tgObj.Spec.ClusterName)
	} else if err != nil {
		return nil, fmt.Errorf("getting target group, %w", err)
	} else {
		zap.S().Debugf("Successfully discovered target group %v for cluster %v", tgObj.Name, tgObj.Spec.ClusterName)
	}
	return status.Created, nil

}

// Finalize deletes the resource from AWS
func (t *TargetGroup) Finalize(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	tgObj := object.(*v1alpha1.TargetGroup)
	existingGroup, err := t.getTargetGroup(ctx, tgObj.Name)
	if err != nil {
		return nil, fmt.Errorf("getting  target group, %w", err)
	}
	if existingGroup != nil {
		if _, err := t.elbv2.DeleteTargetGroupWithContext(ctx, &elbv2.DeleteTargetGroupInput{
			TargetGroupArn: existingGroup.TargetGroupArn,
		}); err != nil {
			return nil, err
		}
		zap.S().Infof("Successfully deleted target group %v", *existingGroup.TargetGroupName)
	}
	return status.Terminated, nil
}

func (t *TargetGroup) createTargetGroup(ctx context.Context, tg *v1alpha1.TargetGroup, vpcID string) (*elbv2.TargetGroup, error) {
	output, err := t.elbv2.CreateTargetGroupWithContext(ctx, &elbv2.CreateTargetGroupInput{
		Name:     aws.String(tg.Name),
		Protocol: aws.String("TCP"),
		Port:     aws.Int64(tg.Spec.Port),
		VpcId:    aws.String(vpcID),
	})
	if err != nil {
		return nil, fmt.Errorf("creating target group %w", err)
	}
	return output.TargetGroups[0], nil
}

func (t *TargetGroup) getTargetGroup(ctx context.Context, name string) (*elbv2.TargetGroup, error) {
	return getTargetGroup(ctx, t.elbv2, name)
}

func getTargetGroup(ctx context.Context, elbv2API *awsprovider.ELBV2, name string) (*elbv2.TargetGroup, error) {
	output, err := elbv2API.DescribeTargetGroupsWithContext(ctx, &elbv2.DescribeTargetGroupsInput{
		Names: []*string{
			aws.String(name),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("getting target group %w", err)
	}
	if len(output.TargetGroups) > 1 {
		return nil, fmt.Errorf("expected target group count one found count %d", len(output.TargetGroups))
	}
	return output.TargetGroups[0], nil
}
