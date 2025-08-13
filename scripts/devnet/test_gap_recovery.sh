#!/bin/bash

# Test Gap Recovery by pausing BVN CCCs
# Requires: go build -tags testnet

set -e

# Configuration
BVN0_PORT=27010
BVN1_PORT=27011
BVN2_PORT=27012

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log() {
    echo -e "${BLUE}[$(date +'%H:%M:%S')]${NC} $1"
}

success() {
    echo -e "${GREEN}✓${NC} $1"
}

error() {
    echo -e "${RED}✗${NC} $1"
    exit 1
}

# Check if devnet is running
check_devnet() {
    log "Checking devnet status..."
    if ! curl -s http://localhost:27004/status > /dev/null 2>&1; then
        error "Devnet not running. Start with: ./devnet_manager.sh"
    fi
    success "Devnet is running"
}

# Pause a BVN's CCC
pause_bvn() {
    local port=$1
    local name=$2
    
    log "Pausing $name CCC..."
    response=$(curl -s -X POST "http://localhost:$port/debug/ccc/pause" 2>&1 || true)
    
    if echo "$response" | grep -q "paused"; then
        success "$name CCC paused"
    else
        error "Failed to pause $name: $response"
    fi
}

# Resume a BVN's CCC
resume_bvn() {
    local port=$1
    local name=$2
    
    log "Resuming $name CCC..."
    response=$(curl -s -X POST "http://localhost:$port/debug/ccc/resume" 2>&1 || true)
    
    if echo "$response" | grep -q "resumed"; then
        success "$name CCC resumed"
    else
        error "Failed to resume $name: $response"
    fi
}

# Check CCC status
check_status() {
    local port=$1
    local name=$2
    
    response=$(curl -s "http://localhost:$port/debug/ccc/status" 2>&1 || true)
    echo "$name status: $response"
}

# Main test scenarios
main() {
    clear
    echo "======================================"
    echo "   GAP RECOVERY TEST (REAL CCC)"
    echo "======================================"
    echo ""
    echo "This test requires Accumulate built with:"
    echo "  go build -tags testnet"
    echo ""
    
    check_devnet
    
    echo ""
    echo "TEST 1: Single BVN Isolation"
    echo "-----------------------------"
    
    # Pause BVN1 for 10 seconds
    pause_bvn $BVN1_PORT "BVN1"
    
    log "BVN1 is now isolated from the network"
    log "Messages will accumulate, creating gaps..."
    
    # Check status
    sleep 2
    check_status $BVN0_PORT "BVN0"
    check_status $BVN1_PORT "BVN1"
    check_status $BVN2_PORT "BVN2"
    
    # Wait
    log "Waiting 10 seconds..."
    sleep 10
    
    # Resume BVN1
    resume_bvn $BVN1_PORT "BVN1"
    
    log "BVN1 reconnected - gap recovery should begin"
    log "Watch for gap request messages in logs"
    
    sleep 5
    
    echo ""
    echo "TEST 2: Multiple BVN Isolation"
    echo "-------------------------------"
    
    # Pause BVN0 and BVN2
    pause_bvn $BVN0_PORT "BVN0"
    pause_bvn $BVN2_PORT "BVN2"
    
    log "BVN0 and BVN2 isolated - only BVN1 active"
    
    sleep 10
    
    # Resume one at a time
    resume_bvn $BVN0_PORT "BVN0"
    log "BVN0 resumed, waiting 5s..."
    sleep 5
    
    resume_bvn $BVN2_PORT "BVN2"
    log "BVN2 resumed"
    
    sleep 5
    
    echo ""
    echo "TEST 3: Cascading Isolation"
    echo "----------------------------"
    
    # Pause in sequence
    pause_bvn $BVN0_PORT "BVN0"
    sleep 3
    
    pause_bvn $BVN1_PORT "BVN1"
    sleep 3
    
    pause_bvn $BVN2_PORT "BVN2"
    log "All BVNs paused - network frozen"
    
    sleep 5
    
    # Resume all
    log "Resuming all BVNs..."
    resume_bvn $BVN0_PORT "BVN0"
    resume_bvn $BVN1_PORT "BVN1"
    resume_bvn $BVN2_PORT "BVN2"
    
    log "All BVNs resumed - massive gap recovery expected"
    
    sleep 10
    
    echo ""
    echo "======================================"
    echo "         TEST COMPLETE"
    echo "======================================"
    echo ""
    echo "Check the logs for:"
    echo "  - 'Gap detected' messages"
    echo "  - 'Gap recovered' messages"
    echo "  - Sequence number jumps"
    echo ""
    echo "Logs location:"
    echo "  tail -f .devnet-test/bvn*/node*/accumulate.log | grep -E 'Gap|sequence|CCC'"
}

# Run if executed directly
if [ "${BASH_SOURCE[0]}" = "${0}" ]; then
    main "$@"
fi