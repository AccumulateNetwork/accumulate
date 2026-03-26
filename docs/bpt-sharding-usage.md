# BPT Parallel Sharding - Usage Guide

## Overview

The Binary Patricia Tree (BPT) now supports optional parallel sharding for improved performance on multi-core systems. When enabled, the tree is partitioned into independent shards that can be updated concurrently with zero contention.

## Configuration

### Config File (TOML)

Add the following to your `accumulate.toml` configuration file:

```toml
[bpt.sharding]
enabled = true    # Enable parallel sharding (default: false)
depth = 4         # Number of shard bits: 4 = 16 shards (default: 4)
```

### Recommended Shard Depths

| Depth | Shards | Best For              |
|-------|--------|-----------------------|
| 3     | 8      | 8-core systems        |
| 4     | 16     | 16-core systems       |
| 5     | 32     | 32-core systems       |
| 6     | 64     | 64+ core systems      |

**Note:** Depths beyond 6 show diminishing returns. Start with depth=4 (16 shards) and tune based on your workload.

## Usage Examples

### Basic Usage (No Code Changes Required)

If you're using the standard database integration, no code changes are needed. Simply update your configuration:

```toml
# Enable sharding in accumulate.toml
[bpt.sharding]
enabled = true
depth = 4
```

The system will automatically use `ShardedBPT` when enabled, or regular `BPT` when disabled.

### Direct API Usage

If you're creating BPT instances directly in code, use the factory function:

```go
import (
    "gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
    "gitlab.com/accumulatenetwork/accumulate/internal/node/config"
)

// Load configuration
cfg, err := config.Load(configDir)
if err != nil {
    return err
}

// Validate BPT configuration
if err := cfg.Accumulate.BPT.Validate(); err != nil {
    return fmt.Errorf("invalid BPT config: %w", err)
}

// Create BPT using configuration
bptConfig := bpt.Config{
    ShardingEnabled: cfg.Accumulate.BPT.Sharding.Enabled,
    ShardDepth:      int(cfg.Accumulate.BPT.Sharding.Depth),
}

tree, err := bpt.NewFromConfig(bptConfig, store, key)
if err != nil {
    return err
}

// Use tree normally - same API for both sharded and non-sharded
tree.Insert(key, value)
rootHash, err := tree.GetRootHash()
```

### Using the BPTree Interface

For maximum flexibility, use the `BPTree` interface which works with both implementations:

```go
var tree bpt.BPTree

if useSharding {
    tree, err = bpt.NewShardedBPT(store, key, 4)
} else {
    tree = bpt.New(nil, nil, store, key)
}

// Same API for both implementations
tree.Insert(key, value)
value, err := tree.Get(key)
tree.Delete(key)
rootHash, err := tree.GetRootHash()
```

## Migration Guide

### Enabling Sharding on Existing Nodes

Sharding can be enabled on existing nodes **without migration**:

1. **Stop the node**
2. **Update configuration:**
   ```toml
   [bpt.sharding]
   enabled = true
   depth = 4
   ```
3. **Start the node**

The existing BPT data remains compatible. The sharded implementation reads the same data structure and produces identical root hashes.

### Disabling Sharding

To disable sharding (revert to regular BPT):

1. Stop the node
2. Set `enabled = false` in configuration
3. Start the node

No data migration is needed in either direction.

## Performance Tuning

### Choosing Shard Depth

Start with the default (depth=4, 16 shards) and monitor:

- **CPU Usage:** Should be near 100% across all cores during heavy updates
- **Latency:** Should decrease proportionally with core count
- **Throughput:** Should scale linearly with cores (up to ~16-32 cores)

If CPU usage is low (<80%), try increasing depth by 1. If diminishing returns, keep current depth.

### Monitoring

Key metrics to track:

- **Block commit time:** Should decrease with sharding
- **GetRootHash latency:** Parallel execution across shards
- **Insert/Update throughput:** Should scale with core count

