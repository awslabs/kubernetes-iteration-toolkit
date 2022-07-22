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
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/components/iamauthenticator"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/substrate/pkg/utils/discovery"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/certs"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/controlplane"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/etcd"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/kubeconfig"
	kubeadmutil "k8s.io/kubernetes/cmd/kubeadm/app/util"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/config"
	"knative.dev/pkg/logging"
	"knative.dev/pkg/ptr"
	"os"
	"path"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strings"
)

const (
	clusterConfigDir       = ".kit/env"
	kubeconfigPath         = "/etc/kubernetes"
	kubeconfigFile         = "etc/kubernetes/admin.conf"
	certPKIPath            = "/etc/kubernetes/pki"
	clusterManifestPath    = "/etc/kubernetes/manifests"
	kubeletSystemdPath     = "/etc/systemd/system"
	kubeletConfigPath      = "/var/lib/kubelet/"
	authenticatorConfigDir = "/etc/aws-iam-authenticator"
	kubernetesVersionTag   = "v1.21.2-eks-1-21-4"
	imageRepository        = "public.ecr.aws/eks-distro/kubernetes"
	etcdVersionTag         = "v3.4.16-eks-1-21-7"
	etcdImageRepository    = "public.ecr.aws/eks-distro/etcd-io"
	//Todo: until we have irsa support - https://github.com/awslabs/kubernetes-iteration-toolkit/issues/186,
	//this role name is tightly coupled with tekton pipelines and tasks; Please ensure you change tasks/ accordingly if you change this rolename
	TenantControlPlaneNodeRole = "tenant-controlplane-node-role"
)

type Config struct {
	S3                *s3.S3
	STS               *sts.STS
	S3Uploader        *s3manager.Uploader
	clusterConfigPath string
}

func (c *Config) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if substrate.Status.Cluster.APIServerAddress == nil {
		return reconcile.Result{Requeue: true}, nil
	}
	if c.clusterConfigPath == "" {
		if err := c.ensureKitEnvDir(); err != nil {
			return reconcile.Result{}, fmt.Errorf("ensuring kit env dir, %w", err)
		}
	}
	// ensure S3 bucket
	if err := c.ensureBucket(ctx, substrate); err != nil {
		return reconcile.Result{}, fmt.Errorf("ensuring S3 bucket, %w", err)
	}
	// create all configs file
	cfg := DefaultClusterConfig(substrate)
	if err := c.generateCerts(cfg, substrate); err != nil {
		return reconcile.Result{}, fmt.Errorf("generating certs, %w", err)
	}
	if err := c.kubeConfigs(cfg, substrate); err != nil {
		return reconcile.Result{}, fmt.Errorf("generating kube config, %w", err)
	}
	if err := c.generateStaticPodManifests(cfg, substrate); err != nil {
		return reconcile.Result{}, fmt.Errorf("generating manifests, %w", err)
	}
	if err := c.kubeletSystemService(cfg, substrate); err != nil {
		return reconcile.Result{}, fmt.Errorf("generating kubelet service config, %w", err)
	}
	// deploy aws IAM authenticator
	if err := c.ensureAuthenticatorConfig(ctx, substrate); err != nil {
		return reconcile.Result{}, fmt.Errorf("generating authenticator config, %w", err)
	}
	if err := c.staticPodSpecForAuthenticator(ctx, substrate); err != nil {
		return reconcile.Result{}, fmt.Errorf("generating authenticator config, %w", err)
	}
	// upload to s3 bucket
	if err := c.S3Uploader.UploadWithIterator(ctx, NewDirectoryIterator(
		aws.StringValue(discovery.Name(substrate)), path.Join(c.clusterConfigPath, aws.StringValue(discovery.Name(substrate))))); err != nil {
		return reconcile.Result{}, fmt.Errorf("uploading to S3 %w", err)
	}
	logging.FromContext(ctx).Debugf("Uploaded cluster configuration to s3://%s", aws.StringValue(discovery.Name(substrate)))
	substrate.Status.Cluster.KubeConfig = ptr.String(path.Join(c.clusterConfigPath, aws.StringValue(discovery.Name(substrate)), kubeconfigFile))
	return reconcile.Result{}, nil
}

