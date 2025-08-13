#!/bin/bash

# Load Test Runner with Flexible DevNet Configuration
# Automates running load tests with different DevNet configurations

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
RESULTS_DIR="$SCRIPT_DIR/test_results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
NC='\033[0m'

# Test configurations
declare -A TEST_CONFIGS=(
    ["minimal"]="2:1:0"      # 2 BVNs, 1 validator, 0 followers
    ["standard"]="2:3:1"     # 2 BVNs, 3 validators, 1 follower
    ["large"]="3:3:2"        # 3 BVNs, 3 validators, 2 followers
    ["cross_chain"]="3:2:1"  # 3 BVNs for cross-chain testing
    ["stress"]="5:3:2"       # 5 BVNs for stress testing
)

# Test suites
declare -A TEST_SUITES=(
    ["basic"]="multi_validator_conductor.go"
    ["routing"]="crosschain_routing_load.go"
    ["blocking"]="per_destination_blocking.go"
    ["error"]="crosschain_error_retry.go"
    ["full"]="multi_validator_conductor.go crosschain_routing_load.go per_destination_blocking.go"
)

# Logging functions
log() { echo -e "${BLUE}[$(date +'%H:%M:%S')]${NC} $1"; }
error() { echo -e "${RED}[$(date +'%H:%M:%S')] ERROR:${NC} $1"; }
success() { echo -e "${GREEN}[$(date +'%H:%M:%S')] SUCCESS:${NC} $1"; }
info() { echo -e "${CYAN}[$(date +'%H:%M:%S')] INFO:${NC} $1"; }
warn() { echo -e "${YELLOW}[$(date +'%H:%M:%S')] WARNING:${NC} $1"; }

# Create results directory
mkdir -p "$RESULTS_DIR"

# Function to start DevNet with configuration
start_devnet() {
    local config_name=$1
    local config="${TEST_CONFIGS[$config_name]}"
    
    if [ -z "$config" ]; then
        error "Unknown configuration: $config_name"
        return 1
    fi
    
    IFS=':' read -r bvns validators followers <<< "$config"
    
    info "🚀 Starting DevNet: $config_name (BVNs:$bvns, Validators:$validators, Followers:$followers)"
    
    # Use the flexible devnet_config.sh script
    "$SCRIPT_DIR/devnet_config.sh" start $bvns $validators $followers
    
    # Wait a bit for stabilization
    sleep 5
    
    # Verify DevNet is running
    if ! "$SCRIPT_DIR/devnet_config.sh" status | grep -q "API responding"; then
        error "DevNet failed to start properly"
        return 1
    fi
    
    success "DevNet started successfully"
    return 0
}

# Function to stop DevNet
stop_devnet() {
    info "🛑 Stopping DevNet..."
    "$SCRIPT_DIR/devnet_config.sh" stop
    sleep 2
}

# Function to run a single test
run_test() {
    local test_file=$1
    local config_name=$2
    local result_file="$RESULTS_DIR/${config_name}_${test_file%.go}_$TIMESTAMP.txt"
    
    if [ ! -f "$SCRIPT_DIR/$test_file" ]; then
        error "Test file not found: $test_file"
        return 1
    fi
    
    info "🧪 Running test: $test_file"
    
    # Run the test and capture output
    if timeout 300 go run "$SCRIPT_DIR/$test_file" > "$result_file" 2>&1; then
        success "Test completed: $test_file"
        
        # Extract key metrics
        extract_metrics "$result_file"
        return 0
    else
        error "Test failed: $test_file"
        echo "Check logs at: $result_file"
        return 1
    fi
}

# Function to extract and display metrics
extract_metrics() {
    local result_file=$1
    
    # Extract common metrics
    local tps=$(grep -oP 'TPS: \K[\d.]+' "$result_file" 2>/dev/null || echo "N/A")
    local success_rate=$(grep -oP 'Success rate: \K[\d.]+' "$result_file" 2>/dev/null || echo "N/A")
    local total_tx=$(grep -oP 'Total transactions: \K\d+' "$result_file" 2>/dev/null || echo "N/A")
    
    info "📊 Metrics: TPS=$tps, Success=$success_rate%, Transactions=$total_tx"
}

