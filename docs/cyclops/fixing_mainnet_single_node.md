# Fixing Mainnet Single Node Deployment

## Overview
This document details the exact errors, file formats, and code issues encountered when deploying a Cyclops mainnet single-node validator.

## Error Analysis

### 1. Routing Configuration Error
**Error Message:**
```
panic: expected values with 10 at 2:0, found none
```

**Exact Location:** 
- File: `/internal/api/routing/tree.go:89`
- Function: `buildPrefixTree()`
- Error Line: `return nil, errors.InternalError.WithFormat("expected values with %b at %d:%d, found none", i, offset, depth)`
- Called from: `NewRouter()` → `NewRouteTree()` → `buildPrefixTree()`
- Call Stack: `genesis.Init()` (line 102) → `routing.NewRouter()` → `NewRouteTree()` → `buildPrefixTree()`

**Root Cause:** The routing table is missing the `value: 0` field for the first routing entry. The prefix tree builder expects all possible bit patterns to be covered, but the first entry (length=2, partition="Directory") is missing `"value": 0`.

### 2. Key Format Errors
**Error Message:**
```
bad seed length: 96
```

**Root Cause:** Mismatch between hex and base64 encoding expectations in key handling.

## File Format Analysis

### Network Configuration File: `cyclops-network.json`

**Location:** `/tmp/cyclops/node/artifacts/cyclops-network.json`

**Key Fields and Formats:**

#### Validator Public Keys
```json
{
  "validators": [
    {
      "publicKey": "Qtsc9T9HyIr/FhFO+Vp20XDd0dSZHX972jj8YBpbgOM=",
      "partitions": {
        "Directory": { "active": true },
        "bvn-cyclops": { "active": true }
      }
    }
  ]
}
```
- **Format:** Base64-encoded Ed25519 public key (32 bytes)
- **Encoding:** Standard base64 encoding
- **Length:** 44 characters (32 bytes * 4/3 + padding)

#### Current Routing Configuration (BROKEN)
```json
{
  "globals": {
    "routing": {
      "routes": [
        {
          "length": 2,
          "partition": "Directory"
          // MISSING: "value": 0
        },
        {
          "length": 2,
          "partition": "bvn-cyclops",
          "value": 1
        },
        {
          "length": 3,
          "partition": "Directory",
          "value": 6
        },
        {
          "length": 4,
          "partition": "bvn-cyclops",
          "value": 14
        },
        {
          "length": 4,
          "partition": "Directory",
          "value": 15
        }
      ]
    }
  }
}
```

**Problem:** The first routing entry is missing `"value": 0`. The prefix tree builder in `buildPrefixTree()` expects all bit patterns to be covered. When it processes length=2 routes, it looks for values 0, 1, 2, 3 (binary 00, 01, 10, 11). It finds value 1 but not value 0, causing the panic "expected values with 10 at 2:0, found none" (looking for binary 10 = decimal 2).

### Node Key File: `node_key.json`

**Location:** `.accumulate/config/node_key.json`

**Format:**
```json
{
  "type": "tendermint/PrivValidatorKey",
  "priv_key": {
    "type": "ed25519",
    "value": "hex-encoded-64-character-string"
  }
}
```
- **Private Key Format:** Hex-encoded Ed25519 seed (32 bytes = 64 hex characters)
- **Expected by:** Tendermint/CometBFT consensus engine

### Validator Key Files: `priv_validator_key.json`

**Location:** `.accumulate/config/priv_validator_key.json`

**Format:**
```json
{
  "address": "hex-encoded-address",
  "pub_key": {
    "type": "ed25519",
    "value": "base64-encoded-public-key"
  },
  "priv_key": {
    "type": "ed25519", 
    "value": "base64-encoded-private-key-plus-public-key"
  }
}
```
- **Public Key:** Base64-encoded (32 bytes)
- **Private Key:** Base64-encoded (64 bytes: 32-byte seed + 32-byte public key)

## Code Analysis

### Key Generation and Handling

#### File: `cmd_generate_key.go`
**Struct Definition:**
```go
type PrivValidatorKey struct {
    Address string `json:"address"`
    PubKey  struct {
        Type  string `json:"type"`
        Value string `json:"value"` // base64-encoded
    } `json:"pub_key"`
    PrivKey struct {
        Type  string `json:"type"`
        Value string `json:"value"` // base64-encoded
    } `json:"priv_key"`
}
```

#### File: `internal/node/daemon/init.go`
**Problem Location:** Lines 217-218, 225-226
```go
func BuildGenesisDocs(...) {
    // This expects node.PrivValKey to be raw bytes
    privKey := tmed25519.PrivKey(node.PrivValKey)
    // But network JSON stores base64-encoded strings
}
```

