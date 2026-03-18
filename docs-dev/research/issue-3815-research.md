# Research: Implement BPT Sync Recovery

## Summary

This research documents the existing infrastructure and identifies requirements for implementing BPT-level sync recovery. The codebase already has partial implementations for state sync, snapshot restoration, and certificate synchronization. BPT sync recovery needs to build on these foundations to provide: (1) requesting missing BPT entries from neighbors, (2) background BPT walk to fill gaps, (3) validation passes until all nodes valid, and (4) filling missing chain entries.

## Verified Facts

### Fact 1: BPT Structure - Binary Patricia Tree Implementation
- **Source**: `pkg/database/bpt/bpt.go:16-153` and `pkg/database/bpt/node.go:17-37`
- **Content**: BPT is a Binary Patricia Tree with three node types: `emptyNode`, `leaf`, and `branch`. Each node implements the `node` interface with methods: `Type()`, `IsDirty()`, `getHash()`, `copyWith()`, `writeTo()`, `readFrom()`.
- **Confidence**: HIGH

### Fact 2: BPT Entry Format - Key-Value Pairs
- **Source**: `pkg/database/bpt/bpt.go:16-19`
- **Content**:
```go
type KeyValuePair struct {
    Key   *record.Key
    Value []byte
}
```
- **Confidence**: HIGH

### Fact 3: BPT Root Hash Computation
- **Source**: `pkg/database/bpt/bpt.go:30-49`
- **Content**: `GetRootHash()` executes pending updates, loads root node, and returns the hash. Branch hashes are computed by combining left/right child hashes with SHA-256.
- **Confidence**: HIGH

### Fact 4: BPT Iteration Support
- **Source**: `pkg/database/bpt/iterate.go:28-80`
- **Content**: `Iterate(window int)` returns an `Iterator` that walks the BPT in order, returning `KeyValuePair` slices. Uses `walkRange()` to traverse branches and collect leaf values.
- **Confidence**: HIGH

### Fact 5: BPT Snapshot Save/Load
- **Source**: `pkg/database/bpt/bpt_savestate.go:21-210`
- **Content**: `SaveSnapshotV1()` writes: 8-byte node count, then for each node: 32-byte key, 32-byte hash, 8-byte offset to value. Values follow with 8-byte length prefix. `LoadSnapshotV1()` reads this format and calls `storeState()` for each entry.
- **Confidence**: HIGH

### Fact 6: BPT Receipt Generation (Merkle Proofs)
- **Source**: `pkg/database/bpt/bpt_receipt.go:18-93`
- **Content**: `GetReceipt()` constructs a Merkle proof by walking from leaf to root, collecting sibling hashes at each level. Returns a `*merkle.Receipt` with start value, entries, and anchor.
- **Confidence**: HIGH

### Fact 7: Existing State Sync Framework
- **Source**: `pkg/consensus/snapshot/sync.go:27-32`
- **Content**: Protocol IDs defined:
```go
ProtocolSnapshotList   = "/acc/consensus/snapshot/list/1.0.0"
ProtocolSnapshotFetch  = "/acc/consensus/snapshot/fetch/1.0.0"
ProtocolSnapshotChunk  = "/acc/consensus/snapshot/chunk/1.0.0"
ProtocolStateSync      = "/acc/consensus/state-sync/1.0.0"
```
- **Confidence**: HIGH

### Fact 8: StateSync Implementation
- **Source**: `pkg/consensus/snapshot/sync.go:216-354`
- **Content**: `StateSync` struct handles discovery, download, verify, and apply phases. Uses `SnapshotStore` and `Restorer`. Sync flow: discover snapshots from peers → select best → download → verify → apply → resume.
- **Confidence**: HIGH

### Fact 9: Snapshot Structure
- **Source**: `pkg/consensus/snapshot/snapshot.go:58-85`
- **Content**: Snapshot contains: Version, Height, Round, StateHash ([32]byte), Committee, Certificates array, Timestamp, and optional Metadata.
- **Confidence**: HIGH

