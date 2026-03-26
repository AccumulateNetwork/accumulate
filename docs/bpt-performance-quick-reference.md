# BPT Performance Analysis - Quick Reference

## TL;DR

**Current:** 6x speedup vs non-sharded
**Potential:** 20-30x speedup with optimizations

**Top 3 Issues:**
1. False sharing (8 mutexes per cache line) → 38-91% contention
2. Memory allocations (7 allocs per insert) → GC pressure
3. Key hash overhead (4.93% CPU time)

**Top 3 Fixes (7 hours work for 2.5-4x gain):**
1. Pad mutexes to 64 bytes (2 hrs → 2-3x contention reduction)
2. Optimize key hash (1 hr → 10-15% speedup)
3. Object pooling (4 hrs → 20-30% fewer allocations)

---

## Performance Numbers

### Current State
```
Workload     Sharded (16)  Non-Sharded  Speedup  Contention
--------     ------------  -----------  -------  ----------
Insert       15M ops/sec   2.3M ops/sec  6.5x    62-91%
Get          57M ops/sec   11M ops/sec   5.2x    62-91%
GetRootHash  1.63ms        6.15ms        3.8x    N/A
```

### After Priority 1 Optimizations
```
Workload     Expected      Current      Improvement
--------     --------      -------      -----------
Insert       40-50M/sec    15M/sec      2.7-3.3x
Get          115-170M/sec  57M/sec      2.0-3.0x
GetRootHash  5-7x speedup  3.8x         1.3-1.8x
Contention   10-30%        62-91%       3-6x reduction
```

---

## Critical Issues Detail

### Issue #1: False Sharing (CRITICAL)
**Problem:** `shardMu []sync.Mutex` - 8 mutexes per 64-byte cache line

**Evidence:**
- Mutex size: 8 bytes
- Cache line: 64 bytes
- Mutexes per line: 8
- Contention: 38-91% (expected <10%)

**Fix (2 hours):**
```go
type paddedMutex struct {
    mu sync.Mutex
    _  [56]byte  // Pad to 64 bytes
}

type ShardedBPT struct {
    shardMu []paddedMutex  // Was: []sync.Mutex
}

// Update all lock calls:
s.shardMu[i].mu.Lock()   // Was: s.shardMu[i].Lock()
```

**Impact:** 2-3x reduction in lock contention

---

### Issue #2: Memory Allocations
**Problem:** 7 allocations per insert operation

**Breakdown:**
- Key hash: 1-2 allocs
- Value copy: 1 alloc
- Mutation struct: 1 alloc
- Map entry: 1 alloc
- Tree nodes: 2-3 allocs

**Fix (4 hours):**
```go
var mutationPool = sync.Pool{
    New: func() interface{} { return &mutation{} },
}

func (b *BPT) Insert(key *record.Key, value []byte) error {
    m := mutationPool.Get().(*mutation)
    // Reuse m.value slice if possible
    if cap(m.value) >= len(value) {
        m.value = m.value[:len(value)]
    } else {
        m.value = make([]byte, len(value))
    }
    copy(m.value, value)
    // ... use m ...
}
```

**Impact:** 20-30% fewer allocations, reduced GC pressure

---

### Issue #3: Key Hash Overhead
**Problem:** 4.93% CPU time in hash computation

**Fix (1 hour):**
```go
// Pre-compute hash in hot paths
func (s *ShardedBPT) Insert(key *record.Key, value []byte) error {
    keyHash := key.Hash()  // Compute once
    shardID := int(keyHash[0] >> (8 - s.shardDepth))
    // Use keyHash throughout
}

// Inline fast path
func (k *Key) Hash() KeyHash {
    if k.hash != nil {
        return *k.hash  // Inline this check
    }
    return k.computeHash()  // Out of line
}
```

**Impact:** 10-15% speedup

---

## Optimal Configuration

### By System Size
| Cores    | Shards | Depth | Rationale                    |
|----------|--------|-------|------------------------------|
| 4-8      | 16     | 4     | Matches thread count         |
| 8-16     | 32     | 5     | Optimal for mid-range        |
| 16-32    | 32-64  | 5-6   | Balance parallelism/overhead |
| 32+      | 64     | 6     | Full utilization             |

### By Workload
| Workload     | Shards | Why                           |
|--------------|--------|-------------------------------|
| Write-heavy  | 32     | Best insert throughput        |
| Read-heavy   | 64     | Minimizes read contention     |
| Balanced     | 32     | Good all-around               |
| GetRootHash  | 16     | Least coordination overhead   |

---

## Scalability

### Diminishing Returns
```
GetRootHash (10K entries):
  16 shards: 1.63ms (3.8x speedup) ← Sweet spot
  32 shards: 1.84ms (3.3x speedup) - 12% slower
  64 shards: 2.08ms (3.0x speedup) - 27% slower
```

