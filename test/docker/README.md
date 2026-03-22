# Load Testing Docker Environment

Docker Compose configurations for running Accumulate load testing networks.

## Overview

This directory contains two deployment options:

1. **All-in-one** (`docker-compose.yml`) - Simple single-container deployment
2. **Distributed** (`docker-compose.distributed.yml`) - Multi-container deployment

## Deployment Options

### Option 1: All-in-One (Recommended for Development)

Runs all nodes in a single container using `accumulated run devnet` mode.

**Advantages:**
- Simple setup and teardown
- Lower resource usage
- Faster startup
- Good for rapid iteration

**Configuration:**
- 3 BVNs (BVN0, BVN1, BVN2)
- 4 validators per BVN
- 1 Directory Network
- 16GB memory limit

**Usage:**

```bash
# Start
docker compose -f test/docker/docker-compose.yml up -d

# View logs
docker compose -f test/docker/docker-compose.yml logs -f

# Check status
docker compose -f test/docker/docker-compose.yml ps

# Access API
curl http://localhost:8080/v3/describe

# Stop and cleanup
docker compose -f test/docker/docker-compose.yml down -v
```

### Option 2: Distributed (For Production-Like Testing)

Runs each validator in a separate container for better isolation.

**Advantages:**
- Individual node control
- Fault injection testing
- Per-node monitoring
- Realistic network topology

**Configuration:**
- 12 separate containers (4 per BVN)
- 2GB memory per container
- Independent restart capability

**Usage:**

```bash
# Start all nodes
docker compose -f test/docker/docker-compose.distributed.yml up -d

# Start specific BVN only
docker compose -f test/docker/docker-compose.distributed.yml up -d bvn0-0 bvn0-1 bvn0-2 bvn0-3

# Stop specific node (fault testing)
docker compose -f test/docker/docker-compose.distributed.yml stop bvn1-2

# View individual node logs
docker compose -f test/docker/docker-compose.distributed.yml logs -f bvn0-0

# Restart node
docker compose -f test/docker/docker-compose.distributed.yml restart bvn1-2

# Stop all
docker compose -f test/docker/docker-compose.distributed.yml down -v
```

## Network Topology

### All-in-One Mode

```
┌─────────────────────────────────────┐
│   accumulate-load-testnet           │
│                                     │
│  ┌───────┐  ┌───────┐  ┌───────┐   │
│  │ BVN0  │  │ BVN1  │  │ BVN2  │   │
│  │ 4 val │  │ 4 val │  │ 4 val │   │
│  └───────┘  └───────┘  └───────┘   │
│         ↓        ↓        ↓         │
│            ┌──────────┐             │
│            │    DN    │             │
│            └──────────┘             │
└─────────────────────────────────────┘
```

### Distributed Mode

```
BVN0                BVN1                BVN2
┌──────┐           ┌──────┐           ┌──────┐
│bvn0-0│           │bvn1-0│           │bvn2-0│
├──────┤           ├──────┤           ├──────┤
│bvn0-1│           │bvn1-1│           │bvn2-1│
├──────┤           ├──────┤           ├──────┤
│bvn0-2│           │bvn1-2│           │bvn2-2│
├──────┤           ├──────┤           ├──────┤
│bvn0-3│           │bvn1-3│           │bvn2-3│
└──────┘           └──────┘           └──────┘
    ↓                  ↓                  ↓
    └──────────────────┴──────────────────┘
                       ↓
                  ┌────────┐
                  │   DN   │
                  └────────┘
```

## Port Mapping

### All-in-One

| Service | Port | Description |
|---------|------|-------------|
| BVN0 API | 8080 | HTTP API for BVN0 |
| BVN1 API | 8081 | HTTP API for BVN1 |
| BVN2 API | 8082 | HTTP API for BVN2 |
| DN API | 8083 | HTTP API for Directory Network |
| P2P | 26656-26670 | Peer-to-peer communication |
| Metrics | 26660-26674 | Prometheus metrics |

### Distributed

