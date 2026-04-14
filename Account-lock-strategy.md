# Cross-Shard Account Lock Strategy for Accumulate Protocol

## Executive Summary

This document outlines the lock acquisition strategy for cross-shard transactions in a 64-shard parallel execution environment. The strategy prevents deadlocks while maintaining correctness and performance.

**Selected Strategy: Deterministic Shard Ordering with Fine-Grained Locks**

Lock acquisition is deterministic by always acquiring locks in ascending shard ID order. Within each shard, accounts are locked in ascending lexicographic order by their URL. This total ordering eliminates circular wait conditions and prevents deadlocks entirely, with minimal overhead.

---

## Context

### The Problem

With 64 shards executing transactions in parallel, cross-shard transactions pose a deadlock risk:

- Transaction T1 needs to modify accounts in shards [A, B]
- Transaction T2 needs to modify accounts in shards [B, A]
- Without deterministic locking, T1 locks shard A, T2 locks shard B
- T1 waits for B (held by T2), T2 waits for A (held by T1)
- **Deadlock**

### Current Implementation Context

From the codebase analysis:

1. **Sharded BPT**: Already uses per-shard locking with cache-line-padded mutexes (`shardMu[index]`)
   - Each shard is routed deterministically: `shardID = keyHash[0] >> (8 - shardDepth)`
   - Shard IDs range from 0 to 63 for 64 shards (depth=6)

2. **Database Batch Model**: Transactions operate through batch.Account(url) which resolves to a specific shard
   - Accounts are identified by URL
   - Multiple accounts can be affected in a single transaction

3. **Transaction Types**: SendTokens, SyntheticDepositTokens, etc. can affect multiple accounts
   - Source account + recipient account(s)
   - All must be locked atomically for transaction consistency

---

## Strategy Comparison

### Option A: Deterministic Order (SELECTED)
**Lock acquisition order**: Sort affected shard IDs, then sort accounts within each shard by URL, always lock in ascending order.

**Pros:**
- Eliminates deadlocks via total ordering (no circular waits possible)
- Minimal overhead: single sort pass
- Simple to implement and verify
- Works with existing ShardedBPT shard routing
- Composable: sub-transactions inherit safety

**Cons:**
- Requires identifying all touched accounts upfront
- May hold locks longer than strictly necessary

### Option B: Lock Timeout with Retry
**Acquire locks with timeout, retry from scratch on timeout.**

**Pros:**
- No need to know all touched accounts upfront
- Lower latency in contention-free case

**Cons:**
- Deadlock not prevented, merely detected/recovered
- Starvation risk if retry keeps failing
- Adds complexity: timeout tuning, exponential backoff
- Harder to test and reason about correctness

### Option C: Global Transaction Ordering
**Serialize all transactions by a global sequence number.**

**Pros:**
- Simple to reason about
- Guaranteed correctness

**Cons:**
- Eliminates parallelism entirely (defeats purpose of 64 shards)
- Massive performance hit

### Option D: Hybrid Approach
**Try deterministic order; timeout→fallback to serialization.**

**Pros:**
- Combines benefits of A and C

**Cons:**
- Added complexity for diminishing returns
- Still needs deterministic ordering (A)

---

## Selected Strategy: Deterministic Shard Ordering

### Core Principle

**All transactions acquire locks in a globally consistent order:**
1. **Shard level**: Acquire in ascending shard ID order (0 → 63)
2. **Account level**: Within each shard, acquire in ascending URL lexicographic order

This creates a total ordering that prevents cycles → no deadlocks.

### Lock Acquisition Order Algorithm

```
function acquireLocks(transaction, batch):
    // Step 1: Identify all accounts touched by this transaction
    touchedAccounts = []
    for each message in transaction.messages:
        for each account in identifyAccountsFor(message):
            touchedAccounts.append(account)
    
    // Step 2: Dedup and sort by (shardID, urlString)
    uniqueAccounts = dedup(touchedAccounts)
    sortedAccounts = sort(uniqueAccounts, by=(
        account.hash() % 64,  // shard ID (0-63)
        account.url.String()   // URL lexicographically
    ))
    
    // Step 3: Acquire locks in order
    acquiredLocks = []
    for each account in sortedAccounts:
        lock = shardMu[account.shardID].mu
        lock.Lock()
        acquiredLocks.append((shardID, account))
    
    // Step 4: Load/validate account state
    // (protected by locks)
    accountState = batch.Account(account.url).Main().Get()
    
    return acquiredLocks, accountState
```

