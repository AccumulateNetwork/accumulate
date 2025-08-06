# Cyclops Dual Node Deployment Workflow

## Complete Guide to Dual Snapshot Restoration and Node Deployment

**Date**: 2025-07-07 04:45 CDT  
**Version**: 2.0 - Updated with partition-specific restoration  
**Status**: ✅ **PRODUCTION READY**

---

## Overview

This guide provides the complete workflow for deploying a Cyclops validator node with proper dual snapshot restoration. Based on critical architectural analysis, this workflow ensures correct partition isolation and database restoration.

## Prerequisites

### System Requirements
- Linux system (Ubuntu/Debian recommended)
- Go 1.19+ installed
- Git for repository access
- Minimum 50GB disk space
- Network connectivity for P2P

### Required Artifacts
- `accumulated` binary (built from source)
- `cyclops-network.json` (network configuration)
- `Directory-partition.snap` (DN genesis snapshot)
- `bvn-cyclops-partition.snap` (BVN genesis snapshot)
- Validator keys for both partitions

## Architecture Understanding

### Dual Node Structure
```
work-dir/
├── dnn/                      # Directory Network Node
│   ├── config/
│   │   ├── accumulate.toml   # DN configuration
│   │   ├── config.toml       # DN Tendermint config
│   │   └── priv_validator_key.json
│   └── data/
│       ├── accumulate.db/    # DN database
│       └── priv_validator_state.json
└── bvnn/                     # Block Validator Network Node
    ├── config/
    │   ├── accumulate.toml   # BVN configuration
    │   ├── config.toml       # BVN Tendermint config
    │   └── priv_validator_key.json
    └── data/
        ├── accumulate.db/    # BVN database
        └── priv_validator_state.json
```

### Key Architectural Principles
1. **Partition Isolation**: Each partition has completely separate database and config
2. **Snapshot Specificity**: Each snapshot restores to exactly one partition database
3. **Work-Dir Targeting**: Restoration requires partition-specific work-dir paths
4. **Database Independence**: DN and BVN databases are completely isolated

## Step-by-Step Deployment Workflow

### Phase 1: Environment Setup

#### 1.1 Create Deployment Directory
```bash
mkdir -p /tmp/cyclops/node/artifacts
cd /tmp/cyclops/node/artifacts
```

#### 1.2 Build Accumulate Binary
```bash
# Clone and build (if not already done)
git clone https://gitlab.com/AccumulateNetwork/accumulate.git
cd accumulate
go build -o accumulated ./cmd/accumulated
cp accumulated /tmp/cyclops/node/artifacts/
cd /tmp/cyclops/node/artifacts
```

#### 1.3 Verify Required Artifacts
```bash
# Check all required files exist
ls -la accumulated                    # Binary
ls -la cyclops-network.json         # Network config
ls -la Directory-partition.snap     # DN snapshot
ls -la bvn-cyclops-partition.snap   # BVN snapshot
```

### Phase 2: Dual Node Initialization

#### 2.1 Initialize Dual Node Structure
```bash
# Initialize dual node with both DN and BVN partitions
./accumulated init dual cyclops.Directory cyclops.bvn-cyclops \
    --work-dir "$PWD" \
    --network cyclops-network.json
```

**What this creates**:
- `dnn/` directory with DN configuration and keys
- `bvnn/` directory with BVN configuration and keys
- Separate Tendermint configs for each partition
- Validator keys for both partitions

#### 2.2 Verify Initialization
```bash
# Check dual node structure was created
tree dnn/ bvnn/

# Verify configs exist
cat dnn/config/accumulate.toml
cat bvnn/config/accumulate.toml
```

### Phase 3: Partition-Specific Snapshot Restoration

#### 3.1 Restore Directory Network Snapshot
```bash
echo "Restoring Directory partition snapshot..."

# CRITICAL: Use dnn work-dir for DN snapshot
./accumulated restore-snapshot "Directory-partition.snap" \
    --work-dir "$PWD/dnn"

# Verify DN database was created
ls -la dnn/data/accumulate.db/
```

#### 3.2 Restore Block Validator Network Snapshot
```bash
echo "Restoring BVN partition snapshot..."

# CRITICAL: Use bvnn work-dir for BVN snapshot
./accumulated restore-snapshot "bvn-cyclops-partition.snap" \
    --work-dir "$PWD/bvnn"

# Verify BVN database was created
ls -la bvnn/data/accumulate.db/
```

#### 3.3 Validate Snapshot Restoration
```bash
# Check both databases exist and have content
du -sh dnn/data/accumulate.db/
du -sh bvnn/data/accumulate.db/

# Verify no cross-contamination
echo "DN partition config:"
grep "partition-id" dnn/config/accumulate.toml

echo "BVN partition config:"
grep "partition-id" bvnn/config/accumulate.toml
```

