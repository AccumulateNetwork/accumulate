# Specification: BPT Sync Recovery

## Overview

BPT sync recovery is a mechanism for recovering from state divergence in DAG-BFT consensus. When a node detects that its local Binary Patricia Tree (BPT) state differs from the expected state (derived from committed certificates), this system coordinates the recovery process.

## Components

### 1. BPTSyncer (`pkg/consensus/primary/bpt_sync.go`)

**Purpose:** Request missing BPT entries from network peers via gossip.

**Interface Requirements:**
```go
// BPTStore - implementations MUST be thread-safe
type BPTStore interface {
    GetEntry(keyHash [32]byte) ([]byte, error)
    PutEntry(keyHash [32]byte, value []byte) error
    HasEntry(keyHash [32]byte) (bool, error)
}
```

**Configuration:**
| Parameter | Default | Description |
|-----------|---------|-------------|
| BatchInterval | 100ms | Collection window for batching requests |
| DeduplicationInterval | 10s | Minimum time before re-requesting the same key |
| RetryTimeout | 30s | Time before a request is considered failed |
| MaxRetries | 5 | Maximum attempts per key hash |
| JitterMax | 200ms | Maximum random jitter added to batch timing |

**Operation:**
1. `RequestMissing(keyHashes)` queues keys for sync
2. Keys are deduplicated against in-flight requests
3. After `BatchInterval + random(0, JitterMax)`, batch is sent via `BroadcastBPTSyncRequest`
4. Responses arrive via `SubscribeBPTSyncResponses` channel
5. Received entries are stored and callback invoked
6. Failed requests are retried up to `MaxRetries` times

**Message Format (gossip.BPTSyncRequest):**
```
[RequestID: 8 bytes][RequesterLen: 4 bytes][Requester: variable]
[NumKeys: 4 bytes][Key1: 32 bytes][Key2: 32 bytes]...
```

**Message Format (gossip.BPTSyncResponse):**
```
[RequestID: 8 bytes][NumEntries: 4 bytes]
[KeyHash1: 32 bytes][ValueLen1: 4 bytes][Value1: variable]...
[NumMissing: 4 bytes][MissingKey1: 32 bytes]...
```

**Limits:**
- `MaxBPTSyncKeys = 1000` - Maximum keys per request
- `MaxBPTSyncEntries = 100` - Maximum entries per response
- `MaxBPTValueSize = 1024` - Maximum value size in bytes

### 2. BPTWalker (`pkg/consensus/primary/bpt_walker.go`)

**Purpose:** Periodically walk the BPT to detect divergence and queue missing entries for sync.

**Interface Requirements:**
```go
// BPTHashProvider - implementations MUST be thread-safe
type BPTHashProvider interface {
    GetExpectedRootHash() *[32]byte  // Returns nil if not available
    GetLocalRootHash() ([32]byte, error)
    GetBranchHash(key [32]byte) ([32]byte, bool, error)
    GetLeafValue(keyHash [32]byte) ([]byte, bool, error)
    IterateKeys(fn func(keyHash [32]byte, value []byte) error) error
}
```

**Configuration:**
| Parameter | Default | Description |
|-----------|---------|-------------|
| WalkInterval | 5s | Time between walk cycles |
| BatchSize | 100 | Entries to process per step |
| MaxPendingRequests | 1000 | Maximum queued missing keys |

**Operation:**
1. Every `WalkInterval`, compare `GetLocalRootHash()` to `GetExpectedRootHash()`
2. If hashes match: state is consistent, clear divergence flag
3. If hashes differ: set divergence flag, find missing keys
4. Queue missing keys to BPTSyncer (up to `MaxPendingRequests`)
5. When entries received, `OnEntryReceived()` removes from pending

**Current Limitation:** The `findMissingKeys()` implementation is simplified. It re-uses pending keys rather than performing a full tree-walking comparison. A production implementation should:
1. Compare branch hashes at each level
2. Only descend into branches with mismatched hashes
3. Collect leaf keys from mismatched subtrees

### 3. BPTValidator (`pkg/consensus/primary/bpt_validator.go`)

**Purpose:** Run validation passes to confirm convergence after sync.

**Interface Requirements:**
```go
// BPTValidationProvider - implementations MUST be thread-safe
type BPTValidationProvider interface {
    GetExpectedRootHash() *[32]byte
    GetLocalRootHash() ([32]byte, error)
    RecalculateHashes() error  // Force recomputation of BPT hashes
}
```

**Configuration:**
| Parameter | Default | Description |
|-----------|---------|-------------|
| ValidationInterval | 2s | Time between validation passes |
| MaxPasses | 10 | Maximum passes before escalating to full resync |
| ConvergenceThreshold | 3 | Consecutive successes required for convergence |

**State Machine:**
```
[Idle] --start--> [Running] --hash_match--> (count consecutiveSuccess)
                      |                            |
                      |                    (count >= threshold)
                      |                            |
                      v                            v
              [Running] <--retry-- (hash_mismatch) [Converged]
                      |
              (passes >= MaxPasses)
                      |
                      v
                  [Failed]
```

