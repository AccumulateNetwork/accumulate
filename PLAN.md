# Plan: Deploy Follower Using Repaired Dec 1 Snapshots

## Overview

Deploy a local Accumulate follower node using the repaired Dec 1 2025 snapshots. The follower will sync with the mainnet by connecting to existing validator peers.

## Important Notes

### Snapshots Are Known Good
We have repeatedly deployed followers from the Dec 1 2025 snapshots. If the snapshots validate, they are good and will support deploying a follower. There is no need to question the snapshot integrity.

### Goal: Build Tools, Not AI Solutions
The objective is to **BUILD TOOLS** that deploy followers. We are creating reusable command-line tools (`deploy-follower`, `follower-monitor`, etc.) that anyone can use. This is not about finding a one-off AI-assisted solution.

### Critical: Route All Output to Log Files
**All command output MUST be redirected to log files.** Long-running commands and verbose output will crash the AI context. Always use:
```bash
command > /tmp/output.log 2>&1
```
Check results with `tail -100 /tmp/output.log` rather than streaming output directly.

## Prerequisites

### Repaired Snapshots (Already Done)
- `/media/paul/Expansion/snapshots/2025-12-01-repaired/directory.snap` - DN snapshot (8.5 GB)
- `/media/paul/Expansion/snapshots/2025-12-01-repaired/cyclops.snap` - BVN-Cyclops snapshot (15.7 GB)

### Required Binaries
- `accumulated` - The main Accumulate daemon (already built at `./accumulated`)
- `deploy-follower` - Deployment automation tool (at `./tools/deploy-follower/deploy-follower`)
- `follower-monitor` (optional) - Web UI for monitoring (at `./tools/follower-monitor/follower-monitor`)

## Deployment Steps

### Step 1: Build Required Tools
```bash
# Build deploy-follower if not already built
go build -o deploy-follower ./tools/deploy-follower/

# Build follower-monitor (optional)
cd tools/follower-monitor && go build -o follower-monitor . && cd ../..
```

### Step 2: Create Deployment Directory Structure
The deploy-follower tool will create:
```
/home/paul/accumulate-follower/
├── accumulated              # Binary copy
├── follower-monitor         # Binary copy (optional)
├── start.sh                 # Startup script
├── stop.sh                  # Shutdown script
├── start-monitor.sh         # Monitor startup (if monitor provided)
├── follower.pid             # PID file (created at runtime)
├── logs/
│   ├── follower.log         # Main follower log
│   ├── monitor.log          # Monitor log
│   ├── restore-genesis-dn.log
│   └── restore-genesis-bvn.log
├── dnn/                     # Directory Network node
│   ├── config/
│   │   ├── accumulate.toml  # Accumulate config
│   │   ├── tendermint.toml  # CometBFT P2P config
│   │   ├── genesis.json     # Chain genesis
│   │   ├── node_key.json    # P2P identity
│   │   └── priv_validator_key.json
│   └── data/
│       ├── accumulate.db/   # Accumulate state DB
│       ├── state.db/        # CometBFT state
│       ├── blockstore.db/   # CometBFT blocks
│       └── priv_validator_state.json
└── bvnn/                    # Block Validator Network node
    ├── config/              # Same structure as dnn
    └── data/
```

### Step 3: Run Deployment
```bash
./deploy-follower \
  --work-dir /home/paul/accumulate-follower \
  --dn-snapshot /media/paul/Expansion/snapshots/2025-12-01-repaired/directory.snap \
  --bvn-snapshot /media/paul/Expansion/snapshots/2025-12-01-repaired/cyclops.snap \
  --accumulated ./accumulated \
  --monitor ./tools/follower-monitor/follower-monitor \
  --network mainnet \
  --bvn Cyclops
```

This command will:
1. **Create directory structure** - dnn/, bvnn/, logs/
2. **Copy binaries** - accumulated and follower-monitor to work directory
3. **Run restore-genesis for DN** - Extracts databases from directory.snap
4. **Run restore-genesis for BVN** - Extracts databases from cyclops.snap
5. **Configure follower mode** - Updates accumulate.toml with API listen addresses
6. **Configure peers** - Sets persistent_peers in tendermint.toml
7. **Generate node keys** - Creates unique identity for this follower
8. **Create scripts** - start.sh, stop.sh, start-monitor.sh

### Step 4: Configuration Details

#### Network Ports (Default)
| Service | Port | Description |
|---------|------|-------------|
| DN RPC | 16592 | CometBFT RPC for DN |
| DN P2P | 16591 | CometBFT P2P for DN |
| DN API | 16595 | Accumulate API for DN |
| BVN RPC | 16692 | CometBFT RPC for BVN |
| BVN P2P | 16691 | CometBFT P2P for BVN |
| BVN API | 16695 | Accumulate API for BVN |
| Monitor | 8080 | Web UI |

#### Peer Configuration
The deploy-follower tool configures these default mainnet peers:
- DN: `3029240e829e58e399bc7b6115bb6bc947cc24c7@apollo-mainnet.accumulate.defidevs.io:16591`
- BVN: `3029240e829e58e399bc7b6115bb6bc947cc24c7@apollo-mainnet.accumulate.defidevs.io:16691`

### Step 5: Start the Follower
```bash
cd /home/paul/accumulate-follower
./start.sh
```

Or with the --start flag during deployment:
```bash
./deploy-follower ... --start
```

