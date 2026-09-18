# Snapshot Restore Issues and Fixes

This document describes issues discovered during snapshot-based follower deployment and the fixes applied.

## Issue 1: CometBFT State Validation Error

### Problem

When restoring from V2 snapshots with block data (height ~10.6M), the `restore-genesis` command failed with:

```
Error: failed to save state to state.db: lastHeightChanged cannot be greater than ValidatorsInfo height
```

### Root Cause

The `LoadSnapshot` function in `internal/node/daemon/snapshots.go` was creating a CometBFT genesis document with `InitialHeight` set to the snapshot's block height (e.g., 10,641,161). This caused:

1. `MakeGenesisState()` to set `LastHeightValidatorsChanged = InitialHeight`
2. State saved with `LastBlockHeight = 0`
3. CometBFT validation failed because `LastHeightValidatorsChanged > LastBlockHeight + 1`

### Fix

Changed genesis creation to use `InitialHeight: 1` instead of the snapshot's block height:

```go
// Before (broken)
InitialHeight: consensusDoc.Block.Height,

// After (fixed)
InitialHeight: 1, // Use 1 for follower sync compatibility
```

This allows CometBFT to initialize properly. The follower will:
1. Start with CometBFT state at height 0
2. Load the Accumulate database from the snapshot
3. Sync forward from peers to catch up to current network state

**File:** `internal/node/daemon/snapshots.go:367`

## Issue 2: Block.FromProto Panic on Minimal Blocks

### Problem

When deserializing consensus sections from snapshots, CometBFT's strict block validation caused panics for minimal blocks that don't have all required fields (like LastCommit signatures).

### Root Cause

The `Block.FromProto` function in `pkg/types/cometbft/types.go` would panic on any validation error, even for minimal blocks that contain valid header data we need.

### Fix

Added graceful error handling to extract essential header fields when full validation fails:

```go
func (b *Block) FromProto(c cmtproto.Block) {
    d, err := types.BlockFromProto(&c)
    if err != nil {
        // For snapshot consensus sections, we may have minimal blocks
        // that don't pass CometBFT's strict validation. In this case,
        // we can still extract the essential header fields.
        if c.Header.ChainID != "" {
            b.Header.ChainID = c.Header.ChainID
            b.Header.Height = c.Header.Height
            b.Header.Time = c.Header.Time
            return
        }
        panic(err)
    }
    *(*types.Block)(b) = *d
}
```

**File:** `pkg/types/cometbft/types.go:65-79`

## Issue 3: BVN Snapshot Root Hash Mismatch

### Problem

During BVN (Cyclops) snapshot restore, the process failed with:

```
Error: restore snapshot: failed to restore database: root hash does not match
```

The error indicates:
- Expected hash (from snapshot header): `E1D930B82FA252A6...`
- Got hash (computed after restore): `4F31718D6810130C...`

### Root Cause (Under Investigation)

The `create-snap` tool may not be collecting all necessary data for the BPT (Binary Patricia Tree) to compute correctly. Possible causes:

1. **Incomplete BPT collection**: The BPT section may be missing some nodes
2. **Records ordering**: Records may need to be processed in a specific order
3. **Transaction data**: Some pending transaction data may be missing

### Required Investigation

The `cmd/create-snap/main.go` tool needs review to ensure:

1. All BPT nodes are collected in the correct order
2. All account records are included
3. All message/transaction records are included
4. The collection happens at a consistent database state

## Issue 4: Restored Node Had No Merkle Element Index (#4328, #4330)

### Problem

A node restored from a V2 snapshot diverged from its genesis-built peers in two
opposite ways:

