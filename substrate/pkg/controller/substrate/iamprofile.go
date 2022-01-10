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

type iamProfile struct {
	IAM *iam.IAM
}

func (i *iamProfile) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if _, err := i.IAM.CreateInstanceProfileWithContext(ctx, &iam.CreateInstanceProfileInput{InstanceProfileName: aws.String(instanceProfileName(substrate.Name))}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeEntityAlreadyExistsException {
			return reconcile.Result{}, fmt.Errorf("creating instance profile, %w", err)
		}
		logging.FromContext(ctx).Infof("Found instance profile %s", instanceProfileName(substrate.Name))
	} else {
		logging.FromContext(ctx).Infof("Created instance profile %s", instanceProfileName(substrate.Name))
	}
	if _, err := i.IAM.AddRoleToInstanceProfile(&iam.AddRoleToInstanceProfileInput{
		InstanceProfileName: aws.String(instanceProfileName(substrate.Name)),
		RoleName:            aws.String(roleName(substrate.Name)),
	}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeLimitExceededException {
			return reconcile.Result{}, fmt.Errorf("adding role to instance profile, %w", err)
		}
		logging.FromContext(ctx).Infof("Found role %s on instance profile %s", roleName(substrate.Name), instanceProfileName(substrate.Name))
	} else {
		logging.FromContext(ctx).Infof("Added role %s to instance profile %s", roleName(substrate.Name), instanceProfileName(substrate.Name))
	}
	return reconcile.Result{}, nil
}

func (i *iamProfile) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if _, err := i.IAM.RemoveRoleFromInstanceProfileWithContext(ctx, &iam.RemoveRoleFromInstanceProfileInput{
		InstanceProfileName: aws.String(instanceProfileName(substrate.Name)),
		RoleName:            aws.String(roleName(substrate.Name)),
	}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeNoSuchEntityException {
			return reconcile.Result{}, fmt.Errorf("removing instance profile from role %w,", err)
		}
	} else {
		logging.FromContext(ctx).Infof("Deleted role %s from instance profile %s", roleName(substrate.Name), instanceProfileName(substrate.Name))
	}
	if _, err := i.IAM.DeleteInstanceProfileWithContext(ctx, &iam.DeleteInstanceProfileInput{InstanceProfileName: aws.String(instanceProfileName(substrate.Name))}); err != nil {
		if err.(awserr.Error).Code() != iam.ErrCodeNoSuchEntityException {
			return reconcile.Result{}, fmt.Errorf("deleting instance profile %w,", err)
		}
	} else {
		logging.FromContext(ctx).Infof("Deleted instance profile %s", instanceProfileName(substrate.Name))
	}
	return reconcile.Result{}, nil
}

func instanceProfileName(identifier string) string {
	return fmt.Sprintf("substrate-node-profile-for-%s", identifier)
}
