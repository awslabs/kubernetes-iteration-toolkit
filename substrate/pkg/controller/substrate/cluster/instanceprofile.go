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
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/utils/discovery"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type InstanceProfile struct {
	IAM *iam.IAM
}

type role struct {
	name            *string
	policy          *string
	managedPolicies []string
}

func (i *InstanceProfile) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	for _, desired := range desiredRolesFor(substrate) {
		result, err := i.CreateInstanceProfile(ctx, desired.name, desired.policy, desired.managedPolicies)
		if err != nil {
			return result, err
		}
	}
	return reconcile.Result{}, nil
}

func (i *InstanceProfile) CreateInstanceProfile(ctx context.Context, resourceName, policy *string, managedPolicies []string) (reconcile.Result, error) {
	//Todo: remove fargate service principle when we have this in place - https://github.com/awslabs/kubernetes-iteration-toolkit/issues/186
	// Role
	if _, err := i.IAM.CreateRole(&iam.CreateRoleInput{RoleName: resourceName, AssumeRolePolicyDocument: aws.String(`{
	"Version": "2012-10-17",
	"Statement": [
		{
			"Effect": "Allow",
			"Action": "sts:AssumeRole",
			"Principal": {
				"Service": [
				 "ec2.amazonaws.com",
				 "eks-fargate-pods.amazonaws.com"
				 ]
			}
		}
	]}`)}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeEntityAlreadyExistsException {
			return reconcile.Result{}, fmt.Errorf("creating role, %w", err)
		}
		logging.FromContext(ctx).Debugf("Found role %s", aws.StringValue(resourceName))
	} else {
		logging.FromContext(ctx).Infof("Created role %s", aws.StringValue(resourceName))
	}
	// Policy
	if policy != nil {
		if _, err := i.IAM.PutRolePolicyWithContext(ctx, &iam.PutRolePolicyInput{RoleName: resourceName, PolicyName: resourceName, PolicyDocument: policy}); err != nil {
			return reconcile.Result{}, fmt.Errorf("adding policy to role, %w", err)
		}
		logging.FromContext(ctx).Infof("Ensured policy %s for %s", aws.StringValue(resourceName), aws.StringValue(resourceName))
	}
	// Managed Policies
	for _, policy := range managedPolicies {
		if _, err := i.IAM.AttachRolePolicyWithContext(ctx, &iam.AttachRolePolicyInput{RoleName: resourceName, PolicyArn: aws.String(policy)}); err != nil {
			return reconcile.Result{}, fmt.Errorf("attaching role policy %w", err)
		}
		logging.FromContext(ctx).Debugf("Ensured managed policy %s for %s", policy, aws.StringValue(resourceName))
	}
	// Profile
	if _, err := i.IAM.CreateInstanceProfileWithContext(ctx, &iam.CreateInstanceProfileInput{InstanceProfileName: resourceName}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeEntityAlreadyExistsException {
			return reconcile.Result{}, fmt.Errorf("creating instance profile, %w", err)
		}
		logging.FromContext(ctx).Debugf("Found instance profile %s", aws.StringValue(resourceName))
	} else {
		logging.FromContext(ctx).Infof("Created instance profile %s", aws.StringValue(resourceName))
	}
	// Binding
	if _, err := i.IAM.AddRoleToInstanceProfile(&iam.AddRoleToInstanceProfileInput{InstanceProfileName: resourceName, RoleName: resourceName}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeLimitExceededException {
			return reconcile.Result{}, fmt.Errorf("adding role to instance profile, %w", err)
		}
		logging.FromContext(ctx).Debugf("Found role %s on instance profile %s", aws.StringValue(resourceName), aws.StringValue(resourceName))
	} else {
		logging.FromContext(ctx).Infof("Added role %s to instance profile %s", aws.StringValue(resourceName), aws.StringValue(resourceName))
	}
	return reconcile.Result{}, nil
}

func (i *InstanceProfile) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	for _, desired := range desiredRolesFor(substrate) {
		result, err := i.DeleteInstanceProfile(ctx, desired.name, desired.policy, desired.managedPolicies)
		if err != nil {
			return result, err
		}
	}
	return reconcile.Result{}, nil
}

