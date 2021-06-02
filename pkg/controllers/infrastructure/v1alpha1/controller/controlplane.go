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
	"fmt"

	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/awsprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/controllers"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/resource"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/status"

	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	certsphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/certs"
	configutil "k8s.io/kubernetes/cmd/kubeadm/app/util/config"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type controlPlane struct {
	ec2api *awsprovider.EC2
	client.Client
}

// NewControlPlaneController returns a controller for managing VPCs in AWS
func NewControlPlaneController(ec2api *awsprovider.EC2, restIface client.Client) *controlPlane {
	return &controlPlane{ec2api: ec2api, Client: restIface}
}

// Name returns the name of the controller
func (c *controlPlane) Name() string {
	return "control-plane"
}

// For returns the resource this controller is for.
func (c *controlPlane) For() controllers.Object {
	return &v1alpha1.ControlPlane{}
}

type ResourceManager interface {
	Create(context.Context, *v1alpha1.ControlPlane) error
}

// Reconcile will check if the resource exists is AWS if it does sync status,
// else create the resource and then sync status with the ControlPlane.Status
// object
func (c *controlPlane) Reconcile(ctx context.Context, object controllers.Object) (*reconcile.Result, error) {
	controlPlane := object.(*v1alpha1.ControlPlane)
	resources := []ResourceManager{
		&resource.VPC{KubeClient: c.Client},
		&resource.S3{KubeClient: c.Client},
		&resource.Subnet{KubeClient: c.Client, Region: *c.ec2api.Config.Region},
		&resource.InternetGateway{KubeClient: c.Client},
		&resource.ElasticIP{KubeClient: c.Client},
		&resource.NatGateway{KubeClient: c.Client},
		&resource.RouteTable{KubeClient: c.Client},
		&resource.SecurityGroup{KubeClient: c.Client},
		&resource.Role{KubeClient: c.Client},
		&resource.Profile{KubeClient: c.Client},
		&resource.Policy{KubeClient: c.Client},
		&resource.LaunchTemplate{KubeClient: c.Client},
		&resource.AutoScalingGroup{KubeClient: c.Client},
		&resource.NetworkLoadBalancer{KubeClient: c.Client},
		&resource.TargetGroup{KubeClient: c.Client},
	}
	for _, resource := range resources {
		if err := resource.Create(ctx, controlPlane); err != nil {
			return nil, fmt.Errorf("creating resources %v", err)
		}
	}
	// Once these resources are created we need to create PKI and Kube objects and push them to S3
	return status.Created, nil
}

func (c *controlPlane) Finalize(_ context.Context, _ controllers.Object) (*reconcile.Result, error) {
	return status.Terminated, nil
}

func (c *controlPlane) CreateETCDCA() error {

	cfg, err := c.generateInitConfig()
	if err != nil {
		return err
	}
	ca := certsphase.KubeadmCertEtcdCA()
	return certsphase.CreateCACertAndKeyFiles(ca, cfg)
}

func (c *controlPlane) generateInitConfig() (*kubeadmapi.InitConfiguration, error) {
	internalcfg, err := configutil.DefaultedStaticInitConfiguration()
	if err != nil {
		return nil, err
	}
	// Get DNS name for ETCD loadbalancer
	// Get DNS name for Master loadbalancer
	internalcfg.CertificatesDir = "/tmp/etcd-certs"
	// internalcfg.ClusterConfiguration.ImageRepository = "public.ecr.aws/eks-distro/kubernetes"
	return internalcfg, nil
}
