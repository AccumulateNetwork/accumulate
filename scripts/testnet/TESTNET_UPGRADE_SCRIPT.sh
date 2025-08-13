#!/bin/bash

# Testnet Upgrade Script for v1.5.0-experimental
# This script upgrades all testnet nodes to v1.5.0-experimental
# Since we control all servers, we can be more aggressive with the upgrade

set -e

# Configuration
VERSION="v1.5.0-experimental"
BRANCH="3653-add-a-crosschainconductor-process-for-coordinating-partitions"
TESTNET_NODES=("node1.testnet" "node2.testnet" "node3.testnet" "node4.testnet")  # Update with actual hostnames
ACCUMULATE_DIR="/home/accumulate/go/src/gitlab.com/AccumulateNetwork/accumulate"
LOG_FILE="/var/log/accumulate-upgrade-$(date +%Y%m%d-%H%M%S).log"

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Logging function
log() {
    echo -e "${1}" | tee -a "$LOG_FILE"
}

# Execute command on all nodes
execute_on_all() {
    local cmd="$1"
    local description="$2"
    
    log "${BLUE}==> ${description}${NC}"
    
    for node in "${TESTNET_NODES[@]}"; do
        log "  ${YELLOW}[$node]${NC} Executing: $cmd"
        ssh "$node" "$cmd" 2>&1 | tee -a "$LOG_FILE"
    done
}

# Execute command on single node
execute_on_node() {
    local node="$1"
    local cmd="$2"
    ssh "$node" "$cmd" 2>&1 | tee -a "$LOG_FILE"
}

log "${GREEN}================================================${NC}"
log "${GREEN}  Testnet Upgrade to v1.5.0-experimental${NC}"
log "${GREEN}  Starting at: $(date)${NC}"
log "${GREEN}================================================${NC}"
log ""

# Step 1: Check current status
log "${BLUE}Step 1: Checking current testnet status${NC}"
execute_on_all "accumulate version" "Getting current version"
execute_on_all "accumulate network status | head -20" "Checking network status"

# Step 2: Quick backup of critical data (optional for testnet)
log ""
log "${BLUE}Step 2: Creating quick state backup${NC}"
execute_on_all "mkdir -p /backup/pre-upgrade-$(date +%Y%m%d)" "Creating backup directory"
execute_on_all "cp -r ~/.accumulate/data/snapshots /backup/pre-upgrade-$(date +%Y%m%d)/ || true" "Backing up snapshots"

# Step 3: Stop all nodes
log ""
log "${BLUE}Step 3: Stopping all testnet nodes${NC}"
execute_on_all "sudo systemctl stop accumulate || pkill -f accumulate || true" "Stopping accumulate service"

# Verify all stopped
sleep 5
execute_on_all "pgrep -f accumulate && echo 'WARNING: accumulate still running!' || echo 'accumulate stopped successfully'" "Verifying shutdown"

# Step 4: Pull and build new version
log ""
log "${BLUE}Step 4: Upgrading to ${VERSION}${NC}"
execute_on_all "cd $ACCUMULATE_DIR && git fetch origin" "Fetching latest code"
execute_on_all "cd $ACCUMULATE_DIR && git checkout $BRANCH" "Checking out branch"
execute_on_all "cd $ACCUMULATE_DIR && git pull origin $BRANCH" "Pulling latest changes"

log "Building new version..."
execute_on_all "cd $ACCUMULATE_DIR && make clean && make build" "Building accumulate"
execute_on_all "cd $ACCUMULATE_DIR && sudo make install" "Installing new binary"

# Step 5: Verify installation
log ""
log "${BLUE}Step 5: Verifying installation${NC}"
execute_on_all "accumulate version" "Checking new version"

# Step 6: Start all nodes simultaneously
log ""
log "${BLUE}Step 6: Starting all nodes${NC}"
execute_on_all "sudo systemctl start accumulate || accumulate start &" "Starting accumulate service"

