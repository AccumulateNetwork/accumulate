# AppHash Handling in Accumulate

This document describes how Accumulate computes and communicates the `AppHash` to CometBFT (formerly Tendermint), establishing consensus on the blockchain state.

## Overview

The **AppHash** is a 32-byte cryptographic hash that represents the complete state of the Accumulate blockchain at a given block height. It is:

- **Computed by Accumulate** as the root hash of the Binary Patricia Tree (BPT)
- **Communicated to CometBFT** via the ABCI (Application Blockchain Interface)
- **Included in block headers** by CometBFT for consensus verification
- **Used for state validation** when nodes sync or restore from snapshots

**Key fact**: At any block height H, the CometBFT `header.app_hash` is exactly equal to the BPT root hash computed by Accumulate. There is no transformation or combination with other hashes.

## Architecture

```
+------------------+          ABCI          +------------------+
|                  | <-------------------> |                  |
|    CometBFT      |   FinalizeBlock()     |   Accumulate     |
|   (Consensus)    |   ResponseAppHash     |   (Application)  |
|                  |                        |                  |
+------------------+                        +------------------+
        |                                           |
        v                                           v
+------------------+                        +------------------+
| Block Header     |                        | Database Batch   |
| - Height         |                        | - BPT (state)    |
| - AppHash  <-----|------------------------|-- GetRootHash()  |
| - ...            |                        | - Accounts       |
+------------------+                        +------------------+
```

## ABCI Interface Methods

Accumulate implements the CometBFT ABCI interface in `internal/node/abci/accumulator.go`. The following methods communicate AppHash:

### 1. Info (Startup/Handshake)

**File**: `internal/node/abci/accumulator.go:230-288`

When CometBFT starts, it calls `Info()` to determine the application's last known state:

```go
func (app *Accumulator) Info(ctx context.Context, req *abci.RequestInfo) (*abci.ResponseInfo, error) {
    // ...
    res.LastBlockHeight = int64(block.Index)
    res.LastBlockAppHash = hash[:]  // BPT root hash
    // ...
}
```

This allows CometBFT to determine if replay is needed.

### 2. InitChain (Genesis)

**File**: `internal/node/abci/accumulator.go:289-376`

At genesis, the initial AppHash is returned:

```go
func (app *Accumulator) InitChain(ctx context.Context, req *abci.RequestInitChain) (*abci.ResponseInitChain, error) {
    // ... initialize genesis state ...
    return &abci.ResponseInitChain{AppHash: root[:]}, nil
}
```

### 3. FinalizeBlock (Block Execution)

**File**: `internal/node/abci/accumulator.go:378-507`

This is the primary method where AppHash is computed and returned after executing a block:

```go
func (app *Accumulator) FinalizeBlock(ctx context.Context, req *abci.RequestFinalizeBlock) (*abci.ResponseFinalizeBlock, error) {
    // ... execute transactions ...

    // Get the new root
    root, err := app.blockState.Hash()
    if err != nil {
        return nil, err
    }
    res.AppHash = root[:]

    // ... return response ...
}
```

## Code Path: BPT Root to AppHash

The complete code path from BPT computation to ABCI response:

### Step 1: FinalizeBlock Response

**File**: `internal/node/abci/accumulator.go:451-457`

```go
} else {
    // Get the new root
    root, err := app.blockState.Hash()
    if err != nil {
        return nil, err
    }
    res.AppHash = root[:]
}
```

### Step 2: BlockState.Hash() Interface

**File**: `internal/core/execute/execute.go:110-135`

```go
type BlockState interface {
    // ...
    // Hash returns the block hash.
    Hash() ([32]byte, error)
    // ...
}
```

### Step 3: V1/V2 Executor Implementation

**V1**: `internal/core/execute/multi/v1.go:227-229`
```go
func (s *BlockStateV1) Hash() ([32]byte, error) {
    return s.Block.Batch.GetBptRootHash()
}
```

**V2**: `internal/core/execute/v2/block/block.go:48-50`
```go
func (s *BlockState) Hash() ([32]byte, error) {
    return s.Batch.GetBptRootHash()
}
```

### Step 4: Database Batch GetBptRootHash

**File**: `internal/database/bpt.go:25-33`

```go
func (b *Batch) GetBptRootHash() ([32]byte, error) {
    err := b.UpdateBPT()
    if err != nil {
        return [32]byte{}, errors.UnknownError.Wrap(err)
    }
    return b.BPT().GetRootHash()
}
```

### Step 5: BPT.GetRootHash

**File**: `pkg/database/bpt/bpt.go:30-49`

