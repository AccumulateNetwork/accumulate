# DevNet Configuration Guide

## Overview

The Accumulate DevNet now supports flexible configuration of partitions (BVNs) and validators, making it easy to test different network topologies without writing custom code.

## Quick Start

### Using devnet_config.sh

The simplest way to start a DevNet with custom configuration:

```bash
# Start with 3 BVNs, 3 validators per BVN, 1 follower per BVN
./devnet_config.sh start 3 3 1

# Quick minimal setup (2 BVNs, 1 validator each)
./devnet_config.sh quick

# Standard setup (2 BVNs, 3 validators, 1 follower)
./devnet_config.sh standard

# Large setup for stress testing
./devnet_config.sh large

# Multi-partition setup (5 BVNs)
./devnet_config.sh multi
```

### Using load_test_runner.sh

Run load tests with different configurations automatically:

```bash
# Run full test suite with standard configuration
./load_test_runner.sh suite full standard

# Run cross-chain specific tests with 3 BVNs
./load_test_runner.sh cross-chain

# Run performance benchmarks across different configs
./load_test_runner.sh benchmark

# Run all test combinations
./load_test_runner.sh all
```

## Configuration Options

### Command Line Parameters

The `accumulated` command supports these DevNet parameters:

- `-b, --bvns <int>` - Number of Block Validator Networks (partitions)
- `-v, --validators <int>` - Number of validator nodes per partition
- `-f, --followers <int>` - Number of follower nodes per partition
- `--port <int>` - Base port for network listeners
- `--reset` - Reset state before starting
- `-w, --work-dir <path>` - Working directory for data

### Pre-defined Configurations

| Configuration | BVNs | Validators | Followers | Use Case |
|--------------|------|------------|-----------|----------|
| minimal | 2 | 1 | 0 | Quick testing, development |
| standard | 2 | 3 | 1 | General testing, balanced setup |
| large | 3 | 3 | 2 | Stress testing, high availability |
| cross_chain | 3 | 2 | 1 | Cross-chain transaction testing |
| stress | 5 | 3 | 2 | Maximum stress testing |

## Resource Requirements

### Port Usage

Each node requires 4 ports:
- RPC port
- P2P port  
- Metrics port
- API port

Total ports = (validators + followers) × (1 + BVNs) × 4

Example: 3 BVNs with 3 validators and 1 follower each:
- Directory Network: 4 nodes × 4 ports = 16 ports
- BVN nodes: 3 BVNs × 4 nodes × 4 ports = 48 ports
- Total: 64 ports starting from base port 26656

### Memory Requirements

Approximate memory usage:
- Per validator: ~200-300 MB
- Per follower: ~150-200 MB
- Base overhead: ~500 MB

Example: 3 BVNs with 3 validators and 1 follower:
- Total nodes: 16 (4 DN + 12 BVN)
- Estimated memory: ~3-4 GB

## Test Scenarios

### 1. Basic Functionality Testing

```bash
# Minimal setup for quick iteration
./devnet_config.sh quick
./load_test_runner.sh test multi_validator_conductor.go minimal
```

### 2. Cross-Chain Transaction Testing

```bash
# 3 BVNs for testing cross-partition transactions
./devnet_config.sh start 3 2 1
./load_test_runner.sh cross-chain
```

### 3. High Availability Testing

```bash
# Multiple validators per partition
./devnet_config.sh start 2 5 2
./load_test_runner.sh suite full large
```

### 4. Stress Testing

```bash
# Maximum partitions and validators
./devnet_config.sh start 5 3 2
./load_test_runner.sh benchmark
```

## Configuration Files

Create custom configuration files in `devnet_configs/`:

```bash
# Create sample configs
./devnet_config.sh configs

# Load from config file
./devnet_config.sh load devnet_configs/custom.conf
```

Example configuration file:
```bash
# custom.conf
BVNS=4
VALIDATORS=2
FOLLOWERS=1
```

