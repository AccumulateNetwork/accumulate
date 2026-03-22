# Test Scripts

Utility scripts for managing the load testing environment.

## Scripts

### cleanup.sh

Comprehensive cleanup script for removing all load testing artifacts.

**Features:**
- Remove Docker containers, volumes, and networks
- Clean log files and monitoring data
- Optionally remove test wallet
- Dry-run mode for safe preview
- Selective cleanup options

**Usage:**

```bash
# Standard cleanup (Docker + logs + volumes)
./test/scripts/cleanup.sh

# Complete cleanup including wallet
./test/scripts/cleanup.sh --all --force

# Preview cleanup actions
./test/scripts/cleanup.sh --dry-run

# Selective cleanup
./test/scripts/cleanup.sh --docker --logs --no-volumes
```

**Options:**

| Option | Description | Default |
|--------|-------------|---------|
| `--all` | Clean everything | - |
| `--docker` | Clean Docker resources | yes |
| `--no-docker` | Skip Docker cleanup | no |
| `--logs` | Clean log files | yes |
| `--no-logs` | Skip log cleanup | no |
| `--wallet` | Clean test wallet | no |
| `--volumes` | Clean Docker volumes | yes |
| `--no-volumes` | Keep Docker volumes | no |
| `--network` | Clean Docker networks | yes |
| `--dry-run` | Preview without executing | no |
| `--force` | Skip confirmation prompts | no |

**Examples:**

```bash
# Quick cleanup keeping volumes
./test/scripts/cleanup.sh --no-volumes --force

# Complete reset including wallet
./test/scripts/cleanup.sh --all

# Preview what would be cleaned
./test/scripts/cleanup.sh --all --dry-run

# Clean only logs
./test/scripts/cleanup.sh --no-docker --logs
```

### reset.sh

Complete environment reset for fresh load testing.

**Features:**
- Cleanup existing resources
- Rebuild Docker images
- Recreate test wallet
- Start network
- Configurable for different deployment modes

**Usage:**

```bash
# Full reset with defaults
./test/scripts/reset.sh

# Reset for distributed mode
./test/scripts/reset.sh --mode distributed

# Reset without starting
./test/scripts/reset.sh --no-start

# Quick reset (skip cleanup and build)
./test/scripts/reset.sh --skip-cleanup --no-build --force
```

**Options:**

| Option | Description | Default |
|--------|-------------|---------|
| `--mode <mode>` | Network mode (simple\|distributed) | simple |
| `--no-build` | Skip Docker rebuild | no |
| `--no-wallet` | Skip wallet creation | no |
| `--no-start` | Don't start network | no |
| `--skip-cleanup` | Skip cleanup phase | no |
| `--wallet-config <opts>` | Custom wallet options | --lite 1000 --adi-token 1000 --adi-data 1000 |
| `--force` | Skip confirmations | no |

**Examples:**

```bash
# Standard full reset
./test/scripts/reset.sh

# Custom wallet configuration
./test/scripts/reset.sh --wallet-config "--lite 500 --adi-token 500"

# Reset distributed mode without rebuild
./test/scripts/reset.sh --mode distributed --no-build

# Prepare but don't start
./test/scripts/reset.sh --no-start
```

**Reset Process:**

1. **Cleanup** - Remove all existing resources
2. **Build** - Rebuild Docker images
3. **Create Wallet** - Generate test accounts
4. **Start Network** - Launch containers

## Common Workflows

### Clean Slate for Testing

```bash
# Complete reset
./test/scripts/reset.sh --force

# Start monitoring
./test/monitor/network-monitor.sh &

# Run load test
load-generator --setup --nodes=http://localhost:8080/v3
```

### Quick Restart

```bash
# Fast restart without rebuild
./test/scripts/reset.sh --skip-cleanup --no-build --force
```

### Cleanup After Testing

```bash
# Remove everything except wallet
./test/scripts/cleanup.sh --force

# Remove everything including wallet
./test/scripts/cleanup.sh --all --force
```

### Switch Deployment Modes

```bash
# From simple to distributed
./test/scripts/cleanup.sh --force
./test/scripts/reset.sh --mode distributed
```

### Preserve Data Between Tests

```bash
# Cleanup but keep volumes and wallet
./test/scripts/cleanup.sh --no-volumes --docker --logs
```

## Troubleshooting

### Cleanup fails with "device busy"

```bash
# Stop all containers first
docker stop $(docker ps -a -q)

# Then cleanup
./test/scripts/cleanup.sh --force
```

### Reset hangs during build

```bash
# Build separately to see errors
docker compose -f test/docker/docker-compose.yml build

# Then continue reset
./test/scripts/reset.sh --no-build
```

### Wallet creation fails

```bash
# Install test-wallet CLI
go install ./cmd/test-wallet

# Verify installation
which test-wallet

# Try reset again
./test/scripts/reset.sh
```

### Permission denied errors

```bash
# Make scripts executable
chmod +x test/scripts/*.sh

# Run again
./test/scripts/cleanup.sh
```

## Advanced Usage

### Automated Testing Pipeline

```bash
#!/bin/bash
# Automated test pipeline

# Reset environment
./test/scripts/reset.sh --force

# Start monitoring
./test/monitor/network-monitor.sh > /tmp/monitor.log 2>&1 &
MONITOR_PID=$!

# Run load test
load-generator \
  --nodes=http://localhost:8080/v3 \
  --setup \
  --tps=1000 \
  --duration=30m

# Stop monitor
kill $MONITOR_PID

# Collect results and cleanup
cp /tmp/network-monitor.log ./results/
./test/scripts/cleanup.sh --force
```

### Selective Component Reset

```bash
# Reset only Docker
./test/scripts/cleanup.sh --docker --no-logs
./test/scripts/reset.sh --no-wallet --no-build

# Reset only wallet
./test/scripts/cleanup.sh --wallet --no-docker --no-logs
./test/scripts/reset.sh --no-build --no-start
```

### Custom Cleanup

```bash
# Cleanup script handles common cases, but for custom needs:

# Remove specific volumes
docker volume rm load-testnet-data

# Clean specific networks
docker network rm load-test-net

# Remove old images
docker image prune -a
```

## Integration

### With CI/CD

```yaml
# GitLab CI example
test:load:
  script:
    - ./test/scripts/reset.sh --force
    - ./test/monitor/network-monitor.sh &
    - load-generator --setup --tps=1000 --duration=5m
  after_script:
    - ./test/scripts/cleanup.sh --all --force
```

### With Monitoring

```bash
# Start monitor before reset
./test/monitor/network-monitor.sh &

# Reset environment
./test/scripts/reset.sh

# Monitor will continue watching
```

### With Load Generator

```bash
# Reset and get funder key
./test/scripts/reset.sh
FUNDER_KEY=$(test-wallet export-keys ~/.accumulate/test-wallet.json | jq -r '.funder.privateKey')

# Run load generator
load-generator --funder-key="$FUNDER_KEY" --setup
```

## See Also

- [Docker Deployment](../docker/README.md) - Network deployment guide
- [Monitoring](../monitor/README.md) - Health monitoring tools
- [Test Wallet](../wallet/README.md) - Test account management