### Step 6: Monitor Sync Progress
```bash
# Check DN status
curl -s http://localhost:16592/status | jq '.result.sync_info'

# Check BVN status
curl -s http://localhost:16692/status | jq '.result.sync_info'

# Or use follower-monitor web UI
./start-monitor.sh
# Open http://localhost:8080
```

Key fields to watch:
- `latest_block_height` - Current synced height
- `catching_up` - true while syncing, false when caught up
- `latest_block_time` - Timestamp of latest block

### Step 7: Check Status and Stop
```bash
# Check status
./deploy-follower --work-dir /home/paul/accumulate-follower --status

# Stop follower
./stop.sh
# Or: ./deploy-follower --work-dir /home/paul/accumulate-follower --stop
```

## What Gets Extracted from Snapshots

The restore-genesis process extracts:

1. **accumulate.db** - The Accumulate application state (accounts, transactions, etc.)
2. **state.db** - CometBFT consensus state (validators, height, etc.)
3. **blockstore.db** - CometBFT block history
4. **genesis.json** - Created from snapshot header with:
   - Chain ID (MainNet.Directory or MainNet.Cyclops)
   - Initial height from snapshot
   - App hash (root hash of state)
   - Validators
   - Consensus parameters

## Expected Sync Behavior

1. **Initial State**: Follower starts at snapshot height (~11.75M blocks)
2. **Catch-up**: Downloads missing blocks from peers since Dec 1
3. **Sync Time**: Depends on network speed and blocks to catch up
4. **Steady State**: Once caught up, processes new blocks as they're produced

## Troubleshooting

### Follower Won't Start
```bash
# Check logs
tail -100 /home/paul/accumulate-follower/logs/follower.log

# Common issues:
# - Port already in use: Change ports in accumulate.toml/tendermint.toml
# - Peer connection failed: Check firewall, try alternate peers
# - Database corruption: Re-run restore-genesis
```

### Sync Stalled
```bash
# Check peer connections
curl -s http://localhost:16592/net_info | jq '.result.n_peers'

# Add more peers if needed by editing:
# dnn/config/tendermint.toml - persistent_peers
# bvnn/config/tendermint.toml - persistent_peers
```

### Database Issues
```bash
# If Badger database errors occur:
# The restore-genesis command automatically repairs MANIFEST files
# For persistent issues, re-extract from snapshot
```

## Execution Log

This section tracks each execution attempt, errors encountered, and fixes applied.

### Attempt 1
**Status**: IN PROGRESS
**Date**: 2025-12-06

**Tool Fixes Applied**:
1. **Skip stale state.db/blockstore.db extraction** - The archived CometBFT databases in snapshots are from height 1-2, not the current snapshot height. Skipping extraction lets CometBFT create fresh state.
2. **Fix BVN partition-id** - Changed from "BVN.Cyclops" to "Cyclops" so URLs are constructed correctly as `bvn-cyclops.acme` instead of `bvn-BVN.Cyclops.acme`
3. **Add CometBFT port configuration** - DN: P2P=16591, RPC=16592; BVN: P2P=16691, RPC=16692
4. **Disable Prometheus for BVN** - Chain ID contains dots which create invalid metric names

**Current Status**:
- Follower starts and both nodes respond on their RPC ports ✓
- Connects to mainnet peers ✓
- Block sync fails with: `wrong Block.Header.LastBlockID. Expected :0:000000000000, got <actual_hash>`

**Root Cause**:
CometBFT block sync requires prior block history to validate new blocks. Our snapshots contain:
- Accumulate app state at height 11,750,072 ✓
- Genesis with initial_height = 11,750,073 ✓
- **Missing**: Proper CometBFT state.db/blockstore.db at matching height ✗

The archived state.db in snapshots was created at genesis (height 1-2), not at the snapshot height.

**Options to Resolve**:
1. **State Sync**: Configure CometBFT state sync to fetch recent state from mainnet RPC servers
2. **Create proper snapshots**: Include CometBFT state captured at snapshot time
3. **Manual state creation**: Create synthetic CometBFT state matching genesis initial_height

**Next Steps**: Implement state sync configuration in deploy-follower

---

## Iterative Execution Process

The plan will be executed iteratively until it works end-to-end:

1. **Execute** - Run the deployment steps
2. **Log Errors** - Record any errors in the Execution Log section
3. **Fix Issues** - Modify code/configuration to resolve errors
4. **Reset** - Clean up failed deployment (`rm -rf /home/paul/accumulate-follower`)
5. **Repeat** - Run again until successful

### Success Criteria
- [ ] deploy-follower tool builds without errors
- [ ] restore-genesis extracts DN snapshot successfully
- [ ] restore-genesis extracts BVN snapshot successfully
- [ ] Configuration files are created correctly
- [ ] Follower starts without crashing
- [ ] DN connects to peers and begins syncing
- [ ] BVN connects to peers and begins syncing
- [ ] RPC endpoints respond to status queries

---

## Summary Commands

```bash
# Full deployment and start
./deploy-follower \
  --work-dir /home/paul/accumulate-follower \
  --dn-snapshot /media/paul/Expansion/snapshots/2025-12-01-repaired/directory.snap \
  --bvn-snapshot /media/paul/Expansion/snapshots/2025-12-01-repaired/cyclops.snap \
  --accumulated ./accumulated \
  --monitor ./tools/follower-monitor/follower-monitor \
  --start

# Check status
curl -s http://localhost:16592/status | jq '.result.sync_info.latest_block_height'
curl -s http://localhost:16692/status | jq '.result.sync_info.latest_block_height'

# Stop
cd /home/paul/accumulate-follower && ./stop.sh
```
