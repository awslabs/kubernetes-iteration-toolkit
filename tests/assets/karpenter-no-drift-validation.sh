#!/bin/bash

# Script to validate that an eks cluster with karpenter installed will not drift
# Validates:
#  1. Nodepools have a disruption budget of 0% set
#  2. Nodeclasses have pinned ami's
echo "Checking disruption budgets in nodepool specs..."
echo "-----------------------------------"

FAILED=0
# Get all nodepools and check their disruption budget settings
for nodepool in $(kubectl get nodepools -o json | jq -r '.items[] | {name: .metadata.name, budgets: .spec.disruption.budgets[].nodes} | @json'); do
    echo $nodepool
    NAME=$(echo $nodepool | jq -r '.name')
    NDB=$(echo $nodepool | jq -r '.budgets')
    
    # Remove any % symbol and convert to number
    NDB_NUM=$(echo $NDB | sed 's/%//')
    
    if [ "$NDB_NUM" -eq 0 ]; then
        echo "✅ Disruption budget correctly set to $NDB for nodepool: $NAME"
    else
        echo "❌ Disruption budget too high for nodepool: $NAME (current: $NDB)"
        export FAILED=1
    fi
done

echo "Checking AMI versions in EC2NodeClass resources..."
echo "------------------------------------------------"

# Get EC2NodeClass resources and check for @latest
for nodeclass in $(kubectl get ec2nodeclasses -o json | jq -r '.items[] | {name: .metadata.name, ami: .spec.amiFamily, amiSelector: .spec.amiSelectorTerms} | @json'); do
    NAME=$(echo $nodeclass | jq -r '.name')
    AMI_FAMILY=$(echo $nodeclass | jq -r '.ami')
    AMI_SELECTOR=$(echo $nodeclass | jq -r '.amiSelector')
    
    echo "NodeClass: $NAME"
    echo "AMI Family: $AMI_FAMILY"
    echo "AMI Selector Terms: $AMI_SELECTOR"
    
    # Check if @latest is used in any selector terms
    if echo "$AMI_SELECTOR" | grep -q "@latest"; then
        echo "❌ WARNING: @latest version detected in NodeClass $NAME"
        export FAILED=1
    else
        echo "✅ No @latest version found in NodeClass $NAME"
    fi
    echo "------------------------------------------------"
done

echo "-----------------------------------"
if [ $FAILED -eq 1 ]; then
    echo "❌ Some nodepools or nodeclasses do not have the correct configuration"
    exit 1
else
    echo "✅ All nodepools or nodeclasses have the correct configuration"
    exit 0
fi
