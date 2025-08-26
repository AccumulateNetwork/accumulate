# DevNet Testing Guide with CCC Gap Recovery

## Overview

This branch includes a sophisticated DevNet configuration system and CrossChain Conductor (CCC) with pause/resume functionality for testing gap recovery.

## Key Components

### 1. DevNet Configuration Script (`devnet_config.sh`)

A flexible script to launch DevNet with custom configurations:

```bash
# Start with specific configuration
./devnet_config.sh start <bvns> <validators> <followers>

# Examples:
./devnet_config.sh start 3 3 2         # 3 BVNs, 3 validators, 2 followers each
./devnet_config.sh quick               # Minimal: 2 BVNs, 1 validator each
./devnet_config.sh standard            # Standard: 2 BVNs, 3 validators, 1 follower
./devnet_config.sh large               # Large: 3 BVNs, 3 validators, 2 followers
./devnet_config.sh multi               # Multi-partition: 5 BVNs for cross-chain testing
```

### 2. CCC Pause/Resume Testing

The CCC includes pause/resume functionality (when built with `testnet` tag) for simulating network partitions:

#### Building with Testnet Tag
```bash
# Build accumulate with testnet features enabled
go build -tags testnet -o accumulated ./cmd/accumulated

# Start DevNet with testnet build
./devnet_config.sh start 3 3 1
```

#### Using Interactive Pause Test
```bash
# Run the interactive pause test
./interactive_pause_test.sh

# This provides a menu to:
# - Pause/resume individual partitions
# - Monitor gap detection
# - Test recovery mechanisms
```

## Gap Recovery Testing Workflow

### Step 1: Start DevNet with Multiple Partitions
```bash
# Clean any existing DevNet
./devnet_config.sh clean

# Start with 3 BVNs for testing cross-partition communication
./devnet_config.sh start 3 3 1
```

### Step 2: Generate Load
```bash
# In another terminal, start generating transactions
./devnet_load_test.sh

# This creates continuous cross-partition traffic
```

### Step 3: Simulate Partition Failure
```bash
# Use the interactive test to pause a partition
./interactive_pause_test.sh

# Select option to pause BVN1
# This simulates network partition - messages to/from BVN1 are dropped
```

### Step 4: Observe Gap Detection
While BVN1 is paused:
- Other partitions continue generating messages
- Sequence gaps accumulate
- Messages that would go to BVN1 are queued

### Step 5: Resume and Watch Recovery
```bash
# Resume BVN1 through the interactive menu
# Watch as:
# 1. BVN1 detects gaps in sequences
# 2. Sends gap requests with LastKnownSequence
# 3. Source partitions reset their SentTxIndex
# 4. Collection proofs batch all missing messages
# 5. Gaps are filled efficiently
```

## Key Features Demonstrated

### Simple Index-Based Recovery
- Each partition tracks `SentTxIndex` per destination
- On failure: Index doesn't advance
- On gap request: Index resets to what destination has
- Next send includes everything from that point

### Collection Proof Efficiency
- Multiple messages sent with single proof
- Reduces overhead during recovery
- Especially efficient for large gaps

### Self-Healing Properties
- No complex state machines
- Failed sends automatically retry with all pending messages
- System recovers automatically when connectivity returns

## Monitoring and Metrics

### Check CCC Status
```bash
# Check if CCC is paused (requires testnet build)
curl http://localhost:27010/debug/ccc/status

# View destination metrics
curl http://localhost:27010/debug/ccc/metrics
```

### Monitor Gap Recovery
```bash
# Watch logs for gap detection and recovery
tail -f devnet_config.log | grep -E "gap|recovery|reset"

# Key log messages:
# "Sequence gap detected" - Gap identified
# "Reset send index for gap recovery" - Recovery initiated
# "Successfully sent batch" - Recovery completed
```

## Configuration Files

Create custom configurations:

```bash
# Generate sample configs
./devnet_config.sh configs

# Load a specific configuration
./devnet_config.sh load devnet_configs/multi_partition.conf
```

Example configuration file:
```bash
# multi_partition.conf
BVNS=5
VALIDATORS=2
FOLLOWERS=1
```

## Troubleshooting

### CCC Pause Not Working
- Ensure built with `testnet` tag: `go build -tags testnet`
- Check debug endpoint: `curl http://localhost:27010/debug/ccc/status`

### Gap Recovery Not Triggering
- Verify messages are being sent between partitions
- Check sequence tracking: Messages must be sequenced
- Monitor logs for gap detection

### Performance Issues
- Reduce partition count for testing
- Use `quick` configuration for minimal setup
- Monitor resource usage with `top` or `htop`

## Advanced Testing

### Cascade Failures
1. Start with 5 partitions (`multi` config)
2. Pause multiple partitions simultaneously
3. Resume in different orders
4. Verify all gaps are recovered

### Large Gap Recovery
1. Pause a partition for extended period
2. Generate hundreds of messages
3. Resume and measure recovery time
4. Monitor collection proof sizes

### Network Jitter Simulation
1. Rapidly pause/resume partitions
2. Verify no message loss
3. Check for duplicate processing
4. Monitor performance impact

## Summary

This branch provides a complete testing environment for the CrossChain Conductor's gap recovery mechanism:

- **Flexible DevNet**: Configure any number of BVNs, validators, and followers
- **Pause/Resume**: Simulate network partitions with testnet build
- **Gap Recovery**: Simple index-based recovery with collection proofs
- **Interactive Testing**: Menu-driven testing interface
- **Comprehensive Monitoring**: Metrics and logging for debugging

The system demonstrates that gap recovery can be simple yet effective, using just index tracking and collection proofs to handle network disruptions efficiently.