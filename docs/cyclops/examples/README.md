# Cyclops Consensus Examples

This directory contains example consensus JSON files generated for the Cyclops network deployment.

## Files

### consensus_dn.json
CometBFT consensus section for the Directory Network (DN) partition.

**Chain ID**: `cyclops.Directory`
**Partition Type**: `directory`
**Validators**: 1 (acc://defidevs.acme)

### consensus_bvn0.json  
CometBFT consensus section for the Block Validator Network (BVN) partition.

**Chain ID**: `cyclops.bvn-cyclops`
**Partition Type**: `blockValidator`
**Validators**: 1 (acc://defidevs.acme)

## Generation Commands

These files were generated using:

```bash
# Directory Network consensus
./analyze generate-consensus-section \
  --network-config cyclops-network.json \
  --partition Directory \
  --output consensus_dn.json

# BVN consensus  
./analyze generate-consensus-section \
  --network-config cyclops-network.json \
  --partition bvn-cyclops \
  --output consensus_bvn0.json
```

## Structure

Both files follow the CometBFT GenesisDoc format:

```json
{
  "genesis_time": "2025-07-07T02:28:37Z",
  "chain_id": "cyclops.{partition-name}",
  "initial_height": 1,
  "consensus_params": {
    "block": { "max_bytes": 22020096, "max_gas": -1 },
    "evidence": { "max_age_num_blocks": 100000, "max_age_duration": 172800000000000, "max_bytes": 1048576 },
    "validator": { "pub_key_types": ["ed25519"] },
    "version": { "app": 0 },
    "abci": { "vote_extensions_enable_height": 0 }
  },
  "validators": [
    {
      "address": "40507A6CEC83EF9DEDB62445FD6CEA25EFE22C30",
      "pub_key": "Qtsc9T9HyIr/FhFO+Vp20XDd0dSZHX972jj8YBpbgOM=",
      "power": 1,
      "name": "acc://defidevs.acme"
    }
  ],
  "app_hash": ""
}
```

## Key Features

- **Chain ID Format**: `cyclops.{partition-name}` for proper partition identification
- **Validator Addressing**: CometBFT-compatible addresses derived from Ed25519 public keys
- **Base64 Public Keys**: Preserved from network JSON configuration
- **Equal Voting Power**: All validators assigned power = 1
- **Default Parameters**: Standard CometBFT consensus parameters for production use

## Usage in Deployment

These consensus files are embedded into partition snapshots during the extract phase:

```bash
./analyze extract cyclops-network.json cyclops-genesis.snap
```

The extract command automatically locates and embeds the appropriate consensus file for each partition.

## Validation

Verify the consensus files:

```bash
# Validate JSON structure
jq '.' consensus_dn.json > /dev/null && echo "Directory consensus: Valid"
jq '.' consensus_bvn0.json > /dev/null && echo "BVN consensus: Valid"

# Check validator consistency
echo "Network validator public key:"
jq -r '.globals.network.validators[0].publicKey' ../../../tmp/cyclops/artifacts/cyclops-network.json

echo "Directory consensus public key:"
jq -r '.validators[0].pub_key' consensus_dn.json

echo "BVN consensus public key:"  
jq -r '.validators[0].pub_key' consensus_bvn0.json
```

All three should match: `Qtsc9T9HyIr/FhFO+Vp20XDd0dSZHX972jj8YBpbgOM=`

## Validation Commands

### Automated Validation Script
```bash
# Run comprehensive validation
./validate-consensus.sh
```

The validation script checks:
- File existence and JSON validity
- Required fields (chain_id, validators, consensus_params)
- Validator consistency between partitions
- Public key format (base64 Ed25519)
- Consensus parameter consistency

### Manual JSON Structure Validation
```bash
# Validate JSON syntax
jq '.' consensus_dn.json
jq '.' consensus_bvn0.json

# Check validator count
jq '.validators | length' consensus_dn.json
jq '.validators | length' consensus_bvn0.json

# Verify chain IDs
jq -r '.chain_id' consensus_dn.json
jq -r '.chain_id' consensus_bvn0.json
```

### Validator Information
```bash
# Extract validator details
jq '.validators[0] | {address, pub_key, power, name}' consensus_dn.json
jq '.validators[0] | {address, pub_key, power, name}' consensus_bvn0.json

# Verify public key format (should be base64)
echo "Qtsc9T9HyIr/FhFO+Vp20XDd0dSZHX972jj8YBpbgOM=" | base64 -d | wc -c
# Should output: 32 (Ed25519 key length)
```

## See Also

- [Consensus Generation Fix](../consensus-generation-fix.md) - Technical details of the fix
- [Cyclops Automation README](../cyclops-automation-readme.md) - Complete deployment workflow
- [Network JSON Structure](../../network/network-json-structure.md) - Network configuration format
