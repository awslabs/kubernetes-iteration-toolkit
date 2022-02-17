#!/bin/bash -eu

# https://cluster-api-aws.sigs.k8s.io/getting-started.html
AWS_REGION=us-west-2

# Create Infrastructure
kind create cluster --name bootstrap --kubeconfig bootstrap.kubeconfig || true
clusterawsadm bootstrap iam create-cloudformation-stack --region ${AWS_REGION} # 0.12s user 0.07s system 0% cpu 2:31.64 total
export AWS_B64ENCODED_CREDENTIALS=$(clusterawsadm bootstrap credentials encode-as-profile --region ${AWS_REGION})
clusterctl init --infrastructure aws --kubeconfig bootstrap.kubeconfig

# Create Cluster
kubectl apply --kubeconfig bootstrap.kubeconfig -f ./substrate.yaml
kubectl get cluster substrate -w --kubeconfig bootstrap.kubeconfig

# Pivot Cluster
clusterctl get kubeconfig substrate --kubeconfig bootstrap.kubeconfig >substrate.kubeconfig
kubectl apply -f https://docs.projectcalico.org/v3.21/manifests/calico.yaml --kubeconfig substrate.kubeconfig
clusterctl init --infrastructure aws --kubeconfig substrate.kubeconfig
clusterctl move --kubeconfig bootstrap.kubeconfig --to-kubeconfig substrate.kubeconfig
