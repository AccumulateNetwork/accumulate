# Unified Sharding Architecture: BPT + Transaction Execution

**Status:** Design Phase  
**Priority:** High (enables 10K+ TPS)  
**Date:** 2026-04-14

---

## Executive Summary

The BPT (Binary Patricia Tree) already has a sophisticated sharding implementation using power-of-2 partitioning (16-64 shards) with per-shard locks and parallel root hash computation.

**Proposal:** Use the **same sharding mechanism** for transaction execution within a partition. Both BPT updates and transaction execution can be sharded by key/account address using identical routing, enabling full parallelism within each partition.

**Expected Impact (with 64 shards):**
- 12-16x TPS improvement per partition (with 64-core systems)
- Full parallelism for transaction execution (64 goroutines)
- Unified codebase (same sharding logic for BPT + transactions)
- Deterministic account routing (same account always → same shard)
- Configurable shard count (power-of-2: 4, 8, 16, 32, 64, 128, 256)

---

## Current State: BPT Sharding (IMPLEMENTED ✓)

### ShardedBPT Architecture

**Location:** `pkg/database/bpt/sharded.go`

**Design:**
```
ShardedBPT (16-64 shards)
├─ Shard 0 (BPT instance)
├─ Shard 1 (BPT instance)
├─ ...
└─ Shard N (BPT instance)

Routing: key[0] >> (8 - shardDepth) → shard ID
```

**Synchronization:**
- Per-shard `paddedMutex` (cache-line padded to prevent false sharing)
- Parallel `GetRootHash()` using goroutines
- Zero contention between different shards

**Key Features:**
- Configurable depth (4-6 bits = 16-64 shards)
- Cache-line padding in `paddedMutex` struct
- Hierarchical root hash combining
- Zero database changes (storage format identical)

### Performance

Current BPT performance with sharding:
- **4 shards:** ~3.5x speedup
- **16 shards:** ~10-12x speedup
- **32 shards:** ~12-16x speedup

---

## Transaction Execution: Current State (NEEDS SHARDING)

### Current Processing

**Problem:** Transactions are processed sequentially per partition
```go
Block Execution:
├─ Transaction 1 → Account A (sequential)
├─ Transaction 2 → Account B (sequential)
├─ Transaction 3 → Account A (sequential)
└─ Transaction 4 → Account C (sequential)

Bottleneck: All updates go to single BPT → lock contention
```

### Issue: Sequential Account Processing

1. Transactions processed in order
2. Each updates accounts independently
3. All BPT updates serialized through single lock
4. Limited by single-core speed

---

## Unified Sharding Proposal

### Architecture: Shard by Account/Key (64 Shards)

**Concept (64 shards = shardDepth 6):**
```
Block:
├─ Shard 0 (N transactions) → Account routing by key[0] >> 2 = 0
├─ Shard 1 (M transactions) → Account routing by key[0] >> 2 = 1
├─ ...
├─ Shard 32 (K transactions) → Accounts with key[0] >> 2 = 32
├─ ...
└─ Shard 63 (L transactions) → Accounts with key[0] >> 2 = 63

All 64 shards execute in PARALLEL goroutines with:
  - Independent account locks (per shard)
  - Dedicated BPT shard instance
  - Zero inter-shard contention
```

### Routing Function (UNIFIED)

```go
// With 64 shards: shardDepth = 6 (2^6 = 64)
// key[0] >> (8 - 6) = key[0] >> 2
// Extracts bits 7-2 from first byte → 64 possible values (0-63)

func RouteToShard(accountKey [32]byte, shardDepth int) int {
    // SAME routing as BPT
    // For 64 shards (depth=6): key[0] >> 2 gives shard 0-63
    return int(accountKey[0] >> (8 - shardDepth))
}

// Example with 64 shards:
// accountKey[0] = 0b11010110 (214)
// shift >> 2 = 0b00110101 = 53 → Shard 53
//
// accountKey[0] = 0b00001010 (10)
// shift >> 2 = 0b00000010 = 2 → Shard 2

// Used for:
// 1. BPT operations (INSERT, UPDATE)
// 2. Account state changes
// 3. Transaction dispatch

// Same shard index always → same cache locality → same BPT instance
```

### Execution Flow: Parallel by Shard

```
StartBlock:
  ├─ For each shard in parallel:
  │   ├─ Create account lock set for shard
  │   ├─ Collect transactions routed to shard
  │   ├─ Process in order:
  │   │   ├─ Transaction 1: Lock account(s), execute, update state
  │   │   ├─ Transaction 2: Lock account(s), execute, update state
  │   │   └─ Transaction N: Lock account(s), execute, update state
  │   ├─ All BPT updates go to shard's BPT
  │   └─ Unlock accounts
  │
  └─ Wait for all shards to complete (WaitGroup)

EndBlock:
  ├─ GetRootHash() (parallel across shards)
  ├─ Finalize block state
  └─ Broadcast

// Zero contention between shards!
// Same shard = serialized but isolated from others
```

