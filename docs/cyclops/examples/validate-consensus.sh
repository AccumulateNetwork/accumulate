#!/bin/bash

# Consensus JSON Validation Script
# Validates the structure and consistency of generated consensus files

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "🔍 Validating Consensus JSON Files"
echo "=================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to print status
print_status() {
    local status=$1
    local message=$2
    if [ "$status" = "OK" ]; then
        echo -e "${GREEN}✓${NC} $message"
    elif [ "$status" = "WARN" ]; then
        echo -e "${YELLOW}⚠${NC} $message"
    else
        echo -e "${RED}✗${NC} $message"
    fi
}

# Check if files exist
echo "📁 Checking file existence..."
if [ -f "consensus_dn.json" ]; then
    print_status "OK" "Directory consensus file exists"
else
    print_status "ERROR" "Directory consensus file missing"
    exit 1
fi

if [ -f "consensus_bvn0.json" ]; then
    print_status "OK" "BVN consensus file exists"
else
    print_status "ERROR" "BVN consensus file missing"
    exit 1
fi

# Validate JSON structure
echo -e "\n📋 Validating JSON structure..."
if jq '.' consensus_dn.json > /dev/null 2>&1; then
    print_status "OK" "Directory consensus JSON is valid"
else
    print_status "ERROR" "Directory consensus JSON is invalid"
    exit 1
fi

if jq '.' consensus_bvn0.json > /dev/null 2>&1; then
    print_status "OK" "BVN consensus JSON is valid"
else
    print_status "ERROR" "BVN consensus JSON is invalid"
    exit 1
fi

# Check required fields
echo -e "\n🔧 Checking required fields..."

# Directory Network checks
DN_CHAIN_ID=$(jq -r '.chain_id' consensus_dn.json)
if [ "$DN_CHAIN_ID" = "cyclops.Directory" ]; then
    print_status "OK" "Directory chain ID correct: $DN_CHAIN_ID"
else
    print_status "ERROR" "Directory chain ID incorrect: $DN_CHAIN_ID"
fi

DN_VALIDATOR_COUNT=$(jq '.validators | length' consensus_dn.json)
if [ "$DN_VALIDATOR_COUNT" -eq 1 ]; then
    print_status "OK" "Directory has $DN_VALIDATOR_COUNT validator"
else
    print_status "WARN" "Directory has $DN_VALIDATOR_COUNT validators (expected 1)"
fi

# BVN checks
BVN_CHAIN_ID=$(jq -r '.chain_id' consensus_bvn0.json)
if [ "$BVN_CHAIN_ID" = "cyclops.bvn-cyclops" ]; then
    print_status "OK" "BVN chain ID correct: $BVN_CHAIN_ID"
else
    print_status "ERROR" "BVN chain ID incorrect: $BVN_CHAIN_ID"
fi

BVN_VALIDATOR_COUNT=$(jq '.validators | length' consensus_bvn0.json)
if [ "$BVN_VALIDATOR_COUNT" -eq 1 ]; then
    print_status "OK" "BVN has $BVN_VALIDATOR_COUNT validator"
else
    print_status "WARN" "BVN has $BVN_VALIDATOR_COUNT validators (expected 1)"
fi

# Check validator consistency
echo -e "\n👤 Checking validator consistency..."

DN_VALIDATOR_ADDR=$(jq -r '.validators[0].address' consensus_dn.json)
BVN_VALIDATOR_ADDR=$(jq -r '.validators[0].address' consensus_bvn0.json)

if [ "$DN_VALIDATOR_ADDR" = "$BVN_VALIDATOR_ADDR" ]; then
    print_status "OK" "Validator addresses match: $DN_VALIDATOR_ADDR"
else
    print_status "ERROR" "Validator addresses differ: DN=$DN_VALIDATOR_ADDR, BVN=$BVN_VALIDATOR_ADDR"
fi

DN_VALIDATOR_PUBKEY=$(jq -r '.validators[0].pub_key' consensus_dn.json)
BVN_VALIDATOR_PUBKEY=$(jq -r '.validators[0].pub_key' consensus_bvn0.json)

if [ "$DN_VALIDATOR_PUBKEY" = "$BVN_VALIDATOR_PUBKEY" ]; then
    print_status "OK" "Validator public keys match"
else
    print_status "ERROR" "Validator public keys differ"
fi

DN_VALIDATOR_NAME=$(jq -r '.validators[0].name' consensus_dn.json)
BVN_VALIDATOR_NAME=$(jq -r '.validators[0].name' consensus_bvn0.json)

if [ "$DN_VALIDATOR_NAME" = "$BVN_VALIDATOR_NAME" ]; then
    print_status "OK" "Validator names match: $DN_VALIDATOR_NAME"
else
    print_status "ERROR" "Validator names differ: DN=$DN_VALIDATOR_NAME, BVN=$BVN_VALIDATOR_NAME"
fi

# Check consensus parameters
echo -e "\n⚙️  Checking consensus parameters..."

DN_MAX_BYTES=$(jq -r '.consensus_params.block.max_bytes' consensus_dn.json)
BVN_MAX_BYTES=$(jq -r '.consensus_params.block.max_bytes' consensus_bvn0.json)

if [ "$DN_MAX_BYTES" = "$BVN_MAX_BYTES" ] && [ "$DN_MAX_BYTES" = "22020096" ]; then
    print_status "OK" "Block max_bytes consistent: $DN_MAX_BYTES"
else
    print_status "WARN" "Block max_bytes inconsistent: DN=$DN_MAX_BYTES, BVN=$BVN_MAX_BYTES"
fi

DN_PUBKEY_TYPES=$(jq -r '.consensus_params.validator.pub_key_types[0]' consensus_dn.json)
BVN_PUBKEY_TYPES=$(jq -r '.consensus_params.validator.pub_key_types[0]' consensus_bvn0.json)

if [ "$DN_PUBKEY_TYPES" = "$BVN_PUBKEY_TYPES" ] && [ "$DN_PUBKEY_TYPES" = "ed25519" ]; then
    print_status "OK" "Public key types consistent: $DN_PUBKEY_TYPES"
else
    print_status "ERROR" "Public key types inconsistent: DN=$DN_PUBKEY_TYPES, BVN=$BVN_PUBKEY_TYPES"
fi

# Validate base64 public key format
echo -e "\n🔑 Validating public key format..."

if echo "$DN_VALIDATOR_PUBKEY" | base64 -d > /dev/null 2>&1; then
    KEY_LENGTH=$(echo "$DN_VALIDATOR_PUBKEY" | base64 -d | wc -c)
    if [ "$KEY_LENGTH" -eq 32 ]; then
        print_status "OK" "Public key is valid base64 Ed25519 (32 bytes)"
    else
        print_status "ERROR" "Public key wrong length: $KEY_LENGTH bytes (expected 32)"
    fi
else
    print_status "ERROR" "Public key is not valid base64"
fi

# Summary
echo -e "\n📊 Validation Summary"
echo "===================="
echo "Directory Network:"
echo "  Chain ID: $DN_CHAIN_ID"
echo "  Validators: $DN_VALIDATOR_COUNT"
echo "  Validator: $DN_VALIDATOR_NAME"
echo "  Address: $DN_VALIDATOR_ADDR"
echo ""
echo "Block Validator Network:"
echo "  Chain ID: $BVN_CHAIN_ID"
echo "  Validators: $BVN_VALIDATOR_COUNT"
echo "  Validator: $BVN_VALIDATOR_NAME"
echo "  Address: $BVN_VALIDATOR_ADDR"
echo ""
print_status "OK" "Consensus files validation complete"
