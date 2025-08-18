#!/bin/bash

# Bootstrap Server Configuration Checker
# This script helps diagnose issues with the bootstrap server configuration

echo "Bootstrap Server Configuration Checker"
echo "======================================"
echo ""

BOOTSTRAP_HOST="bootstrap.accumulate.defidevs.io"
BOOTSTRAP_PORT="16593"
EXPECTED_PEER_ID="12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg"

echo "Target Bootstrap Server: $BOOTSTRAP_HOST:$BOOTSTRAP_PORT"
echo "Expected Peer ID: $EXPECTED_PEER_ID"
echo ""

# Check DNS resolution
echo "1. Checking DNS resolution..."
DNS_IP=$(dig +short $BOOTSTRAP_HOST 2>/dev/null | head -1)
if [ -z "$DNS_IP" ]; then
    echo "   ❌ Failed to resolve $BOOTSTRAP_HOST"
    echo "   This indicates a DNS issue"
else
    echo "   ✓ Resolved to: $DNS_IP"
fi
echo ""

# Check TCP connectivity
echo "2. Checking TCP connectivity to port $BOOTSTRAP_PORT..."
if timeout 5 bash -c "echo > /dev/tcp/$BOOTSTRAP_HOST/$BOOTSTRAP_PORT" 2>/dev/null; then
    echo "   ✓ Port $BOOTSTRAP_PORT is open"
else
    echo "   ❌ Cannot connect to port $BOOTSTRAP_PORT"
    echo "   Possible issues:"
    echo "   - Bootstrap server is not running"
    echo "   - Firewall blocking port $BOOTSTRAP_PORT"
    echo "   - Wrong port configured"
fi
echo ""

# Check if it's an Accumulate node using the JSON-RPC API
echo "3. Checking if it's running Accumulate (via HTTP API)..."
HTTP_RESPONSE=$(curl -s -m 5 "http://$BOOTSTRAP_HOST:16595/v3" \
    -X POST \
    -H 'Content-Type: application/json' \
    -d '{"jsonrpc":"2.0","method":"node-info","params":{},"id":1}' 2>/dev/null)

if [ -n "$HTTP_RESPONSE" ]; then
    PEER_ID=$(echo "$HTTP_RESPONSE" | python3 -c "import sys, json; print(json.load(sys.stdin).get('result', {}).get('peerID', ''))" 2>/dev/null)
    if [ -n "$PEER_ID" ]; then
        echo "   ✓ Accumulate HTTP API is responding"
        echo "   Reported Peer ID: $PEER_ID"
        if [ "$PEER_ID" = "$EXPECTED_PEER_ID" ]; then
            echo "   ✓ Peer ID matches expected value"
        else
            echo "   ❌ Peer ID mismatch!"
            echo "   This means the bootstrap server has the wrong P2P key configured"
        fi
    else
        echo "   ⚠ HTTP API responded but couldn't parse peer ID"
    fi
else
    echo "   ⚠ HTTP API not responding (this might be normal for a bootstrap-only node)"
fi
echo ""

# Try to connect with the debug tool
echo "4. Testing P2P connectivity with debug tool..."
if [ -f "./debug" ]; then
    echo "   Running: ./debug test-p2p mainnet"
    timeout 10 ./debug test-p2p mainnet 2>&1 | grep -E "(Connected|Failed|Error|timeout)" | head -5
else
    echo "   ⚠ Debug tool not found. Build it with: go build -o debug ./tools/cmd/debug"
fi
echo ""

# Check what services are listening on the server (if we have SSH access)
echo "5. Configuration recommendations:"
echo ""
echo "If the bootstrap server is not working correctly:"
echo ""
echo "a) SSH into the bootstrap server (accumulate-p2p-bootstrap in us-east-2)"
echo ""
echo "b) Check if accumulated is running:"
echo "   sudo systemctl status accumulated"
echo "   # or"
echo "   ps aux | grep accumulated"
echo ""
echo "c) Check the configuration file:"
echo "   cat /etc/accumulate/accumulate.toml"
echo "   # Should match the bootstrap configuration in:"
echo "   # docs/configuration/bootstrap-server-accumulate.toml"
echo ""
echo "d) Check the logs:"
echo "   sudo journalctl -u accumulated -n 100"
echo "   # or"
echo "   tail -f /var/log/accumulated.log"
echo ""
echo "e) Verify the P2P key generates the correct peer ID:"
echo "   accumulated key export --key-type p2p"
echo ""
echo "f) Check network connectivity:"
echo "   sudo netstat -tlnp | grep 16593"
echo "   sudo iptables -L -n | grep 16593"
echo ""
echo "g) If needed, restart with correct configuration:"
echo "   sudo systemctl stop accumulated"
echo "   # Update /etc/accumulate/accumulate.toml"
echo "   sudo systemctl start accumulated"
echo ""
echo "======================================"
echo "Diagnostic complete"