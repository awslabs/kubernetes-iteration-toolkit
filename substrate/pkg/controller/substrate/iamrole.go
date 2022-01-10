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

type iamRole struct {
	iamClient *iam.IAM
}

var assumeRolePolicyDocument = `{
	"Version": "2012-10-17",
	"Statement": [
		{
			"Action": "sts:AssumeRole",
			"Effect": "Allow",
			"Principal": {
				"Service": "ec2.amazonaws.com"
			}
		}
	]
}`

// Create will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the substrate.Status
func (i *iamRole) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if _, err := i.iamClient.CreateRole(&iam.CreateRoleInput{
		AssumeRolePolicyDocument: aws.String(assumeRolePolicyDocument),
		RoleName:                 aws.String(roleName(substrate.Name)),
	}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeEntityAlreadyExistsException {
			return reconcile.Result{}, fmt.Errorf("creating role, %w", err)
		}
		logging.FromContext(ctx).Infof("Found role %s", roleName(substrate.Name))
	} else {
		logging.FromContext(ctx).Infof("Created role %s", roleName(substrate.Name))
	}
	return reconcile.Result{}, nil
}

// Finalize deletes the resource from AWS
func (i *iamRole) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if _, err := i.iamClient.DeleteRoleWithContext(ctx, &iam.DeleteRoleInput{
		RoleName: aws.String(roleName(substrate.Name)),
	}); err != nil {
		if err.(awserr.Error).Code() == iam.ErrCodeDeleteConflictException {
			return reconcile.Result{Requeue: true}, nil
		}
		if err.(awserr.Error).Code() != iam.ErrCodeNoSuchEntityException {
			return reconcile.Result{}, err
		}
	} else {
		logging.FromContext(ctx).Infof("Deleted role %s", roleName(substrate.Name))
	}
	return reconcile.Result{}, nil
}

func roleName(identifier string) string {
	return fmt.Sprintf("substrate-node-role-for-%s", identifier)
}
