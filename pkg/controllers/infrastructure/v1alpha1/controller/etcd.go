package controller

import (
	"context"
	"fmt"
	"io/ioutil"
	"path"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/errors"
	"go.uber.org/zap"
	"k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	certsphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/certs"
	etcdphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/etcd"
)

func (c *controlPlane) createETCDBootstrapFiles(ctx context.Context, controlPlane *v1alpha1.ControlPlane, cfg *kubeadmapi.InitConfiguration) error {
	nodes, err := getEtcdInstancesFor(ctx, controlPlane.Name, c.ec2api)
	if err != nil {
		return err
	}
	zap.S().Infof("ETCD nodes are %+v", nodes)
	// if len(nodes) == controlPlane.Spec.Etcd.Instances.Count {
	if len(nodes) == 3 {
		for _, node := range nodes {
			zap.S().Infof("ETCD node is %+v", node)
		}
		cfg.CertificatesDir = path.Join(clusterCertsBasePath, controlPlane.Name)
		if err := c.CreateEtcdCertsAndManifests(ctx, cfg, controlPlane.Name, nodes); err != nil {
			return fmt.Errorf("error creating ETCD certs and manifets, %w", err)
		}
		zap.S().Infof("Successfully created ETCD certs and Manifests files")
		cfg.Etcd.Local = nil
		cfg.Etcd.External = &kubeadm.ExternalEtcd{
			CAFile:   "/etc/kubernetes/pki/etcd/ca.crt",
			CertFile: "/etc/kubernetes/pki/apiserver-etcd-client.crt",
			KeyFile:  "/etc/kubernetes/pki/apiserver-etcd-client.key",
		}
		cfg.LocalAPIEndpoint.BindPort = 443 // TODO get from controlPlane spec
		version := "v1.19.8-eks-1-19-4"
		if controlPlane.Spec.KubernetesVersion != "" {
			version = controlPlane.Spec.KubernetesVersion
		}
		cfg.ClusterConfiguration.KubernetesVersion = version
		cfg.ClusterConfiguration.ImageRepository = "public.ecr.aws/eks-distro/kubernetes"
		cfg.CertificatesDir = path.Join(clusterCertsBasePath, controlPlane.Name)
		if err := c.CreateKubeletConfigFilesForNodes(ctx, controlPlane.Name, cfg, nodes); err != nil {
			return err
		}
		if err := c.CreatekubeletSystemServiceForETCD(ctx, controlPlane.Name, cfg, nodes); err != nil {
			return err
		}
	}
	return nil
}

func copyEtcdRootCerts(clusterName, nodeName string) error {
	src := path.Join(clusterCertsBasePath, clusterName, "/etcd")
	dst := path.Join(clusterCertsBasePath, clusterName, nodeName, etcdCertsPKIPath)
	zap.S().Infof("Copying certs from %s->%s", src, dst)
	if err := CopyCACertsFrom(src, dst); err != nil {
		return err
	}
	return nil
}

func (c *controlPlane) CreateEtcdCertsAndManifests(ctx context.Context, cfg *kubeadmapi.InitConfiguration, clusterName string, nodes []*Node) error {
	etcdLoadBalancer, err := getEtcdLoadBalancer(ctx, clusterName, c.elbv2)
	if err != nil && errors.IsELBLoadBalancerNotExists(err) {
		return fmt.Errorf("waiting for ETCD loadbalancer, %w", errors.WaitingForSubResources)
	}
	// Create the root CA for ETCD and copy it to all the instances
	ca := certsphase.KubeadmCertEtcdCA()
	if err := certsphase.CreateCACertAndKeyFiles(ca, cfg); err != nil {
		return fmt.Errorf("creating etcd root CA, %w", err)
	}
	for _, node := range nodes {
		if err := copyEtcdRootCerts(clusterName, node.ID); err != nil {
			return err
		}
		// pkiPath := path.Join(clusterCertsBasePath, clusterName, node.ID, etcdCertsPKIPath)
		// if err := createDir(pkiPath); err != nil {
		// 	return err
		// }
		// if err := CopyCA(path.Join(clusterCertsBasePath, clusterName, "/etcd"), pkiPath); err != nil {
		// 	return err
		// }
	}
	// Generate the hosts path for all the ETCD nodes
	hosts := []string{}
	for _, node := range nodes {
		hosts = append(hosts, fmt.Sprintf("%s=https://%s:2380", node.ID, node.IPAddress))
	}
	// Generate required certs and Keys and static pod manifests for every etcd node
	for _, node := range nodes {
		cfg.CertificatesDir = path.Join(clusterCertsBasePath, clusterName, node.ID, certPKIPath)
		cfg.Etcd.Local = &kubeadm.LocalEtcd{
			ServerCertSANs: []string{node.IPAddress, aws.StringValue(etcdLoadBalancer.DNSName)},
			PeerCertSANs:   []string{node.IPAddress, aws.StringValue(etcdLoadBalancer.DNSName)},
			DataDir:        "/var/lib/etcd",
			ExtraArgs: map[string]string{
				"initial-cluster":             strings.Join(hosts, ","),
				"initial-cluster-state":       "new",
				"name":                        node.ID,
				"listen-peer-urls":            fmt.Sprintf("https://%s:2380", node.IPAddress),
				"listen-client-urls":          fmt.Sprintf("https://%s:2379", node.IPAddress),
				"advertise-client-urls":       fmt.Sprintf("https://%s:2379", node.IPAddress),
				"initial-advertise-peer-urls": fmt.Sprintf("https://%s:2380", node.IPAddress),
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
		manifest := path.Join(clusterCertsBasePath, clusterName, node.ID, clusterManifestPath)
		// etcd phase adds cfg.CertificatesDir to static pod yaml for pods to read the certs from
		cfg.CertificatesDir = certPKIPath
		if err := etcdphase.CreateLocalEtcdStaticPodManifestFile(manifest, "", cfg.NodeRegistration.Name, &cfg.ClusterConfiguration, &cfg.LocalAPIEndpoint); err != nil {
			return fmt.Errorf("error creating local etcd static pod manifest file %w", err)
		}
		zap.S().Infof("ETCD Pod manifest created for node %s", node.ID)
		// Upload to S3 for all instances
		if err := uploadDirectories(ctx, bucketName(clusterName), path.Join(clusterCertsBasePath, clusterName, node.ID), c.s3uploader); err != nil {
			return fmt.Errorf("uploading files to S3 %v, %w", node.ID, err)
		}
	}
	return nil
}

func (c *controlPlane) CreatekubeletSystemServiceForETCD(ctx context.Context, clusterName string, cfg *kubeadmapi.InitConfiguration, nodes []*Node) error {
	for _, node := range nodes {
		if err := createDir(path.Join(clusterCertsBasePath, clusterName, node.ID, kubeletSystemdPath)); err != nil {
			return err
		}
		kubeletSvcPath := path.Join(clusterCertsBasePath, clusterName, node.ID, etcdKubeletSystemdConfPath)
		if err := ioutil.WriteFile(kubeletSvcPath, []byte(kubeletServiceForETCDNodes), 0644); err != nil {
			return fmt.Errorf("writing kubelet configuration %q, %w", kubeletSvcPath, err)
		}
	}
	return nil
}
