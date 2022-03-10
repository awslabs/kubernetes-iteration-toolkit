# kitctl

## Installing kitctl

```bash
brew tap awslabs/kit https://github.com/awslabs/kubernetes-iteration-toolkit.git
brew install kitctl
```

## Usage
kitctl helps provision an AWS infrastructure environment to deploy Kubernetes clusters using kit-operator. It runs a single node kubernetes cluster in a VPC installed with all the required controllers like Karpeneter, KIT-operator, ELB controller and EBS CSI Driver to manage the lifecycle of clusters. kitctl also creates the necessary IAM permissions required for these controllers.
To get started make sure you have admin access to AWS.

### Bootstrap an environment in AWS to run Kubernetes clusters using kit-operator

```bash
kitctl bootstrap
```
> Set KUBECONFIG to access the environment with the kubeconfig location provided from this command

### Configure Karpenter to provision nodes for the Kubernetes cluster control plane

```bash
GUEST_CLUSTER_NAME=foo # Desired Cluster name
SUBSTRATE_CLUSTER_NAME=test-substrate
```

```bash
cat <<EOF | kubectl apply -f -
apiVersion: karpenter.sh/v1alpha5
kind: Provisioner
metadata:
  name: master-nodes
spec:
  kubeletConfiguration:
    clusterDNS:
      - "10.96.0.10"
  labels:
    kit.k8s.sh/app: ${GUEST_CLUSTER_NAME}-apiserver
    kit.k8s.sh/control-plane-name: ${GUEST_CLUSTER_NAME}
  provider:
    instanceProfile: kit-${SUBSTRATE_CLUSTER_NAME}-tenant-controlplane-node-role
    subnetSelector:
      karpenter.sh/discovery: ${SUBSTRATE_CLUSTER_NAME}
    securityGroupSelector:
      kubernetes.io/cluster/${SUBSTRATE_CLUSTER_NAME}: owned
    tags:
      kit.aws/substrate: ${SUBSTRATE_CLUSTER_NAME}
  ttlSecondsAfterEmpty: 30
EOF
```

```bash
cat <<EOF | kubectl apply -f -
apiVersion: karpenter.sh/v1alpha5
kind: Provisioner
metadata:
  name: etcd-nodes
spec:
  kubeletConfiguration:
    clusterDNS:
      - "10.96.0.10"
  labels:
    kit.k8s.sh/app: ${GUEST_CLUSTER_NAME}-etcd
    kit.k8s.sh/control-plane-name: ${GUEST_CLUSTER_NAME}
  provider:
    instanceProfile: kit-${SUBSTRATE_CLUSTER_NAME}-tenant-controlplane-node-role
    subnetSelector:
      karpenter.sh/discovery: ${SUBSTRATE_CLUSTER_NAME}
    securityGroupSelector:
      kubernetes.io/cluster/${SUBSTRATE_CLUSTER_NAME}: owned
    tags:
      kit.aws/substrate: ${SUBSTRATE_CLUSTER_NAME}
  ttlSecondsAfterEmpty: 30
EOF
```

### Provision a Kubernetes cluster control plane

```bash
cat <<EOF | kubectl apply -f -
apiVersion: kit.k8s.sh/v1alpha1
kind: ControlPlane
metadata:
  name: ${GUEST_CLUSTER_NAME} # Desired Cluster name
spec:
  etcd:
    replicas: 3
  master:
    apiServer:
      replicas: 1
EOF
```

### cleanup

- To remove the kubernetes cluster provisioned using kit-operator

```bash
kubectl delete controlplane ${GUEST_CLUSTER_NAME}
```

- To clean up the AWS environment

```bash
kitctl helps provision an AWS infrastructure environment to deploy Kubernetes clusters using kit-operator. It runs a single node kubernetes cluster with all the required controllers like Karpeneter, KIT-operator, ELB controller and EBS CSI Driver to manage the lifecycle of clusters. kitctl also creates the necessary delete
```

### Debug logs
Run the `kitctl helps provision an AWS infrastructure environment to deploy Kubernetes clusters using kit-operator. It runs a single node kubernetes cluster with all the required controllers like Karpeneter, KIT-operator, ELB controller and EBS CSI Driver to manage the lifecycle of clusters. kitctl also creates the necessary` commands with `--debug` flag