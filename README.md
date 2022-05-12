# What is the Kubernetes Iteration Toolkit?

## What is KIT?

[KIT](https://github.com/awslabs/kubernetes-iteration-toolkit) is a set of decoupled tools designed to accelerate the development of Kubernetes through testing. It combines a variety of open source projects to define an opinionated way to rapidly configure and test Kubernetes components on AWS.

## Why did we build KIT?

The EKS Scalability team is responsible for improving performance across the Kubernetes stack. We started our journey by manually running tests against modified EKS dev clusters. This helped us to identify some bottlenecks, but results were difficult to demonstrate and reproduce. We wanted to increase the velocity of our discoveries, as well as our confidence in our results. We set out to build automation to help us configure cluster components, execute well known test workloads, and analyze the results. This evolved into KIT, and we’re ready to share it to help accelerate testing in other teams.

## What can I do with KIT?

KIT can help you run scale tests against a KIT cluster or an EKS cluster, collect logs and metrics from the cluster control plane and nodes to help analyze the performance for Kubernetes cluster. KIT comes with a set of tools like Karpenter, ELB controller, Prometheus, Grafana and Tekton etc. installed and configured to manage cluster lifecycle, run tests and collect results.

## What are KIT Environments?

KIT Environments provide an opinionated testing environment with support for test workflow execution, analysis, and observability. Developers can use `kitctl` cli to create a personal or shared testing environment for oneshot or periodic tests. KIT Environments are EKS Clusters that come preinstalled with a suite of Kubernetes operators that enable the execution, analysis, and persistence.

Additionally, KIT Environments provide a library of predefined [Tasks](https://github.com/awslabs/kubernetes-iteration-toolkit/tree/c6925e3db92ae909cafb2751b153dd8221d6fd55/tests/tasks) to configure clusters, generate load, and analyze results. For example, you can combine the “MegaXL KIT Cluster” task and “upstream pod density load generator” task to reproduce the scalability team’s MegaXL test results. You can then swap in the “EKS Cluster” task and verify the results as improvements are merged into EKS. You can also parameterize existing tasks or define your own to meet your use cases.

## What are KIT Clusters?

KIT Clusters enables developers to declaratively configure eks-like clusters with arbitrary modifications. Using a Kubernetes CRD, you can modify the EC2 instance types, container image, environment variables, or command line arguments of any cluster component. These configurations can be [checked into git](https://github.com/awslabs/kubernetes-iteration-toolkit/blob/main/operator/docs/examples/cluster-1.21.yaml) and reproduced for periodic regression testing or against new test scenarios.

KIT Clusters are implemented using Kubernetes primitives like deployments, statefulsets, and services. More advanced use cases can be achieved by implementing a new feature in the [KIT Cluster Operator](https://github.com/awslabs/kubernetes-iteration-toolkit/tree/main/operator) and exposing it as a new parameter in the CRD. You can install the KIT Cluster Operator on any Kubernetes cluster or with `kitctl bootstrap`.

## How do I get started with KIT?

KIT-v0.1 (alpha) is available now. You can get started with kitctl by installing `kitctl` [here](https://github.com/awslabs/kubernetes-iteration-toolkit/tree/main/substrate#installing-kitctl) or by reaching out in our [Slack channel](https://amzn-aws.slack.com/archives/C02HZDEQ678). We’re focused on delivering features that unblock scalability testing, contributions are welcome.