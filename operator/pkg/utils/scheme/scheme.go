package scheme

import (
	"github.com/awslabs/kit/operator/pkg/apis/controlplane/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

var (
	ManagementCluster = runtime.NewScheme()
	KitCluster        = runtime.NewScheme()
)

func init() {
	_ = clientgoscheme.AddToScheme(ManagementCluster)
	_ = v1alpha1.AddToScheme(ManagementCluster)

	_ = clientgoscheme.AddToScheme(KitCluster)
}
