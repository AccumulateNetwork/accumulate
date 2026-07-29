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
