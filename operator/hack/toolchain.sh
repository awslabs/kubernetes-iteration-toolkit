#!/bin/bash

set -eu -o pipefail

main() {
    tools
    kubebuilder
}

tools() {
    cd tools
    go mod tidy
    GO111MODULE=on cat tools.go | grep _ | awk -F'"' '{print $2}' | xargs -tI % go install -v %
}

kubebuilder() {
    KUBEBUILDER_ASSETS="/usr/local/kubebuilder"
    sudo rm -rf $KUBEBUILDER_ASSETS
    sudo mkdir -p $KUBEBUILDER_ASSETS
    sudo mv "$(setup-envtest use -p path 1.19.x)" $KUBEBUILDER_ASSETS/bin
    find $KUBEBUILDER_ASSETS
}

main "$@"