func (i *InstanceProfile) DeleteInstanceProfile(ctx context.Context, resourceName, policy *string, managedPolicies []string) (reconcile.Result, error) {
	// Policy
	if policy != nil {
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

func desiredRolesFor(substrate *v1alpha1.Substrate) []role {
	return []role{{
		// Roles and policies attached to the substrate node
		name: discovery.Name(substrate), policy: aws.String(`{
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
						"ec2:TerminateInstances",
						"ec2:DescribeLaunchTemplates",
						"ec2:DescribeInstances",
						"ec2:DescribeSecurityGroups",
						"ec2:DescribeSubnets",
						"ec2:DescribeInstanceTypes",
						"ec2:DescribeInstanceTypeOfferings",
						"ec2:DescribeAvailabilityZones",
						"ec2:DescribeAccountAttributes",
						"ec2:DescribeInternetGateways",
						"ec2:DescribeVpcs",
						"ec2:DescribeNetworkInterfaces",
						"ec2:DescribeTags",
						"ec2:GetCoipPoolUsage",
						"ec2:DescribeCoipPools",
						"ec2:CreateLaunchTemplateVersion",
						"ec2:DeleteLaunchTemplate",
						"ec2:DescribeLaunchTemplateVersions",
						"ec2:AuthorizeSecurityGroupIngress",
						"ec2:RevokeSecurityGroupIngress",
						"ec2:CreateSnapshot",
						"ec2:AttachVolume",
						"ec2:DetachVolume",
						"ec2:ModifyVolume",
						"ec2:DescribeVolumes",
						"ec2:DescribeVolumesModifications",
						"ec2:DeleteTags",
						"ec2:CreateVolume",
						"ec2:DeleteVolume",
						"ec2:DeleteSnapshot",
						"elasticloadbalancing:DescribeLoadBalancers",
						"elasticloadbalancing:DescribeLoadBalancerAttributes",
						"elasticloadbalancing:DescribeListeners",
						"elasticloadbalancing:DescribeListenerCertificates",
						"elasticloadbalancing:DescribeSSLPolicies",
						"elasticloadbalancing:DescribeRules",
						"elasticloadbalancing:DescribeTargetGroups",
						"elasticloadbalancing:DescribeTargetGroupAttributes",
						"elasticloadbalancing:DescribeTargetHealth",
						"elasticloadbalancing:DescribeTags",
						"elasticloadbalancing:CreateLoadBalancer",
						"elasticloadbalancing:CreateTargetGroup",
						"elasticloadbalancing:CreateListener",
						"elasticloadbalancing:DeleteListener",
						"elasticloadbalancing:CreateRule",
						"elasticloadbalancing:DeleteRule",
						"elasticloadbalancing:AddTags",
						"elasticloadbalancing:RemoveTags",
						"elasticloadbalancing:ModifyLoadBalancerAttributes",
						"elasticloadbalancing:SetIpAddressType",
						"elasticloadbalancing:SetSecurityGroups",
						"elasticloadbalancing:SetSubnets",
						"elasticloadbalancing:DeleteLoadBalancer",
						"elasticloadbalancing:ModifyTargetGroup",
						"elasticloadbalancing:ModifyTargetGroupAttributes",
						"elasticloadbalancing:DeleteTargetGroup",
						"elasticloadbalancing:RegisterTargets",
						"elasticloadbalancing:DeregisterTargets",
						"elasticloadbalancing:SetWebAcl",
						"elasticloadbalancing:ModifyListener",
						"elasticloadbalancing:AddListenerCertificates",
						"elasticloadbalancing:RemoveListenerCertificates",
						"elasticloadbalancing:ModifyRule",
						"iam:CreateRole",
						"iam:PassRole",
						"iam:AddRoleToInstanceProfile",
						"iam:CreateInstanceProfile",
						"iam:AttachRolePolicy",
						"iam:RemoveRoleFromInstanceProfile",
						"iam:DeleteInstanceProfile",
						"iam:DetachRolePolicy",
						"iam:DeleteRole",
						"iam:TagRole",
						"iam:GetRole",
						"iam:GetInstanceProfile",
						"iam:CreateServiceLinkedRole",
						"iam:ListServerCertificates",
						"iam:GetServerCertificate",
						"shield:GetSubscriptionState",
						"shield:DescribeProtection",
						"shield:CreateProtection",
						"shield:DeleteProtection",
						"waf-regional:GetWebACL",
						"waf-regional:GetWebACLForResource",
						"waf-regional:AssociateWebACL",
						"waf-regional:DisassociateWebACL",
						"wafv2:GetWebACL",
						"wafv2:GetWebACLForResource",
						"wafv2:AssociateWebACL",
						"wafv2:DisassociateWebACL",
						"cognito-idp:DescribeUserPoolClient",
						"acm:ListCertificates",
						"acm:DescribeCertificate",
						"ssm:GetParameter",
						"autoscaling:CreateOrUpdateTags",
						"autoscaling:CreateAutoScalingGroup",
						"autoscaling:DeleteAutoScalingGroup",
						"autoscaling:UpdateAutoScalingGroup",
						"autoscaling:SetDesiredCapacity",
						"autoscaling:DescribeAutoScalingGroups",
						"logs:*",
						"cloudwatch:*"
					],
					"Resource": ["*"]
				}
			]
		}`),
		managedPolicies: []string{
			"arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore",
			"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
			"arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
			"arn:aws:iam::aws:policy/AmazonPrometheusRemoteWriteAccess",
		},
	}, {
		// Roles and policies attached to the nodes provisioned by Karpenter
		// Todo: remove `eks, iam action once we have this support for this - https://github.com/awslabs/kubernetes-iteration-toolkit/issues/186`
		name: discovery.Name(substrate, TenantControlPlaneNodeRole), policy: aws.String(`{
			"Version": "2012-10-17",
			"Statement": [
				{
					"Effect": "Allow",
					"Action": [
						"iam:GetRole",
						"iam:PassRole",
						"iam:CreateServiceLinkedRole",
						"iam:ListAttachedRolePolicies",
						"kms:Encrypt",
						"kms:Decrypt",
						"logs:*",
						"eks:*",
						"s3:*",
						"cloudwatch:*"
					],
					"Resource": ["*"]
				}
			]
		}`),
		managedPolicies: []string{
			"arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore",
			"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
			"arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy",
			"arn:aws:iam::aws:policy/AmazonEKSClusterPolicy",
			"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
			"arn:aws:iam::aws:policy/AmazonPrometheusRemoteWriteAccess",
			"arn:aws:iam::aws:policy/AmazonS3FullAccess",
		},
	}}
}