### Critical Design Points

1. **Account Lock Routing**
   ```go
   type ShardedAccountLocks struct {
       shardDepth int
       locks      []*AccountLockSet  // Per-shard account locks
   }
   
   func (sal *ShardedAccountLocks) Lock(account *url.URL) {
       shard := RouteToShard(account.Hash(), sal.shardDepth)
       sal.locks[shard].Lock(account)
   }
   ```

2. **Transaction Dispatch**
   ```go
   // Group transactions by shard (zero-copy, just index)
   shardedTxns := make([][]Transaction, numShards)
   for _, txn := range block.Transactions {
       shard := RouteToShard(txn.Principal, shardDepth)
       shardedTxns[shard] = append(shardedTxns[shard], txn)
   }
   
   // Execute each shard in parallel
   for shard := range shardedTxns {
       go executeShardTransactions(shard, shardedTxns[shard])
   }
   ```

3. **Cross-Shard Transactions**
   ```
   Problem: Transaction touches accounts in multiple shards
   
   Solutions:
   A) Two-phase locking: Acquire locks in deterministic order
      Lock all affected accounts (across shards) before executing
      
   B) Single-shard routing: Route by primary account only
      Secondary accounts same-shard constraint in validation
      
   C) Deadlock detection: Timeout + retry in different epoch
      
   Recommendation: Option A (two-phase locking)
   - Prevents deadlock with consistent ordering
   - Serializes only cross-shard transactions (rare)
   - Single-shard transactions have zero contention
   ```

---

## Shard Configuration

### Power-of-2 Requirement

Shards **MUST** be a power of 2 to align with BPT tree structure:

| Shards | Depth | Bits Used | Recommended For |
|--------|-------|-----------|-----------------|
| 4 | 2 | 2 bits | Small clusters |
| 8 | 3 | 3 bits | Medium clusters |
| 16 | 4 | 4 bits | Standard (16-core) |
| **64** | **6** | **6 bits** | **Production default** |
| 256 | 8 | 8 bits | Very large (256+ cores) |

**Rationale:**
- Routing uses bit-shift: `key[0] >> (8 - shardDepth)`
- Power-of-2 shards = clean binary alignment
- BPT naturally partitions at power-of-2 boundaries
- Cache-line effects optimal at 64 shards (typical L3 cache)

### Default Configuration

```go
type ExecutorConfig struct {
    // EnableSharding: if true, use 64 shards by default
    EnableSharding bool
    
    // ShardCount: must be power of 2 (4, 8, 16, 32, 64, 128, 256, ...)
    // If not power of 2, NewShardedExecutor returns error
    // Default: 64 (recommended for production)
    ShardCount int
}

// Usage
cfg := ExecutorConfig{
    EnableSharding: true,
    ShardCount: 64,  // Standard production default
}

exec, err := NewExecutor(cfg)
if err != nil {
    // Handle invalid ShardCount (not power of 2)
}
```

### Validation Function

```go
// IsPowerOfTwo returns true if n is a power of 2
func IsPowerOfTwo(n int) bool {
    return n > 0 && (n & (n - 1)) == 0
}

// ShardDepthFromCount returns the bit depth for a given shard count
// e.g., 64 shards → depth 6
func ShardDepthFromCount(count int) (int, error) {
    if !IsPowerOfTwo(count) {
        return 0, fmt.Errorf("shard count must be power of 2, got %d", count)
    }
    // Find log2(count)
    depth := 0
    for (1 << depth) < count {
        depth++
    }
    return depth, nil
}
```

---

## Implementation Plan

### Phase 1: Design & Validation (2-3 days)

1. **Finalize shard count = 64**
   - Validate power-of-2 constraint (depth = 6)
   - Document routing algorithm
   - Confirm cache-line optimization

2. **Formalize account lock semantics**
   - How many locks per transaction?
   - Lock order for determinism
   - Deadlock prevention strategy

3. **Analyze cross-shard transactions**
   - What % of transactions touch multiple accounts?
   - Different shards or same shard?
   - Performance impact with 64 shards

4. **Design state finalization**
   - How to consolidate shard state?
   - BPT root hash computation (already parallel)
   - Validator set updates

### Phase 2: Core Implementation (5-7 days)

1. **Add configuration to database schema** (~100 lines)
   ```go
   // In database package
   type ExecutorConfig struct {
       ExecutorShardCount int  // Power of 2: 4, 8, 16, 32, 64, ...
   }
   
   // Methods:
   // batch.GetExecutorConfig() (*ExecutorConfig, error)
   // batch.PutExecutorConfig(*ExecutorConfig) error
   ```

