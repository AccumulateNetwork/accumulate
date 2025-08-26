# Devnet Configuration Guide

## Overview

The devnet is implemented as part of the `accumulated` binary and can be configured through environment variables for flexible testing scenarios.

## Starting a Devnet

### Basic Start
```bash
accumulated run devnet
```

### With Custom Port
```bash
accumulated run devnet --port 27000 --bvns 4 --validators 3
```

### Parameters
- `--port`: Base port (default: 26656). JSON-RPC will be on port+4
- `--bvns`: Number of block validator networks (default: 2)
- `--validators`: Number of validators per partition (default: 2)
- `--followers`: Number of followers per partition (default: 1)
- `-w`: Working directory for configuration and data

## Environment Variables

The tests support the following environment variables for configuration:

### ACCUMULATE_DEVNET_URL
Full URL to the devnet endpoint.
```bash
export ACCUMULATE_DEVNET_URL="http://localhost:27004/v3"
```

### ACCUMULATE_DEVNET_PORT
Just the port number (tests will add /v3 path).
```bash
export ACCUMULATE_DEVNET_PORT="27004"
```

### ACCUMULATE_DEVNET_HOST
Host to connect to (default: localhost).
```bash
export ACCUMULATE_DEVNET_HOST="192.168.1.100"
```

## Multiple Devnets

To run multiple devnets for different test scenarios:

### Scenario 1: Development Devnet
```bash
# Terminal 1: Development devnet on port 26656
accumulated run devnet --port 26656 -w .devnet-dev

# Run tests against it
export ACCUMULATE_DEVNET_PORT=26660
go test ./...
```

### Scenario 2: CI/CD Devnet
```bash
# Terminal 2: CI devnet on port 27000
accumulated run devnet --port 27000 -w .devnet-ci --bvns 4

# Run tests against it
export ACCUMULATE_DEVNET_PORT=27004
go test ./...
```

### Scenario 3: Stress Testing Devnet
```bash
# Terminal 3: Large devnet for stress testing
accumulated run devnet --port 28000 -w .devnet-stress --bvns 8 --validators 5

# Run stress tests
export ACCUMULATE_DEVNET_PORT=28004
go test -bench=. ./...
```

## Port Discovery

If no environment variables are set, the tests will automatically scan for devnets in this order:

1. Check `ACCUMULATE_DEVNET_URL` environment variable
2. Check `ACCUMULATE_DEVNET_PORT` environment variable
3. Auto-detect by scanning common base ports:
   - 27000 (+4 for JSON-RPC = 27004)
   - 26656 (+4 for JSON-RPC = 26660)
   - 8000 (+4 for JSON-RPC = 8004)
4. Try specific known ports:
   - 27004, 26660, 8545, 9545

## Docker Support

For containerized devnets:

```yaml
# docker-compose.yml
version: '3.8'
services:
  devnet:
    image: accumulate:latest
    command: run devnet --port 26656
    ports:
      - "26660:26660"  # JSON-RPC port
    environment:
      - ACCUMULATE_LOG_LEVEL=info
    volumes:
      - ./devnet-data:/root/.accumulate
```

Then configure tests:
```bash
export ACCUMULATE_DEVNET_URL="http://localhost:26660/v3"
go test ./...
```

## Kubernetes Support

For Kubernetes deployments:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: accumulate-devnet
spec:
  ports:
    - port: 26660
      name: jsonrpc
  selector:
    app: accumulate-devnet
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: accumulate-devnet
spec:
  replicas: 1
  selector:
    matchLabels:
      app: accumulate-devnet
  template:
    metadata:
      labels:
        app: accumulate-devnet
    spec:
      containers:
      - name: devnet
        image: accumulate:latest
        command: ["accumulated", "run", "devnet", "--port", "26656"]
        ports:
        - containerPort: 26660
```

Access via port-forward:
```bash
kubectl port-forward svc/accumulate-devnet 26660:26660
export ACCUMULATE_DEVNET_URL="http://localhost:26660/v3"
go test ./...
```

## Debugging Connection Issues

### Check if devnet is running
```bash
ps aux | grep accumulated
```

### Check listening ports
```bash
ss -tln | grep -E ':(27[0-9]{3}|26[0-9]{3})'
```

### Test connection manually
```bash
curl -X POST http://localhost:27004/v3 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"faucet","params":{"account":"acc://test"}}'
```

### View devnet logs
```bash
# If running in foreground
# Check console output

# If running with systemd
journalctl -u accumulate-devnet -f

# If running in docker
docker logs accumulate-devnet -f
```

## Best Practices

1. **Use environment variables** for CI/CD pipelines
2. **Document port assignments** when running multiple devnets
3. **Use different working directories** for each devnet instance
4. **Monitor resource usage** - each devnet consumes CPU and memory
5. **Clean up old devnets** - stop and remove working directories when done

## Troubleshooting

### "Address already in use"
Another devnet is running on that port. Either:
- Stop the existing devnet
- Choose a different port
- Use the existing devnet

### "Connection refused"
Devnet is not running or wrong port. Check:
- Is accumulated process running?
- Is the JSON-RPC port correct? (base port + 4)
- Are firewall rules blocking the connection?

### "Faucet failed"
The devnet faucet might be:
- Rate limited - wait and retry
- Out of funds - restart devnet
- Not initialized - wait for devnet to fully start