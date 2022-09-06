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
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/utils/discovery"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type SecurityGroup struct {
	EC2 *ec2.EC2
}

func (s *SecurityGroup) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	return s.CreateResource(ctx, substrate.Name, &substrate.Spec, &substrate.Status)
}

func (s *SecurityGroup) CreateResource(ctx context.Context, name string, spec *v1alpha1.SubstrateSpec, status *v1alpha1.SubstrateStatus) (reconcile.Result, error) {
	vpcID := status.Infrastructure.VPCID
	if vpcID == nil {
		return reconcile.Result{Requeue: true}, nil
	}
	securityGroupID, err := s.CreateAndAuthorizeSecurityGroupIngress(ctx, name, aws.StringValue(vpcID))
	status.Infrastructure.SecurityGroupID = securityGroupID
	return reconcile.Result{}, err
}

func (s *SecurityGroup) CreateAndAuthorizeSecurityGroupIngress(ctx context.Context, name string, vpcID string) (*string, error) {
	securityGroup, err := s.ensure(ctx, name, vpcID)
	if err != nil {
		return nil, err
	}
	securityGroupID := securityGroup.GroupId
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
			return securityGroupID, fmt.Errorf("authorizing security group ingress, %w", err)
		}
		logging.FromContext(ctx).Debugf("Found ingress rules for security group %s", aws.StringValue(discovery.NameFrom(name)))
	} else {
		logging.FromContext(ctx).Infof("Created ingress rules for security group %s", aws.StringValue(discovery.NameFrom(name)))
	}
	return securityGroupID, nil
}

func (s *SecurityGroup) ensure(ctx context.Context, name string, vpcID string) (*ec2.SecurityGroup, error) {
	describeSecurityGroupsOutput, err := s.EC2.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{Filters: discovery.Filters(name, discovery.NameFrom(name))})
	if err != nil {
		return nil, fmt.Errorf("describing security groups, %w", err)
	}
	if len(describeSecurityGroupsOutput.SecurityGroups) > 0 {
		logging.FromContext(ctx).Debugf("Found security group %s", aws.StringValue(discovery.NameFrom(name)))
		return describeSecurityGroupsOutput.SecurityGroups[0], nil
	}
	createSecurityGroupOutput, err := s.EC2.CreateSecurityGroupWithContext(ctx, &ec2.CreateSecurityGroupInput{
		Description: aws.String(fmt.Sprintf("Substrate node to allow access to substrate cluster endpoint for %s", name)),
		GroupName:   discovery.NameFrom(name),
		VpcId:       aws.String(vpcID),
		TagSpecifications: []*ec2.TagSpecification{{
			ResourceType: aws.String(ec2.ResourceTypeSecurityGroup),
			Tags:         append(discovery.Tags(name, discovery.NameFrom(name)), &ec2.Tag{Key: aws.String("kubernetes.io/cluster/" + name), Value: aws.String("owned")}),
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("creating security group, %w", err)
	}
	logging.FromContext(ctx).Infof("Created security group %s", aws.StringValue(createSecurityGroupOutput.GroupId))
	return &ec2.SecurityGroup{GroupId: createSecurityGroupOutput.GroupId}, nil
}

func (s *SecurityGroup) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	return s.DeleteResource(ctx, substrate.Name)
}

func (s *SecurityGroup) DeleteResource(ctx context.Context, name string) (reconcile.Result, error) {
	describeSecurityGroupsOutput, err := s.EC2.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{Filters: discovery.Filters(name)})
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
