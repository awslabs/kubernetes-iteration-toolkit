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
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/awsprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/controllers"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/errors"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/status"
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
	return &v1alpha1.LaunchTemplate{}
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (l *launchTemplate) Reconcile(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	ltObj := object.(*v1alpha1.LaunchTemplate)
	templates, err := l.getLaunchTemplates(ctx, ltObj.Spec.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("getting launch template, %w", err)
	}
	if !existingTemplateMatchesDesired(templates, ltObj.Name) { // TODO check if existing LT is same as desired LT
		if _, err := l.createLaunchTemplate(ctx, ltObj); err != nil {
			return nil, fmt.Errorf("creating launch template, %w", err)
		}
		zap.S().Infof("Successfully created launch template %v for cluster %v", ltObj.Name, ltObj.Spec.ClusterName)
	} else {
		zap.S().Debugf("Successfully discovered launch template %v for cluster %v", ltObj.Name, ltObj.Spec.ClusterName)
	}
	return status.Created, nil
}

// Finalize deletes the resource from AWS
func (l *launchTemplate) Finalize(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	ltObj := object.(*v1alpha1.LaunchTemplate)
	launchTemplates, err := l.getLaunchTemplates(ctx, ltObj.Spec.ClusterName)
	if err != nil {
		return nil, fmt.Errorf("getting launch template, %w", err)
	}
	for _, launchTemplate := range launchTemplates {
		if aws.StringValue(launchTemplate.LaunchTemplateName) == ltObj.Name {
			_, err := l.ec2api.DeleteLaunchTemplateWithContext(ctx, &ec2.DeleteLaunchTemplateInput{
				LaunchTemplateName: launchTemplate.LaunchTemplateName,
			})
			if err != nil {
				return nil, err
			}
			zap.S().Infof("Successfully deleted launch template %v for cluster %v", ltObj.Name, ltObj.Spec.ClusterName)
		}
	}
	return status.Terminated, nil
}

func (l *launchTemplate) createLaunchTemplate(ctx context.Context, launchTemplate *v1alpha1.LaunchTemplate) (*ec2.CreateLaunchTemplateOutput, error) {
	// Get Security group
	securityGroupID := l.desiredSecurityGroupID(ctx, launchTemplate.Name, launchTemplate.Spec.ClusterName)
	if securityGroupID == "" {
		return nil, fmt.Errorf("waiting for security group, %w", errors.WaitingForSubResources)
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
				Name: aws.String(v1alpha1.ProfileName(launchTemplate.Name)),
			},
			Monitoring:       &ec2.LaunchTemplatesMonitoringRequest{Enabled: aws.Bool(true)},
			SecurityGroupIds: []*string{aws.String(securityGroupID)},
			UserData:         aws.String(base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(userData, launchTemplate.Spec.ClusterName)))),
		},
		LaunchTemplateName: aws.String(launchTemplate.Name),
		TagSpecifications:  generateEC2Tags(l.Name(), launchTemplate.Spec.ClusterName),
	}
	output, err := l.ec2api.CreateLaunchTemplate(input)
	if err != nil {
		return nil, fmt.Errorf("creating launch template, %w", err)
	}
	return output, nil
}

func (l *launchTemplate) desiredSecurityGroupID(ctx context.Context, templateName, clusterName string) string {
	securityGroups, err := getSecurityGroups(ctx, l.ec2api, clusterName)
	if err != nil || len(securityGroups) == 0 {
		return ""
	}
	desiredGroupName := fmt.Sprintf("%s-security-group", templateName)
	for _, group := range securityGroups {
		if aws.StringValue(group.GroupName) == desiredGroupName {
			return aws.StringValue(group.GroupId)
		}
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
sudo systemctl restart docker

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

ETCD_VER=v3.4.16
GITHUB_URL=https://github.com/etcd-io/etcd/releases/download
DOWNLOAD_URL=${GITHUB_URL}

rm -f /tmp/etcd-${ETCD_VER}-linux-amd64.tar.gz
rm -rf /tmp/etcd-download-test && mkdir -p /tmp/etcd-download-test

curl -L ${DOWNLOAD_URL}/${ETCD_VER}/etcd-${ETCD_VER}-linux-amd64.tar.gz -o /tmp/etcd-${ETCD_VER}-linux-amd64.tar.gz
tar xzvf /tmp/etcd-${ETCD_VER}-linux-amd64.tar.gz -C /tmp/etcd-download-test --strip-components=1
rm -f /tmp/etcd-${ETCD_VER}-linux-amd64.tar.gz
/tmp/etcd-download-test/etcdctl version

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
KUBELET_KUBEADM_ARGS="--cgroup-driver=systemd --network-plugin=cni --pod-infra-container-image=public.ecr.aws/eks-distro/kubernetes/pause:3.2"
EOF

sudo systemctl enable --now kubelet

sudo mkdir -p /etc/systemd/system/kubelet.service.d

systemctl daemon-reload
systemctl restart kubelet

sudo mkdir -p /etc/kit/
cat <<EOF | sudo tee /etc/kit/sync.sh
#!/bin/env bash
while [ true ]; do
 dirs=("/etc/systemd/system/kubelet.service.d" "/etc/kubernetes" "/var/lib/kubelet")
 INSTANCE_ID=$(curl -s http://169.254.169.254/latest/meta-data/instance-id)
 CLUSTERNAME=%s
 for dir in "\${dirs[@]}"; do
    echo "\$(date) Syncing S3 files for \$dir"
    mkdir -p \$dir
    existing_checksum=\$(ls -alR \$dir | md5sum)
    aws s3 sync s3://kit-\$CLUSTERNAME/tmp/\$CLUSTERNAME/\$INSTANCE_ID"\$dir" "\$dir"
    new_checksum=\$(ls -alR \$dir | md5sum)
    if [ "\$new_checksum" != "\$existing_checksum" ]; then
		echo "Successfully synced from S3 \$dir"
	  	echo "Restarting Kubelet service" 
	   	systemctl daemon-reload
	   	systemctl restart kubelet
    fi
 done
 sleep 10
done
EOF

chmod a+x /etc/kit/sync.sh
/etc/kit/sync.sh > /tmp/sync-kit-files.log&`
)
