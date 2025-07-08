# Network JSON Structure Documentation

## Overview

This document provides a comprehensive analysis of the network JSON structure used by the Accumulate network configuration system. Understanding this structure is critical for proper validator setup and network operations.

## Problem Analysis

During Cyclops validator preparation, we discovered that the `update-network-keys` command was corrupting the network JSON file by using an incomplete struct definition that only preserved a subset of the required fields.

## Complete Network JSON Structure

### Root Level Structure

```json
{
  "id": "network-identifier",
  "template": "validator-template-config",
  "globals": {
    "oracle": { ... },
    "globals": { ... },
    "network": { ... },
    "routing": { ... }
  }
}
```

### Detailed Field Breakdown

#### 1. Root Fields

- **`id`** (string): Network identifier (e.g., "cyclops", "MainNet")
- **`template`** (string): TOML configuration template for validators

#### 2. Oracle Configuration (`globals.oracle`)

```json
{
  "oracle": {
    "price": 5000
  }
}
```

- **`price`** (integer): Oracle price setting

#### 3. Global Settings (`globals.globals`)

```json
{
  "globals": {
    "majorBlockSchedule": "0 0 * * *",
    "executorVersion": "v2baikonur",
    "feeSchedule": {
      "createIdentitySliding": [4800000, 1200000, 350000, 90000, 25000, 7000, 1800],
      "createSubIdentity": 2500,
      "createTokenAccount": 10000,
      // ... more fee settings
    },
    "limits": {
      "accountAuthorities": 20,
      "bookPages": 20,
      "dataEntryParts": 100,
      // ... more limits
    },
    "operatorAcceptThreshold": {
      "denominator": 3,
      "numerator": 2
    },
    "validatorAcceptThreshold": {
      "denominator": 3,
      "numerator": 2
    },
    "values": {
      "acmeSupply": 500000000000000,
      "acmePrecision": 8,
      "acmeIssuedSupply": 500000000000000
    }
  }
}
```

#### 4. Network Configuration (`globals.network`)

This is the most critical section for validator operations:

```json
{
  "network": {
    "networkName": "cyclops",
    "partitions": [
      {
        "id": "bvn-cyclops",
        "type": "blockValidator"
      },
      {
        "id": "Directory",
        "type": "directory"
      }
    ],
    "validators": [
      {
        "operator": "acc://defidevs.acme",
        "publicKey": "base64-encoded-public-key",
        "partitions": [
          {
            "id": "Directory",
            "active": true
          },
          {
            "id": "bvn-cyclops", 
            "active": true
          }
        ]
      }
    ]
  }
}
```

**Key Points:**
- **`partitions`**: Top-level partition definitions (required by extract command)
- **`validators[].partitions`**: Per-validator partition assignments
- **`publicKey`**: Base64-encoded validator public key (updated by update-network-keys)

#### 5. Routing Configuration (`globals.routing`)

```json
{
  "routing": {
    "overrides": [
      {
        "account": "acc://staking.acme",
        "partition": "Directory"
      }
    ],
    "routes": [
      {
        "length": 2,
        "partition": "Apollo"
      }
    ]
  }
}
```

## Go Struct Definitions

### Extract Command Structure

The extract command expects this structure (from `a_extract_network.go`):

```go
type NetworkConfig struct {
    ID string `json:"id"`
    
    Globals struct {
        Oracle struct {
            Price int `json:"price"`
        } `json:"oracle"`
        
        Globals struct {
            // Nested globals fields
        } `json:"globals"`
        
        Network struct {
            NetworkName string `json:"networkName"`
            
            Partitions []struct {
                ID   string `json:"id"`
                Type string `json:"type"`
            } `json:"partitions"`
            
            Validators []struct {
                Operator string `json:"operator"`
                PublicKey string `json:"publicKey"`
                
                Partitions []struct {
                    ID string `json:"id"`
                    Active bool `json:"active"`
                } `json:"partitions"`
            } `json:"validators"`
        } `json:"network"`
    } `json:"globals"`
}
```

### Update Network Keys Structure (INCOMPLETE)

The current `update-network-keys` command uses this incomplete structure:

```go
type networkConfig struct {
    Globals struct {
        Network struct {
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

**Problem**: This struct is missing:
- Root `id` field
- `template` field  
- `oracle` configuration
- `globals.globals` nested structure
- `network.networkName` field
- `network.partitions` array
- `routing` configuration

## Issues and Solutions

### Issue 1: Struct Mismatch
**Problem**: The `update-network-keys` command uses an incomplete struct, causing data loss.

**Solution**: Update the `networkConfig` struct to include all fields, or use `json.RawMessage` for preservation.

### Issue 2: Extract Command Parsing
**Problem**: Extract command shows "Partitions Count: 0" even with correct JSON.

**Root Cause**: The JSON structure is correct, but there may be parsing issues in the extract command.

### Issue 3: Base64 vs Hex Encoding
**Problem**: Public keys are stored as base64 but some code expects hex.

**Solution**: Ensure consistent encoding throughout the pipeline.

## Best Practices

### 1. Preserve Complete Structure
Always use complete struct definitions that preserve all JSON fields:

```go
// Use json.RawMessage for unknown fields
type SafeNetworkConfig struct {
    ID       string          `json:"id"`
    Template string          `json:"template,omitempty"`
    Globals  json.RawMessage `json:"globals"`
}
```

### 2. Backup Before Modifications
Always create backups before modifying network JSON:

```bash
cp cyclops-network.json cyclops-network.json.bak
```

### 3. Validate After Updates
Verify structure integrity after updates:

```bash
jq . cyclops-network.json > /dev/null  # Validate JSON syntax
./analyze extract cyclops-network.json --help  # Test parsing
```

## Command Reference

### Extract Command
```bash
./analyze extract <network.json> <snapshot> --partition-snapshots <output-dir>
```

### Update Network Keys
```bash
./analyze update-network-keys --network <network.json> --artifacts <keys-dir>
```

### Update Consensus
```bash
./analyze update-consensus --artifacts <artifacts-dir>
```

## Troubleshooting

### "Partitions Count: 0" Error
1. Verify `globals.network.partitions` array exists
2. Check JSON syntax with `jq`
3. Ensure struct definitions match JSON structure

### Public Key Encoding Issues
1. Verify keys are base64 encoded in network JSON
2. Check if extract command expects hex encoding
3. Add conversion logic if needed

### File Corruption After Updates
1. Use complete struct definitions
2. Implement field preservation logic
3. Always backup before modifications

## Example Files

### Minimal Cyclops Network JSON
```json
{
  "id": "cyclops",
  "globals": {
    "oracle": {
      "price": 5000
    },
    "network": {
      "networkName": "cyclops",
      "partitions": [
        {
          "id": "bvn-cyclops",
          "type": "blockValidator"
        },
        {
          "id": "Directory",
          "type": "directory"
        }
      ],
      "validators": [
        {
          "operator": "acc://defidevs.acme",
          "publicKey": "base64-key-here",
          "partitions": [
            {
              "id": "Directory",
              "active": true
            },
            {
              "id": "bvn-cyclops",
              "active": true
            }
          ]
        }
      ]
    }
  }
}
```

This documentation should be updated as we discover more about the network JSON structure and resolve the current parsing issues.
