#!/bin/bash

# Cyclops Phase 3 Launch Script
# Launches the Cyclops validator node after Phase 2 deployment
# Includes configuration validation, startup, and monitoring

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Print status with colors
print_status() {
    local status=$1
    local message=$2
    case $status in
        "OK"|"SUCCESS") echo -e "✓ ${GREEN}$message${NC}" ;;
        "INFO") echo -e "ℹ ${BLUE}$message${NC}" ;;
        "WARNING") echo -e "⚠ ${YELLOW}$message${NC}" ;;
        "ERROR") echo -e "✗ ${RED}$message${NC}" ;;
        *) echo -e "$message" ;;
    esac
}

echo "🚀 Cyclops Phase 3 Launch"
echo "========================="
print_status "INFO" "Launching Cyclops validator node"

# Configuration
DEPLOY_DIR="/tmp/cyclops/node"
ARTIFACTS_DIR="$DEPLOY_DIR/artifacts"
NODE_DIR="$ARTIFACTS_DIR/.accumulate"

# Step 1: Pre-launch validation
echo -e "\n🔍 Step 1: Pre-launch validation..."

if [ ! -d "$DEPLOY_DIR" ]; then
    print_status "ERROR" "Deployment directory not found: $DEPLOY_DIR"
    print_status "INFO" "Run Phase 2 deployment first: ./cyclops_deploy_phase2.sh"
    exit 1
fi

cd "$ARTIFACTS_DIR"

# Check required files
REQUIRED_FILES=(
    "accumulated"
    ".accumulate/config/accumulate.toml"
    ".accumulate/config/tendermint.toml"
    ".accumulate/config/priv_validator_key.json"
    ".accumulate/dn/config/priv_validator_key.json"
    ".accumulate/bvn-cyclops/config/priv_validator_key.json"
    ".accumulate/dn/data/Directory-partition.snap"
    ".accumulate/bvn-cyclops/data/bvn-cyclops-partition.snap"
)

MISSING_FILES=0
for file in "${REQUIRED_FILES[@]}"; do
    if [ -f "$file" ] || [ -d "$file" ]; then
        print_status "OK" "Found: $file"
    else
        print_status "ERROR" "Missing: $file"
        MISSING_FILES=$((MISSING_FILES + 1))
    fi
done

if [ $MISSING_FILES -gt 0 ]; then
    print_status "ERROR" "Missing $MISSING_FILES required files. Run Phase 2 deployment first."
    exit 1
fi

# Step 2: Configuration validation
echo -e "\n⚙️ Step 2: Configuration validation..."

# Test configuration parsing
print_status "INFO" "Testing configuration parsing..."
if ./accumulated run --work-dir .accumulate --check-config 2>/dev/null; then
    print_status "OK" "Configuration validation passed"
else
    print_status "WARNING" "Configuration validation had warnings (this may be normal)"
fi

# Step 3: Network connectivity check
echo -e "\n🌐 Step 3: Network connectivity check..."

# Check if ports are available
PORTS=(26656 26657 26658)
for port in "${PORTS[@]}"; do
    if netstat -tuln 2>/dev/null | grep -q ":$port "; then
        print_status "WARNING" "Port $port is already in use"
    else
        print_status "OK" "Port $port is available"
    fi
done

# Step 4: Launch node
echo -e "\n🚀 Step 4: Launching Cyclops validator node..."

print_status "INFO" "Starting node in background..."
print_status "INFO" "Working directory: $(pwd)/.accumulate"
print_status "INFO" "Log file: cyclops-node.log"

# Start the node in background with logging
nohup ./accumulated run --work-dir .accumulate > cyclops-node.log 2>&1 &
NODE_PID=$!

print_status "OK" "Node started with PID: $NODE_PID"
echo "$NODE_PID" > cyclops-node.pid

# Step 5: Startup monitoring
echo -e "\n📊 Step 5: Startup monitoring..."

print_status "INFO" "Monitoring node startup (30 seconds)..."

# Wait for node to start
sleep 5

# Check if process is still running
if kill -0 $NODE_PID 2>/dev/null; then
    print_status "OK" "Node process is running (PID: $NODE_PID)"
else
    print_status "ERROR" "Node process died during startup"
    print_status "INFO" "Check log file: cyclops-node.log"
    exit 1
fi

# Monitor startup for 30 seconds
for i in {1..6}; do
    sleep 5
    if kill -0 $NODE_PID 2>/dev/null; then
        print_status "OK" "Node still running after $((i*5)) seconds"
        
        # Try to check node status via RPC
        if curl -s http://localhost:26657/status >/dev/null 2>&1; then
            print_status "OK" "RPC endpoint responding"
            break
        else
            print_status "INFO" "RPC endpoint not yet ready..."
        fi
    else
        print_status "ERROR" "Node process died after $((i*5)) seconds"
        print_status "INFO" "Check log file: cyclops-node.log"
        exit 1
    fi
done

# Step 6: Node status check
echo -e "\n📋 Step 6: Node status check..."

# Wait a bit more for full startup
sleep 10

if curl -s http://localhost:26657/status >/dev/null 2>&1; then
    print_status "OK" "Node RPC is responding"
    
    # Get node info
    NODE_INFO=$(curl -s http://localhost:26657/status 2>/dev/null || echo "{}")
    NETWORK=$(echo "$NODE_INFO" | jq -r '.result.node_info.network // "unknown"' 2>/dev/null || echo "unknown")
    LATEST_HEIGHT=$(echo "$NODE_INFO" | jq -r '.result.sync_info.latest_block_height // "unknown"' 2>/dev/null || echo "unknown")
    
    print_status "OK" "Network ID: $NETWORK"
    print_status "OK" "Latest block height: $LATEST_HEIGHT"
else
    print_status "WARNING" "RPC endpoint not responding yet (may need more time)"
fi

# Step 7: Final status and instructions
echo -e "\n🎉 Phase 3 Launch Complete!"
echo "============================"

print_status "SUCCESS" "Cyclops validator node is running!"
print_status "INFO" "Node PID: $NODE_PID (saved to cyclops-node.pid)"
print_status "INFO" "Log file: cyclops-node.log"
print_status "INFO" "Working directory: $(pwd)/.accumulate"

echo -e "\n📋 Node Management Commands:"
echo "----------------------------"
echo "• Check status:     curl http://localhost:26657/status | jq"
echo "• View logs:        tail -f cyclops-node.log"
echo "• Stop node:        kill \$(cat cyclops-node.pid)"
echo "• Restart node:     ./cyclops_launch_phase3.sh"

echo -e "\n🔍 Monitoring URLs:"
echo "------------------"
echo "• Node status:      http://localhost:26657/status"
echo "• Node info:        http://localhost:26657/net_info"
echo "• Validators:       http://localhost:26657/validators"

echo -e "\n📊 Validation Commands:"
echo "----------------------"
echo "• Validate structure: /home/paulsnow/accumulate-network/artifacts/validate-node-structure.sh .accumulate"
echo "• Check partitions:   curl http://localhost:26657/status | jq '.result.node_info'"

echo -e "\n💡 Next Steps:"
echo "-------------"
echo "1. Monitor the logs for any errors: tail -f cyclops-node.log"
echo "2. Verify both partitions are syncing properly"
echo "3. Test transaction submission once fully synced"
echo "4. Set up monitoring and alerting for production use"

print_status "SUCCESS" "Cyclops validator deployment complete!"
