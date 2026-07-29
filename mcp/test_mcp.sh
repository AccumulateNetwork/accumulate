#!/bin/bash

# Test script for MCP-Accumulate against testnet
# This script tests all implemented MCP tools

set -e

MCP_SERVER="./mcp-accumulate"
NETWORK="testnet"

echo "=== MCP-Accumulate Test Suite ==="
echo "Network: $NETWORK"
echo ""

# Check if binary exists
if [ ! -f "$MCP_SERVER" ]; then
    echo "Building MCP server..."
    go build -o mcp-accumulate
fi

# Function to call MCP tool
call_tool() {
    local tool_name=$1
    local params=$2

    echo "{"
    echo "  \"jsonrpc\": \"2.0\","
    echo "  \"id\": 1,"
    echo "  \"method\": \"tools/call\","
    echo "  \"params\": {"
    echo "    \"name\": \"$tool_name\","
    echo "    \"arguments\": $params"
    echo "  }"
    echo "}"
}

# Test 1: Query a known testnet account
echo "=== Test 1: Query Account ==="
call_tool "accumulate_query_account" '{"url": "acc://dn.acme", "network": "testnet"}' | \
    $MCP_SERVER 2>&1 | head -50

echo ""
echo "=== Test 2: Network Status ==="
call_tool "accumulate_network_status" '{"network": "testnet"}' | \
    $MCP_SERVER 2>&1 | head -50

echo ""
echo "=== Test 3: Node Info ==="
call_tool "accumulate_node_info" '{"network": "testnet"}' | \
    $MCP_SERVER 2>&1 | head -50

echo ""
echo "=== Test 4: Create Lite Account ==="
call_tool "accumulate_create_lite_account" '{"public_key": "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"}' | \
    $MCP_SERVER 2>&1 | head -20

echo ""
echo "=== Tests Complete ==="
