# Follower Node Deployment Guide

Complete guide for deploying an Accumulate follower node from validator snapshots.

## Overview

A **follower node** is a non-validating node that syncs blockchain state and provides API access without participating in consensus.

| Feature | Follower | Validator |
|---------|----------|-----------|
| Syncs blockchain | Yes | Yes |
| API access | Yes | Yes |
| Votes on blocks | No | Yes |
| Requires staking | No | Yes |

## Repository Structure

All paths in this document are relative to the repository root:
```
/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/
```

### Key Components

| Component | Path | Description |
|-----------|------|-------------|
| Main binary | `cmd/accumulated/` | Accumulate node daemon |
| Snapshot command | `cmd/accumulated/cmd_snapshot.go` | validate-snapshot, restore-genesis commands |
| Snapshot creation tool | `cmd/create-snap/main.go` | Creates portable snapshots from node backup |
| Snapshot restore logic | `internal/node/daemon/snapshots.go` | Core restore implementation |
| Snapshot format | `pkg/database/snapshot/` | Snapshot file format and reader/writer |
| Follower monitor | `tools/follower-monitor/main.go` | Web UI for monitoring sync status |
| Default config | `internal/node/config/` | Node configuration templates |

## System Requirements

### Hardware
- **CPU**: 4+ cores
- **RAM**: 16GB minimum, 32GB recommended
- **Disk**: 500GB+ SSD (NVMe recommended)
  - DN database: ~100GB
  - BVN database: ~150GB per BVN
  - Growth: ~10-20GB/month

### Software
- Go 1.21+ (for building)
- Linux (Ubuntu 22.04 recommended)

### Network
- Outbound TCP to bootstrap server (ports 16591-16693)

---

## Phase 1: Build Binaries

```bash
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate

# Build main binary
go build -o accumulated ./cmd/accumulated

# Build snapshot creation tool
go build -o create-snap ./cmd/create-snap

# Build follower monitor (optional)
go build -o follower-monitor ./tools/follower-monitor

# Verify
./accumulated version
./create-snap -help
```

**Output binaries:**
- `accumulated` - Main node binary
- `create-snap` - Snapshot creation tool
- `follower-monitor` - Web-based sync monitor

---

## Phase 2: Create Snapshots from Validator Backup

### Prerequisites

A complete validator node backup containing:
```
$BACKUP_DIR/
├── dnn/data/
│   ├── accumulate.db/    # Accumulate protocol state
│   ├── blockstore.db/    # CometBFT blocks with commit signatures
│   └── state.db/         # CometBFT consensus state
└── bvnn/data/
    ├── accumulate.db/
    ├── blockstore.db/
    └── state.db/
```

### Determine Database Type

Check the source database format:
- **BadgerDB**: Contains `.vlog` and `.sst` files (older AWS production nodes)
- **LevelDB**: Contains `.ldb` files (newer installations)

```bash
ls $BACKUP_DIR/dnn/data/accumulate.db/
# BadgerDB: 000001.vlog, MANIFEST, etc.
# LevelDB: 000001.ldb, MANIFEST-000001, etc.
```

### Create DN Snapshot

```bash
export BACKUP_DIR=/media/paul/Expansion/databases/2025-12-01-aws-validator-node
export SNAPSHOT_DIR=/tmp/snapshots
mkdir -p $SNAPSHOT_DIR

./create-snap \
  -db $BACKUP_DIR/dnn/data/accumulate.db \
  -blockstore $BACKUP_DIR/dnn/data/blockstore.db \
  -statedb $BACKUP_DIR/dnn/data/state.db \
  -output $SNAPSHOT_DIR/directory.snap \
  -partition Directory \
  -type badger \
  -genesis
```

### Create BVN Snapshot

BVN snapshots require the DN database to read the NetworkDefinition for validator info:

```bash
./create-snap \
  -db $BACKUP_DIR/bvnn/data/accumulate.db \
  -blockstore $BACKUP_DIR/bvnn/data/blockstore.db \
  -statedb $BACKUP_DIR/bvnn/data/state.db \
  -dn-db $BACKUP_DIR/dnn/data/accumulate.db \
  -output $SNAPSHOT_DIR/cyclops.snap \
  -partition Cyclops \
  -type badger \
  -genesis
```

### Create-snap Flags Reference