# Function to run a test suite
run_test_suite() {
    local suite_name=$1
    local config_name=$2
    
    local tests="${TEST_SUITES[$suite_name]}"
    if [ -z "$tests" ]; then
        error "Unknown test suite: $suite_name"
        return 1
    fi
    
    info "📦 Running test suite: $suite_name with config: $config_name"
    
    # Start DevNet
    if ! start_devnet "$config_name"; then
        error "Failed to start DevNet"
        return 1
    fi
    
    local failed_tests=0
    local passed_tests=0
    
    # Run each test in the suite
    for test in $tests; do
        if run_test "$test" "$config_name"; then
            ((passed_tests++))
        else
            ((failed_tests++))
        fi
        
        # Brief pause between tests
        sleep 2
    done
    
    # Stop DevNet
    stop_devnet
    
    # Report results
    echo ""
    info "📈 Suite Results: $suite_name"
    success "  Passed: $passed_tests"
    if [ $failed_tests -gt 0 ]; then
        error "  Failed: $failed_tests"
    fi
    
    return $failed_tests
}

# Function to run all combinations
run_all_combinations() {
    info "🔄 Running all test combinations..."
    
    local total_tests=0
    local failed_tests=0
    
    for config in "${!TEST_CONFIGS[@]}"; do
        for suite in "${!TEST_SUITES[@]}"; do
            echo ""
            info "═══════════════════════════════════════════"
            info "Configuration: $config, Suite: $suite"
            info "═══════════════════════════════════════════"
            
            if run_test_suite "$suite" "$config"; then
                ((total_tests++))
            else
                ((total_tests++))
                ((failed_tests++))
            fi
        done
    done
    
    # Final summary
    echo ""
    info "═══════════════════════════════════════════"
    info "📊 FINAL SUMMARY"
    info "═══════════════════════════════════════════"
    success "Total test runs: $total_tests"
    if [ $failed_tests -gt 0 ]; then
        error "Failed runs: $failed_tests"
    else
        success "All tests passed!"
    fi
    
    info "Results saved in: $RESULTS_DIR"
}

# Function to run specific cross-chain tests
run_cross_chain_tests() {
    info "🔗 Running cross-chain specific tests..."
    
    # Use 3 BVN configuration for cross-chain tests
    if ! start_devnet "cross_chain"; then
        error "Failed to start DevNet for cross-chain testing"
        return 1
    fi
    
    # Run cross-chain specific tests
    local tests=(
        "crosschain_routing_load.go"
        "per_destination_blocking.go"
    )
    
    for test in "${tests[@]}"; do
        run_test "$test" "cross_chain"
        sleep 2
    done
    
    stop_devnet
}

# Function to run performance benchmarks
run_benchmarks() {
    info "⚡ Running performance benchmarks..."
    
    local configs=("minimal" "standard" "large")
    local benchmark_file="$RESULTS_DIR/benchmark_$TIMESTAMP.csv"
    
    # Create CSV header
    echo "Config,BVNs,Validators,Followers,TPS,SuccessRate,TotalTx" > "$benchmark_file"
    
    for config in "${configs[@]}"; do
        IFS=':' read -r bvns validators followers <<< "${TEST_CONFIGS[$config]}"
        
        info "Benchmarking configuration: $config"
        
        if ! start_devnet "$config"; then
            error "Failed to start DevNet for $config"
            continue
        fi
        
        # Run standard load test
        local result_file="$RESULTS_DIR/benchmark_${config}_$TIMESTAMP.txt"
        timeout 120 go run "$SCRIPT_DIR/multi_validator_conductor.go" > "$result_file" 2>&1
        
        # Extract metrics
        local tps=$(grep -oP 'TPS: \K[\d.]+' "$result_file" 2>/dev/null || echo "0")
        local success_rate=$(grep -oP 'Success rate: \K[\d.]+' "$result_file" 2>/dev/null || echo "0")
        local total_tx=$(grep -oP 'Total transactions: \K\d+' "$result_file" 2>/dev/null || echo "0")
        
        # Save to CSV
        echo "$config,$bvns,$validators,$followers,$tps,$success_rate,$total_tx" >> "$benchmark_file"
        
        stop_devnet
        sleep 5
    done
    
    success "Benchmarks completed. Results saved to: $benchmark_file"
    
    # Display results
    echo ""
    info "📊 Benchmark Results:"
    column -t -s',' "$benchmark_file"
}

