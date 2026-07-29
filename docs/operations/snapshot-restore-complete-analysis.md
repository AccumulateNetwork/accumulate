# Accumulate Snapshot Restore: Complete Analysis

## Goal

**Create an automated, repeatable process to:**
1. **Capture snapshots** from a running validator's data directory
2. **Restore snapshots** to deploy new follower nodes that exactly recreate the validator's state

The objective is to enable anyone to deploy an Accumulate follower node in hours (not weeks) by:
- Taking periodic snapshots from validators
- Distributing portable snapshot files
- Restoring those snapshots to initialize new followers

**Success criteria**: A follower restored from a snapshot must have an identical `accumulate.db` state to the original validator at the snapshot height, and must be able to sync forward from that point.

---

## Executive Summary

This document describes the complete journey of attempting to restore a follower node from validator snapshots taken on November 17, 2025. Despite having a complete backup of all validator data including the blockstore, a critical piece of data (the CometBFT commit signatures) was not included in the snapshot format, preventing successful restore at the correct blockchain height.

---

## CometBFT State Sync Specification

### Official CometBFT State Sync Architecture

CometBFT's state sync is designed as a **two-source system**:

| Data Type | Source | Description |
|-----------|--------|-------------|
| Application State | ABCI Snapshot Chunks | Application-specific state data in chunks |
| Consensus Data | Light Client RPC | Block headers, commits, validator sets |

