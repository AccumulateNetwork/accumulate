#!/bin/bash
# Comprehensive test script for MCP database access

MCP_BIN="./mcp-accumulate"

echo "=== Comprehensive MCP Database Test ==="
echo ""

# Test 1: Query ACME token account
echo "Test 1: Query ACME token account from DN database"
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"accumulate_db_query_account","arguments":{"url":"acc://ACME","database":"2025-07-13-dn"}}}' | $MCP_BIN 2>/dev/null | jq -r '.result.content[0].text' | jq .
echo ""

# Test 2: Query network operators
echo "Test 2: Query network operators from DN database"
echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"accumulate_db_query_account","arguments":{"url":"acc://dn.acme/network","database":"2025-07-13-dn"}}}' | $MCP_BIN 2>/dev/null | jq -r '.result.content[0].text' | jq .
echo ""

# Test 3: Query chain from ACME account
echo "Test 3: Query main chain from ACME account"
echo '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"accumulate_db_query_chain","arguments":{"url":"acc://ACME","chain_name":"main","database":"2025-07-13-dn","start":0,"count":5}}}' | $MCP_BIN 2>/dev/null | jq -r '.result.content[0].text' | jq .
echo ""

# Test 4: Get BPT hash from DN database
echo "Test 4: Get BPT root hash from 2025-07-13-dn"
echo '{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"accumulate_db_get_bpt_hash","arguments":{"database":"2025-07-13-dn"}}}' | $MCP_BIN 2>/dev/null | jq -r '.result.content[0].text' | jq .
echo ""

# Test 5: List accounts from BVN0 database
echo "Test 5: List first 5 accounts from 2025-07-13-bvn0"
echo '{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"accumulate_db_list_accounts","arguments":{"database":"2025-07-13-bvn0","limit":5}}}' | $MCP_BIN 2>/dev/null | jq -r '.result.content[0].text' | jq .
echo ""

echo "=== All Tests Complete ==="
