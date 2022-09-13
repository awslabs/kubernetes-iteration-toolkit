#!/bin/bash

controller-gen \
		object:headerFile="hack/boilerplate.go.txt" \
		paths="./pkg/..."

./hack/boilerplate.sh
