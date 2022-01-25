# KIT (Kubernetes Iteration Toolkit) Operator

KIT-operator is an experimental project that makes it easy to provision a Kubernetes cluster in AWS. It runs a controller in an existing Kubernetes cluster, users can provision a new cluster by following the steps listed below.
By default, operator deploys Kubernetes images from the [Amazon EKS Distro](https://distro.eks.amazonaws.com/).
Currently, the operator can only be deployed on an existing (substrate) Kubernetes cluster and depends on other controllers (listed below) to be able to provison a guest cluster in AWS.

KIT uses the [operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) in Kubernetes to regularly reconcile the cluster state in AWS to the desired state defined in the spec. KIT follows the battery included model where a cluster can be created with passing just the cluster name.

## Getting started

### Overview

- Create a Kubernetes (substrate) Cluster
- Install the AWS Load Balancer Controller
- Install Karpenter
- Install the KIT operator


### Create a substrate cluster with eksctl
```bash
SUBSTRATE_CLUSTER_NAME=kit-management-cluster
GUEST_CLUSTER_NAME=example
AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
AWS_REGION=us-west-2
```

This will create a substrate cluster with the necessary tags for Karpenter:
```bash
cat <<EOF > cluster.yaml
---
apiVersion: eksctl.io/v1alpha5
kind: ClusterConfig
metadata:
  name: ${SUBSTRATE_CLUSTER_NAME}
  region: ${AWS_REGION}
  version: "1.21"
  tags:
    karpenter.sh/discovery: ${SUBSTRATE_CLUSTER_NAME}
managedNodeGroups:
  - instanceType: m5.large
    amiFamily: AmazonLinux2
    name: ${SUBSTRATE_CLUSTER_NAME}-ng
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
  --cluster=${SUBSTRATE_CLUSTER_NAME} \
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
  --set clusterName=${SUBSTRATE_CLUSTER_NAME} \
  --set serviceAccount.create=false \
  --set serviceAccount.name=aws-load-balancer-controller 
```

### Install Karpenter on your cluster
- [Karpenter controller](https://karpenter.sh/v0.5.5/getting-started/)

Create the KarpenterNode IAM Role which has the necessary policies attached for Karpenter nodes. Additionally, we attach
the AmazonEKSClusterPolicy to the KarpenterNode IAM role so that KCM pods running on these nodes have the necessary permissions
to run the AWS Cloud Provider.
```bash
aws cloudformation deploy  \
  --stack-name Karpenter-${SUBSTRATE_CLUSTER_NAME} \
  --template-file docs/karpenter.cloudformation.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameter-overrides ClusterName=${SUBSTRATE_CLUSTER_NAME}
```
Allow instances using the KarpenterNode IAM role to authenticate to your cluster.
```bash
eksctl create iamidentitymapping \
  --username system:node:{{EC2PrivateDNSName}} \
  --cluster ${SUBSTRATE_CLUSTER_NAME} \
  --arn arn:aws:iam::${AWS_ACCOUNT_ID}:role/KarpenterNodeRole-${SUBSTRATE_CLUSTER_NAME} \
  --group system:bootstrappers \
  --group system:nodes
```
Create the KarpenterController IAM Role and associate it to the karpenter service account with IRSA.
```bash
eksctl create iamserviceaccount \
  --cluster $SUBSTRATE_CLUSTER_NAME --name karpenter --namespace karpenter \
  --attach-policy-arn arn:aws:iam::$AWS_ACCOUNT_ID:policy/KarpenterControllerPolicy-$SUBSTRATE_CLUSTER_NAME \
  --approve
```
Install the Karpenter Helm Chart.
```bash
helm repo add karpenter https://charts.karpenter.sh
helm repo update
helm upgrade --install karpenter karpenter/karpenter --namespace karpenter \
  --create-namespace --set serviceAccount.create=false --version v0.5.5 \
  --set controller.clusterName=${SUBSTRATE_CLUSTER_NAME} \
  --set controller.clusterEndpoint=$(aws eks describe-cluster --name ${SUBSTRATE_CLUSTER_NAME} --query "cluster.endpoint" --output json) \
  --wait # for the defaulting webhook to install before creating a Provisioner
```
#### Configure Karpenter provisioners to be able to provision the right kind of nodes
Create the two following provisioners for Karpenter to be able to provisioner nodes for master and etcd
```bash
cat <<EOF | kubectl apply -f -
apiVersion: karpenter.sh/v1alpha5
kind: Provisioner
metadata:
  name: master-nodes
spec:
  labels:
    kit.k8s.sh/app: ${GUEST_CLUSTER_NAME}-apiserver
    kit.k8s.sh/control-plane-name: ${GUEST_CLUSTER_NAME}
  provider:
    instanceProfile: KarpenterNodeInstanceProfile-${SUBSTRATE_CLUSTER_NAME}
    subnetSelector:
      karpenter.sh/discovery: ${SUBSTRATE_CLUSTER_NAME}
    securityGroupSelector:
      kubernetes.io/cluster/${SUBSTRATE_CLUSTER_NAME}: owned
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
  labels:
    kit.k8s.sh/app: ${GUEST_CLUSTER_NAME}-etcd
    kit.k8s.sh/control-plane-name: ${GUEST_CLUSTER_NAME}
  provider:
    instanceProfile: KarpenterNodeInstanceProfile-${SUBSTRATE_CLUSTER_NAME}
    subnetSelector:
      karpenter.sh/discovery: ${SUBSTRATE_CLUSTER_NAME}
    securityGroupSelector:
      kubernetes.io/cluster/${SUBSTRATE_CLUSTER_NAME}: owned
  ttlSecondsAfterEmpty: 30
EOF
```
### Deploy KIT operator

#### Create an IAM role and policy which will be assumed by KIT operator to be able to talk AWS APIs
```bash
aws cloudformation deploy  \
  --template-file docs/kit.cloudformation.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --stack-name kitControllerPolicy \
  --parameter-overrides ClusterName=${SUBSTRATE_CLUSTER_NAME}
```

#### Associate the policy we just created to the kit-controller service account 

```bash
eksctl create iamserviceaccount \
  --name kit-controller \
  --namespace kit \
  --cluster ${SUBSTRATE_CLUSTER_NAME} \
  --attach-policy-arn arn:aws:iam::${AWS_ACCOUNT_ID}:policy/KitControllerPolicy-${SUBSTRATE_CLUSTER_NAME} \
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
  --cluster ${SUBSTRATE_CLUSTER_NAME} \
  --region=$AWS_REGION
```