### Phase 4: Configuration Validation

#### 4.1 Verify DN Configuration
```bash
cat dnn/config/accumulate.toml
```

**Expected DN Config**:
```toml
[describe]
  type = "directory"
  partition-id = "Directory"

[network]
  id = "cyclops"

[storage]
  type = "leveldb"
  path = "data/accumulate.db"
```

#### 4.2 Verify BVN Configuration
```bash
cat bvnn/config/accumulate.toml
```

**Expected BVN Config**:
```toml
[describe]
  type = "blockValidator"
  partition-id = "bvn-cyclops"

[network]
  id = "cyclops"

[storage]
  type = "leveldb"
  path = "data/accumulate.db"
```

#### 4.3 Verify Key Files
```bash
# Check validator keys exist with correct permissions
ls -la dnn/config/priv_validator_key.json
ls -la bvnn/config/priv_validator_key.json

# Verify permissions (should be 600)
stat -c "%a" dnn/config/priv_validator_key.json
stat -c "%a" bvnn/config/priv_validator_key.json
```

### Phase 5: Node Startup

#### 5.1 Start Dual Node
```bash
# Start dual node with parent work-dir
./accumulated run --work-dir "$PWD"
```

**Important**: The startup work-dir points to the parent directory containing both `dnn/` and `bvnn/` subdirectories, allowing the dual node process to access both partition configurations.

#### 5.2 Monitor Startup Logs
```bash
# In another terminal, monitor logs
tail -f dnn/logs/accumulate.log
tail -f bvnn/logs/accumulate.log
```

#### 5.3 Verify Node Status
```bash
# Check node is running both partitions
curl http://localhost:26657/status
curl http://localhost:26658/status  # BVN port
```

## Validation Commands

### Complete Structure Validation
```bash
#!/bin/bash
# validate-dual-node.sh

echo "=== Dual Node Structure Validation ==="

# Check directories
REQUIRED_DIRS=(
    "dnn"
    "dnn/config"
    "dnn/data"
    "bvnn"
    "bvnn/config"
    "bvnn/data"
)

for dir in "${REQUIRED_DIRS[@]}"; do
    if [ -d "$dir" ]; then
        echo "✅ Directory exists: $dir"
    else
        echo "❌ Missing directory: $dir"
    fi
done

# Check configuration files
REQUIRED_CONFIGS=(
    "dnn/config/accumulate.toml"
    "dnn/config/config.toml"
    "bvnn/config/accumulate.toml"
    "bvnn/config/config.toml"
)

for config in "${REQUIRED_CONFIGS[@]}"; do
    if [ -f "$config" ]; then
        echo "✅ Config exists: $config"
    else
        echo "❌ Missing config: $config"
    fi
done

# Check databases
REQUIRED_DBS=(
    "dnn/data/accumulate.db"
    "bvnn/data/accumulate.db"
)

for db in "${REQUIRED_DBS[@]}"; do
    if [ -d "$db" ]; then
        size=$(du -sh "$db" | cut -f1)
        echo "✅ Database exists: $db ($size)"
    else
        echo "❌ Missing database: $db"
    fi
done

# Check validator keys
REQUIRED_KEYS=(
    "dnn/config/priv_validator_key.json"
    "bvnn/config/priv_validator_key.json"
)

for key in "${REQUIRED_KEYS[@]}"; do
    if [ -f "$key" ]; then
        perms=$(stat -c "%a" "$key")
        if [ "$perms" = "600" ]; then
            echo "✅ Key exists with correct permissions: $key ($perms)"
        else
            echo "⚠️  Key exists but wrong permissions: $key ($perms, should be 600)"
        fi
    else
        echo "❌ Missing key: $key"
    fi
done

echo "=== Validation Complete ==="
```

### Network Connectivity Test
```bash
# Test P2P connectivity
./accumulated network status --work-dir "$PWD"

# Test API endpoints
curl -s http://localhost:26657/status | jq '.result.node_info'
curl -s http://localhost:26658/status | jq '.result.node_info'
```

## Troubleshooting Guide

### Common Issues and Solutions

#### 1. "Database not found" Error
**Symptoms**: Node fails to start with database path errors
**Cause**: Incorrect work-dir during snapshot restoration
**Solution**:
```bash
# Verify snapshots were restored to correct locations
ls -la dnn/data/accumulate.db/
ls -la bvnn/data/accumulate.db/

# If missing, re-run restoration with correct work-dirs
./accumulated restore-snapshot "Directory-partition.snap" --work-dir "$PWD/dnn"
./accumulated restore-snapshot "bvn-cyclops-partition.snap" --work-dir "$PWD/bvnn"
```

