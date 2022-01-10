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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kit/substrate/pkg/utils/discovery"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type securityGroup struct {
	EC2 *ec2.EC2
}

func (s *securityGroup) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if substrate.Status.VPCID == nil {
		return reconcile.Result{Requeue: true}, nil
	}
	securityGroup, err := s.ensure(ctx, substrate)
	if err != nil {
		return reconcile.Result{}, err
	}
	substrate.Status.SecurityGroupID = securityGroup.GroupId
	if _, err := s.EC2.AuthorizeSecurityGroupIngressWithContext(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: securityGroup.GroupId,
		IpPermissions: []*ec2.IpPermission{{
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int64(443),
			ToPort:     aws.Int64(443),
			IpRanges:   []*ec2.IpRange{{CidrIp: aws.String("0.0.0.0/0")}},
		}},
	}); err != nil {
		if err.(awserr.Error).Code() != "InvalidPermission.Duplicate" {
			return reconcile.Result{}, fmt.Errorf("authorizing security group ingress, %w", err)
		}
		logging.FromContext(ctx).Infof("Found ingress rules for security group %s", securityGroupName(substrate.Name))
	} else {
		logging.FromContext(ctx).Infof("Created ingress rules for security group %s", securityGroupName(substrate.Name))
	}
	return reconcile.Result{}, nil
}

func (s *securityGroup) ensure(ctx context.Context, substrate *v1alpha1.Substrate) (*ec2.SecurityGroup, error) {
	describeSecurityGroupsOutput, err := s.EC2.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{Filters: discovery.Filters(substrate.Name, securityGroupName(substrate.Name))})
	if err != nil {
		return nil, fmt.Errorf("describing security groups, %w", err)
	}
	if len(describeSecurityGroupsOutput.SecurityGroups) > 0 {
		logging.FromContext(ctx).Infof("Found security group %s", securityGroupName(substrate.Name))
		return describeSecurityGroupsOutput.SecurityGroups[0], nil
	}
	createSecurityGroupOutput, err := s.EC2.CreateSecurityGroupWithContext(ctx, &ec2.CreateSecurityGroupInput{
		Description:       aws.String(fmt.Sprintf("Substrate node to allow access to substrate cluster endpoint for %s", substrate.Name)),
		GroupName:         aws.String(securityGroupName(substrate.Name)),
		VpcId:             substrate.Status.VPCID,
		TagSpecifications: discovery.Tags(ec2.ResourceTypeSecurityGroup, substrate.Name, securityGroupName(substrate.Name)),
	})
	if err != nil {
		return nil, fmt.Errorf("creating security group, %w", err)
	}
	logging.FromContext(ctx).Infof("Created security group %s", aws.StringValue(createSecurityGroupOutput.GroupId))
	return &ec2.SecurityGroup{GroupId: createSecurityGroupOutput.GroupId}, nil
}

// Finalize deletes the resource from AWS
func (s *securityGroup) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	describeSecurityGroupsOutput, err := s.EC2.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{Filters: discovery.Filters(substrate.Name, securityGroupName(substrate.Name))})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("describing security groups, %w", err)
	}
	for _, securityGroup := range describeSecurityGroupsOutput.SecurityGroups {
		if _, err := s.EC2.DeleteSecurityGroupWithContext(ctx, &ec2.DeleteSecurityGroupInput{GroupId: securityGroup.GroupId}); err != nil {
			if err.(awserr.Error).Code() == "DependencyViolation" {
				return reconcile.Result{Requeue: true}, nil
			}
			return reconcile.Result{}, fmt.Errorf("deleting security group, %w", err)
		}
		logging.FromContext(ctx).Infof("Deleted security group %s", aws.StringValue(securityGroup.GroupId))
	}
	return reconcile.Result{}, nil
}

func securityGroupName(identifier string) string {
	return fmt.Sprintf("substrate-group-for-%s", identifier)
}