| Flag | Required | Description |
|------|----------|-------------|
| `-db` | Yes | Path to accumulate.db directory |
| `-blockstore` | Yes | Path to blockstore.db directory |
| `-statedb` | Yes | Path to state.db directory |
| `-output` | Yes | Output .snap file path |
| `-partition` | Yes | Partition name: Directory, Apollo, Yutu, Cyclops |
| `-type` | Yes | Database type: `badger` or `leveldb` |
| `-dn-db` | BVN only | Path to DN accumulate.db (for NetworkDefinition) |
| `-genesis` | No | Skip message/transaction history (faster) |
| `-peers` | No | CometBFT persistent peers to embed in snapshot |

### Snapshot Contents

The snapshot file includes:
- **SectionTypeConsensus**: Validators, commit signatures, LastBlockID, LastResultsHash
- **SectionTypeRecords**: Accumulate accounts and chain data
- **SectionTypeBPT**: Binary Patricia Tree for verification
- **SectionTypeCometStateDB**: Raw state.db archive (gzip tar)
- **SectionTypeCometBlockstoreDB**: Raw blockstore.db archive (gzip tar)
- **SectionTypeAccumulateDB**: Raw accumulate.db archive (gzip tar)

---

## Phase 3: Validate Snapshots

```bash
./accumulated validate-snapshot $SNAPSHOT_DIR/directory.snap
./accumulated validate-snapshot $SNAPSHOT_DIR/cyclops.snap
```

**Expected output:**
```
Validating snapshot: directory.snap

Version: 2
Root Hash: abcd1234...
Partition: acc://Directory.acme
Block Index: 11828256
Timestamp: 2025-12-01T12:00:00Z

Sections (7 total):
  - Records         (offset: 100, size: 52428800 bytes)
  - BPT             (offset: 52428900, size: 1048576 bytes)
  - Consensus       (offset: 53477476, size: 4096 bytes)
  - CometStateDB    (offset: 53481572, size: 1048576 bytes)
  - CometBlockstoreDB (offset: 54530148, size: 10485760 bytes)
  - AccumulateDB    (offset: 65015908, size: 104857600 bytes)

Consensus Section:
  Chain ID: MainNet.Directory
  Validators: 7
  Block Height: 11828256
  Block Time: 2025-12-01T12:00:00Z

=== VALIDATION SUMMARY ===

[OK] Snapshot is valid and can be restored
```

**Validation checks performed** (see `cmd/accumulated/cmd_snapshot.go:78-192`):
- Snapshot version (must be v2)
- Root hash present
- System ledger info present
- Consensus section with validators
- Block data for CometBFT initialization

---

## Phase 4: Restore Snapshots

### Set Environment

```bash
export WORK_DIR=/home/paul/.accumulate/mainnet-follower
export SNAPSHOT_DIR=/tmp/snapshots
```

### Restore Directory Network

```bash
# NOTE: --work-dir is a GLOBAL flag and MUST come BEFORE the subcommand
./accumulated --work-dir $WORK_DIR/dnn restore-genesis $SNAPSHOT_DIR/directory.snap
```

### Restore BVN

```bash
./accumulated --work-dir $WORK_DIR/bvnn restore-genesis $SNAPSHOT_DIR/cyclops.snap
```

### What restore-genesis Creates

The restore process (implemented in `internal/node/daemon/snapshots.go:259-743`) creates:

```
$WORK_DIR/
├── dnn/
│   ├── config/
│   │   ├── accumulate.toml      # Accumulate config with libp2p bootstrap
│   │   ├── tendermint.toml      # CometBFT config (needs persistent_peers!)
│   │   ├── genesis.json         # CometBFT genesis with validators and AppHash
│   │   ├── node_key.json        # P2P node identity
│   │   └── priv_validator_key.json
│   └── data/
│       ├── accumulate.db/       # Restored Accumulate database
│       ├── state.db/            # CometBFT state (from archive)
│       ├── blockstore.db/       # CometBFT blocks (from archive)
│       └── priv_validator_state.json
└── bvnn/
    ├── config/
    │   └── (same structure)
    └── data/
        └── (same structure)
```

### Restore Process Details

1. **Extract consensus section** - Reads validators, commit signatures, LastBlockID
2. **Create genesis.json** - CometBFT genesis with AppHash from snapshot
3. **Initialize state.db** - Sets LastBlockHeight to snapshot height
4. **Initialize blockstore.db** - Sets base/height and saves seen commit
5. **Extract database archives** - Unpacks raw state.db, blockstore.db, accumulate.db

---

## Phase 5: Configure Peers

Accumulate uses **two separate P2P networks**:
- **libp2p**: Protocol messages and peer discovery (auto-configured)
- **CometBFT**: Block sync and consensus (requires manual configuration)

