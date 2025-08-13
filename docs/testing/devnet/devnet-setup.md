# DevNet Setup Guide

This guide covers how to properly set up and run an Accumulate DevNet for development and testing.

## Overview

**DevNet** is a local development network that creates a complete Accumulate blockchain environment on your machine. It includes:

- **Directory Network (DN)**: The core identity and routing layer
- **Block Validator Networks (BVNs)**: Multiple validator networks for consensus
- **API Endpoints**: JSON-RPC v2 API for interaction
- **Test Environment**: Pre-configured for development and testing

## Network Types

- **Cyclops**: MainNet (production network)
- **Kermit**: TestNet (public test network)
- **DevNet**: Local development network (this guide)

## Prerequisites

- Go 1.19+ installed
- Accumulate repository cloned locally
- Terminal access

## Quick Start

### Method 1: Using accumulated daemon (Recommended)

```bash
# Navigate to repository root
cd /path/to/accumulate

# Step 1: Initialize DevNet
go run ./cmd/accumulated run devnet --init-only --reset -w .nodes

# Step 2: Run DevNet
go run ./cmd/accumulated run devnet -w .nodes
```

### Method 2: Using shell script

```bash
# Navigate to repository root
cd /path/to/accumulate

# Run the updated script
bash scripts/devnet-init-run.sh
```

### Method 3: Using test automation (May have dependency issues)

```bash
# Navigate to repository root
cd /path/to/accumulate

# Run the test automation script
cd test/cmd/devnet
go run main.go
```

**Note**: Method 3 may encounter dependency issues with external packages. Use Method 1 for reliable setup.

## DevNet Configuration

### Default Configuration

- **Base Port**: 26656
- **API Endpoint**: `http://127.0.0.1:26660/v2`
- **Working Directory**: `.nodes/`
- **Network Name**: "DevNet"
- **BVNs**: 2 (default)
- **Validators per BVN**: 2 (default)
- **Followers per BVN**: 1 (default)

### Available Flags

```bash
go run ./cmd/accumulated run devnet --help
```

Key flags:
- `--init-only`: Initialize and exit, do not run
- `--reset`: Reset state before starting
- `--soft-reset`: Reset only if necessary
- `-w, --work-dir`: Working directory (default: `~/.accumulate`)
- `-b, --bvns`: Number of BVNs (default: 2)
- `-v, --validators`: Validators per partition (default: 2)
- `-f, --followers`: Followers per partition (default: 1)
- `--port`: Base port (default: 26656)
- `--name`: Network name (default: "DevNet")

## API Endpoints

Once DevNet is running, you can access:

### Main API Endpoint
- **URL**: `http://127.0.0.1:26660/v2`
- **Protocol**: JSON-RPC 2.0
- **Usage**: Primary endpoint for API calls

### Individual Node Endpoints
- **BVN Nodes**: `http://127.0.1.x:26657/v2` (where x = 2,3,4,5,6,7)
- **Protocol**: JSON-RPC 2.0
- **Usage**: Direct node access

## Testing Your DevNet

### Basic Connectivity Test

```bash
# Query the Directory Network identity
curl -s -X POST http://127.0.0.1:26660/v2 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"query","params":{"url":"acc://dn.acme"},"id":1}'
```

Expected response:
```json
{
  "jsonrpc": "2.0",
  "result": {
    "type": "identity",
    "data": {
      "type": "identity",
      "url": "acc://dn.acme"
    }
  },
  "id": 1
}
```

### Create Test Identity

```bash
# Create a test identity
curl -s -X POST http://127.0.0.1:26660/v2 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "submit",
    "params": {
      "envelope": {
        "transaction": [{
          "header": {
            "principal": "acc://dn.acme",
            "initiator": "acc://dn.acme"
          },
          "body": {
            "type": "createIdentity",
            "url": "acc://test-identity"
          }
        }]
      }
    },
    "id": 1
  }'
```

### Query Created Identity

```bash
# Wait a few seconds for transaction processing, then query
curl -s -X POST http://127.0.0.1:26660/v2 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"query","params":{"url":"acc://test-identity"},"id":1}'
```

## Complete Testing Script

Save this as `test-devnet.sh`:

