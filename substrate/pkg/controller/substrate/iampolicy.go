package substrate

import (
	"context"
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"knative.dev/pkg/logging"
)

const (
	allowIPAttachment = `{
  "Version": "2012-10-17",
  "Statement": [
	{
	"Sid": "AllowEIPAttachment",
	"Effect": "Allow",
	"Resource": [
		"*"
	],
	"Action": [
		"ec2:AssociateAddress"
	]
	}
  ]
}`
	IPAttachmentPolicy = "AllowIPAttachment"
	ssmPolicy          = "AmazonSSMManagedInstanceCore"
	policyARNPrefix    = "arn:aws:iam::aws:policy/"
)

type iamPolicy struct {
	iamClient *iam.IAM
}

// Create will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with substrate.Status
func (i *iamPolicy) Create(ctx context.Context, substrate *v1alpha1.Substrate) error {
	for _, policy := range []policyInfo{
		&inlinePolicy{i, IPAttachmentPolicy, allowIPAttachment},
		&managedPolicy{i, ssmPolicy},
	} {
		if err := policy.attachToRole(ctx, roleName(substrate.Name)); err != nil {
			return err
		}
	}
	return nil
}

type policyInfo interface {
	attachToRole(ctx context.Context, roleName string) error
}

type inlinePolicy struct {
	*iamPolicy
	policyName string
	policyDoc  string
}

func (p *inlinePolicy) attachToRole(ctx context.Context, roleName string) error {
	output, err := p.getRolePolicy(ctx, p.policyName, roleName)
	if err != nil && !iamResourceNotFound(err) {
		return fmt.Errorf("getting role policy, %w", err)
	}
	if output != nil {
		decodedPolicyDoc, err := url.QueryUnescape(*output.PolicyDocument)
		if err != nil {
			return fmt.Errorf("Failed to decode policy document, %w", err)
		}
		if decodedPolicyDoc == p.policyDoc {
			return nil
		}
	}
	if _, err := p.iamClient.PutRolePolicyWithContext(ctx, &iam.PutRolePolicyInput{
		RoleName:       aws.String(roleName),
		PolicyName:     aws.String(p.policyName),
		PolicyDocument: aws.String(p.policyDoc),
	}); err != nil {
		return fmt.Errorf("adding inline policy to role, %w", err)
	}
	return nil
}

type managedPolicy struct {
	*iamPolicy
	policyName string
}

func (p *managedPolicy) attachToRole(ctx context.Context, roleName string) error {
	output, err := p.getRolePolicy(ctx, p.policyName, roleName)
	if err != nil && !iamResourceNotFound(err) {
		return fmt.Errorf("getting role policy, %w", err)
	}
	if output == nil || aws.StringValue(output.PolicyName) != p.policyName {
		if _, err := p.iamClient.AttachRolePolicyWithContext(ctx, &iam.AttachRolePolicyInput{
			RoleName:  aws.String(roleName),
			PolicyArn: aws.String(policyARNPrefix + p.policyName),
		}); err != nil {
			return fmt.Errorf("adding managed policy to role, %w", err)
		}
	}
	return nil
}

// Delete deletes the resource from AWS
func (i *iamPolicy) Delete(ctx context.Context, substrate *v1alpha1.Substrate) error {
	for _, policy := range []string{IPAttachmentPolicy, ssmPolicy} {
		if _, err := i.iamClient.DeleteRolePolicyWithContext(ctx, &iam.DeleteRolePolicyInput{
			RoleName:   aws.String(roleName(substrate.Name)),
			PolicyName: aws.String(policy),
		}); err != nil && !iamResourceNotFound(err) {
			return fmt.Errorf("Failed to delete role policy, %v, %w", policy, err)
		}
		logging.FromContext(ctx).Infof("Successfully removed policy %s from role %s", policy, roleName(substrate.Name))
	}
	return nil
}

func (i *iamPolicy) getRolePolicy(ctx context.Context, policy, role string) (*iam.GetRolePolicyOutput, error) {
	output, err := i.iamClient.GetRolePolicyWithContext(ctx, &iam.GetRolePolicyInput{
		PolicyName: aws.String(policy),
		RoleName:   aws.String(role),
	})
	if err != nil {
		return nil, err
	}
	return output, nil
}
