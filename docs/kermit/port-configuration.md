# Kermit Testnet Port Configuration Reference

**Date**: 2025-08-05  
**Status**: ✅ **VERIFIED AND DOCUMENTED**  
**Purpose**: Definitive port configuration reference for Kermit testnet infrastructure

## Overview

This document provides the complete port configuration specification for the Kermit testnet, derived from actual source code analysis and verified against running nodes.

## Port Calculation Formula

All Accumulate node ports are calculated using the following formula:

```
Final Port = BasePort + ServiceOffset + PartitionOffset
```

## Port Offset Constants

### Service Port Offsets

From `/internal/node/config/enums_gen.go`:

```go
const PortOffsetTendermintP2P = 0      // Tendermint P2P networking
const PortOffsetTendermintRpc = 1      // Tendermint RPC interface  
const PortOffsetAccumulateP2P = 2      // Accumulate P2P networking
const PortOffsetPrometheus = 3         // Prometheus metrics endpoint
const PortOffsetAccumulateApi = 4      // Accumulate v3 JSON-RPC API
```

### Partition Type Offsets

From `/internal/node/config/config.go`:

```go
const PortOffsetDirectory = 0          // Directory Network (DN)
const PortOffsetBlockValidator = 100   // Block Validator Network (BVN)  
const PortOffsetBlockSummary = 200     // Block Summary Network (BSN)
```

## Kermit Testnet Configuration

### Base Configuration

From `/docs/configuration/accumulate-kermit.toml`:
- **Network Name**: "Kermit"
- **Base Port**: 16591 (BVN Chico listen port)

### BVN Validator Ports

All BVN validators use the same port configuration:

| Service | Calculation | Port | Status |
|---------|-------------|------|--------|
| **Accumulate v3 API** | 16591 + 4 + 100 | **16695** | ✅ **PRIMARY API** |
| **Tendermint RPC** | 16591 + 1 + 100 | **16692** | ✅ Verified |
| **Accumulate P2P** | 16591 + 2 + 100 | **16693** | ✅ Network sync |
| **Tendermint P2P** | 16591 + 0 + 100 | **16691** | ✅ Consensus |
| **Prometheus** | 16591 + 3 + 100 | **16694** | ✅ Metrics |

### Verified Node Endpoints

| Node | IP Address | API Port | RPC Port | Status |
|------|------------|----------|----------|--------|
| **Chico (BVN0)** | 18.232.151.41 | 16695 | 16692 | ✅ Healthy |
| **Harpo (BVN1)** | 52.91.59.159 | 16695 | 16692 | ✅ Healthy |
| **Groucho (BVN2)** | 54.226.145.213 | 16695 | 16692 | ✅ Healthy |

## Code References

### Port Configuration Logic

1. **Address Builder**: `/internal/node/daemon/address.go`
   - `AddressBuilder.String()` - Implements port calculation formula
   - `ConfigureNodePorts()` - Applies port configuration to node config

2. **Port Constants**: `/internal/node/config/config.go` and `enums_gen.go`
   - Service and partition offset definitions
   - Port calculation constants

3. **Node Configuration**: `/internal/node/daemon/init.go`
   - `ConfigureNodePorts()` function applies offsets to node services

### Configuration Files

1. **Network Config**: `/docs/configuration/accumulate-kermit.toml`
   - Base port and network settings
   - BVN partition definitions

2. **API Endpoints**: `/pkg/accumulate/api.go`
   - Well-known network endpoints
   - Kermit API server default: `http://kermit-api.accumulate.defidevs.io:16692`

## Bootstrap Server Port Configuration

The Kermit testnet bootstrap server is configured as a **gateway node** that provides network discovery and peer connectivity services.

### Bootstrap Server Ports

| Service | Port | Protocol | Purpose |
|---------|------|----------|----------|
| **Gateway/P2P** | **16591** | TCP | Primary bootstrap and P2P discovery |
| **Gateway/P2P** | **16591** | QUIC | Alternative P2P transport |
| **HTTP API** | **26660** | HTTP | Bootstrap node API (if enabled) |

### Bootstrap Configuration

The bootstrap server uses a **gateway** configuration type instead of the standard validator configuration:

```toml
# bootstrap-kermit.toml
network = "Kermit"

# Gateway configuration for bootstrap node
[[configurations]]
  type = "gateway"
  listen = "/ip4/0.0.0.0/tcp/16591"

[p2p]
  listen = [
    "/ip4/0.0.0.0/tcp/16591",
    "/ip4/0.0.0.0/tcp/16591/quic"
  ]
  discovery-mode = "server"
```

### Bootstrap Peer Addresses

BVN nodes connect to bootstrap servers using these peer addresses:

```toml
dn-bootstrap-peers = [
  "/dns/kermit-bvn1.accumulate.defidevs.io/tcp/16591/p2p/12D3KooWSsiT3rtjGJhtu68emgp7zPJyH9MeFkmxtCdCK6T1Nvxd",
  "/dns/kermit-bvn2.accumulate.defidevs.io/tcp/16591/p2p/12D3KooWMqpLym3XSy3zQRRy2xudFTjjstoX97ZaW6pvLTUgwcYg"
]
```

