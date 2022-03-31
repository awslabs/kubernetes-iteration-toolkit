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

package etcd

import (
	"context"
	"fmt"

	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/apis/controlplane/v1alpha1"
	"github.com/awslabs/kubernetes-iteration-toolkit/operator/pkg/utils/object"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Reconciles a ConfigMap that contains a bash script used for etcd cluster bootstrapping. This ConfigMap is mounted
// as a volume in pods created as part of the etcd StatefulSet. This bash script is available in the container at
// /etc/kubernetes/bootstrap.sh and executed in the container on start-up. Container arguments to this script
// are passed directly to the etcd binary. Note that we manually construct a v1.ConfigMap struct rather than calling
// object.GenerateConfigMap because double quote characters are not correctly populated in the ConfigMap by the UniversalDecoder.
func (c *Controller) reconcileBootstrapConfig(ctx context.Context, controlPlane *v1alpha1.ControlPlane) error {
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      bootstrapConfigMapName(controlPlane.ClusterName()),
			Namespace: controlPlane.Namespace,
		},
		Data: map[string]string{
			"bootstrap.sh": bootstrapCmd(controlPlane),
		},
	}
	return c.kubeClient.EnsurePatch(ctx, &v1.ConfigMap{}, object.WithOwner(controlPlane, configMap))
}

// bootstrapAdvertizePeerURL is the same as advertizePeerURL but uses curly brackets rather than parentheses around
// the NODE_ID environment variable. When executing the bootstrap Configmap as a bash script in the etcd container,
// we will get this error if we try using parentheses: ./etc/kubernetes/bootstrap.sh: line 27: NODE_ID: command not found
func bootstrapCmd(controlPlane *v1alpha1.ControlPlane) string {
	etcdctlEndpoint := fmt.Sprintf("https://%s:2379", serviceFQDN(controlPlane))
	bootstrapAdvertizePeerURL := fmt.Sprintf("https://${NODE_ID}.%s-etcd.%s.svc.cluster.local:2380",
		controlPlane.ClusterName(), controlPlane.Namespace)
	return fmt.Sprintf(bootstrapScript, etcdctlEndpoint, bootstrapAdvertizePeerURL)
}

func bootstrapConfigMapName(clusterName string) string {
	return fmt.Sprintf("%s-etcd-bootstrap", clusterName)
}

// All etcd arguments are passed directly from the container arguments except for the initial-cluster-state flag which
// is conditionally set by this bash script
var bootstrapScript = `#!/bin/sh

CERT_FLAGS="--cacert /etc/kubernetes/pki/etcd/server/server.crt --cert /etc/kubernetes/pki/etcd/peer/peer.crt --key /etc/kubernetes/pki/etcd/peer/peer.key"
ETCD_INITIAL_CLUSTER_STATE=existing
ENDPOINT_FLAG="--endpoints %[1]s"
echo "etcd_bootstrap: using etcdctl endpoints $ENDPOINT_FLAG"
# if this call to list members fails or if this member has an empty advertise client URL, assume we're 
# bootstrapping a new etcd member. A member that hasn't had its etcd process started will have an empty advertise client address
MEMBER_INFO=$(etcdctl $CERT_FLAGS $ENDPOINT_FLAG member list)
ETCDCTL_EXIT_CODE=$?
HAS_EMTPY_ADVERTISE_ADDRESS=$(echo "$MEMBER_INFO" | grep $NODE_ID | grep ", ,")
if [ "$ETCDCTL_EXIT_CODE" -ne 0 ] || [ -n "$HAS_EMTPY_ADVERTISE_ADDRESS" ]; then
  ETCD_INITIAL_CLUSTER_STATE=new
fi
echo "etcd_bootstrap: cluster state $ETCD_INITIAL_CLUSTER_STATE"
echo "etcd_bootstrap: member info $MEMBER_INFO"
echo "etcd_bootstrap: current node $NODE_ID"
MEMBER_INFO_FOR_CURRENT_NODE=$(echo "$MEMBER_INFO" | grep $NODE_ID | grep -v ", ,")
CONTENTS=$(ls -A /var/lib/etcd/member)
echo "etcd_bootstrap: contents of /var/lib/etcd/member $CONTENTS"
# if this pod is already a member that has been started, it has its advertise client address populated
# from having the etcd process previously started, and it doesn't have the contents of its data-dir,
# remove the member and a new member back
if [ -n "$MEMBER_INFO_FOR_CURRENT_NODE" ] && [ -z "$(ls -A /var/lib/etcd/member)" ]; then
  echo "etcd_bootstrap: we're removing the current member and adding back"
  etcdctl $CERT_FLAGS $ENDPOINT_FLAG endpoint health
  MEMBER_ID=$(echo $MEMBER_INFO_FOR_CURRENT_NODE | awk '{print $1}' | sed 's/.$//')
  echo "etcd_bootstrap: removing member $MEMBER_ID"
  etcdctl $CERT_FLAGS $ENDPOINT_FLAG member remove $MEMBER_ID
  echo "etcd_bootstrap: adding member back"
  etcdctl $CERT_FLAGS $ENDPOINT_FLAG member add $NODE_ID --peer-urls %[2]s
fi
etcd $@ --initial-cluster-state=$ETCD_INITIAL_CLUSTER_STATE
`