### Verify libp2p Bootstrap (auto-configured)

```bash
grep bootstrap-peers $WORK_DIR/dnn/config/accumulate.toml
```

Should show:
```toml
bootstrap-peers = [
  "/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx"
]
```

### Configure CometBFT Persistent Peers (required)

```bash
# Check current setting
grep persistent_peers $WORK_DIR/dnn/config/tendermint.toml
grep persistent_peers $WORK_DIR/bvnn/config/tendermint.toml
```

If empty, add peers:

```bash
# DN peer (port 16591)
sed -i 's/persistent_peers = ""/persistent_peers = "3029240e829e58e399bc7b6115bb6bc947cc24c7@apollo-mainnet.accumulate.defidevs.io:16591"/' \
  $WORK_DIR/dnn/config/tendermint.toml

# BVN peer (port 16691)
sed -i 's/persistent_peers = ""/persistent_peers = "3029240e829e58e399bc7b6115bb6bc947cc24c7@apollo-mainnet.accumulate.defidevs.io:16691"/' \
  $WORK_DIR/bvnn/config/tendermint.toml
```

### Bootstrap Server Reference

| Service | Host | Port | Protocol |
|---------|------|------|----------|
| DN libp2p | bootstrap.accumulate.defidevs.io | 16593 | libp2p |
| BVN libp2p | bootstrap.accumulate.defidevs.io | 16693 | libp2p |
| DN CometBFT | apollo-mainnet.accumulate.defidevs.io | 16591 | CometBFT P2P |
| BVN CometBFT | apollo-mainnet.accumulate.defidevs.io | 16691 | CometBFT P2P |

---

## Phase 6: Start the Follower

### Command Format

**CRITICAL**: The `run-dual` command requires arguments on a single line. Do NOT use backslash line continuations.

```bash
# Basic start
./accumulated run-dual $WORK_DIR/dnn $WORK_DIR/bvnn

# With truncate flag (recommended for first run after restore)
./accumulated run-dual $WORK_DIR/dnn $WORK_DIR/bvnn --truncate

# Expanded paths (same thing)
./accumulated run-dual /home/paul/.accumulate/mainnet-follower/dnn /home/paul/.accumulate/mainnet-follower/bvnn
```

### Background Execution with Logging

```bash
./accumulated run-dual $WORK_DIR/dnn $WORK_DIR/bvnn --truncate > /tmp/follower.log 2>&1 &
tail -f /tmp/follower.log
```

---

## Phase 7: Verify Sync Status

### Check Peer Connections

Wait 30+ seconds after startup, then:

```bash
# DN peer count
curl -s http://localhost:16592/net_info | jq '.result.n_peers'

# BVN peer count
curl -s http://localhost:16692/net_info | jq '.result.n_peers'
```

Expected: `n_peers > 0`

### Check CometBFT Sync Status

```bash
# DN sync info
curl -s http://localhost:16592/status | jq '.result.sync_info'

# BVN sync info
curl -s http://localhost:16692/status | jq '.result.sync_info'
```

Expected during sync:
```json
{
  "catching_up": true,
  "latest_block_height": "11828300"
}
```

### Check Accumulate API

```bash
# DN API
curl -s http://localhost:16595/v2 -d '{"jsonrpc":"2.0","id":1,"method":"status"}' | jq

# BVN API
curl -s http://localhost:16695/v2 -d '{"jsonrpc":"2.0","id":1,"method":"status"}' | jq
```

**Note**: `lastBlock: null` is normal during initial CometBFT catch-up. The `bvnHeight` field shows Accumulate-layer progress.

### Using Follower Monitor

```bash
./follower-monitor
# Opens web UI at http://localhost:8080
```

---

## Phase 8: Run as System Service

### Create Systemd Service

```bash
sudo tee /etc/systemd/system/accumulate-follower.service << 'EOF'
[Unit]
Description=Accumulate Follower Node
After=network.target

[Service]
Type=simple
User=paul
WorkingDirectory=/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate
ExecStart=/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/accumulated run-dual /home/paul/.accumulate/mainnet-follower/dnn /home/paul/.accumulate/mainnet-follower/bvnn --truncate
Restart=always
RestartSec=10
StandardOutput=append:/var/log/accumulate-follower.log
StandardError=append:/var/log/accumulate-follower.log

[Install]
WantedBy=multi-user.target
EOF
```

### Enable and Start

```bash
sudo systemctl daemon-reload
sudo systemctl enable accumulate-follower
sudo systemctl start accumulate-follower
sudo systemctl status accumulate-follower
```

