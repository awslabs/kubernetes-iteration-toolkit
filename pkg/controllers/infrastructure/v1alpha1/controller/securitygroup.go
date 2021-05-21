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
	"github.com/prateekgogia/kit/pkg/apis/infrastructure/v1alpha1"
	"github.com/prateekgogia/kit/pkg/awsprovider"
	"github.com/prateekgogia/kit/pkg/controllers"
	"github.com/prateekgogia/kit/pkg/kiterr"
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
	return &v1alpha1.ControlPlane{}
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (s *securityGroup) Reconcile(ctx context.Context, object controllers.Object) (reconcile.Result, error) {
	controlPlane := object.(*v1alpha1.ControlPlane)
	// 1. Get the VPC ID for the control plane
	zap.S().Infof("CAME IN Create Security groups")
	vpcID := controlPlane.Status.Infrastructure.VPCID
	if vpcID == "" {
		return WaitingForSubResource, fmt.Errorf("vpc does not exist %w", kiterr.WaitingForSubResources)
	}
	// 2. Get existing security groups
	existingGroups, err := s.getSecurityGroups(ctx, controlPlane.Name)
	if err != nil {
		return WaitingForSubResource, err
	}
	// 3. Map group names to the group ID
	groups := map[string]string{}
	for _, groupFound := range existingGroups {
		groups[aws.StringValue(groupFound.GroupName)] = *groupFound.GroupId
	}
	// 4. If group is not created, create the group
	if groups[masterInstancesGroupName] == "" {
		masterGroup, err := s.createGroupForMasterNodes(ctx, controlPlane.Name, vpcID)
		if err != nil {
			return ResourceFailedProgressing, err
		}
		groups[masterInstancesGroupName] = *masterGroup.GroupId
		zap.S().Infof("Successfully created security group %v for cluster %v", masterInstancesGroupName, controlPlane.Name)
	} else {
		zap.S().Debugf("Successfully discovered security group %v for cluster %v", masterInstancesGroupName, controlPlane.Name)
	}
	if groups[etcdInstancesGroupName] == "" {
		masterGroupID := groups[masterInstancesGroupName]
		etcdGroup, err := s.createGroupForETCDNodes(ctx, masterGroupID, controlPlane.Name, vpcID)
		if err != nil {
			return ResourceFailedProgressing, err
		}
		groups[etcdInstancesGroupName] = *etcdGroup.GroupId
		zap.S().Infof("Successfully created security group %v for cluster %v", etcdInstancesGroupName, controlPlane.Name)
	} else {
		zap.S().Debugf("Successfully discovered security group %v for cluster %v", etcdInstancesGroupName, controlPlane.Name)
	}
	// 5. Sync security group IDs with status
	controlPlane.Status.Infrastructure.SecurityGroupMasterNodesID = groups[masterInstancesGroupName]
	controlPlane.Status.Infrastructure.SecurityGroupETCDNodesID = groups[etcdInstancesGroupName]
	return ResourceCreated, nil
}

// Finalize deletes the resource from AWS
func (s *securityGroup) Finalize(ctx context.Context, object controllers.Object) (reconcile.Result, error) {
	controlPlane := object.(*v1alpha1.ControlPlane)
	existingGroups, err := s.getSecurityGroups(ctx, controlPlane.Name)
	if err != nil {
		return ResourceFailedProgressing, err
	}
	groups := map[string]string{}
	for _, groupFound := range existingGroups {
		groups[aws.StringValue(groupFound.GroupName)] = *groupFound.GroupId
	}
	// We have to delete etcd SG before master SG because master SG is added as a referenced in etcd SG
	for _, groupID := range []string{groups[etcdInstancesGroupName], groups[masterInstancesGroupName]} {
		if groupID == "" {
			continue
		}
		if _, err := s.ec2api.DeleteSecurityGroupWithContext(ctx, &ec2.DeleteSecurityGroupInput{
			GroupId: aws.String(groupID),
		}); err != nil {
			return ResourceFailedProgressing, err
		}
	}
	controlPlane.Status.Infrastructure.SecurityGroupMasterNodesID = ""
	controlPlane.Status.Infrastructure.SecurityGroupETCDNodesID = ""
	return ResourceTerminated, nil
}

func (s *securityGroup) createGroupForMasterNodes(ctx context.Context, clusterName, vpcID string) (*ec2.CreateSecurityGroupOutput, error) {
	result, err := s.ec2api.CreateSecurityGroupWithContext(ctx, &ec2.CreateSecurityGroupInput{
		Description:       aws.String("Master Instances Security groups to allow HTTPS traffic"),
		GroupName:         aws.String(masterInstancesGroupName),
		VpcId:             aws.String(vpcID),
		TagSpecifications: generateEC2Tags(s.Name(), clusterName),
	})
	if err != nil {
		return nil, fmt.Errorf("creating security group for master %w", err)
	}
	input := &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: result.GroupId,
		IpPermissions: []*ec2.IpPermission{
			&ec2.IpPermission{
				FromPort: aws.Int64(443),
				ToPort:   aws.Int64(443),
				IpRanges: []*ec2.IpRange{&ec2.IpRange{
					CidrIp: aws.String("0.0.0.0/0"),
				}},
				IpProtocol: aws.String("tcp"),
			},
		},
	}
	if _, err := s.ec2api.AuthorizeSecurityGroupIngressWithContext(ctx, input); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *securityGroup) createGroupForETCDNodes(ctx context.Context, masterSG, clusterName, vpcID string) (*ec2.CreateSecurityGroupOutput, error) {
	result, err := s.ec2api.CreateSecurityGroupWithContext(ctx, &ec2.CreateSecurityGroupInput{
		Description:       aws.String("ETCD Instances Security groups to allow etcd traffic"),
		GroupName:         aws.String(etcdInstancesGroupName),
		VpcId:             aws.String(vpcID),
		TagSpecifications: generateEC2Tags(s.Name(), clusterName),
	})
	if err != nil {
		return nil, fmt.Errorf("creating security group for etcd %w", err)
	}
	input := &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: result.GroupId,
		IpPermissions: []*ec2.IpPermission{
			&ec2.IpPermission{
				FromPort:   aws.Int64(2379),
				ToPort:     aws.Int64(2380),
				IpProtocol: aws.String("tcp"),
				UserIdGroupPairs: []*ec2.UserIdGroupPair{
					&ec2.UserIdGroupPair{
						GroupId: result.GroupId,
					},
				},
			},
			&ec2.IpPermission{
				FromPort:   aws.Int64(2379),
				ToPort:     aws.Int64(2379),
				IpProtocol: aws.String("tcp"),
				UserIdGroupPairs: []*ec2.UserIdGroupPair{
					&ec2.UserIdGroupPair{
						GroupId: aws.String(masterSG),
					},
				},
			},
		},
	}
	if _, err := s.ec2api.AuthorizeSecurityGroupIngressWithContext(ctx, input); err != nil {
		return nil, err
	}
	return result, nil
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
