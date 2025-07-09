# Cyclops Network JSON Reference

This document provides the reference configuration for the Cyclops network, including the original network JSON structure that serves as the foundation for validator deployment automation.

## Overview

The Cyclops network is a two-partition Accumulate network designed for validator testing and development. It consists of:

- **Directory Network (DN)**: The primary directory partition
- **Block Validator Network (BVN)**: A single BVN partition named `bvn-cyclops`
- **Single Validator**: `acc://defidevs.acme` operating on both partitions

## Reference Files

### Primary Reference
- **File**: `cyclops-network-reference.json`
- **Purpose**: Clean reference copy of the original network configuration
- **Status**: Read-only, preserved for documentation
- **Usage**: Template for creating new deployments or restoring corrupted configurations

### Working Copy
- **File**: `/home/paulsnow/accumulate-network/artifacts/cyclops-network.json`
- **Purpose**: Active configuration used by automation scripts
- **Status**: Modified by `update-network-keys` and other automation tools
- **Usage**: Runtime configuration for deployment scripts

## Network Configuration Structure

### Root Level Fields

```json
{
  "id": "cyclops",                    // Network identifier
  "template": "...",                  // Node configuration template
  "globals": { ... }                  // Global network settings
}
```

### Key Components

#### 1. Network Identity
- **Network ID**: `cyclops`
- **Network Name**: `cyclops` (in `globals.network.networkName`)

#### 2. Node Configuration Template
The template defines the base configuration for all validator nodes:
- **Type**: `coreValidator`
- **Healing**: Disabled (`enable-healing = false`)
- **Snapshots**: Disabled (`enable-snapshots = false`)
- **Storage**: LevelDB backend

#### 3. Global Settings

##### Oracle Configuration
```json
"oracle": {
  "price": 5000
}
```

##### Fee Schedule
Comprehensive fee structure for all transaction types, including:
- Identity creation (sliding scale)
- Account operations
- Token operations
- Synthetic transactions

##### System Limits
Resource limits for various operations:
- Account authorities: 20
- Book pages: 20
- Data entry size: 100KB
- Identity accounts: 1000

##### Consensus Thresholds
- **Operator Accept Threshold**: 2/3
- **Validator Accept Threshold**: 2/3

##### ACME Token Configuration
- **Total Supply**: 5,000,000 ACME
- **Precision**: 8 decimal places
- **Issued Supply**: 5,000,000 ACME

#### 4. Routing Configuration

The routing section defines how accounts are distributed across partitions using a simplified routing table with critical system account overrides:

```json
"routing": {
  "overrides": [
    {
      "account": "acc://ACME",
      "partition": "Directory"
    },
    {
      "account": "acc://dn.acme",
      "partition": "Directory"
    },
    {
      "account": "acc://staking.acme",
      "partition": "Directory"
    },
    {
      "account": "acc://bvn-cyclops.acme",
      "partition": "bvn-cyclops"
    }
  ],
  "routes": [
    {
      "length": 1,
      "value": 0,
      "partition": "bvn-cyclops"
    }
  ]
}
```

**Routing Logic**:
- **Default Route**: All accounts route to `bvn-cyclops` partition (1-bit routing, value=0)
- **System Account Overrides**: Critical system accounts explicitly routed to Directory:
  - `acc://ACME` - Root network account
  - `acc://dn.acme` - Directory network account
  - `acc://staking.acme` - Staking system account (critical)
  - `acc://bvn-cyclops.acme` - BVN partition account (routes to self)

**Design Rationale**:
- **Simplified Routing**: Single default route reduces complexity and potential routing errors
- **Override-Based**: System accounts use explicit overrides for guaranteed correct routing
- **1-Bit Routing**: Minimal routing table size with maximum reliability

#### 5. Network Topology

##### Partitions
```json
"partitions": [
  { "id": "bvn-cyclops", "type": "blockValidator" },
  { "id": "Directory", "type": "directory" }
]
```

##### Validators
```json
"validators": [
  {
    "operator": "acc://defidevs.acme",
    "publicKey": "yCVhlnfscukrTa+Y9TU2eVbf46nJqaPpA7NoXJw/8uE=",
    "partitions": [
      { "id": "Directory", "active": true },
      { "id": "bvn-cyclops", "active": true }
    ]
  }
]
```

## Automation Script Modifications

### Fields Modified by Scripts