### Memory Overhead
```
Shards   Memory    Allocs    Worth It?
------   ------    ------    ---------
16       +8%       +1%       ✓ Yes
32       +17%      +2%       ✓ Yes
64       +42%      +5%       ⚠ Only for read-heavy
```

---

## Implementation Priority

### Priority 1: MUST DO (2.5-4x gain, 7 hours)
1. **Pad mutexes** (2 hrs)
   - File: `pkg/database/bpt/sharded.go:34`
   - Change: `[]sync.Mutex` → `[]paddedMutex`
   - Impact: 2-3x contention reduction

2. **Optimize hash** (1 hr)
   - File: `pkg/database/bpt/sharded.go:88-103`
   - Change: Pre-compute `keyHash` in Insert/Get/Delete
   - Impact: 10-15% speedup

3. **Object pooling** (4 hrs)
   - File: `pkg/database/bpt/mutate.go:26-36`
   - Change: Add `sync.Pool` for mutations
   - Impact: 20-30% fewer allocations

### Priority 2: SHOULD DO (1.2-1.5x gain, 8 hours)
4. **Worker pool** (6 hrs)
   - File: `pkg/database/bpt/sharded.go:125-159`
   - Change: Use bounded parallelism in GetRootHash
   - Impact: 15-25% for 64+ shards

5. **Batch small trees** (2 hrs)
   - File: `pkg/database/bpt/sharded.go:125-159`
   - Change: Don't spawn goroutines for ≤4 shards
   - Impact: 10-15% for small trees

### Priority 3: NICE TO HAVE
6. Adaptive sharding (high complexity)
7. NUMA-aware placement (platform-specific)

---

## Testing After Changes

### Verify Correctness
```bash
# Run all tests
go test ./pkg/database/bpt -v

# Specific tests
go test ./pkg/database/bpt -run TestSharded

# Race detector
go test ./pkg/database/bpt -race
```

### Measure Performance
```bash
# Before/after comparison
go test ./pkg/database/bpt -bench=. -benchmem -benchtime=3s

# Contention analysis
go test ./pkg/database/bpt -bench=BenchmarkContention -benchtime=1s

# CPU profile
go test ./pkg/database/bpt -bench=. -cpuprofile=cpu.prof
go tool pprof -top cpu.prof
```

### Expected Results
```
Before (baseline):
  BenchmarkShardedInsert/16-shards/goroutines=16: 101 ns/op, 224 B/op, 7 allocs/op
  BenchmarkContention/16-shards/goroutines=16: 84.00% contention

After (target):
  BenchmarkShardedInsert/16-shards/goroutines=16: 30-40 ns/op, 160 B/op, 4-5 allocs/op
  BenchmarkContention/16-shards/goroutines=16: 15-25% contention
```

---

## Files to Modify

### Priority 1 Changes
```
pkg/database/bpt/sharded.go
├─ Line 30-37:  Add paddedMutex type
├─ Line 88-93:  Update Insert to pre-compute hash
├─ Line 98-103: Update Get to pre-compute hash
└─ Line 108-113: Update Delete to pre-compute hash

pkg/database/bpt/mutate.go
├─ Line 10-15:  Add mutationPool
└─ Line 26-36:  Update Insert to use pool

pkg/types/record/key.go (optional)
└─ Line 110-127: Inline fast path of Hash()
```

---

## Quick Reference: False Sharing

### What is it?
Multiple CPUs accessing different data on the same cache line causes unnecessary cache invalidation.

### In our code:
```
Cache line 0: shardMu[0-7]   ← 8 mutexes
Cache line 1: shardMu[8-15]  ← 8 mutexes
...

Thread A locks shardMu[0] → Cache line 0 invalidated
Thread B accessing shardMu[1-7] → Must reload cache line
```

### The fix:
```
Cache line 0:  shardMu[0]    ← 1 mutex + 56 bytes padding
Cache line 1:  shardMu[1]    ← 1 mutex + 56 bytes padding
...

Thread A locks shardMu[0] → Only cache line 0 invalidated
Thread B accessing shardMu[1] → Cache line 1 still valid ✓
```

### Why it matters:
- 8 mutexes per line = 8x more cache traffic
- With 64 shards on 12 cores: Expected 0% collisions, actual 38-91%
- Fix: 2-3x contention reduction for 3.5KB memory cost

---

## Contact & Resources

**Full Report:** `/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/docs/bpt-performance-analysis.md`

**Benchmark Results:** `/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/docs/bpt-benchmark-results.md`

**Source Code:**
- Implementation: `pkg/database/bpt/sharded.go`
- Tests: `pkg/database/bpt/sharded_test.go`
- Benchmarks: `pkg/database/bpt/sharded_bench_test.go`
