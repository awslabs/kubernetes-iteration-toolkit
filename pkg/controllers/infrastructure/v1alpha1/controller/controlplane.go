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
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/awsprovider"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/controllers"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/errors"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/resource"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/status"
	"go.uber.org/zap"

	"k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	certsphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/certs"
	etcdphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/etcd"
	kubeletphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/kubelet"
	configutil "k8s.io/kubernetes/cmd/kubeadm/app/util/config"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
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
	instances, err := getEtcdInstancesFor(context.TODO(), controlPlane.Name, c.ec2api)
	if err != nil {
		return nil, err
	}
	nodeIPs := []string{}
	nodeNames := []string{}
	for _, instance := range instances {
		nodeIPs = append(nodeIPs, aws.StringValue(instance.PrivateIpAddress))
		nodeNames = append(nodeNames, aws.StringValue(instance.InstanceId))
	}
	if len(nodeIPs) > 0 {
		zap.S().Infof("ETCD Instance IP %v and nodes are %v", nodeIPs, nodeNames)
		cfg.CertificatesDir = path.Join("/tmp/", controlPlane.Name)
		if err := c.CreateEtcdFiles(cfg, controlPlane.Name, nodeIPs, nodeNames); err != nil {
			return nil, fmt.Errorf("error creating ETCD CA %w", err)
		}
		cfg, err = c.generateInitConfig(controlPlane, cfg)
		if err != nil {
			return nil, err
		}
		cfg.CertificatesDir = path.Join("/tmp/", controlPlane.Name)
		zap.S().Infof("Certificates dir is %s", cfg.CertificatesDir)
		if err := c.CreateKubeletConfigFilesForETCDNodes(ctx, cfg, controlPlane.Name, nodeNames); err != nil {
			return nil, err
		}
	}
	// Generate Master config files

	return status.Created, nil
}

func (c *controlPlane) Finalize(_ context.Context, _ controllers.Object) (*reconcile.Result, error) {
	return status.Terminated, nil
}

func (c *controlPlane) CreateKubeletConfigFilesForETCDNodes(ctx context.Context, cfg *kubeadmapi.InitConfiguration, clusterName string, nodeNames []string) error {
	for _, nodeName := range nodeNames {
		kubeletConfigDir := path.Join(cfg.CertificatesDir, nodeName, "/var/lib/kubelet/")
		if err := kubeletphase.WriteKubeletDynamicEnvFile(&cfg.ClusterConfiguration, &cfg.NodeRegistration, false, kubeletConfigDir); err != nil {
			return fmt.Errorf("writing Kubelet Dynamic Env File, %w", err)
		}
		if err := kubeletphase.WriteConfigToDisk(&cfg.ClusterConfiguration, kubeletConfigDir); err != nil {
			return fmt.Errorf("writing kubelet configuration to disk, %w", err)
		}
		bucketName := fmt.Sprintf("kit-%s", clusterName)
		if err := uploadDirectories(ctx, bucketName, path.Join(cfg.CertificatesDir, nodeName), c.s3uploader); err != nil {
			return fmt.Errorf("uploading kubelet config for node %v, %w", nodeName, err)
		}
	}
	return nil
}