**References:**
- [CometBFT State Sync Documentation (v0.37)](https://docs.cometbft.com/v0.37/core/state-sync)
- [CometBFT ABCI App Requirements (v0.38)](https://docs.cometbft.com/v0.38/spec/abci/abci++_app_requirements)
- [Cosmos SDK Snapshots Package](https://pkg.go.dev/github.com/cosmos/cosmos-sdk/snapshots)

### ABCI Snapshot Format (What Goes in the Snapshot)

Per the CometBFT specification, ABCI snapshots contain:

```protobuf
message Snapshot {
    uint64 height = 1;    // Height at which snapshot was taken
    uint32 format = 2;    // Application-defined format version
    uint32 chunks = 3;    // Number of chunks
    bytes  hash = 4;      // SHA-256 hash of entire snapshot
    Metadata metadata = 5; // Application-specific metadata
}
```

**Key Properties Required:**
- **Consistency**: Snapshot at single isolated height, no concurrent modifications
- **Asynchronous**: Must not halt chain progress
- **Determinism**: Identical byte-level output across nodes at same height

### Light Client Data (What Comes from Peers)

The **commit signature data** is NOT part of the ABCI snapshot. Instead, CometBFT fetches it from **Light Client RPC servers** during state sync:

```bash
# How CometBFT obtains trust parameters from RPC
curl -s https://rpc-server:26657/commit | jq "{
  height: .result.signed_header.header.height,
  hash: .result.signed_header.commit.block_id.hash
}"
```

**Required Configuration for State Sync:**
```toml
[statesync]
enable = true
rpc_servers = "rpc1:26657,rpc2:26657"  # Minimum 2 RPC servers
trust_height = 10641161                  # Height to trust
trust_hash = "ABCD1234..."              # BlockID hash at trust_height
trust_period = "168h"                    # Verification window
```

### The Bootstrap Process

1. **Discovery**: CometBFT queries peers via `ListSnapshots` ABCI call
2. **Offer**: Snapshots offered to application via `OfferSnapshot`
3. **Download**: Chunks fetched via `LoadSnapshotChunk`
4. **Apply**: Chunks applied via `ApplySnapshotChunk`
5. **Verification**: CometBFT fetches **light block (commit)** from RPC peers
6. **Validation**: App hash verified against the trusted chain hash
7. **Transition**: Node joins consensus with truncated block history

### Critical Insight: Our Scenario

**Standard State Sync requires live network peers** to provide the commit via Light Client RPC.

**Our scenario is "offline restore"** - we have a snapshot file but no live peers to query for the commit.

For offline/standalone restore to work, the snapshot format **must include the commit** because there are no peers to fetch it from. This is where the Accumulate snapshot format diverges from the CometBFT state sync assumption.

### What the Accumulate Snapshot Format Should Include

For complete offline restore capability, our snapshot needs:

| Component | CometBFT Source | Accumulate Snapshot |
|-----------|-----------------|---------------------|
| App State | ABCI Snapshot | ✅ Included (BPT + Records) |
| Consensus Params | Genesis/RPC | ✅ Included |
| Validator Set | Genesis/RPC | ✅ Included |
| Block Header | Light Client | ✅ Included (partial) |
| **Commit (Signatures)** | **Light Client RPC** | ❌ **MISSING** |

**The Commit is the missing piece** that prevents offline restore at non-genesis heights.

---

## 1. What We Captured

### Source: Validator Backup - November 17, 2025

On November 17, 2025, we captured a **complete backup** of the Accumulate MainNet validator, including:

#### Directory Network (DN)
- **Location**: `/media/paul/Expansion/databases/validator_backup_20251117/extracted/dnn/`
- **Block Height**: 10,641,161
- **Timestamp**: 2025-11-17 20:54:42 UTC
- **Chain ID**: MainNet.Directory
- **Contents**:
  - `data/accumulate.db` - Full Accumulate application database (LevelDB)
  - `data/blockstore.db` - CometBFT block storage with commits
  - `data/state.db` - CometBFT state database
  - `data/evidence.db` - Evidence database
  - `data/tx_index.db` - Transaction index
  - `config/` - Node configuration files
  - `config/genesis.json` - Genesis document
  - `config/priv_validator_key.json` - Validator key (for signing)

#### Block Validator Network - Cyclops (BVN)
- **Location**: `/media/paul/Expansion/databases/validator_backup_20251117/extracted/bvnn/`
- **Block Height**: 10,639,083
- **Timestamp**: 2025-11-17 20:41:59 UTC
- **Chain ID**: MainNet.Cyclops
- **Contents**: Same structure as DN

### App Hash (State Root) at Capture
- **DN**: `5C59946BDCA1EED7382E935730B183096DC0D57485298232378864D261758C06`
- **BVN**: `E1D930B82FA252A6F42FAFDF921A21684A12D1F7E4244D6C049D458AF23265E3`

### Validator Information
- **Address**: `F4E2D01AD88CCD00D1E37AFEAC7FADAF6594019A`
- **Public Key**: `c4SRXfGWCe41oXl6ZjrePadpUSLNt+u1QwanVzdPyug=` (Ed25519)
- **Name**: `Validator-f4e2d01a`
- **Voting Power**: 1

---

## 2. How the Snapshots Were Built

### Snapshot Creation Tool
We used the custom `create-snap` tool located at `cmd/create-snap/` to create portable snapshot files from the validator's Accumulate database.

### Snapshot Format (Version 2)
The snapshot format consists of 5 sections:

| Section | Type | Description |
|---------|------|-------------|
| 0 | Header | Metadata about the snapshot |
| 1 | Consensus | CometBFT genesis parameters, validators, block header |
| 2 | BPT | Binary Patricia Tree (Merkle proof structure) |
| 3 | Records | All Accumulate account records and state |
| 4 | (Additional data) | Various supporting data |

### Commands Used
```bash
# Create DN snapshot
./create-snap \
  -db /media/paul/Expansion/databases/validator_backup_20251117/extracted/dnn/data/accumulate.db \
  -output directory-nov17-new.snap \
  -partition Directory \
  -type leveldb

# Create BVN snapshot
./create-snap \
  -db /media/paul/Expansion/databases/validator_backup_20251117/extracted/bvnn/data/accumulate.db \
  -dn-db /media/paul/Expansion/databases/validator_backup_20251117/extracted/dnn/data/accumulate.db \
  -output cyclops-nov17-new.snap \
  -partition Cyclops \
  -type leveldb
```

### Resulting Snapshot Files
- **DN Snapshot**: `directory-nov17-new.snap` (~66 MB)
- **BVN Snapshot**: `cyclops-nov17-new.snap` (~1.5 GB)

### What the Consensus Section Contains
```go
type GenesisDoc struct {
    ChainID    string
    Params     *ConsensusParams  // Block size, evidence params, etc.
    Validators []*Validator       // Validator set with public keys
    Block      *Block             // Minimal block header (height, time, chain_id)
}
```

---

## 3. How We Deploy the Snapshots

### Step 1: Initialize Follower Node Configuration
```bash
# Create directory structure
mkdir -p /mnt/secondary/follower-nov17-new/dnn/config
mkdir -p /mnt/secondary/follower-nov17-new/bvnn/config

# Copy base configuration (accumulate.toml, node_key.json, etc.)
```

### Step 2: Restore from Snapshot
```bash
# Restore DN
accumulated restore-snapshot \
  --work-dir /mnt/secondary/follower-nov17-new/dnn \
  /mnt/secondary/snapshots-fixed/directory-nov17-new.snap

# Restore BVN
accumulated restore-snapshot \
  --work-dir /mnt/secondary/follower-nov17-new/bvnn \
  /mnt/secondary/snapshots-fixed/cyclops-nov17-new.snap
```

### What the Restore Process Does
1. Opens the snapshot file and reads sections
2. Extracts consensus parameters to create `genesis.json`
3. Initializes CometBFT's `state.db` with:
   - `InitialHeight` = snapshot height (e.g., 10,641,161)
   - `LastBlockHeight` = snapshot height
   - `Validators` and `LastValidators` with proposer selection
   - `AppHash` matching the snapshot root
4. Initializes `blockstore.db` with height matching state
5. Creates `priv_validator_state.json`
6. Restores the Accumulate database from BPT and records sections

### Step 3: Start the Follower
```bash
accumulated run-dual \
  /mnt/secondary/follower-nov17-new/dnn \
  /mnt/secondary/follower-nov17-new/bvnn
```

---

## 4. What Is Currently Wrong

### The Error
```
panic: failed to reconstruct vote set from commit: failed to verify vote with
ChainID MainNet.Directory and PubKey PubKeyEd25519{...}: invalid signature
```

### Root Cause Analysis

When CometBFT starts at any height greater than 0, it performs the following validation:

```go
// In consensus/state.go
func (cs *State) reconstructLastCommit(state sm.State) {
    if state.LastBlockHeight == 0 {
        return  // Skip for genesis
    }

    // Load the commit from blockstore
    commit := cs.blockStore.LoadSeenCommit(state.LastBlockHeight)

    // Convert to vote set and VERIFY SIGNATURES
    seenCommit, err := cs.votesFromSeenCommit(state, commit)

    // Require +2/3 majority
    if !seenCommit.HasTwoThirdsMajority() {
        panic("commit does not have +2/3 majority")
    }
}
```

The `votesFromSeenCommit` function calls `Commit.ToVoteSet()` which **cryptographically validates each signature** against the validator's public key.

### What We Tried

| Approach | Result |
|----------|--------|
| Set `LastBlockHeight = snapshotHeight - 1` | "app block height is higher than core" - ABCI handshake fails |
| Create commit with `BlockIDFlagAbsent` | "commit does not have +2/3 majority" - no voting power |
| Create commit with `BlockIDFlagCommit` + placeholder signature | "invalid signature" - ED25519 validation fails |

### The Fundamental Problem

**CometBFT requires a cryptographically valid commit (signed by validators) to start at any non-genesis height.**

Our snapshot format includes:
- ✅ Accumulate application state
- ✅ Validator set with public keys
- ✅ Consensus parameters
- ✅ Block height and time
- ❌ **Commit signatures** (the actual ED25519 signatures from validators)

---

## 5. Why We Don't Have the Blockstore Data

### We DID Have It

The original validator backup **absolutely included** the complete `blockstore.db`:

```
/media/paul/Expansion/databases/validator_backup_20251117/extracted/
├── dnn/
│   └── data/
│       ├── accumulate.db     ✅ Used for snapshot
│       ├── blockstore.db     ❌ NOT included in snapshot format
│       ├── state.db          ❌ NOT included in snapshot format
│       └── ...
└── bvnn/
    └── data/
        ├── accumulate.db     ✅ Used for snapshot
        ├── blockstore.db     ❌ NOT included in snapshot format
        └── ...
```

### Why It Wasn't Included

1. **Snapshot Format Design**: The Accumulate snapshot format was designed to capture the **application state** (accounts, transactions, balances), not the **consensus state** (blocks, commits, votes).

2. **Historical Context**: Snapshots were originally intended for:
   - Database migration between versions
   - State verification
   - Disaster recovery to genesis
   - NOT for bootstrapping at arbitrary heights

3. **Size Considerations**: Including the full blockstore would dramatically increase snapshot size (potentially 100s of GB for full history).

4. **The Oversight**: When designing the snapshot consensus section, only the minimal data needed for genesis was included (validators, params, initial height). The **commit** for the snapshot height was not considered necessary because typical restore scenarios started from height 1.

### Current State of the Backup Drive

The external drive (`/media/paul/Expansion`) containing the original validator backup with `blockstore.db` is **not currently mounted**:

```bash
$ ls -la /media/paul/
No /media/paul
```

If mounted, we could potentially:
1. Copy `blockstore.db` directly to the follower
2. Extract the commit at height 10,641,161 and use it during restore

---

## 6. Why Syncing from Block 1 Doesn't Work

### The Scale of the Problem

| Metric | Value |
|--------|-------|
| Current MainNet Height | ~10.6 million blocks |
| Average Block Time | ~0.5 seconds |
| Blocks to Sync | 10,641,161 |
| Estimated Sync Time | **2-4 weeks** (optimistic) |

### Technical Challenges

1. **Block Download**: Each block must be downloaded from peers
2. **Signature Verification**: Each block's commit must be verified
3. **State Execution**: Each transaction must be re-executed
4. **Database Writes**: Millions of database operations
5. **Network Bandwidth**: Sustained high bandwidth for weeks

### Business Impact

- **Time to Production**: Weeks instead of hours
- **Operational Cost**: Extended compute resources
- **Risk Window**: Extended period where follower is not operational
- **Data Freshness**: By the time sync completes, node is still behind

### The Irony

We have a **complete, verified snapshot** of the exact state at height 10.6M, but we can't use it because we're missing ~500 bytes of signature data that exists in a database file we have (but didn't include in the portable snapshot).

---

## 7. Solutions

### Immediate Workaround (If Backup Drive Available)

1. Mount the external drive
2. Copy `blockstore.db` from validator backup to follower:
   ```bash
   cp -r /media/paul/Expansion/databases/validator_backup_20251117/extracted/dnn/data/blockstore.db \
         /mnt/secondary/follower-nov17-new/dnn/data/
   ```
3. The existing `blockstore.db` has the commit at height 10,641,161

### Permanent Fix (Snapshot Format Enhancement)

Modify the snapshot format to include the commit:

1. **Update Schema** (`pkg/types/cometbft/schema.yml`):
   ```yaml
   GenesisDoc:
     class: composite
     fields:
       - name: ChainID
         type: string
       - name: Params
         type: '*ConsensusParams'
       - name: Validators
         type: '[]*Validator'
       - name: Block
         type: '*Block'
       - name: Commit          # NEW FIELD
         type: bytes           # Serialized CometBFT Commit
   ```

2. **Update `create-snap`**: Extract commit from blockstore during snapshot creation

3. **Update `restore-snapshot`**: Use real commit when initializing blockstore

### Alternative: CometBFT State Sync

Enable CometBFT's built-in state sync feature:
- Requires peers that support state sync
- Fetches commit from network peers
- More complex configuration

---

## 8. Lessons Learned

1. **Snapshots Must Be Complete**: Any data required to start a node must be in the snapshot
2. **Test the Full Workflow**: Create snapshot → Deploy → Start → Verify
3. **Document Dependencies**: CometBFT's commit requirement was not documented
4. **Preserve Raw Backups**: Keep original database files accessible
5. **Version the Format**: Snapshot format changes need versioning and migration

---

## 9. Current Code Changes

The following changes have been made to `internal/node/daemon/snapshots.go`:

1. ✅ Use snapshot height for `InitialHeight` (not hardcoded 1)
2. ✅ Set `LastBlockHeight` to match app height
3. ✅ Initialize `Validators` and `LastValidators` with proposer
4. ✅ Set `LastBlockID` to match commit BlockID
5. ✅ Create seen commit in blockstore (currently with placeholder signatures)

These changes are correct but incomplete - they need the real commit data to work.

---

## 10. Next Steps

1. **Mount external drive** and verify blockstore.db is accessible
2. **Either**:
   - Copy blockstore directly (quick workaround)
   - Implement commit extraction in snapshot format (permanent fix)
3. **Test complete workflow** with real commit data
4. **Document the process** for future deployments

---

*Document created: November 30, 2025*
*Last updated: November 30, 2025*
