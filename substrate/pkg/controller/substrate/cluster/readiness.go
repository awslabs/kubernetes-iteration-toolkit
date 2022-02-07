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
	"net"
	"net/http"
	"os"
	"syscall"

	"github.com/awslabs/kit/substrate/pkg/apis/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	kubeconfigFile = "etc/kubernetes/admin.conf"
)

// Readiness checks if the substrate API server endpoint it ready and sets the
// ready status on the *v1alpha1.Substrate object indicating other controllers
// like kube-proxy, rbac to proceed
type Readiness struct{}

func (r *Readiness) Create(ctx context.Context, substrate *v1alpha1.Substrate) (reconcile.Result, error) {
	if substrate.Status.Cluster.KubeConfig == nil {
		return reconcile.Result{Requeue: true}, nil
	}
	// create kubernetes interface to substrate cluster
	client, err := kubeconfig.ClientSetFromFile(*substrate.Status.Cluster.KubeConfig)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("creating Kube client from admin config, %w", err)
	}
	response := client.RESTClient().Get().AbsPath("/readyz").Do(ctx)
	if response.Error() != nil {
		var netErr *net.OpError
		var syscallErr *os.SyscallError
		var statusErr *apierrors.StatusError
		// When the instance is still coming up a timeout error is seen.
		// When the instance up but API server is not yet running, connection refused error is seen.
		// When API server is up but not yet ready, a 5xx error is returned by the server
		if os.IsTimeout(response.Error()) ||
			(errors.As(response.Error(), &netErr) && errors.As(netErr.Err, &syscallErr) && errors.Is(syscallErr.Err, syscall.ECONNREFUSED)) ||
			(errors.As(response.Error(), &statusErr) && statusErr.Status().Code != http.StatusOK) {
			return reconcile.Result{Requeue: true}, nil
		}
		return reconcile.Result{Requeue: true}, fmt.Errorf("verifying control plane ready, %w, %#v", response.Error(), response.Error())
	}
	result, err := response.Raw()
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("getting response result, %w", err)
	}
	if string(result) == "ok" {
		substrate.Ready()
		return reconcile.Result{}, nil
	}
	return reconcile.Result{}, fmt.Errorf("api server not yet ready status is %v", string(result))
}

func (r *Readiness) Delete(_ context.Context, _ *v1alpha1.Substrate) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
