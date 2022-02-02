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

package cluster

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kit/substrate/pkg/utils/discovery"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type InstanceProfile struct {
	IAM *iam.IAM
}

var AssumeRolePolicyDocument = aws.String(`{
	"Version": "2012-10-17",
	"Statement": [
		{
			"Effect": "Allow",
			"Action": "sts:AssumeRole",
			"Principal": {
				"Service": "ec2.amazonaws.com"
			}
		}
	]
}`)

var SubstrateNodePolicyDocument = aws.String(`{
	"Version": "2012-10-17",
	"Statement": [
		{
			"Effect": "Allow",
			"Action": [
				"s3:GetObject",
				"s3:ListBucket",
				"ec2:DescribeAddresses",
				"ec2:AssociateAddress",
				"ec2:CreateLaunchTemplate",
				"ec2:CreateFleet",
				"ec2:RunInstances",
				"ec2:CreateTags",
				"iam:PassRole",
				"ec2:TerminateInstances",
				"ec2:DescribeLaunchTemplates",
				"ec2:DescribeInstances",
				"ec2:DescribeSecurityGroups",
				"ec2:DescribeSubnets",
				"ec2:DescribeInstanceTypes",
				"ec2:DescribeInstanceTypeOfferings",
				"ec2:DescribeAvailabilityZones",
				"ssm:GetParameter"
			],
			"Resource": ["*"]
		}
	]
}`)

var ManagedPoliciesForSubstrateNode = []string{
	"arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore",
	"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
	"arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
}

var ManagedPoliciesForNodeProvisionedByKarpenter = []string{
	"arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore",
	"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
	"arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
	"arn:aws:iam::aws:policy/AmazonEKSClusterPolicy",
	"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
}

type roleInfo struct {
	objectName      *string
	policyDocument  *string
	managedPolicies []string
}

func (i *InstanceProfile) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	for _, desired := range desiredRolesFor(substrate) {
		result, err := i.create(ctx, desired.objectName, desired.policyDocument, desired.managedPolicies)
		if err != nil {
			return result, err
		}
	}
	return reconcile.Result{}, nil
}

func (i *InstanceProfile) create(ctx context.Context, resourceName, policyDocument *string, managedPolicies []string) (reconcile.Result, error) {
	// Role
	if _, err := i.IAM.CreateRole(&iam.CreateRoleInput{RoleName: resourceName, AssumeRolePolicyDocument: AssumeRolePolicyDocument}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeEntityAlreadyExistsException {
			return reconcile.Result{}, fmt.Errorf("creating role, %w", err)
		}
		logging.FromContext(ctx).Infof("Found role %s", aws.StringValue(resourceName))
	} else {
		logging.FromContext(ctx).Infof("Created role %s", aws.StringValue(resourceName))
	}
	// Policy
	if policyDocument != nil {
		if _, err := i.IAM.PutRolePolicyWithContext(ctx, &iam.PutRolePolicyInput{RoleName: resourceName, PolicyName: resourceName, PolicyDocument: policyDocument}); err != nil {
			return reconcile.Result{}, fmt.Errorf("adding policy to role, %w", err)
		} else {
			logging.FromContext(ctx).Infof("Created policy %s for %s", aws.StringValue(resourceName), aws.StringValue(resourceName))
		}
	}
	// Managed Policies
	for _, policy := range managedPolicies {
		if _, err := i.IAM.AttachRolePolicyWithContext(ctx, &iam.AttachRolePolicyInput{RoleName: resourceName, PolicyArn: aws.String(policy)}); err != nil {
			return reconcile.Result{}, fmt.Errorf("attaching role policy %w", err)
		}
		logging.FromContext(ctx).Infof("Ensured managed policy %s for %s", policy, aws.StringValue(resourceName))
	}
	// Profile
	if _, err := i.IAM.CreateInstanceProfileWithContext(ctx, &iam.CreateInstanceProfileInput{InstanceProfileName: resourceName}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeEntityAlreadyExistsException {
			return reconcile.Result{}, fmt.Errorf("creating instance profile, %w", err)
		}
		logging.FromContext(ctx).Infof("Found instance profile %s", aws.StringValue(resourceName))
	} else {
		logging.FromContext(ctx).Infof("Created instance profile %s", aws.StringValue(resourceName))
	}
	// Binding
	if _, err := i.IAM.AddRoleToInstanceProfile(&iam.AddRoleToInstanceProfileInput{InstanceProfileName: resourceName, RoleName: resourceName}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeLimitExceededException {
			return reconcile.Result{}, fmt.Errorf("adding role to instance profile, %w", err)
		}
		logging.FromContext(ctx).Infof("Found role %s on instance profile %s", aws.StringValue(resourceName), aws.StringValue(resourceName))
	} else {
		logging.FromContext(ctx).Infof("Added role %s to instance profile %s", aws.StringValue(resourceName), aws.StringValue(resourceName))
	}
	return reconcile.Result{}, nil
}

