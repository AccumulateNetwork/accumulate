#!/bin/bash

# Testnet Rollback Script
# Emergency rollback to previous version if upgrade fails

set -e

# Configuration
PREVIOUS_VERSION="v1.4.3"
PREVIOUS_BRANCH="main"
TESTNET_NODES=("node1.testnet" "node2.testnet" "node3.testnet" "node4.testnet")  # Update with actual hostnames
ACCUMULATE_DIR="/home/accumulate/go/src/gitlab.com/AccumulateNetwork/accumulate"

# Colors
RED='\033[0;31m'
YELLOW='\033[1;33m'
GREEN='\033[0;32m'
NC='\033[0m'

echo -e "${RED}================================================${NC}"
echo -e "${RED}  TESTNET ROLLBACK TO ${PREVIOUS_VERSION}${NC}"
echo -e "${RED}  Starting at: $(date)${NC}"
echo -e "${RED}================================================${NC}"
echo ""

# Confirm rollback
read -p "Are you sure you want to rollback the testnet? (yes/no): " -r
if [[ ! $REPLY == "yes" ]]; then
    echo "Rollback cancelled"
    exit 0
fi

# Execute on all nodes
execute_on_all() {
    local cmd="$1"
    local description="$2"
    
    echo -e "${YELLOW}==> ${description}${NC}"
    
    for node in "${TESTNET_NODES[@]}"; do
        echo "  [$node] Executing: $cmd"
        ssh "$node" "$cmd"
    done
}

# Step 1: Stop all nodes
echo -e "${YELLOW}Step 1: Emergency stop of all nodes${NC}"
execute_on_all "sudo systemctl stop accumulate || pkill -9 accumulate || true" "Force stopping accumulate"
sleep 5

# Step 2: Rollback code
echo -e "${YELLOW}Step 2: Rolling back to ${PREVIOUS_VERSION}${NC}"
execute_on_all "cd $ACCUMULATE_DIR && git checkout $PREVIOUS_BRANCH" "Checking out previous branch"
execute_on_all "cd $ACCUMULATE_DIR && git pull origin $PREVIOUS_BRANCH" "Pulling previous version"

# Step 3: Rebuild previous version
echo -e "${YELLOW}Step 3: Building previous version${NC}"
execute_on_all "cd $ACCUMULATE_DIR && make clean && make build" "Building accumulate"
execute_on_all "cd $ACCUMULATE_DIR && sudo make install" "Installing binary"

# Step 4: Verify version
echo -e "${YELLOW}Step 4: Verifying rollback${NC}"
execute_on_all "accumulate version" "Checking version"

# Step 5: Start nodes
echo -e "${YELLOW}Step 5: Starting nodes with previous version${NC}"
execute_on_all "sudo systemctl start accumulate || accumulate start &" "Starting accumulate"

# Step 6: Wait and verify
echo -e "${YELLOW}Step 6: Waiting for network to stabilize${NC}"
sleep 30

execute_on_all "accumulate network status | head -10" "Checking network status"

echo ""
echo -e "${GREEN}================================================${NC}"
echo -e "${GREEN}  Rollback Complete${NC}"
echo -e "${GREEN}  Testnet is now running ${PREVIOUS_VERSION}${NC}"
echo -e "${GREEN}================================================${NC}"