2. **Create `ShardedExecutor` wrapper** (~500 lines)
   ```go
   type ShardedExecutor struct {
       shardCount int
       shardDepth int
       executors  []*Executor          // Per-shard executor instances
       locks      *ShardedAccountLocks
       txnQueue   chan *Delivery
   }
   
   func NewShardedExecutor(shardCount int) (*ShardedExecutor, error) {
       if !IsPowerOfTwo(shardCount) {
           return nil, fmt.Errorf("shard count must be power of 2")
       }
       // shardCount = 64 → shardDepth = 6
       // shardCount = 16 → shardDepth = 4
       // etc.
   }
   ```

3. **Implement transaction dispatcher** (~200 lines)
   ```go
   func (se *ShardedExecutor) DispatchTransactions(txns []*Delivery)
   // Groups transactions by shard using RouteToShard(account, shardDepth)
   ```

4. **Implement parallel shard execution** (~300 lines)
   ```go
   func (se *ShardedExecutor) StartBlock() 
   // Spawns 64 goroutines (one per shard) using WaitGroup
   // Each executes its transactions independently
   ```

5. **Implement state consolidation** (~200 lines)
   ```go
   func (se *ShardedExecutor) EndBlock() 
   // Waits for all shards to complete
   // Merges shard states → block state
   ```

### Phase 3: Testing (4-5 days)

1. **Unit tests**
   - Transaction routing correctness
   - Account lock safety
   - Cross-shard transaction handling

2. **Race tests**
   ```bash
   go test -race ./internal/core/execute/v1/...
   ```

3. **Functional tests**
   - Blocks executed identically (sharded vs non-sharded)
   - Root hashes match
   - All transactions processed

4. **Performance tests**
   - Baseline (non-sharded)
   - 4, 8, 16, 32 shard configurations
   - Concurrent load tests
   - Cross-shard transaction impact

### Phase 4: Integration (3-4 days)

1. **Wire into consensus layer**
2. **Add configuration options**
3. **Fallback to non-sharded on error**
4. **Documentation and examples**

### Phase 5: Optimization (2-3 days)

1. **Profile hotspots**
2. **Tune padding and lock granularity**
3. **Benchmark against target (10K TPS)**

**Total Estimate:** 16-22 days

---

## Code Structure

```
internal/core/execute/v1/
├─ executor.go (existing, single-shard)
├─ executor_sharded.go (NEW)
│   ├─ type ShardedExecutor
│   ├─ func (se *ShardedExecutor) StartBlock()
│   ├─ func (se *ShardedExecutor) EndBlock()
│   ├─ func (se *ShardedExecutor) dispatchTransactions()
│   ├─ func (se *ShardedExecutor) executeShardTransactions()
│   └─ func (se *ShardedExecutor) mergeShardStates()
│
├─ shard_locks.go (NEW)
│   ├─ type ShardedAccountLocks
│   ├─ type ShardedAccountLockSet
│   ├─ func (sal *ShardedAccountLocks) Lock()
│   ├─ func (sal *ShardedAccountLocks) Unlock()
│   └─ func RouteToShard()
│
└─ shard_test.go (NEW)
    ├─ TestTransactionRouting()
    ├─ TestConcurrentExecution()
    ├─ TestCrossShardTransaction()
    ├─ TestRootHashEquivalence()
    └─ BenchmarkParallelExecution()
```

---

## Configuration (Database-Stored)

### Storage in Database

Shard configuration is stored in the database with zero impact on storage format (pure in-memory routing):

```go
// In database under network config
type ExecutorConfig struct {
    // ExecutorShardCount: number of shards (MUST be power of 2)
    // Valid values: 4, 8, 16, 32, 64, 128, 256, 512, 1024
    // Default: 64 (recommended for production)
    // 
    // CRITICAL: Must be power of 2 to align with BPT tree structure
    // and enable proper bit-shift routing: key[0] >> (8 - log2(count))
    //
    // Storage Impact: NONE - this is pure in-memory routing
    // Can change between restarts without any data migration
    ExecutorShardCount int `json:"executorShardCount"`
}
```

### Loading Configuration

```go
// On node startup
func (node *Node) Initialize(batch *database.Batch) error {
    // Read shard config from database
    config, err := batch.GetExecutorConfig()
    if err != nil && !errors.Is(err, ErrNotFound) {
        return err
    }
    
    // Use default if not set
    if config == nil {
        config = &ExecutorConfig{
            ExecutorShardCount: 64,  // Production default
        }
    }
    
    // Validate power-of-2
    if !IsPowerOfTwo(config.ExecutorShardCount) {
        return fmt.Errorf("executor shard count must be power of 2, got %d", 
            config.ExecutorShardCount)
    }
    
    // Create executor with configured shard count
    node.executor, err = NewShardedExecutor(config.ExecutorShardCount)
    return err
}
```