func (i *InstanceProfile) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	for _, desired := range desiredRolesFor(substrate) {
		result, err := i.delete(ctx, desired.objectName, desired.policyDocument, desired.managedPolicies)
		if err != nil {
			return result, err
		}
	}
	return reconcile.Result{}, nil
}

func (i *InstanceProfile) delete(ctx context.Context, resourceName, policyDocument *string, managedPolicies []string) (reconcile.Result, error) {
	// Policy
	if policyDocument != nil {
		if _, err := i.IAM.DeleteRolePolicyWithContext(ctx, &iam.DeleteRolePolicyInput{RoleName: resourceName, PolicyName: resourceName}); err != nil {
			if err.(awserr.Error).Code() != iam.ErrCodeNoSuchEntityException {
				return reconcile.Result{}, fmt.Errorf("removing policy from role, %w", err)
			}
		} else {
			logging.FromContext(ctx).Infof("Deleted policy %s from role %s", aws.StringValue(resourceName), aws.StringValue(resourceName))
		}
	}
	// Managed Policies
	for _, policy := range managedPolicies {
		if _, err := i.IAM.DetachRolePolicyWithContext(ctx, &iam.DetachRolePolicyInput{RoleName: resourceName, PolicyArn: aws.String(policy)}); err != nil {
			if err.(awserr.Error).Code() != iam.ErrCodeNoSuchEntityException {
				return reconcile.Result{}, fmt.Errorf("detatching policy from role, %w", err)
			}
		} else {
			logging.FromContext(ctx).Infof("Deleted policy %s from role %s", aws.StringValue(resourceName), aws.StringValue(resourceName))
		}
	}
	// Binding
	if _, err := i.IAM.RemoveRoleFromInstanceProfileWithContext(ctx, &iam.RemoveRoleFromInstanceProfileInput{RoleName: resourceName, InstanceProfileName: resourceName}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeNoSuchEntityException {
			return reconcile.Result{}, fmt.Errorf("removing instance profile from role %w,", err)
		}
	} else {
		logging.FromContext(ctx).Infof("Deleted role %s from instance profile %s", aws.StringValue(resourceName), aws.StringValue(resourceName))
	}
	// Profile
	if _, err := i.IAM.DeleteInstanceProfileWithContext(ctx, &iam.DeleteInstanceProfileInput{InstanceProfileName: resourceName}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeNoSuchEntityException {
			return reconcile.Result{}, fmt.Errorf("deleting instance profile %w,", err)
		}
	} else {
		logging.FromContext(ctx).Infof("Deleted instance profile %s", aws.StringValue(resourceName))
	}
	// Role
	if _, err := i.IAM.DeleteRoleWithContext(ctx, &iam.DeleteRoleInput{RoleName: resourceName}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeNoSuchEntityException {
			return reconcile.Result{}, fmt.Errorf("deleting role, %w", err)
		}
	} else {
		logging.FromContext(ctx).Infof("Deleted role %s", aws.StringValue(resourceName))
	}
	return reconcile.Result{}, nil
}

func desiredRolesFor(substrate *v1alpha1.Substrate) []roleInfo {
	return []roleInfo{{
		// Roles and policies attached to the substrate node
		objectName: discovery.Name(substrate), policyDocument: SubstrateNodePolicyDocument,
		managedPolicies: ManagedPoliciesForSubstrateNode,
	}, {
		// Roles and policies attached to the nodes provisioned by Karpenter
		objectName:      discovery.Name(substrate, KarpenterNodeRole),
		managedPolicies: ManagedPoliciesForNodeProvisionedByKarpenter,
	}}
}
