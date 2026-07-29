# Snapshot Restore Implementation Status

**Date**: 2025-11-29
**Status**: Implementation Complete - Tested and Working
**Branch**: `3691-mcp-server-for-accumulate`

## Summary

Full implementation of snapshot restore functionality completed. This includes:
- MCP tool `accumulate_restore_from_snapshots` with pre-validation
- MCP tool `accumulate_validate_snapshot` for snapshot inspection
- CLI commands `validate-snapshot` and `restore-genesis`
- Core fixes to CometBFT state initialization in `LoadSnapshot()`

## What Was Implemented

### Files Created/Modified

1. **`cmd/accumulated/cmd_snapshot.go`** (NEW)
   - `validate-snapshot` command - validates snapshot files for restore compatibility
   - `restore-genesis` command - standalone restore without pre-existing config
   - Checks version, sections, consensus data, validators, root hash
   - Auto-detects partition from consensus ChainID

2. **`internal/node/daemon/snapshots.go`** (MODIFIED)
   - Fixed ED25519 public key handling (slice vs array bug)
   - Uses CometBFT's `SaveAs()` for proper JSON serialization
   - Initializes `state.db` with proper genesis state
   - Initializes `blockstore.db` at height 0
   - Creates `priv_validator_state.json`

3. **`mcp/server/tools_snapshot_restore.go`** (MODIFIED)
   - Added `validateSnapshot()` function for pre-restore validation
   - Added `validateSnapshotTool()` MCP handler
   - Added `SnapshotValidationResult` struct
   - Integrated pre-validation into `restoreFromSnapshots()`

4. **`mcp/server/tool_definitions.go`** (MODIFIED)
   - Added tool definition for `accumulate_validate_snapshot`

5. **`mcp/server/server.go`** (MODIFIED)
   - Added dispatcher case for `accumulate_validate_snapshot`

6. **`cmd/accumulated-bootstrap/info_server.go`** (MODIFIED)
   - Added `/connect` endpoint for peer connection requests

### Features Implemented

#### Phase 1: Diagnostic Tools (Complete)
- `accumulated validate-snapshot <file>` command
- Checks: version, root hash, consensus section, validators, BPT, records
- Clear output with issues and warnings
- Exit code 0 for valid, 1 for invalid

#### Phase 2: MCP Tool Improvements (Complete)
- `accumulate_validate_snapshot` MCP tool
- Pre-validation before restore (validates both DN and BVN snapshots)
- Clear error messages with validation details
- Returns structured validation results

#### Phase 3: Standalone Restore Command (Complete)
- `accumulated restore-genesis <snapshot>` command
- Works without pre-existing config files
- Auto-detects partition from consensus ChainID
- Creates default follower configuration
- Supports `--network` and `--partition` flags

#### Phase 4: Core Snapshot Restore Fixes (Complete)
- Fixed ED25519 public key slice/array handling
- Proper genesis.json serialization via CometBFT
- CometBFT state.db initialization from genesis
- CometBFT blockstore.db initialization at height 0
- Creates priv_validator_state.json

## Usage

### Validate a Snapshot

```bash
accumulated validate-snapshot /path/to/snapshot.snap
```

Output:
```
Validating snapshot: /path/to/snapshot.snap

Version: 2
Root Hash: b166048d9c3c89417ea3aec01afa0e671332391e08660f9d8c1ee6605bacb79b
Partition: acc://dn.acme/ledger
Block Index: 1
Timestamp: 2025-07-13 13:49:18 +0000 UTC

Sections (5 total):
  - header          (offset: 64, size: 115 bytes)
  - consensus       (offset: 256, size: 152 bytes)
  - bpt             (offset: 512, size: 2553 bytes)
  - records         (offset: 3136, size: 107508 bytes)
  - records         (offset: 110720, size: 1980278 bytes)

Consensus Section:
  Chain ID: MainNet.Directory
  Validators: 1

=== VALIDATION SUMMARY ===

[OK] Snapshot is valid and can be restored
```

### Restore from Snapshot (Standalone)

```bash
accumulated restore-genesis --work-dir=/path/to/node directory-genesis.snap
```

### Restore via MCP

```json
{
  "method": "tools/call",
  "params": {
    "name": "accumulate_restore_from_snapshots",
    "arguments": {
      "dn_snapshot": "/path/to/dn.snap",
      "bvn_snapshot": "/path/to/bvn.snap",
      "work_dir": "/var/accumulate/follower-1",
      "network": "MainNet",
      "bvn_name": "Cyclops"
    }
  }
}
```

### Validate via MCP

```json
{
  "method": "tools/call",
  "params": {
    "name": "accumulate_validate_snapshot",
    "arguments": {
      "snapshot_path": "/path/to/snapshot.snap"
    }
  }
}
```

## Validation Checks

The validation checks:
1. **Version**: Must be v2 snapshot format
2. **Root Hash**: Must be present in header
3. **Consensus Section**: Required for genesis.json creation
4. **ChainID**: Extracted from consensus for partition detection
5. **Validators**: Counted and reported
6. **Records Section**: Required for database restore
7. **BPT Section**: Checked (warning if missing)

## Known Issues Resolved

### Critical Issues (Fixed)
- Node key generation now handled
- Bootstrap peers configured
- Dual-node config implemented
- restore-snapshot command tested and working

### CometBFT State Issues (Fixed)
- state.db properly initialized from genesis
- blockstore.db initialized at correct height
- priv_validator_state.json created
- Genesis JSON serialization fixed (int64 as strings)

## Testing Status

### Build Status
- All packages compile successfully
- MCP tests pass

### Validation Testing
- Tested with real DN and BVN snapshots
- Correctly identifies consensus section
- Properly extracts partition from ChainID
- Validates all required sections

## Related Documentation

- **Design Docs**: `mcp/snapshot_restore_readme.md`
- **Implementation Plan**: `PLAN.md` (in repository root)
- **Accman Review**: `mcp/accman_snapshot_restore_review.md`

---

**Last Updated**: 2025-11-29
**Branch**: 3691-mcp-server-for-accumulate
