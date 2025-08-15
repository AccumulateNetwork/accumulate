#!/bin/bash

# Interactive CCC Pause Test Script
# This script demonstrates pausing and resuming partitions during load testing

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
DN_PORT=27004
BVN0_PORT=27010  
BVN1_PORT=27011
BVN2_PORT=27012

echo -e "${CYAN}===================================${NC}"
echo -e "${CYAN}  CCC Gap Recovery Test Dashboard ${NC}"
echo -e "${CYAN}===================================${NC}"
echo ""
echo -e "${YELLOW}This test demonstrates partition isolation and gap recovery${NC}"
echo -e "${YELLOW}Prerequisites: DevNet running with testnet build tag${NC}"
echo ""

# Function to check partition status
check_status() {
    local port=$1
    local name=$2
    
    # Try to get CCC status (will fail if not testnet build)
    response=$(curl -s "http://localhost:${port}/debug/ccc/status" 2>/dev/null || echo "")
    
    if [[ "$response" == *"paused"* ]]; then
        echo -e "${RED}⏸️  $name: PAUSED${NC}"
    elif [[ -n "$response" ]]; then
        echo -e "${GREEN}▶️  $name: RUNNING${NC}"
    else
        echo -e "${YELLOW}❓ $name: No CCC debug endpoint (not testnet build?)${NC}"
    fi
}

# Function to pause partition
pause_partition() {
    local port=$1
    local name=$2
    
    echo -e "${BLUE}Pausing $name...${NC}"
    response=$(curl -s -X POST "http://localhost:${port}/debug/ccc/pause" 2>/dev/null || echo "failed")
    
    if [[ "$response" != "failed" ]]; then
        echo -e "${GREEN}✅ $name paused successfully${NC}"
        echo -e "${YELLOW}   Messages will be dropped, creating gaps${NC}"
    else
        echo -e "${RED}❌ Failed to pause $name (endpoint not available)${NC}"
    fi
}

# Function to resume partition
resume_partition() {
    local port=$1
    local name=$2
    
    echo -e "${BLUE}Resuming $name...${NC}"
    response=$(curl -s -X POST "http://localhost:${port}/debug/ccc/resume" 2>/dev/null || echo "failed")
    
    if [[ "$response" != "failed" ]]; then
        echo -e "${GREEN}✅ $name resumed successfully${NC}"
        echo -e "${YELLOW}   Gap recovery will begin automatically${NC}"
    else
        echo -e "${RED}❌ Failed to resume $name${NC}"
    fi
}

# Function to show menu
show_menu() {
    echo ""
    echo -e "${CYAN}=== Partition Control Menu ===${NC}"
    echo "1) Show partition status"
    echo "2) Pause DN"
    echo "3) Pause BVN0"
    echo "4) Pause BVN1"
    echo "5) Pause BVN2"
    echo "6) Resume DN"
    echo "7) Resume BVN0"
    echo "8) Resume BVN1"
    echo "9) Resume BVN2"
    echo "a) Pause ALL partitions"
    echo "r) Resume ALL partitions"
    echo "t) Run test transactions"
    echo "l) View devnet logs (last 20 lines)"
    echo "q) Quit"
    echo ""
    echo -n "Select option: "
}

# Function to run test transactions
run_test_transactions() {
    echo -e "${BLUE}Running test transactions...${NC}"
    
    # Simple test using devnet endpoint
    for i in {1..10}; do
        curl -s -X POST "http://localhost:${DN_PORT}/v2" \
            -H "Content-Type: application/json" \
            -d '{"jsonrpc":"2.0","id":1,"method":"query","params":{"url":"acc://dn/version"}}' \
            > /dev/null 2>&1 && echo -n "." || echo -n "x"
    done
    echo ""
    echo -e "${GREEN}✅ Test transactions sent${NC}"
}

# Main loop
while true; do
    show_menu
    read -r choice
    
    case $choice in
        1)
            echo ""
            echo -e "${CYAN}=== Partition Status ===${NC}"
            check_status $DN_PORT "DN"
            check_status $BVN0_PORT "BVN0"
            check_status $BVN1_PORT "BVN1"
            check_status $BVN2_PORT "BVN2"
            ;;
        2)
            pause_partition $DN_PORT "DN"
            ;;
        3)
            pause_partition $BVN0_PORT "BVN0"
            ;;
        4)
            pause_partition $BVN1_PORT "BVN1"
            ;;
        5)
            pause_partition $BVN2_PORT "BVN2"
            ;;
        6)
            resume_partition $DN_PORT "DN"
            ;;
        7)
            resume_partition $BVN0_PORT "BVN0"
            ;;
        8)
            resume_partition $BVN1_PORT "BVN1"
            ;;
        9)
            resume_partition $BVN2_PORT "BVN2"
            ;;
        a)
            echo -e "${YELLOW}Pausing all partitions...${NC}"
            pause_partition $DN_PORT "DN"
            pause_partition $BVN0_PORT "BVN0"
            pause_partition $BVN1_PORT "BVN1"
            pause_partition $BVN2_PORT "BVN2"
            echo -e "${RED}⚠️  All partitions paused - network isolated!${NC}"
            ;;
        r)
            echo -e "${YELLOW}Resuming all partitions...${NC}"
            resume_partition $DN_PORT "DN"
            resume_partition $BVN0_PORT "BVN0"
            resume_partition $BVN1_PORT "BVN1"
            resume_partition $BVN2_PORT "BVN2"
            echo -e "${GREEN}✅ All partitions resumed - gap recovery in progress${NC}"
            ;;
        t)
            run_test_transactions
            ;;
        l)
            echo -e "${CYAN}=== Recent DevNet Logs ===${NC}"
            if [ -f "devnet.log" ]; then
                tail -20 devnet.log
            else
                echo -e "${YELLOW}No devnet.log found in current directory${NC}"
            fi
            ;;
        q)
            echo -e "${GREEN}Goodbye!${NC}"
            exit 0
            ;;
        *)
            echo -e "${RED}Invalid option${NC}"
            ;;
    esac
done