package substrate

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"knative.dev/pkg/logging"
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
func (i *iamRole) Create(ctx context.Context, substrate *v1alpha1.Substrate) error {
	// check if the role exists
	role, err := i.getRole(ctx, substrate.Name)
	if err != nil {
		return err
	}
	if role == nil {
		// create role in IAM
		_, err := i.iamClient.CreateRole(&iam.CreateRoleInput{
			AssumeRolePolicyDocument: aws.String(assumeRolePolicyDocument),
			RoleName:                 aws.String(roleName(substrate.Name)),
		})
		if err != nil {
			return fmt.Errorf("creating role, %w", err)
		}
		logging.FromContext(ctx).Infof("Successfully created role %v", roleName(substrate.Name))
		return nil
	}
	logging.FromContext(ctx).Debugf("Successfully discovered role %v", roleName(substrate.Name))
	return nil
}

// Finalize deletes the resource from AWS
func (i *iamRole) Delete(ctx context.Context, substrate *v1alpha1.Substrate) error {
	// check if the role exists
	role, err := i.getRole(ctx, substrate.Name)
	if err != nil {
		return fmt.Errorf("getting role %w", err)
	}
	if role == nil {
		return nil
	}
	if _, err := i.iamClient.DeleteRoleWithContext(ctx, &iam.DeleteRoleInput{
		RoleName: aws.String(roleName(substrate.Name)),
	}); err != nil {
		return err
	}
	logging.FromContext(ctx).Infof("Successfully deleted role %s", roleName(substrate.Name))
	return nil
}

func (i *iamRole) getRole(ctx context.Context, identifier string) (*iam.GetRoleOutput, error) {
	role, err := i.iamClient.GetRoleWithContext(ctx, &iam.GetRoleInput{
		RoleName: aws.String(roleName(identifier)),
	})
	if err != nil && iamResourceNotFound(err) {
		return nil, nil
	}
	return role, err
}

func roleName(identifier string) string {
	return fmt.Sprintf("substrate-node-role-for-%s", identifier)
}
