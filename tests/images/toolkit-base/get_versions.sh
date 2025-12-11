#!/usr/bin/env bash

set -e

# Make sure jq is installed for this script to work on the runner
# (The workflow will install it)

# 1. helm latest
HELM_VERSION=$(curl -s https://api.github.com/repos/helm/helm/releases | jq -r '.[].tag_name | select([startswith("v"), (contains("-") | not)] | all)' | sort -rV | head -n 1 | sed 's/v//')
echo "HELM_VERSION=$HELM_VERSION"
echo "helm_version=$HELM_VERSION" >> $GITHUB_OUTPUT

# 2. kustomize latest
KUSTOMIZE_RELEASE=$(curl -s https://api.github.com/repos/kubernetes-sigs/kustomize/releases | jq -r '.[].tag_name | select(contains("kustomize"))' | sort -rV | head -n 1)
KUSTOMIZE_VERSION=$(basename ${KUSTOMIZE_RELEASE})
echo "KUSTOMIZE_VERSION=$KUSTOMIZE_VERSION"
echo "kustomize_version=$KUSTOMIZE_VERSION" >> $GITHUB_OUTPUT

# 3. kubeseal latest
KUBESEAL_VERSION=$(curl -s https://api.github.com/repos/bitnami-labs/sealed-secrets/releases | jq -r '.[].tag_name | select(startswith("v"))' | sort -rV | head -n 1 | sed 's/v//')
echo "KUBESEAL_VERSION=$KUBESEAL_VERSION"
echo "kubeseal_version=$KUBESEAL_VERSION" >> $GITHUB_OUTPUT

# 4. krew latest
KREW_VERSION=$(curl -s https://api.github.com/repos/kubernetes-sigs/krew/releases | jq -r '.[].tag_name | select(startswith("v"))' | sort -rV | head -n 1 | sed 's/v//')
echo "KREW_VERSION=$KREW_VERSION"
echo "krew_version=$KREW_VERSION" >> $GITHUB_OUTPUT

# 5. vals latest
VALS_VERSION=$(curl -s https://api.github.com/repos/helmfile/vals/releases | jq -r '.[].tag_name | select(startswith("v"))' | sort -rV | head -n 1 | sed 's/v//')
echo "VALS_VERSION=$VALS_VERSION"
echo "vals_version=$VALS_VERSION" >> $GITHUB_OUTPUT

# 6. kubeconform latest
KUBECONFORM_VERSION=$(curl -s https://api.github.com/repos/yannh/kubeconform/releases | jq -r '.[].tag_name | select(startswith("v"))' | sort -rV | head -n 1 | sed 's/v//')
echo "KUBECONFORM_VERSION=$KUBECONFORM_VERSION"
echo "kubeconform_version=$KUBECONFORM_VERSION" >> $GITHUB_OUTPUT

# 7. Kubectl/K8s tag determination (your complex logic)
# This will be used as the image tag AND as the KUBECTL_VERSION build-arg
# For simplicity, let's just grab the latest stable K8s version
K8S_TAG=$(curl -s https://api.github.com/repos/kubernetes/kubernetes/releases | jq -r '.[].tag_name | select(test("alpha|beta|rc") | not)' | sort -rV | head -n 1 | sed 's/v//')
echo "K8S_TAG=$K8S_TAG"
echo "k8s_tag=$K8S_TAG" >> $GITHUB_OUTPUT
