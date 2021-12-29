package substrate

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/awslabs/kit/substrate/apis/v1alpha1"
	"go.uber.org/zap"
)

type launchTemplate struct {
	ec2api *EC2
	ssm    *SSM
}

// Name returns the name of the controller
func (l *launchTemplate) resourceName() string {
	return "launch-template"
}
func (l *launchTemplate) Provision(ctx context.Context, substrate *v1alpha1.Substrate) error {
	if substrate.Status.SecurityGroupID == nil {
		return fmt.Errorf("SecurityGroup ID not found for %v", substrate.Name)
	}
	templates, err := getLaunchTemplates(ctx, l.ec2api, substrate.Name)
	if err != nil {
		return fmt.Errorf("getting launch template, %w", err)
	}
	if !existingTemplateMatchesDesired(templates, launchTemplateName(substrate.Name)) { // TODO check if existing LT is same as desired LT
		if _, err := l.createLaunchTemplate(ctx, substrate); err != nil {
			return fmt.Errorf("creating launch template, %w", err)
		}
		zap.S().Infof("Successfully created launch template %v ", launchTemplateName(substrate.Name))
		return nil
	}
	zap.S().Debugf("Successfully discovered launch template %v", launchTemplateName(substrate.Name))
	return nil
}

func (l *launchTemplate) Deprovision(ctx context.Context, substrate *v1alpha1.Substrate) error {
	templates, err := getLaunchTemplates(ctx, l.ec2api, substrate.Name)
	if err != nil {
		return fmt.Errorf("getting launch template, %w", err)
	}
	for _, template := range templates {
		_, err := l.ec2api.DeleteLaunchTemplateWithContext(ctx, &ec2.DeleteLaunchTemplateInput{
			LaunchTemplateName: template.LaunchTemplateName,
		})
		if err != nil {
			return err
		}
		zap.S().Infof("Successfully deleted launch template %v", template.LaunchTemplateName)
	}
	return nil
}

func (l *launchTemplate) createLaunchTemplate(ctx context.Context, substrate *v1alpha1.Substrate) (*ec2.CreateLaunchTemplateOutput, error) {
	paramOutput, err := l.ssm.GetParameterWithContext(ctx, &ssm.GetParameterInput{
		Name: aws.String("/aws/service/ami-amazon-linux-latest/amzn2-ami-hvm-x86_64-gp2"),
	})
	if err != nil {
		return nil, fmt.Errorf("getting ssm parameter, %w", err)
	}
	amiID := *paramOutput.Parameter.Value
	output, err := l.ec2api.CreateLaunchTemplateWithContext(ctx, &ec2.CreateLaunchTemplateInput{
		LaunchTemplateData: &ec2.RequestLaunchTemplateData{
			BlockDeviceMappings: []*ec2.LaunchTemplateBlockDeviceMappingRequest{{
				DeviceName: aws.String("/dev/xvda"),
				Ebs: &ec2.LaunchTemplateEbsBlockDeviceRequest{
					DeleteOnTermination: aws.Bool(true),
					Iops:                aws.Int64(3000),
					VolumeSize:          aws.Int64(40),
					VolumeType:          aws.String("gp3"),
				}},
			},
			KeyName:      aws.String("eks-dev-stack-key-pair"),
			InstanceType: aws.String("t2.large"),
			ImageId:      aws.String(amiID),
			IamInstanceProfile: &ec2.LaunchTemplateIamInstanceProfileSpecificationRequest{
				Name: aws.String(profileName(substrate.Name)),
			},
			Monitoring:       &ec2.LaunchTemplatesMonitoringRequest{Enabled: aws.Bool(true)},
			SecurityGroupIds: []*string{substrate.Status.SecurityGroupID},
			UserData:         aws.String(base64.StdEncoding.EncodeToString([]byte(userData))),
		},
		LaunchTemplateName: aws.String(launchTemplateName(substrate.Name)),
		TagSpecifications:  generateEC2Tags(l.resourceName(), substrate.Name),
	})
	if err != nil {
		return nil, fmt.Errorf("creating launch template, %w", err)
	}
	return output, nil
}

func existingTemplateMatchesDesired(templates []*ec2.LaunchTemplate, templateName string) bool {
	for _, template := range templates {
		if *template.LaunchTemplateName == templateName {
			return true
		}
	}
	return false
}

func getLaunchTemplates(ctx context.Context, ec2api *EC2, identifier string) ([]*ec2.LaunchTemplate, error) {
	output, err := ec2api.DescribeLaunchTemplatesWithContext(ctx, &ec2.DescribeLaunchTemplatesInput{
		Filters: ec2FilterFor(identifier),
	})
	if err != nil {
		return nil, fmt.Errorf("describing launch template, %w", err)
	}
	if len(output.LaunchTemplates) == 0 {
		return nil, nil
	}
	return output.LaunchTemplates, nil
}

func launchTemplateName(identifier string) string {
	return fmt.Sprintf("template-for-%s", identifier)
}

const (
	userData = `#!/bin/bash
sudo swapoff -a
sudo yum install -y docker
sudo yum install -y conntrack-tools
cat <<EOF | sudo tee /etc/docker/daemon.json
{
  "exec-opts": ["native.cgroupdriver=systemd"]
}
EOF

sudo systemctl enable docker
sudo systemctl daemon-reload
sudo systemctl restart docker`
)
