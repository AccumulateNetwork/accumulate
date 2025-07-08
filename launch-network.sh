#!/bin/bash

# Script to launch Accumulate network with the modified code
# This will start the node with the BPT check skipping enabled

# Set variables
CONFIG_DIR=$1

if [ -z "$CONFIG_DIR" ]; then
  echo "Usage: $0 <config-dir>"
  echo "Example: $0 /path/to/config"
  exit 1
fi

# Check if config directory exists
if [ ! -d "$CONFIG_DIR" ]; then
  echo "Error: Config directory not found: $CONFIG_DIR"
  exit 1
fi

echo "=== Launching Accumulate Network ==="
echo "Config: $CONFIG_DIR"

# Start the node
echo -e "\n=== Starting Node ==="
./accumulated run --config "$CONFIG_DIR"

# Note: The script will not reach here unless the node is stopped
echo "Node stopped"
