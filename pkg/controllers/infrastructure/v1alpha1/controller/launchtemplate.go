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
	"encoding/base64"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/prateekgogia/kit/pkg/apis/infrastructure/v1alpha1"
	"github.com/prateekgogia/kit/pkg/awsprovider"
	"github.com/prateekgogia/kit/pkg/controllers"
	"github.com/prateekgogia/kit/pkg/status"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	MasterInstanceLaunchTemplateName = "master-instance-launch-template-cluster-%s"
	ETCDInstanceLaunchTemplateName   = "etcd-instance-launch-template-cluster-%s"
)

type launchTemplate struct {
	ec2api *awsprovider.EC2
	ssm    *awsprovider.SSM
}

// NewLaunchTemplateController returns a controller for managing LaunchTemplates in AWS
func NewLaunchTemplateController(ec2api *awsprovider.EC2, ssm *awsprovider.SSM) *launchTemplate {
	return &launchTemplate{ec2api: ec2api, ssm: ssm}
}

// Name returns the name of the controller
func (l *launchTemplate) Name() string {
	return "launch-template"
}

// For returns the resource this controller is for.
func (l *launchTemplate) For() controllers.Object {
	return &v1alpha1.ControlPlane{}
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (l *launchTemplate) Reconcile(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	controlPlane := object.(*v1alpha1.ControlPlane)
	templates, err := l.getLaunchTemplates(ctx, controlPlane.Name)
	if err != nil {
		return nil, fmt.Errorf("getting launch template, %w", err)
	}
	for _, templateName := range l.desiredLaunchTemplates(controlPlane.Name) {
		if existingTemplateMatchesDesired(templates, templateName) { // TODO check if existing LT is same as desired LT
			zap.S().Debugf("Successfully discovered launch template %v for cluster %v", templateName, controlPlane.Name)
			continue
		}
		if _, err := l.createLaunchTemplate(ctx, templateName, controlPlane.Name); err != nil {
			return nil, fmt.Errorf("creating launch template, %w", err)
		}
		zap.S().Infof("Successfully created launch template %v for cluster %v", templateName, controlPlane.Name)
	}
	return status.Created, nil
}

// Finalize deletes the resource from AWS
func (l *launchTemplate) Finalize(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	controlPlane := object.(*v1alpha1.ControlPlane)
	launchTemplates, err := l.getLaunchTemplates(ctx, controlPlane.Name)
	if err != nil {
		return nil, fmt.Errorf("getting launch template, %w", err)
	}
	for _, launchTemplate := range launchTemplates {
		_, err := l.ec2api.DeleteLaunchTemplateWithContext(ctx, &ec2.DeleteLaunchTemplateInput{
			LaunchTemplateName: launchTemplate.LaunchTemplateName,
		})
		if err != nil {
			return nil, err
		}
		zap.S().Infof("Successfully deleted launch template %v for cluster %v", *launchTemplate.LaunchTemplateName, controlPlane.Name)
	}
	return status.Terminated, nil
}

func (l *launchTemplate) createLaunchTemplate(ctx context.Context, templateName, clusterName string) (*ec2.CreateLaunchTemplateOutput, error) {
	// Get Security group
	securityGroupID := l.desiredSecurityGroupID(ctx, templateName, clusterName)
	// Get IAM instance profile ARN
	instanceProfileName := l.desiredInstanceProfileName(templateName, clusterName)
	if securityGroupID == "" || instanceProfileName == "" {
		return nil, fmt.Errorf("failed to find security group and profile ")
	}
	paramOutput, err := l.ssm.GetParameterWithContext(ctx, &ssm.GetParameterInput{
		Name: aws.String("/aws/service/ami-amazon-linux-latest/amzn2-ami-hvm-x86_64-gp2"),
	})
	if err != nil {
		return nil, fmt.Errorf("getting ssm parameter, %w", err)
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
			InstanceType: aws.String("t2.large"),
			ImageId:      aws.String(amiID),
			IamInstanceProfile: &ec2.LaunchTemplateIamInstanceProfileSpecificationRequest{
				// Arn:  aws.String("arn:aws:iam::674320443449:instance-profile/cluster-foo-master-instance-profile"),
				Name: aws.String(fmt.Sprintf(instanceProfileName, clusterName)),
			},
			Monitoring: &ec2.LaunchTemplatesMonitoringRequest{Enabled: aws.Bool(true)},
			// SecurityGroupIds: []*string{aws.String("sg-0c44ff3e16005d370")},
			SecurityGroupIds: []*string{aws.String(securityGroupID)},
			UserData:         aws.String(base64.StdEncoding.EncodeToString([]byte(userData))),
		},
		LaunchTemplateName: aws.String(templateName),
		TagSpecifications:  generateEC2Tags(l.Name(), clusterName),
	}
	output, err := l.ec2api.CreateLaunchTemplate(input)
	if err != nil {
		return nil, fmt.Errorf("creating launch template, %w", err)
	}
	return output, nil
}

func (l *launchTemplate) desiredLaunchTemplates(clusterName string) []string {
	return []string{
		fmt.Sprintf(MasterInstanceLaunchTemplateName, clusterName),
		fmt.Sprintf(ETCDInstanceLaunchTemplateName, clusterName),
	}
}

