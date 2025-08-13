#!/bin/bash

# Interactive Load Test with Account Management
# Allows pausing partitions and monitoring protocol state

set -e

DEVNET_URL="${1:-http://127.0.0.1:27004/v2}"
DURATION="${2:-300}"  # Default 5 minutes
ACCOUNTS="${3:-5}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m'

echo -e "${CYAN}=== Interactive Accumulate Load Test ===${NC}"
echo -e "${BLUE}Target:${NC} $DEVNET_URL"
echo -e "${BLUE}Duration:${NC} $DURATION seconds"
echo -e "${BLUE}Accounts:${NC} $ACCOUNTS"
echo
echo -e "${YELLOW}Controls:${NC}"
echo "  Press 'p' to pause/unpause test"
echo "  Press 's' to show partition status"
echo "  Press 'q' to quit"
echo

# Global state
PAUSED=false
RUNNING=true
START_TIME=$(date +%s)
TOTAL_REQUESTS=0
SUCCESSFUL_REQUESTS=0
FAILED_REQUESTS=0

# Log function
log() { echo -e "${BLUE}[$(date +'%H:%M:%S')]${NC} $1"; }
error() { echo -e "${RED}[$(date +'%H:%M:%S')] ERROR:${NC} $1"; }
success() { echo -e "${GREEN}[$(date +'%H:%M:%S')] SUCCESS:${NC} $1"; }
info() { echo -e "${CYAN}[$(date +'%H:%M:%S')] INFO:${NC} $1"; }

# Test connectivity
log "Testing DevNet connectivity..."
if curl -s -X POST "$DEVNET_URL" \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"query","params":{"url":"acc://dn.acme"},"id":1}' | grep -q '"result"'; then
    success "DevNet is accessible"
else
    error "DevNet is not accessible at $DEVNET_URL"
    exit 1
fi

# Show partition status
show_partitions() {
    echo
    info "Current Partition Status:"
    echo -e "${CYAN}──────────────────────────────────────${NC}"
    
    # Query network info
    response=$(curl -s -X POST "$DEVNET_URL" \
        -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","method":"network","params":{},"id":1}')
    
    if echo "$response" | grep -q '"result"'; then
        # Extract partition info
        echo "$response" | grep -oP '"id":"[^"]*"' | cut -d'"' -f4 | while read partition; do
            echo "  • $partition"
        done
    else
        echo "  Unable to query partition status"
    fi
    
    echo -e "${CYAN}──────────────────────────────────────${NC}"
    echo
}

# Function to send test transactions
send_transaction() {
    local account_id=$1
    
    while $RUNNING; do
        if $PAUSED; then
            sleep 0.5
            continue
        fi
        
        # Send a query request (simulating transaction)
        local request_id=$((RANDOM % 100000))
        response=$(curl -s -X POST "$DEVNET_URL" \
            -H "Content-Type: application/json" \
            -d "{\"jsonrpc\":\"2.0\",\"method\":\"query\",\"params\":{\"url\":\"acc://dn.acme\"},\"id\":$request_id}" 2>/dev/null)
        
        ((TOTAL_REQUESTS++))
        
        if echo "$response" | grep -q '"result"'; then
            ((SUCCESSFUL_REQUESTS++))
        else
            ((FAILED_REQUESTS++))
        fi
        
        # Random delay between 0.1 and 0.5 seconds
        sleep $(echo "scale=2; 0.1 + $RANDOM / 65536" | bc -l)
    done
}

# Display real-time statistics
display_stats() {
    local elapsed=$(($(date +%s) - START_TIME))
    local tps=0
    
    if [ $elapsed -gt 0 ]; then
        tps=$(echo "scale=2; $SUCCESSFUL_REQUESTS / $elapsed" | bc -l 2>/dev/null || echo "0")
    fi
    
    printf "\r${CYAN}[%3ds]${NC} " $elapsed
    printf "Requests: ${BLUE}%d${NC} | " $TOTAL_REQUESTS
    printf "Success: ${GREEN}%d${NC} | " $SUCCESSFUL_REQUESTS
    printf "Failed: ${RED}%d${NC} | " $FAILED_REQUESTS
    printf "TPS: ${MAGENTA}%.2f${NC} | " $tps
    
    if $PAUSED; then
        printf "${YELLOW}[PAUSED]${NC}"
    else
        printf "${GREEN}[RUNNING]${NC}"
    fi
    
    printf "     " # Clear any remaining characters
}

# Keyboard input handler
handle_input() {
    while $RUNNING; do
        read -n 1 -s -t 0.1 key 2>/dev/null
        
        case "$key" in
            p|P)
                if $PAUSED; then
                    PAUSED=false
                    echo
                    info "Test RESUMED"
                else
                    PAUSED=true
                    echo
                    info "Test PAUSED - Press 'p' to resume"
                fi
                ;;
            s|S)
                show_partitions
                ;;
            q|Q)
                echo
                info "Stopping test..."
                RUNNING=false
                ;;
        esac
    done
}

# Start transaction workers
log "Starting $ACCOUNTS transaction workers..."
PIDS=()
for ((i=1; i<=ACCOUNTS; i++)); do
    send_transaction $i &
    PIDS+=($!)
done

# Start keyboard handler
handle_input &
INPUT_PID=$!

# Main monitoring loop
END_TIME=$((START_TIME + DURATION))
while $RUNNING && [ $(date +%s) -lt $END_TIME ]; do
    display_stats
    sleep 0.5
done

# Cleanup
RUNNING=false
echo
log "Stopping workers..."

# Kill all background processes
for pid in "${PIDS[@]}"; do
    kill $pid 2>/dev/null || true
done
kill $INPUT_PID 2>/dev/null || true

# Wait for processes to finish
sleep 1

# Final statistics
echo
echo -e "${CYAN}=== Load Test Results ===${NC}"
TOTAL_TIME=$(($(date +%s) - START_TIME))
echo -e "${BLUE}Duration:${NC} ${TOTAL_TIME}s"
echo -e "${BLUE}Total Requests:${NC} $TOTAL_REQUESTS"
echo -e "${GREEN}Successful:${NC} $SUCCESSFUL_REQUESTS"
echo -e "${RED}Failed:${NC} $FAILED_REQUESTS"

if [ $TOTAL_REQUESTS -gt 0 ]; then
    SUCCESS_RATE=$(echo "scale=2; $SUCCESSFUL_REQUESTS * 100 / $TOTAL_REQUESTS" | bc -l)
    AVG_TPS=$(echo "scale=2; $SUCCESSFUL_REQUESTS / $TOTAL_TIME" | bc -l)
    echo -e "${BLUE}Success Rate:${NC} ${SUCCESS_RATE}%"
    echo -e "${BLUE}Average TPS:${NC} $AVG_TPS"
fi

echo
show_partitions

if [ $FAILED_REQUESTS -eq 0 ]; then
    success "✅ Load test completed with no failures!"
else
    info "⚠️ Load test completed with $FAILED_REQUESTS failures"
fi

echo
echo -e "${CYAN}=== Partition Management ===${NC}"
echo "To manage partitions, run these commands in another terminal:"
echo "  # Stop a partition:"
echo "  docker stop accumulate-bvn0-0  # Stop BVN0"
echo "  "
echo "  # Start a partition:"
echo "  docker start accumulate-bvn0-0  # Start BVN0"
echo "  "
echo "  # View partition logs:"
echo "  docker logs -f accumulate-bvn0-0"
echo