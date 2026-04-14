# Sharding Operations Guide

## Configuration

### Shard Count

The shard count determines how many independent execution lanes process
transactions in parallel. Valid values are powers of two from 4 to 256:

| Shards | Depth | Recommended For |
|--------|-------|-----------------|
| 4      | 2     | Development, testing, low-core VMs |
| 8      | 3     | 8-core servers |
| 16     | 4     | 16-core servers |
| 32     | 5     | 32-core servers |
| **64** | **6** | **Production default, 32-64 core servers** |
| 128    | 7     | High-core-count servers (64+ cores) |
| 256    | 8     | Maximum parallelism, 128+ cores |

The default is **64 shards**. This is appropriate for most production
deployments. Only change it if you have specific performance data indicating a
different value would be better.

### When to Adjust Shard Count

**Increase shards when:**
- CPU utilization during block processing is low despite high transaction volume.
- You have more CPU cores than the current shard count.
- Block processing time is dominated by transaction execution (not I/O or
  consensus).

**Decrease shards when:**
- Most shards are idle during block processing (uneven load).
- Goroutine scheduling overhead is significant (visible in CPU profiles as
  `runtime.schedule` or `runtime.findrunnable`).
- Memory pressure is high -- each shard maintains its own database batch.

**Do not change shards when:**
- The bottleneck is disk I/O, network, or consensus.
- Transaction volume is low (sharding overhead exceeds parallelism benefit).

### Setting the Shard Count

The shard count is configured as part of the node's execution configuration.
It takes effect at the next block boundary -- in-flight blocks complete with
the previous shard count.

## Monitoring

### Per-Shard Execution Metrics

Each `PerShardExecutor` tracks:

- **Loaded account count:** Number of unique accounts accessed per shard per
  block. Check for uneven distribution -- if one shard handles 10x the accounts
  of others, the load is skewed.

### Diagnosing Uneven Load

Uneven shard load happens when a small number of ADIs generate most of the
transaction volume. Since routing is based on ADI hash, all transactions for a
popular identity land on the same shard.

**Indicators:**
- One or two shards consistently take longer than others during block processing.
- `LoadedAccountCount()` varies widely across shards.

**Remediation:**
- Uneven load is a natural consequence of ADI-based routing and cannot be
  eliminated without breaking the single-ADI-single-shard guarantee.
- Increasing the shard count will not help if the skew is caused by a single
  hot ADI -- that ADI's shard will still be the bottleneck.
- If a single ADI is the bottleneck, the solution is to spread its activity
  across multiple ADIs at the application level.

### Block Processing Timeline

A typical block with 64 shards processes as follows:

```
Time ->

BeginBlock:     [Open 64 batches]                    ~microseconds
Execute:        [====Shard 0====]
                [====Shard 1====]
                [====Shard 2====]
                   ...                               ~parallel
                [====Shard 63===]
Commit:         [S0][S1][S2]...[S63]                 ~sequential
BPT Root Hash:  [====Parallel hash====][Combine]     ~parallel + sequential
```

The commit phase is sequential because all shard batches write to the same
underlying database. The BPT root hash computation parallelizes the
per-shard `executePending()` calls, then combines roots in a fast hierarchical
hash.

## Troubleshooting

### Block Processing Is Slow Despite Many Cores

1. Check that the shard count is appropriate for your core count.
2. Profile with `go tool pprof` -- look for lock contention in
   `sync.Mutex.Lock` or `runtime.semacquire`.
3. Check for hot ADIs causing shard skew (see "Diagnosing Uneven Load" above).
4. Check if the bottleneck is the sequential commit phase, not execution.

### Root Hash Mismatch Between Nodes

The sharded BPT produces the same root hash as a non-sharded BPT with the same
data. If nodes disagree on root hash:

1. This is NOT caused by different shard counts -- the root hash is
   shard-count-independent.
2. Check for data divergence (different transactions applied, or different
   execution order).
3. Check for database corruption.

### High Memory Usage

Each shard maintains its own database batch with cached state. With 64 shards,
memory usage is roughly 64x the per-batch overhead. If memory is constrained:

1. Reduce the shard count.
2. Monitor per-shard loaded account counts -- shards with many loaded accounts
   use more memory.

### Panics During Execution

Panics in multi-shard execution are caught by the parallel execution framework
and converted to errors. The panic message and stack will appear in the error.
All affected shards are rolled back.

For single-shard execution (the fast path), panics propagate normally and must
be caught by the caller.
