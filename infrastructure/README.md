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

Flux is setup, by deafult, to monitor the KIT git repo path `./infrastructure/k8s-config/clusters/kit-infrastructure`, which includes other add-ons that do not require AWS credentials such as tekton, prometheus, grafana, and the metrics-server. 

### Getting Started:

To launch the KIT infrastructure, ensure you have the following installed:
 - [aws-cli](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html) and configured
 - [CDK v2 installed](https://docs.aws.amazon.com/cdk/v2/guide/cli.html)
 - [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl-macos/)

By default, the CDK application will wire up a Flux config that will monitor for k8s manifests within the KIT repo at `infrastructure/k8s-config/clusters/kit-infrastructure`.
The parameters supplied to the CDK app will allow you to wire up your own application's repo so that you can place tekton test files and other cluster components there.

 As an example, below are the parameters used for the [Karpenter](https://github.com/aws/karpenter) project.

 ```shell
cdk bootstrap
cdk deploy KITInfrastructure --no-rollback \
  -c TestFluxRepoName="karpenter" \
  -c TestFluxRepoURL="https://github.com/aws/karpenter" \
  -c TestFluxRepoBranch="main" \
  -c TestFluxRepoPath="./test/infrastructure/clusters/test-infra" \
  -c TestNamespace="karpenter-tests" \
  -c TestServiceAccount="karpenter-tests"
 ```

### Context Parameters:

| Context Param      | Description                                                                                | Default                                                 |   |   |
|--------------------|--------------------------------------------------------------------------------------------|---------------------------------------------------------|---|---|
| FluxRepoURL        | Flux Source git repo URL to synchronize KIT infrastructure like Tekton                     | https://github.com/awslabs/kubernetes-iteration-toolkit |   |   |
| FluxRepoBranch     | Flux Source git repo branch to synchronize KIT infrastructure                              | main                                                    |   |   |
| FluxRepoPath       | Flux Source git repo path to Kubernetes resources                                          | ./infrastructure/k8s-config/clusters/kit-infrastructure |   |   |
| TestFluxRepoName   | Flux Source git repo name to synchronize application tests like Tekton Tasks and Pipelines |                                                         |   |   |
| TestFluxRepoURL    | Flux Source git repo URL to synchronize application tests                                  |                                                         |   |   |
| TestFluxRepoBranch | Flux Source git repo branch to synchronize application tests                               |                                                         |   |   |
| TestFluxRepoPath   | Flux Source git repo path to Kubernetes resources                                          |                                                         |   |   |
| TestNamespace      | Namespace for application tests to run in                                                  |                                                         |   |   |
| TestServiceAccount | Service Account for application tests to run with                                          |                                                         |   |   |
