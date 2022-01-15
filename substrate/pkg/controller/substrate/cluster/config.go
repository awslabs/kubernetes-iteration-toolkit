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
	"fmt"
	"os"
	"path"
	"path/filepath"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kit/substrate/pkg/utils/discovery"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
	kubeadmconstants "k8s.io/kubernetes/cmd/kubeadm/app/constants"
	certsphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/certs"
	"k8s.io/kubernetes/cmd/kubeadm/app/phases/controlplane"
	etcdphase "k8s.io/kubernetes/cmd/kubeadm/app/phases/etcd"
	configutil "k8s.io/kubernetes/cmd/kubeadm/app/util/config"
	"knative.dev/pkg/logging"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	clusterCertsBasePath = "/tmp/"
	kubeconfigPath       = "/etc/kubernetes"
	certPKIPath          = "/etc/kubernetes/pki"
	clusterManifestPath  = "/etc/kubernetes/manifests"
	kubeletSystemdPath   = "/etc/systemd/system/kubelet.service.d/"
	kubeletConfigPath    = "/var/lib/kubelet/"
	kubernetesVersionTag = "v1.21.2-eks-1-21-4"
	imageRepository      = "public.ecr.aws/eks-distro/kubernetes"
)

type Config struct {
	S3         *s3.S3
	S3Uploader *s3manager.Uploader
}

func (c *Config) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if substrate.Status.Cluster.Address == nil {
		return reconcile.Result{Requeue: true}, nil
	}
	// ensure S3 bucket
	if err := c.ensureBucket(ctx, substrate); err != nil {
		return reconcile.Result{}, fmt.Errorf("ensuring S3 bucket, %w", err)
	}
	// create all configs file
	cfg := defaultClusterConfig(substrate)
	if err := c.generateCerts(cfg, substrate); err != nil {
		return reconcile.Result{}, fmt.Errorf("generating certs, %w", err)
	}
	if err := c.generateStaticPodManifests(cfg, substrate); err != nil {
		return reconcile.Result{}, fmt.Errorf("generating manifests, %w", err)
	}
	// upload to s3 bucket
	if err := c.S3Uploader.UploadWithIterator(ctx, NewDirectoryIterator(
		aws.StringValue(discovery.Name(substrate)), path.Join(clusterCertsBasePath, substrate.Name))); err != nil {
		return reconcile.Result{}, fmt.Errorf("uploading to S3 %w", err)
	}
	logging.FromContext(ctx).Infof("Uploaded cluster configuration to s3://%s", aws.StringValue(discovery.Name(substrate)))
	return reconcile.Result{}, nil
}

func (c *Config) Delete(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	// delete the s3 bucket
	if err := s3manager.NewBatchDeleteWithClient(c.S3).Delete(ctx, s3manager.NewDeleteListIterator(
		c.S3, &s3.ListObjectsInput{Bucket: discovery.Name(substrate)})); err != nil {
		return reconcile.Result{}, fmt.Errorf("deleting objects from bucket %s, %w", aws.StringValue(discovery.Name(substrate)), err)
	}
	if _, err := c.S3.DeleteBucketWithContext(ctx, &s3.DeleteBucketInput{Bucket: discovery.Name(substrate)}); err != nil {
		return reconcile.Result{}, fmt.Errorf("deleting S3, %w", err)
	}
	logging.FromContext(ctx).Infof("Deleted S3 bucket %s", aws.StringValue(discovery.Name(substrate)))
	return reconcile.Result{}, nil
}

func (c *Config) generateCerts(cfg *kubeadm.InitConfiguration, substrate *v1alpha1.Substrate) error {
	cfg.CertificatesDir = path.Join(clusterCertsBasePath, substrate.Name, certPKIPath)
	certTree, err := certsphase.GetDefaultCertList().AsMap().CertTree()
	if err != nil {
		return err
	}
	if err := certTree.CreateTree(cfg); err != nil {
		return fmt.Errorf("error creating cert tree, %w", err)
	}
	return nil
}

