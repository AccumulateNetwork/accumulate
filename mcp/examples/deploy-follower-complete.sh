#!/bin/bash
# Complete Follower Deployment Example using Accumulate MCP
#
# This script shows end-to-end deployment from database snapshots to running follower
# using the Accumulate MCP tools.

set -e

# Configuration
DN_DATABASE="${DN_DATABASE:-/media/paul/Expansion/databases/2025-10-13-dn}"
BVN_DATABASE="${BVN_DATABASE:-/media/paul/Expansion/databases/2025-10-13-bvn}"
WORK_DIR="${WORK_DIR:-/var/lib/accumulate-follower}"
CONTAINER_NAME="${CONTAINER_NAME:-accumulate-follower}"
NETWORK="${NETWORK:-MainNet}"
MCP_SERVER="${MCP_SERVER:-./mcp-server}"

echo "=== Accumulate Follower Deployment ==="
echo ""
echo "Configuration:"
echo "  DN Database:    $DN_DATABASE"
echo "  BVN Database:   $BVN_DATABASE"
echo "  Work Directory: $WORK_DIR"
echo "  Container:      $CONTAINER_NAME"
echo "  Network:        $NETWORK"
echo ""

# Helper function to call MCP tools
call_mcp_tool() {
    local tool_name="$1"
    local params="$2"

    local request=$(cat <<EOF
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "$tool_name",
    "arguments": $params
  }
}
EOF
    )

    echo "$request" | "$MCP_SERVER"
}

# Step 1: Verify prerequisites
echo "Step 1: Verifying prerequisites..."
test -d "$DN_DATABASE" || { echo "ERROR: DN database not found at $DN_DATABASE"; exit 1; }
test -d "$BVN_DATABASE" || { echo "ERROR: BVN database not found at $BVN_DATABASE"; exit 1; }
test -d "$DN_DATABASE/data/accumulate.db" || { echo "ERROR: DN database missing accumulate.db"; exit 1; }
test -d "$BVN_DATABASE/data/accumulate.db" || { echo "ERROR: BVN database missing accumulate.db"; exit 1; }
echo "✓ Database snapshots verified"
echo ""

# Step 2: Check for genesis files
echo "Step 2: Checking for genesis files..."
GENESIS_INFO=$(call_mcp_tool "accumulate_get_genesis_files" '{"network":"mainnet","bvn":"1"}')
echo "$GENESIS_INFO" | jq '.'

DN_GENESIS=$(echo "$GENESIS_INFO" | jq -r '.dn_genesis_snap')
BVN_GENESIS=$(echo "$GENESIS_INFO" | jq -r '.bvn_genesis_snap')

if [ -f "$DN_GENESIS" ] && [ -f "$BVN_GENESIS" ]; then
    echo "✓ Genesis files found"
    GENESIS_PARAMS=", \"dn_genesis_snap\": \"$DN_GENESIS\", \"bvn_genesis_snap\": \"$BVN_GENESIS\""
else
    echo "⚠ Genesis files not found (proceeding without them)"
    GENESIS_PARAMS=""
fi
echo ""

# Step 3: Initialize follower
echo "Step 3: Initializing follower..."
INIT_PARAMS=$(cat <<EOF
{
  "dn_database": "$DN_DATABASE",
  "bvn_database": "$BVN_DATABASE",
  "work_dir": "$WORK_DIR",
  "container_name": "$CONTAINER_NAME",
  "network": "$NETWORK"$GENESIS_PARAMS
}
EOF
)

INIT_RESULT=$(call_mcp_tool "accumulate_init_follower" "$INIT_PARAMS")
echo "$INIT_RESULT" | jq '.'

if echo "$INIT_RESULT" | jq -e '.status == "initialized"' > /dev/null; then
    echo "✓ Follower initialized successfully"
else
    echo "✗ Initialization failed"
    exit 1
fi
echo ""

# Step 4: Run follower in Docker
echo "Step 4: Starting follower in Docker..."
RUN_PARAMS=$(cat <<EOF
{
  "work_dir": "$WORK_DIR",
  "container_name": "$CONTAINER_NAME"
}
EOF
)

RUN_RESULT=$(call_mcp_tool "accumulate_run_follower" "$RUN_PARAMS")
echo "$RUN_RESULT" | jq '.'

if echo "$RUN_RESULT" | jq -e '.status == "started"' > /dev/null; then
    echo "✓ Follower started successfully"
else
    echo "✗ Failed to start follower"
    exit 1
fi
echo ""

# Step 5: Verify follower is running
echo "Step 5: Verifying follower status..."
sleep 3  # Give container time to start

STATUS_PARAMS='{"container_name":"'$CONTAINER_NAME'"}'
STATUS_RESULT=$(call_mcp_tool "accumulate_follower_status" "$STATUS_PARAMS")
echo "$STATUS_RESULT" | jq '.'

if echo "$STATUS_RESULT" | jq -e '.running == true' > /dev/null; then
    echo "✓ Follower is running"
else
    echo "✗ Follower is not running"
    docker logs "$CONTAINER_NAME"
    exit 1
fi
echo ""

# Step 6: Check logs
echo "Step 6: Recent follower logs:"
docker logs --tail 20 "$CONTAINER_NAME"
echo ""

echo "=== Deployment Complete! ==="
echo ""
echo "Container: $CONTAINER_NAME"
echo "Status: docker ps | grep $CONTAINER_NAME"
echo "Logs: docker logs -f $CONTAINER_NAME"
echo "DN API: http://localhost:16591"
echo "BVN API: http://localhost:16691"
echo ""
echo "To check sync status:"
echo "  curl http://localhost:16592/status | jq '.result.sync_info'"
