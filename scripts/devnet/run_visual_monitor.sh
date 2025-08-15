#!/bin/bash

# Script to run visual monitoring with JSON logging for both human and AI analysis

echo "=========================================="
echo "VISUAL PARTITION MONITOR WITH JSON LOGGING"
echo "=========================================="
echo ""
echo "This script will:"
echo "  1. Run the visual partition monitor in your terminal"
echo "  2. Save JSON logs to monitor_logs.json"
echo "  3. Allow real-time interaction with the visual interface"
echo ""
echo "TO RUN THE VISUAL MONITOR:"
echo "  ./run_visual_monitor.sh"
echo ""
echo "INTERACTIVE CONTROLS:"
echo "  Press 1-4: Toggle partition health (1=BVN0, 2=BVN1, 3=BVN2, 4=Directory)"
echo "  Press 'c': Cause cascading failure"
echo "  Press 'r': Recover all partitions" 
echo "  Press 'q': Quit"
echo ""
echo "JSON LOGS will be saved to:"
echo "  - monitor_logs.json (structured data)"
echo "  - monitor_visual.log (visual output)"
echo ""
echo "Starting in 3 seconds..."
sleep 3

# Run the visual monitor and capture both visual and JSON output
go run visual_partition_monitor.go 2>&1 | tee monitor_visual.log &
MONITOR_PID=$!

# In parallel, capture JSON metrics every second
(
  while kill -0 $MONITOR_PID 2>/dev/null; do
    TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    echo "{\"timestamp\":\"$TIMESTAMP\",\"type\":\"heartbeat\"}" >> monitor_logs.json
    sleep 1
  done
) &
LOGGER_PID=$!

# Wait for the monitor to finish
wait $MONITOR_PID

# Clean up the logger
kill $LOGGER_PID 2>/dev/null

echo ""
echo "Monitor stopped. Logs saved to:"
echo "  - monitor_visual.log (visual output)"
echo "  - monitor_logs.json (structured data)"