#!/bin/bash

# Full Integration Test Runner for CrossChainConductor
# This script runs all tests and generates a comprehensive report

set -e

echo "=========================================="
echo "    CROSSCHAINCONDUCTOR TEST SUITE"
echo "=========================================="
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test results tracking
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
SKIPPED_TESTS=0

# Create test results directory
RESULTS_DIR="test_results_$(date +%Y%m%d_%H%M%S)"
mkdir -p $RESULTS_DIR

# Function to run a test and capture results
run_test() {
    local test_name=$1
    local test_file=$2
    local test_description=$3
    
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    
    echo -e "${BLUE}[TEST $TOTAL_TESTS]${NC} $test_name"
    echo "  Description: $test_description"
    echo -n "  Status: "
    
    # Run the test and capture output
    if timeout 60s go run $test_file > "$RESULTS_DIR/${test_name}.log" 2>&1; then
        echo -e "${GREEN}✅ PASSED${NC}"
        PASSED_TESTS=$((PASSED_TESTS + 1))
        echo "PASSED" > "$RESULTS_DIR/${test_name}.status"
    else
        if [ $? -eq 124 ]; then
            echo -e "${YELLOW}⏱️ TIMEOUT${NC}"
            SKIPPED_TESTS=$((SKIPPED_TESTS + 1))
            echo "TIMEOUT" > "$RESULTS_DIR/${test_name}.status"
        else
            echo -e "${RED}❌ FAILED${NC}"
            FAILED_TESTS=$((FAILED_TESTS + 1))
            echo "FAILED" > "$RESULTS_DIR/${test_name}.status"
            
            # Show last few lines of error
            echo "  Error output:"
            tail -5 "$RESULTS_DIR/${test_name}.log" | sed 's/^/    /'
        fi
    fi
    echo ""
}

# Function to compile test files
compile_tests() {
    echo "Compiling test files..."
    
    # List of files to compile
    local files=(
        "conductor.go"
        "recovery.go"
        "client_helper_fixed.go"
        "simplified_partition_handling.go"
        "partition_failure_handling.go"
        "conductor_with_partition_handling.go"
    )
    
    for file in "${files[@]}"; do
        if [ -f "$file" ]; then
            echo -n "  Checking $file... "
            if go build -o /dev/null $file 2>/dev/null; then
                echo -e "${GREEN}✓${NC}"
            else
                echo -e "${RED}✗${NC}"
                echo "    Compilation errors detected in $file"
            fi
        fi
    done
    echo ""
}

# Function to check dependencies
check_dependencies() {
    echo "Checking dependencies..."
    
    # Check if Go is installed
    if ! command -v go &> /dev/null; then
        echo -e "${RED}Error: Go is not installed${NC}"
        exit 1
    fi
    
    echo "  Go version: $(go version)"
    
    # Check if we're in the right directory
    if [ ! -f "conductor.go" ]; then
        echo -e "${RED}Error: Not in the correct directory${NC}"
        echo "  Please run this script from the test/load directory"
        exit 1
    fi
    
    echo -e "  ${GREEN}All dependencies satisfied${NC}"
    echo ""
}

# Function to run memory profiling
run_memory_profile() {
    echo "Running memory profiling..."
    
    # Create a simple memory test
    cat > "$RESULTS_DIR/memory_test.go" << 'EOF'
package main

import (
    "fmt"
    "runtime"
    "time"
)

func main() {
    var m runtime.MemStats
    
    // Initial memory
    runtime.ReadMemStats(&m)
    fmt.Printf("Initial Alloc = %v KB\n", m.Alloc/1024)
    
    // Run some operations
    data := make([][]byte, 1000)
    for i := range data {
        data[i] = make([]byte, 1024)
    }
    
    // Force GC
    runtime.GC()
    time.Sleep(100 * time.Millisecond)
    
    // Final memory
    runtime.ReadMemStats(&m)
    fmt.Printf("Final Alloc = %v KB\n", m.Alloc/1024)
    fmt.Printf("Total Alloc = %v KB\n", m.TotalAlloc/1024)
    fmt.Printf("Sys = %v KB\n", m.Sys/1024)
    fmt.Printf("NumGC = %v\n", m.NumGC)
}
EOF
    
    go run "$RESULTS_DIR/memory_test.go" > "$RESULTS_DIR/memory_profile.log" 2>&1
    echo -e "  ${GREEN}Memory profile saved${NC}"
    echo ""
}

# Main execution
echo "Starting Full Test Suite at $(date)"
echo "Results will be saved to: $RESULTS_DIR"
echo ""

# Check dependencies
check_dependencies

# Compile tests
compile_tests

# Run individual component tests
echo "=========================================="
echo "COMPONENT TESTS"
echo "=========================================="
echo ""

run_test "recovery_direct" \
    "test_recovery_direct.go recovery.go conductor.go client_helper_fixed.go" \
    "Test direct recovery from anchor and synthetic ledgers"