## Monitoring and Debugging

### Check DevNet Status

```bash
# Show current DevNet status
./devnet_config.sh status

# View logs
tail -f devnet_config.log

# Check specific partition logs
grep "BVN1" devnet_config.log
```

### Common Issues and Solutions

1. **Port conflicts**
   ```bash
   # Use different base port
   BASE_PORT=27000 ./devnet_config.sh start 3 3 1
   ```

2. **Memory issues with large configurations**
   ```bash
   # Reduce followers for memory-constrained systems
   ./devnet_config.sh start 3 2 0
   ```

3. **Slow startup with many validators**
   ```bash
   # Increase timeout in wait_for_devnet() function
   # Edit devnet_config.sh: max_retries=120
   ```

## Load Test Integration

The load tests automatically adapt to the DevNet configuration:

### Automatic Partition Detection

Tests query the network to determine:
- Number of active BVNs
- Validator distribution
- Network topology

### Cross-Partition Testing

With 3+ BVNs, tests automatically:
- Distribute accounts across all partitions
- Test all cross-partition routes
- Measure inter-partition latency

### Example Test Adaptation

```go
// Tests automatically detect partition count
partitions := getActivePartitions()
for i := 0; i < partitions; i++ {
    // Create accounts in each partition
    createAccountInPartition(i)
}

// Test all cross-partition routes
for src := 0; src < partitions; src++ {
    for dst := 0; dst < partitions; dst++ {
        if src != dst {
            testCrossPartitionRoute(src, dst)
        }
    }
}
```

## Performance Tuning

### For Development (Fast Iteration)
```bash
# Minimal resources, fast startup
./devnet_config.sh start 2 1 0
```

### For Integration Testing
```bash
# Balanced setup
./devnet_config.sh start 2 3 1
```

### For Production Simulation
```bash
# High availability setup
./devnet_config.sh start 3 5 2
```

### For Stress Testing
```bash
# Maximum load
./devnet_config.sh start 5 3 2
```

## Continuous Integration

Example CI pipeline configuration:

```yaml
# .gitlab-ci.yml or .github/workflows/test.yml
test:
  script:
    - ./devnet_config.sh start 3 2 1
    - ./load_test_runner.sh suite full cross_chain
    - ./devnet_config.sh stop
```

## Troubleshooting

### View All Running Nodes
```bash
ps aux | grep accumulated
lsof -i :26656-26700
```

### Clean Restart
```bash
./devnet_config.sh clean
./devnet_config.sh start 3 3 1
```

### Debug Mode
```bash
# Edit devnet_config.sh to add debug flag
cmd="$cmd --debug"
```

## Best Practices

1. **Start Small**: Begin with minimal configuration and scale up
2. **Monitor Resources**: Watch memory and CPU usage with larger configurations
3. **Clean Between Tests**: Use `clean` command to ensure fresh state
4. **Use Configuration Files**: Store common configurations for reproducibility
5. **Automate Testing**: Use load_test_runner.sh for consistent test execution

## Future Enhancements

Planned improvements:
- [ ] Dynamic scaling (add/remove BVNs at runtime)
- [ ] Kubernetes deployment templates
- [ ] Grafana dashboard templates
- [ ] Automated performance regression testing
- [ ] Network partition simulation
- [ ] Byzantine fault injection

## Summary

The flexible DevNet configuration system eliminates the need to write custom code for different network topologies. Key benefits:

- **Easy Scaling**: Change BVNs and validators with simple parameters
- **Reproducible Testing**: Configuration files ensure consistent test environments
- **Automated Testing**: Integration with load test runner for comprehensive testing
- **Resource Efficient**: Configure only what you need for specific tests
- **CI/CD Ready**: Simple commands for integration into pipelines

This system makes it trivial to test Accumulate under various network conditions, from minimal development setups to large-scale stress testing scenarios.