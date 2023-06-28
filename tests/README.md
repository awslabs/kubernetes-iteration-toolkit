# Kubernetes Iteration Toolkit Tests with Tekton

### Overview:
Tekton is a powerful and flexible open-source framework for creating CI/CD systems, allowing developers to build, test, and deploy across cloud providers and on-premise systems. This documents steps for setting up kit load test tasks and pipelines in tekton infrastructure.

### Prerequisites
* A kubernetes cluster with tekton infrastructure configured. [Ref](https://github.com/awslabs/kubernetes-iteration-toolkit/tree/main/infrastructure) for setting up KITInfra.
* [Kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl-macos/)


### Installing tekton pipelines
```
cd tests
kubectl apply -n scalability -f tasks -R
kubectl apply -n scalability -f pipelines -R
```

### Running a single pipelinerun
```
cat <<EOF | kubectl apply -f -
apiVersion: tekton.dev/v1beta1
kind: PipelineRun
metadata:
generateName: awscli-eks-load-sample
spec:
pipelineRef:
    name: awscli-eks-cl2loadtest
timeout: 9h0m0s
workspaces:
    - name: source
    emptyDir: {}
    - name: config
    volumeClaimTemplate:
        spec:
        accessModes:
            - ReadWriteOnce
        storageClassName: gp2
        resources:
            requests:
            storage: 1Gi
    - name: results
    emptyDir: {}
params:
    - name: cluster-name
    value: "awscli-eks-load-100"
    - name: desired-nodes
    value: "100"
    - name: pods-per-node
    value: "10"
    - name: nodes-per-namespace
    value: "100"
    - name: cl2-load-test-throughput
    value: "20"
    - name: results-bucket
    value: kit-eks-scalability/kit-eks-5k/$(date +%s)
    - name: slack-message
    value: "You can monitor here - https://tekton.scalability.eks.aws.dev/#/namespaces/tekton-pipelines/pipelineruns ;5k node "
    - name: slack-hook
    value: <slack hook url. Pass empty if you don't have one>
    - name: vpc-cfn-url
    value: "https://raw.githubusercontent.com/awslabs/kubernetes-iteration-toolkit/main/tests/assets/amazon-eks-vpc.json"
podTemplate:
    nodeSelector:
    kubernetes.io/arch: amd64
serviceAccountName: tekton-pipelines-executor
EOF
```

### Scheduling recurrent test runs.
To schedule recurrent test runs, you can leverage cron-job to trigger a pipeline run using trigger-template. Please refer to below example for sample cron job and pipelinerun template.

```
cat <<EOF | kubectl apply -f -
---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: awscli-eks-load-5k-example
  namespace: scalability
spec:
  schedule: "13 15 * * *"
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: curl
              image: curlimages/curl
              args: ["curl", "-X", "POST", "--data", "{}", "el-awscli-eks-load-5k.scalability.svc.cluster.local:8080"]
          restartPolicy: Never
---
apiVersion: triggers.tekton.dev/v1alpha1
kind: TriggerBinding
metadata:
  name: awscli-eks-load-5k
  namespace: scalability
spec:
  params:
    - name: servicerole
      value: <service role ARN. Example of Service Role- https://github.com/awslabs/kubernetes-iteration-toolkit/blob/main/tests/assets/eks_service_role.json>
---
apiVersion: triggers.tekton.dev/v1alpha1
kind: EventListener
metadata:
  name: awscli-eks-load-5k
  namespace: scalability
spec:
  serviceAccountName: tekton-triggers
  triggers:
    - name: cron
      bindings:
        - ref: awscli-eks-load-5k
      template:
        ref: awscli-eks-load-5k
---
apiVersion: triggers.tekton.dev/v1alpha1
kind: TriggerTemplate
metadata:
  name: awscli-eks-load-5k
  namespace: scalability
spec:
  resourcetemplates:
    - apiVersion: tekton.dev/v1beta1
      kind: PipelineRun
      metadata:
        generateName: awscli-eks-load-5k-
      spec:
        pipelineRef:
          name: awscli-eks-cl2loadtest-with-addons
        timeout: 9h0m0s
        workspaces:
          - name: source
            emptyDir: {}
          - name: config
            volumeClaimTemplate:
              spec:
                accessModes:
                  - ReadWriteOnce
                storageClassName: gp2
                resources:
                  requests:
                    storage: 1Gi
          - name: results
            emptyDir: {}
        params:
          - name: cluster-name
            value: "awscli-eks-load-100"
          - name: desired-nodes
            value: "100"
          - name: pods-per-node
            value: "10"
          - name: nodes-per-namespace
            value: "100"
          - name: cl2-load-test-throughput
            value: "20"
          - name: results-bucket
            value: <replace it a s3 bucket to store test results>
          - name: slack-message
            value: "You can monitor here - https://tekton.scalability.eks.aws.dev/#/namespaces/tekton-pipelines/pipelineruns ;5k node "
          - name: slack-hook
            value: <slack hook url>
          - name: vpc-cfn-url
            value: "https://raw.githubusercontent.com/awslabs/kubernetes-iteration-toolkit/main/tests/assets/amazon-eks-vpc.json"
        podTemplate:
          nodeSelector:
            kubernetes.io/arch: amd64
        serviceAccountName: tekton-pipelines-executor
EOF
```

### Test Images

Tekton tasks can leverage image like clusterloader2 to perform various kinds of tests like load, pod-density on K8s cluster on KIT infra. 
To build the docker image for clusterloader2 use the below command which takes `branch` as a build arg which lets us build clusterloader2 for a given branch on this repo[here](https://github.com/kubernetes/perf-tests/tree/master/clusterloader2)

- docker build --build-arg branch=release-1.23 ./images/clusterloader2/