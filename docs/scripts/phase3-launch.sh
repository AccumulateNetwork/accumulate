#!/bin/bash

# Phase 3 - Launch Cyclops validator node
set -e

DEPLOY_DIR="/tmp/cyclops/node"
LOG_DIR="/tmp/cyclops/logs"
# Ensure logs directory exists
mkdir -p "$LOG_DIR"

# Check deployment directory exists
if [ ! -d "$DEPLOY_DIR" ]; then
    echo "ERROR: Deployment directory not found: $DEPLOY_DIR"
    echo "Run Phase 2 deployment first: ./phase2-deploy.sh"
    exit 1
fi

cd "$DEPLOY_DIR"

# Launch node
nohup /tmp/cyclops/artifacts/accumulated run --work-dir . > "$LOG_DIR/cyclops-node.log" 2>&1 &
NODE_PID=$!

echo "$NODE_PID" > "$LOG_DIR/cyclops-node.pid"

echo "Node started with PID: $NODE_PID"
echo "Log file: $LOG_DIR/cyclops-node.log"

echo "=== Phase 3 Complete ==="

# Monitor node process for halting
echo "Monitoring node process (PID: $NODE_PID)... Press Ctrl+C to stop monitoring."
wait $NODE_PID
EXIT_CODE=$?
echo "ERROR: Node halted unexpectedly with exit code $EXIT_CODE"
echo "Check logs at $LOG_DIR/cyclops-node.log"
exit $EXIT_CODE
