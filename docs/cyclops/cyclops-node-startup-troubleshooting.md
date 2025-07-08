# Cyclops Node Startup Troubleshooting Guide

## Overview
This document provides a step-by-step troubleshooting guide for resolving common Cyclops validator node startup issues. These steps were validated during actual deployment testing on 2025-07-07.

## Prerequisites
- Cyclops network artifacts prepared (snapshots, keys, network config)
- `accumulated` and `analyze` binaries available
- Partition snapshots: `bvn-cyclops-partition.snap` and `Directory-partition.snap`

## Common Startup Issues and Solutions

### Issue 1: Missing node_key.json File

**Error:**
```
Error: load daemon: reading config file: read: open .accumulate/bvn1-1/config/node_key.json: no such file or directory
```

**Root Cause:** The `init network` command creates directory structure but doesn't generate the required `node_key.json` file.

**Solution:**
```bash
cd /path/to/artifacts
./analyze generate-node-key .accumulate/bvn1-1/config/node_key.json
```

**Validation:**
```bash
ls -la .accumulate/bvn1-1/config/node_key.json
# Should show: -rw------- (600 permissions)
```

### Issue 2: Unknown Partition Type PartitionType:0

**Error:**
```
ERROR Service failed error="unknown partition type PartitionType:0"
```

**Root Cause:** Missing or incorrect `[describe]` section in `accumulate.toml`.

**Solution:** Update `.accumulate/bvn1-1/config/accumulate.toml`:
```toml
[describe]
  type = "blockValidator"
  partition-id = "bvn-cyclops"
```

**Key Points:**
- Use `"blockValidator"` for BVN nodes (not "coreValidator")
- Use `"directory"` for Directory nodes
- Partition ID must match network configuration

### Issue 3: Unknown Storage Format

**Error:**
```
ERROR Service failed error="open database: unknown storage format \"\""
```

**Root Cause:** Missing `[storage]` section in configuration.

**Solution:** Add to `accumulate.toml`:
```toml
[storage]
  type = "leveldb"
  path = "data/accumulate.db"

[network]
  id = "cyclops"

[logging]
  level = "info"
```

### Issue 4: TOML Configuration Conflicts

**Error:**
```
Error: toml: key table already exists as a configurations, but should be an array table
```

**Root Cause:** Conflicting `[configurations]` and `[[configurations]]` sections.

**Solution:** Use clean TOML structure without conflicts:
```toml
[describe]
  type = "blockValidator"
  partition-id = "bvn-cyclops"

[storage]
  type = "leveldb"
  path = "data/accumulate.db"

[network]
  id = "cyclops"

[logging]
  level = "info"

[p2p]
  [p2p.key]
    address = "AS12dNz8cb3WGLEqzwXjKpTBKWzy4iVBicTS39woa8jKxHpXFYDm"
    type = "raw"
```

### Issue 5: BPT Restoration Error (Current)

**Error:**
```
Error: load snapshot: failed to restore database: update BPT: update BPT entry for acc://cs.acme: cannot modify account - observer is not set
```

**Root Cause:** BPT (Binary Patricia Tree) restoration issue with snapshot.

**Status:** Under investigation - requires BPT restoration strategy implementation.

## Complete Node Startup Workflow

### Step 1: Initialize Network Structure
```bash
# Create network structure with init network command
./accumulated init network cyclops-init-network.json --work-dir .accumulate
```

### Step 2: Generate Missing Node Key
```bash
# Generate node key for the created partition directory
./analyze generate-node-key .accumulate/bvn1-1/config/node_key.json
```

### Step 3: Fix Configuration File
Create proper `accumulate.toml`:
```bash
cat > .accumulate/bvn1-1/config/accumulate.toml << 'EOF'
[describe]
  type = "blockValidator"
  partition-id = "bvn-cyclops"

[storage]
  type = "leveldb"
  path = "data/accumulate.db"

[network]
  id = "cyclops"

[logging]
  level = "info"
EOF
```

### Step 4: Restore Partition Snapshot
```bash
# Restore BVN partition snapshot to the node directory
./accumulated restore-snapshot ../partition-snapshots/bvn-cyclops-partition.snap --work-dir .accumulate/bvn1-1
```

### Step 5: Start Node
```bash
# Start the validator node
./accumulated run --work-dir .accumulate/bvn1-1
```

## File Structure After Setup
```
.accumulate/bvn1-1/
├── config/
│   ├── accumulate.toml          # Main configuration
│   ├── node_key.json           # P2P node key (600 perms)
│   ├── priv_validator_key.json # Validator key (600 perms)
│   └── priv_validator_state.json # Validator state
└── data/
    ├── accumulate.db/          # Restored database
    └── priv_validator_state.json
```

## Validation Commands

### Check File Permissions
```bash
ls -la .accumulate/bvn1-1/config/
# node_key.json and priv_validator_key.json should be 600
```

### Validate Configuration
```bash
# Check TOML syntax
./accumulated config validate --work-dir .accumulate/bvn1-1
```

### Check Database
```bash
# Verify database was restored
ls -la .accumulate/bvn1-1/data/accumulate.db/
```

## Key Configuration Values

### Partition Types
- `"directory"` - Directory Network nodes
- `"blockValidator"` - Block Validator Network nodes
- `"blockSummary"` - Block Summary nodes
- `"bootstrap"` - Bootstrap nodes

### Storage Types
- `"leveldb"` - LevelDB storage (recommended)
- `"badger"` - Badger storage (alternative)

### Network IDs
- `"cyclops"` - Cyclops testnet
- `"mainnet"` - Mainnet (future)

## Troubleshooting Tips

1. **Always check file permissions** - Private keys must be 600
2. **Validate TOML syntax** - Use proper sections without conflicts
3. **Check partition IDs** - Must match network configuration
4. **Verify snapshots exist** - Ensure partition snapshots are available
5. **Monitor logs** - Use `--log-level debug` for detailed output

## Next Steps
- Resolve BPT restoration issue
- Test complete node startup
- Document multi-node deployment
- Create automation scripts

## See Also
- [Cyclops Network Configuration](cyclops-network-configuration.md)
- [Node Directory Design](cyclops-node-directory-design.md)
- [BPT Restoration Design](../technical/bpt-restoration-design.md)
