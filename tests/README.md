# Kubernetes Iteration Toolkit Tests with Tekton

### Overview:
Tekton is a powerful and flexible open-source framework for creating CI/CD systems, allowing developers to build, test, and deploy across cloud providers and on-premise systems.



### Test Images

Tekton tasks can leverage image like clusterloader2 to perform various kinds of tests like load, pod-density on K8s cluster on KIT infra. 
To build the docker image for clusterloader2 use the below command which takes `branch` as a build arg which lets us build clusterloader2 for a given branch on this repo[here](https://github.com/kubernetes/perf-tests/tree/master/clusterloader2)

- docker build --build-arg branch=release-1.23 ./images/clusterloader2/