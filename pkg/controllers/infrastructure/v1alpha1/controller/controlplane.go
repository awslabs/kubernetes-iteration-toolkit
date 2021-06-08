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
	"io/ioutil"
	"path"
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
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/certs"
	certsphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/certs"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/controlplane"
	etcdphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/etcd"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/kubeconfig"
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

	etcdLoadBalancer, err := getEtcdLoadBalancer(context.TODO(), controlPlane.Name, c.elbv2)
	if err != nil && errors.IsELBLoadBalancerNotExists(err) {
		return nil, fmt.Errorf("waiting for ETCD loadbalancer, %w", errors.WaitingForSubResources)
	}
	instances, err := getEtcdInstancesFor(context.TODO(), controlPlane.Name, c.ec2api)
	if err != nil {
		return nil, err
	}
	etcdNodeIPs := []string{}
	etcdNodeNames := []string{}
	for _, instance := range instances {
		etcdNodeIPs = append(etcdNodeIPs, aws.StringValue(instance.PrivateIpAddress))
		etcdNodeNames = append(etcdNodeNames, aws.StringValue(instance.InstanceId))
	}
	zap.S().Infof("ETCD Instance IP %v and nodes are %v", etcdNodeIPs, etcdNodeNames)
	etcdNodeName := ""
	if len(etcdNodeIPs) == 3 {
		etcdNodeName = etcdNodeNames[0]

		cfg.CertificatesDir = path.Join("/tmp/", controlPlane.Name)
		if err := c.CreateEtcdFiles(cfg, controlPlane.Name, aws.StringValue(etcdLoadBalancer.DNSName), etcdNodeIPs, etcdNodeNames); err != nil {
			return nil, fmt.Errorf("error creating ETCD CA %w", err)
		}
		cfg, err = c.generateInitConfig(controlPlane, cfg)
		if err != nil {
			return nil, err
		}
		cfg.CertificatesDir = path.Join("/tmp/", controlPlane.Name)
		if err := c.CreateKubeletConfigFilesForNodes(ctx, cfg, etcdNodeNames); err != nil {
			return nil, err
		}
		if err := c.CreatekubeletSystemServiceForETCD(ctx, cfg, etcdNodeNames); err != nil {
			return nil, err
		}
	}
	masterInstances, err := getMasterInstancesFor(ctx, controlPlane.Name, c.ec2api)
	if err != nil {
		return nil, err
	}
	masterNodeIPs := []string{}
	masterNodeNames := []string{}
	for _, instance := range masterInstances {
		masterNodeIPs = append(masterNodeIPs, aws.StringValue(instance.PrivateIpAddress))
		masterNodeNames = append(masterNodeNames, aws.StringValue(instance.InstanceId))
	}
	zap.S().Infof("Master Instance IP %v and nodes are %v", masterNodeIPs, masterNodeNames)
	cfg.CertificatesDir = path.Join("/tmp/", controlPlane.Name)
	// Generate ROOT CA
	certs := certsphase.Certificates{certsphase.KubeadmCertRootCA()}
	certTree, err := certs.AsMap().CertTree()
	if err != nil {
		return nil, err
	}
	if err := certTree.CreateTree(cfg); err != nil {
		return nil, fmt.Errorf("error creating root CA, %w", err)
	}
	for _, node := range masterNodeNames {
		if err := copyETCDCertsToMaster(etcdNodeName, node, controlPlane.Name); err != nil {
			return nil, fmt.Errorf("failed to copy ETCD certificates to master for testing, %w", err)
		}
		// Copy root CA to this node
		if err := copyRootCerts(controlPlane.Name, node); err != nil {
			return nil, fmt.Errorf("failed to copy certs, %w", err)
		}
	}
	if err := c.CreateKubeletConfigFilesForNodes(ctx, cfg, masterNodeNames); err != nil {
		return nil, err
	}
	zap.S().Infof("ETCD bootstrap complete starting master bootstrap")
	masterLB, err := getMasterLoadBalancer(ctx, controlPlane.Name, c.elbv2)
	if err != nil && errors.IsELBLoadBalancerNotExists(err) {
		return nil, fmt.Errorf("waiting for master load balancer, %w", errors.WaitingForSubResources)
	} else if err != nil {
		return nil, err
	}
	// Root CA is already created at /tmp/clustername/
	// copy to every master node to use that
	for _, masterNodeName := range masterNodeNames {
		for _, etcdNodeIP := range etcdNodeIPs {
			cfg.Etcd.External.Endpoints = append(cfg.Etcd.External.Endpoints, fmt.Sprintf("https://%s:2379", etcdNodeIP))
		}
		cfg.LocalAPIEndpoint.AdvertiseAddress = masterNodeIPs[0]
		cfg.ControlPlaneEndpoint = aws.StringValue(masterLB.DNSName) + ":443"
		cfg.APIServer.CertSANs = []string{aws.StringValue(masterLB.DNSName), masterNodeIPs[0], masterNodeNames[0], "kubernetes", "kubernetes.default", "kubernetes.default.svc", "kubernetes.default.svc.cluster.local", "10.96.0.1"}
		cfg.APIServer.ExtraArgs = map[string]string{
			"advertise-address": cfg.LocalAPIEndpoint.AdvertiseAddress,
			"secure-port":       "443",
		}
		cfg.NodeRegistration = kubeadmapi.NodeRegistrationOptions{
			Name: masterNodeName,
		}
		// Generate Master PKI and config on one of the master nodes
		if err := c.BootstrapMasterConfigFiles(ctx, cfg, masterNodeName, controlPlane.Name); err != nil {
			return nil, fmt.Errorf("failed to bootstrap master node %v, %w", masterNodeName, err)
		}
	}
	// Upload to S3 for all instances
	for _, nodeName := range append(masterNodeNames, etcdNodeNames...) {
		bucketName := fmt.Sprintf("kit-%s", controlPlane.Name)
		if err := uploadDirectories(ctx, bucketName, path.Join("/tmp/", controlPlane.Name, nodeName), c.s3uploader); err != nil {
			return nil, fmt.Errorf("uploading files to S3 %v, %w", nodeName, err)
		}
	}
	// Generate RBAC, tokens to the cluster
	if len(masterNodeNames) > 0 {
		masterNodeName := masterNodeNames[0]
		zap.S().Infof("Starting Bootstrap for master nodes")
		if err := c.bootstrapMasterNodes(ctx, controlPlane.Name, masterNodeName, cfg); err != nil {
			zap.S().Errorf("Failed to bootstrap, %v", err)
		}
	}
	return status.Created, nil
}

