# How to use KIT for Kubernetes testing

[Kubernetes-Iteration-Toolkit](https://github.com/awslabs/kubernetes-iteration-toolkit) is a toolkit that creates a testing environment and tools to manage the lifecycle of Kubernetes clusters (EKS or vanilla Kubernetes), run tests against new or existing clusters and collect metrics & logs from the control plane for analysis.

Toolkit consists of the following components -  kubernetes cluster (management cluster), operators and tools like Tekton, prometheus, Karpenter, ELB/EBS controller all pre-installed in a KIT environment. Toolkit comes prebaked with some set of actions a user can easily run like-

* `Create a KIT guest/EKS cluster`
* `Create n number of nodes` or
* `Create x number of Kubernetes objects.`

A user can combine these various tasks to run these tests in a pipeline and collect results. 

If you are starting with KIT refer [use-cases covered by KIT](#use-cases-supported-with-kit) to make sure your use-case is supported, if its not listed or its not clear feel free to file an issue in the repo.

## Getting started

* `kitctl` binary helps with creating the KIT environment in an AWS account. Install kitctl locally on your mac-

```bash
brew tap awslabs/kit https://github.com/awslabs/kubernetes-iteration-toolkit.git
brew install kitctl 
```

* Bootstrap a KIT environment using the kitctl

```bash
export AWS_REGION=us-west-2
kitctl bootstrap kitctl-$(whoami) # Optional environment name
```

Note: Make sure you are logged into the AWS account and have admin privilege, and the environment name is unique to you as KIT uses it create an S3 bucket in your AWS account.

* Bootstrap takes about 5 minutes,  as part of this bootstrap it configures the AWS infra such as VPC, security groups, subnets, route tables etc. and a management Kubernetes cluster. At the end of the bootstrap command it prints an `export KUBECONFIG` command, run this command to access the management cluster using kubectl.

Note: This environment we just created comes pre-installed with Tekton for running the tests and a Prometheus stack for monitoring.

* To access the Tekton dashboard `kubectl port-forward svc/tekton-dashboard -n tekton-pipelines 9097:9097` and you will see the templates for tasks and pipelines in Tekton.
* To access the Grafana dashboard `kubectl port-forward svc/kube-prometheus-stack-grafana -n monitoring 8080:80`
## Running tests

At this point a user can run tests against either an EKS cluster or vanilla Kubernetes cluster provisioned by KIT operator called [guest clusters](https://quip-amazon.com/xCtbA6C6X7dy/How-to-use-KIT-for-Kubernetes-testing#temp:C:VfOa1f3b6a5d7de51afb05d574ef) running in the environment. 
Difference between testing against the two cluster types-

* EKS cluster doesn’t allow changing any flags or control plane images, however, if you want to make any flags changes or want to run custom docker images for API server or any other Kubernetes control plane component use guest cluster for testing
* Metrics that we collect from EKS clusters are limited to API server metrics and CW metrics (available internally to EKS teams), we don’t get the node level metrics from EKS control plane nodes. In guest clusters, we are able to collect API server, etcd, scheduler, KCM and node level metrics.
* Guest clusters do not support testing Managed node groups or Fargate or IRSA at this point.


### How to run tests

Add tasks and pipelines from the KIT repo, (we have a [task](https://github.com/awslabs/kubernetes-iteration-toolkit/issues/207) pending to automate this step)

```
# cd github.com/awslabs/kubernetes-iteration-toolkit/tests
# kubectl apply -n tekton-pipelines -f pipelines -R
# kubectl apply -n tekton-pipelines -f tasks -R
```

* Access the Tekton dashboard on [http://localhost:9097](http://localhost:9097/), make sure you have run port-forward command mentioned above.
* Under the tasks tab look for the pre-loaded tasks that can be run.
* Under pipelines tab are some sample pipelines flow which help create logic regarding run for running multiple tasks for an end-to-end test flow. Example pipeline combines the following tasks-
    * Create an EKS cluster + create a managed node group with `n` nodes + deploy `n` pods in the cluster + measure  pod startup latency as part of the test
* These pipelines can be triggered from dashboard using pipeline runs or by creating a CRD for pipeline run.
* Based on these samples, a users can create their own pipelines for specific tasks based on their test use case and run these pipelines using pipeline runs.


How to collect metrics

* Access the Grafana dashboard on [http://localhost:8080](http://localhost:8080/), make sure you have run port-forward command mentioned above.
* All the core Kubernetes components like API Server, KCM, scheduler, etcd, authenticator are being scraped and their metrics are being plotted in their own Grafana dashboards
* For node level metrics in Grafana refer to NodeExporter/Nodes dashboard

Adding additional dashboards from Github repo-

```bash
# wget grafana dashboard from the repo
wget https://raw.githubusercontent.com/awslabs/kubernetes-iteration-toolkit/main/substrate/monitoring/master-dashboard.json

# Create configmap in management cluster to load this dashboard
kubectl create configmap master-dashboard -n monitoring --from-file=master-dashboard.json
kubectl label configmap master-dashboard -n monitoring  grafana_dashboard=1
```
>Note: we are [tracking the task](https://github.com/awslabs/kubernetes-iteration-toolkit/issues/207) on automating this step as well where the Grafana dashboards are pre-loaded in Grafana

Accessing logs for kube components-
[WIP](https://github.com/awslabs/kubernetes-iteration-toolkit/issues/106)

### Use-cases supported with KIT

* As a Kubernetes developer/user, I want to run some tests and as part of these tests I want to-
    - create x number of objects(pods, secrets, config maps) in a cluster
    - tweak a flag in Kubernetes control plane and deploy some workload
    - run an e2e scalability test in an EKS cluster to create x number of different objects and y number of nodes
    - test a custom API server/Scheduler image with some changes in a Kubernetes cluster, by creating x number of objects
    - run etcd or master components on specific EC2 instance types and run scale tests
    - in an existing EKS cluster can I run a load test, get all the metrics, resource usage and validation tests

As part of these tests, I want to capture-
    - resource utilization (cpu and memory) for master and etcd instances
    - latency for the API server calls is not impacted and SLO’s are not breached.
    - metrics for core Kubernetes components like scheduler, KCM, etcd etc.

## Key Terms

**KIT/clusters or guest clusters -** These are rapid prototyping vanilla Kubernetes clusters provisioned using using EKS-distro images. They take about 3-4 minutes to provision on ec2 nodes and less than 30 seconds to update their configurations. These clusters are provisioned by KIT-operator running in the KIT environment