- **By append (#4328).** `<partition>/votes` grew an entry its peers skipped, so
  the account hash, the chain anchor and the BPT root all differed.
- **By refusal (#4330).** The node answered "I never received that anchor" about
  an anchor sitting on its own `anchor(dn)/root` chain.

A poisoned database passes `VerifyHash` and produces the same app hash as a clean
one, so there was no point at which the node learned it was different until it
diverged.

### Root Cause

A snapshot carries a chain's entries but not its **merkle element index**. The
index is an index record, so `collectOptions` walks with `IgnoreIndices: true`
and the BPT does not cover it, and nothing rebuilt it afterwards: the only
`postRestore` implementor was `internal/database/events.go`. So a restored node
had no element index for anything predating its snapshot.

That index answers two different questions during execution:

- **Is this entry already on the chain?** `Chain.AddEntry` reads it to skip a
  duplicate. Production offers a duplicate nearly every block: `block_begin.go`
  captures the ABCI `CommitInfo` into `<partition>/votes`, and that
  transaction's hash carries no height, time, block hash or signature, so in
  steady state it is byte-identical block after block. With no index the skip
  never fires.
- **Do we hold this anchor?** `holdsAnchorRoot` (`msg_synthetic.go`, via
  `IndexOf`), the proof checks in `create_token_account.go` and
  `set_lite_account_delegate.go` (via `HeightOf`), and
  `internal/database/indexing/receipts.go` all ask it about existence. With no
  index the answer is always no.

### Fix

`database.Restore` now rebuilds every chain's element index from its entries,
for every account, once, at restore
(`internal/database/snapshot_chain_index.go`).

Two properties of that rebuild are load-bearing:

1. **First occurrence, not last.** `AddEntry` writes the index only when no
   record exists, so on a node that built its own chain a repeated hash is
   indexed at the height it *first* appeared at. A rebuild that writes the last
   occurrence repairs the dedup and the existence checks — those read presence,
   not the value — but breaks the receipt plane:
   `indexing.getIndexedChainReceipt` does `Receipt(HeightOf(entry), anchorIndex)`,
   and if `HeightOf` names an occurrence *after* that anchor the receipt fails
   outright with `invalid range: from (26) > to (21)`. A missing index is a clean
   `NotFound` a caller can handle; a plausible wrong index is not.
2. **Committed in chunks.** The rebuild commits and replaces its batch every
   `BatchRecordLimit` entries (default 50,000), the way the restore loop already
   does. Buffering every write into one in-memory batch is fine on a 14-account
   simulator and will not fit a real store's millions of accounts and tens of
   millions of entries.
3. **A filtered restore is tolerated, an unfiltered one is not.** A restore that
   passes a `Predicate` may legitimately keep a chain's head and drop the records
   its entries live in — `genesis.Extract` does exactly that, keeping `Main` and
   the `MainChain` `Head` while dropping the chain's mark points (`States`) for
   anything that is not a data account. Since a snapshot carries no `Element`
   records either (`Element` is an index record, so `IgnoreIndices` drops it),
   `Chain.Entry` then cannot reconstruct any element below the last mark point,
   and every account with more than 256 entries is unreadable. With a `Predicate`
   in play the rebuild leaves that chain unindexed and carries on; with no
   `Predicate` an unreadable entry means a corrupt database and still fails the
   restore.

   It **stops** at the first unreadable entry rather than skipping past it.
   `Entry` succeeds at or above the last mark point (those live in the head's
   hash list), so skipping would index a tail while leaving the history
   unscanned, and a hash whose first occurrence is in that unscanned history
   would be recorded at a *later* position — reintroducing the wrong value
   point 1 exists to prevent.

### Operator impact

**A restore now takes materially longer.** The rebuild reads every entry of every
chain of every account, so its cost is linear in the total number of chain
entries in the snapshot, on top of the restore itself. This is a one-time cost
per restore; it is not paid again at startup.

**What has been measured, and what has not.** On a 100k-account x 20-entry
construction (2M entries), the rebuild cost 22s against a 6.4s baseline, and
peak heap was **+340 MiB over baseline at the default chunk of 50,000** — the
figure to check against a GOMEMLIMIT. Those numbers come from synthetic chains
in a freshly written badger database in a temp directory on a developer
workstation; "mainnet shape" describes the account-to-entry ratio, not mainnet.
**No mainnet restore has ever been timed.** Extrapolating linearly gives roughly
six to seven minutes for 35M entries, but treat that as a FLOOR rather than an
estimate: the rebuild pays two guaranteed key misses per entry — `Chain.Entry`
reads `Element(i)`, which a v2 snapshot never carries, and the existence check
reads `ElementIndex(hash)`, which misses on every entry of a fresh restore — and
on a multi-hundred-GB LSM with a cold page cache those cost materially more than
they do here. The measured rate already fell from 188k entries/s at 240k entries
to about 90k/s at 2M, on a tiny store. Peak heap is bounded by the chunk rather
than by the entry count; wall clock is not.

**Files:** `internal/database/snapshot_chain_index.go`,
`internal/database/snapshot.go`

**Not fixed here — the v1 path, which is WORSE and is reachable from a node.**
`internal/database/snapshot/restore.go:137` rebuilds the index for *system
accounts only* (the gate is
`if _, ok := protocol.ParsePartitionUrl(acct.Url.RootIdentity()); ok`), so a user
account restored from a v1 snapshot gets no index for its pre-snapshot entries.

For the system accounts it *does* rebuild, it uses an unconditional ascending
`Put` (`merkle_snapshot.go:117-146`, `RestoreElementIndexFromHead` and
`RestoreElementIndexFromMarkPoints`), so **the last occurrence of a repeated hash
wins** — the exact variant rejected above, already shipped on this path. And
`<partition>/anchors` is a system account, so the damage is not confined to the
read-side receipt plane: `AddChainEntry2` returns `c.HeightOf(entry)` on its
duplicate branch (`v2/chain/state_state.go:148-150`), that value flows through
`DidReceiveAnchor` into `anchorChain.Receipt(received.Index, entry.Source)` at
`block_end.go:670`, and that receipt is built into the **outgoing anchor**. A
v1-restored node can therefore emit a different anchor receipt from its peers, or
fail block production outright when the range inverts.

**v1 restore is reachable from a node**, not only from tooling:
`snapshot.FullRestore` (`internal/database/snapshot/full.go:70-93`)
version-dispatches, and is called from `internal/node/abci/accumulator.go:344`
(InitChain), `internal/node/abci/snapshot.go:146` (state sync
`ApplySnapshotChunk`), `internal/node/daemon/snapshots.go:507`, and
`internal/bsn/executor.go:150`.

This was left out of the change above deliberately — it is a distinct defect on a
distinct path and wants its own fix and its own tests — not because it is
unreachable. See **#4341**, and #4321 for the off-by-one arithmetic in the same two functions.

## create-snap Tool Requirements

The `cmd/create-snap/main.go` tool creates V2 snapshots with consensus sections. It needs the following improvements:

### Current Capabilities

- Creates V2 snapshots from LevelDB or Badger databases
- Reads SystemLedger for block height and timestamp
- Creates consensus section with Block data (ChainID, Height, Time)
- Extracts validators from NetworkDefinition

### Improvements Needed

1. **BPT Integrity Verification**
   - After collection, verify the BPT root hash matches the header
   - Log detailed BPT statistics (node count, depth, etc.)

2. **Full Records Collection**
   - Ensure all record types are collected
   - Add progress logging for large databases
   - Consider checkpointing for resumable collection

3. **Validation Mode**
   - Add `--validate` flag to verify snapshot without writing
   - Compare computed hash against header hash

4. **Error Handling**
   - Better error messages when collection fails
   - Partial snapshot cleanup on failure

### Example Usage

```bash
# Create DN snapshot
./create-snap -db /path/to/dnn/data/accumulate.db \
  -output /output/dn.snap \
  -partition Directory \
  -type leveldb

# Create BVN snapshot (requires DN database for network definition)
./create-snap -db /path/to/bvnn/data/accumulate.db \
  -dn-db /path/to/dnn/data/accumulate.db \
  -output /output/bvn.snap \
  -partition Cyclops \
  -type leveldb
```

## Testing Recommendations

1. **Small Database Test**: Test snapshot creation/restore on a devnet first
2. **Hash Verification**: Add post-restore hash verification
3. **Sync Test**: Verify restored follower can sync from mainnet peers
4. **Dual-Node Test**: Test both DN and BVN together with `run-dual`

## Related Files

- `internal/node/daemon/snapshots.go` - LoadSnapshot function
- `pkg/types/cometbft/types.go` - Block.FromProto function
- `cmd/create-snap/main.go` - Snapshot creation tool
- `cmd/accumulated/cmd_snapshot.go` - restore-genesis command
- `pkg/database/snapshot/` - Snapshot format definitions
- `internal/database/snapshot.go` - V2 collect and `database.Restore`
- `internal/database/snapshot_chain_index.go` - Merkle element index rebuild (Issue 4)
