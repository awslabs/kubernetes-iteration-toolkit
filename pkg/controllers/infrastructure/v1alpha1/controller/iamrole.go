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
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/awsprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/controllers"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/errors"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/status"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type iamRole struct {
	iam *awsprovider.IAM
}

// NewIAMRoleController returns a controller for managing elasticIPs in AWS
func NewIAMRoleController(iam *awsprovider.IAM) *iamRole {
	return &iamRole{iam: iam}
}

// Name returns the name of the controller
func (i *iamRole) Name() string {
	return "iam-role"
}

// For returns the resource this controller is for.
func (i *iamRole) For() controllers.Object {
	return &v1alpha1.Role{}
}

const (
	MasterInstanceRoleName = "master-instance-role-cluster-%s"
	// MasterInstanceProfileName = "master-instance-profile-cluster-%s"
	ETCDInstanceRoleName = "etcd-instance-role-cluster-%s"
	// ETCDInstanceProfileName   = "etcd-instance-profile-cluster-%s"
)

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

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (i *iamRole) Reconcile(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	roleObj := object.(*v1alpha1.Role)
	role, err := i.getRole(ctx, roleObj.Spec.RoleName)
	if errors.IsIAMResourceNotFound(err) {
		// Create role in IAM
		role, err := i.iam.CreateRole(&iam.CreateRoleInput{
			AssumeRolePolicyDocument: aws.String(assumeRolePolicyDocument),
			RoleName:                 aws.String(roleObj.Spec.RoleName),
		})
		if err != nil {
			return nil, fmt.Errorf("creating role, %w", err)
		}
		zap.S().Infof("Successfully created role %v for cluster %v", *role.Role.RoleName, roleObj.Spec.ClusterName)
		return status.Created, nil
	} else if err != nil {
		return nil, fmt.Errorf("getting role, %w", err)
	}
	zap.S().Debugf("Successfully discovered role %v for cluster %v", *role.Role.RoleName, roleObj.Spec.ClusterName)
	return status.Created, nil
}

// Finalize deletes the resource from AWS
func (i *iamRole) Finalize(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	roleObj := object.(*v1alpha1.Role)
	if _, err := i.getRole(ctx, roleObj.Spec.RoleName); err != nil && errors.IsIAMResourceNotFound(err) {
		return status.Terminated, nil
	}
	if _, err := i.iam.DeleteRoleWithContext(ctx, &iam.DeleteRoleInput{
		RoleName: aws.String(roleObj.Spec.RoleName),
	}); err != nil {
		return nil, err
	}
	zap.S().Infof("Successfully deleted role %s", roleObj.Spec.RoleName)
	return status.Terminated, nil
}

func (i *iamRole) getRole(ctx context.Context, roleName string) (*iam.GetRoleOutput, error) {
	return getRole(ctx, i.iam, roleName)
}

func getRole(ctx context.Context, iamApi *awsprovider.IAM, roleName string) (*iam.GetRoleOutput, error) {
	role, err := iamApi.GetRoleWithContext(ctx, &iam.GetRoleInput{
		RoleName: aws.String(roleName),
	})
	return role, err
}