func (c *Config) generateStaticPodManifests(cfg *kubeadm.InitConfiguration, substrate *v1alpha1.Substrate) error {
	manifestDir := path.Join(clusterCertsBasePath, substrate.Name, clusterManifestPath)
	// etcd phase adds cfg.CertificatesDir to static pod yaml for pods to read the certs from
	cfg.CertificatesDir = certPKIPath
	if err := etcdphase.CreateLocalEtcdStaticPodManifestFile(
		manifestDir, "", cfg.NodeRegistration.Name, &cfg.ClusterConfiguration, &cfg.LocalAPIEndpoint, false); err != nil {
		return fmt.Errorf("error creating local etcd static pod manifest file %w", err)
	}
	for _, componentName := range []string{
		kubeadmconstants.KubeAPIServer,
		kubeadmconstants.KubeControllerManager,
		kubeadmconstants.KubeScheduler} {
		err := controlplane.CreateStaticPodFiles(path.Join(clusterCertsBasePath, substrate.Name, clusterManifestPath), "",
			&cfg.ClusterConfiguration, &cfg.LocalAPIEndpoint, false, componentName)
		if err != nil {
			return fmt.Errorf("creating static pod file for %v, %w", componentName, err)
		}
	}
	return nil
}

func (c *Config) ensureBucket(ctx context.Context, substrate *v1alpha1.Substrate) error {
	if _, err := c.S3.CreateBucket(&s3.CreateBucketInput{Bucket: discovery.Name(substrate),
		CreateBucketConfiguration: &s3.CreateBucketConfiguration{LocationConstraint: c.S3.Config.Region},
	}); err != nil {
		if err.(awserr.Error).Code() != s3.ErrCodeBucketAlreadyOwnedByYou {
			return fmt.Errorf("creating S3 bucket, %w", err)
		}
		logging.FromContext(ctx).Infof("Found s3 bucket %s", aws.StringValue(discovery.Name(substrate)))
	} else {
		logging.FromContext(ctx).Infof("Created s3 bucket %s", aws.StringValue(discovery.Name(substrate)))
	}
	return nil
}

func defaultClusterConfig(substrate *v1alpha1.Substrate) *kubeadm.InitConfiguration {
	defaultStaticConfig, err := configutil.DefaultedStaticInitConfiguration()
	runtime.Must(err)
	// etcd specific config
	defaultStaticConfig.ClusterConfiguration.KubernetesVersion = kubernetesVersionTag
	defaultStaticConfig.ClusterConfiguration.ImageRepository = imageRepository
	defaultStaticConfig.Etcd.Local = &kubeadm.LocalEtcd{
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
	masterElasticIP := aws.StringValue(substrate.Status.Cluster.Address)
	defaultStaticConfig.LocalAPIEndpoint.AdvertiseAddress = masterElasticIP
	defaultStaticConfig.LocalAPIEndpoint.BindPort = 443
	defaultStaticConfig.ControlPlaneEndpoint = masterElasticIP + ":443"
	defaultStaticConfig.APIServer.CertSANs = []string{masterElasticIP, substrate.Name,
		"kubernetes", "kubernetes.default", "kubernetes.default.svc", "kubernetes.default.svc.cluster.local", "10.96.0.1"}
	defaultStaticConfig.APIServer.ExtraArgs = map[string]string{
		"advertise-address": masterElasticIP,
		"secure-port":       "443",
	}
	if defaultStaticConfig.Scheduler.ExtraArgs == nil {
		defaultStaticConfig.Scheduler.ExtraArgs = map[string]string{}
	}
	if defaultStaticConfig.ControllerManager.ExtraArgs == nil {
		defaultStaticConfig.ControllerManager.ExtraArgs = map[string]string{}
	}
	defaultStaticConfig.NodeRegistration = kubeadm.NodeRegistrationOptions{Name: substrate.Name}
	return defaultStaticConfig
}

// DirectoryIterator represents an iterator of a specified directory
type DirectoryIterator struct {
	filePaths []string
	bucket    string
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
	// f := d.next.f
	return s3manager.BatchUploadObject{
		Object: &s3manager.UploadInput{Bucket: &d.bucket, Key: &d.next.path, Body: d.next.f},
		After:  d.next.f.Close,
	}
}