```go
func (b *BPT) GetRootHash() ([32]byte, error) {
    // Execute pending updates
    err := b.executePending()
    if err != nil {
        return [32]byte{}, errors.UnknownError.Wrap(err)
    }

    // Ensure the root node is loaded
    r := b.getRoot()
    err = r.load()
    if err != nil {
        return [32]byte{}, errors.UnknownError.WithFormat("load root: %w", err)
    }

    // Return its hash
    h, _ := r.getHash()
    return h, nil
}
```

## Flowchart

```
                    ┌─────────────────────────────────────────┐
                    │           CometBFT Consensus            │
                    └─────────────────┬───────────────────────┘
                                      │
                                      │ ABCI FinalizeBlock()
                                      ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Accumulator.FinalizeBlock()                         │
│                     internal/node/abci/accumulator.go:378                   │
│                                                                             │
│  1. Begin block                                                             │
│  2. Process transactions                                                    │
│  3. Close block → BlockState                                                │
│  4. Call blockState.Hash() ─────────────────────────────────┐               │
│  5. Set res.AppHash = root[:]                               │               │
│  6. Return FinalizeBlock response                           │               │
└─────────────────────────────────────────────────────────────│───────────────┘
                                                              │
                                                              ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           BlockState.Hash()                                 │
│            internal/core/execute/multi/v1.go:227 (V1 Executor)              │
│            internal/core/execute/v2/block/block.go:48 (V2 Executor)         │
│                                                                             │
│  return s.Batch.GetBptRootHash()                                            │
└─────────────────────────────────────────────────────────────┬───────────────┘
                                                              │
                                                              ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                         Batch.GetBptRootHash()                              │
│                       internal/database/bpt.go:25                           │
│                                                                             │
│  1. UpdateBPT() - apply pending account changes to BPT                      │
│  2. Return b.BPT().GetRootHash()                                            │
└─────────────────────────────────────────────────────────────┬───────────────┘
                                                              │
                                                              ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                           BPT.GetRootHash()                                 │
│                        pkg/database/bpt/bpt.go:30                           │
│                                                                             │
│  1. executePending() - process pending key-value updates                    │
│  2. getRoot().load() - ensure root node is loaded                           │
│  3. Return root.Hash - the 32-byte Merkle root                              │
└─────────────────────────────────────────────────────────────┬───────────────┘
                                                              │
                                                              ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              [32]byte                                       │
│                         BPT Root Hash                                       │
│                                                                             │
│  This is the AppHash - a cryptographic commitment to all account state      │
│  in the partition at this block height.                                     │
└─────────────────────────────────────────────────────────────────────────────┘
```

## BPT and Account State

The BPT (Binary Patricia Tree) stores mappings from account key hashes to account state hashes:

```
BPT Root Hash (AppHash)
        │
        ├── Account 1 Key Hash → Account 1 State Hash
        ├── Account 2 Key Hash → Account 2 State Hash
        ├── Account 3 Key Hash → Account 3 State Hash
        └── ...
```

Each account's state hash is computed by the **Observer** (`internal/database/observer.go`), which hashes:
- Account main state (type, URL, etc.)
- All chains belonging to the account (main chain, pending chain, signature chains, etc.)

The BPT efficiently combines all account hashes into a single root hash using Merkle tree properties.

## Snapshots and AppHash Verification

When creating or restoring snapshots, the AppHash is used for verification:

**File**: `internal/node/abci/snapshot.go:127-149`

```go
bptHash := app.snapshots.request.AppHash  // Expected hash from snapshot
// ...
root, err := batch.GetBptRootHash()       // Computed hash after restore
// Verify they match
```

This ensures snapshot integrity - a restored database must produce the same BPT root hash that was recorded when the snapshot was created.

## Key Files Reference

| Component | File | Line |
|-----------|------|------|
| ABCI Application | `internal/node/abci/accumulator.go` | - |
| FinalizeBlock AppHash | `internal/node/abci/accumulator.go` | 457 |
| Info LastBlockAppHash | `internal/node/abci/accumulator.go` | 248 |
| InitChain AppHash | `internal/node/abci/accumulator.go` | 311, 375 |
| BlockState interface | `internal/core/execute/execute.go` | 110-135 |
| V1 BlockState.Hash | `internal/core/execute/multi/v1.go` | 227-229 |
| V2 BlockState.Hash | `internal/core/execute/v2/block/block.go` | 48-50 |
| Batch.GetBptRootHash | `internal/database/bpt.go` | 25-33 |
| BPT.GetRootHash | `pkg/database/bpt/bpt.go` | 30-49 |
| Observer (hash computation) | `internal/database/observer.go` | - |

## See Also

- [Receipts](./receipts.md) - How Merkle proofs connect transaction hashes to the BPT root
- [SMT/BPT](../internal/database/smt/README.md) - Binary Patricia Tree implementation details
