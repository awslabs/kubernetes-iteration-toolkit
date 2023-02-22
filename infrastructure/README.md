# Kubernetes Iteration Toolkit Infrastructure

### Overview:

The Kubernetes Iteration Toolkit (KIT) infrastructure is a Cloud Development Kit (CDK) application that stands up the base infrastructure to test in
a Kubernetes-native manner. The K8s components installed directly within the CDK application are for ease of bootstrapping and applying IAM permissions.
All other components are able to be installed via Flux resources.

### Components:

KIT Infrastructure creates a base K8s cluster with below add-ons by default but also provides an ability to not install some of these addons optionally through CDK context. Add-ons include permissions scoped to the pod using IAM Roles for Service Accounts (IRSA).

- EKS Cluster (host cluster)
- EKS Managed Node Group (for critical add-ons mentioned below)
- EBS CSI Driver (optional)
- AWS Load Balancer Controller
- Karpenter (optional)
- Flux v2
- Kubernetes Iteration Toolkit (KIT) Operator (optional)

Flux is setup, by deafult, to monitor the KIT git repo path `./infrastructure/k8s-config/clusters/kit-infrastructure`, which includes other add-ons that do not require AWS credentials such as tekton, prometheus, grafana, the metrics-server and [perf dash].

#### Perf Dash

Perf dash is an optional addon and disabled by default.

To bring it up, there are a few places you need to ensure they are wired correctly.

- A k8s service account named `perfdash-log-fetcher` is expected and has the permissions to fetch the logs from the s3 bucket.
- A ConfigMap containing the target S3 bucket name that hosts the log and the jobs config for perf dash must be created so that it can be mounted as volume in the Deployment. An example configmap can be:
```
apiVersion: v1
kind: ConfigMap
metadata:
  name: perfdash-config
  namespace: perfdash
  labels:
    app.kubernetes.io/name: perfdash
    app.kubernetes.io/component: config
    app.kubernetes.io/part-of: perfdash
data:
  PERFDASH_LOG_BUCKET: my-bucket-name
  jobs.yaml: |
    periodics:
      - name: "kit-eks-1k"
        tags:
          - "perfDashPrefix: 1K test"
          - "perfDashJobType: performance"
      - name: "kit-eks-2k"
        tags:
        - "perfDashPrefix: 2k test"
        - "perfDashJobType: performance"
```
- The logs for different runs in the bucket should have the layout like the following:
```
my-s3-bucket
├── kit-eks-1k # note: this should match the name in perf dash config.
│   ├── 123
│   │   └── cl2-logs
│   └── 124
│       └── cl2-logs
└── kit-eks-2k # note: this should match the name in perf dash config.
    ├── 125
    │   └── cl2-logs
    └── 126
        └── cl2-logs
```

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

As an example, below are the parmeters used if you want to selectively enable some addons like perfdash and disable some addons like Karpenter, EBSCSIDriver, KIT.

```shell
cdk bootstrap
cdk deploy KITInfrastructure --no-rollback \
  -c TestNamespace="tekton-pipelines" \
  -c TestServiceAccount="tekton-pipelines-executor" \
  -c AWSEBSCSIDriverAddon=false \
  -c KarpenterAddon=false \
  -c KITAddon=false \
  -c FluxRepoAddonPaths="./infrastructure/k8s-config/clusters/addons/"
```

### Dependent IAM policies:

The application is caching IAM policies of two components as static files in `lib/addons/cached/`:
* aws-load-balancer-controller
* aws-ebs-csi-driver

These IAM policies are pinned to a version and downloaded form Github. When the version of these components change
in an upgrade, the cache needs to be updated by modifying the version values in `cache-iam-policies.sh` and executing file.

