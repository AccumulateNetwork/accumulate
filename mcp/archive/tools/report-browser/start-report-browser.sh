#!/bin/bash
# Start the MCP Report Browser on port 8080

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Build if needed
if [ ! -f "./report-browser" ] || [ "main.go" -nt "./report-browser" ]; then
    echo "Building report browser..."
    go build -o report-browser .
fi

# Kill existing instance
killall report-browser 2>/dev/null
sleep 1

# Start server
echo "Starting Report Browser on http://localhost:8080"
./report-browser --port 8080 --dir ../../ > report-browser.log 2>&1 &

PID=$!
echo "Report Browser started with PID: $PID"
echo "Access it at: http://localhost:8080"
echo "Logs at: $SCRIPT_DIR/report-browser.log"