**Key Differences from BVN Validators:**
- Bootstrap servers use **base port 16591** directly (no partition offset)
- Gateway type configuration instead of validator configuration
- P2P discovery mode set to "server" for peer discovery
- Supports both TCP and QUIC protocols on the same port

## Verification Commands

### Test BVN API Connectivity

```bash
# Test Accumulate v3 API (PRIMARY)
curl -X POST -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"node-info","params":{},"id":1}' \
  http://18.232.151.41:16695/v3

# Expected Response:
# {"result":{"network":"Kermit","version":"v1.4.1",...}}
```

### Test Tendermint RPC

```bash
# Test Tendermint RPC interface
curl http://54.226.145.213:16692/status

# Expected Response:
# {"jsonrpc":"2.0","id":"","result":{"node_info":{"version":"0.38.0-rc3",...}}}
```

### Verify All BVN Nodes

```bash
# Script to verify all BVN endpoints
for ip in 18.232.151.41 52.91.59.159 54.226.145.213; do
  echo "Testing $ip:16695..."
  curl -s -X POST -H "Content-Type: application/json" \
    -d '{"jsonrpc":"2.0","method":"node-info","params":{},"id":1}' \
    http://$ip:16695/v3 | jq -r '.result.network + " " + .result.version'
done
```

### Test Bootstrap Server Connectivity

```bash
# Test bootstrap server P2P ports
for server in kermit-bvn1.accumulate.defidevs.io kermit-bvn2.accumulate.defidevs.io; do
  echo "Testing bootstrap server $server:16591..."
  nc -zv $server 16591 2>&1 | grep -q "succeeded" && echo "✅ $server:16591 accessible" || echo "❌ $server:16591 not accessible"
done
```

### Verify Bootstrap Peer Connectivity

```bash
# Test if bootstrap servers are reachable via P2P
# This tests the actual peer addresses used by BVN nodes
echo "Bootstrap peer addresses from configuration:"
echo "  /dns/kermit-bvn1.accumulate.defidevs.io/tcp/16591/p2p/12D3KooWSsiT3rtjGJhtu68emgp7zPJyH9MeFkmxtCdCK6T1Nvxd"
echo "  /dns/kermit-bvn2.accumulate.defidevs.io/tcp/16591/p2p/12D3KooWMqpLym3XSy3zQRRy2xudFTjjstoX97ZaW6pvLTUgwcYg"
```

## Port Mismatch Resolution

### Root Cause

The **API server expects BVN nodes on port 16692** (standard), but the **Accumulate v3 API actually runs on port 16695** (correct per code).

### Solution

Update API server configuration to connect to BVN nodes on port **16695**:

```toml
# API server configuration update needed
[accumulate.api]
bvn-endpoints = [
  "http://18.232.151.41:16695",  # Chico
  "http://52.91.59.159:16695",   # Harpo  
  "http://54.226.145.213:16695"  # Groucho
]
```

## Testing and Validation

### Unit Tests

Port configuration logic is tested in:
- `/internal/node/config/config_test.go`
- Address builder tests in daemon package

### Integration Tests

Network connectivity tests verify:
- P2P port accessibility (16693)
- API endpoint responses (16695)
- Tendermint RPC functionality (16692)

### Monitoring

Prometheus metrics available on port 16694:
- Node health and consensus status
- Network connectivity metrics
- API request statistics

## Troubleshooting

### Common Issues

1. **"Connection refused" on port 16692**
   - **Cause**: Expecting Accumulate API on Tendermint RPC port
   - **Solution**: Use port 16695 for Accumulate v3 API

2. **"No live peers" errors**
   - **Cause**: API server connecting to wrong ports
   - **Solution**: Update BVN endpoint configuration to use 16695

3. **Version reporting as "unknown"**
   - **Cause**: Querying Tendermint RPC instead of Accumulate API
   - **Solution**: Query node-info on port 16695, not 16692

### Diagnostic Commands

```bash
# Check port accessibility
nc -zv 18.232.151.41 16695  # Should succeed
nc -zv 18.232.151.41 16692  # Should succeed (Tendermint RPC)

# Verify service types
curl -s http://18.232.151.41:16692/status | jq .result.node_info.version  # Tendermint
curl -s -X POST -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"node-info","params":{},"id":1}' \
  http://18.232.151.41:16695/v3 | jq .result.version  # Accumulate
```

## References

- **Source Code**: GitLab AccumulateNetwork/accumulate
- **Configuration**: `/docs/configuration/accumulate-kermit.toml`
- **Address Logic**: `/internal/node/daemon/address.go`
- **Port Constants**: `/internal/node/config/config.go`
- **Network Status**: Kermit testnet documentation in `/docs/kermit/`

---

**Last Updated**: 2025-08-05 02:59 CDT  
**Verified Against**: Accumulate v1.4.1, Kermit testnet live nodes
