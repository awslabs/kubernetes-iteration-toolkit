#!/bin/bash

# Script to validate AWS VPC CNI configuration
# 1. aws-node vpc cni version is >= 1.19.4
# 2. aws-node container env vars
# 3. VPC subnets have /12 CIDR with prefix reservation

set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# Function to check if a version is greater than or equal to another
version_ge() {
  [[ "$(echo -e "$1\n$2" | sort -V | head -n1)" == "$2" ]]
}

# Function to validate VPC CNI settings
validate_vpc_cni_setting() {
  # 1. Check aws-node VPC CNI version
  echo "Checking aws-node VPC CNI version..."
  CNI_VERSION=$(kubectl get daemonset aws-node -n kube-system -o jsonpath='{.spec.template.spec.containers[?(@.name=="aws-node")].image}' | grep -oE '[0-9]+\.[0-9]+\.[0-9]+')

  if [ -z "$CNI_VERSION" ]; then
    echo -e "${RED}[FAIL]${NC} Could not determine aws-node VPC CNI version"
    return 1
  fi

  if version_ge "$CNI_VERSION" "1.19.4"; then
    echo -e "${GREEN}[PASS]${NC} aws-node VPC CNI version $CNI_VERSION is >= 1.19.4"
  else
    echo -e "${RED}[FAIL]${NC} aws-node VPC CNI version $CNI_VERSION is < 1.19.4"
    return 1
  fi

  # 2. Check required environment variables
  echo "Checking aws-node container environment variables..."

  # Get environment variables from the aws-node container
  DISABLE_LEAKED_ENI_CLEANUP=$(kubectl get daemonset aws-node -n kube-system -o jsonpath='{.spec.template.spec.containers[?(@.name=="aws-node")].env[?(@.name=="DISABLE_LEAKED_ENI_CLEANUP")].value}')
  ENABLE_PREFIX_DELEGATION=$(kubectl get daemonset aws-node -n kube-system -o jsonpath='{.spec.template.spec.containers[?(@.name=="aws-node")].env[?(@.name=="ENABLE_PREFIX_DELEGATION")].value}')
  MINIMUM_IP_TARGET=$(kubectl get daemonset aws-node -n kube-system -o jsonpath='{.spec.template.spec.containers[?(@.name=="aws-node")].env[?(@.name=="MINIMUM_IP_TARGET")].value}')
  WARM_IP_TARGET=$(kubectl get daemonset aws-node -n kube-system -o jsonpath='{.spec.template.spec.containers[?(@.name=="aws-node")].env[?(@.name=="WARM_IP_TARGET")].value}')

  local validation_failed=0

  # Check DISABLE_LEAKED_ENI_CLEANUP
  if [ "$DISABLE_LEAKED_ENI_CLEANUP" == "true" ]; then
    echo -e "${GREEN}[PASS]${NC} DISABLE_LEAKED_ENI_CLEANUP is set to 'true'"
  else
    echo -e "${RED}[FAIL]${NC} DISABLE_LEAKED_ENI_CLEANUP is not set to 'true'. Current value: $DISABLE_LEAKED_ENI_CLEANUP"
    validation_failed=1
  fi

  # Check ENABLE_PREFIX_DELEGATION
  if [ "$ENABLE_PREFIX_DELEGATION" == "true" ]; then
    echo -e "${GREEN}[PASS]${NC} ENABLE_PREFIX_DELEGATION is set to 'true'"
  else
    echo -e "${RED}[FAIL]${NC} ENABLE_PREFIX_DELEGATION is not set to 'true'. Current value: $ENABLE_PREFIX_DELEGATION"
    validation_failed=1
  fi

  # Check MINIMUM_IP_TARGET
  if [ "$MINIMUM_IP_TARGET" == "30" ]; then
    echo -e "${GREEN}[PASS]${NC} MINIMUM_IP_TARGET is set to '30'"
  else
    echo -e "${RED}[FAIL]${NC} MINIMUM_IP_TARGET is not set to '30'. Current value: $MINIMUM_IP_TARGET"
    validation_failed=1
  fi

  # Check WARM_IP_TARGET
  if [ "$WARM_IP_TARGET" == "5" ]; then
    echo -e "${GREEN}[PASS]${NC} WARM_IP_TARGET is set to '5'"
  else
    echo -e "${RED}[FAIL]${NC} WARM_IP_TARGET is not set to '5'. Current value: $WARM_IP_TARGET"
    validation_failed=1
  fi

  return $validation_failed
}

