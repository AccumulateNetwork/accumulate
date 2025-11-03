# Accumulate DevNet Launch and Testing Guide

This guide explains how to launch an Accumulate DevNet (local development network) and test the MCP-Accumulate server against it.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Launching DevNet](#launching-devnet)
3. [Testing MCP Server Against DevNet](#testing-mcp-server-against-devnet)
4. [Complete Testing Workflow](#complete-testing-workflow)
5. [DevNet Network Endpoints](#devnet-network-endpoints)
6. [Troubleshooting](#troubleshooting)

## Prerequisites

You need access to the Accumulate DevNet repository. The recommended location is:

```
/home/paul/go/src/gitlab.com/AccumulateNetwork/Devnet
```

If you don't have it, clone it:

```bash
git clone https://gitlab.com/AccumulateNetwork/Devnet
```

## Launching DevNet

### Method 1: Using Devnet CLI (Recommended)

The DevNet CLI tool provides the simplest way to run a local network.

#### Quick Start (3 Commands)

```bash
# Navigate to Devnet directory
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/Devnet

# Start DevNet
go run ./cmd/devnet start

# Wait for network to initialize (10-30 seconds)
sleep 30

# Verify it's running
curl http://127.0.0.1:26660/v3/describe
```

#### Default Configuration

The DevNet starts with:
- **2 BVNs** (Block Validation Networks)
- **3 validators** per BVN
- **1 follower** node
- Both v2 and v3 API endpoints enabled

#### Additional DevNet Commands

```bash
# Check network status
go run ./cmd/devnet status

# Stop DevNet
go run ./cmd/devnet stop

# Complete reset (deletes all data)
go run ./cmd/devnet reset

# Monitor network in real-time
go run ./cmd/devnet monitor

# View metrics
go run ./cmd/devnet monitor metrics

# Run load test (100 transactions)
go run ./cmd/devnet load --txs 100
```

### Method 2: Using Official `accumulated` Binary

If you prefer the official method:

```bash
# Navigate to Accumulate repository
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate

# Initialize DevNet (one-time setup)
go run ./cmd/accumulated init devnet -w .nodes -f 0 -v 1 -b 1 --no-empty-blocks --no-website --reset

# Run DevNet
go run ./cmd/accumulated run devnet -w .nodes
```

**Parameters:**
- `-w .nodes` - Working directory for data storage
- `-b 1` - Number of BVNs (Block Validation Networks)
- `-v 1` - Number of validators per BVN
- `-f 0` - Number of follower nodes
- `--no-empty-blocks` - Prevent empty blocks from being stored
- `--reset` - ⚠️ WARNING: Permanently deletes existing data

## DevNet Network Endpoints

DevNet provides multiple API endpoints. For MCP-Accumulate testing, use the **Directory Network** endpoint:

### Primary Endpoints (Recommended for MCP)

| Network | v3 API Endpoint | v2 API Endpoint (Legacy) |
|---------|-----------------|--------------------------|
| **Directory Network** | `http://127.0.0.1:26660/v3` | `http://127.0.0.1:26660/v2` |
| BVN0 | `http://127.0.0.1:26760/v3` | `http://127.0.0.1:26760/v2` |
| BVN1 | `http://127.0.0.1:26860/v3` | `http://127.0.0.1:26860/v2` |

**Use the Directory Network endpoint** (`http://127.0.0.1:26660/v3`) for all MCP tools.

### Port Scheme

The DevNet uses a predictable port scheme:
- **Directory Network:** 26660 (API), 26656 (P2P), 26657 (RPC)
- **BVN0:** 26760 (API), 26756 (P2P), 26757 (RPC)
- **BVN1:** 26860 (API), 26856 (P2P), 26857 (RPC)

## Testing MCP Server Against DevNet

The MCP-Accumulate server supports custom network endpoints. All tools accept a `network` parameter that can be:
- `"mainnet"` - Production network (default)
- `"testnet"` - Public test network
- `"http://127.0.0.1:26660/v3"` - Local DevNet (custom endpoint)

### Step 1: Build the MCP Server

```bash
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/mcp-accumulate
go build -o mcp-accumulate
```

### Step 2: Test with Direct MCP Tool Calls

You can test MCP tools directly via stdin using JSON-RPC:

```bash
# Test script provided in test_mcp.sh
# Modify it to use DevNet instead of testnet:

#!/bin/bash
MCP_SERVER="./mcp-accumulate"
NETWORK="http://127.0.0.1:26660/v3"

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

# Query the directory network account
call_tool "accumulate_query_account" "{\"url\": \"acc://dn.acme\", \"network\": \"$NETWORK\"}" | \
    $MCP_SERVER 2>&1 | head -50
```

### Step 3: Create Test Script for DevNet

Create a new file `test_devnet.sh`:

```bash
#!/bin/bash

# Test script for MCP-Accumulate against LOCAL DevNet
# This script tests all implemented MCP tools against your local network

set -e

MCP_SERVER="./mcp-accumulate"
NETWORK="http://127.0.0.1:26660/v3"

echo "=== MCP-Accumulate DevNet Test Suite ==="
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

# Test 1: Query directory network account
echo "=== Test 1: Query Directory Network Account ==="
call_tool "accumulate_query_account" "{\"url\": \"acc://dn.acme\", \"network\": \"$NETWORK\"}" | \
    $MCP_SERVER 2>&1 | head -50

echo ""
echo "=== Test 2: Network Status ==="
call_tool "accumulate_network_status" "{\"network\": \"$NETWORK\"}" | \
    $MCP_SERVER 2>&1 | head -50

echo ""
echo "=== Test 3: Node Info ==="
call_tool "accumulate_node_info" "{\"network\": \"$NETWORK\"}" | \
    $MCP_SERVER 2>&1 | head -50

echo ""
echo "=== Test 4: Create Lite Account ==="
call_tool "accumulate_create_lite_account" "{\"public_key\": \"0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef\"}" | \
    $MCP_SERVER 2>&1 | head -20

echo ""
echo "=== Tests Complete ==="
```

Make it executable:

```bash
chmod +x test_devnet.sh
```

## Complete Testing Workflow

Here's a complete end-to-end workflow for testing transactions against DevNet:

### Prerequisites

You'll need the DevNet repository for the quickstart demo that creates test accounts:

```bash
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/Devnet
```

### Workflow Steps

```bash
# Step 1: Start DevNet
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/Devnet
go run ./cmd/devnet start

# Step 2: Wait for network initialization
sleep 30

# Step 3: Run quickstart demo to create test accounts
# This creates lite accounts and gets test ACME from faucet
go run quickstart_demo.go

# The quickstart demo will output:
# - Public/private key pairs
# - Lite account addresses
# - Transaction hashes
# Save these for use with MCP tools

# Step 4: Test MCP server against DevNet
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/mcp-accumulate

# Query an account created by quickstart demo
./mcp-accumulate <<EOF
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "accumulate_query_account",
    "arguments": {
      "url": "acc://YOUR_LITE_ACCOUNT_HERE/ACME",
      "network": "http://127.0.0.1:26660/v3"
    }
  }
}
EOF

# Step 5: Send tokens between accounts
# Use the private key from quickstart demo
./mcp-accumulate <<EOF
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "accumulate_send_tokens",
    "arguments": {
      "from": "acc://SOURCE_ACCOUNT/ACME",
      "to": "acc://DESTINATION_ACCOUNT/ACME",
      "amount": "1.0",
      "private_key": "YOUR_PRIVATE_KEY_HERE",
      "network": "http://127.0.0.1:26660/v3"
    }
  }
}
EOF

# Step 6: Query the transaction
# Use the transaction hash from step 5
./mcp-accumulate <<EOF
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "accumulate_query_tx",
    "arguments": {
      "txid": "TRANSACTION_HASH_HERE",
      "network": "http://127.0.0.1:26660/v3"
    }
  }
}
EOF

# Step 7: Clean up when done
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/Devnet
go run ./cmd/devnet stop
```

### Example: Complete Transaction Test

Here's a concrete example using the quickstart demo output:

```bash
# 1. Start DevNet and create test accounts
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/Devnet
go run ./cmd/devnet start
sleep 30
go run quickstart_demo.go

# Example output from quickstart_demo.go:
# Account 1: acc://56e2a7be7aa1f6799c1c6276af85d48ab61cd6d3c07a8fad/ACME
# Public Key: a1b2c3d4e5f6...
# Private Key: 9876543210abcdef...

# 2. Query the account balance
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/mcp-accumulate
echo '{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "accumulate_query_account",
    "arguments": {
      "url": "acc://56e2a7be7aa1f6799c1c6276af85d48ab61cd6d3c07a8fad/ACME",
      "network": "http://127.0.0.1:26660/v3"
    }
  }
}' | ./mcp-accumulate

# 3. Send tokens (if you have two accounts from quickstart)
echo '{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "accumulate_send_tokens",
    "arguments": {
      "from": "acc://56e2a7be7aa1f6799c1c6276af85d48ab61cd6d3c07a8fad/ACME",
      "to": "acc://ANOTHER_ACCOUNT_HERE/ACME",
      "amount": "2.5",
      "private_key": "9876543210abcdef...",
      "network": "http://127.0.0.1:26660/v3"
    }
  }
}' | ./mcp-accumulate
```

## Using Claude Desktop with DevNet

To use Claude Desktop with your local DevNet, you can't directly configure it (since the MCP server doesn't store network preference). Instead, when using tools through Claude, always specify the network parameter:

**Example interaction:**

```
You: "Query account acc://dn.acme on my local DevNet"

Claude will call:
{
  "name": "accumulate_query_account",
  "arguments": {
    "url": "acc://dn.acme",
    "network": "http://127.0.0.1:26660/v3"
  }
}
```

**Important:** Make sure to tell Claude to use `"network": "http://127.0.0.1:26660/v3"` for all DevNet operations.

## Testing with ED25519 Key Pairs

If you need to create your own key pairs for testing:

### Option 1: Using Go Code

Create `generate_keys.go`:

```go
package main

import (
    "crypto/ed25519"
    "crypto/rand"
    "encoding/hex"
    "fmt"

    "gitlab.com/accumulatenetwork/accumulate/protocol"
)

func main() {
    // Generate ED25519 key pair
    pubKey, privKey, _ := ed25519.GenerateKey(rand.Reader)

    // Generate lite account URL
    liteUrl := protocol.LiteAuthorityForKey(pubKey, protocol.SignatureTypeED25519)

    fmt.Printf("Public Key:  %s\n", hex.EncodeToString(pubKey))
    fmt.Printf("Private Key: %s\n", hex.EncodeToString(privKey))
    fmt.Printf("Lite Account: %s/ACME\n", liteUrl.String())
}
```

Run it:

```bash
go run generate_keys.go
```

### Option 2: Use the MCP Tool

```bash
# Generate a key pair using your own method, then:
echo '{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "accumulate_create_lite_account",
    "arguments": {
      "public_key": "YOUR_PUBLIC_KEY_HERE"
    }
  }
}' | ./mcp-accumulate
```

## Amount Conversion Reference

All amounts in Accumulate use **precision units**: 1 ACME = 100,000,000 units

When using MCP tools, specify amounts as strings in ACME:

| ACME | String Value | Precision Units |
|------|--------------|-----------------|
| 0.001 | `"0.001"` | 100,000 |
| 0.01 | `"0.01"` | 1,000,000 |
| 1.0 | `"1.0"` | 100,000,000 |
| 10.0 | `"10.0"` | 1,000,000,000 |
| 100.0 | `"100.0"` | 10,000,000,000 |

The MCP server automatically converts string amounts to precision units (see `server/tools.go:145`):

```go
amount, err := strconv.ParseFloat(amountStr, 64)
amountInCredits := int64(amount * 1e8)
```

## Troubleshooting

### DevNet Won't Start

```bash
# Check for port conflicts
lsof -i :26660
lsof -i :26760
lsof -i :26860

# Kill any old processes
pkill -f accumulated

# Reset DevNet completely
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/Devnet
go run ./cmd/devnet reset
```

### API Not Responding

```bash
# DevNet may take up to 30 seconds to fully initialize
sleep 30

# Check status
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/Devnet
go run ./cmd/devnet status

# Verify endpoints are accessible
curl http://127.0.0.1:26660/v3/describe
```

### MCP Tool Returns Error

1. **Check network is running:**
   ```bash
   curl http://127.0.0.1:26660/v3/describe
   ```

2. **Verify network parameter:**
   - Must be exact: `"http://127.0.0.1:26660/v3"`
   - Not: `"devnet"` or `"local"`

3. **Check account exists:**
   - Lite accounts are created automatically on first transaction
   - Use faucet from quickstart demo to fund accounts

### Transaction Fails

Common issues:

1. **Insufficient balance:** Use the faucet to add test ACME
   ```bash
   cd /home/paul/go/src/gitlab.com/AccumulateNetwork/Devnet
   go run quickstart_demo.go
   ```

2. **Invalid private key:** Must be 64 bytes (128 hex characters)

3. **Wrong account URL:** Lite accounts must match format:
   ```
   acc://[64-char-hex]/ACME
   ```

## Key Files and Locations

| Component | Location | Purpose |
|-----------|----------|---------|
| DevNet CLI | `/home/paul/go/src/gitlab.com/AccumulateNetwork/Devnet/cmd/devnet` | DevNet management tool |
| Quickstart Demo | `/home/paul/go/src/gitlab.com/AccumulateNetwork/Devnet/quickstart_demo.go` | Create test accounts |
| MCP Server | `/home/paul/go/src/gitlab.com/AccumulateNetwork/mcp-accumulate` | MCP-Accumulate server |
| MCP Binary | `/home/paul/go/src/gitlab.com/AccumulateNetwork/mcp-accumulate/mcp-accumulate` | Built MCP server |
| Test Script | `/home/paul/go/src/gitlab.com/AccumulateNetwork/mcp-accumulate/test_mcp.sh` | Testnet test script |
| Client Code | `/home/paul/go/src/gitlab.com/AccumulateNetwork/mcp-accumulate/client/client.go` | Network endpoint handling |

## Network Configuration in Code

The MCP server supports custom endpoints via the client (see `client/client.go:42`):

```go
func getEndpoint(network string) string {
    switch network {
    case "mainnet", "":
        return MainnetEndpoint
    case "testnet":
        return TestnetEndpoint
    default:
        return network  // Any custom URL is passed through
    }
}
```

This means you can use **any valid RPC endpoint URL** as the network parameter.

## Official Documentation

- **Accumulate DevNet Docs:** https://docs.accumulatenetwork.io/accumulate/setup/local-devnet
- **Accumulate SDK:** v1.4.2
- **Main Repository:** https://gitlab.com/accumulatenetwork/accumulate
- **DevNet Repository:** https://gitlab.com/AccumulateNetwork/Devnet

## Quick Reference Card

```bash
# Start DevNet
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/Devnet
go run ./cmd/devnet start && sleep 30

# Create test accounts
go run quickstart_demo.go

# Test MCP query
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/mcp-accumulate
echo '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"accumulate_query_account","arguments":{"url":"acc://dn.acme","network":"http://127.0.0.1:26660/v3"}}}' | ./mcp-accumulate

# Stop DevNet
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/Devnet
go run ./cmd/devnet stop
```

## Summary

This guide provides everything needed to:
1. ✅ Launch a local Accumulate DevNet
2. ✅ Configure MCP-Accumulate to use DevNet
3. ✅ Test all MCP tools against your local network
4. ✅ Create test accounts and transactions
5. ✅ Troubleshoot common issues

The DevNet provides a complete local blockchain environment for development and testing without requiring mainnet or testnet access.