func (l *launchTemplate) desiredSecurityGroupID(ctx context.Context, templateName, clusterName string) string {
	securityGroups, err := getSecurityGroups(ctx, l.ec2api, clusterName)
	if err != nil || len(securityGroups) == 0 {
		return ""
	}
	switch templateName {
	case fmt.Sprintf(MasterInstanceLaunchTemplateName, clusterName):
		for _, group := range securityGroups {
			if *group.GroupName == masterInstancesGroupName {
				return *group.GroupId
			}
		}
	case fmt.Sprintf(ETCDInstanceLaunchTemplateName, clusterName):
		for _, group := range securityGroups {
			if *group.GroupName == etcdInstancesGroupName {
				return *group.GroupId
			}
		}
	}
	return ""
}

// func (l *launchTemplate) desiredSecurityGroupName(ctx context.Context, templateName, clusterName string) string {
// 	switch templateName {
// 	case fmt.Sprintf(MasterInstanceLaunchTemplateName, clusterName):
// 		return masterInstancesGroupName
// 	case fmt.Sprintf(ETCDInstanceLaunchTemplateName, clusterName):
// 		return etcdInstancesGroupName
// 	}
// 	return ""
// }

func (l *launchTemplate) desiredInstanceProfileName(templateName string, clusterName string) string {
	switch templateName {
	case fmt.Sprintf(MasterInstanceLaunchTemplateName, clusterName):
		return MasterInstanceProfileName
	case fmt.Sprintf(ETCDInstanceLaunchTemplateName, clusterName):
		return ETCDInstanceProfileName
	}
	return ""
}

func (l *launchTemplate) getLaunchTemplates(ctx context.Context, clusterName string) ([]*ec2.LaunchTemplate, error) {
	return getLaunchTemplates(ctx, l.ec2api, clusterName)
}

func getLaunchTemplates(ctx context.Context, ec2api *awsprovider.EC2, clusterName string) ([]*ec2.LaunchTemplate, error) {
	output, err := ec2api.DescribeLaunchTemplatesWithContext(ctx, &ec2.DescribeLaunchTemplatesInput{
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

const (
	userData = `#!/bin/bash
yum install -y https://s3.amazonaws.com/ec2-downloads-windows/SSMAgent/latest/linux_amd64/amazon-ssm-agent.rpm

sudo swapoff -a
sudo yum install -y docker
sudo yum install -y conntrack-tools
sudo systemctl start docker
sudo systemctl enable docker

# Pull and tag Docker images
sudo docker pull public.ecr.aws/eks-distro/kubernetes/pause:v1.18.9-eks-1-18-1;\
sudo docker pull public.ecr.aws/eks-distro/coredns/coredns:v1.7.0-eks-1-18-1; \
sudo docker pull public.ecr.aws/eks-distro/etcd-io/etcd:v3.4.14-eks-1-18-1; \
sudo docker tag public.ecr.aws/eks-distro/kubernetes/pause:v1.18.9-eks-1-18-1 public.ecr.aws/eks-distro/kubernetes/pause:3.2; \
sudo docker tag public.ecr.aws/eks-distro/coredns/coredns:v1.7.0-eks-1-18-1 public.ecr.aws/eks-distro/kubernetes/coredns:1.6.7; \
sudo docker tag public.ecr.aws/eks-distro/etcd-io/etcd:v3.4.14-eks-1-18-1 public.ecr.aws/eks-distro/kubernetes/etcd:3.4.3-0

# Set SELinux in permissive mode (effectively disabling it)
sudo setenforce 0
sudo sed -i 's/^SELINUX=enforcing$/SELINUX=permissive/' /etc/selinux/config

cat <<EOF | sudo tee /etc/modules-load.d/k8s.conf
br_netfilter
EOF

cat <<EOF | sudo tee /etc/sysctl.d/k8s.conf
net.bridge.bridge-nf-call-ip6tables = 1
net.bridge.bridge-nf-call-iptables = 1
EOF
sudo sysctl --system

cat <<EOF | sudo tee /etc/yum.repos.d/kubernetes.repo
[kubernetes]
name=Kubernetes
baseurl=https://packages.cloud.google.com/yum/repos/kubernetes-el7-\$basearch
enabled=1
gpgcheck=1
repo_gpgcheck=1
gpgkey=https://packages.cloud.google.com/yum/doc/yum-key.gpg https://packages.cloud.google.com/yum/doc/rpm-package-key.gpg
exclude=kubelet kubeadm kubectl
EOF

sudo yum install -y kubelet kubeadm kubectl --disableexcludes=kubernetes

cd /usr/bin
sudo rm kubelet kubeadm kubectl
sudo wget https://distro.eks.amazonaws.com/kubernetes-1-18/releases/1/artifacts/kubernetes/v1.18.9/bin/linux/amd64/kubelet; \
sudo wget https://distro.eks.amazonaws.com/kubernetes-1-18/releases/1/artifacts/kubernetes/v1.18.9/bin/linux/amd64/kubeadm; \
sudo wget https://distro.eks.amazonaws.com/kubernetes-1-18/releases/1/artifacts/kubernetes/v1.18.9/bin/linux/amd64/kubectl
chmod +x kubeadm kubectl kubelet

sudo mkdir -p /var/lib/kubelet
cat > /var/lib/kubelet/kubeadm-flags.env <<EOF
KUBELET_KUBEADM_ARGS="--cgroup-driver=systemd —network-plugin=cni —pod-infra-container-image=public.ecr.aws/eks-distro/kubernetes/pause:3.2"
EOF

sudo systemctl enable --now kubelet`
)
