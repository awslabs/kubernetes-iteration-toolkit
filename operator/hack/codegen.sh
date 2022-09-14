#!/bin/bash
set -ex

controller-gen crd \
    object:headerFile="hack/boilerplate.go.txt" \
    webhook \
    paths="./pkg/..." \
    output:crd:artifacts:config=config \
    output:webhook:artifacts:config=config

./hack/boilerplate.sh

# CRDs don't currently jive with VolatileTime, which has an Any type.
perl -pi -e 's/Any/string/g' config/kit.k8s.sh_controlplanes.yaml

# Kubectl apply fails if the annotations is too long with error -
# The CustomResourceDefinition "controlplanes.kit.k8s.sh" is invalid: metadata.annotations: Too long: must have at most 262144 bytes
# More info: https://stackoverflow.com/a/62409266
yq eval 'del(.. | select(has("description")).description)' -i config/kit.k8s.sh_controlplanes.yaml
yq eval 'del(.. | select(has("ephemeralContainers")).ephemeralContainers)' -i config/kit.k8s.sh_controlplanes.yaml
yq eval 'del(.. | select(has("initContainers")).initContainers)' -i config/kit.k8s.sh_controlplanes.yaml


mv config/kit.k8s.sh_controlplanes.yaml charts/kit-operator/crds/control-plane-crd.yaml
mv config/kit.k8s.sh_dataplanes.yaml charts/kit-operator/crds/data-plane-crd.yaml
