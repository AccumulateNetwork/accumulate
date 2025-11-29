# Snapshot Restore Implementation Plan

## Problem Statement

The snapshot restore functionality (`accumulated restore-snapshot` and the MCP tool `accumulate_restore_from_snapshots`) has multiple failure modes preventing successful follower deployment from Nov 17 snapshots:

1. **Config Loading Chicken-and-Egg**: `restore-snapshot` requires `tendermint.toml` and `accumulate.toml` to exist before it can run, but users often don't have correct configs
2. **Missing Consensus Section**: Snapshots created without `create-snap` tool may lack the consensus section needed to create `genesis.json`
3. **AppHash Mismatch**: Even when genesis.json is created, the AppHash may not match the snapshot root hash, causing CometBFT to fail
4. **State.db Initialization**: CometBFT state.db needs proper initialization to match the snapshot state

## Current Architecture

### Snapshot Creation Flow
```
create-snap tool (cmd/create-snap/main.go)
    → Opens database directly (no config needed)
    → Calls database.Collect() with DidWriteHeader callback
    → DidWriteHeader writes consensus section with validators from NetworkDefinition
    → Creates v2 snapshot with: Header, Records, BPT, Consensus sections
```

### Snapshot Restore Flow
```
accumulated restore-snapshot (cmd/accumulated/cmd_sync.go)
    → Requires --work-dir with existing config files
    → accumulated.Load() calls config.Load() which reads:
        - config/tendermint.toml
        - config/accumulate.toml
    → daemon.LoadSnapshot() (internal/node/daemon/snapshots.go):
        1. Opens snapshot, reads sections
        2. Looks for SectionTypeConsensus
        3. If found: creates genesis.json with validators + AppHash
        4. Initializes state.db and blockstore.db
        5. Restores Accumulate database via snapshot.FullRestore()
```

### MCP Tool Flow
```
accumulate_restore_from_snapshots (mcp/server/tools_snapshot_restore.go)
    → Creates config files first via config.Store()
    → Generates node keys
    → Calls `accumulated restore-snapshot` via exec.Command
```

## Root Cause Analysis

### Issue 1: Config File Requirement
The daemon must be loaded via `accumulated.Load()` which requires config files. This is needed because `daemon.LoadSnapshot()` uses `d.Config` for:
- Database path (`d.Config.Accumulate.Storage.Path`)
- Root directory for genesis.json output
- CometBFT config for state.db location

**Current workaround in MCP tool**: Creates configs before calling restore-snapshot.
**Problem**: If the MCP tool's config creation fails or is incomplete, restore-snapshot fails.

### Issue 2: Missing Consensus Section
Snapshots created by the daemon's `collectSnapshot()` method (during normal operation) do NOT include a consensus section. Only `create-snap` tool adds it.

**From internal/node/daemon/snapshots.go:159**:
```go
_, err = batch.Collect(file, d.Config.Accumulate.PartitionUrl().URL, &coredb.CollectOptions{...})
// No DidWriteHeader callback - no consensus section added!
```

**From cmd/create-snap/main.go:143**:
```go
_, err = db.Collect(file, partitionURL, &database.CollectOptions{
    DidWriteHeader: func(w *snapshot.Writer) error {
        // Writes consensus section with validators
    },
})
```

### Issue 3: AppHash Handling
The genesis.json must have an `app_hash` that exactly matches the snapshot's root hash. The `LoadSnapshot()` function correctly sets this from `rd.Header.RootHash`, but if the consensus section is missing or malformed, the genesis.json may have empty/wrong AppHash.

### Issue 4: CometBFT State Initialization
`LoadSnapshot()` initializes:
- `state.db` at height 0 with the AppHash
- `blockstore.db` at height 0
- `priv_validator_state.json`

If these aren't created or are created with wrong values, CometBFT fails on startup.

## Proposed Fixes

### Fix 1: Add Standalone Restore Command (No Config Required)
Create a new command that can restore a snapshot without requiring pre-existing config files:

```go
// cmd/accumulated/cmd_restore_standalone.go
var cmdRestoreStandalone = &cobra.Command{
    Use:   "restore-genesis [snapshot-file]",
    Short: "Restore a genesis snapshot to initialize a new node (no config required)",
    Args:  cobra.ExactArgs(1),
    Run:   restoreGenesis,
}

func restoreGenesis(_ *cobra.Command, args []string) {
    // 1. Open snapshot file directly
    // 2. Read header to get partition info
    // 3. Create default config based on snapshot metadata
    // 4. Run full restore
    // 5. Generate node keys
}
```

