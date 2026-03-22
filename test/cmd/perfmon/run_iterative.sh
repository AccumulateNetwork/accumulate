#!/bin/bash
# Run iterative performance testing with increasing load
# Finds maximum sustainable TPS for the network

set -e

OUTPUT_DIR="${1:-./perfmon_results/iterative_$(date +%Y%m%d_%H%M%S)}"
SERVER_URL="${2:-http://127.0.1.1:26660/v2}"
INITIAL_TPS="${3:-100}"
MAX_TPS="${4:-1000}"
INCREMENT="${5:-100}"
MINUTES="${6:-5}"

echo "=== Accumulate Iterative Performance Testing ==="
echo "Output directory: $OUTPUT_DIR"
echo "Server URL: $SERVER_URL"
echo "TPS Range: $INITIAL_TPS to $MAX_TPS (increment: $INCREMENT)"
echo "Duration per iteration: $MINUTES minutes"
echo

# Check if server is available
if ! curl -s -f "$SERVER_URL/status" > /dev/null 2>&1; then
    echo "ERROR: Server not responding at $SERVER_URL"
    echo "Please start devnet first with: cd test/cmd/devnet && go run . start"
    exit 1
fi

echo "Server is available, starting iterative testing..."
echo "This will take approximately $(( (MAX_TPS - INITIAL_TPS) / INCREMENT * MINUTES + MINUTES )) minutes"
echo

# Run iterative tests
go run . \
    -url "$SERVER_URL" \
    -output "$OUTPUT_DIR" \
    -initial-tps "$INITIAL_TPS" \
    -max-tps "$MAX_TPS" \
    -tps-increment "$INCREMENT" \
    -iteration-minutes "$MINUTES" \
    -interval 5s

echo
echo "=== Iterative Testing Complete ==="
echo "Results saved to: $OUTPUT_DIR"
echo
echo "View final report:"
echo "  cat $OUTPUT_DIR/final_report.txt"
echo
echo "View summary CSV:"
echo "  cat $OUTPUT_DIR/summary.csv"
echo
echo "Generate visualizations (requires gnuplot):"
echo "  cd $OUTPUT_DIR && gnuplot plot.gnu"
echo
