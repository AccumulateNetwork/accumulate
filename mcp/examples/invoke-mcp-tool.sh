#!/bin/bash
# Example: How to invoke Accumulate MCP tools from bash scripts
#
# Usage: ./invoke-mcp-tool.sh <tool-name> <json-params>

set -e

MCP_SERVER="${MCP_SERVER:-./mcp-server}"
TOOL_NAME="$1"
PARAMS="$2"

if [ -z "$TOOL_NAME" ]; then
    echo "Usage: $0 <tool-name> <json-params>"
    echo ""
    echo "Examples:"
    echo "  $0 accumulate_get_genesis_files '{\"network\":\"mainnet\"}'"
    echo "  $0 accumulate_follower_status '{\"container_name\":\"accumulate-follower\"}'"
    exit 1
fi

# Build JSON-RPC request
REQUEST=$(cat <<EOF
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "$TOOL_NAME",
    "arguments": $PARAMS
  }
}
EOF
)

# Invoke MCP server via stdio
echo "$REQUEST" | "$MCP_SERVER" 2>&1

# Alternative: HTTP mode (if MCP server running in HTTP mode)
# curl -s -X POST http://localhost:8080 \
#   -H "Content-Type: application/json" \
#   -d "$REQUEST"
