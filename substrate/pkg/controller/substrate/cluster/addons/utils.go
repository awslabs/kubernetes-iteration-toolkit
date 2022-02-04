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

package addons

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path"
	"syscall"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	"github.com/awslabs/kit/substrate/pkg/controller/substrate/cluster"
	"github.com/awslabs/kit/substrate/pkg/utils/discovery"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"
)

var (
	ErrWaitingForSubstrateEndpoint = fmt.Errorf("waiting for substrate cluster endpoint to be available")
)

const (
	kubeAdminFilePath = "etc/kubernetes/admin.conf"
)

func KubeClientFor(ctx context.Context, substrate *v1alpha1.Substrate) (*kubernetes.Clientset, error) {
	// check if the kube-admin file for the cluster exists
	kubeAdminFile := path.Join(cluster.ClusterCertsBasePath, aws.StringValue(discovery.Name(substrate)), kubeAdminFilePath)
	if _, err := os.Stat(kubeAdminFile); errors.Is(err, os.ErrNotExist) {
		return nil, ErrWaitingForSubstrateEndpoint
	} else if err != nil {
		return nil, err
	}
	// create kubernetes interface to substrate cluster
	client, err := kubeconfig.ClientSetFromFile(kubeAdminFile)
	if err != nil {
		return nil, fmt.Errorf("creating Kube client from admin config, %w", err)
	}
	namespaces, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		var netErr *net.OpError
		var syscallErr *os.SyscallError
		if os.IsTimeout(err) ||
			(errors.As(err, &netErr) && errors.As(netErr.Err, &syscallErr) && errors.Is(syscallErr.Err, syscall.ECONNREFUSED)) {
			return nil, ErrWaitingForSubstrateEndpoint
		}
		return nil, fmt.Errorf("verifying control plane ready, %w", err)
	}
	// We check for the kube-system namespace to be created, because there can be a race condition where
	// API server endpoint is available but not everything is synced, and kube-system namespace is not yet created.
	// In such cases, other addons deployment will fail because kube-system namespace doesnot exist yet.
	for _, namespace := range namespaces.Items {
		if namespace.Name == "kube-system" {
			return client, nil
		}
	}
	return nil, ErrWaitingForSubstrateEndpoint
}
