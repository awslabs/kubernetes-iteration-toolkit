#!/bin/bash -eu

# Delete Cluster
clusterctl move --kubeconfig ./substrate.kubeconfig --to-kubeconfig ./bootstrap.kubeconfig
kubectl delete --kubeconfig bootstrap.kubeconfig -f ./substrate.yaml || true
kind delete cluster --name bootstrap