**Operation:**
1. Only runs when walker reports `IsDiverged() == true`
2. Each pass: `RecalculateHashes()`, compare root hashes
3. On match: increment `consecutiveSuccess`
4. After `ConvergenceThreshold` consecutive matches: emit `onConverged` callback
5. On mismatch: reset `consecutiveSuccess`, trigger walker
6. After `MaxPasses` without convergence: emit `onFailed` callback

### 4. ChainGapFiller (`pkg/consensus/primary/chain_gap_filler.go`)

**Purpose:** Fill missing chain entries after BPT sync completes.

**Interface Requirements:**
```go
// ChainEntryProvider - implementations MUST be thread-safe
type ChainEntryProvider interface {
    GetChainHead(chainKey [32]byte) (count uint64, anchor [32]byte, err error)
    GetExpectedChainAnchor(chainKey [32]byte) ([32]byte, bool)
    IterateChainKeys(fn func(chainKey [32]byte, expectedAnchor [32]byte) error) error
}

// ChainEntryRequester - for fetching entries from network
type ChainEntryRequester interface {
    RequestChainEntries(ctx context.Context, chainKey [32]byte, startIndex, count uint64) ([][]byte, error)
    InsertChainEntry(chainKey [32]byte, index uint64, entry []byte) error
}
```

**Configuration:**
| Parameter | Default | Description |
|-----------|---------|-------------|
| CheckInterval | 5s | Time between gap checks |
| BatchSize | 100 | Entries to request per batch |
| MaxPending | 1000 | Maximum queued gaps |

**Operation:**
1. Every `CheckInterval`, iterate chain keys
2. For each chain: compare local anchor to expected anchor (from BPT)
3. If mismatch: add `ChainGap` to pending list
4. For each pending gap: request entries, insert, verify anchor
5. If still mismatched: create new gap for remaining entries

**Note:** Chain entry requests use the `ChainEntryRequester` interface, which should be implemented using existing API mechanisms (not a new gossip topic).

### 5. BPTRecoveryCoordinator (`pkg/consensus/recovery/bpt_recovery.go`)

**Purpose:** Coordinate the overall recovery process.

**Recovery State Machine:**
```
[Idle] --start--> [Detecting] --> [Syncing] --> [Validating] --> [FillingChains] --> [Complete]
                       |              |              |                  |
                       |              |              v                  |
                       |              |          [Failed] <-------------+
                       |              |              ^                  |
                       |              +---(timeout)--+                  |
                       +-------------(timeout)----------------------->>+
```

**State Transitions:**
| From | To | Condition |
|------|----|-----------|
| Idle | Detecting | `Start()` called |
| Detecting | Syncing | Immediately after start |
| Syncing | Validating | `!walker.IsDiverged() && walker.PendingGapCount() == 0` |
| Validating | FillingChains | `validator.State() == ValidationStateConverged` |
| FillingChains | Complete | `chainFiller.PendingGapCount() == 0` |
| Any | Failed | Timeout (`RecoveryTimeout`, default 5m) or 3 validation failures |

**Callbacks:**
- `onComplete()` - Recovery succeeded
- `onFailed(reason)` - Recovery failed with reason

**Wiring:**
1. BPTSyncer.onEntryReceived → BPTWalker.OnEntryReceived
2. BPTValidator.onConverged → Coordinator.onValidationConverged
3. BPTValidator.onFailed → Coordinator.onValidationFailed

## Gossip Integration

**Topic:** `acc/{partition}/consensus/bpt-sync`

**Message Types:**
- Request (type byte = 0x01): `BPTSyncRequest`
- Response (type byte = 0x02): `BPTSyncResponse`

**Flow:**
1. Node A broadcasts `BPTSyncRequest` with missing key hashes
2. All nodes receive via gossip subscription
3. Nodes with entries broadcast `BPTSyncResponse`
4. Node A receives responses, stores entries, updates walker

## Thread Safety

All interface implementations (`BPTStore`, `BPTHashProvider`, `BPTValidationProvider`, `ChainEntryProvider`, `ChainEntryRequester`) **MUST be thread-safe**. The coordinator runs multiple goroutines concurrently:
- Syncer: request handler, response handler, retry loop
- Walker: walk loop
- Validator: validation loop
- ChainFiller: gap check loop
- Coordinator: monitor loop

## Error Handling

1. **Network errors:** Requests are retried up to `MaxRetries` times
2. **Store errors:** Logged and operation continues
3. **Timeout:** After `RecoveryTimeout`, coordinator transitions to Failed state
4. **Validation failures:** Up to 3 attempts before failing

## Metrics

Each component exposes metrics via `Metrics()` method:
- BPTSyncer: requestsSent, responsesRecv, entriesRecv
- BPTWalker: walkCycles, gapsFound, gapsResolved
- BPTValidator: totalPasses, successfulPasses, failedPasses
- ChainGapFiller: gapsDetected, gapsFilled, entriesRecv

## Testing

All components have corresponding `*_test.go` files with:
- Unit tests for individual operations
- Integration tests for component interactions
- Edge case coverage (nil inputs, empty slices, errors)

Run tests: `go test ./pkg/consensus/primary/... ./pkg/consensus/recovery/... -v`
