#!/bin/bash

# Comprehensive test suite runner with visual monitoring and JSON logging

echo "================================================================================"
echo "              ACCUMULATE COLLECTION PROOF TEST SUITE"
echo "================================================================================"
echo ""
echo "This script will run the complete test suite with visual monitoring."
echo ""

# Function to run a test and capture output
run_test() {
    local test_name=$1
    local test_file=$2
    local output_file=$3
    
    echo "🔄 Running: $test_name"
    echo "   Output: $output_file"
    go run $test_file > $output_file 2>&1
    if [ $? -eq 0 ]; then
        echo "   ✅ Completed successfully"
    else
        echo "   ❌ Failed (check $output_file for details)"
    fi
    echo ""
}

# Create results directory
mkdir -p test_results
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
RESULTS_DIR="test_results/run_$TIMESTAMP"
mkdir -p $RESULTS_DIR

echo "📁 Results will be saved to: $RESULTS_DIR"
echo ""
echo "================================================================================
                              TEST EXECUTION
================================================================================"
echo ""

# 1. Run collection proof performance test
run_test "Collection Proof Performance Test" \
    "test_collection_proof_performance.go" \
    "$RESULTS_DIR/performance_test.log"

# 2. Run batch proof integration test
run_test "Batch Proof Integration Test" \
    "test_batch_proof_integration.go" \
    "$RESULTS_DIR/integration_test.log"

# 3. Run optimized synthetic sender test
run_test "Optimized Synthetic Sender Test" \
    "optimized_synthetic_sender.go" \
    "$RESULTS_DIR/synthetic_sender_test.log"

echo "================================================================================
                           VISUAL MONITORING
================================================================================"
echo ""
echo "Now starting the visual monitor with JSON logging..."
echo ""
echo "TO VIEW THE VISUAL INTERFACE:"
echo "  The visual display will appear here in your terminal"
echo ""
echo "TO VIEW JSON LOGS IN ANOTHER TERMINAL:"
echo "  tail -f monitor_metrics.json | jq '.'"
echo ""
echo "TO ANALYZE RESULTS LATER:"
echo "  cat $RESULTS_DIR/*.log"
echo "  cat monitor_metrics.json | jq '.' | less"
echo ""
echo "INTERACTIVE CONTROLS FOR VISUAL MONITOR:"
echo "  1-4: Toggle partition health"
echo "  c: Cause cascading failure"
echo "  r: Recover all partitions"
echo "  b: Simulate batch proof optimization"
echo "  q: Quit monitor"
echo ""
echo "Starting visual monitor in 3 seconds..."
sleep 3

# 4. Run visual monitor with JSON logging
echo "" > monitor_metrics.json  # Clear previous metrics
go run visual_monitor_with_json.go

# Copy results to results directory
cp monitor_metrics.json $RESULTS_DIR/

echo ""
echo "================================================================================
                           TEST SUITE COMPLETE
================================================================================"
echo ""
echo "📊 RESULTS SUMMARY:"
echo ""
echo "All test results saved to: $RESULTS_DIR/"
echo ""
echo "Files generated:"
ls -la $RESULTS_DIR/
echo ""
echo "TO ANALYZE JSON METRICS:"
echo "  cat $RESULTS_DIR/monitor_metrics.json | jq '.network'"
echo "  cat $RESULTS_DIR/monitor_metrics.json | jq '.performance'"
echo "  cat $RESULTS_DIR/monitor_metrics.json | jq '.partitions'"
echo ""
echo "TO VIEW EFFICIENCY GAINS:"
echo "  cat $RESULTS_DIR/monitor_metrics.json | jq '.network.proof_savings'"
echo ""
echo "✅ Test suite execution complete!"