### [Optional] Creating Amazon Managed Prometheus Workspace:
```
AMP_WORKSPACE_NAME=<workspace-name>
AMP_WORKSPACE_ID=`aws amp create-workspace --alias $AMP_WORKSPACE_NAME --output text --query workspaceId`
echo $AMP_WORKSPACE_ID # this amp workspace can be used to remote write cluster metrics from test tasks. Ref: https://github.com/awslabs/kubernetes-iteration-toolkit/blob/main/tests/pipelines/eks/awscli-cl2-load-with-addons.yaml#L17
```

### [Optional] Creating Amazon Managed Grafana Dashboard. [Ref](https://docs.aws.amazon.com/grafana/latest/userguide/AMP-adding-AWS-config.html)
* Open the Amazon Managed Grafana console at https://console.aws.amazon.com/grafana/.
* In the upper left corner of the page, choose the menu icon and then choose All workspaces.
* Choose the name of the workspace. (use the workspace id and name you got from the previous command)
* If you didn't choose to use service-managed permissions for this workspace when you created it, then change from using customer-managed permissions to use service-managed permissions to ensure that the proper IAM roles and policies are enabled for using the AWS data source configuration option in the Grafana workspace console. To do so, choose the edit icon by IAM role and then choose Service managed, Save changes. For more information, see Amazon Managed Grafana permissions and policies for AWS data sources (https://docs.aws.amazon.com/grafana/latest/userguide/AMG-manage-permissions.html).
* Choose the Data sources tab. Then select the check box for Amazon Managed Service for Prometheus, and choose Actions, Enable service-managed policy.
* Choose the Data sources tab again, and then choose Configure in Grafana in the Amazon Managed Service for Prometheus row.
* Sign into the Grafana workspace console using IAM Identity Center if necessary.
* In the left navigation bar in the Grafana workspace console, choose the AWS icon and then choose AWS services, Prometheus.
* Select the Region that you want Amazon Managed Grafana to search to discover Amazon Managed Service for Prometheus workspaces, and then select the accounts and Amazon Managed Service for Prometheus workspaces that you want to add, and then choose Add data source.

### Installing tekton tasks and pipelines
Follow the steps from here: https://github.com/awslabs/kubernetes-iteration-toolkit/tree/main/tests

### Context Parameters:

| Context Param       | Description                                                                                | Default                                                 |   |   |
|-------------------- |--------------------------------------------------------------------------------------------|---------------------------------------------------------|---|---|
| FluxRepoURL         | Flux Source git repo URL to synchronize KIT infrastructure like Tekton                     | https://github.com/awslabs/kubernetes-iteration-toolkit |   |   |
| FluxRepoBranch      | Flux Source git repo branch to synchronize KIT infrastructure                              | main                                                    |   |   |
| FluxRepoPath        | Flux Source git repo path to Kubernetes resources                                          | ./infrastructure/k8s-config/clusters/kit-infrastructure |   |   |
| FluxRepoAddonPaths      | Flux Source git repo paths (separted by comma) to Kubernetes addons.                       |                                                         |   |   |
| TestFluxRepoName    | Flux Source git repo name to synchronize application tests like Tekton Tasks and Pipelines |                                                         |   |   |
| TestFluxRepoURL     | Flux Source git repo URL to synchronize application tests                                  |                                                         |   |   |
| TestFluxRepoBranch  | Flux Source git repo branch to synchronize application tests                               |                                                         |   |   |
| TestFluxRepoPath    | Flux Source git repo path to Kubernetes resources                                          |                                                         |   |   |
| TestNamespace       | Namespace for application tests to run in                                                  |                                                         |   |   |
| TestServiceAccount  | Service Account for application tests to run with                                          |                                                         |   |   |
| KITAddon            | KIT CRD addon that gets installed on KIT Infrastructure by default                         | true                                                    |   |   |                               
| KarpenterAddon      | Karpenter CRD addon that gets installed on KIT Infrastructure by default                   | true                                                    |   |   | 
| AWSEBSCSIDriverAddon| AWSEBSCSIDriver CRD addon that gets installed on KIT Infrastructure by default             | true                                                    |   |   |

[perf dash]: https://github.com/kubernetes/perf-tests/tree/master/perfdash
