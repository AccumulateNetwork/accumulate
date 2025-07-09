#!/bin/bash

# Phase 1 - Generate partition snapshots and config files
set -e

ARTIFACTS_DIR="/tmp/cyclops/artifacts"

echo "=== Phase 1: Generate Snapshots ==="

# Work entirely within /tmp/cyclops/artifacts (populated by Phase 0)
cd "$ARTIFACTS_DIR"

# Split the universal snapshot into partition-specific snapshots with embedded metadata
echo "Splitting universal snapshot..."
./analyze extract cyclops-genesis.snap cyclops-network.json Directory.toml bvn-cyclops.toml --partition-snapshots "$ARTIFACTS_DIR"

# Skipping CometBFT genesis JSON generation (using snapshots)
# echo "Generating CometBFT genesis JSON..."
# ./accumulated --work-dir "$ARTIFACTS_DIR" init genesis cyclops-network.json
# echo "Genesis JSON files generated:"
# ls -1 *.json

echo "Done. Generated artifacts:"
ls -la *.snap *.toml

echo "=== Phase 1 Complete ==="