### Deadlock Prevention Guarantee

**Theorem**: No circular wait is possible under deterministic ordering.

**Proof**: 
- Suppose transactions T1, T2, ..., Tn form a cycle (each waits for the next's locks)
- T1 waits for locks held by T2
- T2 must be holding a lock L that T1 needs
- By deterministic ordering, T1 acquires locks in a fixed order O1
- By deterministic ordering, T2 acquires locks in a fixed order O2
- If T1 needs L and T2 holds L, then L must come before some lock T2 was waiting for
- But T2 also acquires in ascending order, so T2 wouldn't wait for a lock that comes before L in its order
- Contradiction → no cycle possible

### Account Identification for Common Transaction Types

For transaction body type analysis:

**SendTokens**:
- Source account: from `st.Origin` (authenticated sender)
- Recipient accounts: from `body.To[].Url`
- Lock order: [source] + sorted([recipients])

**SyntheticDepositTokens** (generated for cross-shard):
- Source partition account (implicit)
- Recipient account: `body.To`
- Lock order: [source partition] + [recipient]

**CreateIdentity/CreateAccount**:
- Parent account: from URL hierarchy
- New account: from creation parameters
- Lock order: [parent] + [new]

**UpdateAccountAuth**:
- Target account: from `tx.Header.Principal`
- Key book account: from auth chain reference
- Lock order: [target] + [key book]

### Implementation Pattern

```go
type LockedAccounts struct {
    locks []sync.Locker  // In order of acquisition
    accounts map[string]*Account  // Cached account state
}

// AcquireAccountLocks acquires locks on all accounts touched by a transaction
// in deterministic order to prevent deadlocks.
func (b *Batch) AcquireAccountLocks(tx *Delivery) (*LockedAccounts, error) {
    // 1. Identify touched accounts from transaction body
    touched := identifyTouchedAccounts(tx)
    
    // 2. Sort by (shardID, url)
    sort.Slice(touched, func(i, j int) bool {
        a, b := touched[i], touched[j]
        shardA := hashToShard(a.Hash())
        shardB := hashToShard(b.Hash())
        if shardA != shardB {
            return shardA < shardB
        }
        return a.String() < b.String()
    })
    
    // 3. Acquire locks and load state
    la := &LockedAccounts{
        locks: make([]sync.Locker, 0, len(touched)),
        accounts: make(map[string]*Account),
    }
    
    for _, urlStr := range touched {
        url := parseURL(urlStr)
        lock := b.getShardLock(url)
        lock.Lock()
        la.locks = append(la.locks, lock)
        
        account := b.Account(url)
        la.accounts[urlStr] = account
    }
    
    return la, nil
}

// Release releases all acquired locks in reverse order
func (la *LockedAccounts) Release() {
    // Release in reverse order (LIFO)
    for i := len(la.locks) - 1; i >= 0; i-- {
        la.locks[i].Unlock()
    }
}
```

---

## Deadlock Detection and Recovery (Fallback)

While deadlock is prevented by design, production systems may encounter:
- Bugs in account identification logic
- Unforeseen transaction patterns
- Cascading failures

### Fallback Mechanism

1. **Timeout on Lock Acquisition**: If lock not acquired within timeout (e.g., 5 seconds), assume deadlock
   ```go
   acquired := make(chan bool, 1)
   go func() {
       lock.Lock()
       acquired <- true
   }()
   
   select {
   case <-acquired:
       // success
   case <-time.After(5 * time.Second):
       // deadlock detected, rollback and retry
       return ErrDeadlockDetected
   }
   ```

2. **Rollback and Retry**: On deadlock detection:
   - Release all acquired locks immediately
   - Clear batch state
   - Retry transaction (with exponential backoff)

3. **Observability**: Log deadlock events with:
   - Transaction hash
   - Accounts involved
   - Lock wait stack traces
   - Retry count and delays

### Deadlock Detection Scenario

```
Block execution deadlock scenario:

1. SendTokens touches account1 (shard 5) → account2 (shard 10)
2. SyntheticDepositTokens touches account2 (shard 10) → account1 (shard 5)

Without deterministic ordering:
  T1: Lock shard 5, wait for shard 10
  T2: Lock shard 10, wait for shard 5
  → Deadlock

With deterministic ordering:
  T1: Lock shard 5, then shard 10 (always shard 5 first)
  T2: Lock shard 10, then shard 5 (always shard 5 first)
  → T2 blocks on shard 5 until T1 releases
  → T1 completes, releases all locks
  → T2 proceeds
  → No deadlock
```

---

## Test Scenarios

### 1. Basic Non-Crossing Transactions
**Test**: Two transactions, each touching different account sets
- T1 touches accounts in shards [1, 3, 5]
- T2 touches accounts in shards [2, 4, 6]
**Expected**: Both execute in parallel with no contention
**Assertion**: Locks acquired in order per transaction

### 2. Partial Overlap (Safe)
**Test**: Two transactions with overlapping shard sets
- T1 touches shards [1, 3, 5] (accounts A, B, C)
- T2 touches shards [3, 5, 7] (accounts D, E, F)
**Expected**: Lock on shard 3 serializes; no deadlock
**Assertion**: T1 or T2 waits for other on shard 3, both complete

### 3. Complete Overlap (Stress)
**Test**: Two transactions touching same account
- T1: account1 (shard 5)
- T2: account1 (shard 5)
**Expected**: Perfect serialization on shard 5 lock
**Assertion**: Second transaction waits on first

### 4. Cross-Shard Reverse Order (Deadlock Candidate)
**Test**: Two transactions, opposite shard access order
- T1 touches [shard 5, shard 10] (account1 in shard 5, account2 in shard 10)
- T2 touches [shard 10, shard 5] (account2 in shard 10, account1 in shard 5)
**Expected**: No deadlock due to deterministic ordering
**Assertion**: Both acquire locks in ascending shard order; one waits on first shard

### 5. Many Shards (Stress)
**Test**: Transaction touching accounts in many shards
- T1 touches 32 accounts spread across 32 shards (random distribution)
- T2 touches different 32 accounts
**Expected**: Execute in parallel
**Assertion**: Lock order deterministic; no deadlock even under stress

### 6. Timeout and Retry
**Test**: Simulate lock acquisition timeout
- Inject delay in lock acquisition
- Trigger timeout
- Verify rollback and retry
**Expected**: Transaction retried and eventually succeeds
**Assertion**: No data corruption; state consistent after retry

### 7. Cascade Scenario
**Test**: Three transactions, creating potential multi-level deadlock
- T1 touches [shard A, shard B]
- T2 touches [shard B, shard C]
- T3 touches [shard C, shard A]
**Expected**: No deadlock; one transaction waits on the other
**Assertion**: Total ordering prevents cycle

---

## Implementation Checklist

- [ ] **Phase 1**: Implement `identifyTouchedAccounts()` for each transaction type
- [ ] **Phase 1**: Implement `AcquireAccountLocks()` with deterministic sorting
- [ ] **Phase 1**: Add lock timeout with rollback
- [ ] **Phase 2**: Integrate with block execution engine
- [ ] **Phase 3**: Add comprehensive testing (test scenarios above)
- [ ] **Phase 3**: Add deadlock detection observability/metrics
- [ ] **Phase 4**: Performance benchmarking under contention
- [ ] **Phase 5**: Documentation and runbooks

---

## Performance Considerations

### Lock Contention
- Per-shard locks minimize contention (64 independent locks)
- Cache-line padding prevents false sharing
- Deterministic ordering has negligible overhead (single sort)

### Lock Hold Times
- Locks held only during account state modification
- Batch semantics ensure atomic transitions
- No nested locks (prevents reentrancy bugs)

### Scalability
- Linear growth with number of accounts touched (typically 2-3 per transaction)
- O(n log n) sort, n = touched accounts (trivial for n ≤ 10)
- No contention between different account sets

---

## Future Enhancements

1. **Read-Write Locks**: Distinguish read-only transactions from write transactions
   - Read-heavy transactions could share read locks
   - Trades complexity for higher concurrency

2. **Optimistic Locking**: Validate consistency without holding locks
   - Reduces lock hold time
   - Requires conflict detection and retry logic

3. **Adaptive Ordering**: Learn from actual contention patterns
   - Route frequently-together accounts to same shard
   - Dynamic shard rebalancing (complex)

---

## Conclusion

**Deterministic shard ordering is the optimal strategy** for cross-shard transactions in Accumulate:

- **Correctness**: Proven deadlock-free via total ordering
- **Performance**: Minimal overhead, high parallelism
- **Simplicity**: Easy to implement, test, and reason about
- **Resilience**: Fallback timeout mechanism for edge cases

The strategy leverages Accumulate's existing ShardedBPT per-shard locking while adding transaction-level coordination. With proper implementation and testing, it provides safe, efficient parallel execution across 64 shards.
