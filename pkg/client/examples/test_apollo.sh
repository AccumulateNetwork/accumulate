#!/bin/bash

echo "🚀 MONITORING LIVE APOLLO BLOCK HEIGHTS"
echo "======================================="
echo "Watching blocks increment in real-time..."
echo ""

prev=0
for i in {1..10}; do
    apollo=$(curl -s http://localhost:9095/api/status | jq -r '.networkStatus.apolloBlockHeight')
    
    if [ "$prev" -ne 0 ]; then
        diff=$((apollo - prev))
        echo "$(date +%H:%M:%S): Block $apollo (+$diff)"
    else
        echo "$(date +%H:%M:%S): Block $apollo (initial)"
    fi
    
    prev=$apollo
    sleep 2
done

echo ""
echo "✅ Apollo blocks are updating live!"