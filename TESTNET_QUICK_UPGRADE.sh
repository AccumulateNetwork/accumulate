#!/bin/bash

# Quick Testnet Upgrade Script (No Backup, Aggressive Timing)
# For testnet only - NOT for production!

set -e

# Configuration - UPDATE THESE
TESTNET_NODES=("node1" "node2" "node3" "node4")  # Update with actual hostnames/IPs
VERSION="v1.5.0-experimental"
BRANCH="3653-add-a-crosschainconductor-process-for-coordinating-partitions"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}=== Quick Testnet Upgrade to ${VERSION} ===${NC}"
echo "Nodes: ${TESTNET_NODES[@]}"
echo ""

# Parallel execution function
parallel_exec() {
    local cmd="$1"
    echo -e "${YELLOW}Running: $cmd${NC}"
    for node in "${TESTNET_NODES[@]}"; do
        ssh "$node" "$cmd" &
    done
    wait
}

# Step 1: Kill all nodes immediately
echo "1. Stopping all nodes..."
parallel_exec "pkill -9 accumulate || true"
sleep 2

# Step 2: Pull and build (parallel)
echo "2. Upgrading code..."
parallel_exec "cd /home/accumulate/go/src/gitlab.com/AccumulateNetwork/accumulate && git fetch && git checkout $BRANCH && git pull && make clean && make build && sudo make install"

# Step 3: Start all nodes (parallel)
echo "3. Starting nodes..."
parallel_exec "accumulate start > /var/log/accumulate.log 2>&1 &"

# Step 4: Quick verification
echo "4. Waiting 10 seconds for network..."
sleep 10

echo "5. Checking status..."
ssh "${TESTNET_NODES[0]}" "accumulate network status | head -5"

echo -e "${GREEN}Done! Upgrade complete in ~30 seconds${NC}"
echo ""
echo "Monitor with:"
echo "  ssh ${TESTNET_NODES[0]} 'accumulate network status'"
echo "  ssh ${TESTNET_NODES[0]} 'tail -f /var/log/accumulate.log'"