### Fix 2: Ensure Snapshots Have Consensus Section
Modify the daemon's `collectSnapshot()` to include consensus section:

```go
// internal/node/daemon/snapshots.go
func (d *Daemon) collectSnapshot(...) {
    _, err = batch.Collect(file, d.Config.Accumulate.PartitionUrl().URL, &coredb.CollectOptions{
        // ... existing options ...
        DidWriteHeader: func(w *snapshot.Writer) error {
            return d.writeConsensusSection(w)
        },
    })
}
```

### Fix 3: Improve MCP Tool Error Handling
The MCP tool should:
1. Verify snapshot has consensus section before attempting restore
2. If no consensus section, use `create-snap` approach to add one
3. Provide clear error messages about what's missing

### Fix 4: Create Snapshot Validation Tool
Add a command to validate snapshots before restore:

```go
// cmd/accumulated/cmd_validate_snapshot.go
var cmdValidateSnapshot = &cobra.Command{
    Use:   "validate-snapshot [file]",
    Short: "Validate a snapshot file for restore compatibility",
    Run: func(cmd *cobra.Command, args []string) {
        // Check: version, header, consensus section, root hash
        // Report: what's present, what's missing, estimated restore success
    },
}
```

## Implementation Order

### Phase 1: Diagnostic Tools (Low Risk)
1. Add `validate-snapshot` command
2. Add snapshot info to existing debug tools
3. Document snapshot requirements

### Phase 2: Improve MCP Tool (Medium Risk)
1. Add pre-restore validation
2. Better error messages
3. Option to regenerate consensus section if missing

### Phase 3: Standalone Restore (Medium Risk)
1. New `restore-genesis` command that doesn't require config
2. Auto-detects partition from snapshot
3. Creates minimal config during restore

### Phase 4: Improve Snapshot Collection (Higher Risk, Network Impact)
1. Add consensus section to daemon snapshots
2. Requires careful testing - affects all snapshot creation
3. May need version flag for backwards compatibility

## Files to Modify

| File | Change |
|------|--------|
| `cmd/accumulated/cmd_sync.go` | Add validate-snapshot command |
| `cmd/accumulated/main.go` | Register new commands |
| `mcp/server/tools_snapshot_restore.go` | Add validation, better errors |
| `internal/node/daemon/snapshots.go` | Add consensus section to collectSnapshot |
| NEW: `cmd/accumulated/cmd_restore_standalone.go` | Standalone restore command |

## Testing Plan

1. **Unit Tests**: Snapshot validation functions
2. **Integration Tests**:
   - Restore from snapshot with consensus section
   - Restore from snapshot without consensus section (should fail gracefully)
   - Full follower deployment flow
3. **Manual Tests**:
   - Restore Nov 17 snapshots to /mnt/secondary
   - Start follower and verify sync

## Deployment Workflow After Fixes

```bash
# 1. Validate snapshots (new command)
accumulated validate-snapshot directory-genesis-nov17.snap
accumulated validate-snapshot cyclops-genesis-nov17.snap

# 2. Restore using standalone command (new, no config required)
accumulated restore-genesis --work-dir=/mnt/secondary/follower-nov17/dnn directory-genesis-nov17.snap
accumulated restore-genesis --work-dir=/mnt/secondary/follower-nov17/bvnn cyclops-genesis-nov17.snap

# 3. Create dual-node config
# (or use MCP tool which will create configs)

# 4. Start follower
accumulated run-dual /mnt/secondary/follower-nov17/dnn /mnt/secondary/follower-nov17/bvnn
```

## Immediate Workaround (Before Fixes)

If snapshots lack consensus sections, use `create-snap` to regenerate them from the Nov 17 database directories:

```bash
# Assuming Nov 17 node directories are available
cd /path/to/nov17-backup

# Create DN snapshot with consensus section
./create-snap -db dnn/data/accumulate.db -output directory-genesis-fixed.snap -partition Directory -type leveldb

# Create BVN snapshot with consensus section (needs DN for validators)
./create-snap -db bvnn/data/accumulate.db -dn-db dnn/data/accumulate.db -output cyclops-genesis-fixed.snap -partition Cyclops -type leveldb
```
