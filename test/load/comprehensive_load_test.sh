#!/bin/bash

# Comprehensive Load Test with Account Creation and Transaction Streaming
# Creates ADIs, token accounts, data accounts, and streams transactions

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
DEVNET_URL="${1:-http://127.0.0.1:27004}"
DURATION="${2:-300}"  # Default 5 minutes
CONCURRENT_ACCOUNTS="${3:-10}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

echo -e "${CYAN}=== Comprehensive Accumulate Load Test ===${NC}"
echo -e "${BLUE}Target:${NC} $DEVNET_URL"
echo -e "${BLUE}Duration:${NC} $DURATION seconds"
echo -e "${BLUE}Concurrent Accounts:${NC} $CONCURRENT_ACCOUNTS"
echo

# Function to log with timestamp
log() { echo -e "${BLUE}[$(date +'%H:%M:%S')]${NC} $1"; }
error() { echo -e "${RED}[$(date +'%H:%M:%S')] ERROR:${NC} $1"; }
success() { echo -e "${GREEN}[$(date +'%H:%M:%S')] SUCCESS:${NC} $1"; }
info() { echo -e "${CYAN}[$(date +'%H:%M:%S')] INFO:${NC} $1"; }

# Check DevNet connectivity
log "Testing DevNet connectivity..."
if ! curl -s -X POST "$DEVNET_URL/v2" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"query","params":{"url":"acc://dn.acme"},"id":1}' | grep -q '"result"'; then
    error "DevNet is not accessible at $DEVNET_URL"
    exit 1
fi
success "DevNet is accessible"

# Get faucet account from devnet
log "Getting faucet account..."
FAUCET_RESPONSE=$(curl -s -X POST "$DEVNET_URL/v2" \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"faucet","params":{"url":"acc://test-faucet-'$(date +%s%N)'/ACME"},"id":1}')

if echo "$FAUCET_RESPONSE" | grep -q '"error"'; then
    error "Failed to get faucet account"
    echo "$FAUCET_RESPONSE"
    exit 1
fi

FAUCET_URL=$(echo "$FAUCET_RESPONSE" | grep -o '"url":"[^"]*"' | cut -d'"' -f4)
info "Faucet account: $FAUCET_URL"

# Statistics tracking
TOTAL_REQUESTS=0
SUCCESSFUL_REQUESTS=0
FAILED_REQUESTS=0
ADIS_CREATED=0
TOKEN_ACCOUNTS_CREATED=0
DATA_ACCOUNTS_CREATED=0
TRANSACTIONS_SENT=0

# Function to create an ADI
create_adi() {
    local adi_name="test-adi-$1-$(date +%s%N)"
    local req_id=$((RANDOM % 100000))
    
    local response=$(curl -s -X POST "$DEVNET_URL/v2" \
      -H "Content-Type: application/json" \
      -d "{\"jsonrpc\":\"2.0\",\"method\":\"execute\",\"params\":{
        \"sponsor\":\"$FAUCET_URL\",
        \"transaction\":{
          \"header\":{\"principal\":\"$FAUCET_URL\"},
          \"body\":{
            \"type\":\"createIdentity\",
            \"url\":\"acc://$adi_name\",
            \"keyHash\":\"$(openssl rand -hex 32)\",
            \"keyBookUrl\":\"acc://$adi_name/book\"
          }
        }
      },\"id\":$req_id}")
    
    ((TOTAL_REQUESTS++))
    
    if echo "$response" | grep -q '"result"'; then
        ((SUCCESSFUL_REQUESTS++))
        ((ADIS_CREATED++))
        echo "acc://$adi_name"
        return 0
    else
        ((FAILED_REQUESTS++))
        return 1
    fi
}

# Function to create token account
create_token_account() {
    local adi_url=$1
    local account_name="tokens-$(date +%s%N)"
    local req_id=$((RANDOM % 100000))
    
    local response=$(curl -s -X POST "$DEVNET_URL/v2" \
      -H "Content-Type: application/json" \
      -d "{\"jsonrpc\":\"2.0\",\"method\":\"execute\",\"params\":{
        \"sponsor\":\"$adi_url\",
        \"transaction\":{
          \"header\":{\"principal\":\"$adi_url\"},
          \"body\":{
            \"type\":\"createTokenAccount\",
            \"url\":\"$adi_url/$account_name\",
            \"tokenUrl\":\"acc://ACME\"
          }
        }
      },\"id\":$req_id}")
    
    ((TOTAL_REQUESTS++))
    
    if echo "$response" | grep -q '"result"'; then
        ((SUCCESSFUL_REQUESTS++))
        ((TOKEN_ACCOUNTS_CREATED++))
        return 0
    else
        ((FAILED_REQUESTS++))
        return 1
    fi
}

# Function to create data account
create_data_account() {
    local adi_url=$1
    local account_name="data-$(date +%s%N)"
    local req_id=$((RANDOM % 100000))
    
    local response=$(curl -s -X POST "$DEVNET_URL/v2" \
      -H "Content-Type: application/json" \
      -d "{\"jsonrpc\":\"2.0\",\"method\":\"execute\",\"params\":{
        \"sponsor\":\"$adi_url\",
        \"transaction\":{
          \"header\":{\"principal\":\"$adi_url\"},
          \"body\":{
            \"type\":\"createDataAccount\",
            \"url\":\"$adi_url/$account_name\"
          }
        }
      },\"id\":$req_id}")
    
    ((TOTAL_REQUESTS++))
    
    if echo "$response" | grep -q '"result"'; then
        ((SUCCESSFUL_REQUESTS++))
        ((DATA_ACCOUNTS_CREATED++))
        return 0
    else
        ((FAILED_REQUESTS++))
        return 1
    fi
}

