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
	cpv1alpha1 "github.com/awslabs/kit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/apis/dataplane/v1alpha1"
	"github.com/awslabs/kit/operator/pkg/awsprovider"
	"github.com/awslabs/kit/operator/pkg/awsprovider/iam"
	"github.com/awslabs/kit/operator/pkg/awsprovider/securitygroup"
	"github.com/awslabs/kit/operator/pkg/controllers/master"
	"github.com/awslabs/kit/operator/pkg/errors"
	"github.com/awslabs/kit/operator/pkg/kubeprovider"
	"github.com/awslabs/kit/operator/pkg/utils/keypairs"
	"github.com/awslabs/kit/operator/pkg/utils/object"
	"github.com/awslabs/kit/operator/pkg/utils/secrets"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
	"knative.dev/pkg/ptr"
)

const (
	TagKeyNameForAWSResources = "kit.k8s.sh/cluster-name"
	// TODO https://github.com/awslabs/kubernetes-iteration-toolkit/issues/61
	dnsClusterIP = "10.100.0.10"
)

type Controller struct {
	ec2api     *awsprovider.EC2
	ssm        *awsprovider.SSM
	kubeclient *kubeprovider.Client
}

// NewController returns a controller for managing LaunchTemplates in AWS
func NewController(ec2api *awsprovider.EC2, ssm *awsprovider.SSM, client *kubeprovider.Client) *Controller {
	return &Controller{ec2api: ec2api, ssm: ssm, kubeclient: client}
}

func (c *Controller) Reconcile(ctx context.Context, dataplane *v1alpha1.DataPlane) error {
	// get launch template
	templates, err := c.getLaunchTemplates(ctx, dataplane.Spec.ClusterName)
	if err != nil && !errors.IsLaunchTemplateDoNotExist(err) {
		return fmt.Errorf("getting launch template, %w", err)
	}
	if !existingTemplateMatchesDesired(templates, dataplane.Spec.ClusterName) { // TODO check if existing LT is same as desired LT
		// create launch template
		if err := c.createLaunchTemplate(ctx, dataplane); err != nil {
			return fmt.Errorf("creating launch template, %w", err)
		}
		zap.S().Infof("[%s] Created launch template", dataplane.Spec.ClusterName)
		return nil
	}
	return nil
}

func (c *Controller) Finalize(ctx context.Context, dataplane *v1alpha1.DataPlane) error {
	return c.deleteLaunchTemplate(ctx, TemplateName(dataplane.Spec.ClusterName))
}

func (c *Controller) deleteLaunchTemplate(ctx context.Context, templateName string) error {
	if _, err := c.ec2api.DeleteLaunchTemplateWithContext(ctx, &ec2.DeleteLaunchTemplateInput{
		LaunchTemplateName: ptr.String(templateName),
	}); err != nil {
		if errors.IsLaunchTemplateDoNotExist(err) {
			return nil
		}
		return fmt.Errorf("deleting launch template, %w", err)
	}
	return nil
}