### Fact 10: Certificate Sync Mechanism
- **Source**: `pkg/consensus/primary/cert_sync.go:94-127`
- **Content**: `CertSyncer` handles requesting missing certificates from peers. Implements batching (`BatchInterval`), deduplication (`DeduplicationInterval`), and retry logic (`MaxRetries = 10`).
- **Confidence**: HIGH

### Fact 11: Gossip Layer Topics
- **Source**: `pkg/consensus/gossip/gossip.go:39-46`
- **Content**: GossipLayer provides channels for: batches, headers, votes, certificates, syncRequests, and syncResponses.
- **Confidence**: HIGH

### Fact 12: State Hash Tracking
- **Source**: `pkg/consensus/types/state_verification.go:179-206`
- **Content**: `StateHashTracker` tracks local and remote state hashes by round. Detects divergence when hashes don't match. Used for cross-validator state verification.
- **Confidence**: HIGH

### Fact 13: Recovery Manager
- **Source**: `pkg/consensus/recovery.go:106-168`
- **Content**: `RecoveryManager.Recover()` steps: load checkpoint → validate → restore state (primary round, bullshark last commit, DAG) → catch up with peers via certificate sync.
- **Confidence**: HIGH

### Fact 14: Database Snapshot Restore
- **Source**: `internal/database/snapshot/restore.go:32-271`
- **Content**: `RestoreVisitor` processes accounts, transactions, and signatures from snapshots. Calls `batch.UpdateBPT()` after restoring entries. Uses batching (10000 items per batch).
- **Confidence**: HIGH

### Fact 15: Light Client Chain Sync
- **Source**: `exp/light/sync.go:32-203`
- **Content**: `PullAccount()` and `PullAccountWithChains()` fetch account state and chain entries. Compares local vs remote chain heads, identifies mismatches via `identifyBadEntry()`, and pulls missing entries in batches of 1000.
- **Confidence**: HIGH

### Fact 16: BPT-Centric Architecture
- **Source**: `docs/architecture/consensus-and-state-optimization.md:77-113`
- **Content**: Layered sync model: Layer 1 (BPT + Major Block Proof), Layer 2 (Catch-up Sync for recent data), Layer 3 (Interesting Chains), Layer 4 (Full Archival). BPT is authoritative state.
- **Confidence**: HIGH

### Fact 17: State Divergence Handling in Service
- **Source**: `internal/node/dagbft/service.go:530-562`
- **Content**: `onStateDivergence()` halts the service when state divergence is detected. Sets `halted = true`, records `haltReason`, emits event.
- **Confidence**: HIGH

### Fact 18: Request State Sync Stub
- **Source**: `internal/node/dagbft/service.go:648-667`
- **Content**: `RequestStateSync()` is a stub that logs intent but doesn't implement actual sync. Comment indicates sync would be via `snapshot.StateSync`.
- **Confidence**: HIGH

### Fact 19: Resume After Sync
- **Source**: `internal/node/dagbft/service.go:669-696`
- **Content**: `ResumeAfterSync()` resets halt state, updates `lastBlockIndex`, and records new state hash. Requires service to be halted.
- **Confidence**: HIGH

### Fact 20: BPT Mutation Operations
- **Source**: `pkg/database/bpt/mutate.go:24-77`
- **Content**: `Insert()` adds to pending map (deferred), `Delete()` marks for deletion. `executePending()` applies all mutations to tree.
- **Confidence**: HIGH

## Code References

### Primary Implementation Files
- `pkg/database/bpt/bpt.go` - BPT core operations, Get, GetRootHash
- `pkg/database/bpt/node.go` - Node types (emptyNode, leaf, branch)
- `pkg/database/bpt/iterate.go` - BPT iteration for walking entries
- `pkg/database/bpt/bpt_savestate.go` - Snapshot save/load
- `pkg/database/bpt/bpt_receipt.go` - Merkle proof generation
- `pkg/database/bpt/mutate.go` - Insert/Delete operations

### Sync/Recovery Infrastructure
- `pkg/consensus/snapshot/sync.go` - State sync protocol
- `pkg/consensus/snapshot/snapshot.go` - Snapshot structure
- `pkg/consensus/snapshot/restore.go` - Snapshot restoration
- `pkg/consensus/primary/cert_sync.go` - Certificate synchronization
- `pkg/consensus/recovery.go` - Recovery manager