```bash
#!/bin/bash
# Complete DevNet testing script

set -e

echo "Starting DevNet..."
# Initialize DevNet
go run ./cmd/accumulated run devnet --init-only --reset -w .nodes

# Start DevNet in background
go run ./cmd/accumulated run devnet -w .nodes &
DEVNET_PID=$!

# Wait for DevNet to start
echo "Waiting for DevNet to start..."
sleep 15

# Test all API endpoints
echo "Testing API endpoints..."

# 1. Query Directory Network
echo "1. Testing Directory Network query..."
curl -s -X POST http://127.0.0.1:26660/v2 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"query","params":{"url":"acc://dn.acme"},"id":1}' | jq '.result.type'

# 2. Test individual node
echo "2. Testing individual BVN node..."
curl -s -X POST http://127.0.1.2:26657/v2 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"query","params":{"url":"acc://dn.acme"},"id":1}' | jq '.result.type'

# 3. Create test identity
echo "3. Creating test identity..."
curl -s -X POST http://127.0.0.1:26660/v2 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"submit","params":{"envelope":{"transaction":[{"header":{"principal":"acc://dn.acme","initiator":"acc://dn.acme"},"body":{"type":"createIdentity","url":"acc://test-identity"}}]}},"id":1}' | jq '.result'

# 4. Wait and query the created identity
echo "4. Querying created identity..."
sleep 3  # Wait for transaction processing
curl -s -X POST http://127.0.0.1:26660/v2 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"query","params":{"url":"acc://test-identity"},"id":1}' | jq '.result.type'

echo "All tests completed successfully!"

# Cleanup
kill $DEVNET_PID
```

## Troubleshooting

### Common Issues

1. **Port Already in Use**
   ```bash
   # Check what's using the ports
   ss -tlnp | grep 26660
   ss -tlnp | grep 26657
   
   # Kill existing processes
   pkill accumulated
   ```

2. **Peer Connection Errors**
   - These are normal during startup as nodes discover each other
   - Wait 15-30 seconds for the network to stabilize

3. **API Not Responding**
   - Ensure DevNet has fully started (wait 15+ seconds)
   - Check that processes are running: `ps aux | grep accumulated`
   - Verify ports are listening: `ss -tlnp | grep accumulated`

4. **Working Directory Issues**
   - Use absolute paths for `--work-dir`
   - Ensure directory is writable
   - Use `--reset` to clean up corrupted state

### Debug Mode

Run DevNet with debug logging:

```bash
go run ./cmd/accumulated run devnet -w .nodes --debug
```

### Clean Reset

To completely reset DevNet state:

```bash
# Stop any running DevNet
pkill accumulated

# Remove working directory
rm -rf .nodes

# Reinitialize
go run ./cmd/accumulated run devnet --init-only --reset -w .nodes
```

## Advanced Configuration

### Custom Network Parameters

```bash
# Run with custom configuration
go run ./cmd/accumulated run devnet \
  -w .nodes \
  --bvns 3 \
  --validators 3 \
  --followers 2 \
  --port 27000 \
  --name "CustomDevNet"
```

### Integration with Development Tools

DevNet is designed to integrate with:
- **CLI Tools**: Use `accumulate` CLI to interact with DevNet
- **SDKs**: Point SDKs to `http://127.0.0.1:26660/v2`
- **Testing Frameworks**: Use DevNet for automated testing
- **Development Workflows**: Start/stop DevNet in CI/CD pipelines

## Production Warning

⚠️ **IMPORTANT**: DevNet is NOT suitable for production use. It:
- Uses simplified consensus mechanisms
- Has reduced security measures
- Includes development shortcuts
- Is designed for testing only

For production, use:
- **MainNet**: Cyclops network
- **TestNet**: Kermit network

## Next Steps

1. **Explore APIs**: See `/docs/api/api-interfaces-reference.md`
2. **CLI Tools**: Check `/docs/tools/` for command-line utilities
3. **SDK Integration**: Use DevNet endpoints in your applications
4. **Testing**: Build comprehensive test suites against DevNet

## Support

For issues with DevNet setup:
1. Check this documentation
2. Review logs with `--debug` flag
3. Verify system requirements
4. Check GitHub issues for known problems
