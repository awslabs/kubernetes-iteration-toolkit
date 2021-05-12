# KIT Development guide

If you are trying out KIT or developing KIT follow these instructions to get started-

## Prerequisites
 - Go version (1.16 or higher)
 - [Ko version](https://github.com/google/ko#install) (v0.8.2 or higher)
 - [ECR repository](https://docs.aws.amazon.com/AmazonECR/latest/userguide/repository-create.html)

## Build

### Create an ECR image repo

```bash
AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
AWS_REGION=us-west-2
CONTAINER_IMAGE_REGISTRY=${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com

aws ecr create-repository --repository-name kit --region ${AWS_REGION}
aws ecr get-login-password --region ${AWS_REGION} | docker login --username AWS --password-stdin $CONTAINER_IMAGE_REGISTRY
```

## Deploy

Ko will build the Docker image and push the image to the container repo created in ECR in the last step. Once the image is published, Ko will `kubectl apply` KIT YAML in `config` directory. This will install KIT to the cluster listed in `kubectl config current-context`

```bash
KO_DOCKER_REPO=$CONTAINER_IMAGE_REGISTRY/kit ko apply --bare -f config
```

## IAM permissions

When running KIT in an EKS cluster, KIT controller pod needs IAM permissions to be able to manage AWS resources, follow these steps to be able to create a role with minimum set of permissions required. This will create a role `KITControllerRole` in AWS IAM using CloudFormation and setup IRSA for Kit controller.

### Create IAM role and policy
```bash
CLUSTER_NAME=<EKS_CLUSTER_NAME>
AWS_REGION=<AWS_REGION>
AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)

aws cloudformation deploy \
  --stack-name kit-${CLUSTER_NAME} \
  --template-file ./docs/kit.cloudformation.yaml \
  --capabilities CAPABILITY_NAMED_IAM \
  --parameter-overrides ClusterName=${CLUSTER_NAME} OpenIDConnectIdentityProvider=$(aws eks describe-cluster --name ${CLUSTER_NAME} | jq -r ".cluster.identity.oidc.issuer" | cut -c9-)
```

### Setup IRSA, Kit Controller Role
```bash
# Enables IRSA for your cluster. This command is idempotent, but only needs to be executed once per cluster.
eksctl utils associate-iam-oidc-provider \
--region ${AWS_REGION} \
--cluster ${CLUSTER_NAME} \
--approve

# Setup service account (Note: service account gets created as part of `ko apply` in the deploy section above)
kubectl patch serviceaccount kit -n kit --patch "$(cat <<-EOM
metadata:
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::${AWS_ACCOUNT_ID}:role/KitControllerRole-${CLUSTER_NAME}
EOM
)"

# Restart Kit controller to setup service account with the right set of permissions
kubectl delete pods -n kit -l control-plane=kit
```

## Delete KIT
To delete KIT from Kubernetes cluster

```bash
ko delete -f config/
```