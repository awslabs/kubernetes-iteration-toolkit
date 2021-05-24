# KIT - Kubernetes Iteration Tool

KIT is an experimental project that makes it easy to deploy a Kubernetes cluster in AWS.
KIT helps provision a Kubernetes cluster in AWS from scratch starting from VPC, subnets, master and etcd instances. By default, KIT deploys Kubernetes images from the [Amazon EKS Distro](https://distro.eks.amazonaws.com/).

KIT uses the [operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) in Kubernetes to regularly reconcile the cluster state in AWS to the desired state defined in the spec. KIT follows the battery included model where a cluster can be created with passing just the cluster name. It allows user to configure infrastructure parameters like subnet CIDRs, master/etcd instance count, AMI IDs and Kubernetes master/etcd component flags.

## Getting started

KIT is currently in alpha, there are no public images available to deploy, at the moment users will have to build the Docker images themselves to play with KIT. Follow the [Developer guide](docs/DEVELOPER_GUIDE.md) for instructions to deploy KIT in a Kubernetes cluster
> TODO Add instructions to try KIT on KIND

Once KIT operator is deployed in a Kubernetes cluster. You can create a new Kubernetes control plane by setting the desired cluster name and creating the following CRD object.

```bash
cat <<EOF | kubectl apply -f -
apiVersion: kit.k8s.amazonaws.com/v1alpha1
kind: ControlPlane
metadata:
  name: foo # Desired Cluster name
spec: {}
EOF
```

> This will create a Kubernetes cluster named `foo`

KIT operator will use the defaults and provision a kubernetes control plane in a new VPC, all the AWS resources created by KIT are tagged in AWS with `kit.k8s.amazonaws.com/cluster-name=foo`

> TODO add instructions to be able to configure control plane parameters.


kubeadm init --control-plane-endpoint "foo-lb-7174398f05be7a01.elb.us-east-2.amazonaws.com:6443" --upload-certs --image-repository public.ecr.aws/eks-distro/kubernetes --kubernetes-version v1.19.8-eks-1-19-4