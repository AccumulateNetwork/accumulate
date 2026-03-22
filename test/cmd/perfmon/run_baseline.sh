#!/bin/bash
# Run baseline performance test at 100 TPS
# This establishes baseline metrics for comparison

set -e

OUTPUT_DIR="${1:-./perfmon_results/baseline_$(date +%Y%m%d_%H%M%S)}"
SERVER_URL="${2:-http://127.0.1.1:26660/v2}"

echo "=== Accumulate Performance Baseline Test ==="
echo "Output directory: $OUTPUT_DIR"
echo "Server URL: $SERVER_URL"
echo

# Check if server is available
if ! curl -s -f "$SERVER_URL/status" > /dev/null 2>&1; then
    echo "ERROR: Server not responding at $SERVER_URL"
    echo "Please start devnet first with: cd test/cmd/devnet && go run . start"
    exit 1
fi

echo "Server is available, starting baseline test..."
echo

# Run 100 TPS baseline for 5 minutes
go run . \
    -url "$SERVER_URL" \
    -output "$OUTPUT_DIR" \
    -initial-tps 100 \
    -max-tps 100 \
    -iteration-minutes 5 \
    -interval 5s

echo
echo "=== Baseline Test Complete ==="
echo "Results saved to: $OUTPUT_DIR"
echo
echo "View report:"
echo "  cat $OUTPUT_DIR/final_report.txt"
echo
echo "View metrics:"
echo "  cat $OUTPUT_DIR/run_100_tps_*/report.txt"
echo
