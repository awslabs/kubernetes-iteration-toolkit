# KIT Development guide

If you are trying out KIT or developing KIT follow these instructions to get started-

## Prerequisites
 - Go version (1.16 or higher)
 - [Ko version](https://github.com/google/ko#install) (v0.8.2 or higher)
 - [ECR repository](https://docs.aws.amazon.com/AmazonECR/latest/userguide/repository-create.html)

## Build and Deploy

### Create an ECR image repo

```bash
AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
AWS_REGION=us-west-2
CONTAINER_IMAGE_REGISTRY=${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com

aws ecr create-repository --repository-name kit --region ${AWS_REGION}
aws ecr get-login-password --region ${AWS_REGION} | docker login --username AWS --password-stdin $CONTAINER_IMAGE_REGISTRY
```

## Deploy

Makefile call `Ko` and `Ko` will build the Docker image and push the image to the container repo created in ECR in the last step. Once the image is published, Ko will `kubectl apply` KIT YAML(s) in `config` directory. This will install KIT operator and the required configs to the cluster listed in `kubectl config current-context`

```bash
make deploy CONTAINER_IMAGE_REGISTRY=$CONTAINER_IMAGE_REGISTRY
```

## Delete KIT
To delete KIT from Kubernetes cluster

```bash
make delete
```