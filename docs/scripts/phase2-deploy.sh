#!/bin/bash

# Phase 2 - Create node directory structure and copy files
set -e

ARTIFACTS_DIR="/tmp/cyclops/artifacts"
NODE_DIR="/tmp/cyclops/node"

echo "=== Phase 2: Deploy Node Structure ==="

# Create node directory structure (CometBFT expects data directory in working dir)
mkdir -p "$NODE_DIR/config"
mkdir -p "$NODE_DIR/data"
mkdir -p "$NODE_DIR/dn/config"
mkdir -p "$NODE_DIR/dn/data"
mkdir -p "$NODE_DIR/bvn-cyclops/config"
mkdir -p "$NODE_DIR/bvn-cyclops/data"

# Copy configuration files
cp "$ARTIFACTS_DIR/accumulate.toml" "$NODE_DIR/config/"
cp "$ARTIFACTS_DIR/config.toml" "$NODE_DIR/config/"
cp "$ARTIFACTS_DIR/tendermint.toml" "$NODE_DIR/config/tendermint.toml"
cp "$ARTIFACTS_DIR/node_key.json" "$NODE_DIR/config/"
cp "$ARTIFACTS_DIR/priv_validator_key.json" "$NODE_DIR/config/"

# Copy P2P node key to partition config directories
cp "$ARTIFACTS_DIR/node_key.json" "$NODE_DIR/dn/config/"
cp "$ARTIFACTS_DIR/node_key.json" "$NODE_DIR/bvn-cyclops/config/"

# Copy validator keys with proper permissions
cp "$ARTIFACTS_DIR/priv_validator_key.json" "$NODE_DIR/dn/config/"
cp "$ARTIFACTS_DIR/priv_validator_key.json" "$NODE_DIR/bvn-cyclops/config/"

# Set proper permissions on all key files
chmod 600 "$NODE_DIR/config/node_key.json"
chmod 600 "$NODE_DIR/config/priv_validator_key.json"
chmod 600 "$NODE_DIR/dn/config/priv_validator_key.json"
chmod 600 "$NODE_DIR/bvn-cyclops/config/priv_validator_key.json"
# Set permissions on P2P node key in partitions
chmod 600 "$NODE_DIR/dn/config/node_key.json"
chmod 600 "$NODE_DIR/bvn-cyclops/config/node_key.json"

# Copy partition snapshots
cp "$ARTIFACTS_DIR/Directory-partition.snap" "$NODE_DIR/dn/data/" 2>/dev/null || true
cp "$ARTIFACTS_DIR/bvn-cyclops-partition.snap" "$NODE_DIR/bvn-cyclops/data/" 2>/dev/null || true

# Copy configuration to partition config directories
cp "$ARTIFACTS_DIR/accumulate.toml" "$NODE_DIR/dn/config/"
cp "$ARTIFACTS_DIR/tendermint.toml" "$NODE_DIR/dn/config/tendermint.toml"
cp "$ARTIFACTS_DIR/accumulate.toml" "$NODE_DIR/bvn-cyclops/config/"
cp "$ARTIFACTS_DIR/tendermint.toml" "$NODE_DIR/bvn-cyclops/config/tendermint.toml"

# Initialize databases from snapshots
# 🔄 Restore Directory partition snapshot
"$ARTIFACTS_DIR/accumulated" restore-snapshot "$ARTIFACTS_DIR/Directory-partition.snap" --work-dir "$NODE_DIR/dn"
# 🔄 Restore BVN partition snapshot
"$ARTIFACTS_DIR/accumulated" restore-snapshot "$ARTIFACTS_DIR/bvn-cyclops-partition.snap" --work-dir "$NODE_DIR/bvn-cyclops"

# Copy partition snapshots as genesis files and update CometBFT config for snapshots
# Directory partition
cp "$ARTIFACTS_DIR/Directory-partition.snap" "$NODE_DIR/config/Directory-partition.snap"
sed -i 's|^genesis_file *=.*|genesis_file = "config/Directory-partition.snap"|' "$NODE_DIR/config/tendermint.toml"

# BVN partition
cp "$ARTIFACTS_DIR/bvn-cyclops-partition.snap" "$NODE_DIR/config/bvn-cyclops-partition.snap"
sed -i 's|^genesis_file *=.*|genesis_file = "config/bvn-cyclops-partition.snap"|' "$NODE_DIR/config/tendermint.toml"

# Create CometBFT validator state file
cat > "$NODE_DIR/data/priv_validator_state.json" << 'EOF'
{
  "height": "0",
  "round": 0,
  "step": 0
}
EOF

# Node will initialize databases from snapshots on startup

echo "Done. Node structure created:"
find "$NODE_DIR" -type f | sort
