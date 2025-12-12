# Snapshot Restore with CometBFT Consensus State

## Overview

This document describes the implementation of CometBFT consensus state restoration from Accumulate snapshots. This feature allows follower nodes to be initialized from snapshots that include the necessary CometBFT state.

## Implementation

### Modified File

`internal/node/daemon/snapshots.go` - The `LoadSnapshot` function (lines 237-400)

### Key Changes

1. **Extract Consensus Section**: The snapshot restore process now extracts the consensus section from v2 snapshots, which contains:
   - ChainID (e.g., "MainNet.Cyclops", "MainNet.Directory")
   - Consensus parameters (block size, evidence parameters, etc.)
   - Validator set information

2. **Generate CometBFT genesis.json**: Creates a properly formatted `genesis.json` file in `{work-dir}/config/genesis.json` with:
   - ChainID from the snapshot
   - AppHash (root hash) from the snapshot header
   - UTC-formatted genesis time
   - Consensus parameters
   - Validator set (converted from Accumulate format to CometBFT format)

3. **Validator Conversion**: Converts validators from Accumulate's internal format to CometBFT's GenesisValidator format:
   - Handles ED25519 public keys
   - Preserves voting power and validator names
   - Maintains validator addresses

### Snapshot Format

Accumulate v2 snapshots contain the following sections:
- **Header**: Version, root hash, system ledger info
- **Consensus** (150-152 bytes): CometBFT state including ChainID, params, validators
- **BPT**: Binary Patricia Tree
- **Records**: Account and chain data

### Requirements for Follower Nodes

To start a follower node from a snapshot, you need:

1. **Snapshot Files**:
   - Directory Network: `directory-genesis.snap`
   - Block Validator Network: `{bvn-name}-genesis.snap`

2. **Configuration Files** (in `{work-dir}/config/`):
   - `accumulate.toml` - Accumulate configuration
   - `tendermint.toml` - CometBFT configuration
   - `priv_validator_key.json` - Validator key (for followers, can be dummy)
   - `node_key.json` - P2P node key

3. **Initial State File** (in `{work-dir}/data/`):
   - `priv_validator_state.json` - Initial validator state: `{"height":"0","round":0,"step":0}`

## Usage

### Restore Snapshot

```bash
accumulated restore-snapshot --work-dir <node-directory> <snapshot-file>
```

Example:
```bash
# Restore BVN snapshot
accumulated restore-snapshot --work-dir ./bvnn ./cyclops-genesis.snap

# Restore DN snapshot
accumulated restore-snapshot --work-dir ./dnn ./directory-genesis.snap
```

### What Gets Created

The restore process creates:
1. `{work-dir}/data/accumulate.db/` - Badger database with account state
2. `{work-dir}/config/genesis.json` - CometBFT genesis document

### Start Follower Node

```bash
accumulated run -w <node-directory>
```

The node will:
1. Read the genesis.json to initialize CometBFT state
2. Load the Accumulate database
3. Begin syncing blocks from the network

## Technical Details

### Genesis Document Format

The generated `genesis.json` follows CometBFT's format with numeric fields as strings:

```json
{
  "genesis_time": "2025-11-24T15:10:04.005887926Z",
  "chain_id": "MainNet.Cyclops",
  "initial_height": "1",
  "consensus_params": {
    "block": {
      "max_bytes": "22020096",
      "max_gas": "-1"
    },
    "evidence": {
      "max_age_num_blocks": "100000",
      "max_age_duration": "172800000000000",
      "max_bytes": "1048576"
    },
    "validator": {
      "pub_key_types": ["ed25519"]
    }
  },
  "validators": [
    {
      "address": "F4E2D01AD88CCD00D1E37AFEAC7FADAF6594019A",
      "pub_key": {
        "type": "tendermint/PubKeyEd25519",
        "value": "<base64-encoded-key>"
      },
      "power": "1",
      "name": "Validator-f4e2d01a"
    }
  ],
  "app_hash": "DBCC13E8B68727D8103C75C1915F2BD4C4FB254CC5E87648A9199D6D96E362E8"
}
```

### AppHash Verification

The AppHash in genesis.json MUST match the root hash from the snapshot header. CometBFT verifies this during startup:
- If they don't match: `panic: state.AppHash does not match AppHash after replay`
- The hex-encoded AppHash from the snapshot header is used

### Database Initialization

CometBFT creates its own databases on first start:
- `state.db` - Consensus state
- `blockstore.db` - Block storage
- `tx_index.db` - Transaction index
- `evidence.db` - Evidence storage

These are created fresh and should NOT be copied from validator nodes.

## Troubleshooting

### Common Issues

1. **AppHash Mismatch**
   - Error: `state.AppHash does not match AppHash after replay`
   - Fix: Ensure genesis.json was created from the same snapshot as accumulate.db
   - Delete CometBFT databases and restart

2. **Invalid Genesis Time Format**
   - Error: `JSON time must be UTC and end with 'Z'`
   - Fix: The code now uses `.UTC()` to ensure proper formatting

3. **Invalid Integer Encoding**
   - Error: `invalid 64-bit integer encoding "1", expected string`
   - Fix: CometBFT requires numeric fields as strings in genesis.json
   - The code uses proper JSON marshaling with string conversions

4. **Nil Validator Set**
   - Error: `nil pointer dereference` in ValidatorSet
   - Fix: Validators from snapshot are now properly converted and included

5. **Permission Denied on Databases**
   - Error: `open {path}/LOCK: permission denied`
   - Fix: Ensure databases aren't owned by root; delete and recreate if needed

## Docker Deployment

For containerized deployment, the snapshot restore should be done during container initialization:

```dockerfile
# In dockerfile or entrypoint script
RUN accumulated restore-snapshot --work-dir /data/bvnn /snapshots/cyclops-genesis.snap
RUN accumulated restore-snapshot --work-dir /data/dnn /snapshots/directory-genesis.snap

# Start the nodes
CMD ["accumulated", "run", "-w", "/data/bvnn"]
```

## MCP Integration

The accman MCP server should:

1. Download snapshots from validator backups
2. Extract snapshot files
3. Run `accumulated restore-snapshot` for each partition
4. Copy required configuration files
5. Start the follower nodes

See the accman documentation for specific implementation details.

## Verification

To verify a successful restore:

```bash
# Check genesis.json was created
ls -la {work-dir}/config/genesis.json

# Check Accumulate database size
du -sh {work-dir}/data/accumulate.db

# Verify AppHash matches
grep app_hash {work-dir}/config/genesis.json
# Should match the snapshot's root hash

# Check validator count
grep -A 5 validators {work-dir}/config/genesis.json
```

## References

- Accumulate snapshot format: `pkg/database/snapshot/`
- CometBFT genesis: https://docs.cometbft.com/v0.38/core/using-cometbft#genesis
- Snapshot creation code: `cmd/accumulated/run/snapshot.go`
