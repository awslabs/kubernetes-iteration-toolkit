package cni

import (
	"fmt"
	"path"

	"go.uber.org/zap"
	apps "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	kuberuntime "k8s.io/apimachinery/pkg/runtime"
	clientset "k8s.io/client-go/kubernetes"
	clientsetscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/kubernetes/cmd/kubeadm/app/util/apiclient"
	kubeconfigutil "k8s.io/kubernetes/cmd/kubeadm/app/util/kubeconfig"
)

func main() {

	kubeconfigAdminPath := path.Join("/tmp", "foo", "i-0ec8e8c81720914fc", "etc/kubernetes/admin.conf")
	client, err := kubeconfigutil.ClientSetFromFile(kubeconfigAdminPath)
	if err != nil {
		return
	}
	if err := EnsureCNIAddOn(client); err != nil {
		zap.S().Infof("Failed creating CNI Addon, %v", err)
		return
	}

}
func EnsureCNIAddOn(client clientset.Interface) error {
	zap.S().Info("Creating addon CNI")
	cniClusterRoles := &rbac.ClusterRole{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), []byte(CNIClusterRole), cniClusterRoles); err != nil {
		return fmt.Errorf("decoding CNI cluster role, %w", err)
	}
	// Create the Clusterroles for weaveCNI or update it in case it already exists
	if err := apiclient.CreateOrUpdateClusterRole(client, cniClusterRoles); err != nil {
		return err
	}

	cniClusterRolesBinding := &rbac.ClusterRoleBinding{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), []byte(CNIClusterRoleBinding), cniClusterRolesBinding); err != nil {
		return fmt.Errorf("decoding CNI cluster role binding, %w", err)
	}
	// Create the role bindings for cni or update it in case it already exists
	if err := apiclient.CreateOrUpdateClusterRoleBinding(client, cniClusterRolesBinding); err != nil {
		return err
	}

	cniServiceAccount := &v1.ServiceAccount{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), []byte(CNIServiceAccount), cniServiceAccount); err != nil {
		return fmt.Errorf("decoding CNI service account, %w", err)
	}
	// Create the service account for cni or update it in case it already exists
	if err := apiclient.CreateOrUpdateServiceAccount(client, cniServiceAccount); err != nil {
		return err
	}

	cniRoles := &rbac.Role{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), []byte(CNIRole), cniRoles); err != nil {
		return fmt.Errorf("decoding CNI role, %w", err)
	}
	// Create the role for weaveCNI or update it in case it already exists
	if err := apiclient.CreateOrUpdateRole(client, cniRoles); err != nil {
		return err
	}

	cniRoleBinding := &rbac.RoleBinding{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), []byte(CNIRoleBinding), cniRoleBinding); err != nil {
		return fmt.Errorf("decoding CNI role binding, %w", err)
	}
	// Create the rolebindings for cni or update it in case it already exists
	if err := apiclient.CreateOrUpdateRoleBinding(client, cniRoleBinding); err != nil {
		return err
	}

	cniDaemonSet := &apps.DaemonSet{}
	if err := kuberuntime.DecodeInto(clientsetscheme.Codecs.UniversalDecoder(), []byte(CNIDaemonSet), cniDaemonSet); err != nil {
		return fmt.Errorf("decoding CNI daemonset, %w", err)
	}
	// Create the Daemon set for cni or update it in case it already exists
	if err := apiclient.CreateOrUpdateDaemonSet(client, cniDaemonSet); err != nil {
		return err
	}
	return nil
}
