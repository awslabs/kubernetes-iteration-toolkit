#!/bin/bash

set -eu -o pipefail

TEMP_DIR=$(mktemp -d)
trap "rm -rf $TEMP_DIR" EXIT

main() {
    tools
}

tools() {
    cd tools
    go mod tidy
    GO111MODULE=on cat tools.go | grep _ | awk -F'"' '{print $2}' | xargs -tI % go install -v %
}

main "$@"