# Function to validate AWS VPC configuration
validate_aws_vpc_config() {
  echo "Checking VPC subnets for /12 CIDR blocks and prefix delegation..."

  # Get cluster VPC ID from EKS cluster
  CLUSTER_NAME=$(kubectl config current-context | cut -d '/' -f2)
  VPC_ID=$(aws eks describe-cluster --endpoint=https://api.beta.us-west-2.wesley.amazonaws.com --name $CLUSTER_NAME --query 'cluster.resourcesVpcConfig.vpcId' --output text 2>/dev/null)

  if [ -z "$VPC_ID" ]; then
    echo -e "${RED}[FAIL]${NC} Could not determine VPC ID"
    return 1
  fi
  
  echo "VPC_ID: $VPC_ID"

  # Get subnets in the VPC
  SUBNETS=$(aws ec2 describe-subnets --filters "Name=vpc-id,Values=$VPC_ID" --query 'Subnets[*].{ID:SubnetId,CIDR:CidrBlock}' --output json)

  if [ -z "$SUBNETS" ]; then
    echo -e "${RED}[FAIL]${NC} Could not retrieve subnets for VPC $VPC_ID"
    return 1
  fi

  # Check if subnets have /12 CIDR blocks
  SUBNET_COUNT=$(echo $SUBNETS | jq length)
  VALID_SUBNET_COUNT=0

  for ((i=0; i<$SUBNET_COUNT; i++)); do
    SUBNET_ID=$(echo $SUBNETS | jq -r ".[$i].ID")
    CIDR_BLOCK=$(echo $SUBNETS | jq -r ".[$i].CIDR")
    CIDR_PREFIX=$(echo $CIDR_BLOCK | cut -d '/' -f2)
    
    if [ "$CIDR_PREFIX" == "12" ]; then
      # Check for subnet CIDR reservations using the correct command
      PREFIX_COUNT=$(aws ec2 get-subnet-cidr-reservations --subnet-id $SUBNET_ID --query 'length(SubnetIpv4CidrReservations[?ReservationType==`prefix`])' --output text 2>/dev/null || echo "0")
      
      if [ "$PREFIX_COUNT" -gt 0 ]; then
        echo -e "${GREEN}[PASS]${NC} Subnet $SUBNET_ID has /12 CIDR block and prefix reservations"
        VALID_SUBNET_COUNT=$((VALID_SUBNET_COUNT + 1))
      else
        echo -e "${YELLOW}[WARN]${NC} Subnet $SUBNET_ID has /12 CIDR block but no prefix reservations found"
      fi
    else
      echo -e "${YELLOW}[WARN]${NC} Subnet $SUBNET_ID has /$CIDR_PREFIX CIDR block (not /12)"
    fi
  done

  if [ $VALID_SUBNET_COUNT -gt 0 ]; then
    echo -e "${GREEN}[PASS]${NC} Found $VALID_SUBNET_COUNT subnets with /12 CIDR blocks and prefix delegation"
    return 0
  else
    echo -e "${RED}[FAIL]${NC} No subnets with /12 CIDR blocks and prefix delegation found"
    return 1
  fi
}

# Run validations
validation_failed=0

if ! validate_vpc_cni_setting; then
  validation_failed=1
fi

if ! validate_aws_vpc_config; then
  validation_failed=1
fi

# Check if all validations passed
if [ $validation_failed -eq 1 ]; then
  echo -e "${RED}Validation FAILED.${NC} Please address the issues highlighted above."
  exit 1
else
  echo -e "${GREEN}All validations PASSED.${NC}"
  exit 0
fi
