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

var PolicyDocument = aws.String(`{
	"Version": "2012-10-17",
	"Statement": [
		{
			"Effect": "Allow",
			"Action": [
				"ec2:AssociateAddress"
			],
			"Resource": ["*"]
		}
	]
}`)

var ManagedPolicies = []string{
	"arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore",
}

func (i *InstanceProfile) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	// Role
	if _, err := i.IAM.CreateRole(&iam.CreateRoleInput{RoleName: discovery.Name(substrate), AssumeRolePolicyDocument: AssumeRolePolicyDocument}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeEntityAlreadyExistsException {
			return reconcile.Result{}, fmt.Errorf("creating role, %w", err)
		}
		logging.FromContext(ctx).Infof("Found role %s", aws.StringValue(discovery.Name(substrate)))
	} else {
		logging.FromContext(ctx).Infof("Created role %s", aws.StringValue(discovery.Name(substrate)))
	}
	// Policy
	if _, err := i.IAM.PutRolePolicyWithContext(ctx, &iam.PutRolePolicyInput{RoleName: discovery.Name(substrate), PolicyName: discovery.Name(substrate), PolicyDocument: PolicyDocument}); err != nil {
		return reconcile.Result{}, fmt.Errorf("adding policy to role, %w", err)
	} else {
		logging.FromContext(ctx).Infof("Created policy %s for %s", aws.StringValue(discovery.Name(substrate)), aws.StringValue(discovery.Name(substrate)))
	}
	// Managed Policies
	for _, policy := range ManagedPolicies {
		if _, err := i.IAM.AttachRolePolicyWithContext(ctx, &iam.AttachRolePolicyInput{RoleName: discovery.Name(substrate), PolicyArn: aws.String(policy)}); err != nil {
			return reconcile.Result{}, fmt.Errorf("attaching role policy %w", err)
		}
		logging.FromContext(ctx).Infof("Ensured managed policy %s for %s", policy, aws.StringValue(discovery.Name(substrate)))
	}
	// Profile
	if _, err := i.IAM.CreateInstanceProfileWithContext(ctx, &iam.CreateInstanceProfileInput{InstanceProfileName: discovery.Name(substrate)}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeEntityAlreadyExistsException {
			return reconcile.Result{}, fmt.Errorf("creating instance profile, %w", err)
		}
		logging.FromContext(ctx).Infof("Found instance profile %s", aws.StringValue(discovery.Name(substrate)))
	} else {
		logging.FromContext(ctx).Infof("Created instance profile %s", aws.StringValue(discovery.Name(substrate)))
	}
	// Binding
	if _, err := i.IAM.AddRoleToInstanceProfile(&iam.AddRoleToInstanceProfileInput{InstanceProfileName: discovery.Name(substrate), RoleName: discovery.Name(substrate)}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeLimitExceededException {
			return reconcile.Result{}, fmt.Errorf("adding role to instance profile, %w", err)
		}
		logging.FromContext(ctx).Infof("Found role %s on instance profile %s", aws.StringValue(discovery.Name(substrate)), aws.StringValue(discovery.Name(substrate)))
	} else {
		logging.FromContext(ctx).Infof("Added role %s to instance profile %s", aws.StringValue(discovery.Name(substrate)), aws.StringValue(discovery.Name(substrate)))
	}
	return reconcile.Result{}, nil
}

func (i *InstanceProfile) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	// Policy
	if _, err := i.IAM.DeleteRolePolicyWithContext(ctx, &iam.DeleteRolePolicyInput{RoleName: discovery.Name(substrate), PolicyName: discovery.Name(substrate)}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeNoSuchEntityException {
			return reconcile.Result{}, fmt.Errorf("removing policy from role, %w", err)
		}
	} else {
		logging.FromContext(ctx).Infof("Deleted policy %s from role %s", aws.StringValue(discovery.Name(substrate)), aws.StringValue(discovery.Name(substrate)))
	}
	// Managed Policies
	for _, policy := range ManagedPolicies {
		if _, err := i.IAM.DetachRolePolicyWithContext(ctx, &iam.DetachRolePolicyInput{RoleName: discovery.Name(substrate), PolicyArn: aws.String(policy)}); err != nil {
			if err.(awserr.Error).Code() != iam.ErrCodeNoSuchEntityException {
				return reconcile.Result{}, fmt.Errorf("detatching policy from role, %w", err)
			}
		} else {
			logging.FromContext(ctx).Infof("Deleted policy %s from role %s", aws.StringValue(discovery.Name(substrate)), aws.StringValue(discovery.Name(substrate)))
		}
	}
	// Binding
	if _, err := i.IAM.RemoveRoleFromInstanceProfileWithContext(ctx, &iam.RemoveRoleFromInstanceProfileInput{RoleName: discovery.Name(substrate), InstanceProfileName: discovery.Name(substrate)}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeNoSuchEntityException {
			return reconcile.Result{}, fmt.Errorf("removing instance profile from role %w,", err)
		}
	} else {
		logging.FromContext(ctx).Infof("Deleted role %s from instance profile %s", aws.StringValue(discovery.Name(substrate)), aws.StringValue(discovery.Name(substrate)))
	}
	// Profile
	if _, err := i.IAM.DeleteInstanceProfileWithContext(ctx, &iam.DeleteInstanceProfileInput{InstanceProfileName: discovery.Name(substrate)}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeNoSuchEntityException {
			return reconcile.Result{}, fmt.Errorf("deleting instance profile %w,", err)
		}
	} else {
		logging.FromContext(ctx).Infof("Deleted instance profile %s", aws.StringValue(discovery.Name(substrate)))
	}
	// Role
	if _, err := i.IAM.DeleteRoleWithContext(ctx, &iam.DeleteRoleInput{RoleName: discovery.Name(substrate)}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeNoSuchEntityException {
			return reconcile.Result{}, fmt.Errorf("deleting role, %w", err)
		}
	} else {
		logging.FromContext(ctx).Infof("Deleted role %s", aws.StringValue(discovery.Name(substrate)))
	}
	return reconcile.Result{}, nil
}
