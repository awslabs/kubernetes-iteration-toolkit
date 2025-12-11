#!/usr/bin/env bash

set -e

# --- 1. Find all Static Tool Versions (Combined into a single JSON object) ---

HELM_VERSION=$(curl -s https://api.github.com/repos/helm/helm/releases | jq -r '.[].tag_name | select([startswith("v"), (contains("-") | not)] | all)' | sort -rV | head -n 1 | sed 's/v//')
KUSTOMIZE_RELEASE=$(curl -s https://api.github.com/repos/kubernetes-sigs/kustomize/releases | jq -r '.[].tag_name | select(contains("kustomize"))' | sort -rV | head -n 1)
KUSTOMIZE_VERSION=$(basename ${KUSTOMIZE_RELEASE})
KUBESEAL_VERSION=$(curl -s https://api.github.com/repos/bitnami-labs/sealed-secrets/releases | jq -r '.[].tag_name | select(startswith("v"))' | sort -rV | head -n 1 | sed 's/v//')
KREW_VERSION=$(curl -s https://api.github.com/repos/kubernetes-sigs/krew/releases | jq -r '.[].tag_name | select(startswith("v"))' | sort -rV | head -n 1 | sed 's/v//')
VALS_VERSION=$(curl -s https://api.github.com/repos/helmfile/vals/releases | jq -r '.[].tag_name | select(startswith("v"))' | sort -rV | head -n 1 | sed 's/v//')
KUBECONFORM_VERSION=$(curl -s https://api.github.com/repos/yannh/kubeconform/releases | jq -r '.[].tag_name | select(startswith("v"))' | sort -rV | head -n 1 | sed 's/v//')


# Construct a single, compacted JSON object without extra spaces
LATEST_TOOLS_JSON=$(
  jq -n -c \
    --arg helm "$HELM_VERSION" \
    --arg kustomize "$KUSTOMIZE_VERSION" \
    --arg kubeseal "$KUBESEAL_VERSION" \
    --arg krew "$KREW_VERSION" \
    --arg vals "$VALS_VERSION" \
    --arg kubeconform "$KUBECONFORM_VERSION" \
    '{
      "helm_version": $helm, 
      "kustomize_version": $kustomize, 
      "kubeseal_version": $kubeseal, 
      "krew_version": $krew, 
      "vals_version": $vals, 
      "kubeconform_version": $kubeconform
    }'
)

# Use the 'LATEST_TOOLS_JSON' variable directly, ensuring no leading spaces
echo "latest_tools=$LATEST_TOOLS_JSON" >> $GITHUB_OUTPUT

# Optional: Keep the echo for logging, but ONLY to stdout, not GITHUB_OUTPUT
echo "Found static tools: $LATEST_TOOLS_JSON"


# --- 2. Find the top 4 latest K8s minor versions (Output as a JSON Array) ---

RELEASES=$(curl -s https://api.github.com/repos/kubernetes/kubernetes/releases | jq -r '.[].tag_name | select(test("alpha|beta|rc") | not)')

MINOR_VERSIONS=()
for RELEASE in $RELEASES; do
  MINOR_VERSION=$(echo $RELEASE | awk -F'.' '{print $1"."$2}')
  if [[ ! " ${MINOR_VERSIONS[@]} " =~ " ${MINOR_VERSION} " ]]; then
    MINOR_VERSIONS+=($MINOR_VERSION)
  fi
done

SORTED_MINOR_VERSIONS=($(echo "${MINOR_VERSIONS[@]}" | tr ' ' '\n' | sort -rV))

K8S_TAGS=()
for i in $(seq 0 3); do
  MINOR_VERSION="${SORTED_MINOR_VERSIONS[$i]}"
  LATEST_VERSION=$(echo "$RELEASES" | grep "^$MINOR_VERSION\." | sort -rV | head -1 | sed 's/v//')
  K8S_TAGS+=("$LATEST_VERSION")
done

# Convert the bash array into a single-line JSON array string (using -c flag for compact output)
K8S_TAGS_JSON=$(printf '%s\n' "${K8S_TAGS[@]}" | jq -R . | jq -s -c .)

echo "k8s_versions=$K8S_TAGS_JSON" >> $GITHUB_OUTPUT
echo "Found K8s versions: ${K8S_TAGS[*]}"
