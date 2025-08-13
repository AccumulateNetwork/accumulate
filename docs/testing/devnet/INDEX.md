# DevNet Documentation Index

[← Back to Testing Index](../INDEX.md) | [← Back to Main Index](../../INDEX.md)

## Overview
DevNet is Accumulate's local development network, providing a complete blockchain environment for development, testing, and debugging.

## Quick Start
```bash
# Start DevNet with default configuration
./scripts/devnet/devnet_config.sh start

# Start with custom configuration (3 BVNs, 3 validators, 1 follower)
./scripts/devnet/devnet_config.sh start 3 3 1

# Quick minimal setup
./scripts/devnet/devnet_config.sh quick
```

## Core Documentation

### Setup and Configuration
- [DevNet Setup Guide](devnet-setup.md) - Complete setup instructions
- [DevNet Configuration](DEVNET_CONFIGURATION.md) - Configuration options
- [DevNet Design](DEVNET_DESIGN.md) - Architecture and design details
- [DevNet Testing Guide](DEVNET_TESTING_GUIDE.md) - Testing workflows

### Related Scripts
- [`devnet_config.sh`](../../../scripts/devnet/devnet_config.sh) - Main configuration script
- [`devnet_manager.sh`](../../../scripts/devnet/devnet_manager.sh) - DevNet management
- [`devnet_load_test.sh`](../../../scripts/devnet/devnet_load_test.sh) - Load testing

## Configuration Options

### Pre-defined Configurations
| Profile | BVNs | Validators | Followers | Use Case |
|---------|------|------------|-----------|----------|
| `quick` | 2 | 1 | 0 | Quick testing |
| `standard` | 2 | 3 | 1 | Balanced testing |
| `large` | 3 | 3 | 2 | Stress testing |
| `multi` | 5 | 2 | 1 | Cross-chain testing |

### Custom Configuration
```bash
# Start with custom parameters
./scripts/devnet/devnet_config.sh start <bvns> <validators> <followers>

# Example: 4 BVNs, 2 validators each, 1 follower each
./scripts/devnet/devnet_config.sh start 4 2 1
```

## Network Architecture

### Components
1. **Directory Network (DN)**
   - Identity management
   - Routing information
   - Global state

2. **Block Validator Networks (BVNs)**
   - Transaction processing
   - Consensus
   - State management

3. **CrossChain Conductor (CCC)**
   - Cross-partition messaging
   - Gap recovery
   - Message ordering

### Port Allocation
- Base port: 26656 (configurable)
- Each node uses 4 consecutive ports
- See [Port Configuration](../../kermit/port-configuration.md)

## Testing Features

### Gap Recovery Testing
- [Gap Recovery Design](../../design/crosschain/GAP_RECOVERY_ACTUAL.md)
- [`gap_recovery_demo.sh`](../../../scripts/devnet/gap_recovery_demo.sh) - Demo script
- [`interactive_pause_test.sh`](../../../scripts/devnet/interactive_pause_test.sh) - Interactive testing

### Pause/Resume (testnet build)
Build with testnet tag to enable:
```bash
go build -tags testnet -o accumulated ./cmd/accumulated
```

Debug endpoints:
- `POST /debug/ccc/pause` - Pause partition
- `POST /debug/ccc/resume` - Resume partition
- `GET /debug/ccc/status` - Check status
- `GET /debug/ccc/metrics` - View metrics

## API Endpoints

### Default Endpoints
- Directory Network: `http://localhost:26660/v2`
- BVN0: `http://localhost:27010/v2`
- BVN1: `http://localhost:27011/v2`
- BVN2: `http://localhost:27012/v2`

### Health Checks
```bash
# Check DevNet status
curl http://localhost:26660/v2/status

# Query network info
curl http://localhost:26660/v2/network/status
```

## Management Scripts

### Lifecycle Management
- **Start**: `./scripts/devnet/devnet_config.sh start`
- **Stop**: `./scripts/devnet/devnet_config.sh stop`
- **Clean**: `./scripts/devnet/devnet_config.sh clean`
- **Status**: `./scripts/devnet/devnet_config.sh status`

### Testing Scripts
- [`run_full_test_suite.sh`](../../../scripts/devnet/run_full_test_suite.sh) - Complete test suite
- [`quick_test.sh`](../../../scripts/devnet/quick_test.sh) - Quick validation
- [`comprehensive_load_test.sh`](../../../scripts/devnet/comprehensive_load_test.sh) - Load testing

## Resource Requirements

### Memory
- Minimal (2 BVNs, 1 validator): ~1 GB
- Standard (2 BVNs, 3 validators): ~2-3 GB
- Large (3 BVNs, 3 validators): ~3-4 GB
- Stress (5 BVNs, 3 validators): ~5-6 GB

### Ports
Total ports = (validators + followers) × (1 + BVNs) × 4

Example: 3 BVNs with 2 validators, 1 follower each:
- Directory Network: 12 ports (3 nodes × 4 ports)
- BVN nodes: 36 ports (9 nodes × 4 ports)
- Total: 48 ports

## Troubleshooting

### Common Issues

#### Port Conflicts
```bash
# Check port usage
lsof -i :26656-26700

# Use different base port
BASE_PORT=27000 ./scripts/devnet/devnet_config.sh start
```

#### Slow Startup
- Wait 15-30 seconds for network formation
- Check logs: `tail -f devnet_config.log`
- Verify resources: `top` or `htop`

#### API Not Responding
```bash
# Check process status
ps aux | grep accumulated

# Check logs
tail -100 devnet_config.log

# Restart DevNet
./scripts/devnet/devnet_config.sh clean
./scripts/devnet/devnet_config.sh start
```

## Monitoring

### Logs
- Main log: `devnet_config.log`
- Node logs: `.devnet-test/*/node.log`

### Metrics
- Prometheus endpoint: `http://localhost:26661/metrics`
- Custom metrics via debug endpoints

### Real-time Monitoring
```bash
# Watch logs
tail -f devnet_config.log | grep -E "error|warn|gap"

# Monitor processes
watch -n 1 'ps aux | grep accumulated'

# Check network status
watch -n 2 'curl -s http://localhost:26660/v2/status | jq .'
```

## Integration

### CI/CD
```yaml
# GitLab CI example
test:
  script:
    - ./scripts/devnet/devnet_config.sh start 3 2 1
    - ./scripts/devnet/run_full_test_suite.sh
    - ./scripts/devnet/devnet_config.sh clean
```

### Docker
```dockerfile
FROM golang:1.19
WORKDIR /accumulate
COPY . .
RUN go build -o accumulated ./cmd/accumulated
CMD ["./scripts/devnet/devnet_config.sh", "start"]
```

## Advanced Features

### Custom Genesis
- Modify genesis parameters
- Configure initial accounts
- Set custom chain IDs

### Network Simulation
- Simulate network delays
- Inject failures
- Test edge cases

### Performance Testing
- Load generation
- Stress testing
- Resource monitoring

## Best Practices

1. **Clean State**: Always clean before starting fresh tests
2. **Resource Monitoring**: Watch memory and CPU usage
3. **Log Analysis**: Check logs for errors and warnings
4. **Incremental Testing**: Start small, increase complexity
5. **Documentation**: Document test scenarios and results

## Related Documentation

- [Load Testing](../load/INDEX.md) - Performance testing
- [Gap Recovery](../../design/crosschain/GAP_RECOVERY_ACTUAL.md) - Recovery mechanisms
- [Network Documentation](../../network/INDEX.md) - Network architecture
- [API Documentation](../../api/INDEX.md) - API reference