### Updating Configuration

```go
// Operator can update via API/CLI
func (node *Node) UpdateExecutorShardCount(ctx context.Context, newCount int) error {
    // Validate
    if !IsPowerOfTwo(newCount) {
        return fmt.Errorf("must be power of 2, got %d", newCount)
    }
    
    // Store in database
    batch := node.db.Begin(ctx, false)
    defer batch.Discard()
    
    config, _ := batch.GetExecutorConfig()
    config.ExecutorShardCount = newCount
    batch.PutExecutorConfig(config)
    
    if err := batch.Commit(); err != nil {
        return err
    }
    
    // Update in-memory executor (takes effect on next block)
    node.executor, _ = NewShardedExecutor(newCount)
    return nil
}
```

### Benefits of Database Storage

1. **Persistent:** Shard count survives restarts
2. **No Migration:** Database format unchanged (pure routing)
3. **Dynamic:** Can update via API without code changes
4. **Observable:** Part of node configuration state
5. **Auditable:** Configuration changes are persisted
6. **Zero Downtime:** Takes effect on next block execution

---

## Safety Guarantees

### Correctness

1. **Deterministic Routing**
   - Same account always routes to same shard
   - No re-routing or cross-shard churn

2. **Account Lock Safety**
   - All affected accounts locked before execution
   - Cross-shard: two-phase locking prevents deadlock
   - Same-shard: standard mutex

3. **State Consistency**
   - All shards complete before EndBlock
   - BPT root hash computed in parallel (existing)
   - State finalization atomic

### Concurrency

1. **No Data Races**
   - Each shard independent until EndBlock
   - Account locks prevent concurrent modification
   - BPT already thread-safe per-shard

2. **Zero Cross-Shard Contention**
   - Different keys → different shards → independent
   - Same key → same shard → serialized locally
   - Lock contention reduced by factor of N

---

## Deployment

### Configuration Steps

**Step 1: Deploy new binary** (includes ShardedExecutor implementation)

**Step 2: Set shard count in database**
```go
// Via node API or CLI:
node.UpdateExecutorShardCount(ctx, 64)

// This:
// 1. Validates power-of-2 (64 = 2^6 ✓)
// 2. Stores in database (zero storage impact)
// 3. Updates in-memory executor
// 4. Takes effect on next block
```

**Step 3: Verify**
```bash
# Check config in database
accumulated query executor-config

# Monitor: Should see ~12-16x TPS increase on next blocks
accumulated status
```

### Zero-Downtime Reconfiguration

To adjust shard count without restart:
```go
// Current: 64 shards, stable
// Want: 32 shards for testing

1. Wait for current block to finalize
2. Call node.UpdateExecutorShardCount(ctx, 32)  // Must be power of 2
3. Takes effect on next block
4. No data migration (pure in-memory routing)
5. Revert anytime: UpdateExecutorShardCount(ctx, 64)
```

### Backward Compatibility

- Same storage format (shard count has **zero DB impact**)
- Can read same data with any power-of-2 shard count
- Configuration persisted in database
- No migration required
- Can adjust shard count between blocks anytime

---

## Success Criteria

- ✅ Root hashes identical to non-sharded execution
- ✅ 12-16x TPS improvement with 64 shards (on 64-core systems)
- ✅ Zero data races (`go test -race`)
- ✅ Cross-shard transactions handled correctly
- ✅ Backward compatible (can disable sharding)
- ✅ Configuration enforces power-of-2 shard counts
- ✅ Default: 64 shards (configurable: 4, 8, 16, 32, 64, 128, 256)
- ✅ Shard count stored in database (no migration needed)
- ✅ Achieves 10K+ TPS target per partition

---

## Risk Mitigation

| Risk | Mitigation |
|------|-----------|
| Cross-shard deadlock | Two-phase locking with ordering |
| Lock starvation | Timeout + fallback to sequential |
| Performance regression | Feature-gated, fallback available |
| Correctness bugs | Equivalence tests, race tests |
| Configuration complexity | Sensible defaults (4-6 bits) |

---

## References

- BPT Sharding Design: `docs/bpt-parallel-sharding-design.md`
- ShardedBPT Implementation: `pkg/database/bpt/sharded.go`
- Executor V1: `internal/core/execute/v1/`
- Issue #3888: BPT parallel updates
- Issue #3892: 10K TPS infrastructure

---

## Next Steps

1. **Review & Approve Design** (this document)
2. **Finalize Account Lock Strategy** (option A/B/C)
3. **Analyze Cross-Shard Transaction Rate** (production data)
4. **Begin Phase 1: Design & Validation**

**Estimated Timeline:** 3-4 weeks to production-ready

---

**Author:** Claude Code  
**Date:** 2026-04-14  
**Status:** Ready for Design Review
