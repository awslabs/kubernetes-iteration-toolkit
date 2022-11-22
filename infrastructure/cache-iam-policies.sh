#!/usr/bin/env bash
set -euo pipefail

# Downloads and saves cached versions of IAM policies for aws-load-balancer-controller and aws-ebs-csi-driver
# This file must be executed to update the cache when these two dependencies are upgraded

CACHE_DIR="./lib/addons/cached"

LOAD_BALANCER_CONTROLLER_VERSION="v2.4.2"
LOAD_BALANCER_CACHED_FILE="${CACHE_DIR}/aws-load-balancer-controller-iam-policy-${LOAD_BALANCER_CONTROLLER_VERSION}.json"
LOAD_BALANCER_CACHED_URL="https://raw.githubusercontent.com/kubernetes-sigs/aws-load-balancer-controller/${LOAD_BALANCER_CONTROLLER_VERSION}/docs/install/iam_policy.json"

EBS_CSI_DRIVER_VERSION="v1.9.0"
EBS_CSI_DRIVER_FILE="${CACHE_DIR}/aws-ebs-csi-driver-iam-policy-${EBS_CSI_DRIVER_VERSION}.json"
EBS_CSI_DRIVER_URL="https://raw.githubusercontent.com/kubernetes-sigs/aws-ebs-csi-driver/${EBS_CSI_DRIVER_VERSION}/docs/example-iam-policy.json"

curl -o "${LOAD_BALANCER_CACHED_FILE}" "${LOAD_BALANCER_CACHED_URL}"
curl -o "${EBS_CSI_DRIVER_FILE}" "${EBS_CSI_DRIVER_URL}"
