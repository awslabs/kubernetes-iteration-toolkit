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

	"github.com/aws/aws-sdk-go/aws"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/apis/infrastructure/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/pkg/errors"
	"go.uber.org/zap"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/certs"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/controlplane"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/kubeconfig"
)

func (c *controlPlane) createMasterBootstrapFiles(ctx context.Context, controlPlane *v1alpha1.ControlPlane, cfg *kubeadmapi.InitConfiguration) error {
	masterNodes, err := getMasterInstancesFor(ctx, controlPlane.Name, c.ec2api)
	if err != nil {
		return err
	}
	etcdNodes, err := getEtcdInstancesFor(ctx, controlPlane.Name, c.ec2api)
	if err != nil {
		return err
	}
	masterLB, err := getMasterLoadBalancer(ctx, controlPlane.Name, c.elbv2)
	if err != nil && errors.IsELBLoadBalancerNotExists(err) {
		return fmt.Errorf("waiting for master load balancer, %w", errors.WaitingForSubResources)
	} else if err != nil {
		return err
	}
	zap.S().Infof("Master nodes are %+v", masterNodes)
	if len(masterNodes) == 3 && len(etcdNodes) == 3 {
		for _, node := range masterNodes {
			zap.S().Infof("Master node is %+v", node)
		}
		etcdNodeID := etcdNodes[0].ID
		for _, node := range masterNodes {
			// Copy ETCD certs to this node
			if err := copyETCDCertsToMaster(etcdNodeID, node.ID, controlPlane.Name); err != nil {
				return fmt.Errorf("failed to copy ETCD certificates to master for testing, %w", err)
			}
			// Copy root CA to this node
			if err := copyMasterRootCerts(controlPlane.Name, node.ID); err != nil {
				return fmt.Errorf("failed to copy root certs, %w", err)
			}
		}
		if err := c.CreateKubeletConfigFilesForNodes(ctx, controlPlane.Name, cfg, masterNodes); err != nil {
			return err
		}

		for _, masterNode := range masterNodes {
			for _, etcdNode := range etcdNodes {
				cfg.Etcd.External.Endpoints = append(cfg.Etcd.External.Endpoints, fmt.Sprintf("https://%s:2379", etcdNode.IPAddress))
			}
			cfg.LocalAPIEndpoint.AdvertiseAddress = masterNode.IPAddress
			cfg.ControlPlaneEndpoint = aws.StringValue(masterLB.DNSName) + ":443"
			cfg.APIServer.CertSANs = []string{aws.StringValue(masterLB.DNSName), masterNode.IPAddress, masterNode.ID, "kubernetes", "kubernetes.default", "kubernetes.default.svc", "kubernetes.default.svc.cluster.local", "10.96.0.1"}
			cfg.APIServer.ExtraArgs = map[string]string{
				"advertise-address": cfg.LocalAPIEndpoint.AdvertiseAddress,
				"secure-port":       "443",
			}
			if cfg.APIServer.ExtraArgs == nil {
				cfg.APIServer.ExtraArgs = map[string]string{}
			}
			if cfg.Scheduler.ExtraArgs == nil {
				cfg.Scheduler.ExtraArgs = map[string]string{}
			}
			if cfg.ControllerManager.ExtraArgs == nil {
				cfg.ControllerManager.ExtraArgs = map[string]string{}
			}
			unifyKeys(cfg.APIServer.ExtraArgs, controlPlane.Spec.Master.APIServer.Args)
			unifyKeys(cfg.Scheduler.ExtraArgs, controlPlane.Spec.Master.Scheduler.Args)
			unifyKeys(cfg.ControllerManager.ExtraArgs, controlPlane.Spec.Master.ControllerManager.Args)
			cfg.NodeRegistration = kubeadmapi.NodeRegistrationOptions{
				Name: masterNode.ID,
			}
			// Generate Master PKI and config on one of the master nodes
			if err := c.BootstrapMasterConfigFiles(ctx, cfg, masterNode.ID, controlPlane.Name); err != nil {
				return fmt.Errorf("failed to bootstrap master node %v, %w", masterNode.ID, err)
			}
			if err := uploadDirectories(ctx, bucketName(controlPlane.Name), path.Join(clusterCertsBasePath, controlPlane.Name, masterNode.ID), c.s3uploader); err != nil {
				return fmt.Errorf("uploading files to S3 %v, %w", masterNode.ID, err)
			}
		}
		nodes := append(masterNodes, etcdNodes...)
		for _, node := range nodes {
			if err := uploadDirectories(ctx, bucketName(controlPlane.Name), path.Join(clusterCertsBasePath, controlPlane.Name, node.ID), c.s3uploader); err != nil {
				return fmt.Errorf("uploading files to S3 %v, %w", node.ID, err)
			}
		}
		// Connect with any master node to create a client
		masterNodeID := masterNodes[0].ID
		zap.S().Infof("Starting Bootstrap for master nodes")
		if err := c.bootstrapMasterNodes(ctx, controlPlane.Name, masterNodeID, cfg); err != nil {
			zap.S().Errorf("Failed to bootstrap, %v", err)
		}
	}
	return nil
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

func copyETCDCertsToMaster(etcdNodeName, masterNodeName, clusterName string) error {
	for _, p := range []string{"etcd/ca.crt", "apiserver-etcd-client.crt", "apiserver-etcd-client.key"} {
		src := path.Join(clusterCertsBasePath, clusterName, etcdNodeName, certPKIPath, p)
		dest := path.Join(clusterCertsBasePath, clusterName, masterNodeName, certPKIPath, p)
		if err := copyCerts(src, dest); err != nil {
			return err
		}
	}
	// scp /etc/kubernetes/pki/etcd/ca.crt "${CONTROL_PLANE}":
	// 	scp /etc/kubernetes/pki/apiserver-etcd-client.crt "${CONTROL_PLANE}":
	// 	scp /etc/kubernetes/pki/apiserver-etcd-client.key "${CONTROL_PLANE}":

	return nil
}

func copyMasterRootCerts(clusterName, masterNodeName string) error {
	src := path.Join(clusterCertsBasePath, clusterName)
	dest := path.Join(clusterCertsBasePath, clusterName, masterNodeName, certPKIPath)
	if err := CopyCACertsFrom(src, dest); err != nil {
		return err
	}
	return nil
}
