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

# 1. Check aws-node VPC CNI version
echo "Checking aws-node VPC CNI version..."
CNI_VERSION=$(kubectl get daemonset aws-node -n kube-system -o jsonpath='{.spec.template.spec.containers[?(@.name=="aws-node")].image}' | grep -oE '[0-9]+\.[0-9]+\.[0-9]+')

if [ -z "$CNI_VERSION" ]; then
  echo -e "${RED}[FAIL]${NC} Could not determine aws-node VPC CNI version"
  exit 1
fi

if version_ge "$CNI_VERSION" "1.19.4"; then
  echo -e "${GREEN}[PASS]${NC} aws-node VPC CNI version $CNI_VERSION is >= 1.19.4"
else
  echo -e "${RED}[FAIL]${NC} aws-node VPC CNI version $CNI_VERSION is < 1.19.4"
  exit 1
fi

# 2. Check required environment variables
echo "Checking aws-node container environment variables..."

# Get environment variables from the aws-node container
DISABLE_LEAKED_ENI_CLEANUP=$(kubectl get daemonset aws-node -n kube-system -o jsonpath='{.spec.template.spec.containers[?(@.name=="aws-node")].env[?(@.name=="DISABLE_LEAKED_ENI_CLEANUP")].value}')
ENABLE_PREFIX_DELEGATION=$(kubectl get daemonset aws-node -n kube-system -o jsonpath='{.spec.template.spec.containers[?(@.name=="aws-node")].env[?(@.name=="ENABLE_PREFIX_DELEGATION")].value}')
MINIMUM_IP_TARGET=$(kubectl get daemonset aws-node -n kube-system -o jsonpath='{.spec.template.spec.containers[?(@.name=="aws-node")].env[?(@.name=="MINIMUM_IP_TARGET")].value}')
WARM_IP_TARGET=$(kubectl get daemonset aws-node -n kube-system -o jsonpath='{.spec.template.spec.containers[?(@.name=="aws-node")].env[?(@.name=="WARM_IP_TARGET")].value}')

# Check DISABLE_LEAKED_ENI_CLEANUP
if [ "$DISABLE_LEAKED_ENI_CLEANUP" == "true" ]; then
  echo -e "${GREEN}[PASS]${NC} DISABLE_LEAKED_ENI_CLEANUP is set to 'true'"
else
  echo -e "${RED}[FAIL]${NC} DISABLE_LEAKED_ENI_CLEANUP is not set to 'true'. Current value: $DISABLE_LEAKED_ENI_CLEANUP"
fi

# Check ENABLE_PREFIX_DELEGATION
if [ "$ENABLE_PREFIX_DELEGATION" == "true" ]; then
  echo -e "${GREEN}[PASS]${NC} ENABLE_PREFIX_DELEGATION is set to 'true'"
else
  echo -e "${RED}[FAIL]${NC} ENABLE_PREFIX_DELEGATION is not set to 'true'. Current value: $ENABLE_PREFIX_DELEGATION"
fi

# Check MINIMUM_IP_TARGET
if [ "$MINIMUM_IP_TARGET" == "30" ]; then
  echo -e "${GREEN}[PASS]${NC} MINIMUM_IP_TARGET is set to '30'"
else
  echo -e "${RED}[FAIL]${NC} MINIMUM_IP_TARGET is not set to '30'. Current value: $MINIMUM_IP_TARGET"
fi

# Check WARM_IP_TARGET
if [ "$WARM_IP_TARGET" == "5" ]; then
  echo -e "${GREEN}[PASS]${NC} WARM_IP_TARGET is set to '5'"
else
  echo -e "${RED}[FAIL]${NC} WARM_IP_TARGET is not set to '5'. Current value: $WARM_IP_TARGET"
fi

# 3. Check VPC subnets CIDR blocks and prefix delegation reservations
echo "Checking VPC subnets for /12 CIDR blocks and prefix delegation..."

