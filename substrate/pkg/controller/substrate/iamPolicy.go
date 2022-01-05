package substrate

import (
	"context"
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"go.uber.org/zap"
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
	iam *IAM
}

// NewIAMPolicyController returns a controller for managing IAM policy in AWS
func NewIAMPolicyController(iam *IAM) *iamPolicy {
	return &iamPolicy{iam: iam}
}

// Create will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with substrate.Status
func (i *iamPolicy) Create(ctx context.Context, substrate *v1alpha1.Substrate) error {
	// assume role already exists in IAM so we skip checking for role in IAM
	// check policy exists on the role
	output, err := i.getRolePolicy(ctx, policyName(substrate.Name), roleName(substrate.Name))
	if err != nil && !iamResourceNotFound(err) {
		return fmt.Errorf("getting role policy, %w", err)
	}
	if !policyFoundMatchesDesired(output, substrateNodePolicy) {
		// Policy is not found or doesn't match the desired policy
		if _, err := i.iam.PutRolePolicyWithContext(ctx, &iam.PutRolePolicyInput{
			RoleName:       aws.String(roleName(substrate.Name)),
			PolicyName:     aws.String(policyName(substrate.Name)),
			PolicyDocument: aws.String(substrateNodePolicy),
		}); err != nil {
			return fmt.Errorf("adding policy to role, %w", err)
		}
		zap.S().Infof("Successfully added policy %v to role %v", policyName, roleName)
		return nil
	}
	return nil
}

// Delete deletes the resource from AWS
func (i *iamPolicy) Delete(ctx context.Context, substrate *v1alpha1.Substrate) error {
	if _, err := i.iam.DeleteRolePolicyWithContext(ctx, &iam.DeleteRolePolicyInput{
		RoleName:   aws.String(roleName(substrate.Name)),
		PolicyName: aws.String(policyName(substrate.Name)),
	}); err != nil {
		zap.S().Errorf("Failed to delete role policy, %v", err)
		return err
	}
	zap.S().Infof("Successfully removed policy %s from role %s", policyName, roleName)
	return nil
}

func (i *iamPolicy) getRolePolicy(ctx context.Context, policy, role string) (*iam.GetRolePolicyOutput, error) {
	output, err := i.iam.GetRolePolicyWithContext(ctx, &iam.GetRolePolicyInput{
		PolicyName: aws.String(policy),
		RoleName:   aws.String(role),
	})
	if err != nil {
		return nil, err
	}
	return output, nil
}

func policyFoundMatchesDesired(output *iam.GetRolePolicyOutput, expectedPolicy string) bool {
	if output != nil {
		decodedPolicyDoc, err := url.QueryUnescape(*output.PolicyDocument)
		if err != nil {
			zap.S().Errorf("Failed to decode policy document, %v", err)
			return false
		}
		return decodedPolicyDoc == expectedPolicy
	}
	return false
}

func policyName(identifier string) string {
	return fmt.Sprintf("substrate-node-policy-for-%s", identifier)
}
