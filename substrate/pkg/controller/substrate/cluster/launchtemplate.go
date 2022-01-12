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

package cluster

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kit/substrate/pkg/utils/discovery"
	"github.com/mitchellh/hashstructure/v2"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type LaunchTemplate struct {
	EC2 *ec2.EC2
	SSM *ssm.SSM
}

func (l *LaunchTemplate) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if substrate.Status.SecurityGroupID == nil {
		return reconcile.Result{Requeue: true}, nil
	}
	parameterOutput, err := l.SSM.GetParameterWithContext(ctx, &ssm.GetParameterInput{Name: aws.String("/aws/service/ami-amazon-linux-latest/amzn2-ami-hvm-arm64-gp2")})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("getting ssm parameter, %w", err)
	}
	launchTemplateData := &ec2.RequestLaunchTemplateData{
		BlockDeviceMappings: []*ec2.LaunchTemplateBlockDeviceMappingRequest{{
			DeviceName: aws.String("/dev/xvda"),
			Ebs: &ec2.LaunchTemplateEbsBlockDeviceRequest{
				DeleteOnTermination: aws.Bool(true),
				Iops:                aws.Int64(3000),
				VolumeSize:          aws.Int64(40),
				VolumeType:          aws.String("gp3"),
			}},
		},
		InstanceType:       substrate.Spec.InstanceType,
		ImageId:            parameterOutput.Parameter.Value,
		IamInstanceProfile: &ec2.LaunchTemplateIamInstanceProfileSpecificationRequest{Name: discovery.Name(substrate)},
		Monitoring:         &ec2.LaunchTemplatesMonitoringRequest{Enabled: aws.Bool(true)},
		SecurityGroupIds:   []*string{substrate.Status.SecurityGroupID},
		UserData: aws.String(base64.StdEncoding.EncodeToString([]byte(`#!/bin/bash
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
			sudo systemctl restart docker`,
		))),
	}
	if _, err := l.EC2.CreateLaunchTemplateWithContext(ctx, &ec2.CreateLaunchTemplateInput{
		LaunchTemplateName: discovery.Name(substrate),
		TagSpecifications:  discovery.Tags(substrate, ec2.ResourceTypeLaunchTemplate, discovery.Name(substrate)),
		LaunchTemplateData: launchTemplateData,
	}); err != nil {
		if err.(awserr.Error).Code() != "InvalidLaunchTemplateName.AlreadyExistsException" {
			return reconcile.Result{}, fmt.Errorf("creating launch template, %w", err)
		}
		logging.FromContext(ctx).Infof("Found launch template %s", aws.StringValue(discovery.Name(substrate)))
	} else {
		logging.FromContext(ctx).Infof("Created launch template %s", aws.StringValue(discovery.Name(substrate)))
	}
	// Only update the launch template if it's changed
	hash, err := hashstructure.Hash(launchTemplateData, hashstructure.FormatV2, &hashstructure.HashOptions{SlicesAsSets: true})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("hashing launch template, %w", err)
	}
	launchTemplateVersionOutput, err := l.EC2.CreateLaunchTemplateVersionWithContext(ctx, &ec2.CreateLaunchTemplateVersionInput{
		ClientToken:        aws.String(fmt.Sprint(hash)),
		LaunchTemplateName: discovery.Name(substrate),
		LaunchTemplateData: launchTemplateData,
	})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("creating launch template version, %w", err)
	}
	substrate.Status.Cluster.LaunchTemplateVersion = aws.String(fmt.Sprint(aws.Int64Value(launchTemplateVersionOutput.LaunchTemplateVersion.VersionNumber)))
	logging.FromContext(ctx).Infof("Created launch template version %s for %s", aws.StringValue(substrate.Status.Cluster.LaunchTemplateVersion), aws.StringValue(discovery.Name(substrate)))
	return reconcile.Result{}, nil
}

func (l *LaunchTemplate) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	launchTemplatesOutput, err := l.EC2.DescribeLaunchTemplatesWithContext(ctx, &ec2.DescribeLaunchTemplatesInput{Filters: discovery.Filters(substrate, discovery.Name(substrate))})
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("describing launch templates, %w", err)
	}
	if len(launchTemplatesOutput.LaunchTemplates) == 0 {
		return reconcile.Result{}, nil
	}
	for _, launchTemplate := range launchTemplatesOutput.LaunchTemplates {
		if _, err := l.EC2.DeleteLaunchTemplateWithContext(ctx, &ec2.DeleteLaunchTemplateInput{LaunchTemplateId: launchTemplate.LaunchTemplateId}); err != nil {
			return reconcile.Result{}, fmt.Errorf("deleting launch template, %w", err)
		}
		logging.FromContext(ctx).Infof("Deleted launch template %v", aws.StringValue(launchTemplate.LaunchTemplateId))
	}
	return reconcile.Result{}, nil
}
