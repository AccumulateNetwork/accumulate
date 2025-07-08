#!/bin/bash

# Script to test snapshot restoration with BPT check skipping
# This will restore a snapshot and start the node

# Set variables
SNAPSHOT_PATH=$1
CONFIG_DIR=$2

if [ -z "$SNAPSHOT_PATH" ] || [ -z "$CONFIG_DIR" ]; then
  echo "Usage: $0 <snapshot-path> <config-dir>"
  echo "Example: $0 /path/to/snapshot.snap /path/to/config"
  exit 1
fi

# Check if snapshot file exists
if [ ! -f "$SNAPSHOT_PATH" ]; then
  echo "Error: Snapshot file not found: $SNAPSHOT_PATH"
  exit 1
fi

# Check if config directory exists
if [ ! -d "$CONFIG_DIR" ]; then
  echo "Error: Config directory not found: $CONFIG_DIR"
  exit 1
fi

echo "=== Testing Snapshot Restoration ==="
echo "Snapshot: $SNAPSHOT_PATH"
echo "Config: $CONFIG_DIR"

# Restore the snapshot
echo -e "\n=== Restoring Snapshot ==="
./accumulated restore-snapshot "$SNAPSHOT_PATH"
RESTORE_RESULT=$?

if [ $RESTORE_RESULT -ne 0 ]; then
  echo "Error: Failed to restore snapshot"
  exit 1
fi

echo "Snapshot restored successfully!"

# Start the node
echo -e "\n=== Starting Node ==="
./accumulated run --config "$CONFIG_DIR"

# Note: The script will not reach here unless the node is stopped
echo "Node stopped"
