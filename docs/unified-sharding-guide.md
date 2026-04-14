# Unified Sharding Guide

This is a consolidated quick-start guide for Accumulate's unified sharding
system. For detailed information, see the individual documents:

- [Architecture](./sharding-architecture.md) -- Design, components, routing
- [Operations](./sharding-operations.md) -- Configuration, monitoring, troubleshooting
- [Development](./sharding-development.md) -- Integration, testing, debugging
- [Performance](./sharding-performance.md) -- Benchmarks, scalability

---

## What is Unified Sharding?

Accumulate partitions transaction execution within each BVN into independent
shards. Each shard processes transactions for a deterministic subset of accounts
in parallel. The same routing algorithm is used for both transaction execution
(ShardedExecutor) and BPT updates (ShardedBPT), hence "unified."

**Default: 64 shards.** Configurable: 4, 8, 16, 32, 64, 128, 256.

## How It Works (30-Second Summary)

1. Each account URL is hashed by its ADI (identity).
2. The top bits of the hash select a shard: `hash[0] >> (8 - depth)`.
3. All accounts under the same ADI always land on the same shard.
4. Each shard has its own database batch and lock -- zero cross-shard contention.
5. At end of block, shards commit sequentially and BPT root hash is computed in parallel.

## Getting Started

### For Operators

The default shard count (64) is appropriate for most deployments. No
configuration is needed unless you are tuning for specific hardware.

See the [shard count table](./sharding-operations.md#shard-count) for
recommendations by core count.

Key monitoring points:
- Per-shard loaded account counts (check for skew)
- Block processing time (should decrease with more shards)
- Memory usage (each shard maintains its own batch)

### For Developers

Transaction handlers do not need to change. Sharding is transparent -- handlers
receive a database batch and operate on it normally.

When adding new transaction types:
- Single-ADI transactions: no special handling needed.
- Cross-ADI transactions: provide the list of affected shard IDs to
  `ExecuteTransactionOnShards`.

See the [developer guide](./sharding-development.md) for code examples.

### For Performance Engineers

Run benchmarks:
```bash
go test -bench=BenchmarkShardedVsNonSharded -benchmem ./pkg/database/bpt/... 2>/tmp/bench.log
tail -50 /tmp/bench.log
```

Key metrics:
- Single-shard overhead: ~0 (fast path, no goroutines)
- Cross-shard overhead: 2.3-2.8x coordination cost
- Scalability: near-linear up to core count, then diminishing returns

See the [performance guide](./sharding-performance.md) for full benchmark suite.

## Cross-Shard Transaction Patterns

### Single-ADI (60% of transactions)

Transactions that touch only accounts under one ADI execute entirely within
one shard. Examples: data writes, single-account token operations, key updates.

No cross-shard coordination needed. Uses the single-shard fast path.

### Cross-ADI (40% of transactions)

Transactions that touch accounts under different ADIs may span multiple shards.
Example: token transfer from `acc://alice.acme/tokens` to `acc://bob.acme/tokens`.

The parallel execution framework handles these:
- Each affected shard runs in its own goroutine.
- All-or-nothing semantics: if any shard fails, all are rolled back.
- Panics in shard goroutines are caught and converted to errors.

### Shard Affinity

Because routing is ADI-based, applications that keep related accounts under
a single ADI get the best performance (all operations are single-shard).

Applications that spread activity across many ADIs benefit from parallelism
but may incur cross-shard overhead for inter-ADI transactions.

## Rollback Procedures

### Transaction-Level Rollback

If a transaction fails during multi-shard execution, `rollbackShards` discards
all affected shard batches. No partial state is committed.

### Block-Level Rollback

Call `ShardedExecutor.Discard()` to discard all shard batches for the current
block. This is the same as calling `PerShardExecutor.Discard()` on each shard.

### Shard Count Change

Changing the shard count requires no data migration. The shard count is pure
in-memory routing -- the same data can be read with any valid shard count.
Changes take effect at the next block boundary.

To revert a shard count change, set it back to the previous value. No rollback
of data is needed.

## Troubleshooting Quick Reference

| Symptom | Likely Cause | Action |
|---------|-------------|--------|
| Slow blocks despite many cores | Wrong shard count | Match shard count to core count |
| One shard much slower than others | Hot ADI | Spread activity across ADIs |
| High memory usage | Too many shards | Reduce shard count |
| Root hash mismatch between nodes | Data divergence | Not caused by sharding -- check transaction ordering |
| Panic in shard execution | Bug in transaction handler | Check error message for shard ID and stack trace |

## Source Files

| File | Description |
|------|-------------|
| `internal/core/execute/sharded_executor.go` | Top-level orchestrator |
| `internal/core/execute/per_shard_executor.go` | Per-shard batch management |
| `internal/core/execute/parallel_execution.go` | Multi-shard execution with rollback |
| `pkg/database/bpt/sharded.go` | Sharded Binary Patricia Tree |
| `pkg/database/bpt/shard_config.go` | Validation and routing functions |