# Function to show available configurations
show_configs() {
    info "📋 Available DevNet Configurations:"
    echo ""
    for config in "${!TEST_CONFIGS[@]}"; do
        IFS=':' read -r bvns validators followers <<< "${TEST_CONFIGS[$config]}"
        echo "  $config:"
        echo "    - BVNs: $bvns"
        echo "    - Validators per BVN: $validators"
        echo "    - Followers per BVN: $followers"
        echo ""
    done
    
    info "📦 Available Test Suites:"
    echo ""
    for suite in "${!TEST_SUITES[@]}"; do
        echo "  $suite: ${TEST_SUITES[$suite]}"
    done
}

# Main execution
case "${1:-help}" in
    "suite")
        # Run a specific test suite with specific config
        if [ -z "$2" ] || [ -z "$3" ]; then
            error "Usage: $0 suite <suite_name> <config_name>"
            echo "Example: $0 suite full standard"
            exit 1
        fi
        run_test_suite "$2" "$3"
        ;;
    
    "test")
        # Run a specific test with specific config
        if [ -z "$2" ] || [ -z "$3" ]; then
            error "Usage: $0 test <test_file> <config_name>"
            echo "Example: $0 test crosschain_routing_load.go large"
            exit 1
        fi
        start_devnet "$3"
        run_test "$2" "$3"
        stop_devnet
        ;;
    
    "all")
        # Run all test combinations
        run_all_combinations
        ;;
    
    "cross-chain")
        # Run cross-chain specific tests
        run_cross_chain_tests
        ;;
    
    "benchmark")
        # Run performance benchmarks
        run_benchmarks
        ;;
    
    "configs")
        # Show available configurations
        show_configs
        ;;
    
    "quick")
        # Quick test with minimal config
        info "🚀 Running quick test..."
        run_test_suite "basic" "minimal"
        ;;
    
    "standard")
        # Standard test suite
        info "🚀 Running standard test suite..."
        run_test_suite "full" "standard"
        ;;
    
    "help"|"-h"|"--help"|"")
        echo "🧪 Load Test Runner with Flexible DevNet Configuration"
        echo ""
        echo "Usage: $0 <command> [options]"
        echo ""
        echo "Commands:"
        echo "  suite <name> <config>  - Run test suite with config"
        echo "  test <file> <config>   - Run single test with config"
        echo "  all                    - Run all test combinations"
        echo "  cross-chain            - Run cross-chain specific tests"
        echo "  benchmark              - Run performance benchmarks"
        echo "  configs                - Show available configurations"
        echo "  quick                  - Quick test with minimal setup"
        echo "  standard               - Standard full test suite"
        echo ""
        echo "Examples:"
        echo "  $0 suite full standard              # Full suite, standard config"
        echo "  $0 test routing.go large            # Single test, large config"
        echo "  $0 cross-chain                      # Cross-chain tests"
        echo "  $0 benchmark                        # Performance comparison"
        echo ""
        echo "Test Suites:"
        echo "  basic     - Basic multi-validator test"
        echo "  routing   - Cross-chain routing test"
        echo "  blocking  - Per-destination blocking test"
        echo "  error     - Error handling and retry test"
        echo "  full      - All core tests"
        echo ""
        echo "Configurations:"
        echo "  minimal    - 2 BVNs, 1 validator (quick)"
        echo "  standard   - 2 BVNs, 3 validators, 1 follower"
        echo "  large      - 3 BVNs, 3 validators, 2 followers"
        echo "  cross_chain - 3 BVNs optimized for cross-chain"
        echo "  stress     - 5 BVNs for stress testing"
        echo ""
        ;;
    
    *)
        error "Unknown command: $1"
        echo "Run '$0 help' for usage"
        exit 1
        ;;
esac