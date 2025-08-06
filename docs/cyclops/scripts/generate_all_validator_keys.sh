#!/bin/bash
# Step 1: Generate a single validator key file for cyclops-network.json
# This single key will be used by both Directory Network (DN) and Block Validator Network (BVN)
# Simplifies key management and eliminates confusion about which key to use.

set -euo pipefail

# NOTE: Run this script from the directory containing both 'analyze' and 'cyclops-network.json'.
NETWORK_JSON="cyclops-network.json"
OUTPUT_DIR="."
ANALYZE_BIN="./analyze"

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required but not installed. Aborting." >&2
  exit 1
fi

if [ ! -f "$NETWORK_JSON" ]; then
  echo "Network config $NETWORK_JSON not found! Run from the directory containing it." >&2
  exit 1
fi

ADIS=$(jq -r '.globals.network.validators[].operator' "$NETWORK_JSON")

for adi in $ADIS; do
  adi_name=$(echo "$adi" | sed 's|acc://||; s|/|-|g; s|\.|-|g')

  # Single validator key for both DN and BVN
  echo "Generating single validator key for $adi -> $OUTPUT_DIR/priv_validator_key.json"
  $ANALYZE_BIN gen-key "$adi" "$OUTPUT_DIR/tmp_$adi_name"
  mv "$OUTPUT_DIR/tmp_$adi_name/priv_validator_key.json" "$OUTPUT_DIR/priv_validator_key.json"
  rm -rf "$OUTPUT_DIR/tmp_$adi_name"
  
  echo "Single validator key generated for both DN and BVN networks."
  break  # Only generate one key for the first ADI (since we're using single key architecture)
done

echo "Single validator key generated and ready for use by both DN and BVN."
