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
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	substrateNodePolicy = `{
	"Version": "2012-10-17",
	"Statement": [
		{
			"Effect": "Allow",
			"Action": "*",
			"Resource": "*"
		}
	]
}`
)

type iamPolicy struct {
	IAM *iam.IAM
}

// Create will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with substrate.Status
func (i *iamPolicy) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if _, err := i.IAM.GetRoleWithContext(ctx, &iam.GetRoleInput{RoleName: aws.String(roleName(substrate.Name))}); err != nil {
		if err.(awserr.Error).Code() == iam.ErrCodeNoSuchEntityException {
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{}, fmt.Errorf("getting role, %w", err)
	}
	if _, err := i.IAM.PutRolePolicyWithContext(ctx, &iam.PutRolePolicyInput{
		RoleName:       aws.String(roleName(substrate.Name)),
		PolicyName:     aws.String(policyName(substrate.Name)),
		PolicyDocument: aws.String(substrateNodePolicy),
	}); err != nil {
		return reconcile.Result{}, fmt.Errorf("adding policy to role, %w", err)
	}
	logging.FromContext(ctx).Infof("Ensured policy %s for %s", policyName(substrate.Name), roleName(substrate.Name))
	return reconcile.Result{}, nil
}

// Delete deletes the resource from AWS
func (i *iamPolicy) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if _, err := i.IAM.DeleteRolePolicyWithContext(ctx, &iam.DeleteRolePolicyInput{
		RoleName:   aws.String(roleName(substrate.Name)),
		PolicyName: aws.String(policyName(substrate.Name)),
	}); err != nil {
		if err.(awserr.Error).Code() == iam.ErrCodeNoSuchEntityException {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("removing policy from role, %w", err)
	}
	logging.FromContext(ctx).Infof("Deleted policy %s from role %s", policyName(substrate.Name), roleName(substrate.Name))
	return reconcile.Result{}, nil
}

func policyName(identifier string) string {
	return fmt.Sprintf("substrate-node-policy-for-%s", identifier)
}