func (c *Config) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	// delete the s3 bucket
	if err := s3manager.NewBatchDeleteWithClient(c.S3).Delete(ctx, s3manager.NewDeleteListIterator(
		c.S3, &s3.ListObjectsInput{Bucket: discovery.Name(substrate)}),
	); err != nil && !strings.Contains(err.(awserr.Error).Error(), "NoSuchBucket") {
		return reconcile.Result{}, fmt.Errorf("deleting objects from bucket %v", err)
	}
	if _, err := c.S3.DeleteBucketWithContext(ctx, &s3.DeleteBucketInput{Bucket: discovery.Name(substrate)}); err != nil {
		if err.(awserr.Error).Code() != s3.ErrCodeNoSuchBucket {
			return reconcile.Result{}, fmt.Errorf("deleting S3, %w", err)
		}
	} else {
		logging.FromContext(ctx).Infof("Deleted S3 bucket %s", aws.StringValue(discovery.Name(substrate)))
	}
	if c.clusterConfigPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("finding HOME dir %v", err)
		}
		c.clusterConfigPath = filepath.Join(home, clusterConfigDir)
	}
	return reconcile.Result{}, os.RemoveAll(path.Join(c.clusterConfigPath, aws.StringValue(discovery.Name(substrate))))
}

func ErrNoSuchBucket(err error) bool {
	if err != nil {
		if aerr := awserr.Error(nil); errors.As(err, &aerr) {
			return aerr.Code() == s3.ErrCodeNoSuchBucket
		}
	}
	return false
}

func (c *Config) generateCerts(cfg *kubeadm.InitConfiguration, substrate *v1alpha1.Substrate) error {
	cfg.CertificatesDir = path.Join(c.clusterConfigPath, aws.StringValue(discovery.Name(substrate)), certPKIPath)
	certTree, err := certs.GetDefaultCertList().AsMap().CertTree()
	if err != nil {
		return err
	}
	if err := certTree.CreateTree(cfg); err != nil {
		return fmt.Errorf("error creating cert tree, %w", err)
	}
	// create private and public keys for service accounts
	return certs.CreateServiceAccountKeyAndPublicKeyFiles(cfg.CertificatesDir, cfg.ClusterConfiguration.PublicKeyAlgorithm())
}

func (c *Config) kubeConfigs(cfg *kubeadm.InitConfiguration, substrate *v1alpha1.Substrate) error {
	// Generate Kube config files for master components
	kubeConfigDir := path.Join(c.clusterConfigPath, aws.StringValue(discovery.Name(substrate)), kubeconfigPath)
	for _, kubeConfigFileName := range []string{
		kubeadmconstants.AdminKubeConfigFileName,
		kubeadmconstants.KubeletKubeConfigFileName,
		kubeadmconstants.ControllerManagerKubeConfigFileName,
		kubeadmconstants.SchedulerKubeConfigFileName} {
		if err := kubeconfig.CreateKubeConfigFile(kubeConfigFileName, kubeConfigDir, cfg); err != nil {
			return fmt.Errorf("creating %v, %w", kubeConfigFileName, err)
		}
	}
	return nil
}

func (c *Config) generateStaticPodManifests(cfg *kubeadm.InitConfiguration, substrate *v1alpha1.Substrate) error {
	manifestDir := path.Join(c.clusterConfigPath, aws.StringValue(discovery.Name(substrate)), clusterManifestPath)
	// etcd phase adds cfg.CertificatesDir to static pod yaml for pods to read the certs from
	cfg.CertificatesDir = certPKIPath
	if err := etcd.CreateLocalEtcdStaticPodManifestFile(
		manifestDir, "", cfg.NodeRegistration.Name, &cfg.ClusterConfiguration, &cfg.LocalAPIEndpoint, false); err != nil {
		return fmt.Errorf("error creating local etcd static pod manifest file %w", err)
	}
	for _, componentName := range []string{
		kubeadmconstants.KubeAPIServer,
		kubeadmconstants.KubeControllerManager,
		kubeadmconstants.KubeScheduler} {
		err := controlplane.CreateStaticPodFiles(path.Join(c.clusterConfigPath, aws.StringValue(discovery.Name(substrate)), clusterManifestPath), "",
			&cfg.ClusterConfiguration, &cfg.LocalAPIEndpoint, false, componentName)
		if err != nil {
			return fmt.Errorf("creating static pod file for %v, %w", componentName, err)
		}
	}
	return nil
}