func (c *Controller) createLaunchTemplate(ctx context.Context, dataplane *v1alpha1.DataPlane) error {
	// Currently, we get the same security group assigned to control plane instances
	// At some point, we will be creating dataplane specific security groups
	securityGroupID, err := securitygroup.New(c.ec2api, c.kubeclient).For(ctx, dataplane.Spec.ClusterName)
	if err != nil {
		return fmt.Errorf("getting security group for control plane nodes, %w", err)
	}
	clusterEndpoint, err := master.GetClusterEndpoint(ctx, c.kubeclient, types.NamespacedName{dataplane.Namespace, dataplane.Spec.ClusterName})
	if err != nil {
		return fmt.Errorf("getting cluster endpoint, %w", err)
	}
	caSecret, err := keypairs.Reconciler(c.kubeclient).GetSecretFromServer(ctx,
		object.NamespacedName(master.RootCASecretNameFor(dataplane.Spec.ClusterName), dataplane.Namespace))
	if err != nil {
		return fmt.Errorf("getting control plane ca certificate, %w", err)
	}
	_, clusterCA := secrets.Parse(caSecret)
	amiID, err := c.amiID(ctx, dataplane)
	if err != nil {
		return fmt.Errorf("getting ami id for worker nodes, %w", err)
	}
	input := &ec2.CreateLaunchTemplateInput{
		LaunchTemplateData: &ec2.RequestLaunchTemplateData{
			BlockDeviceMappings: []*ec2.LaunchTemplateBlockDeviceMappingRequest{{
				DeviceName: ptr.String("/dev/xvda"),
				Ebs: &ec2.LaunchTemplateEbsBlockDeviceRequest{
					DeleteOnTermination: ptr.Bool(true),
					Iops:                ptr.Int64(3000),
					VolumeSize:          ptr.Int64(40),
					VolumeType:          ptr.String("gp3"),
				}},
			},
			InstanceType: ptr.String("t2.xlarge"), // TODO get this from dataplane spec
			ImageId:      ptr.String(amiID),
			IamInstanceProfile: &ec2.LaunchTemplateIamInstanceProfileSpecificationRequest{
				Name: aws.String(iam.KitNodeInstanceProfileNameFor(dataplane.Spec.ClusterName)),
			},
			Monitoring:       &ec2.LaunchTemplatesMonitoringRequest{Enabled: ptr.Bool(true)},
			SecurityGroupIds: []*string{ptr.String(securityGroupID)},
			UserData: ptr.String(base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf(userData,
				dataplane.Spec.ClusterName, v1alpha1.SchemeGroupVersion.Group, dnsClusterIP, base64.StdEncoding.EncodeToString(clusterCA), clusterEndpoint)))),
		},
		LaunchTemplateName: ptr.String(TemplateName(dataplane.Spec.ClusterName)),
		TagSpecifications:  generateEC2Tags("launch-template", dataplane.Spec.ClusterName),
	}
	if _, err := c.ec2api.CreateLaunchTemplate(input); err != nil {
		return fmt.Errorf("creating launch template, %w", err)
	}
	return nil
}

func (c *Controller) amiID(ctx context.Context, dataplane *v1alpha1.DataPlane) (string, error) {
	kubeVersion, err := c.desiredKubernetesVersion(ctx, dataplane)
	if err != nil {
		return "", fmt.Errorf("getting kubernetes version, %w", err)
	}
	paramOutput, err := c.ssm.GetParameterWithContext(ctx, &ssm.GetParameterInput{
		Name: ptr.String(fmt.Sprintf("/aws/service/eks/optimized-ami/%s/amazon-linux-2/recommended/image_id", kubeVersion)),
	})
	if err != nil {
		return "", fmt.Errorf("getting ssm parameter, %w", err)
	}
	return *paramOutput.Parameter.Value, nil
}

func (c *Controller) desiredKubernetesVersion(ctx context.Context, dataplane *v1alpha1.DataPlane) (string, error) {
	cp := &cpv1alpha1.ControlPlane{}
	if err := c.kubeclient.Get(ctx, types.NamespacedName{dataplane.GetNamespace(), dataplane.Spec.ClusterName}, cp); err != nil {
		return "", fmt.Errorf("getting control plane object, %w", err)
	}
	return cp.Spec.KubernetesVersion, nil
}

func (c *Controller) getLaunchTemplates(ctx context.Context, clusterName string) ([]*ec2.LaunchTemplate, error) {
	output, err := c.ec2api.DescribeLaunchTemplatesWithContext(ctx, &ec2.DescribeLaunchTemplatesInput{
		LaunchTemplateNames: []*string{ptr.String(TemplateName(clusterName))},
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
		if aws.StringValue(template.LaunchTemplateName) == TemplateName(clusterName) {
			return true
		}
	}
	return false
}

func generateEC2Tags(svcName, clusterName string) []*ec2.TagSpecification {
	return []*ec2.TagSpecification{{
		ResourceType: ptr.String(svcName),
		Tags: []*ec2.Tag{{
			Key:   ptr.String(TagKeyNameForAWSResources),
			Value: ptr.String(clusterName),
		}, {
			Key:   ptr.String("Name"),
			Value: ptr.String(TemplateName(clusterName)),
		}, {
			Key:   ptr.String(fmt.Sprintf("kubernetes.io/cluster/%s", clusterName)),
			Value: ptr.String("owned"),
		}},
	}}
}

func TemplateName(clusterName string) string {
	return fmt.Sprintf("kit-%s-cluster-nodes", clusterName)
}

var (
	userData = `
#!/bin/bash
yum install -y https://s3.amazonaws.com/ec2-downloads-windows/SSMAgent/latest/linux_amd64/amazon-ssm-agent.rpm
/etc/eks/bootstrap.sh %s.%s \
    --dns-cluster-ip %s \
	--kubelet-extra-args '--node-labels=kit.sh/provisioned=true' \
	--b64-cluster-ca %s \
	--apiserver-endpoint https://%s`
)