#### 2. "Unknown partition type PartitionType:0" Error
**Symptoms**: Node startup fails with partition type error
**Cause**: Incorrect configuration structure
**Solution**:
```bash
# Check partition type is in [describe] section
grep -A 5 "\[describe\]" dnn/config/accumulate.toml
grep -A 5 "\[describe\]" bvnn/config/accumulate.toml

# Should show:
# [describe]
#   type = "directory"        # for DN
#   type = "blockValidator"   # for BVN
```

#### 3. Cross-Partition Database Contamination
**Symptoms**: Wrong partition data in database
**Cause**: Snapshots restored to wrong partitions
**Solution**:
```bash
# Remove contaminated databases
rm -rf dnn/data/accumulate.db/
rm -rf bvnn/data/accumulate.db/

# Restore snapshots to correct partitions
./accumulated restore-snapshot "Directory-partition.snap" --work-dir "$PWD/dnn"
./accumulated restore-snapshot "bvn-cyclops-partition.snap" --work-dir "$PWD/bvnn"
```

#### 4. Permission Errors on Validator Keys
**Symptoms**: "Permission denied" errors during startup
**Cause**: Incorrect file permissions on validator keys
**Solution**:
```bash
# Fix validator key permissions
chmod 600 dnn/config/priv_validator_key.json
chmod 600 bvnn/config/priv_validator_key.json
chmod 600 dnn/config/node_key.json
chmod 600 bvnn/config/node_key.json
```

## Performance Considerations

### Resource Requirements
- **CPU**: Minimum 4 cores (dual partition processing)
- **RAM**: Minimum 8GB (separate databases)
- **Disk**: Minimum 50GB SSD (database growth)
- **Network**: Stable connection for P2P sync

### Database Sizing
```bash
# Monitor database growth
watch -n 60 'du -sh dnn/data/accumulate.db/ bvnn/data/accumulate.db/'

# Check database health
./accumulated db analyze --work-dir "$PWD/dnn"
./accumulated db analyze --work-dir "$PWD/bvnn"
```

## Security Best Practices

### File Permissions
```bash
# Secure validator keys
chmod 600 dnn/config/priv_validator_key.json
chmod 600 bvnn/config/priv_validator_key.json
chmod 600 dnn/config/node_key.json
chmod 600 bvnn/config/node_key.json

# Secure configuration directories
chmod 700 dnn/config/
chmod 700 bvnn/config/
```

### Backup Procedures
```bash
# Backup validator keys
cp dnn/config/priv_validator_key.json ~/backup/dn_validator_key.json.backup
cp bvnn/config/priv_validator_key.json ~/backup/bvn_validator_key.json.backup

# Backup configurations
tar -czf ~/backup/cyclops_configs_$(date +%Y%m%d).tar.gz dnn/config/ bvnn/config/
```

## Operational Procedures

### Regular Maintenance
```bash
# Check node health
./accumulated status --work-dir "$PWD"

# Monitor partition sync
curl -s http://localhost:26657/status | jq '.result.sync_info'
curl -s http://localhost:26658/status | jq '.result.sync_info'

# Check validator participation
curl -s http://localhost:26657/validators | jq '.result.validators'
```

### Log Management
```bash
# Rotate logs
logrotate -f /etc/logrotate.d/accumulate

# Monitor critical errors
tail -f dnn/logs/accumulate.log | grep -i error
tail -f bvnn/logs/accumulate.log | grep -i error
```

## Conclusion

This workflow ensures proper dual node deployment with correct partition isolation and snapshot restoration. The key insight is that snapshot restoration must be performed separately for each partition using partition-specific work directories.

Following this workflow guarantees:
- ✅ Proper database isolation between partitions
- ✅ Correct snapshot restoration to appropriate databases
- ✅ Valid configuration for both DN and BVN partitions
- ✅ Secure file permissions and operational procedures

## Quick Reference Commands

```bash
# Initialize dual node
./accumulated init dual cyclops.Directory cyclops.bvn-cyclops --work-dir "$PWD" --network cyclops-network.json

# Restore snapshots (CRITICAL: Use partition-specific work-dirs)
./accumulated restore-snapshot "Directory-partition.snap" --work-dir "$PWD/dnn"
./accumulated restore-snapshot "bvn-cyclops-partition.snap" --work-dir "$PWD/bvnn"

# Start dual node
./accumulated run --work-dir "$PWD"

# Validate structure
tree dnn/ bvnn/
du -sh dnn/data/accumulate.db/ bvnn/data/accumulate.db/
```

---

**Status**: Production ready workflow with comprehensive validation and troubleshooting procedures.
