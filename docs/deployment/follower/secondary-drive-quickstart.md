# Secondary Drive Deployment Quick Start

Deploy an Accumulate mainnet follower node from snapshots on a secondary drive.

## Prerequisites

- Snapshots on secondary drive (e.g., `/media/paul/Expansion/snapshots/2025-12-01/`)
- Deployment directory on secondary drive (e.g., `/media/paul/Expansion/accumulate-mainnet/`)
- Built `accumulated` binary

## Step 1: Set Paths

```bash
# Adjust these paths to match your setup
export SNAP_DIR=/media/paul/Expansion/snapshots/2025-12-01
export DEPLOY_DIR=/media/paul/Expansion/accumulate-mainnet
export ACCUM_BIN=/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/accumulated
```

## Step 2: Validate Snapshots

```bash
$ACCUM_BIN validate-snapshot $SNAP_DIR/directory.snap
$ACCUM_BIN validate-snapshot $SNAP_DIR/cyclops.snap
```

## Step 3: Restore Snapshots

```bash
# Restore Directory Network
$ACCUM_BIN --work-dir $DEPLOY_DIR/dnn restore-genesis $SNAP_DIR/directory.snap

# Restore Block Validator Network
$ACCUM_BIN --work-dir $DEPLOY_DIR/bvnn restore-genesis $SNAP_DIR/cyclops.snap
```

## Step 4: Configure CometBFT Peers

```bash
# Add DN persistent peer
sed -i 's/persistent_peers = ""/persistent_peers = "3029240e829e58e399bc7b6115bb6bc947cc24c7@apollo-mainnet.accumulate.defidevs.io:16591"/' \
  $DEPLOY_DIR/dnn/config/tendermint.toml

# Add BVN persistent peer
sed -i 's/persistent_peers = ""/persistent_peers = "3029240e829e58e399bc7b6115bb6bc947cc24c7@apollo-mainnet.accumulate.defidevs.io:16691"/' \
  $DEPLOY_DIR/bvnn/config/tendermint.toml
```

## Step 5: Launch Node

```bash
$ACCUM_BIN run-dual $DEPLOY_DIR/dnn $DEPLOY_DIR/bvnn --truncate > /tmp/follower.log 2>&1 &
```

## Step 6: Monitor

```bash
# View logs
tail -f /tmp/follower.log

# Check DN sync status
curl -s http://localhost:16592/status | jq '.result.sync_info'

# Check BVN sync status
curl -s http://localhost:16692/status | jq '.result.sync_info'

# Check peer connections
curl -s http://localhost:16592/net_info | jq '.result.n_peers'
curl -s http://localhost:16692/net_info | jq '.result.n_peers'
```

## Complete Script

Copy and run this script after adjusting paths:

```bash
#!/bin/bash
set -e

# Configuration - ADJUST THESE
SNAP_DIR=/media/paul/Expansion/snapshots
DEPLOY_DIR=/media/paul/Expansion/accumulate-mainnet
ACCUM_BIN=/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/accumulated

# Validate
echo "Validating snapshots..."
$ACCUM_BIN validate-snapshot $SNAP_DIR/directory.snap
$ACCUM_BIN validate-snapshot $SNAP_DIR/cyclops.snap

# Restore
echo "Restoring DN..."
$ACCUM_BIN --work-dir $DEPLOY_DIR/dnn restore-genesis $SNAP_DIR/directory.snap

echo "Restoring BVN..."
$ACCUM_BIN --work-dir $DEPLOY_DIR/bvnn restore-genesis $SNAP_DIR/cyclops.snap

# Configure peers
echo "Configuring peers..."
sed -i 's/persistent_peers = ""/persistent_peers = "3029240e829e58e399bc7b6115bb6bc947cc24c7@apollo-mainnet.accumulate.defidevs.io:16591"/' \
  $DEPLOY_DIR/dnn/config/tendermint.toml

sed -i 's/persistent_peers = ""/persistent_peers = "3029240e829e58e399bc7b6115bb6bc947cc24c7@apollo-mainnet.accumulate.defidevs.io:16691"/' \
  $DEPLOY_DIR/bvnn/config/tendermint.toml

# Launch
echo "Launching node..."
$ACCUM_BIN run-dual $DEPLOY_DIR/dnn $DEPLOY_DIR/bvnn --truncate > /tmp/follower.log 2>&1 &

echo "Node started. Monitor with: tail -f /tmp/follower.log"
```

## Port Reference

| Service | DN Port | BVN Port |
|---------|---------|----------|
| CometBFT P2P | 16591 | 16691 |
| CometBFT RPC | 16592 | 16692 |
| libp2p | 16593 | 16693 |
| Accumulate API | 16595 | 16695 |

## See Also

- [Full Deployment Guide](snapshot-deployment-guide.md) - Comprehensive guide including snapshot creation
