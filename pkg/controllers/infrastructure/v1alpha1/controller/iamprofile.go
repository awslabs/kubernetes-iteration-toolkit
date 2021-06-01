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
	"github.com/prateekgogia/kit/pkg/apis/infrastructure/v1alpha1"
	"github.com/prateekgogia/kit/pkg/awsprovider"
	"github.com/prateekgogia/kit/pkg/controllers"
	"github.com/prateekgogia/kit/pkg/errors"
	"github.com/prateekgogia/kit/pkg/status"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type iamProfile struct {
	iam *awsprovider.IAM
}

// NewIAMProfileController returns a controller for managing elasticIPs in AWS
func NewIAMProfileController(iam *awsprovider.IAM) *iamProfile {
	return &iamProfile{iam: iam}
}

// Name returns the name of the controller
func (i *iamProfile) Name() string {
	return "iam-profile"
}

// For returns the resource this controller is for.
func (i *iamProfile) For() controllers.Object {
	return &v1alpha1.Profile{}
}

const (
	MasterInstanceProfileName = "master-instance-profile-cluster-%s"
	ETCDInstanceProfileName   = "etcd-instance-profile-cluster-%s"
)

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (i *iamProfile) Reconcile(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	profileObj := object.(*v1alpha1.Profile)
	// Create desired profiles if not exist
	var roles []*iam.Role
	profileName := v1alpha1.ProfileName(profileObj.Name)
	// for profileName := range desiredProfilesRolesMapping {
	profile, err := i.getInstanceProfile(ctx, profileName)
	if errors.IsIAMResourceNotFound(err) {
		// Create profile in IAM
		result, err := i.iam.CreateInstanceProfile(&iam.CreateInstanceProfileInput{
			InstanceProfileName: aws.String(profileName),
		})
		if err != nil {
			return nil, fmt.Errorf("creating profile, %w", err)
		}
		zap.S().Infof("Successfully created instance profile %v", *result.InstanceProfile.InstanceProfileName)
		roles = result.InstanceProfile.Roles
	} else if err != nil {
		return nil, fmt.Errorf("getting instance profile, %w", err)
	} else {
		roles = profile.InstanceProfile.Roles
		zap.S().Debugf("Successfully discovered profile %v", *profile.InstanceProfile.InstanceProfileName)
	}
	// Add roles to the Instance Profile
	roleName := fmt.Sprintf("%s-role", profileObj.Name)
	if !rolesContains(roles, roleName) {
		if _, err := i.iam.AddRoleToInstanceProfile(&iam.AddRoleToInstanceProfileInput{
			InstanceProfileName: aws.String(profileName),
			RoleName:            aws.String(roleName),
		}); err != nil && errors.IsIAMResourceNotFound(err) { //if role is not created yet
			return status.Waiting, nil
		} else if err != nil {
			return nil, fmt.Errorf("adding role to instance profile, %w", err)
		}
		zap.S().Debugf("Successfully added role %v to instance profile %v", roleName, profileName)
	}
	return status.Created, nil
}

// Finalize deletes the resource from AWS
func (i *iamProfile) Finalize(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	profileObj := object.(*v1alpha1.Profile)
	profileName := v1alpha1.ProfileName(profileObj.Name)
	roleName := fmt.Sprintf("%s-role", profileObj.Name)
	if _, err := i.iam.RemoveRoleFromInstanceProfileWithContext(ctx, &iam.RemoveRoleFromInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
		RoleName:            aws.String(roleName),
	}); err != nil {
		return nil, err
	}
	if _, err := i.iam.DeleteInstanceProfileWithContext(ctx, &iam.DeleteInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
	}); err != nil {

		return nil, err
	}
	return status.Terminated, nil
}

func rolesContains(roles []*iam.Role, roleName string) bool {
	for _, role := range roles {
		if aws.StringValue(role.RoleName) == roleName {
			return true
		}
	}
	return false
}

func (i *iamProfile) getInstanceProfile(ctx context.Context, profileName string) (*iam.GetInstanceProfileOutput, error) {
	profile, err := i.iam.GetInstanceProfileWithContext(ctx, &iam.GetInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
	})
	return profile, err
}
