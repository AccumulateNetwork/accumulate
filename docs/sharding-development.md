# Sharding Developer Guide

## How Sharding Works for Transaction Handlers

Transaction handlers do not need to be shard-aware. The sharding layer sits
between the block executor and the database, routing each transaction to the
correct shard transparently.

A transaction handler receives a `PerShardExecutor` (or its database batch) and
operates on it exactly as it would on a non-sharded database. The handler does
not know or care which shard it is running on.

## Routing Rules

Every account URL is routed to a shard based on its **identity** (ADI):

```go
shardID := se.RouteAccount(accountURL)
```

Key properties:

- All accounts under the same ADI route to the same shard.
  `acc://alice.acme/tokens` and `acc://alice.acme/data` are always on the same
  shard.
- Routing is deterministic and stateless -- computed from the URL alone.
- Routing uses `IdentityAccountID32()[0] >> (8 - shardDepth)`.

### Cross-Shard Transactions

A transaction that touches accounts under two different ADIs (e.g., a token
transfer from `acc://alice.acme/tokens` to `acc://bob.acme/tokens`) may span
two shards. The parallel execution framework handles this:

1. The caller provides the list of affected shard IDs.
2. `ExecuteTransactionOnShards` runs the execution function on each shard in
   parallel.
3. If all shards succeed, results are collected.
4. If any shard fails or panics, all shards are rolled back.

The single-shard fast path (no goroutines) is used when all affected accounts
are on the same shard, which is the common case for single-ADI operations.

## Adding New Transaction Types

When adding a new transaction type:

1. **No routing changes needed.** The transaction's primary account URL
   determines its shard. The existing `RouteAccount` function handles routing.

2. **Identify affected shards.** If the transaction touches accounts under
   multiple ADIs, list all affected shard IDs when calling
   `ExecuteTransactionOnShards`.

3. **Keep execution functions shard-safe.** The execution function passed to
   `ExecuteTransactionOnShards` receives a `PerShardExecutor`. Access the
   database only through the shard's batch (`shard.Batch()` or
   `shard.Account(url)`). Do not access global state or other shards' batches.

4. **No cross-shard locks.** Never acquire locks on multiple shards manually.
   The parallel execution framework handles synchronization.

Example -- single-shard transaction:

```go
shardID := se.RouteAccount(txn.Source)
result, err := se.ExecuteTransactionOnShards(ctx, []int{shardID},
    func(shard *PerShardExecutor) (interface{}, error) {
        acct := shard.Account(txn.Source)
        // ... modify account state ...
        return nil, nil
    })
```

Example -- cross-shard transaction:

```go
srcShard := se.RouteAccount(txn.Source)
dstShard := se.RouteAccount(txn.Destination)
shards := []int{srcShard, dstShard} // may be the same shard

result, err := se.ExecuteTransactionOnShards(ctx, shards,
    func(shard *PerShardExecutor) (interface{}, error) {
        // This function runs on each affected shard.
        // Each shard only modifies accounts it owns.
        if shard.ID == srcShard {
            // debit source
        }
        if shard.ID == dstShard {
            // credit destination
        }
        return nil, nil
    })
```

## Testing with Sharding

### Unit Tests

Use `NewShardedExecutor(shardCount, nil)` for tests that do not need database
access. The `nil` database is fine for testing routing and shard assignment.

```go
se, err := NewShardedExecutor(4, nil)
require.NoError(t, err)
shard := se.RouteAccount(myURL)
```

### Integration Tests with Database

Use `database.OpenInMemory(nil)` for tests that need real database operations:

```go
db := database.OpenInMemory(nil)
se, err := NewShardedExecutor(4, db)
require.NoError(t, err)
se.BeginBlock()
defer se.Discard()
```

### Testing Shard Distribution

Verify that your account naming produces reasonable shard distribution:

```go
se, err := NewShardedExecutor(4, nil)
counts := make([]int, 4)
for i := 0; i < 1000; i++ {
    u, _ := url.Parse(fmt.Sprintf("acc://user%d.acme/tokens", i))
    counts[se.RouteAccount(u)]++
}
// Each shard should get roughly 250 accounts
```

### Race Condition Testing

Run tests with the Go race detector to verify thread safety:

```bash
go test -race ./internal/core/execute/...
go test -race ./pkg/database/bpt/...
```

The existing test suites include dedicated concurrency tests:
- `sharded_execution_concurrency_test.go` -- parallel execution, panic recovery,
  context cancellation, goroutine leak detection.
- `sharded_execution_correctness_test.go` -- functional correctness with real
  database operations.

## Debugging

### Tracing a Transaction to Its Shard

To find which shard a transaction routes to:

```go
u, _ := url.Parse("acc://alice.acme/tokens")
shardID := se.RouteAccount(u)
fmt.Printf("acc://alice.acme routes to shard %d\n", shardID)
```

Or using the canonical routing function directly:

```go
h := u.IdentityAccountID32()
shardID := bpt.RouteToShard(h, 6) // depth 6 = 64 shards
```

### Inspecting Per-Shard State

During debugging, you can inspect shard state:

```go
for i := 0; i < se.ShardCount(); i++ {
    shard := se.Shard(i)
    fmt.Printf("Shard %d: %d loaded accounts\n", i, shard.LoadedAccountCount())
}
```

### Profiling Shard Contention

Use Go's block profiler to measure lock contention:

```bash
go test -bench=BenchmarkContention -blockprofile=block.out ./pkg/database/bpt/...
go tool pprof -http=:8080 block.out
```

The block profile shows which shard locks have the most contention and helps
identify hot shards.

## Key Source Files

| File | Purpose |
|------|---------|
| `internal/core/execute/sharded_executor.go` | Top-level shard orchestration |
| `internal/core/execute/per_shard_executor.go` | Per-shard database batch management |
| `internal/core/execute/parallel_execution.go` | Multi-shard parallel execution with rollback |
| `pkg/database/bpt/sharded.go` | Sharded Binary Patricia Tree |
| `pkg/database/bpt/shard_config.go` | Shard count validation and routing |
