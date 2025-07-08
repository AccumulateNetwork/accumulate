#!/bin/bash

# Script to launch Accumulate Cyclops network with the modified code
# This will restore partition snapshots and start the node

# Set paths
REPO_DIR="/home/paulsnow/go/src/gitlab.com/AccumulateNetwork/accumulate"
NODE_DIR="/tmp/cyclops/node"
ARTIFACTS_DIR="$NODE_DIR/artifacts"
SNAPSHOTS_DIR="$NODE_DIR/partition-snapshots"
BVN_CONFIG_DIR="$ARTIFACTS_DIR/bvnn/config"

echo "=== Launching Accumulate Cyclops Network ==="

# Copy our modified accumulated binary to the artifacts directory
echo -e "\n=== Copying modified accumulated binary ==="
cp "$REPO_DIR/accumulated" "$ARTIFACTS_DIR/accumulated.new"
chmod +x "$ARTIFACTS_DIR/accumulated.new"

# Restore BVN partition snapshot
echo -e "\n=== Restoring BVN partition snapshot ==="
cd "$ARTIFACTS_DIR"
./accumulated.new restore-snapshot "$SNAPSHOTS_DIR/bvn-cyclops-partition.snap"
if [ $? -ne 0 ]; then
  echo "Error: Failed to restore BVN partition snapshot"
  exit 1
fi

# Restore Directory partition snapshot
echo -e "\n=== Restoring Directory partition snapshot ==="
./accumulated.new restore-snapshot "$SNAPSHOTS_DIR/Directory-partition.snap"
if [ $? -ne 0 ]; then
  echo "Error: Failed to restore Directory partition snapshot"
  exit 1
fi

# Start the node
echo -e "\n=== Starting Node ==="
cd "$ARTIFACTS_DIR"
./accumulated.new run --config "$BVN_CONFIG_DIR"

# Note: The script will not reach here unless the node is stopped
echo "Node stopped"