func (c *controlPlane) BootstrapMasterConfigFiles(ctx context.Context, cfg *kubeadmapi.InitConfiguration, nodeName, clusterName string) error {
	tmp := cfg.CertificatesDir
	defer func() {
		cfg.CertificatesDir = tmp
	}()
	// Create all the config files for one of the master node in the cluster
	cfg.CertificatesDir = path.Join(cfg.CertificatesDir, nodeName, "/etc/kubernetes/pki")
	// Generate PKI for node in the master
	if err := certs.CreatePKIAssets(cfg); err != nil {
		return fmt.Errorf("creating PKI assets, %w", err)
	}
	zap.S().Infof("Created PKI assets")
	// Generate Kube config files for master components
	for _, kubeConfigFileName := range []string{kubeadmconstants.AdminKubeConfigFileName,
		kubeadmconstants.KubeletKubeConfigFileName,
		kubeadmconstants.ControllerManagerKubeConfigFileName,
		kubeadmconstants.SchedulerKubeConfigFileName} {
		err := kubeconfig.CreateKubeConfigFile(kubeConfigFileName, path.Join(tmp, nodeName, "/etc/kubernetes"), cfg)
		if err != nil {
			return fmt.Errorf("creating kubeconfig file for %v, %w", kubeConfigFileName, err)
		}
	}
	// Generate static pod files for kube components
	cfg.CertificatesDir = "/etc/kubernetes/pki"
	for _, componentName := range []string{kubeadmconstants.KubeAPIServer,
		kubeadmconstants.KubeControllerManager,
		kubeadmconstants.KubeScheduler} {
		err := controlplane.CreateStaticPodFiles(path.Join(tmp, nodeName, "/etc/kubernetes/manifests"), "",
			&cfg.ClusterConfiguration, &cfg.LocalAPIEndpoint, componentName)
		if err != nil {
			return fmt.Errorf("creating static pod file for %v, %w", componentName, err)
		}
	}
	zap.S().Infof("Master components created for node %s", nodeName)
	return nil
}

