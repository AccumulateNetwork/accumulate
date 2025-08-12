#!/bin/bash

echo "🔍 Monitoring Accumulate Mainnet Anchor Heights..."
echo "=================================================="
echo "This monitors the REMOTE mainnet at:"
echo "https://mainnet.accumulatenetwork.io"
echo ""

# Get initial height
prev_height=$(curl -s https://mainnet.accumulatenetwork.io/v3 -X POST \
    -H 'Content-Type: application/json' \
    -d '{"jsonrpc":"2.0","method":"network-status","params":{},"id":1}' | \
    jq -r '.result.directoryHeight')

prev_major=$(curl -s https://mainnet.accumulatenetwork.io/v3 -X POST \
    -H 'Content-Type: application/json' \
    -d '{"jsonrpc":"2.0","method":"network-status","params":{},"id":1}' | \
    jq -r '.result.majorBlockHeight')

echo "Starting Height: $prev_height"
echo "Starting Major:  $prev_major"
echo ""
echo "Monitoring for changes..."
echo ""

check_count=0
changes_detected=0

while true; do
    sleep 5
    check_count=$((check_count + 1))
    
    # Get current heights
    response=$(curl -s https://mainnet.accumulatenetwork.io/v3 -X POST \
        -H 'Content-Type: application/json' \
        -d '{"jsonrpc":"2.0","method":"network-status","params":{},"id":1}')
    
    current_height=$(echo "$response" | jq -r '.result.directoryHeight')
    current_major=$(echo "$response" | jq -r '.result.majorBlockHeight')
    
    # Get TPS
    tps=$(curl -s https://mainnet.accumulatenetwork.io/v3 -X POST \
        -H 'Content-Type: application/json' \
        -d '{"jsonrpc":"2.0","method":"metrics","params":{"partition":"Directory"},"id":1}' | \
        jq -r '.result.tps')
    
    # Check for changes
    if [ "$current_height" != "$prev_height" ]; then
        change=$((current_height - prev_height))
        changes_detected=$((changes_detected + 1))
        echo "🎯 $(date +%H:%M:%S) HEIGHT CHANGED! $prev_height → $current_height (+$change blocks) | TPS: $tps"
        prev_height=$current_height
    fi
    
    if [ "$current_major" != "$prev_major" ]; then
        echo "⚡ $(date +%H:%M:%S) MAJOR BLOCK! $prev_major → $current_major"
        prev_major=$current_major
    fi
    
    # Status update every 12 checks (1 minute)
    if [ $((check_count % 12)) -eq 0 ]; then
        echo "📊 $(date +%H:%M:%S) Status: Height=$current_height, Major=$current_major, TPS=$tps, Changes=$changes_detected"
    fi
done