#!/bin/bash

controller-gen \
    object:headerFile="hack/boilerplate.go.txt" \
    webhook \
    crd:trivialVersions=false \
    paths="./pkg/..." \
    output:crd:artifacts:config=config \
    output:webhook:artifacts:config=config

./hack/boilerplate.sh

mv config/kit.k8s.amazonaws.com_controlplanes.yaml config/control-plane-crd.yaml
# CRDs don't currently jive with VolatileTime, which has an Any type.
perl -pi -e 's/Any/string/g' config/control-plane-crd.yaml

# Kubectl apply fails if the annotations is too long with error -
# The CustomResourceDefinition "controlplanes.kit.k8s.amazonaws.com" is invalid: metadata.annotations: Too long: must have at most 262144 bytes
# More info: https://stackoverflow.com/a/62409266
yq eval 'del(.. | select(has("description")).description)' -i config/control-plane-crd.yaml
yq eval 'del(.. | select(has("ephemeralContainers")).ephemeralContainers)' -i config/control-plane-crd.yaml
yq eval 'del(.. | select(has("initContainers")).initContainers)' -i config/control-plane-crd.yaml