func (c *controlPlane) Finalize(_ context.Context, _ controllers.Object) (*reconcile.Result, error) {
	return status.Terminated, nil
}

func (c *controlPlane) CreateKubeletConfigFilesForNodes(ctx context.Context, cfg *kubeadmapi.InitConfiguration, nodeNames []string) error {
	for _, nodeName := range nodeNames {
		kubeletConfigDir := path.Join(cfg.CertificatesDir, nodeName, "/var/lib/kubelet/")
		cfg.NodeRegistration.KubeletExtraArgs = map[string]string{"cgroup-driver": "systemd",
			"network-plugin":            "cni",
			"pod-infra-container-image": "public.ecr.aws/eks-distro/kubernetes/pause:3.2",
		}
		cfg.NodeRegistration.Name = nodeName
		if err := kubeletphase.WriteKubeletDynamicEnvFile(&cfg.ClusterConfiguration, &cfg.NodeRegistration, false, kubeletConfigDir); err != nil {
			return fmt.Errorf("writing Kubelet Dynamic Env File, %w", err)
		}
		if err := kubeletphase.WriteConfigToDisk(&cfg.ClusterConfiguration, kubeletConfigDir); err != nil {
			return fmt.Errorf("writing kubelet configuration to disk, %w", err)
		}
	}
	return nil
}

func (c *controlPlane) CreatekubeletSystemServiceForETCD(ctx context.Context, cfg *kubeadmapi.InitConfiguration, nodeNames []string) error {
	for _, nodeName := range nodeNames {
		if err := createDir(path.Join(cfg.CertificatesDir, nodeName, "/etc/systemd/system/kubelet.service.d/")); err != nil {
			return err
		}
		kubeletSvcPath := path.Join(cfg.CertificatesDir, nodeName, "/etc/systemd/system/kubelet.service.d/20-etcd-service-manager.conf")
		if err := ioutil.WriteFile(kubeletSvcPath, []byte(kubeletServiceForETCDNodes), 0644); err != nil {
			return fmt.Errorf("writing kubelet configuration %q, %w", kubeletSvcPath, err)
		}
	}
	return nil
}

const (
	kubeletServiceForETCDNodes = `[Service]
ExecStart=
#  Replace "systemd" with the cgroup driver of your container runtime. The default value in the kubelet is "cgroupfs".
ExecStart=/usr/bin/kubelet --address=127.0.0.1 --pod-manifest-path=/etc/kubernetes/manifests --cgroup-driver=systemd
Restart=always`
)

