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

### Get the KUBECONFIG for the guest cluster from the management cluster

```bash
kubectl get secret ${GUEST_CLUSTER_NAME}-kube-admin-config -ojsonpath='{.data.config}' | base64 -d > /tmp/kubeconfig
```

### Deploy CNI plugin to the guest cluster for the nodes to be ready

If you are deploying in us-west-2 region run the following command to install AWS CNI plugin, for other regions follow the setup [steps](https://github.com/aws/amazon-vpc-cni-k8s#setup)

```bash
kubectl --kubeconfig=/tmp/kubeconfig apply -f https://raw.githubusercontent.com/aws/amazon-vpc-cni-k8s/release-1.10/config/master/aws-k8s-cni.yaml
```

### Provision worker nodes for the guest cluster

```bash
cat <<EOF | kubectl apply -f -
apiVersion: kit.k8s.sh/v1alpha1
kind: DataPlane
metadata:
  name: ${GUEST_CLUSTER_NAME}-nodes
spec:
  clusterName: ${GUEST_CLUSTER_NAME} # Associated Cluster name
  nodeCount: 1
EOF
```

### Accessing metrics from prometheus and grafana

```bash
kubectl port-forward svc/prometheus-operated -n monitoring 9090:9090&
kubectl port-forward svc/kube-prometheus-stack-grafana -n monitoring 8080:80&
```

Adding additional Grafana dashboards from [monitoring](monitoring/)

```bash
kubectl create configmap master-dashboard -n monitoring --from-file=master-dashboard.json
kubectl label configmap master-dashboard -n monitoring  grafana_dashboard=1
```

> Note: Coming soon, auto-load these dashboards when a KIT environment is created

### Allowing API server to trust kubelet endpoints for the guest cluster

```bash
kubectl certificate approve $(kubectl get csr | grep "Pending" | awk '{print $1}')
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