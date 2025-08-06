# Kermit Bootstrap and Faucet Services Fix

## Overview

This document provides the complete solution for fixing the Kermit testnet's Bootstrap server, API server, and Faucet services based on the official configuration and addressing the identified technical issues.

## Root Cause Analysis

### 1. API Listen Address Conflict
- **Issue**: The new configuration system (cmd/accumulated/run) automatically creates HTTP services, but the old daemon system still expects a valid API.ListenAddress
- **Error**: "unsupported scheme" when listenHttpUrl() tries to parse an empty string
- **Solution**: Add valid HTTP listen address to satisfy both systems

### 2. Bootstrap Peer Connectivity
- **Issue**: Incorrect or unreachable bootstrap peers prevent P2P network formation
- **Solution**: Verify and update bootstrap peers from official Kermit configuration

### 3. Faucet Service Dependencies
- **Issue**: Missing or incorrectly configured faucet account, signing key, and P2P settings
- **Solution**: Proper faucet service initialization with all required parameters

## Configuration Files

### 1. Updated Kermit Configuration (accumulate-kermit.toml)

```toml
network = "Kermit"

# Core validator configuration for Chico BVN
[[configurations]]
  bvn = "Chico"
  type = "coreValidator"
  listen = "/ip4/0.0.0.0/tcp/16591"
  
  # Genesis snapshots
  bvn-genesis = "chico-genesis.snap"
  dn-genesis = "directory-genesis.snap"
  
  # Bootstrap peers from official Kermit network
  dn-bootstrap-peers = [
    "/dns/kermit-dn.accumulatenetwork.io/tcp/16593/p2p/12D3KooWMqpLym3XSy3zQRRy2xudFTjjstoX97ZaW6pvLTUgwcYg",
    "/dns/kermit-bvn1.accumulate.defidevs.io/tcp/16591/p2p/12D3KooWSsiT3rtjGJhtu68emgp7zPJyH9MeFkmxtCdCK6T1Nvxd"
  ]
  bvn-bootstrap-peers = [
    "/dns/kermit-bvn.accumulatenetwork.io/tcp/16593/p2p/12D3KooWMqpLym3XSy3zQRRy2xudFTjjstoX97ZaW6pvLTUgwcYg",
    "/dns/kermit-bvn2.accumulate.defidevs.io/tcp/16591/p2p/12D3KooWMqpLym3XSy3zQRRy2xudFTjjstoX97ZaW6pvLTUgwcYg"
  ]
  
  [configurations.validator-key]
    address = "AS1xm6dh2xw6UUGTXW6qHvtTXJLB9ixkbqqL9wn3ma3rDzEu1JE6"
    type = "raw"

# P2P configuration
[p2p]
  listen = ["/ip4/0.0.0.0/tcp/16591"]
  bootstrap-peers = [
    "/dns/kermit-dn.accumulatenetwork.io/tcp/16593/p2p/12D3KooWMqpLym3XSy3zQRRy2xudFTjjstoX97ZaW6pvLTUgwcYg"
  ]
  
  [p2p.key]
    address = "AS12C5KYzB1oerP6eRyYvELpLzn5JxWRgD131EK3XnFaunhEHvGET"
    type = "raw"

# Logging configuration
[logging]
  level = "info"
  format = "plain"

# Services configuration
[[services]]
  type = "http"
  listen = ["/ip4/0.0.0.0/tcp/26660/http"]
  router = ""

[[services]]
  type = "faucet"
  account = "acc://lite-token-account-from-faucet-key"
  router = ""
  
  [services.signing-key]
    type = "raw"
    # This will be set during initialization
```

### 2. Bootstrap Service Configuration

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
  
  [p2p.key]
    type = "generate"

# HTTP API service
[[services]]
  type = "http"
  listen = ["/ip4/0.0.0.0/tcp/26660/http"]
  router = ""

# Router service
[[services]]
  type = "router"

# Faucet service
[[services]]
  type = "faucet"
  account = ""  # Will be generated
  router = ""
  
  [services.signing-key]
    type = "generate"