func (c *controlPlane) CreateEtcdFiles(cfg *kubeadmapi.InitConfiguration, clusterName, lbName string, nodeIPs, nodeNames []string) error {

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
	// bucketName := fmt.Sprintf("kit-%s", clusterName)
	for i := 0; i < len(nodeIPs); i++ {
		cfg.CertificatesDir = path.Join("/tmp/", clusterName, nodeNames[i], "/etc/kubernetes/pki/")
		cfg.Etcd.Local = &kubeadm.LocalEtcd{
			ServerCertSANs: []string{nodeIPs[i], lbName},
			PeerCertSANs:   []string{nodeIPs[i], lbName},
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
		// if err := uploadDirectories(context.TODO(), bucketName, path.Join("/tmp/", clusterName, nodeNames[i]), c.s3uploader); err != nil {
		// 	zap.S().Errorf("failed to upload %w", err)
		// }
	}
	return nil
}

func CopyCA(src, dst string) error {
	for _, fileName := range []string{"/ca.crt", "/ca.key"} {
		if err := copyFile(path.Join(src, fileName), path.Join(dst, fileName)); err != nil {
			return err
		}
	}
	return nil
}

func copyRootCerts(clusterName, masterNodeName string) error {
	for _, s := range []string{"ca.crt", "ca.key"} {
		src := path.Join("/tmp/", clusterName, s)
		dest := path.Join("/tmp/", clusterName, masterNodeName, "/etc/kubernetes/pki", s)
		if err := createDir(path.Dir(dest)); err != nil {
			return fmt.Errorf("creating directory %v, %w", path.Dir(dest), err)
		}
		if err := copyFile(src, dest); err != nil {
			return err
		}
	}
	return nil
}

func copyETCDCertsToMaster(etcdNodeName, masterNodeName, clusterName string) error {
	for _, p := range []string{"etcd/ca.crt", "apiserver-etcd-client.crt", "apiserver-etcd-client.key"} {
		src := path.Join("/tmp/", clusterName, etcdNodeName, "/etc/kubernetes/pki", p)
		dest := path.Join("/tmp/", clusterName, masterNodeName, "/etc/kubernetes/pki", p)
		if err := createDir(path.Dir(dest)); err != nil {
			return fmt.Errorf("creating directory %v, %w", path.Dir(dest), err)
		}
		if err := copyFile(src, dest); err != nil {
			return err
		}
	}
	// scp /etc/kubernetes/pki/etcd/ca.crt "${CONTROL_PLANE}":
	// 	scp /etc/kubernetes/pki/apiserver-etcd-client.crt "${CONTROL_PLANE}":
	// 	scp /etc/kubernetes/pki/apiserver-etcd-client.key "${CONTROL_PLANE}":

	return nil
}

func (c *controlPlane) generateInitConfig(controlPlane *v1alpha1.ControlPlane, internalcfg *kubeadmapi.InitConfiguration) (*kubeadmapi.InitConfiguration, error) {
	internalcfg.Etcd.Local = nil
	// etcdLoadBalancer, err := getEtcdLoadBalancer(context.TODO(), controlPlane.Name, c.elbv2)
	// if err != nil && errors.IsELBLoadBalancerNotExists(err) {
	// 	return nil, fmt.Errorf("waiting for ETCD loadbalancer, %w", errors.WaitingForSubResources)
	// }
	// masterLoadBalancer, err := getMasterLoadBalancer(context.TODO(), controlPlane.Name, c.elbv2)
	// if err != nil && errors.IsELBLoadBalancerNotExists(err) {
	// 	return nil, fmt.Errorf("waiting for Master loadbalancer, %w", errors.WaitingForSubResources)
	// }
	internalcfg.Etcd.External = &kubeadm.ExternalEtcd{
		// Endpoints: []string{aws.StringValue(etcdLoadBalancer.DNSName)},
		CAFile:   "/etc/kubernetes/pki/etcd/ca.crt",
		CertFile: "/etc/kubernetes/pki/apiserver-etcd-client.crt",
		KeyFile:  "/etc/kubernetes/pki/apiserver-etcd-client.key",
	}
	// internalcfg.LocalAPIEndpoint.AdvertiseAddress = aws.StringValue(masterLoadBalancer.DNSName)
	internalcfg.LocalAPIEndpoint.BindPort = 443 // TODO get from controlPlane spec
	// internalcfg.ControlPlaneEndpoint = aws.StringValue(masterLoadBalancer.DNSName)
	version := "v1.19.8-eks-1-19-4"
	if controlPlane.Spec.KubernetesVersion != "" {
		version = controlPlane.Spec.KubernetesVersion
	}
	internalcfg.ClusterConfiguration.KubernetesVersion = version
	internalcfg.ClusterConfiguration.ImageRepository = "public.ecr.aws/eks-distro/kubernetes"
	// internalcfg.APIServer.CertSANs = []string{aws.StringValue(masterLoadBalancer.DNSName)}
	return internalcfg, nil
}