func (c *controlPlane) CreateEtcdFiles(cfg *kubeadmapi.InitConfiguration, clusterName string, nodeIPs, nodeNames []string) error {

	ca := certsphase.KubeadmCertEtcdCA()
	if err := certsphase.CreateCACertAndKeyFiles(ca, cfg); err != nil {
		return err
	}
	hosts := []string{}
	for i := 0; i < len(nodeIPs); i++ {
		hosts = append(hosts, fmt.Sprintf("%s=https://%s:2380", nodeNames[i], nodeIPs[i]))
		pkiPath := path.Join("/tmp/", clusterName, nodeNames[i], "/etc/kubernetes/pki/etcd")
		if err := createDir(pkiPath); err != nil {
			return err
		}
		if err := CopyCA(path.Join(cfg.CertificatesDir, "/etcd"), pkiPath); err != nil {
			return err
		}
	}

	bucketName := fmt.Sprintf("kit-%s", clusterName)
	for i := 0; i < len(nodeIPs); i++ {
		cfg.CertificatesDir = path.Join("/tmp/", clusterName, nodeNames[i], "/etc/kubernetes/pki/")
		cfg.Etcd.Local = &kubeadm.LocalEtcd{
			ServerCertSANs: []string{nodeIPs[i], "foo-etcd-instances-10cfd2d7cfbd2c92.elb.us-east-2.amazonaws.com"},
			PeerCertSANs:   []string{nodeIPs[i], "foo-etcd-instances-10cfd2d7cfbd2c92.elb.us-east-2.amazonaws.com"},
			DataDir:        "/var/lib/etcd",
			ExtraArgs: map[string]string{
				"initial-cluster":             strings.Join(hosts, ","),
				"initial-cluster-state":       "new",
				"name":                        nodeNames[i],
				"listen-peer-urls":            fmt.Sprintf("https://%s:2380", nodeIPs[i]),
				"listen-client-urls":          fmt.Sprintf("https://%s:2379", nodeIPs[i]),
				"advertise-client-urls":       fmt.Sprintf("https://%s:2379", nodeIPs[i]),
				"initial-advertise-peer-urls": fmt.Sprintf("https://%s:2380", nodeIPs[i]),
			},
		}
		for _, cert := range []*certsphase.KubeadmCert{
			certsphase.KubeadmCertEtcdServer(),
			certsphase.KubeadmCertEtcdPeer(),
			certsphase.KubeadmCertEtcdHealthcheck(),
			certsphase.KubeadmCertEtcdAPIClient(),
		} {
			if err := certsphase.CreateCertAndKeyFilesWithCA(cert, ca, cfg); err != nil {
				return fmt.Errorf("creating %v, %w", cert.Name, err)
			}
		}
		manifest := path.Join("/tmp", clusterName, nodeNames[i], "/etc/kubernetes/manifests")
		cfg.CertificatesDir = "/etc/kubernetes/pki"
		if err := etcdphase.CreateLocalEtcdStaticPodManifestFile(manifest, "", cfg.NodeRegistration.Name, &cfg.ClusterConfiguration, &cfg.LocalAPIEndpoint); err != nil {
			return fmt.Errorf("error creating local etcd static pod manifest file %w", err)
		}
		zap.S().Infof("ETCD Pod manifest created for node %s", nodeNames[i])
		// cfg.CertificatesDir = tmp

		if err := uploadDirectories(context.TODO(), bucketName, path.Join("/tmp/", clusterName, nodeNames[i]), c.s3uploader); err != nil {
			zap.S().Errorf("failed to upload %w", err)
		}
	}
	return nil
}

func CopyCA(src, dst string) error {
	for _, fileName := range []string{"/ca.crt", "/ca.key"} {
		in, err := os.Open(src + fileName)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(dst + fileName)
		if err != nil {
			return err
		}
		defer out.Close()

		_, err = io.Copy(out, in)
		if err != nil {
			return err
		}
	}
	return nil
}

func moveCerts(from, to string) error {
	caFiles := map[string]bool{"ca.crt": true, "ca.key": true}
	err := filepath.Walk(from,
		func(filePath string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || caFiles[info.Name()] {
				return nil
			}
			if err := os.Rename(filePath, path.Join(to, info.Name())); err != nil {
				return err
			}
			return os.Remove(filePath)
		})
	return err
}

func createDir(path string) error {
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0777); err != nil {
			return err
		}
		return nil
	}
	return err

}
func (c *controlPlane) generateInitConfig(controlPlane *v1alpha1.ControlPlane, internalcfg *kubeadmapi.InitConfiguration) (*kubeadmapi.InitConfiguration, error) {
	etcdLoadBalancer, err := getEtcdLoadBalancer(context.TODO(), controlPlane.Name, c.elbv2)
	if err != nil && errors.IsELBLoadBalancerNotExists(err) {
		return nil, fmt.Errorf("waiting for ETCD loadbalancer, %w", errors.WaitingForSubResources)
	}
	masterLoadBalancer, err := getMasterLoadBalancer(context.TODO(), controlPlane.Name, c.elbv2)
	if err != nil && errors.IsELBLoadBalancerNotExists(err) {
		return nil, fmt.Errorf("waiting for Master loadbalancer, %w", errors.WaitingForSubResources)
	}
	internalcfg.Etcd.External = &kubeadm.ExternalEtcd{
		Endpoints: []string{aws.StringValue(etcdLoadBalancer.DNSName)},
		CAFile:    "/etc/kubernetes/pki/etcd/ca.crt",
		CertFile:  "/etc/kubernetes/pki/apiserver-etcd-client.crt",
		KeyFile:   "/etc/kubernetes/pki/apiserver-etcd-client.key",
	}
	internalcfg.LocalAPIEndpoint.BindPort = 443 // TODO get from controlPlane spec
	internalcfg.ControlPlaneEndpoint = aws.StringValue(masterLoadBalancer.DNSName)
	version := "v1.19.8-eks-1-19-4"
	if controlPlane.Spec.KubernetesVersion != "" {
		version = controlPlane.Spec.KubernetesVersion
	}
	internalcfg.ClusterConfiguration.KubernetesVersion = version
	internalcfg.ClusterConfiguration.ImageRepository = "public.ecr.aws/eks-distro/kubernetes"
	return internalcfg, nil
}
