#!/bin/bash

# Enhanced DevNet Load Test Script with Gap Recovery Testing
# Provides multiple testing modes including standard load test and gap recovery test

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default parameters
DEVNET_URL="${DEVNET_URL:-http://127.0.0.1:27004/v2}"
NUM_REQUESTS="${NUM_REQUESTS:-50}"
CONCURRENT_WORKERS="${CONCURRENT_WORKERS:-5}"
TEST_MODE="${1:-standard}"

# Functions
log() {
    echo -e "${BLUE}[$(date +'%H:%M:%S')]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

show_usage() {
    cat << EOF
Enhanced DevNet Load Test Script

Usage: $0 [MODE] [OPTIONS]

MODES:
    standard    - Run standard load test (default)
    gap         - Run gap recovery test with BVN stalling
    stress      - Run stress test with monitoring
    help        - Show this help message

STANDARD MODE OPTIONS:
    Environment variables:
    DEVNET_URL="http://127.0.0.1:27004/v2"  - DevNet URL
    NUM_REQUESTS=50                          - Number of requests
    CONCURRENT_WORKERS=5                     - Concurrent workers

GAP MODE OPTIONS:
    Launches interactive gap recovery test with web dashboard

EXAMPLES:
    # Standard load test
    $0 standard
    
    # Gap recovery test
    $0 gap
    
    # Custom standard test
    NUM_REQUESTS=100 CONCURRENT_WORKERS=10 $0 standard
    
    # Stress test with monitoring
    $0 stress

EOF
}

# Test DevNet connectivity
test_connectivity() {
    log "Testing DevNet connectivity..."
    
    response=$(curl -s -X POST "$DEVNET_URL" \
        -H "Content-Type: application/json" \
        -d '{"jsonrpc":"2.0","method":"query","params":{"url":"acc://dn.acme"},"id":1}' 2>/dev/null || true)
    
    if echo "$response" | grep -q '"result"'; then
        success "DevNet is accessible at $DEVNET_URL"
        return 0
    else
        error "DevNet is not accessible at $DEVNET_URL"
        return 1
    fi
}

# Run standard load test
run_standard_test() {
    log "Starting standard load test..."
    echo ""
    echo "Configuration:"
    echo "  Target: $DEVNET_URL"
    echo "  Requests: $NUM_REQUESTS"
    echo "  Workers: $CONCURRENT_WORKERS"
    echo ""
    
    test_connectivity || exit 1
    
    # Run the original load test
    cd "$SCRIPT_DIR"
    if [ -f "./devnet_load_test.sh" ]; then
        bash ./devnet_load_test.sh "$DEVNET_URL" "$NUM_REQUESTS" "$CONCURRENT_WORKERS"
    else
        error "Standard load test script not found"
    fi
}

# Run gap recovery test
run_gap_test() {
    log "Starting gap recovery test..."
    echo ""
    echo "This test will:"
    echo "  1. Launch a web dashboard on http://localhost:8081"
    echo "  2. Allow you to stall/unstall BVNs"
    echo "  3. Monitor gap detection and recovery"
    echo "  4. Track crosschain message flow"
    echo ""
    
    test_connectivity || exit 1
    
    # Check if gap test script exists
    if [ ! -f "$SCRIPT_DIR/gap_test.sh" ]; then
        warn "Gap test script not found, creating it..."
        # The gap_test.sh was already created above
    fi
    
    # Make it executable and run
    chmod +x "$SCRIPT_DIR/gap_test.sh"
    cd "$SCRIPT_DIR"
    bash ./gap_test.sh
}

# Run stress test with monitoring
run_stress_test() {
    log "Starting stress test with monitoring..."
    echo ""
    echo "This test will:"
    echo "  1. Run continuous load generation"
    echo "  2. Monitor performance metrics"
    echo "  3. Track delivery verification"
    echo "  4. Provide web dashboard on http://localhost:8080"
    echo ""
    
    test_connectivity || exit 1
    
    # Check for stress monitor
    cd "$SCRIPT_DIR"
    if [ -f "stress_monitor_v3.go" ]; then
        log "Building stress monitor..."
        go build -o stress_monitor stress_monitor_v3.go || error "Failed to build stress monitor"
        
        log "Starting stress monitor..."
        ./stress_monitor
    else
        warn "Stress monitor not found, falling back to standard test"
        run_standard_test
    fi
}

# Check prerequisites
check_prerequisites() {
    # Check for Go
    if ! command -v go &> /dev/null; then
        error "Go is not installed. Please install Go 1.18 or later."
    fi
    
    # Check for curl
    if ! command -v curl &> /dev/null; then
        error "curl is not installed. Please install curl."
    fi
    
    # Check for bc (needed for calculations)
    if ! command -v bc &> /dev/null; then
        warn "bc is not installed. Some calculations may not work."
    fi
}

# Main execution
main() {
    check_prerequisites
    
    case "$TEST_MODE" in
        standard)
            run_standard_test
            ;;
        gap)
            run_gap_test
            ;;
        stress)
            run_stress_test
            ;;
        help|--help|-h)
            show_usage
            exit 0
            ;;
        *)
            error "Unknown mode: $TEST_MODE. Use 'help' to see available modes."
            ;;
    esac
}

# Handle interrupts gracefully
trap 'echo ""; warn "Test interrupted by user"; exit 130' INT TERM

# Show header
clear
echo "╔══════════════════════════════════════════╗"
echo "║   ACCUMULATE DEVNET LOAD TEST ENHANCED   ║"
echo "╚══════════════════════════════════════════╝"
echo ""

# Run main function
main "$@"