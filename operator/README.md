# KIT (Kubernetes Iteration Toolkit) Operator

KIT-operator is an experimental project that makes it easy to provision a Kubernetes cluster in AWS. It runs a controller in an existing Kubernetes cluster, users can provision a new cluster by following the steps listed below.
By default, operator deploys Kubernetes images from the [Amazon EKS Distro](https://distro.eks.amazonaws.com/).
Currently, the operator can only be deployed on an existing (substrate) Kubernetes cluster and depends on other controllers (listed below) to be able to provison a guest cluster in AWS.

KIT uses the [operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) in Kubernetes to regularly reconcile the cluster state in AWS to the desired state defined in the spec. KIT follows the battery included model where a cluster can be created with passing just the cluster name.

## Getting started

### Overview

- Create a Kubernetes (management) Cluster
- Install the AWS Load Balancer Controller
- Install AWS EBS CSI Driver
- Install Karpenter
- Install the KIT operator

> Note: All these install steps can be skipped when using [kitctl](https://github.com/awslabs/kubernetes-iteration-toolkit/tree/main/substrate) to provision an environment.

### Create a management cluster with eksctl
```bash
MANAGEMENT_CLUSTER_NAME=kit-management-cluster
GUEST_CLUSTER_NAME=example
AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
AWS_REGION=us-west-2
```

This will create a management cluster with the necessary tags for Karpenter:
```bash
cat <<EOF > cluster.yaml
---
apiVersion: eksctl.io/v1alpha5
kind: ClusterConfig
metadata:
  name: ${MANAGEMENT_CLUSTER_NAME}
  region: ${AWS_REGION}
  version: "1.21"
  tags:
    karpenter.sh/discovery: ${MANAGEMENT_CLUSTER_NAME}
managedNodeGroups:
  - instanceType: m5.large
    amiFamily: AmazonLinux2
    name: ${MANAGEMENT_CLUSTER_NAME}-ng
    desiredCapacity: 1
    minSize: 1
    maxSize: 10
iam:
  withOIDC: true
EOF
eksctl create cluster -f cluster.yaml
 ```
### Install the AWS Load Balancer Controller
- [AWS Load balancer controller](https://docs.aws.amazon.com/eks/latest/userguide/aws-load-balancer-controller.html)

Create the IAM policy that will be attached to the IAM role used by the AWS Load Balancer Controller
```bash
curl -o iam_policy.json https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/v2.3.1/docs/install/iam_policy.json
aws iam create-policy \
    --policy-name AWSLoadBalancerControllerIAMPolicy \
    --policy-document file://iam_policy.json
```
Create the IAM role and associate it to the load balancer controller service account.
```bash
eksctl create iamserviceaccount \
  --cluster=${MANAGEMENT_CLUSTER_NAME} \
  --namespace=kube-system \
  --name=aws-load-balancer-controller \
  --attach-policy-arn=arn:aws:iam::${AWS_ACCOUNT_ID}:policy/AWSLoadBalancerControllerIAMPolicy \
  --override-existing-serviceaccounts \
  --approve
```
Install the controller on your cluster.
```bash
helm repo add eks https://aws.github.io/eks-charts
helm repo update
helm install aws-load-balancer-controller eks/aws-load-balancer-controller \
  -n kube-system \
  --set clusterName=${MANAGEMENT_CLUSTER_NAME} \
  --set serviceAccount.create=false \
  --set replicaCount=1 \
  --set serviceAccount.name=aws-load-balancer-controller 
```

### Install AWS EBS CSI Driver on your cluster

- [AWS EBS CSI Driver](https://github.com/kubernetes-sigs/aws-ebs-csi-driver)

Create an IAM policy that will be attached to the EBS role.

```bash
curl -o example-iam-policy.json https://raw.githubusercontent.com/kubernetes-sigs/aws-ebs-csi-driver/v1.5.1/docs/example-iam-policy.json
aws iam create-policy \
    --policy-name AmazonEBSCSIDriverServiceRolePolicy \
    --policy-document file://example-iam-policy.json
```

Create an IAM role that will be assumed by the CSI driver to access AWS APIS.

```bash
eksctl create iamserviceaccount \
    --name=ebs-csi-controller-sa \
    --namespace=kube-system \
    --cluster=${MANAGEMENT_CLUSTER_NAME} \
    --attach-policy-arn=arn:aws:iam::${AWS_ACCOUNT_ID}:policy/AmazonEBSCSIDriverServiceRolePolicy \
    --approve \
    --override-existing-serviceaccounts \
    --role-name AmazonEKS_EBS_CSI_DriverRole
```

Install the aws-ebs-csi-driver in the management cluster

```bash
helm repo add aws-ebs-csi-driver https://kubernetes-sigs.github.io/aws-ebs-csi-driver
helm repo update
helm upgrade --install aws-ebs-csi-driver \
    --namespace kube-system \
    --set controller.replicaCount=1 \
    --set serviceAccount.create=false \
    aws-ebs-csi-driver/aws-ebs-csi-driver
```

### Install Karpenter on your cluster
- [Karpenter controller](https://karpenter.sh/v0.5.5/getting-started/)

Create the KarpenterNode IAM Role which has the necessary policies attached for Karpenter nodes. Additionally, we attach
the AmazonEKSClusterPolicy to the KarpenterNode IAM role so that KCM pods running on these nodes have the necessary permissions
to run the AWS Cloud Provider.
```bash
aws cloudformation deploy  \
  --stack-name Karpenter-${MANAGEMENT_CLUSTER_NAME} \
  --template-file docs/karpenter.cloudformation.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameter-overrides ClusterName=${MANAGEMENT_CLUSTER_NAME}
```

Allow instances using the KarpenterNode IAM role to authenticate to your cluster.

```bash
eksctl create iamidentitymapping \
  --username system:node:{{EC2PrivateDNSName}} \
  --cluster ${MANAGEMENT_CLUSTER_NAME} \
  --arn arn:aws:iam::${AWS_ACCOUNT_ID}:role/KarpenterNodeRole-${MANAGEMENT_CLUSTER_NAME} \
  --group system:bootstrappers \
  --group system:nodes
```

Create the KarpenterController IAM Role and associate it to the karpenter service account with IRSA.

```bash
eksctl create iamserviceaccount \
  --cluster ${MANAGEMENT_CLUSTER_NAME} --name karpenter --namespace karpenter \
  --role-name "${MANAGEMENT_CLUSTER_NAME}-karpenter" \
  --attach-policy-arn "arn:aws:iam::$AWS_ACCOUNT_ID:policy/KarpenterControllerPolicy-${MANAGEMENT_CLUSTER_NAME}" \
  --role-only --approve
```

Install the Karpenter Helm Chart.

```bash
helm repo add karpenter https://charts.karpenter.sh
helm repo update
helm upgrade --install --namespace karpenter --create-namespace \
  karpenter karpenter/karpenter \
  --version v0.7.3 \
  --set serviceAccount.annotations."eks\.amazonaws\.com/role-arn"="arn:aws:iam::${AWS_ACCOUNT_ID}:role/${MANAGEMENT_CLUSTER_NAME}-karpenter" \
  --set clusterName=${MANAGEMENT_CLUSTER_NAME} \
  --set clusterEndpoint=$(aws eks describe-cluster --name ${MANAGEMENT_CLUSTER_NAME} --query "cluster.endpoint" --output json)  \
  --wait # for the defaulting webhook to install before creating a Provisioner
```

#### Configure Karpenter provisioner to be able to provision the right kind of nodes

Create the following provisioners for Karpenter to be able to provisioner nodes for master and etcd

```bash
cat <<EOF | kubectl apply -f -
apiVersion: karpenter.sh/v1alpha5
kind: Provisioner
metadata:
  name: default
spec:
  requirements:
  - key: kit.k8s.sh/app
    operator: Exists
  - key: kit.k8s.sh/control-plane-name
    operator: Exists
  ttlSecondsAfterEmpty: 30
  provider:
    instanceProfile: KarpenterNodeInstanceProfile-${MANAGEMENT_CLUSTER_NAME}
    tags:
      kit.aws/substrate: ${MANAGEMENT_CLUSTER_NAME}
    subnetSelector:
      karpenter.sh/discovery: ${MANAGEMENT_CLUSTER_NAME}
    securityGroupSelector:
      kubernetes.io/cluster/${MANAGEMENT_CLUSTER_NAME}: owned
EOF
```

#### Install Prometheus monitoring stack to scrape guest cluster control plane 

Installs prometheus, grafana, prometheus-operator and node-exporter and we disable default monitoring configurations. Prometheus will be configured by the KIT operator to scrape the guest cluster components when a guest cluster control plane is provisioned.

```bash
helm install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring --create-namespace \
  --set coreDns.enabled=false \
  --set kubeProxy.enabled=false \
  --set kubeEtcd.enabled=false \
  --set alertmanager.enabled=false \
  --set kubeScheduler.enabled=false \
  --set kubeApiServer.enabled=false \
  --set kubeStateMetrics.enabled=false \
  --set kubeControllerManager.enabled=false \
  --set prometheus.serviceMonitor.selfMonitor=false \
  --set prometheusOperator.serviceMonitor.selfMonitor=false
```

### Deploy KIT operator

#### Create an IAM role and policy which will be assumed by KIT operator to be able to talk AWS APIs

```bash
aws cloudformation deploy  \
  --template-file docs/kit.cloudformation.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --stack-name kitControllerPolicy-${MANAGEMENT_CLUSTER_NAME} \
  --parameter-overrides ClusterName=${MANAGEMENT_CLUSTER_NAME}
```

#### Associate the policy we just created to the kit-controller service account 

```bash
eksctl create iamserviceaccount \
  --name kit-controller \
  --namespace kit \
  --cluster ${MANAGEMENT_CLUSTER_NAME} \
  --attach-policy-arn arn:aws:iam::${AWS_ACCOUNT_ID}:policy/KitControllerPolicy-${MANAGEMENT_CLUSTER_NAME} \
  --approve \
  --override-existing-serviceaccounts \
  --region=${AWS_REGION}
```

#### Install KIT operator to the cluster

```bash
helm repo add kit https://awslabs.github.io/kubernetes-iteration-toolkit/
helm upgrade --install kit-operator kit/kit-operator --namespace kit --create-namespace --set serviceAccount.create=false
```

Once KIT operator is deployed in a Kubernetes cluster. You can create a new Kubernetes control plane and worker nodes by following these steps in any namespace in the substrate cluster

1. Provision a control plane for the guest cluster

```bash
cat <<EOF | kubectl apply -f -
apiVersion: kit.k8s.sh/v1alpha1
kind: ControlPlane
metadata:
  name: ${GUEST_CLUSTER_NAME} # Desired Cluster name
spec: {}
EOF
```

2. Get the admin KUBECONFIG for the guest cluster from the substrate cluster

```bash
kubectl get secret ${GUEST_CLUSTER_NAME}-kube-admin-config -ojsonpath='{.data.config}' | base64 -d > /tmp/kubeconfig
```
> NOTE: It takes about 3-4 minutes for the cluster control plane to be available and healthy

3. Deploy CNI plugin to the guest cluster for the nodes to be ready. If you are deploying in `us-west-2` region run the following command to install AWS CNI plugin

```bash
kubectl --kubeconfig=/tmp/kubeconfig apply -f https://raw.githubusercontent.com/aws/amazon-vpc-cni-k8s/release-1.10/config/master/aws-k8s-cni.yaml
```
> For other regions, follow this guide to deploy the AWS CNI plugin- https://docs.aws.amazon.com/eks/latest/userguide/managing-vpc-cni.html

4. Provision worker nodes for the guest cluster

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
5. Optional: add a default EBS storage class to your KIT cluster.

```bash
cat <<EOF | kubectl --kubeconfig=/tmp/kubeconfig apply -f -
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: gp2
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
provisioner: kubernetes.io/aws-ebs
parameters:
  type: gp2
  fsType: ext4
volumeBindingMode: WaitForFirstConsumer
EOF
```
> TODO add instructions to be able to configure control plane parameters.

---

### Troubleshooting Notes:

> If you run into issues with AWS permissions in KIT-controller, delete the iamserviceaccount and recreate again with the steps mentioned above.
```bash
eksctl delete iamserviceaccount --name kit-controller \
  --namespace kit \
  --cluster ${MANAGEMENT_CLUSTER_NAME} \
  --region=$AWS_REGION
```

