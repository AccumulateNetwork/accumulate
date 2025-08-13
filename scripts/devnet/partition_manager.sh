#!/bin/bash

# Partition Manager for Accumulate DevNet
# This script allows stopping and restarting individual partitions for testing

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
DEVNET_DIR="$ROOT_DIR/.devnet-test"
LOG_FILE="$SCRIPT_DIR/partition_manager.log"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log() {
    echo -e "${BLUE}[$(date +'%Y-%m-%d %H:%M:%S')]${NC} $1" | tee -a "$LOG_FILE"
}

error() {
    echo -e "${RED}[$(date +'%Y-%m-%d %H:%M:%S')] ERROR:${NC} $1" | tee -a "$LOG_FILE"
    exit 1
}

success() {
    echo -e "${GREEN}[$(date +'%Y-%m-%d %H:%M:%S')] SUCCESS:${NC} $1" | tee -a "$LOG_FILE"
}

warn() {
    echo -e "${YELLOW}[$(date +'%Y-%m-%d %H:%M:%S')] WARNING:${NC} $1" | tee -a "$LOG_FILE"
}

# Function to show usage
usage() {
    echo "Partition Manager for Accumulate DevNet"
    echo ""
    echo "Usage: $0 <command> [partition]"
    echo ""
    echo "Commands:"
    echo "  status                    - Show status of all partitions"
    echo "  stop <partition>          - Stop a specific partition (e.g., BVN0, BVN1, Directory)"
    echo "  start <partition>         - Start a specific partition"
    echo "  restart <partition>       - Restart a specific partition"
    echo "  fail <partition>          - Simulate partition failure"
    echo "  recover <partition>       - Recover a failed partition"
    echo "  test-failure              - Run partition failure test scenario"
    echo "  list                      - List all available partitions"
    echo ""
    echo "Examples:"
    echo "  $0 stop BVN1"
    echo "  $0 start BVN1"
    echo "  $0 restart Directory"
    echo "  $0 test-failure"
    exit 1
}

# Function to get partition process ID
get_partition_pid() {
    local partition=$1
    
    # Try to find process by partition name
    local pid=$(pgrep -f "accumulated.*$partition" | head -1)
    
    if [ -z "$pid" ]; then
        # Try alternative search patterns
        pid=$(ps aux | grep -v grep | grep "accumulated" | grep "$partition" | awk '{print $2}' | head -1)
    fi
    
    echo "$pid"
}

# Function to check if partition is running
is_partition_running() {
    local partition=$1
    local pid=$(get_partition_pid "$partition")
    
    if [ ! -z "$pid" ] && kill -0 "$pid" 2>/dev/null; then
        return 0
    else
        return 1
    fi
}

# Function to get all partition names
get_all_partitions() {
    # Default partitions in devnet
    echo "Directory BVN0 BVN1 BVN2"
}

# Function to show partition status
show_status() {
    log "📊 Partition Status"
    echo "────────────────────────────────────────────────────────"
    
    for partition in $(get_all_partitions); do
        if is_partition_running "$partition"; then
            local pid=$(get_partition_pid "$partition")
            echo -e "  ${GREEN}✅${NC} $partition (PID: $pid) - Running"
        else
            echo -e "  ${RED}❌${NC} $partition - Stopped"
        fi
    done
    
    echo "────────────────────────────────────────────────────────"
    
    # Show network connectivity
    echo ""
    log "🌐 Network Connectivity"
    
    # Check if API is responding
    if curl -s -o /dev/null -w "%{http_code}" http://127.0.0.1:26660/v3 | grep -q "200\|404"; then
        echo -e "  ${GREEN}✅${NC} API endpoint is responding"
    else
        echo -e "  ${RED}❌${NC} API endpoint is not responding"
    fi
}

# Function to stop a partition
stop_partition() {
    local partition=$1
    
    if [ -z "$partition" ]; then
        error "Partition name required"
    fi
    
    log "🛑 Stopping partition: $partition"
    
    if ! is_partition_running "$partition"; then
        warn "Partition $partition is not running"
        return 0
    fi
    
    local pid=$(get_partition_pid "$partition")
    
    if [ ! -z "$pid" ]; then
        # Try graceful shutdown first
        kill -TERM "$pid" 2>/dev/null || true
        
        # Wait for process to terminate
        local count=0
        while kill -0 "$pid" 2>/dev/null && [ $count -lt 10 ]; do
            sleep 1
            count=$((count + 1))
        done
        
        # Force kill if still running
        if kill -0 "$pid" 2>/dev/null; then
            kill -9 "$pid" 2>/dev/null || true
        fi
        
        success "Partition $partition stopped (PID: $pid)"
    else
        warn "Could not find PID for partition $partition"
    fi
    
    # Save state
    echo "$(date) - Stopped $partition" >> "$SCRIPT_DIR/partition_state.log"
}