### Service Integration
- `internal/node/dagbft/service.go` - DAG-BFT service with state verification
- `internal/database/snapshot/restore.go` - Database snapshot restoration

### Light Client Reference
- `exp/light/sync.go` - Chain entry sync implementation (reference for chain gap filling)

## Architecture for BPT Sync Recovery

Based on the research, BPT sync recovery should follow this architecture:

### 1. Request Missing BPT Entries from Neighbors

**Existing infrastructure to leverage:**
- `pkg/consensus/gossip/gossip.go` - GossipLayer for peer communication
- `pkg/consensus/primary/cert_sync.go` - Batching/deduplication patterns

**Required new components:**
- BPT sync request/response messages (similar to `CertSyncRequest/CertSyncResponse`)
- Protocol ID for BPT entry requests (e.g., `/acc/consensus/bpt-sync/1.0.0`)
- Request format: list of BPT node keys (32-byte hashes)
- Response format: list of `KeyValuePair` entries

### 2. Background BPT Walk to Fill Gaps

**Existing infrastructure to leverage:**
- `pkg/database/bpt/iterate.go` - BPT iteration
- `pkg/database/bpt/node.go:150-181` - Node loading mechanism

**Required new components:**
- Gap detection: compare local branch hashes against received hashes
- Walk algorithm: DFS/BFS from root, identify branches where local hash differs from expected
- Background goroutine: rate-limited BPT walk that doesn't block consensus

**Algorithm sketch:**
```
1. Get expected root hash from anchor/certificate
2. Walk BPT from root:
   a. Load local node
   b. Compare hash to expected
   c. If match: skip subtree
   d. If mismatch:
      - If branch: recursively check children
      - If leaf: request entry from peers
      - If empty/missing: request subtree
3. Queue missing entries for sync
```

### 3. Validation Passes Until All Nodes Valid

**Existing infrastructure to leverage:**
- `pkg/database/bpt/bpt_receipt.go` - Receipt/proof validation
- `pkg/consensus/types/state_verification.go` - State hash verification

**Required new components:**
- Validation pass coordinator
- Incremental validation: validate subtrees as they're filled
- Convergence tracking: count of invalid nodes, retry logic

**Validation approach:**
```
1. After sync batch completes:
   a. Recompute affected branch hashes
   b. Compare to expected hashes
   c. If still mismatched: re-queue for sync
2. Track convergence:
   - Count remaining invalid nodes
   - If unchanged after N passes: escalate (halt or full resync)
```

### 4. Fill Missing Chain Entries

**Existing infrastructure to leverage:**
- `exp/light/sync.go` - Chain entry sync patterns
- `internal/database/snapshot/restore.go` - Chain restoration

**Required new components:**
- Chain entry gap detection (compare chain head vs BPT entry)
- Chain entry request protocol
- Chain entry insertion with proof verification

**Algorithm:**
```
1. For each BPT entry representing a chain:
   a. Load chain head
   b. Compare to BPT value (chain anchor)
   c. If mismatch: identify missing entries
   d. Request entries from peers
   e. Insert entries, update chain head
   f. Verify BPT entry matches new chain anchor
```

## Open Questions

1. **Priority ordering**: Should BPT sync prioritize certain accounts (e.g., system accounts, validators)?

2. **Bandwidth throttling**: What rate limits should apply to BPT sync requests to avoid network congestion?

3. **Concurrent sync limits**: How many concurrent BPT subtree syncs are allowed?

4. **Proof verification**: Should each synced BPT entry include a proof from a trusted root, or is the final root hash comparison sufficient?

5. **Partial recovery**: Can the node participate in consensus while BPT sync is ongoing (for non-critical subtrees)?

6. **Chain entry format**: What's the exact format for chain entry sync requests/responses?

## Contradictions

No contradictions found between sources. The codebase is consistent in:
- BPT as authoritative state
- Hash-based verification at each level
- Snapshot-based bulk sync + incremental sync for catch-up
- Service halting on state divergence (requires explicit recovery)
