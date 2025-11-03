#!/bin/bash
# Test script for MCP database access

MCP_BIN="./mcp-accumulate"

echo "=== Testing MCP Database Access ==="
echo ""

# Test 1: List all databases
echo "Test 1: List all known databases"
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"accumulate_db_list","arguments":{}}}' | $MCP_BIN 2>/dev/null | jq '.result.content[0].text | fromjson' || echo "Failed to list databases"
echo ""

# Test 2: List accounts from a database
echo "Test 2: List accounts from 2025-07-13-dn database (limit 10)"
echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"accumulate_db_list_accounts","arguments":{"database":"2025-07-13-dn","limit":10}}}' | $MCP_BIN 2>/dev/null | jq '.result.content[0].text | fromjson' || echo "Failed to list accounts"
echo ""

# Test 3: Query a system account
echo "Test 3: Query acc://dn.acme/operators account"
echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"accumulate_db_query_account","arguments":{"url":"acc://dn.acme/operators","database":"2025-07-13-dn"}}}' | $MCP_BIN 2>/dev/null | jq '.result.content[0].text | fromjson' || echo "Failed to query account"
echo ""

echo "=== Tests Complete ==="