# Function to start a partition
start_partition() {
    local partition=$1
    
    if [ -z "$partition" ]; then
        error "Partition name required"
    fi
    
    log "🚀 Starting partition: $partition"
    
    if is_partition_running "$partition"; then
        warn "Partition $partition is already running"
        return 0
    fi
    
    cd "$ROOT_DIR"
    
    # Determine partition port based on name
    local port=""
    case "$partition" in
        Directory)
            port="26660"
            ;;
        BVN0)
            port="26661"
            ;;
        BVN1)
            port="26662"
            ;;
        BVN2)
            port="26663"
            ;;
        *)
            error "Unknown partition: $partition"
            ;;
    esac
    
    # Start the partition
    local log_file="$SCRIPT_DIR/${partition}.log"
    local pid_file="$SCRIPT_DIR/${partition}.pid"
    
    # Launch partition in background
    nohup go run ./cmd/accumulated run \
        --work-dir "$DEVNET_DIR/$partition" \
        --network "$partition" \
        --node "http://127.0.0.1:$port" \
        > "$log_file" 2>&1 &
    
    local pid=$!
    echo $pid > "$pid_file"
    
    # Wait a moment for process to start
    sleep 2
    
    if kill -0 "$pid" 2>/dev/null; then
        success "Partition $partition started (PID: $pid, Port: $port)"
        echo "$(date) - Started $partition" >> "$SCRIPT_DIR/partition_state.log"
    else
        error "Failed to start partition $partition"
    fi
}

# Function to restart a partition
restart_partition() {
    local partition=$1
    
    if [ -z "$partition" ]; then
        error "Partition name required"
    fi
    
    log "🔄 Restarting partition: $partition"
    
    stop_partition "$partition"
    sleep 2
    start_partition "$partition"
}

# Function to simulate partition failure
fail_partition() {
    local partition=$1
    
    if [ -z "$partition" ]; then
        error "Partition name required"
    fi
    
    log "💥 Simulating failure for partition: $partition"
    
    # Use iptables to block network traffic (requires sudo)
    if command -v iptables >/dev/null 2>&1; then
        warn "This would require sudo to block network traffic with iptables"
        warn "For testing, we'll just stop the partition instead"
    fi
    
    # For now, just stop the partition abruptly
    local pid=$(get_partition_pid "$partition")
    if [ ! -z "$pid" ]; then
        kill -9 "$pid" 2>/dev/null || true
        success "Partition $partition failed (killed abruptly)"
        echo "$(date) - Failed $partition" >> "$SCRIPT_DIR/partition_state.log"
    else
        warn "Partition $partition is not running"
    fi
}

# Function to recover a partition
recover_partition() {
    local partition=$1
    
    if [ -z "$partition" ]; then
        error "Partition name required"
    fi
    
    log "🔧 Recovering partition: $partition"
    
    # Remove any network blocks (would require sudo)
    if command -v iptables >/dev/null 2>&1; then
        warn "Would remove network blocks if they were set"
    fi
    
    # Start the partition if not running
    if ! is_partition_running "$partition"; then
        start_partition "$partition"
    else
        warn "Partition $partition is already running"
    fi
    
    echo "$(date) - Recovered $partition" >> "$SCRIPT_DIR/partition_state.log"
}

# Function to run partition failure test
test_partition_failure() {
    log "🧪 Running Partition Failure Test Scenario"
    echo "════════════════════════════════════════════════════════"
    
    # Show initial status
    echo ""
    log "Initial State:"
    show_status
    
    # Test 1: Stop and restart BVN1
    echo ""
    log "Test 1: Stop and restart BVN1"
    echo "────────────────────────────────────────────────────────"
    
    stop_partition "BVN1"
    sleep 3
    
    log "Partition stopped, checking status..."
    if is_partition_running "BVN1"; then
        error "BVN1 should be stopped but is still running"
    fi
    success "BVN1 successfully stopped"
    
    log "Waiting 5 seconds before restart..."
    sleep 5
    
    start_partition "BVN1"
    sleep 3
    
    if is_partition_running "BVN1"; then
        success "BVN1 successfully restarted"
    else
        error "BVN1 failed to restart"
    fi
    
    # Test 2: Simulate cascading failures
    echo ""
    log "Test 2: Cascading Failures"
    echo "────────────────────────────────────────────────────────"
    
    log "Simulating cascading failures..."
    fail_partition "BVN0"
    sleep 2
    fail_partition "BVN2"
    sleep 2
    
    log "Multiple partitions failed, checking status..."
    show_status
    
    # Test 3: Recovery
    echo ""
    log "Test 3: Recovery"
    echo "────────────────────────────────────────────────────────"
    
    log "Recovering failed partitions..."
    recover_partition "BVN0"
    sleep 2
    recover_partition "BVN2"
    sleep 2
    
    log "Recovery complete, checking final status..."
    show_status
    
    echo ""
    echo "════════════════════════════════════════════════════════"
    success "Partition Failure Test Complete!"
}

# Function to list all partitions
list_partitions() {
    log "📋 Available Partitions:"
    for partition in $(get_all_partitions); do
        echo "  - $partition"
    done
}

# Main execution
case "$1" in
    status)
        show_status
        ;;
    stop)
        stop_partition "$2"
        ;;
    start)
        start_partition "$2"
        ;;
    restart)
        restart_partition "$2"
        ;;
    fail)
        fail_partition "$2"
        ;;
    recover)
        recover_partition "$2"
        ;;
    test-failure)
        test_partition_failure
        ;;
    list)
        list_partitions
        ;;
    *)
        usage
        ;;
esac