**Issue:** The code expects `node.PrivValKey` to be raw bytes, but the network JSON unmarshaling provides base64-decoded bytes that may not be in the correct format.

### Network Configuration Loading

#### File: `cmd_init_network.go`
**Function:** `loadNetworkConfiguration()`
```go
func loadNetworkConfiguration(file string) (*NetworkInit, error) {
    // Loads and unmarshals network JSON
    // May not properly handle key format conversion
}
```

### Routing Table Construction

#### File: `internal/api/routing/router.go`
**Problem Location:** Line 68
```go
func NewRouter(network *config.Network) *Router {
    // Expects complete routing table
    // Panics if routing entries are missing
}
```

## Key Format Conversion Issues

### Base64 vs Hex Encoding

1. **Network JSON:** Uses base64 encoding for validator public keys
2. **Tendermint Keys:** Uses hex encoding for private key seeds
3. **CometBFT Genesis:** Expects base64 encoding for validator keys

### Conversion Points

#### In `cmd_generate_consensus.go`:
```go
// CORRECT: Base64 decoding for network JSON public keys
pubKeyBytes, err := base64.StdEncoding.DecodeString(validator.PublicKey)

// INCORRECT: Would be hex.DecodeString() - causes format mismatch
```

#### In `cmd_update_network_keys.go`:
```go
// Network JSON expects base64-encoded public keys
validator.PublicKey = base64.StdEncoding.EncodeToString(pubKeyBytes)
```

## Missing Routing Configuration

### Required Routing Entries
The routing table must cover all possible account identifier patterns:

```json
{
  "globals": {
    "routing": [
      {"length": 2, "value": 0, "partition": "Directory"},
      {"length": 2, "value": 1, "partition": "bvn-cyclops"},
      {"length": 3, "value": 6, "partition": "Directory"},
      {"length": 4, "value": 14, "partition": "bvn-cyclops"},
      {"length": 4, "value": 15, "partition": "Directory"}
    ]
  }
}
```

**Error Cause:** The routing table is incomplete, causing the router initialization to fail with "expected values with 10 at 2:0, found none".

## Genesis Initialization Process

### Command Flow
1. `accumulated init genesis cyclops-network.json --work-dir .accumulate --snapshot cyclops-genesis.snap`
2. `loadNetworkConfiguration()` - Loads network JSON
3. `BuildGenesisDocs()` - Processes network configuration
4. `genesis.Init()` - Initializes partition state
5. `NewRouter()` - **FAILS HERE** due to incomplete routing table

### Snapshot Requirements
The `init genesis` command expects individual partition snapshots:
- `Directory-partition.snap`
- `bvn-cyclops-partition.snap`

**Current Issue:** We have unified `cyclops-genesis.snap` but need individual partition snapshots.

## Solutions Required

### 1. Fix Routing Configuration
Add complete routing table to `cyclops-network.json`:
```json
{
  "globals": {
    "routing": [
      {"length": 2, "value": 0, "partition": "Directory"},
      {"length": 2, "value": 1, "partition": "bvn-cyclops"},
      {"length": 3, "value": 6, "partition": "Directory"},
      {"length": 4, "value": 14, "partition": "bvn-cyclops"},
      {"length": 4, "value": 15, "partition": "Directory"}
    ]
  }
}
```

### 2. Use Individual Partition Snapshots
```bash
accumulated init genesis cyclops-network.json \
  --work-dir .accumulate \
  --snapshot Directory-partition.snap \
  --snapshot bvn-cyclops-partition.snap
```

### 3. Fix Key Format Handling
Ensure consistent base64/hex encoding throughout the key generation and loading pipeline.

## File Permissions and Security

### Required Permissions
- `node_key.json`: 600 (read/write owner only)
- `priv_validator_key.json`: 600 (read/write owner only)
- Configuration files: 644 (readable by all, writable by owner)

## Next Steps

1. **Verify Routing Table:** Ensure complete routing configuration in network JSON
2. **Extract Partition Snapshots:** Split unified snapshot into individual partition snapshots
3. **Test Key Formats:** Validate all key files have correct encoding
4. **Run Genesis Initialization:** Use corrected configuration and snapshots
5. **Validate Node Startup:** Ensure node starts without key or routing errors

## Code Locations for Further Investigation

- **Routing Logic:** `/internal/api/routing/router.go`
- **Genesis Building:** `/internal/node/daemon/init.go`
- **Network Loading:** `/cmd/accumulated/cmd_init_network.go`
- **Key Generation:** `/tools/cmd/analyze/cmd_generate_key.go`
- **Consensus Generation:** `/tools/cmd/analyze/cmd_generate_consensus.go`
