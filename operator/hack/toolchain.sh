#!/bin/bash

set -eux -o pipefail

main() {
    tools
    kubebuilder
}

tools() {
    go install github.com/ahmetb/gen-crd-api-reference-docs@v0.1.5
    go install github.com/fzipp/gocyclo/cmd/gocyclo@v0.3.1
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.41.1
    go install github.com/google/ko@v0.11.2
    go install github.com/mikefarah/yq/v4@v4.16.1
    go install github.com/mitchellh/golicense@v0.2.0
    go install github.com/onsi/ginkgo/ginkgo@v1.16.5
    go install sigs.k8s.io/controller-runtime/tools/setup-envtest@v0.0.0-20220113220429-45b13b951f77
    go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.11.3

    if ! echo "$PATH" | grep -q "${GOPATH:-undefined}/bin\|$HOME/go/bin"; then
        echo "Go workspace's \"bin\" directory is not in PATH. Run 'export PATH=\"\$PATH:\${GOPATH:-\$HOME/go}/bin\"'."
    fi
}

kubebuilder() {
    # These assets are being used for setting up test environment in environment.go
    KUBEBUILDER_ASSETS="/usr/local/bin/kubebuilder-assets"
    rm -rf $KUBEBUILDER_ASSETS
    mkdir -p $KUBEBUILDER_ASSETS
    cp -r $(setup-envtest use -p path 1.19.x --bin-dir=/usr/local/bin/)/* /usr/local/bin/kubebuilder-assets/
    find $KUBEBUILDER_ASSETS
}

main "$@"