[logging]
  level = "info"
  format = "plain"
```

## Service Startup Scripts

### 1. Bootstrap Server Startup

```bash
#!/bin/bash
# start-kermit-bootstrap.sh

set -e

WORK_DIR="${WORK_DIR:-$PWD/kermit-bootstrap}"
CONFIG_FILE="${CONFIG_FILE:-bootstrap-kermit.toml}"
NETWORK="Kermit"

echo "Starting Kermit Bootstrap Server..."
echo "Work Directory: $WORK_DIR"
echo "Config File: $CONFIG_FILE"

# Create work directory
mkdir -p "$WORK_DIR"

# Generate node key if it doesn't exist
if [ ! -f "$WORK_DIR/node-key.json" ]; then
    echo "Generating node key..."
    ./accumulated key generate --type ed25519 --output "$WORK_DIR/node-key.json"
fi

# Generate faucet key if it doesn't exist
if [ ! -f "$WORK_DIR/faucet-key.json" ]; then
    echo "Generating faucet key..."
    ./accumulated key generate --type ed25519 --output "$WORK_DIR/faucet-key.json"
    
    # Create faucet account URL from key
    FAUCET_ACCOUNT=$(./accumulated key export --file "$WORK_DIR/faucet-key.json" --format lite-token-account)
    echo "Faucet Account: $FAUCET_ACCOUNT"
    echo "$FAUCET_ACCOUNT" > "$WORK_DIR/faucet-account.txt"
fi

# Start bootstrap server
echo "Starting bootstrap server..."
./accumulated run \
    --config "$CONFIG_FILE" \
    --work-dir "$WORK_DIR" \
    --log-level info \
    "$NETWORK"
```

### 2. Core Validator Startup

```bash
#!/bin/bash
# start-kermit-validator.sh

set -e

WORK_DIR="${WORK_DIR:-$PWD/kermit-validator}"
CONFIG_FILE="${CONFIG_FILE:-accumulate-kermit.toml}"
NETWORK="Kermit"

echo "Starting Kermit Core Validator..."
echo "Work Directory: $WORK_DIR"
echo "Config File: $CONFIG_FILE"

# Create work directory structure
mkdir -p "$WORK_DIR"/{dnn,bvnn}/{config,data}

# Initialize dual node if not already done
if [ ! -f "$WORK_DIR/dnn/config/accumulate.toml" ]; then
    echo "Initializing dual node..."
    ./accumulated init dual \
        --work-dir "$WORK_DIR" \
        --config "$CONFIG_FILE" \
        "Kermit.Directory" "Kermit.Chico"
fi

# Restore snapshots if they exist and databases are empty
if [ -f "directory-genesis.snap" ] && [ ! -d "$WORK_DIR/dnn/data/accumulate.db" ]; then
    echo "Restoring Directory Network snapshot..."
    ./accumulated restore-snapshot \
        --work-dir "$WORK_DIR/dnn" \
        "directory-genesis.snap"
fi

if [ -f "chico-genesis.snap" ] && [ ! -d "$WORK_DIR/bvnn/data/accumulate.db" ]; then
    echo "Restoring BVN snapshot..."
    ./accumulated restore-snapshot \
        --work-dir "$WORK_DIR/bvnn" \
        "chico-genesis.snap"
fi

# Fix API listen address in both configurations
echo "Fixing API listen addresses..."
for node_dir in dnn bvnn; do
    config_file="$WORK_DIR/$node_dir/config/accumulate.toml"
    if [ -f "$config_file" ]; then
        # Add API listen address if missing
        if ! grep -q "listen-address.*http" "$config_file"; then
            echo "" >> "$config_file"
            echo "[api]" >> "$config_file"
            echo 'listen-address = "http://0.0.0.0:26660"' >> "$config_file"
        fi
    fi
done

# Start the validator
echo "Starting core validator..."
./accumulated run \
    --work-dir "$WORK_DIR" \
    --log-level info \
    "$NETWORK"
```

### 3. Standalone Faucet Service

```bash
#!/bin/bash
# start-kermit-faucet.sh