The following fields are updated by automation scripts:

1. **Validator Public Keys**: Updated by `update-network-keys` command
2. **Validator Partitions**: Ensured to have `active: true` status
3. **Backup Creation**: Original preserved with timestamp

### Preservation Strategy

To prevent loss of the original configuration:

1. **Reference Copy**: This documentation preserves the clean configuration
2. **Backup Creation**: Scripts create timestamped backups before modifications
3. **Validation**: Scripts verify structure integrity after updates

## 🔑 Key Management Context

### What's IN the Network JSON
- **Validator Public Keys**: Used for consensus signing (Ed25519 public keys)
- **Validator Addresses**: Derived from public keys for identification
- **Operator Information**: Account operators for each validator
- **Partition Assignments**: Which partitions each validator serves

### What's NOT in the Network JSON
- **Validator Private Keys**: Stored separately in `priv_validator_key.json`
- **P2P Node Keys**: Stored separately in `node_key.json` for networking
- **Node-Specific Configuration**: Each node manages its own keys

### Key Architecture
The network JSON is part of a **three-file key system**:
1. **`cyclops-network.json`** - Validator public keys and network config (**THIS FILE**)
2. **`priv_validator_key.json`** - Private validator key for consensus signing
3. **`node_key.json`** - P2P networking key (separate from validator key)

📚 **See**: [Key Management Guide](cyclops-key-management-guide.md) for complete architecture details

---

## Usage in Deployment

### Phase 1 Preparation
1. Copy reference JSON to working directory
2. Generate validator keys using `analyze gen-key`
3. Update network JSON with `update-network-keys`
4. Generate consensus sections for each partition
5. Extract partition snapshots

### Restoration
If the working copy becomes corrupted:
```bash
cp docs/cyclops/cyclops-network-reference.json ~/accumulate-network/artifacts/cyclops-network.json
```

## Validation Commands

### Structure Validation
```bash
# Validate JSON syntax
jq empty cyclops-network.json

# Check required fields
jq '.id, .globals.network.networkName, .globals.network.partitions | length' cyclops-network.json

# Verify validator configuration
jq '.globals.network.validators[] | {operator, partitions: .partitions[].id}' cyclops-network.json
```

### Routing Validation
```bash
# Check routing configuration
jq '.globals.routing' cyclops-network.json

# Verify routing overrides
jq '.globals.routing.overrides[] | {account, partition}' cyclops-network.json

# Verify default route
jq '.globals.routing.routes[] | {length, value, partition}' cyclops-network.json

# Count routing rules
jq '.globals.routing | {routes: (.routes | length), overrides: (.overrides | length)}' cyclops-network.json
```

## Security Considerations

### Sensitive Information
- **Validator Public Keys**: Present in working copy, updated by automation
- **Private Keys**: Stored separately in `priv_validator_key_*.json` files
- **Network Configuration**: Contains operational parameters

### File Permissions
- Reference file: Read-only (644)
- Working copy: Read-write for automation (644)
- Private keys: Restricted access (600)

## Troubleshooting

### Common Issues

#### Missing Fields After Update
**Symptom**: Network JSON missing critical fields after `update-network-keys`
**Solution**: Restore from reference copy and re-run preparation

#### Invalid JSON Structure
**Symptom**: JSON parsing errors
**Solution**: Validate with `jq` and restore from backup if needed

#### Missing Routing Configuration
**Symptom**: Account routing failures
**Solution**: Verify routing section exists and has proper structure

### Recovery Procedures

1. **Restore from Reference**:
   ```bash
   cp docs/cyclops/cyclops-network-reference.json ~/accumulate-network/artifacts/cyclops-network.json
   ```

2. **Restore from Backup**:
   ```bash
   ls ~/accumulate-network/artifacts/cyclops-network.json.backup.*
   cp ~/accumulate-network/artifacts/cyclops-network.json.backup.YYYYMMDD_HHMMSS ~/accumulate-network/artifacts/cyclops-network.json
   ```

## See Also

- [Cyclops Deployment Guide](cyclops-deployment-guide.md)
- [Network JSON Structure](../network/network-json-structure.md)
- [Validator Key Management](validator-key-management.md)
- [Consensus Creation Workflow](consensus-creation-workflow.md)

---

*This documentation is optimized for AI assistant navigation and developer productivity. The reference configuration is preserved to ensure reliable Cyclops network deployments.*