# Get cluster VPC ID
VPC_ID=$(aws ec2 describe-instances --instance-ids $(kubectl get nodes -o jsonpath='{.items[0].spec.providerID}' | cut -d '/' -f5) --query 'Reservations[0].Instances[0].VpcId' --output text)

if [ -z "$VPC_ID" ]; then
  echo -e "${RED}[FAIL]${NC} Could not determine VPC ID"
  exit 1
fi

# Get subnets in the VPC
SUBNETS=$(aws ec2 describe-subnets --filters "Name=vpc-id,Values=$VPC_ID" --query 'Subnets[*].{ID:SubnetId,CIDR:CidrBlock}' --output json)

if [ -z "$SUBNETS" ]; then
  echo -e "${RED}[FAIL]${NC} Could not retrieve subnets for VPC $VPC_ID"
  exit 1
fi

# Check if subnets have /12 CIDR blocks
SUBNET_COUNT=$(echo $SUBNETS | jq length)
VALID_SUBNET_COUNT=0

for ((i=0; i<$SUBNET_COUNT; i++)); do
  SUBNET_ID=$(echo $SUBNETS | jq -r ".[$i].ID")
  CIDR_BLOCK=$(echo $SUBNETS | jq -r ".[$i].CIDR")
  CIDR_PREFIX=$(echo $CIDR_BLOCK | cut -d '/' -f2)
  
  if [ "$CIDR_PREFIX" == "12" ]; then
    # Check for subnet CIDR reservations
    IPAM_POOLS=$(aws ec2 describe-ipam-pools --filters "Name=description,Values=*$SUBNET_ID*" --query 'IpamPools[*].{ID:IpamPoolId}' --output json)
    
    if [ "$(echo $IPAM_POOLS | jq length)" -gt 0 ]; then
      echo -e "${GREEN}[PASS]${NC} Subnet $SUBNET_ID has /12 CIDR block and IPAM pool reservation"
      VALID_SUBNET_COUNT=$((VALID_SUBNET_COUNT + 1))
    else
      # Alternative check for CIDR reservations using subnet attributes
      SUBNET_ATTRS=$(aws ec2 describe-subnets --subnet-ids $SUBNET_ID --query 'Subnets[0]' --output json)
      
      # Check if subnet has prefix delegation enabled
      if echo "$SUBNET_ATTRS" | grep -q "true"; then
        echo -e "${GREEN}[PASS]${NC} Subnet $SUBNET_ID has /12 CIDR block and appears to have prefix delegation enabled"
        VALID_SUBNET_COUNT=$((VALID_SUBNET_COUNT + 1))
      else
        echo -e "${YELLOW}[WARN]${NC} Subnet $SUBNET_ID has /12 CIDR block but could not confirm prefix delegation"
      fi
    fi
  else
    echo -e "${YELLOW}[WARN]${NC} Subnet $SUBNET_ID has /$CIDR_PREFIX CIDR block (not /12)"
  fi
done

if [ $VALID_SUBNET_COUNT -gt 0 ]; then
  echo -e "${GREEN}[PASS]${NC} Found $VALID_SUBNET_COUNT subnets with /12 CIDR blocks and prefix delegation"
else
  echo -e "${RED}[FAIL]${NC} No subnets with /12 CIDR blocks and prefix delegation found"
fi

# Check if all validations passed
if [ "$CNI_VERSION" \< "1.19.4" ] || \
   [ "$DISABLE_LEAKED_ENI_CLEANUP" != "true" ] || \
   [ "$ENABLE_PREFIX_DELEGATION" != "true" ] || \
   [ "$MINIMUM_IP_TARGET" != "30" ] || \
   [ "$WARM_IP_TARGET" != "5" ] || \
   [ $VALID_SUBNET_COUNT -eq 0 ]; then
  echo -e "${RED}Validation FAILED.${NC} Please address the issues highlighted above."
  exit 1
else
  echo -e "${GREEN}All validations PASSED.${NC}"
  exit 0
fi