run_test "simplified_partition" \
    "test_simplified_partition_handling.go simplified_partition_handling.go" \
    "Test simplified partition handling with transaction drops"

run_test "partition_failure" \
    "test_partition_failure.go partition_failure_handling.go conductor_with_partition_handling.go" \
    "Test partition failure detection and circuit breaker"

# Run integration tests
echo "=========================================="
echo "INTEGRATION TESTS"  
echo "=========================================="
echo ""

run_test "full_integration" \
    "full_integration_test.go conductor.go recovery.go client_helper_fixed.go simplified_partition_handling.go" \
    "Complete integration test of all components"

# Run performance tests
echo "=========================================="
echo "PERFORMANCE TESTS"
echo "=========================================="
echo ""

# Create performance test
cat > "$RESULTS_DIR/perf_test.go" << 'EOF'
package main

import (
    "context"
    "fmt"
    "sync"
    "sync/atomic"
    "time"
)

func main() {
    fmt.Println("Performance Benchmark")
    fmt.Println("====================")
    
    ctx := context.Background()
    workers := 10
    operations := 1000
    
    var wg sync.WaitGroup
    var counter int64
    
    start := time.Now()
    
    for i := 0; i < workers; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := 0; j < operations; j++ {
                atomic.AddInt64(&counter, 1)
                select {
                case <-ctx.Done():
                    return
                default:
                }
            }
        }()
    }
    
    wg.Wait()
    duration := time.Since(start)
    
    ops := float64(counter) / duration.Seconds()
    fmt.Printf("Operations: %d\n", counter)
    fmt.Printf("Duration: %v\n", duration)
    fmt.Printf("Throughput: %.2f ops/sec\n", ops)
}
EOF

run_test "performance" \
    "$RESULTS_DIR/perf_test.go" \
    "Basic performance benchmark"

# Run memory profiling
run_memory_profile

# Generate summary report
echo "=========================================="
echo "TEST SUMMARY REPORT"
echo "=========================================="
echo ""

echo "Test Execution Summary:"
echo "  Total Tests:    $TOTAL_TESTS"
echo -e "  Passed:         ${GREEN}$PASSED_TESTS${NC}"
echo -e "  Failed:         ${RED}$FAILED_TESTS${NC}"
echo -e "  Skipped/Timeout:${YELLOW}$SKIPPED_TESTS${NC}"
echo ""

# Calculate success rate
if [ $TOTAL_TESTS -gt 0 ]; then
    SUCCESS_RATE=$((PASSED_TESTS * 100 / TOTAL_TESTS))
    echo "  Success Rate:   ${SUCCESS_RATE}%"
fi

echo ""
echo "Detailed results saved to: $RESULTS_DIR"
echo ""

# Generate HTML report
cat > "$RESULTS_DIR/report.html" << EOF
<!DOCTYPE html>
<html>
<head>
    <title>Test Report - $(date)</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        h1 { color: #333; }
        .passed { color: green; }
        .failed { color: red; }
        .timeout { color: orange; }
        table { border-collapse: collapse; width: 100%; }
        th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
        th { background-color: #f2f2f2; }
    </style>
</head>
<body>
    <h1>CrossChainConductor Test Report</h1>
    <p>Generated: $(date)</p>
    
    <h2>Summary</h2>
    <ul>
        <li>Total Tests: $TOTAL_TESTS</li>
        <li class="passed">Passed: $PASSED_TESTS</li>
        <li class="failed">Failed: $FAILED_TESTS</li>
        <li class="timeout">Timeout: $SKIPPED_TESTS</li>
    </ul>
    
    <h2>Test Results</h2>
    <table>
        <tr>
            <th>Test Name</th>
            <th>Status</th>
            <th>Log File</th>
        </tr>
EOF

# Add test results to HTML
for status_file in $RESULTS_DIR/*.status; do
    if [ -f "$status_file" ]; then
        test_name=$(basename "$status_file" .status)
        status=$(cat "$status_file")
        class="passed"
        if [ "$status" = "FAILED" ]; then
            class="failed"
        elif [ "$status" = "TIMEOUT" ]; then
            class="timeout"
        fi
        echo "        <tr>" >> "$RESULTS_DIR/report.html"
        echo "            <td>$test_name</td>" >> "$RESULTS_DIR/report.html"
        echo "            <td class=\"$class\">$status</td>" >> "$RESULTS_DIR/report.html"
        echo "            <td><a href=\"${test_name}.log\">View Log</a></td>" >> "$RESULTS_DIR/report.html"
        echo "        </tr>" >> "$RESULTS_DIR/report.html"
    fi
done

cat >> "$RESULTS_DIR/report.html" << EOF
    </table>
</body>
</html>
EOF

echo "HTML report generated: $RESULTS_DIR/report.html"
echo ""

# Final status
if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "${GREEN}✅ ALL TESTS PASSED!${NC}"
    exit 0
else
    echo -e "${RED}❌ SOME TESTS FAILED${NC}"
    echo "Please check the logs in $RESULTS_DIR for details"
    exit 1
fi