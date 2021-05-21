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
	"github.com/prateekgogia/kit/pkg/kiterr"
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
	return &v1alpha1.ControlPlane{}
}

const (
	MasterInstanceProfileName = "master-instance-profile-cluster-%s"
	ETCDInstanceProfileName   = "etcd-instance-profile-cluster-%s"
)

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (i *iamProfile) Reconcile(ctx context.Context, object controllers.Object) (reconcile.Result, error) {
	controlPlane := object.(*v1alpha1.ControlPlane)
	desiredProfilesRolesMapping := i.profileToRoleMapping(controlPlane.Name)
	actualProfilesRolesMapping := map[string][]*iam.Role{}
	// Create desired profiles if not exist
	for profileName := range desiredProfilesRolesMapping {
		profile, err := i.getInstanceProfile(ctx, profileName)
		if kiterr.IsIAMResourceNotFound(err) {
			// Create profile in IAM
			result, err := i.iam.CreateInstanceProfile(&iam.CreateInstanceProfileInput{
				InstanceProfileName: aws.String(profileName),
			})
			if err != nil {
				return ResourceFailedProgressing, fmt.Errorf("creating profile, %w", err)
			}
			zap.S().Infof("Successfully created instance profile %v", *result.InstanceProfile.InstanceProfileName)
			actualProfilesRolesMapping[profileName] = result.InstanceProfile.Roles
			continue
		} else if err != nil {
			return ResourceFailedProgressing, fmt.Errorf("getting instance profile, %w", err)
		}
		actualProfilesRolesMapping[profileName] = profile.InstanceProfile.Roles
		zap.S().Debugf("Successfully discovered profile %v for cluster %v", *profile.InstanceProfile.InstanceProfileName, controlPlane.Name)
	}
	// Add roles to the Instance Profile
	for profileName, roles := range actualProfilesRolesMapping {
		roleName := desiredProfilesRolesMapping[profileName]
		// if the roles are already to the instance profile
		if rolesContains(roles, roleName) {
			continue
		}
		if _, err := i.iam.AddRoleToInstanceProfile(&iam.AddRoleToInstanceProfileInput{
			InstanceProfileName: aws.String(profileName),
			RoleName:            aws.String(roleName),
		}); err != nil && kiterr.IsIAMResourceNotFound(err) { //if role is not created yet
			return WaitingForSubResource, nil
		} else if err != nil {
			return ResourceFailedProgressing, fmt.Errorf("adding role to instance profile, %w", err)
		}
		zap.S().Debugf("Successfully added role %v to instance profile %v", roleName, profileName)
	}
	return ResourceCreated, nil
}

// Finalize deletes the resource from AWS
func (i *iamProfile) Finalize(ctx context.Context, object controllers.Object) (reconcile.Result, error) {
	controlPlane := object.(*v1alpha1.ControlPlane)
	for profileName, roleName := range i.profileToRoleMapping(controlPlane.Name) {
		if _, err := i.iam.RemoveRoleFromInstanceProfileWithContext(ctx, &iam.RemoveRoleFromInstanceProfileInput{
			InstanceProfileName: aws.String(profileName),
			RoleName:            aws.String(roleName),
		}); err != nil {
			if kiterr.IsIAMResourceNotFound(err) {
				continue
			}
			return ResourceFailedProgressing, err
		}
		if _, err := i.iam.DeleteInstanceProfileWithContext(ctx, &iam.DeleteInstanceProfileInput{
			InstanceProfileName: aws.String(profileName),
		}); err != nil {
			if kiterr.IsIAMResourceNotFound(err) ||
				kiterr.IsIAMResourceDependencyExists(err) {
				continue
			}
			return ResourceFailedProgressing, err
		}
	}
	return ResourceCreated, nil
}

func rolesContains(roles []*iam.Role, roleName string) bool {
	for _, role := range roles {
		if aws.StringValue(role.RoleName) == roleName {
			return true
		}
	}
	return false
}

func (i *iamProfile) profileToRoleMapping(clusterName string) map[string]string {
	masterInstanceProfile := fmt.Sprintf(MasterInstanceProfileName, clusterName)
	etcdInstanceProfile := fmt.Sprintf(ETCDInstanceProfileName, clusterName)
	return map[string]string{
		masterInstanceProfile: fmt.Sprintf(MasterInstanceRoleName, clusterName),
		etcdInstanceProfile:   fmt.Sprintf(ETCDInstanceRoleName, clusterName),
	}
}

func (i *iamProfile) getInstanceProfile(ctx context.Context, profileName string) (*iam.GetInstanceProfileOutput, error) {
	profile, err := i.iam.GetInstanceProfileWithContext(ctx, &iam.GetInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
	})
	return profile, err
}
