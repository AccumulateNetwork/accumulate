#!/bin/bash

echo "🔍 Monitoring Anchor Heights for 1 minute..."
echo "========================================="

# Get starting height
start_height=$(curl -s http://localhost:9093/api/status | jq -r '.networkStatus.directoryHeight')
start_major=$(curl -s http://localhost:9093/api/status | jq -r '.networkStatus.majorBlockHeight')

echo "Starting DN Height: $start_height"
echo "Starting Major Block: $start_major"
echo ""
echo "Time      | DN Height | Change | Major | TPS"
echo "----------|-----------|--------|-------|--------"

# Monitor for 60 seconds
for i in {1..12}; do
    sleep 5
    
    # Get current values
    current=$(curl -s http://localhost:9093/api/status | jq -r '.networkStatus.directoryHeight')
    major=$(curl -s http://localhost:9093/api/status | jq -r '.networkStatus.majorBlockHeight')
    dir_tps=$(curl -s http://localhost:9093/api/metrics | jq -r '.partitions.Directory.tps')
    cyc_tps=$(curl -s http://localhost:9093/api/metrics | jq -r '.partitions.Cyclops.tps')
    
    # Calculate changes
    change=$((current - start_height))
    major_change=$((major - start_major))
    
    # Format output
    printf "%-9s | %-9s | %+6d | %-5s | D:%.3f C:%.3f\n" \
        "$(date +%H:%M:%S)" \
        "$current" \
        "$change" \
        "$major" \
        "$dir_tps" \
        "$cyc_tps"
done

echo "========================================="
final=$(curl -s http://localhost:9093/api/status | jq -r '.networkStatus.directoryHeight')
total_change=$((final - start_height))
echo "Total change: $total_change blocks in 60 seconds"
echo "Rate: $(echo "scale=2; $total_change / 60" | bc) blocks/second"