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
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/awsprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/controllers"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/errors"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/status"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	desiredSecurityGroupCounts = 2
	masterInstancesGroupName   = "Security Group for master nodes"
	etcdInstancesGroupName     = "Security Group for etcd nodes"
)

type securityGroup struct {
	ec2api *awsprovider.EC2
}

// NewSecurityGroupController returns a controller for managing security-group in AWS
func NewSecurityGroupController(ec2api *awsprovider.EC2) *securityGroup {
	return &securityGroup{ec2api: ec2api}
}

// Name returns the name of the controller
func (s *securityGroup) Name() string {
	return "security-group"
}

// For returns the resource this controller is for.
func (s *securityGroup) For() controllers.Object {
	return &v1alpha1.SecurityGroup{}
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (s *securityGroup) Reconcile(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	sgObj := object.(*v1alpha1.SecurityGroup)
	// 1. Get the VPC ID for the control plane status
	vpc, err := getVPC(ctx, s.ec2api, sgObj.Spec.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("getting VPC %w", err)
	}
	if vpc == nil {
		return status.Waiting, fmt.Errorf("vpc does not exist %w", errors.WaitingForSubResources)
	} // 2. Get existing security groups
	existingGroups, err := s.getSecurityGroups(ctx, sgObj.Spec.ClusterName)
	if err != nil {
		return nil, err
	}
	existingGroupIDs := groupsCreated(existingGroups)
	groupID := existingGroupIDs[sgObj.Spec.GroupName]
	if _, ok := existingGroupIDs[sgObj.Spec.GroupName]; !ok {
		// Create security group
		output, err := s.createFor(ctx, sgObj.Spec.GroupName, sgObj.Spec.ClusterName, *vpc.VpcId)
		if err != nil {
			return nil, fmt.Errorf("creating group, %w", err)
		}
		groupID = *output.GroupId
		zap.S().Infof("Successfully created security group %v for cluster %v", sgObj.Spec.GroupName, sgObj.Spec.ClusterName)
	} else {
		zap.S().Debugf("Successfully discovered security groups %v for cluster %v", sgObj.Spec.GroupName, sgObj.Spec.ClusterName)
	}
	if !ingressRuleExists(existingGroups, sgObj.Spec.GroupName) {
		if err := s.addIngressRuleFor(ctx, groupID, sgObj.Spec.GroupName, sgObj.Spec.ClusterName, existingGroupIDs); err != nil {
			return nil, fmt.Errorf("adding ingress rule, %w", err)
		}
		zap.S().Infof("Successfully added ingress rules for security group %v for cluster %v",
			sgObj.Spec.GroupName, sgObj.Name)
	}
	switch sgObj.Spec.GroupName {
	case v1alpha1.GroupName(sgObj.Spec.ClusterName, v1alpha1.MasterInstances):
		sgObj.Status.MasterSecurityGroupID = groupID
	case v1alpha1.GroupName(sgObj.Spec.ClusterName, v1alpha1.MasterInstances):
		sgObj.Status.ETCDSecurityGroupID = groupID
	}
	return status.Created, nil
}

func groupsCreated(sgs []*ec2.SecurityGroup) map[string]string {
	groupNameID := map[string]string{}
	for _, sg := range sgs {
		groupNameID[*sg.GroupName] = *sg.GroupId
	}
	return groupNameID
}

func ingressRuleExists(groups []*ec2.SecurityGroup, groupName string) bool {
	for _, group := range groups {
		if aws.StringValue(group.GroupName) == groupName &&
			len(group.IpPermissions) != 0 {
			return true
		}
	}
	return false
}

func (s *securityGroup) createFor(ctx context.Context, groupName, clusterName, vpcID string) (*ec2.CreateSecurityGroupOutput, error) {
	result, err := s.ec2api.CreateSecurityGroupWithContext(ctx, &ec2.CreateSecurityGroupInput{
		Description:       aws.String(s.createDescriptionFor(groupName, clusterName)),
		GroupName:         aws.String(groupName),
		VpcId:             aws.String(vpcID),
		TagSpecifications: generateEC2Tags(s.Name(), clusterName),
	})
	if err != nil {
		return nil, fmt.Errorf("creating security group for master %w", err)
	}
	return result, nil
}

func (s *securityGroup) addIngressRuleFor(ctx context.Context, groupID, groupName, clusterName string, groups map[string]string) error {
	input, err := s.createIngressInputFor(groupID, groupName, clusterName, groups)
	if err != nil {
		return nil
	}
	if _, err := s.ec2api.AuthorizeSecurityGroupIngressWithContext(ctx, input); err != nil {
		return err
	}
	return nil
}

func (s *securityGroup) createDescriptionFor(groupName, clusterName string) string {
	switch groupName {
	case v1alpha1.GroupName(clusterName, v1alpha1.MasterInstances):
		return "Master Instances Security groups to allow HTTPS traffic"
	case v1alpha1.GroupName(clusterName, v1alpha1.ETCDInstances):
		return "ETCD Instances Security groups to allow etcd traffic"
	}
	zap.S().Infof("group name is %v generated name is %v", groupName, v1alpha1.GroupName(clusterName, v1alpha1.MasterInstances))
	return ""
}

func (s *securityGroup) createIngressInputFor(securitygroupID, groupName, controlPlaneName string, groups map[string]string) (*ec2.AuthorizeSecurityGroupIngressInput, error) {
	switch groupName {
	case v1alpha1.GroupName(controlPlaneName, v1alpha1.MasterInstances):
		return &ec2.AuthorizeSecurityGroupIngressInput{
			GroupId: aws.String(securitygroupID),
			IpPermissions: []*ec2.IpPermission{{
				FromPort: aws.Int64(443),
				ToPort:   aws.Int64(443),
				IpRanges: []*ec2.IpRange{{
					CidrIp: aws.String("0.0.0.0/0"),
				}},
				IpProtocol: aws.String("tcp"),
			}},
		}, nil
	case v1alpha1.GroupName(controlPlaneName, v1alpha1.ETCDInstances):
		masterGroupID, ok := groups[v1alpha1.GroupName(controlPlaneName, v1alpha1.MasterInstances)]
		if !ok {
			return nil, fmt.Errorf("master security group ID not found, %w", errors.WaitingForSubResources)
		}
		return &ec2.AuthorizeSecurityGroupIngressInput{
			GroupId: aws.String(securitygroupID),
			IpPermissions: []*ec2.IpPermission{{
				FromPort:   aws.Int64(2379),
				ToPort:     aws.Int64(2380),
				IpProtocol: aws.String("tcp"),
				UserIdGroupPairs: []*ec2.UserIdGroupPair{{
					GroupId: aws.String(securitygroupID),
				}},
			}, {
				FromPort:   aws.Int64(2379),
				ToPort:     aws.Int64(2379),
				IpProtocol: aws.String("tcp"),
				UserIdGroupPairs: []*ec2.UserIdGroupPair{{
					GroupId: aws.String(masterGroupID),
				}},
			}},
		}, nil
	}
	return nil, nil
}

// Finalize deletes the resource from AWS
func (s *securityGroup) Finalize(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	sgObj := object.(*v1alpha1.SecurityGroup)
	existingGroups, err := s.getSecurityGroups(ctx, sgObj.Spec.ClusterName)
	if err != nil {
		return nil, err
	}
	// get groupName to group ID mapping
	groups := groupsCreated(existingGroups)
	groupID := groups[sgObj.Spec.GroupName]
	if groupID == "" {
		zap.S().Errorf("When removing security group not found in AWS %s", sgObj.Spec.GroupName)
		return status.Terminated, nil
	}
	if _, err := s.ec2api.DeleteSecurityGroupWithContext(ctx, &ec2.DeleteSecurityGroupInput{
		GroupId: aws.String(groupID),
	}); err != nil {
		return nil, err
	}
	switch sgObj.Spec.GroupName {
	case v1alpha1.GroupName(sgObj.Spec.ClusterName, v1alpha1.MasterInstances):
		sgObj.Status.MasterSecurityGroupID = ""
	case v1alpha1.GroupName(sgObj.Spec.ClusterName, v1alpha1.MasterInstances):
		sgObj.Status.ETCDSecurityGroupID = ""
	}
	return status.Terminated, nil
}

func (s *securityGroup) getSecurityGroups(ctx context.Context, clusterName string) ([]*ec2.SecurityGroup, error) {
	return getSecurityGroups(ctx, s.ec2api, clusterName)
}

func getSecurityGroups(ctx context.Context, ec2api *awsprovider.EC2, clusterName string) ([]*ec2.SecurityGroup, error) {
	output, err := ec2api.DescribeSecurityGroups(
		&ec2.DescribeSecurityGroupsInput{
			Filters: ec2FilterFor(clusterName),
		},
	)
	if err != nil {
		return nil, err
	}
	if len(output.SecurityGroups) == 0 {
		return nil, nil
	}
	if len(output.SecurityGroups) > desiredSecurityGroupCounts {
		return nil, fmt.Errorf("expected to find %d security group but found %d",
			desiredSecurityGroupCounts, len(output.SecurityGroups))
	}
	return output.SecurityGroups, nil
}