func (c *Config) ensureBucket(ctx context.Context, substrate *v1alpha1.Substrate) error {
	if _, err := c.S3.CreateBucket(&s3.CreateBucketInput{Bucket: discovery.Name(substrate)}); err != nil {
		if err.(awserr.Error).Code() != s3.ErrCodeBucketAlreadyOwnedByYou {
			return fmt.Errorf("creating S3 bucket, %w", err)
		}
		logging.FromContext(ctx).Debugf("Found s3 bucket %s", aws.StringValue(discovery.Name(substrate)))
	} else {
		logging.FromContext(ctx).Infof("Created s3 bucket %s", aws.StringValue(discovery.Name(substrate)))
	}
	//sets tags on a bucket. Any existing tags are replaced.
	if _, err := c.S3.PutBucketTagging(&s3.PutBucketTaggingInput{
		Bucket: discovery.Name(substrate),
		Tagging: &s3.Tagging{TagSet: []*s3.Tag{{
			Key:   aws.String("kit.aws/substrate"),
			Value: aws.String(substrate.Name)},
		}},
	}); err != nil {
		return fmt.Errorf("adding tag %w", err)
	}
	return nil
}
func (c *Config) kubeletSystemService(cfg *kubeadm.InitConfiguration, substrate *v1alpha1.Substrate) error {
	localDir := path.Join(c.clusterConfigPath, aws.StringValue(discovery.Name(substrate)), kubeletSystemdPath)
	if _, err := os.Stat(localDir); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(localDir, 0777); err != nil {
			return err
		}
	}
	if err := ioutil.WriteFile(path.Join(localDir, "kubelet.service"), []byte(fmt.Sprintf(`[Unit]
After=docker.service iptables-restore.service
Requires=docker.service

[Service]
ExecStart=/usr/bin/kubelet --cluster-dns=10.96.0.10 --cluster-domain=cluster.local --hostname-override=%s --pod-manifest-path=/etc/kubernetes/manifests --kubeconfig=/etc/kubernetes/kubelet.conf  --cgroup-driver=systemd  --container-runtime=docker --network-plugin=cni --pod-infra-container-image=public.ecr.aws/eks-distro/kubernetes/pause:v1.18.9-eks-1-18-1 --node-labels=kit.aws/substrate=control-plane
Restart=always`, substrate.Name)), 0644); err != nil {
		return fmt.Errorf("writing kubelet configuration, %w", err)
	}
	return nil
}

func DefaultClusterConfig(substrate *v1alpha1.Substrate) *kubeadm.InitConfiguration {
	defaultStaticConfig, err := config.DefaultedStaticInitConfiguration()
	runtime.Must(err)
	defaultStaticConfig.ClusterConfiguration.KubernetesVersion = kubernetesVersionTag
	defaultStaticConfig.ClusterConfiguration.ImageRepository = imageRepository
	// In ECR coreDNS images are tagged as "v1.8.4-eks-1-21-9", EnsureDNSAddon
	// in kubeadm validates existing coreDNS image version by parsing the image
	// tags. It validates  in coreDNS library if the version is supported.
	// Versions in coreDNS library are in the format `1.8.4` and it fails to
	// validate. To get around this issue using the image from k8s.gcr.io which
	// are tagged in format `v1.8.4`
	defaultStaticConfig.ClusterConfiguration.DNS.ImageRepository = "k8s.gcr.io/coredns"
	defaultStaticConfig.ClusterConfiguration.DNS.ImageTag = "v1.8.6"
	// etcd specific config
	defaultStaticConfig.Etcd.Local = &kubeadm.LocalEtcd{
		ImageMeta:      kubeadm.ImageMeta{ImageRepository: etcdImageRepository, ImageTag: etcdVersionTag},
		ServerCertSANs: []string{"localhost", "127.0.0.1"},
		PeerCertSANs:   []string{"localhost", "127.0.0.1"},
		DataDir:        "/var/lib/etcd",
		ExtraArgs: map[string]string{
			"initial-cluster":             fmt.Sprintf("%s=https://127.0.0.1:2380", substrate.Name),
			"initial-cluster-state":       "new",
			"name":                        substrate.Name,
			"listen-peer-urls":            "https://127.0.0.1:2380",
			"listen-client-urls":          "https://127.0.0.1:2379",
			"advertise-client-urls":       "https://127.0.0.1:2379",
			"initial-advertise-peer-urls": "https://127.0.0.1:2380",
		},
	}
	// master specific config
	masterElasticIP := aws.StringValue(substrate.Status.Cluster.APIServerAddress)
	defaultStaticConfig.LocalAPIEndpoint.AdvertiseAddress = masterElasticIP
	defaultStaticConfig.LocalAPIEndpoint.BindPort = 8443
	defaultStaticConfig.ControlPlaneEndpoint = masterElasticIP + ":8443"
	defaultStaticConfig.APIServer.CertSANs = []string{masterElasticIP, substrate.Name,
		"kubernetes", "kubernetes.default", "kubernetes.default.svc", "kubernetes.default.svc.cluster.local", "10.96.0.1"}
	defaultStaticConfig.APIServer.ExtraArgs = map[string]string{
		"advertise-address": "0.0.0.0",
		"secure-port":       "8443",
		"authentication-token-webhook-config-file": "/var/aws-iam-authenticator/kubeconfig/kubeconfig.yaml",
	}
	defaultStaticConfig.APIServer.ExtraVolumes = []kubeadm.HostPathMount{{
		Name:      "authenticator-config",
		HostPath:  "/var/aws-iam-authenticator/kubeconfig/kubeconfig.yaml",
		MountPath: "/var/aws-iam-authenticator/kubeconfig/kubeconfig.yaml",
		ReadOnly:  true,
		PathType:  v1.HostPathFileOrCreate,
	}}
	if defaultStaticConfig.Scheduler.ExtraArgs == nil {
		defaultStaticConfig.Scheduler.ExtraArgs = map[string]string{}
	}
	if defaultStaticConfig.ControllerManager.ExtraArgs == nil {
		defaultStaticConfig.ControllerManager.ExtraArgs = map[string]string{}
	}
	defaultStaticConfig.NodeRegistration = kubeadm.NodeRegistrationOptions{
		Name: substrate.Name,
		KubeletExtraArgs: map[string]string{"cgroup-driver": "systemd", "network-plugin": "cni",
			"pod-infra-container-image": imageRepository + "/pause:" + kubernetesVersionTag,
		},
	}
	return defaultStaticConfig
}

