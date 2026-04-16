# Sharding Architecture

## Overview

Accumulate uses a unified sharding system to parallelize transaction execution
within each Block Validator Node (BVN). Transactions are deterministically
routed to independent shards based on the identity (ADI) of the accounts they
touch. Each shard executes transactions against its own database batch with no
cross-shard locking, enabling near-linear throughput scaling on multi-core
hardware.

The default configuration uses **64 shards** (depth 6), which balances
parallelism against coordination overhead for typical server hardware.

## Why Sharding?

Without sharding, all transactions within a block execute sequentially against a
single database batch protected by a global lock. On modern hardware with 16-64
cores, this leaves most CPUs idle during block processing.

Sharding partitions the work so that transactions touching different accounts
execute in parallel on different cores with zero contention. Because Accumulate
routes by ADI (identity), all sub-accounts of a given identity land on the same
shard, preserving atomicity for same-identity operations without coordination.

## Design Decisions

### ADI-Based Routing

Transactions are routed by hashing the **identity URL** (the ADI), not the full
account URL. This means `acc://alice.acme/tokens` and `acc://alice.acme/data`
always land on the same shard. Benefits:

- Most transactions (token transfers, data writes) touch accounts under a single
  ADI and execute entirely within one shard.
- No cross-shard coordination for single-identity transactions.
- Shard assignment is deterministic and stateless -- any node can compute it from
  the URL alone.

The routing function:

```
shard_id = identity_hash[0] >> (8 - shard_depth)
```

Where `identity_hash` is a 32-byte hash of the ADI URL, and `shard_depth` is
`log2(shard_count)`. Only the first byte of the hash is used, which limits the
maximum shard count to 256.

### Power-of-Two Shard Counts

Shard counts must be powers of two (4, 8, 16, 32, 64, 128, 256). This allows
routing via bit-shifting instead of modular arithmetic, which is faster and
produces perfectly uniform distribution across shards.

### 64 Shards as Default

64 shards (depth 6) is the default because:

- It provides enough parallelism for 64-core servers.
- Each shard handles ~4 of the 256 possible first-byte values, giving uniform
  load with realistic account distributions.
- Beyond 64 shards, goroutine scheduling overhead and cache pressure begin to
  offset the parallelism gains.

## Components

### ShardedExecutor

**Location:** `internal/core/execute/sharded_executor.go`

The top-level orchestrator. It owns the array of `PerShardExecutor` instances and
provides methods for block lifecycle management:

- `NewShardedExecutor(shardCount, db)` -- creates the executor with validated
  shard count.
- `RouteAccount(url)` -- returns the shard ID for a given account URL.
- `BeginBlock()` -- opens a writable database batch on each shard.
- `ForEachShard(fn)` -- executes a function on all shards in parallel.
- `Commit()` / `Discard()` -- commits or discards all shard batches.
- `AccountsPerShard(accounts)` -- partitions a list of URLs by shard.
- `ExecuteTransactionOnShards(ctx, shardIDs, fn)` -- executes a transaction
  function across one or more shards with error handling and rollback.

### PerShardExecutor

**Location:** `internal/core/execute/per_shard_executor.go`

Manages a single shard's execution state:

- Owns an independent `database.Batch` for isolated reads and writes.
- Tracks which accounts have been loaded (for diagnostics).
- Provides mutex-protected access to its batch and account state.
- Supports `Commit()` and `Discard()` for batch lifecycle.

### ShardedBPT

**Location:** `pkg/database/bpt/sharded.go`

A parallel-safe Binary Patricia Tree that partitions at a configurable depth:

- Each shard is a standard `BPT` instance with its own padded mutex (64-byte
  cache-line alignment to prevent false sharing).
- `Insert`, `Get`, `Delete` route to the correct shard and acquire only that
  shard's lock.
- `GetRootHash()` computes shard root hashes in parallel, then combines them
  hierarchically to produce the same root hash as a non-sharded BPT.
- Storage format is identical to non-sharded BPT -- no migration needed.

### Shard Configuration

**Location:** `pkg/database/bpt/shard_config.go`

Validation and utility functions:

- `ValidateShardCount(count)` -- checks power-of-two, within [4, 256].
- `RouteToShard(keyHash, depth)` -- canonical routing function shared by BPT and
  executor sharding.
- `DefaultShardCount` = 64.

### Parallel Execution

**Location:** `internal/core/execute/parallel_execution.go`

Multi-shard transaction execution with:

- Single-shard fast path (no goroutine overhead for ~60% of transactions).
- Multi-shard parallel execution with WaitGroup synchronization.
- Panic recovery -- panics in shard goroutines are caught and converted to
  errors.
- All-or-nothing rollback -- if any shard fails, all affected shards are
  discarded.
- Context cancellation support.

## Transaction Flow

```
1. Transaction arrives at the block executor
2. Primary account URL is extracted from the transaction
3. ShardedExecutor.RouteAccount(url) computes shard ID from ADI hash
4. If transaction touches multiple ADIs (e.g., token transfer):
   a. Source and destination shard IDs are computed
   b. If same shard: single-shard execution (fast path)
   c. If different shards: multi-shard parallel execution
5. Transaction executes against the shard's database batch
6. At end of block:
   a. All shard batches are committed sequentially
   b. BPT root hash is computed from parallel shard roots
```

## Shard Routing Diagram

```
Account URL: acc://alice.acme/tokens
                    |
                    v
            IdentityAccountID32()
                    |
                    v
           hash[0] = 0xB7 = 10110111
                    |
                    v
        depth=6: 0xB7 >> 2 = 0x2D = 45
                    |
                    v
              Shard 45 of 64
```

All sub-accounts of `acc://alice.acme` produce the same identity hash, so they
all route to shard 45.

## Root Hash Computation

The ShardedBPT computes a root hash identical to a non-sharded BPT:

```
1. Each shard computes its own root hash IN PARALLEL
   (each shard's executePending() runs concurrently)

2. Shard roots are combined bottom-up in a virtual binary tree:

   Level 0:  [S0] [S1] [S2] [S3] ... [S62] [S63]
   Level 1:  [H(S1,S0)] [H(S3,S2)] ... [H(S63,S62)]
   Level 2:  [H(...)] [H(...)] ...
   ...
   Level 6:  [ROOT]

   Note: BPT uses inverted bit ordering (1=LEFT, 0=RIGHT),
   so pairs are hashed as H(odd, even), not H(even, odd).

3. The final root hash is identical to what a non-sharded BPT
   with the same data would produce.
```

## Thread Safety Model

- **No global locks.** Each shard has its own mutex.
- **Per-shard isolation.** A shard's batch is only accessed under that shard's
  lock. Different shards never contend.
- **Padded mutexes.** Shard mutexes are padded to 64 bytes (one cache line) to
  prevent false sharing on x86/ARM architectures.
- **Deterministic routing.** The same account always routes to the same shard, so
  there is no need for cross-shard locking for single-ADI transactions.
- **Multi-shard transactions.** When a transaction touches multiple shards, each
  shard's work runs in its own goroutine. Results are collected via channels.
  Panics are recovered. Failures trigger rollback of all affected shards.
