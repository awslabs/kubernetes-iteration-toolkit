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

package launchtemplate

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/awslabs/kit/operator/pkg/apis/dataplane/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/awsprovider"
	"github.com/awslabs/kit/operator/pkg/awsprovider/securitygroup"
	"go.uber.org/zap"
)

const (
	TagKeyNameForAWSResources = "kit.k8s.sh/cluster-name"
)

type Controller struct {
	ec2api        *awsprovider.EC2
	ssm           *awsprovider.SSM
	securityGroup *securitygroup.Provider
}

// NewController returns a controller for managing LaunchTemplates in AWS
func NewController(ec2api *awsprovider.EC2, ssm *awsprovider.SSM, sg *securitygroup.Provider) *Controller {
	return &Controller{ec2api: ec2api, ssm: ssm, securityGroup: sg}
}

func (l *Controller) Reconcile(ctx context.Context, dataplane *v1alpha1.DataPlane) error {
	// get launch template
	templates, err := l.getLaunchTemplates(ctx, dataplane.Spec.ClusterName)
	if err != nil {
		return fmt.Errorf("getting launch template, %w", err)
	}
	if !existingTemplateMatchesDesired(templates, dataplane.Spec.ClusterName) { // TODO check if existing LT is same as desired LT
		// if not present create launch template
		if err := l.createLaunchTemplate(ctx, dataplane); err != nil {
			return fmt.Errorf("creating launch template, %w", err)
		}
		zap.S().Infof("Created launch template for cluster %v", dataplane.Spec.ClusterName)
		return nil
	}
	return nil
}

func (c *Controller) createLaunchTemplate(ctx context.Context, dataplane *v1alpha1.DataPlane) error {
	securityGroupID, err := c.securityGroup.For(ctx, dataplane.Spec.ClusterName)
	if err != nil {
		return fmt.Errorf("getting security group for control plane nodes, %w", err)
	}
	paramOutput, err := c.ssm.GetParameterWithContext(ctx, &ssm.GetParameterInput{
		Name: aws.String("/aws/service/eks/optimized-ami/1.20/amazon-linux-2/recommended/image_id"),
	})
	if err != nil {
		return fmt.Errorf("getting ssm parameter, %w", err)
	}
	amiID := *paramOutput.Parameter.Value
	input := &ec2.CreateLaunchTemplateInput{
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
			InstanceType: aws.String("t2.xlarge"),
			ImageId:      aws.String(amiID),
			// TODO Get the right instance profile
			IamInstanceProfile: &ec2.LaunchTemplateIamInstanceProfileSpecificationRequest{
				// Arn:  aws.String("arn:aws:iam::674320443449:instance-profile/cluster-foo-master-instance-profile"),
				Name: aws.String("cluster-foo-master-instance-profile"),
			},
			Monitoring:       &ec2.LaunchTemplatesMonitoringRequest{Enabled: aws.Bool(true)},
			SecurityGroupIds: []*string{aws.String(securityGroupID)},
			UserData:         aws.String(base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(userData, dataplane.Spec.ClusterName, clusterca, endpoint)))),
		},
		LaunchTemplateName: aws.String(templateName(dataplane.Spec.ClusterName)),
		TagSpecifications:  generateEC2Tags("launch-template", dataplane.Spec.ClusterName),
	}
	if _, err := c.ec2api.CreateLaunchTemplate(input); err != nil {
		return fmt.Errorf("creating launch template, %w", err)
	}
	return nil
}

func (l *Controller) getLaunchTemplates(ctx context.Context, clusterName string) ([]*ec2.LaunchTemplate, error) {
	output, err := l.ec2api.DescribeLaunchTemplatesWithContext(ctx, &ec2.DescribeLaunchTemplatesInput{
		Filters: ec2FilterFor(clusterName),
	})
	if err != nil {
		return nil, fmt.Errorf("describing launch template, %w", err)
	}
	if len(output.LaunchTemplates) == 0 {
		return nil, nil
	}
	return output.LaunchTemplates, nil
}

func existingTemplateMatchesDesired(templates []*ec2.LaunchTemplate, clusterName string) bool {
	for _, template := range templates {
		if *template.LaunchTemplateName == templateName(clusterName) {
			return true
		}
	}
	return false
}

func ec2FilterFor(clusterName string) []*ec2.Filter {
	return []*ec2.Filter{{
		Name:   aws.String(fmt.Sprintf("tag:%s", TagKeyNameForAWSResources)),
		Values: []*string{aws.String(clusterName)},
	}}
}

func generateEC2Tags(svcName, clusterName string) []*ec2.TagSpecification {
	return []*ec2.TagSpecification{{
		ResourceType: aws.String(svcName),
		Tags: []*ec2.Tag{{
			Key:   aws.String(TagKeyNameForAWSResources),
			Value: aws.String(clusterName),
		}, {
			Key:   aws.String("Name"),
			Value: aws.String(fmt.Sprintf("%s-%s", clusterName, svcName)),
		}, {
			Key:   aws.String(fmt.Sprintf("kubernetes.io/cluster/%s", clusterName)),
			Value: aws.String("owned"),
		}},
	}}
}

func templateName(clusterName string) string {
	return fmt.Sprintf("kit-%s-cluster-nodes", clusterName)
}

var (
	userData = `
#!/bin/bash
yum install -y https://s3.amazonaws.com/ec2-downloads-windows/SSMAgent/latest/linux_amd64/amazon-ssm-agent.rpm
/etc/eks/bootstrap.sh %s \
	--kubelet-extra-args '--node-labels=kit.sh/provisioned=true' \
	--b64-cluster-ca %s \
	--apiserver-endpoint %s`
)

var (
	clusterca = ``
	endpoint  = ``
)