set -e

WORK_DIR="${WORK_DIR:-$PWD/kermit-faucet}"
NETWORK="Kermit"
BOOTSTRAP_PEERS="${BOOTSTRAP_PEERS:-/dns/kermit-dn.accumulatenetwork.io/tcp/16593/p2p/12D3KooWMqpLym3XSy3zQRRy2xudFTjjstoX97ZaW6pvLTUgwcYg}"

echo "Starting Kermit Faucet Service..."
echo "Work Directory: $WORK_DIR"

# Create work directory
mkdir -p "$WORK_DIR"

# Generate keys if they don't exist
if [ ! -f "$WORK_DIR/node-key.json" ]; then
    echo "Generating node key..."
    ./accumulated key generate --type ed25519 --output "$WORK_DIR/node-key.json"
fi

if [ ! -f "$WORK_DIR/faucet-key.json" ]; then
    echo "Generating faucet key..."
    ./accumulated key generate --type ed25519 --output "$WORK_DIR/faucet-key.json"
fi

# Get faucet account URL
FAUCET_ACCOUNT=$(./accumulated key export --file "$WORK_DIR/faucet-key.json" --format lite-token-account)
echo "Faucet Account: $FAUCET_ACCOUNT"

# Start faucet service
echo "Starting faucet service..."
./accumulated-faucet \
    --node-key "$WORK_DIR/node-key.json" \
    --key "$WORK_DIR/faucet-key.json" \
    --account "$FAUCET_ACCOUNT" \
    --listen "/ip4/0.0.0.0/tcp/16591" \
    --peer "$BOOTSTRAP_PEERS" \
    --peer-db "$WORK_DIR/peerdb.json" \
    --log-level info \
    "$NETWORK"
```

## Validation and Testing

### 1. Service Health Checks

```bash
#!/bin/bash
# validate-kermit-services.sh

echo "=== Kermit Services Validation ==="

# Check bootstrap server
echo "1. Testing Bootstrap Server..."
curl -s "http://localhost:26660/v3/status" | jq '.data.network' || echo "Bootstrap API not responding"

# Check P2P connectivity
echo "2. Testing P2P Connectivity..."
curl -s "http://localhost:26660/v3/network/status" | jq '.data.peers' || echo "P2P status not available"

# Check faucet service
echo "3. Testing Faucet Service..."
if [ -f "kermit-faucet/faucet-account.txt" ]; then
    FAUCET_ACCOUNT=$(cat kermit-faucet/faucet-account.txt)
    curl -s "http://localhost:26660/v3/query/$FAUCET_ACCOUNT" | jq '.data.balance' || echo "Faucet account not found"
fi

# Check validator status
echo "4. Testing Validator Status..."
curl -s "http://localhost:26660/v3/metrics" | grep -E "(consensus|validator)" || echo "Validator metrics not available"

echo "=== Validation Complete ==="
```

### 2. Network Connectivity Test

```bash
#!/bin/bash
# test-kermit-network.sh

echo "=== Kermit Network Connectivity Test ==="

# Test official Kermit endpoints
ENDPOINTS=(
    "https://kermit.accumulatenetwork.io/v2"
    "https://kermit-dn.accumulatenetwork.io/v2"
    "https://kermit-bvn.accumulatenetwork.io/v2"
)

for endpoint in "${ENDPOINTS[@]}"; do
    echo "Testing $endpoint..."
    curl -s --max-time 10 "$endpoint/status" | jq '.data.network' || echo "  ❌ Failed to connect"
done

# Test P2P bootstrap peers
PEERS=(
    "kermit-dn.accumulatenetwork.io:16593"
    "kermit-bvn.accumulatenetwork.io:16593"
    "kermit-bvn1.accumulate.defidevs.io:16591"
    "kermit-bvn2.accumulate.defidevs.io:16591"
)

for peer in "${PEERS[@]}"; do
    echo "Testing P2P peer $peer..."
    timeout 5 nc -z ${peer/:/ } && echo "  ✅ Reachable" || echo "  ❌ Unreachable"
