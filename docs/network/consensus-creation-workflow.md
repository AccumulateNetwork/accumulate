# Consensus Creation Workflow Documentation

## Overview

This document specifies the consensus creation workflow for Cyclops validator preparation. The consensus creation process generates CometBFT-compatible consensus sections that are embedded into partition snapshots during the extract phase.

## Current Command Analysis

### Available Commands

1. **`generate-consensus-section`** - Creates standalone consensus JSON files
2. **`update-consensus`** - Updates existing consensus files with validator keys
3. **`extract`** - Embeds consensus sections into partition snapshots

### Command Details

#### 1. Generate Consensus Section
```bash
./analyze generate-consensus-section \
  --network-config cyclops-network.json \
  --partition <partition-id> \
  --output <output-file>
```

**Purpose**: Creates a standalone CometBFT GenesisDoc structure for a specific partition.

**Output**: JSON file containing:
- Chain ID (format: `cyclops.{partition-name}`)
- Validator set with public keys and voting power
- Genesis time and configuration parameters

#### 2. Update Consensus Files
```bash
./analyze update-consensus --artifacts <artifacts-dir>
```

**Purpose**: Updates existing consensus files with validator public keys from key files.

**Expected Behavior**:
- Reads validator keys from `priv_validator_key_*.json` files
- Updates consensus configuration files
- Creates/updates `consensus_dn.json` and `consensus_bvn0.json`

## Correct Workflow Specification

### Phase 1: Key Generation and Configuration
```bash
# Step 1: Generate validator keys
./analyze generate-key acc://defidevs.acme .

# Step 2: Update network configuration with public keys
./analyze update-network-keys --network cyclops-network.json --artifacts .
```

### Phase 2: Consensus Section Creation
```bash
# Step 3a: Generate consensus section for Directory partition
./analyze generate-consensus-section \
  --network-config cyclops-network.json \
  --partition Directory \
  --output consensus_dn.json

# Step 3b: Generate consensus section for BVN partition  
./analyze generate-consensus-section \
  --network-config cyclops-network.json \
  --partition bvn-cyclops \
  --output consensus_bvn0.json

# Alternative: Use update-consensus to create both files
./analyze update-consensus --artifacts .
```

### Phase 3: Partition Snapshot Extraction
```bash
# Step 4: Extract partition snapshots with consensus sections
./analyze extract cyclops-network.json cyclops-genesis.snap \
  --partition-snapshots ./partition-snapshots
```

**Expected Behavior**: The extract command should:
1. Read the consensus files (`consensus_dn.json`, `consensus_bvn0.json`)
2. Embed them as consensus sections in the respective partition snapshots
3. Create partition-specific snapshots with proper consensus configuration

## Consensus Section Structure

### Expected JSON Structure
```json
{
  "genesis_time": "2024-01-01T00:00:00Z",
  "chain_id": "cyclops.Directory",
  "initial_height": "1",
  "consensus_params": {
    "block": {
      "max_bytes": "22020096",
      "max_gas": "-1"
    },
    "evidence": {
      "max_age_num_blocks": "100000",
      "max_age_duration": "172800000000000"
    },
    "validator": {
      "pub_key_types": ["ed25519"]
    }
  },
  "validators": [
    {
      "address": "validator-address-hex",
      "pub_key": {
        "type": "tendermint/PubKeyEd25519",
        "value": "base64-public-key"
      },
      "power": "1",
      "name": "acc://defidevs.acme"
    }
  ],
  "app_hash": ""
}
```

### Key Fields
- **`chain_id`**: Format `cyclops.{partition-name}` (e.g., `cyclops.Directory`, `cyclops.bvn-cyclops`)
- **`validators`**: Array of validator configurations with public keys from network config
- **`pub_key.value`**: Base64-encoded public key from validator key files
- **`address`**: Hex-encoded validator address derived from public key

## Implementation Requirements

### 1. Generate Consensus Section Command
**Current Status**: ✅ Implemented

**Required Functionality**:
- Read network configuration JSON
- Extract validators for specified partition
- Generate CometBFT-compatible GenesisDoc
- Output to specified file

### 2. Update Consensus Command  
**Current Status**: ✅ Implemented

**Required Functionality**:
- Read validator key files from artifacts directory
- Create consensus files for all partitions
- Update existing consensus files with current validator keys

### 3. Extract Command Integration
**Current Status**: ⚠️ Needs Verification

**Required Functionality**:
- Read consensus files during extraction
- Embed consensus sections into partition snapshots
- Validate consensus section format and content

## Troubleshooting Current Issues

### Issue: "No validators configured for consensus section"

**Root Cause**: The extract command cannot find or parse validator information.

**Possible Causes**:
1. Consensus files (`consensus_dn.json`, `consensus_bvn0.json`) don't exist
2. Consensus files have incorrect format
3. Extract command is not reading consensus files properly
4. Network configuration doesn't have updated validator keys

**Debugging Steps**:
1. Verify consensus files exist and have correct structure
2. Check that network JSON has updated validator public keys
3. Verify extract command is looking in correct location for consensus files
4. Add debug logging to extract command consensus section loading

### Issue: Base64 vs Hex Encoding
**Problem**: Inconsistent key encoding between components.

**Solution**: Ensure all components use base64 encoding for public keys as specified in network JSON.

## Recommended Automation Workflow

### Updated Script Steps
```bash
#!/bin/bash

# Step 1: Generate validator keys
./analyze generate-key acc://defidevs.acme .

# Step 2: Update network configuration
./analyze update-network-keys --network cyclops-network.json --artifacts .

# Step 3: Create consensus sections
./analyze generate-consensus-section \
  --network-config cyclops-network.json \
  --partition Directory \
  --output consensus_dn.json

./analyze generate-consensus-section \
  --network-config cyclops-network.json \
  --partition bvn-cyclops \
  --output consensus_bvn0.json

# Step 4: Verify consensus files
echo "Verifying consensus files..."
jq . consensus_dn.json > /dev/null && echo "✅ consensus_dn.json valid"
jq . consensus_bvn0.json > /dev/null && echo "✅ consensus_bvn0.json valid"

# Step 5: Extract partition snapshots
./analyze extract cyclops-network.json cyclops-genesis.snap \
  --partition-snapshots ./partition-snapshots

# Step 6: Verify consensus sections in snapshots
./analyze info ./partition-snapshots/Directory-partition.snap
./analyze info ./partition-snapshots/bvn-cyclops-partition.snap
```

## Next Steps

1. **Verify Current Implementation**: Test the `generate-consensus-section` command
2. **Update Automation Script**: Use explicit consensus section generation
3. **Debug Extract Command**: Ensure it properly reads and embeds consensus files
4. **Add Validation**: Verify consensus sections are correctly embedded in snapshots
5. **Update Documentation**: Reflect working consensus creation workflow

This workflow should resolve the "no validators configured" error by ensuring consensus sections are properly created before the extract phase.
