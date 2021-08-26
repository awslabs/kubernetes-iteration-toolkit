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
	"go.uber.org/zap"
)

const (
	TagKeyNameForAWSResources = "kit.k8s.sh/cluster-name"
)

type Controller struct {
	ec2api *awsprovider.EC2
	ssm    *awsprovider.SSM
}

// NewLaunchTemplateController returns a controller for managing LaunchTemplates in AWS
func NewController(ec2api *awsprovider.EC2, ssm *awsprovider.SSM) *Controller {
	return &Controller{ec2api: ec2api, ssm: ssm}
	// return &launchTemplate{ec2api: awsprovider.EC2Client(session), ssm: awsprovider.SSMClient(session)}
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
		zap.S().Infof("Successfully created launch templatefor cluster %v", dataplane.Spec.ClusterName)
	} else {
		zap.S().Debugf("Successfully discovered launch template for cluster %v", dataplane.Spec.ClusterName)
	}
	return nil
}
func (l *Controller) createLaunchTemplate(ctx context.Context, dataplane *v1alpha1.DataPlane) error {
	securityGroupID := "sg-0dd66a817537d3411"
	paramOutput, err := l.ssm.GetParameterWithContext(ctx, &ssm.GetParameterInput{
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
			KeyName:      aws.String("dev-account-manually-created-VMs"),
			InstanceType: aws.String("t2.xlarge"),
			ImageId:      aws.String(amiID),
			IamInstanceProfile: &ec2.LaunchTemplateIamInstanceProfileSpecificationRequest{
				// Arn:  aws.String("arn:aws:iam::674320443449:instance-profile/cluster-foo-master-instance-profile"),
				Name: aws.String("cluster-foo-master-instance-profile"),
			},
			Monitoring:       &ec2.LaunchTemplatesMonitoringRequest{Enabled: aws.Bool(true)},
			SecurityGroupIds: []*string{aws.String(securityGroupID)},
			UserData:         aws.String(base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(userData, dataplane.Spec.ClusterName, clusterca, endpoint)))),
		},
		LaunchTemplateName: aws.String(fmt.Sprintf("%s-nodes-template", dataplane.Spec.ClusterName)),
		TagSpecifications:  generateEC2Tags("launch-template", dataplane.Spec.ClusterName),
	}
	output, err := l.ec2api.CreateLaunchTemplate(input)
	if err != nil {
		return fmt.Errorf("creating launch template, %w", err)
	}
	zap.S().Infof("Launch template output is %+v", output)
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

func existingTemplateMatchesDesired(templates []*ec2.LaunchTemplate, templateName string) bool {
	for _, template := range templates {
		if *template.LaunchTemplateName == templateName {
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
		}},
	}}
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
	clusterca = `LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUM1ekNDQWMrZ0F3SUJBZ0lCQURBTkJna3Foa2lHOXcwQkFRc0ZBREFWTVJNd0VRWURWUVFERXdwcmRXSmwKY201bGRHVnpNQjRYRFRJeE1EZ3lOVEUxTURNeU5Gb1hEVE14TURneU16RTFNRE15TkZvd0ZURVRNQkVHQTFVRQpBeE1LYTNWaVpYSnVaWFJsY3pDQ0FTSXdEUVlKS29aSWh2Y05BUUVCQlFBRGdnRVBBRENDQVFvQ2dnRUJBTjhwCkQ4aE9RUi9sRjZmcDBmMXdCcWlZSmh2MjR2ZEhlVlZ2RzFJUDZrRUpPaGs0aEVwQjB5ekVnK1JZeFY0aTMzYmQKekQ2U1dyYUpoL051L0h3S2ZzOTVEY20vSTBZbE1GMjJDckJCUDZVVUFLc2pXak14STZidFB3Qk93dytwUGRhTAphc0tWSVNQSG9KTXNNaU55SWNoUk96dkJEbC8yUUc0TVJwdTcxTXM1VkxXNmJwQlVYK3NMWVVRREJnNk5sTlZoCmJOVUZlWi9EU0lDaXZuQ255Z2dkWGVVSEM2d0JjeGlURGVRRzBPRWgydzdLYWs2R29XbEhsR084OTJ3UkM1eTEKUEpiME5zN3hQVnZ5WjhKZTNlMXZGaHAvcHVBc2RqTlhuSlBhYWFoTlN1VURKazVBQk55b0NyVW9xNk1Nb1VPdQp4M2hCblpMcjlJTmtwa0FjZnJrQ0F3RUFBYU5DTUVBd0RnWURWUjBQQVFIL0JBUURBZ0trTUE4R0ExVWRFd0VCCi93UUZNQU1CQWY4d0hRWURWUjBPQkJZRUZPUjlkT0tNdTZoVnRYZE1iYjRQOWJJc0xIQTNNQTBHQ1NxR1NJYjMKRFFFQkN3VUFBNElCQVFCTU9kbWZlSHBpeEZBWHpBZTNUUWRtRHZDbEVQeEVBMEpzZWEzQWZMTzAyQU51aVY5OQozZEF5cUJRSlY1UFhWZEM1bjduUFhQT0xLSytkV0FYNG44eEhqOFNuUEdoQUFqb1JDQlJoUHZYRXEvMVdGNEZpCmhvOEpLTlZnaHoxZkk4K01CTGdBWXFhVy9rZlB1ZTUzRVZmeVpNRllOMC9uREF1Tno3MEJNRnpNOTBkb0ZTZU4KV1pGQmZrdGNEcS8rR1lvMEJlR0tYRE15RUptNlk2dU5RVWJHN1puaWJMcnFmd2h4aXhEVHpIZmM4dkNqM1QrQwo2emMrYU1yWGQ5YlI4bDVpQzlnQUlvQUpFeFJSZHAxZW8vYUNWNmYxQkdqakw3VnJ2VC85cGFxaUQ3alZXcXBxCkJzSkI3YjJNNkJoTm9WU0djb3pzN2FQRmVjWDNBTDY5dzdxZAotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg==`
	endpoint  = "k8s-default-examplec-0a133f4cf3-75fe4dd366a0459d.elb.us-west-2.amazonaws.com"
)