done

echo "=== Network Test Complete ==="
```

## Troubleshooting Guide

### Common Issues and Solutions

#### 1. "unsupported scheme" Error
**Cause**: Missing API listen address in configuration
**Solution**: Add `[api]` section with `listen-address = "http://0.0.0.0:26660"`

#### 2. P2P Connection Failures
**Cause**: Incorrect bootstrap peers or firewall issues
**Solution**: 
- Verify bootstrap peers are reachable
- Check firewall allows TCP connections on P2P ports
- Ensure node key is properly generated

#### 3. Faucet Account Not Found
**Cause**: Faucet account not properly funded or created
**Solution**:
- Verify faucet key generation
- Check account URL format
- Ensure network connectivity to bootstrap nodes

#### 4. Snapshot Restoration Failures
**Cause**: Partition-specific restoration required
**Solution**:
- Use separate work directories for DN and BVN
- Run restore-snapshot for each partition individually
- Verify snapshot file integrity

### Log Analysis

Key log patterns to monitor:

```bash
# Successful startup
grep "Ready.*service" logs/accumulated.log

# P2P connectivity
grep "peer.*connected" logs/accumulated.log

# API service status
grep "Listening.*http" logs/accumulated.log

# Faucet transactions
grep "faucet.*transaction" logs/accumulated.log
```

## Deployment Checklist

- [ ] Download latest Kermit genesis snapshots
- [ ] Verify bootstrap peer connectivity
- [ ] Generate node and faucet keys
- [ ] Configure API listen addresses
- [ ] Set proper file permissions (600 for keys)
- [ ] Test P2P connectivity
- [ ] Validate API endpoints
- [ ] Monitor service logs
- [ ] Verify faucet functionality
- [ ] Set up monitoring and alerts

## Security Considerations

1. **Key Management**: Store private keys securely with 600 permissions
2. **Network Security**: Use firewall rules to restrict access
3. **API Security**: Consider rate limiting and authentication
4. **Monitoring**: Set up log monitoring for security events
5. **Updates**: Keep software updated with latest security patches

## Maintenance Procedures

### Regular Health Checks
- Monitor disk usage and performance
- Check P2P peer connectivity
- Verify API response times
- Monitor faucet balance and transactions

### Backup Procedures
- Regular database backups
- Key file backups (encrypted)
- Configuration file versioning
- Snapshot archival

### Update Procedures
- Test updates in staging environment
- Coordinate with network for consensus updates
- Maintain rollback capability
- Update monitoring and alerting

## Port Configuration Reference

**📋 [Complete Port Documentation](port-configuration.md)**

The port configuration issues identified in this document are fully analyzed in the dedicated port configuration reference:

### Key Port Findings
- **BVN Accumulate API**: Port **16695** (not 16692 as expected)
- **BVN Tendermint RPC**: Port **16692** (correct)
- **Bootstrap Server Gateway/P2P**: Port **16591** (TCP and QUIC)
- **Bootstrap Server HTTP API**: Port **26660** (if enabled)
- **Root Cause**: API server expects BVN nodes on 16692, but Accumulate API runs on 16695

### Code References
- **Port calculation logic**: `/internal/node/daemon/address.go`
- **Port offset constants**: `/internal/node/config/config.go` and `enums_gen.go`
- **Configuration application**: `/internal/node/daemon/init.go`

### Verification Commands
```bash
# Test correct BVN API port
curl -X POST -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"node-info","params":{},"id":1}' \
  http://18.232.151.41:16695/v3

# Verify Tendermint RPC
curl http://18.232.151.41:16692/status

# Test bootstrap server connectivity
nc -zv kermit-bvn1.accumulate.defidevs.io 16591
nc -zv kermit-bvn2.accumulate.defidevs.io 16591
```

See [port-configuration.md](port-configuration.md) for complete analysis, formulas, and troubleshooting procedures.

---

This comprehensive fix addresses all identified issues with the Kermit Bootstrap and Faucet services, providing production-ready deployment scripts and operational procedures.
