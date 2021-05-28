#!/bin/bash

controller-gen \
    object:headerFile="hack/boilerplate.go.txt" \
    webhook \
    crd:trivialVersions=false \
    paths="./pkg/..." \
    output:crd:artifacts:config=config \
    output:webhook:artifacts:config=config

./hack/boilerplate.sh

# CRDs don't currently jive with VolatileTime, which has an Any type.
perl -pi -e 's/Any/string/g' config/*.yaml
