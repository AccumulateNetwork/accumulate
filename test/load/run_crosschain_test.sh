#!/bin/bash

# Crosschain Load Test Runner
# Usage: ./run_crosschain_test.sh [preset] [version]
# Presets: quick, medium, intensive, marathon
# Versions: realistic (default), simulation

set -e

# Default values
SERVER_URL="http://127.0.0.1:26660/v2"
DURATION="5m"
ADIS=3
ACCOUNTS=6
WORKERS=3
VERBOSE=""

# Parse preset and version
PRESET=${1:-quick}
VERSION=${2:-realistic}

case $PRESET in
    "quick")
        DURATION="2m"
        ADIS=2
        ACCOUNTS=4
        WORKERS=2
        echo "Quick test: 2 minutes, 2 ADIs, 4 accounts, 2 workers"
        ;;
    "medium")
        DURATION="10m"
        ADIS=5
        ACCOUNTS=10
        WORKERS=5
        echo "Medium test: 10 minutes, 5 ADIs, 10 accounts, 5 workers"
        ;;
    "intensive")
        DURATION="30m"
        ADIS=10
        ACCOUNTS=20
        WORKERS=10
        VERBOSE="-v"
        echo "Intensive test: 30 minutes, 10 ADIs, 20 accounts, 10 workers (verbose)"
        ;;
    "conductor")
        echo "Running realistic crosschain conductor load test..."
        go build -o crosschain_conductor crosschain_conductor.go
        if [ $? -eq 0 ]; then
            ./crosschain_conductor -verbose
        else
            echo "Failed to build crosschain conductor test"
            exit 1
        fi
        exit 0
        ;;
    "marathon")
        DURATION="2h"
        ADIS=20
        ACCOUNTS=50
        WORKERS=15
        VERBOSE="-v"
        echo "Marathon test: 2 hours, 20 ADIs, 50 accounts, 15 workers (verbose)"
        ;;
    *)
        echo "Unknown preset: $PRESET"
        echo "Available presets: quick, medium, intensive, marathon, conductor"
        echo "Available versions: realistic, simulation"
        exit 1
        ;;
esac

echo "=========================================="
echo "Crosschain Load Test Configuration"
echo "=========================================="
echo "Server URL: $SERVER_URL"
echo "Duration: $DURATION"
echo "ADIs: $ADIS"
echo "Accounts per ADI: $ACCOUNTS"
echo "Workers: $WORKERS"
echo "Verbose: $([ -n "$VERBOSE" ] && echo "Yes" || echo "No")"
echo "Version: $VERSION"
echo "=========================================="
echo ""

# Check if DevNet is running
echo "Checking DevNet connectivity..."
if ! curl -s -f -X POST "$SERVER_URL" \
    -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"describe","params":{},"id":1}' > /dev/null; then
    echo "❌ DevNet is not accessible at $SERVER_URL"
    echo "Please start DevNet first:"
    echo "  go run ./cmd/accumulated run devnet -w .nodes"
    exit 1
fi
echo "✓ DevNet is accessible"
echo ""

# Build and run the test
echo "Building crosschain load test ($VERSION version)..."
cd "$(dirname "$0")"

if [ "$VERSION" = "realistic" ]; then
    go build -o crosschain_load_test simple_crosschain_test.go
    echo "Using realistic version with actual DevNet API calls"
else
    go build -o crosschain_load_test crosschain_load_test.go
    echo "Using simulation version with mock operations"
fi

echo "Starting crosschain load test..."
echo "Press Ctrl+C to stop early"
echo ""

./crosschain_load_test \
    -server="$SERVER_URL" \
    -duration="$DURATION" \
    -adis="$ADIS" \
    -accounts="$ACCOUNTS" \
    -workers="$WORKERS" \
    $VERBOSE

echo ""
echo "Crosschain load test completed!"
echo "Check the output above for performance metrics and any issues."