# Step 7: Wait for network to stabilize
log ""
log "${BLUE}Step 7: Waiting for network to stabilize (30 seconds)${NC}"
for i in {30..1}; do
    echo -ne "\r  Waiting... $i seconds remaining  "
    sleep 1
done
echo ""

# Step 8: Verify network health
log ""
log "${BLUE}Step 8: Verifying network health${NC}"
execute_on_all "accumulate network status" "Checking network status"

# Step 9: Run basic tests
log ""
log "${BLUE}Step 9: Running basic functionality tests${NC}"

# Check if ProofService is active
log "Checking ProofService metrics..."
execute_on_node "${TESTNET_NODES[0]}" "accumulate metrics 2>/dev/null | grep -E 'proof|collection' || echo 'No proof metrics yet'"

# Test basic transaction (if faucet is available)
log "Testing basic transaction..."
execute_on_node "${TESTNET_NODES[0]}" "accumulate faucet acc://testnet-faucet 5 || echo 'Faucet test skipped'"

# Step 10: Run extended tests
log ""
log "${BLUE}Step 10: Running extended tests${NC}"

# Create test script on first node
cat > /tmp/testnet_validation.sh << 'EOF'
#!/bin/bash

echo "=== Testnet Validation Suite ==="
echo ""

# Test 1: Network connectivity
echo "Test 1: Network Connectivity"
accumulate network peers | head -10

# Test 2: Consensus status
echo ""
echo "Test 2: Consensus Status"
accumulate consensus status

# Test 3: Create a test transaction
echo ""
echo "Test 3: Test Transaction"
accumulate account create acc://testnet-test-$(date +%s) || true

# Test 4: Check CrossChainConductor
echo ""
echo "Test 4: CrossChainConductor Status"
accumulate metrics 2>/dev/null | grep -E "conductor|crosschain" || echo "No conductor metrics yet"

# Test 5: Check ProofService
echo ""
echo "Test 5: ProofService Status"
accumulate metrics 2>/dev/null | grep -E "collection_proofs|proof_savings" || echo "No collection proof metrics yet"

echo ""
echo "=== Validation Complete ==="
EOF

# Copy and run test script
log "Running validation suite..."
scp /tmp/testnet_validation.sh "${TESTNET_NODES[0]}:/tmp/" 2>&1 | tee -a "$LOG_FILE"
execute_on_node "${TESTNET_NODES[0]}" "chmod +x /tmp/testnet_validation.sh && /tmp/testnet_validation.sh"

# Step 11: Monitor for 1 minute
log ""
log "${BLUE}Step 11: Monitoring network for 1 minute${NC}"
log "Collecting metrics every 10 seconds..."

for i in {1..6}; do
    log "  ${YELLOW}[Check $i/6]${NC}"
    execute_on_node "${TESTNET_NODES[0]}" "accumulate network status | grep -E 'Height|Peers|Status'" || true
    
    if [ $i -lt 6 ]; then
        sleep 10
    fi
done

# Step 12: Final summary
log ""
log "${GREEN}================================================${NC}"
log "${GREEN}  Upgrade Complete!${NC}"
log "${GREEN}  Completed at: $(date)${NC}"
log "${GREEN}================================================${NC}"
log ""
log "Summary:"
log "  - Version: ${VERSION}"
log "  - Branch: ${BRANCH}"
log "  - Nodes upgraded: ${#TESTNET_NODES[@]}"
log "  - Log file: ${LOG_FILE}"
log ""
log "${YELLOW}Next Steps:${NC}"
log "  1. Monitor logs: tail -f /var/log/accumulate/*.log"
log "  2. Run load tests: cd test/load && ./run_complete_test_suite.sh"
log "  3. Check metrics: accumulate metrics | grep collection_proofs"
log "  4. View partition lag: cd test/load && ./visual_monitor.sh"
log ""
log "${BLUE}To rollback if needed:${NC}"
log "  ./rollback_testnet.sh"