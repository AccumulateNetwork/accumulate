#!/bin/bash

# Launch all Accumulate SDK example web UIs
# Each app will run on a different port

echo "🚀 Launching Accumulate SDK Example Web UIs"
echo "============================================"
echo ""

# Kill any existing instances
pkill -f "go run.*-web" 2>/dev/null

# Network Monitor on port 8080
echo "Starting Network Monitor on http://localhost:8080"
(cd network_monitor && go run . -web -port 8080) &
MONITOR_PID=$!

# Balance Checker on port 8081
echo "Starting Balance Checker on http://localhost:8081"
(cd balance_checker && go run . -web -port 8081) &
BALANCE_PID=$!

# Account Explorer on port 8082 (when web UI is added)
# echo "Starting Account Explorer on http://localhost:8082"
# (cd account_explorer && go run . -web -port 8082) &
# EXPLORER_PID=$!

# Data Reader on port 8083 (when web UI is added)
# echo "Starting Data Reader on http://localhost:8083"
# (cd data_reader && go run . -web -port 8083) &
# READER_PID=$!

echo ""
echo "✅ Web UIs are starting up..."
echo ""
echo "Available applications:"
echo "  • Network Monitor:  http://localhost:8080"
echo "  • Balance Checker:  http://localhost:8081"
# echo "  • Account Explorer: http://localhost:8082"
# echo "  • Data Reader:      http://localhost:8083"
echo ""
echo "Press Ctrl+C to stop all servers"
echo ""

# Function to handle cleanup
cleanup() {
    echo ""
    echo "Shutting down all servers..."
    kill $MONITOR_PID 2>/dev/null
    kill $BALANCE_PID 2>/dev/null
    # kill $EXPLORER_PID 2>/dev/null
    # kill $READER_PID 2>/dev/null
    pkill -f "go run.*-web" 2>/dev/null
    echo "All servers stopped."
    exit 0
}

# Set up trap to handle Ctrl+C
trap cleanup INT TERM

# Wait for all background processes
wait