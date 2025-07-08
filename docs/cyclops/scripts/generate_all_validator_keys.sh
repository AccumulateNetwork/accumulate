#!/bin/bash
# Step 1: Generate validator key files for all ADIs in cyclops-network.json
# For now, only generate keys for Directory (dn) and the first block validator network (bvn0)
# To extend for more BVNs, update the script and network config parsing accordingly.

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

  # Directory Network key
  echo "Generating DN key for $adi -> $OUTPUT_DIR/priv_validator_key_${adi_name}_dn.json"
  $ANALYZE_BIN gen-key "$adi" "$OUTPUT_DIR/tmp_dn_$adi_name"
  mv "$OUTPUT_DIR/tmp_dn_$adi_name/priv_validator_key.json" "$OUTPUT_DIR/priv_validator_key_${adi_name}_dn.json"
  rm -rf "$OUTPUT_DIR/tmp_dn_$adi_name"

  # Block Validator Network 0 key
  echo "Generating BVN0 key for $adi -> $OUTPUT_DIR/priv_validator_key_${adi_name}_bvn0.json"
  $ANALYZE_BIN gen-key "$adi" "$OUTPUT_DIR/tmp_bvn0_$adi_name"
  mv "$OUTPUT_DIR/tmp_bvn0_$adi_name/priv_validator_key.json" "$OUTPUT_DIR/priv_validator_key_${adi_name}_bvn0.json"
  rm -rf "$OUTPUT_DIR/tmp_bvn0_$adi_name"
done

echo "All validator keys for DN and BVN0 generated."