# Function to send transactions continuously
stream_transactions() {
    local adi_url=$1
    local duration=$2
    local end_time=$(($(date +%s) + duration))
    
    while [ $(date +%s) -lt $end_time ]; do
        # Send a simple query transaction
        local req_id=$((RANDOM % 100000))
        local response=$(curl -s -X POST "$DEVNET_URL/v2" \
          -H "Content-Type: application/json" \
          -d "{\"jsonrpc\":\"2.0\",\"method\":\"query\",\"params\":{\"url\":\"$adi_url\"},\"id\":$req_id}")
        
        ((TOTAL_REQUESTS++))
        
        if echo "$response" | grep -q '"result"'; then
            ((SUCCESSFUL_REQUESTS++))
            ((TRANSACTIONS_SENT++))
        else
            ((FAILED_REQUESTS++))
        fi
        
        # Small delay to prevent overwhelming
        sleep 0.1
    done
}

# Function to show partition status
show_partition_status() {
    log "Partition Status:"
    curl -s -X POST "$DEVNET_URL/v2" \
      -H "Content-Type: application/json" \
      -d '{"jsonrpc":"2.0","method":"describe","params":{},"id":1}' | \
      grep -o '"partitions":\[[^]]*\]' || echo "Unable to get partition status"
}

# Function to display real-time stats
display_stats() {
    local elapsed=$1
    printf "\r${CYAN}[%3ds]${NC} Reqs: %d | Success: %d | Failed: %d | ADIs: %d | Token Accts: %d | Data Accts: %d | TPS: %.2f" \
        $elapsed $TOTAL_REQUESTS $SUCCESSFUL_REQUESTS $FAILED_REQUESTS \
        $ADIS_CREATED $TOKEN_ACCOUNTS_CREATED $DATA_ACCOUNTS_CREATED \
        $(echo "scale=2; $SUCCESSFUL_REQUESTS / $elapsed" | bc -l 2>/dev/null || echo "0")
}

# Main test execution
log "Starting comprehensive load test..."
START_TIME=$(date +%s)

# Phase 1: Create ADIs and accounts
log "Phase 1: Creating ADIs and accounts..."
ADI_URLS=()
for i in $(seq 1 $CONCURRENT_ACCOUNTS); do
    if adi_url=$(create_adi $i); then
        ADI_URLS+=("$adi_url")
        info "Created ADI: $adi_url"
        
        # Create token and data accounts for each ADI
        create_token_account "$adi_url" &
        create_data_account "$adi_url" &
    fi
    
    # Display progress
    display_stats $(($(date +%s) - START_TIME))
done

# Wait for account creation to complete
wait

echo # New line after progress display
log "Phase 2: Streaming transactions..."

# Phase 2: Stream transactions
PIDS=()
for adi_url in "${ADI_URLS[@]}"; do
    stream_transactions "$adi_url" $((DURATION - 30)) &
    PIDS+=($!)
done

# Monitor progress
while true; do
    ELAPSED=$(($(date +%s) - START_TIME))
    display_stats $ELAPSED
    
    if [ $ELAPSED -ge $DURATION ]; then
        break
    fi
    
    sleep 1
done

# Kill streaming processes
for pid in "${PIDS[@]}"; do
    kill $pid 2>/dev/null || true
done

echo # New line after progress display

# Final statistics
END_TIME=$(date +%s)
TOTAL_TIME=$((END_TIME - START_TIME))

echo
echo -e "${CYAN}=== Load Test Results ===${NC}"
echo -e "${BLUE}Duration:${NC} ${TOTAL_TIME}s"
echo -e "${BLUE}Total Requests:${NC} $TOTAL_REQUESTS"
echo -e "${GREEN}Successful:${NC} $SUCCESSFUL_REQUESTS"
echo -e "${RED}Failed:${NC} $FAILED_REQUESTS"
echo -e "${BLUE}Success Rate:${NC} $(echo "scale=2; $SUCCESSFUL_REQUESTS * 100 / $TOTAL_REQUESTS" | bc -l)%"
echo -e "${BLUE}Average TPS:${NC} $(echo "scale=2; $SUCCESSFUL_REQUESTS / $TOTAL_TIME" | bc -l)"
echo
echo -e "${CYAN}=== Accounts Created ===${NC}"
echo -e "${BLUE}ADIs:${NC} $ADIS_CREATED"
echo -e "${BLUE}Token Accounts:${NC} $TOKEN_ACCOUNTS_CREATED"
echo -e "${BLUE}Data Accounts:${NC} $DATA_ACCOUNTS_CREATED"
echo

# Show final partition status
show_partition_status

echo
if [ $FAILED_REQUESTS -eq 0 ]; then
    success "✅ Load test completed successfully with no failures!"
else
    warn "⚠️ Load test completed with $FAILED_REQUESTS failures"
fi

# Instructions for partition control
echo
echo -e "${CYAN}=== Partition Control ===${NC}"
echo "To pause/control partitions during the test, use:"
echo "  ./partition_manager.sh stop BVN0    # Stop BVN0"
echo "  ./partition_manager.sh start BVN0   # Start BVN0"
echo "  ./partition_manager.sh status       # Show partition status"
echo "  ./partition_manager.sh list         # List all partitions"