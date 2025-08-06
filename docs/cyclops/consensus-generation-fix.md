# Consensus Generation Fix - CometBFT Format Conversion

## Overview

The `generate-consensus-section` command creates CometBFT-compatible consensus sections for Accumulate network partitions. This document details the critical fix that resolved the "partition not found" and public key decoding errors.

## Root Cause Analysis

### Problem 1: Struct Mismatch
The original Go struct expected validators to be nested under partitions, but the actual Cyclops network JSON has validators at the network level with partition assignments.

**Original (Incorrect) Structure:**
```json
{
  "globals": {
    "network": {
      "partitions": [
        {
          "id": "Directory",
          "validators": [...]  // ❌ Validators not here
        }
      ]
    }
  }
}
```

**Actual Cyclops Structure:**
```json
{
  "globals": {
    "network": {
      "partitions": [
        {"id": "Directory", "type": "directory"},
        {"id": "bvn-cyclops", "type": "blockValidator"}
      ],
      "validators": [  // ✅ Validators at network level
        {
          "operator": "acc://defidevs.acme",
          "publicKey": "Qtsc9T9HyIr/FhFO+Vp20XDd0dSZHX972jj8YBpbgOM=",
          "partitions": [
            {"id": "Directory", "active": true},
            {"id": "bvn-cyclops", "active": true}
          ]
        }
      ]
    }
  }
}
```

### Problem 2: Public Key Encoding
The code was trying to decode base64 public keys as hex strings.

**Error:** `encoding/hex: invalid byte: U+0051 'Q'`
- Network JSON stores public keys in base64 format
- Code was using `hex.DecodeString()` instead of `base64.StdEncoding.DecodeString()`

### Problem 3: Import Conflicts
Conflicting imports between `crypto/ed25519` and `github.com/cometbft/cometbft/crypto/ed25519`.

## Solution Implementation

### 1. Fixed Network JSON Struct
```go
// Updated struct to match actual Cyclops network JSON
var networkConfig struct {
    Globals struct {
        Network struct {
            Partitions []struct {
                ID   string `json:"id"`
                Type string `json:"type"`
            } `json:"partitions"`
            Validators []struct {
                Operator   string `json:"operator"`
                PublicKey  string `json:"publicKey"`
                Partitions []struct {
                    ID     string `json:"id"`
                    Active bool   `json:"active"`
                } `json:"partitions"`
            } `json:"validators"`
        } `json:"network"`
    } `json:"globals"`
}
```

### 2. Fixed Validator Selection Logic
```go
// Find validators that are assigned to this partition
for _, netValidator := range networkConfig.Globals.Network.Validators {
    // Check if this validator is assigned to the target partition
    for _, partition := range netValidator.Partitions {
        if partition.ID == flagPartition && partition.Active {
            activeValidators = append(activeValidators, ...)
            break
        }
    }
}
```

### 3. Fixed Public Key Decoding
```go
// Changed from hex to base64 decoding
pubKeyBytes, err := base64.StdEncoding.DecodeString(validator.PublicKey)
if err != nil {
    return fmt.Errorf("failed to decode public key for validator %s: %w", validator.Operator, err)
}
```

### 4. Fixed Import Conflicts
```go
import (
    "encoding/base64"
    "encoding/json"
    // ... other imports
    
    "github.com/spf13/cobra"
    "github.com/cometbft/cometbft/types"
    crypted25519 "github.com/cometbft/cometbft/crypto/ed25519"
    stded25519 "crypto/ed25519"
)
```

## CometBFT Format Conversion

The consensus generation performs a critical format conversion from network JSON to CometBFT `GenesisDoc`:

### Input: Network JSON Validator
```json
{
  "operator": "acc://defidevs.acme",
  "publicKey": "Qtsc9T9HyIr/FhFO+Vp20XDd0dSZHX972jj8YBpbgOM=",
  "partitions": [
    {"id": "Directory", "active": true}
  ]
}
```

### Output: CometBFT GenesisValidator
```json
{
  "address": "40507A6CEC83EF9DEDB62445FD6CEA25EFE22C30",
  "pub_key": "Qtsc9T9HyIr/FhFO+Vp20XDd0dSZHX972jj8YBpbgOM=",
  "power": 1,
  "name": "acc://defidevs.acme"
}
```

### Key Transformations:
1. **Address Generation**: Computed from public key using CometBFT addressing
2. **Power Assignment**: All validators get equal voting power (1)
3. **Name Mapping**: Operator URL becomes validator name
4. **Public Key Preservation**: Base64 format maintained

## Usage Examples

### Generate Directory Consensus
```bash
./analyze generate-consensus-section \
  --network-config cyclops-network.json \
  --partition Directory \
  --output consensus_dn.json
```

### Generate BVN Consensus
```bash
./analyze generate-consensus-section \
  --network-config cyclops-network.json \
  --partition bvn-cyclops \
  --output consensus_bvn0.json
```

## Validation Commands

### Verify Consensus Structure
```bash
# Check chain ID format
jq -r '.chain_id' consensus_dn.json
# Output: cyclops.Directory

# Check validator count
jq '.validators | length' consensus_dn.json
# Output: 1

# Check validator address
jq -r '.validators[0].address' consensus_dn.json
# Output: 40507A6CEC83EF9DEDB62445FD6CEA25EFE22C30
```

### Verify Public Key Consistency
```bash
# Network JSON public key
jq -r '.globals.network.validators[0].publicKey' cyclops-network.json

# Consensus JSON public key (should match)
jq -r '.validators[0].pub_key' consensus_dn.json
```

## Integration with Extract Command

The generated consensus files are embedded into partition snapshots:

```bash
./analyze extract cyclops-network.json cyclops-genesis.snap
```

The extract command automatically finds and embeds:
- `consensus_dn.json` → Directory partition snapshot
- `consensus_bvn0.json` → BVN partition snapshot

## Troubleshooting

### Common Errors

**"partition not found"**
- Verify partition ID matches exactly (case-sensitive)
- Check network JSON structure matches expected format

**"invalid byte" in hex decoding**
- Indicates base64 vs hex encoding mismatch
- Verify public key format in network JSON

**"no validators configured"**
- Check validator partition assignments
- Verify `active: true` for target partition

### Debug Commands

```bash
# List available partitions
jq -r '.globals.network.partitions[].id' cyclops-network.json

# Check validator partition assignments
jq -r '.globals.network.validators[0].partitions' cyclops-network.json

# Validate JSON structure
jq '.' consensus_dn.json > /dev/null && echo "Valid JSON"
```

## File Locations

- **Source Code**: `/home/paulsnow/go/src/gitlab.com/AccumulateNetwork/accumulate/tools/cmd/analyze/cmd_generate_consensus.go`
- **Example Files**: `/home/paulsnow/go/src/gitlab.com/AccumulateNetwork/accumulate/docs/cyclops/examples/`
- **Network Config**: `/tmp/cyclops/artifacts/cyclops-network.json`

## See Also

- [Cyclops Automation README](cyclops-automation-readme.md)
- [Network JSON Structure](../network/network-json-structure.md)
- [Debug App Reference](../api/debug-app-reference.md)
- [Consensus Creation Workflow](consensus-creation-workflow.md)