### Benchmarking

Run benchmarks to compare performance:

```bash
# Benchmark without sharding
go test -bench=BenchmarkBPT -benchtime=10s ./pkg/database/bpt

# Benchmark with sharding (depth=4, 16 shards)
go test -bench=BenchmarkSharded -benchtime=10s ./pkg/database/bpt
```

Look for speedups in:
- Parallel inserts (should scale with core count)
- Root hash computation (embarrassingly parallel)
- Concurrent updates (zero contention between shards)

## Validation

The system validates configuration on startup:

```go
if err := cfg.Accumulate.BPT.Validate(); err != nil {
    log.Fatalf("Invalid BPT configuration: %v", err)
}
```

Valid ranges:
- **Depth:** 1-8 (only validated if sharding is enabled)
- **Enabled:** true/false

## Technical Details

### How Sharding Works

1. **Key Routing:** Keys are routed to shards using high-order bits of the key hash
2. **Independent Updates:** Each shard is a complete BPT with its own lock
3. **Parallel Root Hash:** Shard roots are computed in parallel, then combined hierarchically
4. **Same Structure:** Storage format is identical to non-sharded BPT

### Root Hash Equivalence

The sharded BPT produces **identical root hashes** to the non-sharded BPT for the same data. This is guaranteed by:

1. Using the same tree structure and routing logic
2. Combining shard roots with the same hash semantics
3. Extensive testing (see `TestRootHashEquivalence`)

### Thread Safety

- **Per-shard locking:** Each shard has its own mutex
- **Zero contention:** Operations on different shards never block each other
- **Concurrent reads/writes:** Full parallelism within locking constraints

## Troubleshooting

### "shard depth must be between 1 and 8"

Check your configuration:
```toml
[bpt.sharding]
depth = 4  # Must be 1-8
```

### Performance Not Improving

- Verify sharding is enabled: Check logs on startup
- Monitor CPU usage: Should approach 100% across all cores
- Try increasing depth: Add 1 to current depth and retest
- Check workload: Sharding benefits write-heavy workloads most

### Root Hash Mismatch

This should never happen. If it does:
1. Report immediately (critical bug)
2. Include: shard depth, data size, reproduction steps
3. Check for custom BPT modifications

## Examples

### Complete Configuration Example

```toml
# accumulate.toml

[describe]
type = "block-validator"
partition-id = "BVN0"

[storage]
type = "badger"
path = "data/accumulate.db"

[bpt.sharding]
enabled = true
depth = 4  # 16 shards for 16-core system

[api]
listen-address = "tcp://0.0.0.0:16695"
```

### Programmatic Configuration

```go
// Create default config
cfg := config.Default("devnet", protocol.PartitionTypeBlockValidator, config.Validator, "BVN0")

// Enable BPT sharding
cfg.Accumulate.BPT.Sharding.Enabled = true
cfg.Accumulate.BPT.Sharding.Depth = 4

// Validate
if err := cfg.Accumulate.BPT.Validate(); err != nil {
    log.Fatalf("Invalid config: %v", err)
}

// Save
if err := config.Store(cfg); err != nil {
    log.Fatalf("Failed to save config: %v", err)
}
```

## Best Practices

1. **Start Disabled:** Default is disabled for backward compatibility
2. **Test First:** Enable on test environments before production
3. **Monitor:** Track performance metrics before/after enabling
4. **Right-size:** Match shard count to core count (depth=4 for 16 cores)
5. **Document:** Note configuration in deployment docs

## Additional Resources

- **Implementation:** `pkg/database/bpt/sharded.go`
- **Tests:** `pkg/database/bpt/sharded_test.go`
- **Factory:** `pkg/database/bpt/factory.go`
- **Configuration:** `internal/node/config/config.go`

## Support

For issues or questions:
- Check logs for validation errors
- Review metrics for performance issues
- Report bugs with full reproduction steps
