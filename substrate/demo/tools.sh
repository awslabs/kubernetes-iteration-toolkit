#!/bin/bash -eu

KIND_VERSION=v0.11.1
CAPA_VERSION=v1.3.0
CAPI_VERSION=v1.0.4

OS=$(uname | tr 'A-Z' 'a-z')
ARCH=amd64

# Install Kind
go install sigs.k8s.io/kind@${KIND_VERSION}
# Install CAPI
curl -L "https://github.com/kubernetes-sigs/cluster-api/releases/download/${CAPI_VERSION}/clusterctl-${OS}-${ARCH}" -o /usr/local/bin/clusterctl
chmod +x /usr/local/bin/clusterctl
# Install CAPA
curl -L "https://github.com/kubernetes-sigs/cluster-api-provider-aws/releases/download/${CAPA_VERSION}/clusterawsadm-${OS}-${ARCH}" -o /usr/local/bin/clusterawsadm
chmod +x /usr/local/bin/clusterawsadm
