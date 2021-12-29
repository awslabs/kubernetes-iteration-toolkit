package substrate

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"go.uber.org/zap"
)

type iamProfile struct {
	iam *IAM
}

// NewIAMProfileController returns a controller for managing IAM instance profile in AWS
func NewIAMProfileController(iam *IAM) *iamProfile {
	return &iamProfile{iam: iam}
}

// Provision will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with substrate.Status
func (i *iamProfile) Provision(ctx context.Context, substrate *v1alpha1.Substrate) error {
	profile, err := i.getInstanceProfile(ctx, profileName(substrate.Name))
	if err != nil {
		return fmt.Errorf("getting instance profile, %w", err)
	}
	if profile == nil {
		// Create profile in IAM
		result, err := i.iam.CreateInstanceProfile(&iam.CreateInstanceProfileInput{
			InstanceProfileName: aws.String(profileName(substrate.Name)),
		})
		if err != nil {
			return fmt.Errorf("creating profile, %w", err)
		}
		profile = result.InstanceProfile
		zap.S().Infof("Successfully created instance profile %v", *result.InstanceProfile.InstanceProfileName)
	}

	// Add roles to the Instance Profile
	if !rolesContains(profile, roleName(substrate.Name)) {
		if _, err := i.iam.AddRoleToInstanceProfile(&iam.AddRoleToInstanceProfileInput{
			InstanceProfileName: aws.String(profileName(substrate.Name)),
			RoleName:            aws.String(roleName(substrate.Name)),
		}); err != nil {
			return fmt.Errorf("adding role to instance profile, %w", err)
		}
		zap.S().Debugf("Successfully added role %v to instance profile %v", roleName, profileName)
	}
	return nil
}

// Deprovision deletes the resource from AWS
func (i *iamProfile) Deprovision(ctx context.Context, substrate *v1alpha1.Substrate) error {
	// Remove role from profile
	if _, err := i.iam.RemoveRoleFromInstanceProfileWithContext(ctx, &iam.RemoveRoleFromInstanceProfileInput{
		InstanceProfileName: aws.String(profileName(substrate.Name)),
		RoleName:            aws.String(roleName(substrate.Name)),
	}); err != nil {
		return err
	}
	// Delete the profile
	if _, err := i.iam.DeleteInstanceProfileWithContext(ctx, &iam.DeleteInstanceProfileInput{
		InstanceProfileName: aws.String(profileName(substrate.Name)),
	}); err != nil {
		return err
	}
	return nil
}

func (i *iamProfile) getInstanceProfile(ctx context.Context, profileName string) (*iam.InstanceProfile, error) {
	output, err := i.iam.GetInstanceProfileWithContext(ctx, &iam.GetInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
	})
	if iamResourceNotFound(err) {
		return nil, nil
	}
	return output.InstanceProfile, err
}

func rolesContains(profile *iam.InstanceProfile, roleName string) bool {
	for _, role := range profile.Roles {
		if aws.StringValue(role.RoleName) == roleName {
			return true
		}
	}
	return false
}

func profileName(identifier string) string {
	return fmt.Sprintf("substrate-node-profile-for-%s", identifier)
}
