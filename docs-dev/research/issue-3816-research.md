# Research: Backpressure improvements for high load

## Summary

The backpressure improvements for high load are already implemented and working correctly. The implementation includes three key mechanisms: (1) a batch count check in `Submit()` that returns `ErrBackpressure` when pending transaction size exceeds limits, (2) batch eviction in `StoreBatch()` that removes 10% of stored batches when `MaxStoredBatches` is exceeded, and (3) a reduced default `MaxStoredBatches` of 10,000. All consensus and dagbft tests pass, confirming the implementation is stable.

## Verified Facts

### Fact 1: Backpressure in Submit() via MaxPendingSize check
- **Source**: `pkg/consensus/worker/worker.go:203-209`
- **Content**:
  ```go
  // Check backpressure
  if w.pendingSize+len(tx) > w.config.MaxPendingSize {
      w.mu.Unlock()
      return ErrBackpressure
  }
  ```
- **Confidence**: HIGH

### Fact 2: ErrBackpressure error definition
- **Source**: `pkg/consensus/worker/worker.go:38-40`
- **Content**:
  ```go
  // ErrBackpressure is returned when the worker cannot accept more transactions
  // due to memory limits being reached.
  var ErrBackpressure = errors.New("worker backpressure: pending transactions exceed limit")
  ```
- **Confidence**: HIGH

### Fact 3: Default MaxPendingSize limit
- **Source**: `pkg/consensus/worker/worker.go:31`
- **Content**: `DefaultMaxPendingSize  = 10 * 1024 * 1024       // 10MB max pending transactions`
- **Confidence**: HIGH

### Fact 4: Batch eviction in StoreBatch()
- **Source**: `pkg/consensus/worker/worker.go:326-345`
- **Content**:
  ```go
  if _, exists := w.batches[digest]; !exists {
      // Evict random batches if we're at the limit
      // This prevents unbounded memory growth from gossip batches
      if len(w.batches) >= w.config.MaxStoredBatches {
          evictCount := len(w.batches) / 10 // Evict 10%
          if evictCount < 1 {
              evictCount = 1
          }
          evicted := 0
          for d := range w.batches {
              delete(w.batches, d)
              evicted++
              if evicted >= evictCount {
                  break
              }
          }
          slog.Debug("Evicted batches due to storage limit",
              "evicted", evicted,
              "remaining", len(w.batches))
      }
      w.batches[digest] = batch
  }
  ```
- **Confidence**: HIGH

### Fact 5: Default MaxStoredBatches limit
- **Source**: `pkg/consensus/worker/worker.go:32`
- **Content**: `DefaultMaxStoredBatches = 10000                 // max batches stored before eviction`
- **Confidence**: HIGH

### Fact 6: MaxStoredBatches configuration option
- **Source**: `pkg/consensus/worker/worker.go:81-85`
- **Content**:
  ```go
  // MaxStoredBatches is the maximum number of batches to store.
  // When exceeded, random batches are evicted to make room.
  // This prevents unbounded memory growth from gossip batches.
  // Defaults to DefaultMaxStoredBatches.
  MaxStoredBatches int
  ```
- **Confidence**: HIGH

### Fact 7: Backpressure test coverage
- **Source**: `pkg/consensus/worker/worker_test.go:98-112`
- **Content**:
  ```go
  t.Run("backpressure when limit reached", func(t *testing.T) {
      w := worker.New(worker.Config{
          ID:             0,
          Partition:      "test",
          MaxPendingSize: 100, // Very small limit
      }, nil)

      // Fill up to the limit
      err := w.Submit(make([]byte, 90))
      require.NoError(t, err)

      // This should trigger backpressure
      err = w.Submit(make([]byte, 20))
      assert.ErrorIs(t, err, worker.ErrBackpressure)
  })
  ```
- **Confidence**: HIGH

### Fact 8: Configurable channel buffer sizes for high throughput
- **Source**: `pkg/consensus/config/config.go:40-47`
- **Content**:
  ```go
  // Channel buffer defaults - sized for high throughput (~30k+ TPS)
  DefaultCertificateBufferSize = 1000 // Buffer for new certificates channel
  DefaultBatchBufferSize       = 1000 // Buffer for batch gossip channel
  DefaultHeaderBufferSize      = 500  // Buffer for header gossip channel
  DefaultVoteBufferSize        = 1000 // Buffer for vote gossip channel
  DefaultCertSyncBufferSize    = 500  // Buffer for certificate sync channels
  DefaultEnvelopeBufferSize    = 500  // Buffer for inter-partition dispatch
  ```
