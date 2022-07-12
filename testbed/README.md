# Kubernetes Iteration Toolkit Infrastructure

### Overview:

The Kubernetes Iteration Toolkit (KIT) infrastructure is a Cloud Development Kit (CDK) application that stands up the base infrastructure to test in 
a Kubernetes-native manner. The K8s components installed directly within the CDK application are for ease of bootstrapping and applying IAM permissions. 
All other components are able to be installed via Flux resources.  

### Components:

KIT Infrastructure creates a base K8s cluster with a few add-ons. Add-ons include permissions scoped to the pod using IAM Roles for Service Accounts (IRSA).

- EKS Cluster (host cluster)
- EKS Managed Node Group (for critical add-ons mentioned below)
- EBS CSI Driver
- AWS Load Balancer Controller
- Karpenter 
- Flux v2
- Kubernetes Iteration Toolkit (KIT) Operator


### Getting Started:

To launch the KIT infrastructure, ensure you have the following installed:
 - [aws-cli](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html) and configured
 - [CDK v2 installed](https://docs.aws.amazon.com/cdk/v2/guide/cli.html)
 - [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl-macos/)

By default, the CDK application will wire up a Flux config that will monitor for k8s manifests within the KIT repo at `k8s-config/clusters/test-infra`. 
The parameters supplied to the CDK app will allow you to wire up your own application's repo so that you can place tekton test files and other cluster components there.
 
 Below are the parameters used for the [Karpenter](https://github.com/aws/karpenter) project.
 
 ```shell
cdk deploy InfraStack --no-rollback \
  -c FluxRepoURL="https://github.com/awslabs/kubernetes-iteration-toolkit" \
  -c FluxRepoBranch="infrastructure" \
  -c FluxRepoPath="./testbed/k8s-config/clusters/test-infra" \
  -c TestFluxRepoName="karpenter" \
  -c TestFluxRepoURL="https://github.com/aws/karpenter" \
  -c TestFluxRepoBranch="main" \
  -c TestFluxRepoPath="./test/infrastructure/k8s-config/clusters/test-infra" \
  -c TestNamespace="karpenter-tests" \
  -c TestServiceAccount="karpenter-tests"
 ```