Each validator exposes:
- P2P port (26656-26675)
- Metrics port (26660-26679)
- API port (8080-8082 for BVN leaders)

See `docker-compose.distributed.yml` for full port mapping.

## Resource Requirements

### Minimum

- **All-in-One**: 16GB RAM, 4 CPU cores, 50GB disk
- **Distributed**: 24GB RAM, 8 CPU cores, 100GB disk

### Recommended

- **All-in-One**: 32GB RAM, 8 CPU cores, 100GB SSD
- **Distributed**: 48GB RAM, 16 CPU cores, 200GB SSD

## Monitoring

### Docker Stats

```bash
# Monitor resource usage
docker stats

# Watch specific container
docker stats accumulate-load-testnet
```

### Logs

```bash
# Follow all logs
docker compose -f test/docker/docker-compose.yml logs -f

# Grep for errors
docker compose -f test/docker/docker-compose.yml logs | grep -i error

# Export logs
docker compose -f test/docker/docker-compose.yml logs > /tmp/load-test.log
```

### Health Checks

```bash
# All-in-one
curl http://localhost:8080/v3/describe

# Distributed - check each BVN
curl http://localhost:8080/v3/describe  # BVN0
curl http://localhost:8081/v3/describe  # BVN1
curl http://localhost:8082/v3/describe  # BVN2
```

## Troubleshooting

### Container won't start

```bash
# Check logs for errors
docker compose -f test/docker/docker-compose.yml logs

# Verify ports aren't in use
netstat -tulpn | grep -E '(8080|26656)'

# Rebuild image
docker compose -f test/docker/docker-compose.yml build --no-cache
```

### Out of memory

```bash
# Check memory usage
docker stats

# Increase mem_limit in docker-compose.yml
# Restart with new limits
docker compose -f test/docker/docker-compose.yml up -d
```

### Network issues

```bash
# Recreate network
docker compose -f test/docker/docker-compose.yml down
docker network prune
docker compose -f test/docker/docker-compose.yml up -d
```

### Reset everything

```bash
# Complete cleanup
docker compose -f test/docker/docker-compose.yml down -v
docker system prune -a --volumes
```

## Integration with Load Generator

Once the network is running, you can use it with the load generator:

```bash
# Get funder key from test wallet
FUNDER_KEY=$(test-wallet export-keys ~/.accumulate/test-wallet.json | jq -r '.funder.privateKey')

# Setup accounts
load-generator \
  --nodes="http://localhost:8080/v3,http://localhost:8081/v3,http://localhost:8082/v3" \
  --setup \
  --funder-key="$FUNDER_KEY" \
  --accounts=3000

# Run load test
load-generator \
  --nodes="http://localhost:8080/v3,http://localhost:8081/v3,http://localhost:8082/v3" \
  --tps=1000 \
  --duration=30m
```

## Building Custom Images

To build with specific tags or commits:

```bash
# Build with custom version
docker compose -f test/docker/docker-compose.yml build \
  --build-arg GIT_DESCRIBE=v1.2.0 \
  --build-arg GIT_COMMIT=abc123

# Build without cache
docker compose -f test/docker/docker-compose.yml build --no-cache
```

## Advanced Configuration

### Custom Network Configuration

Edit `docker-compose.yml` to customize:

```yaml
command:
  - run
  - devnet
  - --bvns=5              # Increase to 5 BVNs
  - --validators=7        # Increase to 7 validators per BVN
  - --followers=1         # Add follower nodes
  - --name=CustomTest
```

### Enable Debug Logging

```yaml
environment:
  - ACC_LOG_LEVEL=debug
command:
  - run
  - devnet
  - -d                    # Enable debug mode
```

### Custom Database

```yaml
command:
  - run
  - devnet
  - --database=bolt       # Use BoltDB instead of Badger
```

## See Also

- [Test Wallet](../wallet/README.md) - Managing test account keys
- [Load Generator](../../cmd/load-generator/) - Generating transaction load
- [Network Monitor](#) - Real-time monitoring (Issue #3845)
