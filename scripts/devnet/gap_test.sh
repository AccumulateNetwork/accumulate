#!/bin/bash

# Gap Recovery Test Script
# Tests the CrossChain Conductor's gap recovery mechanism by selectively stalling BVNs

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
LOG_FILE="$SCRIPT_DIR/gap_test.log"

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

# Check if devnet is running
check_devnet() {
    log "Checking if devnet is running..."
    
    if ! curl -s http://localhost:27004/status > /dev/null 2>&1; then
        error "Devnet is not running! Please start it first with ./devnet_manager.sh"
    fi
    
    success "Devnet is running"
}

# Build the gap test monitor
build_monitor() {
    log "Building gap test monitor..."
    
    cd "$SCRIPT_DIR"
    if ! go build -o gap_test_monitor gap_test_monitor.go; then
        error "Failed to build gap test monitor"
    fi
    
    success "Gap test monitor built successfully"
}

# Start the gap test monitor
start_monitor() {
    log "Starting gap test monitor..."
    
    # Kill any existing monitor
    if pgrep -f "gap_test_monitor" > /dev/null; then
        log "Killing existing gap test monitor..."
        pkill -f "gap_test_monitor" || true
        sleep 2
    fi
    
    # Start new monitor
    cd "$SCRIPT_DIR"
    ./gap_test_monitor > gap_monitor.log 2>&1 &
    local PID=$!
    
    sleep 3
    
    # Check if it's still running
    if ! kill -0 $PID 2>/dev/null; then
        error "Gap test monitor failed to start. Check gap_monitor.log for details"
    fi
    
    success "Gap test monitor started (PID: $PID)"
    log "Web dashboard available at http://localhost:8081"
}

# Display test scenarios
show_scenarios() {
    echo ""
    echo "======================================"
    echo "  GAP RECOVERY TEST SCENARIOS"
    echo "======================================"
    echo ""
    echo "The web dashboard at http://localhost:8081 provides:"
    echo ""
    echo "1. BVN CONTROLS:"
    echo "   - Stall individual BVNs for 10s or 30s"
    echo "   - Manually unstall BVNs"
    echo "   - Visual status indicators"
    echo ""
    echo "2. MONITORING:"
    echo "   - Real-time source/destination heights per BVN pair"
    echo "   - Gap detection with visual alerts"
    echo "   - Recovery time tracking"
    echo "   - TPS and performance metrics"
    echo ""
    echo "3. TEST SCENARIOS TO TRY:"
    echo ""
    echo "   ${GREEN}Scenario 1: Single BVN Stall${NC}"
    echo "   - Stall BVN1 for 10 seconds"
    echo "   - Watch gaps form between BVN0->BVN1 and BVN2->BVN1"
    echo "   - Observe automatic recovery when BVN1 resumes"
    echo ""
    echo "   ${GREEN}Scenario 2: Multiple BVN Stalls${NC}"
    echo "   - Stall BVN0 and BVN2 simultaneously"
    echo "   - Monitor how BVN1 continues processing"
    echo "   - Watch recovery when stalled BVNs resume"
    echo ""
    echo "   ${GREEN}Scenario 3: Cascading Stalls${NC}"
    echo "   - Stall BVN0 for 30s"
    echo "   - After 10s, stall BVN1 for 20s"
    echo "   - Observe cascading gap formation and recovery"
    echo ""
    echo "   ${GREEN}Scenario 4: Rapid Stall/Unstall${NC}"
    echo "   - Rapidly stall and unstall different BVNs"
    echo "   - Test gap detection sensitivity"
    echo "   - Verify no messages are lost"
    echo ""
    echo "======================================"
    echo ""
}

# Monitor logs for interesting events
monitor_logs() {
    log "Monitoring for gap recovery events..."
    echo ""
    echo "Live gap recovery events:"
    echo "--------------------------"
    
    tail -f gap_monitor.log | grep --line-buffered -E "Gap detected|Gap recovered|Stalling|Unstalled" &
    local TAIL_PID=$!
    
    # Save PID for cleanup
    echo $TAIL_PID > .tail_pid
}

# Cleanup function
cleanup() {
    log "Cleaning up..."
    
    # Kill tail process if exists
    if [ -f .tail_pid ]; then
        kill $(cat .tail_pid) 2>/dev/null || true
        rm .tail_pid
    fi
    
    # Kill gap monitor
    if pgrep -f "gap_test_monitor" > /dev/null; then
        log "Stopping gap test monitor..."
        pkill -f "gap_test_monitor" || true
    fi
    
    success "Cleanup complete"
}

# Set up trap for cleanup
trap cleanup EXIT INT TERM

# Main execution
main() {
    clear
    echo "🔬 ACCUMULATE GAP RECOVERY TEST"
    echo "================================"
    echo ""
    
    check_devnet
    build_monitor
    start_monitor
    show_scenarios
    
    echo "Press Ctrl+C to stop the test..."
    echo ""
    
    monitor_logs
    
    # Wait for user interrupt
    wait
}

# Run main function
main "$@"