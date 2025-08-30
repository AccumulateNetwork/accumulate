#!/bin/bash

echo "Monitoring Apollo Mainnet Block Heights..."
echo "========================================="

for i in {1..10}; do
    response=$(curl -s http://apollo-mainnet.accumulate.defidevs.io:16692/status)
    height=$(echo "$response" | jq -r '.result.sync_info.latest_block_height')
    block_time=$(echo "$response" | jq -r '.result.sync_info.latest_block_time')
    
    echo "$(date +%H:%M:%S): Block $height at $block_time"
    sleep 2
done