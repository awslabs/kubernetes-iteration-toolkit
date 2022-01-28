# KIT Development guide

If you are developing KIT operator, finish the installation steps listed in the [README](../README.md). Once KIT is installed in the substrate cluster, follow these steps to make changes and test the operator.

## Build and Deploy

### Prerequisites

 - Go version (1.16 or higher)
 - [Ko version](https://github.com/google/ko#install) (v0.8.2 or higher)

### Create a [Private ECR repository](https://docs.aws.amazon.com/AmazonECR/latest/userguide/repository-create.html) to push controller and webhook image for kit-operator

```bash
export AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
export AWS_REGION=us-west-2
export CONTAINER_IMAGE_REGISTRY=${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com

aws ecr create-repository --repository-name kit --region ${AWS_REGION}
aws ecr get-login-password --region ${AWS_REGION} | docker login --username AWS --password-stdin $CONTAINER_IMAGE_REGISTRY
```

## Deploy

Makefile calls `Ko` and `Ko` will build the Docker image and push the image to the container repo created in ECR in the last step. Once the image is published, Ko will `kubectl apply` KIT YAML(s) in `config` directory. This will install KIT operator and the required configs to the cluster listed in `kubectl config current-context`

```bash
kubectl create namespace kit
make apply
```

## Delete KIT
To delete KIT from Kubernetes cluster

```bash
make delete
kubectl delete namespace kit
```