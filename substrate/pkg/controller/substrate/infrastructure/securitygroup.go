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

package infrastructure

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

type SecurityGroup struct {
	EC2 *ec2.EC2
}

func (s *SecurityGroup) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if substrate.Status.Infrastructure.VPCID == nil {
		return reconcile.Result{Requeue: true}, nil
	}
	securityGroup, err := s.ensure(ctx, substrate)
	if err != nil {
		return reconcile.Result{}, err
	}
	substrate.Status.Infrastructure.SecurityGroupID = securityGroup.GroupId
	if _, err := s.EC2.AuthorizeSecurityGroupIngressWithContext(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		GroupId: securityGroup.GroupId,
		IpPermissions: []*ec2.IpPermission{{
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int64(8443),
			ToPort:     aws.Int64(8443),
			IpRanges:   []*ec2.IpRange{{CidrIp: aws.String("0.0.0.0/0")}},
		}, {
			// Allow all traffic within this security group
			IpProtocol:       aws.String("-1"),
			FromPort:         aws.Int64(-1),
			ToPort:           aws.Int64(-1),
			UserIdGroupPairs: []*ec2.UserIdGroupPair{{GroupId: securityGroup.GroupId}},
		}},
	}); err != nil {
		if err.(awserr.Error).Code() != "InvalidPermission.Duplicate" {
			return reconcile.Result{}, fmt.Errorf("authorizing security group ingress, %w", err)
		}
		logging.FromContext(ctx).Debugf("Found ingress rules for security group %s", aws.StringValue(discovery.Name(substrate)))
	} else {
		logging.FromContext(ctx).Infof("Created ingress rules for security group %s", aws.StringValue(discovery.Name(substrate)))
	}
	return reconcile.Result{}, nil
}

func (s *SecurityGroup) ensure(ctx context.Context, substrate *v1alpha1.Substrate) (*ec2.SecurityGroup, error) {
	describeSecurityGroupsOutput, err := s.EC2.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{Filters: discovery.Filters(substrate, discovery.Name(substrate))})
	if err != nil {
		return nil, fmt.Errorf("describing security groups, %w", err)
	}
	if len(describeSecurityGroupsOutput.SecurityGroups) > 0 {
		logging.FromContext(ctx).Debugf("Found security group %s", aws.StringValue(discovery.Name(substrate)))
		return describeSecurityGroupsOutput.SecurityGroups[0], nil
	}
	createSecurityGroupOutput, err := s.EC2.CreateSecurityGroupWithContext(ctx, &ec2.CreateSecurityGroupInput{
		Description:       aws.String(fmt.Sprintf("Substrate node to allow access to substrate cluster endpoint for %s", substrate.Name)),
		GroupName:         discovery.Name(substrate),
		VpcId:             substrate.Status.Infrastructure.VPCID,
		TagSpecifications: discovery.Tags(substrate, ec2.ResourceTypeSecurityGroup, discovery.Name(substrate), &ec2.Tag{Key: aws.String("kubernetes.io/cluster/" + substrate.Name), Value: aws.String("owned")}),
	})
	if err != nil {
		return nil, fmt.Errorf("creating security group, %w", err)
	}
	logging.FromContext(ctx).Infof("Created security group %s", aws.StringValue(createSecurityGroupOutput.GroupId))
	return &ec2.SecurityGroup{GroupId: createSecurityGroupOutput.GroupId}, nil
}

func (s *SecurityGroup) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	describeSecurityGroupsOutput, err := s.EC2.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{Filters: discovery.Filters(substrate)})
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