- **Confidence**: HIGH

### Fact 9: Gossip channel sizes increased for high throughput
- **Source**: `pkg/consensus/gossip/gossip.go:22-28`
- **Content**:
  ```go
  // Default channel buffer sizes for message channels.
  // Sized for high throughput (~30k+ TPS) to avoid dropped messages.
  const (
      DefaultBatchChannelSize       = 1000
      DefaultHeaderChannelSize      = 500  // Increased from 100 for high throughput
      DefaultVoteChannelSize        = 1000
      DefaultCertificateChannelSize = 1000 // Increased from 100 for high throughput
      DefaultCertSyncChannelSize    = 500  // Increased from 200 to handle sustained load
  )
  ```
- **Confidence**: HIGH

### Fact 10: CommitBufferSize increased for high throughput
- **Source**: `pkg/consensus/consensus.go:35-37`
- **Content**: `DefaultCommitBufferSize = 5000 // Increased from 1000 for high throughput`
- **Confidence**: HIGH

### Fact 11: Batch pruning after commit (not during)
- **Source**: `pkg/consensus/consensus.go:381-385`
- **Content**:
  ```go
  // NOTE: Batch pruning is handled by the executor (main.go) after reading
  // batches from workers. We must NOT prune here because the committed
  // channel is buffered - pruning before the executor reads would cause
  // "Missing batch for certificate" errors.
  ```
- **Confidence**: HIGH

### Fact 12: Batch pruning in service after processing
- **Source**: `internal/node/dagbft/service.go:388-390`
- **Content**:
  ```go
  // Prune batches from workers now that they've been processed
  for _, w := range s.node.Workers() {
      w.PruneBatches(committedDigests)
  }
  ```
- **Confidence**: HIGH

## Code References

### Primary Implementation Files
| File | Function/Area |
|------|---------------|
| `pkg/consensus/worker/worker.go` | Main backpressure logic (Submit, StoreBatch, eviction) |
| `pkg/consensus/config/config.go` | Configuration constants and defaults |
| `pkg/consensus/gossip/gossip.go` | Channel buffer sizes for gossip |
| `pkg/consensus/consensus.go` | Node-level configuration and commit handling |
| `internal/node/dagbft/service.go` | Batch pruning after block production |

### Key Functions
- `worker.Submit()` - Lines 180-233: Transaction submission with backpressure
- `worker.StoreBatch()` - Lines 315-350: Batch storage with eviction
- `worker.PruneBatches()` - Lines 376-389: Manual pruning of committed batches
- `worker.Config.applyDefaults()` - Lines 93-109: Default configuration

### Test Files
- `pkg/consensus/worker/worker_test.go` - Comprehensive tests including backpressure

## Open Questions

1. **Eviction strategy**: The current eviction strategy uses random selection (Go map iteration order). A more sophisticated LRU or priority-based eviction might be beneficial for production workloads, but the random approach is simpler and sufficient for preventing memory exhaustion.

2. **Eviction location**: Eviction happens in `StoreBatch()` (line 329), which is called when receiving batches from gossip. The issue description mentions eviction in `createAndBroadcastBatch()`, but that function (lines 464-499) does not contain eviction logic - it only stores locally created batches. This may be intentional since locally-created batches are always needed, while gossip batches may be from old rounds.

## Contradictions

None found. The implementation is consistent across all files examined.

## Historical Context

From git log analysis:

1. **Commit 9af858238** (Mar 16, 2026): Initial optimization for high throughput
   - Increased channel buffer sizes
   - Added batch pruning callback (later revised)
   - Expected 50k+ TPS

2. **Commit 202ddc02a** (Mar 16, 2026): Fixed batch pruning and memory management
   - Removed premature batch pruning from `processBullshark()`
   - Added `MaxStoredBatches` limit with 10% eviction
   - Result: ~5,100 TPS sustained, stable memory at ~1.4GB

## Test Results

All tests pass:
- `go test ./pkg/consensus/...` - PASS (all consensus packages)
- `go test ./internal/node/dagbft/...` - PASS (all dagbft packages)
- Build: `go build ./pkg/consensus/...` - SUCCESS
- Build: `go build ./internal/node/dagbft/...` - SUCCESS

Note: Full build of `./cmd/accumulated` fails due to unrelated Logger interface mismatches in `exp/light` package, not related to backpressure changes.
