# Sharding Performance

## Throughput Target

The sharding system targets **10,000+ TPS** on multi-node deployments with
64-shard configurations on 32-64 core servers.

## BPT Sharded Performance

The ShardedBPT provides the most directly measurable performance improvement.
Benchmarks compare sharded vs non-sharded BPT operations under concurrent load.

### Insert Operations

With a non-sharded BPT, all concurrent inserts serialize on a single mutex.
The sharded BPT distributes inserts across independent shard locks, enabling
true parallelism.

Expected scaling pattern:

| Goroutines | Non-Sharded | 16 Shards | 32 Shards | 64 Shards |
|------------|-------------|-----------|-----------|-----------|
| 1          | Baseline    | ~1x       | ~1x       | ~1x       |
| 4          | ~1x (contended) | ~3-4x | ~3-4x    | ~3-4x    |
| 8          | ~1x (contended) | ~6-8x | ~7-8x    | ~7-8x    |
| 16         | ~1x (contended) | ~12-16x | ~14-16x | ~14-16x  |
| 32         | ~1x (contended) | ~12-16x | ~20-28x | ~24-32x  |

Single-goroutine performance has slight overhead from shard routing (~5-10ns
per operation) which is negligible compared to the BPT insert cost.

### Root Hash Computation

`GetRootHash()` is the critical path at end-of-block. Each shard's
`executePending()` runs in parallel, then shard roots are combined
hierarchically.

With 10,000 pending entries across 64 shards:
- Non-sharded: all entries processed sequentially.
- 64 shards: ~156 entries per shard processed in parallel, then a fast
  6-level hash tree combine.

The parallel `executePending()` phase dominates, giving near-linear speedup
proportional to the number of shards (bounded by available cores).

### Lookup Operations

Concurrent reads scale similarly to inserts because each shard has its own lock.
Non-sharded BPT serializes all reads; sharded BPT allows concurrent reads on
different shards.

## Transaction Execution Performance

### Single-Shard Transactions

Approximately 60% of transactions touch only one ADI and execute entirely
within a single shard. These use the fast path in `ExecuteTransactionOnShards`:
no goroutine allocation, no channel communication, no WaitGroup.

Overhead vs direct execution: effectively zero (one function call + shard
lookup).

### Cross-Shard Transactions

Transactions touching two or more ADIs may span multiple shards. These use
the parallel execution path:

- Goroutine per shard + WaitGroup + result channel.
- Overhead: ~2-5 microseconds for goroutine setup and synchronization.
- Execution within each shard runs in parallel.

Cross-shard overhead is 2.3x-2.8x compared to single-shard execution for the
coordination cost alone. The actual transaction execution time within each
shard is unchanged.

### Rollback Cost

When any shard fails during multi-shard execution, all affected shards are
rolled back via `Discard()`. The rollback cost is proportional to the number
of affected shards (typically 2) and the amount of state modified.

## Lock Contention

### False Sharing Prevention

Shard mutexes are padded to 64 bytes (one cache line) to prevent false sharing:

```go
type paddedMutex struct {
    mu sync.Mutex
    _  [56]byte // Padding to 64 bytes
}
```

Without padding, adjacent mutexes share a cache line, causing severe
performance degradation (up to 10x slowdown) on multi-socket systems.

### Contention Profile

Use Go's block profiler to measure actual contention:

```bash
go test -bench=BenchmarkContention -blockprofile=block.out ./pkg/database/bpt/...
go tool pprof -http=:8080 block.out
```

Expected contention patterns:
- **Low shard count + many goroutines:** High contention, visible as time spent
  in `sync.Mutex.Lock`.
- **High shard count + few goroutines:** Near-zero contention, but goroutine
  scheduling overhead increases.
- **Matched shard count and core count:** Optimal -- minimal contention with
  full CPU utilization.

## Memory Overhead

Each shard maintains:
- A `BPT` instance (tree nodes cached in memory).
- A `paddedMutex` (64 bytes per shard).
- A `database.Batch` (during block execution).

For 64 shards:
- Mutex array: 64 * 64 = 4 KB
- BPT instances: proportional to data size, distributed across shards.
- Database batches: 64 independent write batches during block processing.

The memory overhead of sharding itself is minimal. The dominant cost is the
database batch state, which scales with the number of accounts modified per
block, not the number of shards.

## Running Benchmarks

### BPT Benchmarks

```bash
# All sharded benchmarks
go test -bench=BenchmarkSharded -benchmem ./pkg/database/bpt/... 2>/tmp/bench.log
tail -50 /tmp/bench.log

# Direct comparison: sharded vs non-sharded
go test -bench=BenchmarkShardedVsNonSharded -benchmem ./pkg/database/bpt/... 2>/tmp/bench.log

# Root hash computation (critical path)
go test -bench=BenchmarkGetRootHash -benchmem ./pkg/database/bpt/... 2>/tmp/bench.log

# Lock contention analysis
go test -bench=BenchmarkContention -benchmem -blockprofile=block.out ./pkg/database/bpt/... 2>/tmp/bench.log

# Mixed workload (70% reads, 30% writes)
go test -bench=BenchmarkMixedWorkload -benchmem ./pkg/database/bpt/... 2>/tmp/bench.log

# Memory allocation patterns
go test -bench=BenchmarkShardedMemory -benchmem ./pkg/database/bpt/... 2>/tmp/bench.log
```

### Executor Benchmarks

```bash
# Parallel execution tests (verify wall-clock parallelism)
go test -v -run TestConcurrency_ParallelShardExecution ./internal/core/execute/... 2>/tmp/test.log

# Race condition detection
go test -race ./internal/core/execute/... 2>/tmp/race.log
go test -race ./pkg/database/bpt/... 2>/tmp/race.log
```

## Scalability Expectations

### Throughput vs Shard Count

With uniform account distribution and sufficient CPU cores:

| Shards | Expected Speedup | Notes |
|--------|-----------------|-------|
| 4      | ~3.5x           | Good for development |
| 8      | ~7x             | |
| 16     | ~14x            | Typical for 16-core servers |
| 32     | ~25x            | Scheduling overhead begins |
| 64     | ~40-50x         | Production default |
| 128    | ~60-80x         | Diminishing returns |
| 256    | ~80-100x        | Maximum, requires 128+ cores |

Speedup is sub-linear at high shard counts due to:
- Sequential commit phase (all batches write to one database).
- Goroutine scheduling overhead.
- Memory bandwidth contention.
- Possible hot ADIs creating shard imbalance.

### Bottleneck Analysis

At high throughput, the likely bottleneck progression is:

1. **Transaction execution** (addressed by sharding).
2. **Sequential commit** (64 batches commit one at a time).
3. **BPT root hash** (partially parallelized, combine phase is sequential).
4. **Consensus / network** (outside sharding scope).
5. **Disk I/O** (outside sharding scope).