func (c *Config) ensureAuthenticatorConfig(ctx context.Context, substrate *v1alpha1.Substrate) error {
	identity, err := c.STS.GetCallerIdentityWithContext(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("getting caller identity, %w", err)
	}
	configMap, err := iamauthenticator.Config(ctx, substrate.Name, substrate.Namespace,
		aws.StringValue(discovery.Name(substrate, TenantControlPlaneNodeRole)), aws.StringValue(identity.Account))
	if err != nil {
		return fmt.Errorf("creating authenticator config, %w", err)
	}
	logging.FromContext(ctx).Debugf("Created config map for authenticator")
	configDir := path.Join(c.clusterConfigPath, aws.StringValue(discovery.Name(substrate)), authenticatorConfigDir)
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("failed to create directory, %w", err)
	}
	if err := ioutil.WriteFile(path.Join(configDir, "config.yaml"), []byte(configMap.Data["config.yaml"]), 0644); err != nil {
		return fmt.Errorf("writing authenticator config, %w", err)
	}
	return nil
}

func (c *Config) staticPodSpecForAuthenticator(ctx context.Context, substrate *v1alpha1.Substrate) error {
	podTemplateSpec := iamauthenticator.PodSpec(substrate.Name, func(template v1.PodTemplateSpec) v1.PodTemplateSpec {
		template.ObjectMeta.Namespace = "kube-system"
		template.Spec.Volumes = append(template.Spec.Volumes, v1.Volume{Name: "config",
			VolumeSource: v1.VolumeSource{HostPath: &v1.HostPathVolumeSource{Path: authenticatorConfigDir}},
		})
		return template
	})
	serialized, err := kubeadmutil.MarshalToYaml(
		&v1.Pod{ObjectMeta: podTemplateSpec.ObjectMeta, Spec: podTemplateSpec.Spec}, v1.SchemeGroupVersion)
	if err != nil {
		return fmt.Errorf("failed to marshal config map manifest, %w", err)
	}
	if err := ioutil.WriteFile(path.Join(c.clusterConfigPath, aws.StringValue(discovery.Name(substrate)),
		clusterManifestPath, "aws-iam-authenticator.yaml"), serialized, 0644); err != nil {
		return fmt.Errorf("writing authenticator pod yaml, %w", err)
	}
	return nil
}

func (c *Config) ensureKitEnvDir() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("finding HOME dir %v", err)
	}
	c.clusterConfigPath = filepath.Join(home, clusterConfigDir)
	if err := os.MkdirAll(c.clusterConfigPath, 0755); err != nil {
		return fmt.Errorf("creating cluster config dir %v", err)
	}
	return nil
}

// DirectoryIterator represents an iterator of a specified directory
type DirectoryIterator struct {
	filePaths []string
	bucket    string
	localDir  string
	next      struct {
		path string
		f    *os.File
	}
	err error
}

// NewDirectoryIterator builds a new DirectoryIterator
func NewDirectoryIterator(bucket, dir string) s3manager.BatchUploadIterator {
	var paths []string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	return &DirectoryIterator{
		filePaths: paths,
		bucket:    bucket,
		localDir:  dir,
	}
}

// Next returns whether next file exists or not
func (d *DirectoryIterator) Next() bool {
	if len(d.filePaths) == 0 {
		d.next.f = nil
		return false
	}
	d.next.f, d.err = os.Open(d.filePaths[0])
	d.next.path = d.filePaths[0]
	d.filePaths = d.filePaths[1:]
	return true && d.Err() == nil
}

// Err returns error of DirectoryIterator
func (d *DirectoryIterator) Err() error {
	return d.err
}

// UploadObject uploads a file
func (d *DirectoryIterator) UploadObject() s3manager.BatchUploadObject {
	// trim the local path before uploading to S3
	d.next.path = strings.TrimPrefix(d.next.path, d.localDir)
	return s3manager.BatchUploadObject{
		Object: &s3manager.UploadInput{Bucket: &d.bucket, Key: &d.next.path, Body: d.next.f},
		After:  d.next.f.Close,
	}
}
