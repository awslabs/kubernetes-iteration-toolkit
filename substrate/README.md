# kitctl

## Installing kitctl

```bash
brew tap awslabs/kit https://github.com/awslabs/kubernetes-iteration-toolkit.git
brew install kitctl
or
go install github.com/awslabs/kubernetes-iteration-toolkit/substrate/cmd/kitctl@latest
```

## Usage
kitctl helps provision an AWS infrastructure environment to deploy Kubernetes clusters using kit-operator. It runs a single node kubernetes cluster in a VPC installed with all the required controllers like Karpeneter, KIT-operator, ELB controller and EBS CSI Driver to manage the lifecycle of clusters. kitctl also creates the necessary IAM permissions required for these controllers.
To get started make sure you have admin access to AWS.

### Bootstrap an environment in AWS to run Kubernetes clusters using kit-operator

```bash
export AWS_REGION=us-west-2
kitctl bootstrap kitctl-$(whoami) # Optional environment name
```
> Set KUBECONFIG to access the environment with the kubeconfig location provided from this command

### Provision a Kubernetes cluster control plane

```bash
GUEST_CLUSTER_NAME=foo # Desired Cluster name
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
kitctl delete kitctl-$(whoami) # Optional environment name
```

### Debug logs
Run the commands with `--debug` flag to get more detailed logs