### View Logs

```bash
sudo tail -f /var/log/accumulate-follower.log
```

### Configure Log Rotation

```bash
sudo tee /etc/logrotate.d/accumulate-follower << 'EOF'
/var/log/accumulate-follower.log {
    daily
    rotate 7
    compress
    delaycompress
    missingok
    notifempty
    create 0640 paul paul
}
EOF
```

---

## Port Reference

| Service | DN Port | BVN Port | Description |
|---------|---------|----------|-------------|
| CometBFT P2P | 16591 | 16691 | Block sync and gossip |
| CometBFT RPC | 16592 | 16692 | Node status queries |
| libp2p | 16593 | 16693 | Protocol messages |
| Prometheus | 26660 | 16694 | Metrics endpoint |
| Accumulate API | 16595 | 16695 | JSON-RPC queries |

---

## Troubleshooting

### "accepts 2 arg(s), received 4"

**Cause**: Using backslash line continuations with run-dual

**Solution**: Put entire command on single line:
```bash
# WRONG
./accumulated run-dual \
  $WORK_DIR/dnn \
  $WORK_DIR/bvnn

# CORRECT
./accumulated run-dual $WORK_DIR/dnn $WORK_DIR/bvnn
```

### "n_peers: 0"

**Cause**: Missing CometBFT persistent_peers

**Solution**: Add to tendermint.toml:
```toml
[p2p]
persistent_peers = "3029240e829e58e399bc7b6115bb6bc947cc24c7@apollo-mainnet.accumulate.defidevs.io:16591"
```

### "--work-dir flag not recognized"

**Cause**: Flag placed after subcommand

**Solution**: Put --work-dir BEFORE the subcommand:
```bash
# WRONG
accumulated restore-genesis snapshot.snap --work-dir /path

# CORRECT
accumulated --work-dir /path restore-genesis snapshot.snap
```

### "panic: failed to reconstruct vote set from commit"

**Cause**: Snapshot missing commit signatures (created without blockstore.db)

**Solution**: Recreate snapshot with `-blockstore` flag

### "lastBlock: null" in API

**Cause**: Normal during CometBFT catch-up

**Solution**: Wait for `catching_up: false`:
```bash
curl -s http://localhost:16692/status | jq '.result.sync_info.catching_up'
```

### "address already in use"

**Cause**: Previous process still running

**Solution**:
```bash
pkill -9 accumulated
sleep 2
./accumulated run-dual $WORK_DIR/dnn $WORK_DIR/bvnn
```

### "AppHash mismatch"

**Cause**: State inconsistency between genesis.json and database

**Solution**: Use snapshots with raw database archives (current create-snap format)

---

## Configuration Reference

### Key accumulate.toml Settings

```toml
[p2p]
# libp2p bootstrap peers (auto-configured by restore-genesis)
bootstrap-peers = [
  "/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx"
]
```

### Key tendermint.toml Settings

```toml
[p2p]
# CometBFT persistent peers (REQUIRED - must configure manually)
persistent_peers = "3029240e829e58e399bc7b6115bb6bc947cc24c7@apollo-mainnet.accumulate.defidevs.io:16591"

[rpc]
laddr = "tcp://127.0.0.1:16592"
```

---

## Sync Timeline

| Phase | Duration | Description |
|-------|----------|-------------|
| Initialization | 0-30s | Database opens, peer connections |
| CometBFT sync | 1-4 hours | Download blocks from validators |
| Full sync | Varies | Depends on snapshot age |

**Tip**: The `bvnHeight` field shows Accumulate-layer progress even when `lastBlock` is null.

---

## Code References

| File | Line | Description |
|------|------|-------------|
| `cmd/create-snap/main.go` | 37-77 | Command-line flag parsing |
| `cmd/create-snap/main.go` | 287-460 | Consensus section creation |
| `cmd/create-snap/main.go` | 468-544 | Database archive writing |
| `cmd/accumulated/cmd_snapshot.go` | 29-46 | validate-snapshot command |
| `cmd/accumulated/cmd_snapshot.go` | 48-64 | restore-genesis command |
| `cmd/accumulated/cmd_snapshot.go` | 78-192 | Snapshot validation logic |
| `cmd/accumulated/cmd_snapshot.go` | 242-334 | restore-genesis implementation |
| `internal/node/daemon/snapshots.go` | 259-743 | LoadSnapshotWithOptions |
| `internal/node/daemon/snapshots.go` | 661-705 | Raw database archive extraction |
| `pkg/database/snapshot/` | - | Snapshot format definitions |
