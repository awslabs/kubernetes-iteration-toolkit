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
	"path"

	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/awsprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/controllers"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/resource"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/status"

	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	certsphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/certs"
	kubeletphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/kubelet"
	configutil "k8s.io/kubernetes/cmd/kubeadm/app/util/config"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	clusterCertsBasePath = "/tmp/"
	certPKIPath          = "/etc/kubernetes/pki"
	clusterManifestPath  = "/etc/kubernetes/manifests"
	kubeletSystemdPath   = "/etc/systemd/system/kubelet.service.d/"
	kubeletConfigPath    = "/var/lib/kubelet/"
)

var (
	etcdCertsPKIPath           = path.Join(certPKIPath, "etcd")
	etcdKubeletSystemdConfPath = path.Join(kubeletSystemdPath, "20-etcd-service-manager.conf")
)

const (
	kubeletServiceForETCDNodes = `[Service]
ExecStart=
#  Replace "systemd" with the cgroup driver of your container runtime. The default value in the kubelet is "cgroupfs".
ExecStart=/usr/bin/kubelet --address=127.0.0.1 --pod-manifest-path=/etc/kubernetes/manifests --cgroup-driver=systemd
Restart=always`
)

type controlPlane struct {
	ec2api     *awsprovider.EC2
	s3uploader *awsprovider.S3Manager
	elbv2      *awsprovider.ELBV2
	client.Client
}

// NewControlPlaneController returns a controller for managing VPCs in AWS
func NewControlPlaneController(ec2api *awsprovider.EC2, uploader *awsprovider.S3Manager, elbv2 *awsprovider.ELBV2, restIface client.Client) *controlPlane {
	return &controlPlane{ec2api: ec2api, s3uploader: uploader, elbv2: elbv2, Client: restIface}
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
	setDefaults(controlPlane) // TODO moved to webhook defaults for CR object
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
	cfg, err := configutil.DefaultedStaticInitConfiguration()
	if err != nil {
		return nil, err
	}
	// Generate ETCD bootstrap files
	if err := c.createETCDBootstrapFiles(ctx, controlPlane, cfg); err != nil {
		return nil, err
	}
	cfg.CertificatesDir = path.Join(clusterCertsBasePath, controlPlane.Name)
	// Generate ROOT CA for the Master components
	certs := certsphase.Certificates{certsphase.KubeadmCertRootCA()}
	certTree, err := certs.AsMap().CertTree()
	if err != nil {
		return nil, err
	}
	if err := certTree.CreateTree(cfg); err != nil {
		return nil, fmt.Errorf("error creating root CA, %w", err)
	}
	// Generate Master bootstrap files
	if err := c.createMasterBootstrapFiles(ctx, controlPlane, cfg); err != nil {
		return nil, err
	}
	// Install add-ons

	// Generate Kubeconfig files for just the worker nodes
	//
	return status.Created, nil
}

func (c *controlPlane) Finalize(_ context.Context, _ controllers.Object) (*reconcile.Result, error) {
	return status.Terminated, nil
}

func (c *controlPlane) CreateKubeletConfigFilesForNodes(ctx context.Context, clusterName string, cfg *kubeadmapi.InitConfiguration, nodes []*Node) error {
	for _, node := range nodes {
		kubeletConfigDir := path.Join(clusterCertsBasePath, clusterName, node.ID, kubeletConfigPath)
		cfg.NodeRegistration.KubeletExtraArgs = map[string]string{"cgroup-driver": "systemd",
			"network-plugin":            "cni",
			"pod-infra-container-image": "public.ecr.aws/eks-distro/kubernetes/pause:v1.18.9-eks-1-18-1",
		}
		cfg.NodeRegistration.Name = node.ID
		if err := kubeletphase.WriteKubeletDynamicEnvFile(&cfg.ClusterConfiguration, &cfg.NodeRegistration, false, kubeletConfigDir); err != nil {
			return fmt.Errorf("writing Kubelet Dynamic Env File, %w", err)
		}
		if err := kubeletphase.WriteConfigToDisk(&cfg.ClusterConfiguration, kubeletConfigDir); err != nil {
			return fmt.Errorf("writing kubelet configuration to disk, %w", err)
		}
	}
	return nil
}

func bucketName(clusterName string) string {
	return fmt.Sprintf("kit-%s", clusterName)
}
