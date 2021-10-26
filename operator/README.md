# KIT (Kubernetes Iteration Toolkit) Operator

KIT-operator is an experimental project that makes it easy to provision a Kubernetes cluster in AWS. It runs a controller in an existing Kubernetes cluster, users can provision a new cluster by following the steps listed below.
By default, operator deploys Kubernetes images from the [Amazon EKS Distro](https://distro.eks.amazonaws.com/).
Currently, the operator can only be deployed on an existing (substrate) Kubernetes cluster and depends on other controllers (listed below) to be able to provison a guest cluster in AWS.

KIT uses the [operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) in Kubernetes to regularly reconcile the cluster state in AWS to the desired state defined in the spec. KIT follows the battery included model where a cluster can be created with passing just the cluster name.

## Getting started

### Prerequisites

- Kubernetes (substrate) Cluster
- Following controller need to be installed in this cluster-
  - [Karpenter controller](https://karpenter.sh/docs/getting-started/)
  - [AWS Load balancer controller](https://docs.aws.amazon.com/eks/latest/userguide/aws-load-balancer-controller.html)

### Deploy KIT operator

```bash
  SUBSTRATE_CLUSTER_NAME=kit-management-cluster
  GUEST_CLUSTER_NAME=example
  AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
  AWS_REGION=us-west-2
```

#### Create an IAM role and policy which will be assumed by KIT operator to be able to talk AWS APIs

```bash
  aws cloudformation deploy  \
  --template-file docs/kit.cloudformation.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --stack-name kitControllerPolicy
```

#### Associate the policy we just created to the kit-controller service account 

```bash
  eksctl create iamserviceaccount \
    --name kit-controller \
    --namespace kit \
    --cluster ${SUBSTRATE_CLUSTER_NAME} \
    --attach-policy-arn arn:aws:iam::${AWS_ACCOUNT_ID}:policy/KitControllerPolicy \
    --approve \
    --override-existing-serviceaccounts \
    --region=${AWS_REGION}
```

#### Install KIT operator to the cluster

```bash
   helm repo add kit https://awslabs.github.io/kubernetes-iteration-toolkit/
   helm upgrade --install kit-operator kit/kit-operator --namespace kit --create-namespace --version 0.0.1
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
  kubectl get secret example-kube-admin-config -ojsonpath='{.data.config}' | base64 -d > /tmp/kubeconfig
```
> NOTE: It takes about 3-4 minutes for the cluster control plane to be available and healthy

3. Deploy CNI plugin to the guest cluster for the nodes to be ready. If you are deploying in `us-west-2` region run the following command to install AWS CNI plugin

```bash
  kubectl --kubeconfig=/tmp/kubeconfig apply -f https://raw.githubusercontent.com/aws/amazon-vpc-cni-k8s/master/config/v1.9/aws-k8s-cni.yaml
```
> For other regions, follow this guide to deploy the AWS CNI plugin- https://docs.aws.amazon.com/eks/latest/userguide/managing-vpc-cni.html

4. Provision worker nodes for the guest cluster

```
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