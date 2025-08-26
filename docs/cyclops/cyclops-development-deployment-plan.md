# Cyclops Development Deployment Plan

**Status**: 🚧 **DEVELOPMENT SPECIFICATION** - New 4-phase approach with isolated test environment  
**Created**: 2025-07-08 02:02 CDT  
**Purpose**: Define clean development deployment process with no file links and proper isolation

---

## Overview

This plan defines a robust 4-phase deployment process for Cyclops validator nodes that:
- Uses completely isolated test environment (`/tmp/cyclops/`)
- Makes full file copies (no symbolic links)
- Prevents unintended corruption of source artifacts
- Provides clean separation between artifact preparation and node deployment
- Enables repeatable testing and development

## 4-Phase Development Process

### Phase 0: Environment Setup
- **Purpose**: Create isolated test environment with complete file independence
- **Location**: `/tmp/cyclops/` directory
- **Key Features**: No file links, corruption prevention, restart capability
- **Documentation**: [Phase 0: Environment Setup](phase0-environment-setup.md)
- **Testing**: `phase0-restart-tests.sh` script

### Phase 1: Artifact Preparation
- **Purpose**: Prepare network configuration and validator keys
- **Key Operations**: Key generation, network JSON updates, consensus creation

### Phase 2: Node Deployment
- **Purpose**: Create node directory structure and deploy artifacts
- **Key Operations**: Directory creation, file deployment, configuration setup

### Phase 3: Node Launch
- **Purpose**: Start validator nodes and verify operation
- **Key Operations**: Node startup, health checks, consensus validation

## Architecture Principles

### 1. **Complete Isolation**
- All development work happens in `/tmp/cyclops/`
- Source artifacts in `~/accumulate-network/artifacts/` remain untouched
- No cross-contamination between test runs

### 2. **No File Links**
- All files are copied, never linked
- Large partition snapshots are copied despite size
- Prevents corruption of source files
- Ensures complete independence

### 3. **Clear Separation of Concerns**
- **Phase 0**: Environment setup and artifact staging
- **Phase 1**: Artifact preparation and generation
- **Phase 2**: Node structure creation and file deployment
- **Phase 3**: Node launch and validation

### 4. **Repeatable Process**
- Each phase can be run independently
- Clean slate on every deployment
- Comprehensive validation at each step

---

## Phase 0: Environment Setup and Artifact Staging

### Purpose
Create isolated test environment and stage all required artifacts for development.

### Operations

#### 1. Environment Cleanup
```bash
# Remove any existing test environment
rm -rf /tmp/cyclops
```

#### 2. Directory Structure Creation
```bash
# Create base test environment
mkdir -p /tmp/cyclops/artifacts
mkdir -p /tmp/cyclops/node
```

#### 3. Artifact Staging
Copy all required files from source to isolated test environment:

**Core Binaries**:
```bash
cp ~/accumulate-network/artifacts/accumulated /tmp/cyclops/artifacts/
cp ~/accumulate-network/artifacts/analyze /tmp/cyclops/artifacts/
```

**Network Configuration**:
```bash
cp ~/accumulate-network/artifacts/cyclops-network.json /tmp/cyclops/artifacts/
```

**Genesis Snapshots** (Large Files - Full Copy Required):
```bash
cp ~/accumulate-network/artifacts/cyclops-genesis.snap /tmp/cyclops/artifacts/
cp ~/accumulate-network/artifacts/Directory-partition.snap /tmp/cyclops/artifacts/
cp ~/accumulate-network/artifacts/bvn-cyclops-partition.snap /tmp/cyclops/artifacts/
```

**Existing Validator Keys** (if present):
```bash
cp ~/accumulate-network/artifacts/priv_validator_key_*.json /tmp/cyclops/artifacts/ 2>/dev/null || true
```

**Configuration Templates**:
```bash
cp ~/accumulate-network/artifacts/accumulate-template-*.toml /tmp/cyclops/artifacts/ 2>/dev/null || true
cp ~/accumulate-network/artifacts/config-template-*.toml /tmp/cyclops/artifacts/ 2>/dev/null || true
```

**Consensus Sections** (if present):
```bash
cp ~/accumulate-network/artifacts/consensus_*.json /tmp/cyclops/artifacts/ 2>/dev/null || true
```

#### 4. Permissions Setup
```bash
# Ensure binaries are executable
chmod +x /tmp/cyclops/artifacts/accumulated
chmod +x /tmp/cyclops/artifacts/analyze

# Secure validator keys
chmod 600 /tmp/cyclops/artifacts/priv_validator_key_*.json 2>/dev/null || true
```

#### 5. Validation
```bash
# Verify all critical files are present
ls -la /tmp/cyclops/artifacts/accumulated
ls -la /tmp/cyclops/artifacts/analyze
ls -la /tmp/cyclops/artifacts/cyclops-network.json
ls -la /tmp/cyclops/artifacts/*.snap

# Verify directory structure
tree /tmp/cyclops/
```

### Success Criteria
- ✅ `/tmp/cyclops/artifacts/` contains all required files
- ✅ `/tmp/cyclops/node/` exists and is empty
- ✅ All binaries are executable
- ✅ All validator keys have 600 permissions
- ✅ Large snapshot files are fully copied (no links)

---

## Phase 1: Artifact Preparation and Generation

### Purpose
Generate and prepare all artifacts needed for Cyclops validator deployment within the isolated environment.

### Operations

#### 1. Validator Key Generation
Generate Ed25519 validator keys for both partitions if not already present:

```bash
cd /tmp/cyclops/artifacts

# Generate Directory Node validator key
./analyze generate-key acc://defidevs.acme/dn ./
mv priv_validator_key.json priv_validator_key_defidevs-acme_dn.json

# Generate BVN validator key  
./analyze generate-key acc://defidevs.acme/bvn0 ./
mv priv_validator_key.json priv_validator_key_defidevs-acme_bvn0.json

# Secure permissions
chmod 600 priv_validator_key_*.json
```

#### 2. Network Configuration Update
Update network JSON with generated validator public keys:

```bash
# Update network configuration with validator keys
./analyze update-network-keys cyclops-network.json ./

# Validate network structure
jq '.network.partitions | length' cyclops-network.json  # Should be 2
jq '.validators[0].publicKey' cyclops-network.json      # Should not be null
```

#### 3. Consensus Section Generation
Create consensus sections for each partition:

```bash
# Generate Directory consensus section
./analyze generate-consensus-section cyclops-network.json Directory consensus_dn.json

# Generate BVN consensus section
./analyze generate-consensus-section cyclops-network.json bvn-cyclops consensus_bvn0.json

# Validate consensus files
jq '.validators | length' consensus_dn.json    # Should be 1
jq '.validators | length' consensus_bvn0.json  # Should be 1
```

#### 4. Partition Snapshot Extraction
Extract partition-specific snapshots with embedded consensus:

```bash
# Extract Directory partition snapshot
./analyze extract cyclops-genesis.snap Directory Directory-partition.snap

# Extract BVN partition snapshot  
./analyze extract cyclops-genesis.snap bvn-cyclops bvn-cyclops-partition.snap

# Validate extracted snapshots
ls -lh *-partition.snap  # Should show both files with reasonable sizes
```

#### 5. Configuration Template Generation
Generate TOML configuration templates:

```bash
# Generate accumulate.toml templates for each partition type
# Directory Node template
cat > accumulate-template-dn.toml << 'EOF'
[describe]
  type = "directory"
  partition-id = "Directory"

[network]
  id = "cyclops"

[storage]
  type = "leveldb"
  path = "data/accumulate.db"
EOF

# Block Validator Network template
cat > accumulate-template-bvn.toml << 'EOF'
[describe]
  type = "blockValidator"
  partition-id = "bvn-cyclops"

[network]
  id = "cyclops"

[storage]
  type = "leveldb"
  path = "data/accumulate.db"
EOF
```

#### 6. Artifact Validation
```bash
# Verify all required artifacts are present
echo "=== Artifact Validation ==="
ls -la priv_validator_key_*.json     # Validator keys
ls -la consensus_*.json              # Consensus sections
ls -la *-partition.snap              # Partition snapshots
ls -la accumulate-template-*.toml    # Configuration templates
ls -la cyclops-network.json          # Updated network config

# Validate file integrity
jq empty cyclops-network.json       # JSON syntax check
jq empty consensus_*.json            # Consensus syntax check
```

### Success Criteria
- ✅ Validator keys generated for both partitions
- ✅ Network JSON updated with public keys
- ✅ Consensus sections created and validated
- ✅ Partition snapshots extracted successfully
- ✅ Configuration templates generated
- ✅ All artifacts pass validation checks

---

## Phase 2: Node Structure Creation and File Deployment

### Purpose
Create the complete Cyclops validator node directory structure and deploy all artifacts to their correct locations.

### Operations

#### 1. Node Directory Structure Creation
Create the dual-node directory structure for Cyclops validator:

```bash
cd /tmp/cyclops/node

# Create base .accumulate directory structure
mkdir -p .accumulate/config
mkdir -p .accumulate/data

# Create Directory Node partition structure
mkdir -p .accumulate/dn/config
mkdir -p .accumulate/dn/data

# Create BVN partition structure
mkdir -p .accumulate/bvn-cyclops/config
mkdir -p .accumulate/bvn-cyclops/data
```

#### 2. Global Configuration Deployment
Deploy global configuration files:

```bash
# Copy global accumulate.toml (using DN template as base)
cp /tmp/cyclops/artifacts/accumulate-template-dn.toml .accumulate/config/accumulate.toml

# Generate node key for P2P networking
cd /tmp/cyclops/artifacts
./analyze generate-node-key
cp node_key.json /tmp/cyclops/node/.accumulate/config/
chmod 600 /tmp/cyclops/node/.accumulate/config/node_key.json

# Generate CometBFT config.toml
cd /tmp/cyclops/node
../artifacts/accumulated init --work-dir .accumulate
```

#### 3. Directory Node Partition Deployment
Deploy Directory Node specific files:

```bash
# Copy Directory validator key
cp /tmp/cyclops/artifacts/priv_validator_key_defidevs-acme_dn.json \
   .accumulate/dn/config/priv_validator_key.json
chmod 600 .accumulate/dn/config/priv_validator_key.json

# Copy Directory partition snapshot (FULL COPY - NO LINKS)
cp /tmp/cyclops/artifacts/Directory-partition.snap \
   .accumulate/dn/data/Directory-partition.snap

# Create Directory-specific accumulate.toml
cp /tmp/cyclops/artifacts/accumulate-template-dn.toml \
   .accumulate/dn/config/accumulate.toml
```

#### 4. BVN Partition Deployment
Deploy BVN specific files:

```bash
# Copy BVN validator key
cp /tmp/cyclops/artifacts/priv_validator_key_defidevs-acme_bvn0.json \
   .accumulate/bvn-cyclops/config/priv_validator_key.json
chmod 600 .accumulate/bvn-cyclops/config/priv_validator_key.json

# Copy BVN partition snapshot (FULL COPY - NO LINKS)
cp /tmp/cyclops/artifacts/bvn-cyclops-partition.snap \
   .accumulate/bvn-cyclops/data/bvn-cyclops-partition.snap

# Create BVN-specific accumulate.toml
cp /tmp/cyclops/artifacts/accumulate-template-bvn.toml \
   .accumulate/bvn-cyclops/config/accumulate.toml
```

#### 5. Database Initialization
Initialize databases for both partitions:

```bash
cd /tmp/cyclops/node

# Initialize Directory Node database
../artifacts/accumulated init database --work-dir .accumulate/dn

# Initialize BVN database
../artifacts/accumulated init database --work-dir .accumulate/bvn-cyclops
```

#### 6. File Duplication Handling
Handle intentional file duplications (no links allowed):

```bash
# If validator keys need to be in multiple locations, copy them
# Example: Global validator key copies
cp .accumulate/dn/config/priv_validator_key.json \
   .accumulate/config/priv_validator_key_dn.json 2>/dev/null || true

cp .accumulate/bvn-cyclops/config/priv_validator_key.json \
   .accumulate/config/priv_validator_key_bvn.json 2>/dev/null || true

# Maintain proper permissions on all copies
chmod 600 .accumulate/config/priv_validator_key_*.json 2>/dev/null || true
```

#### 7. Structure Validation
```bash
# Validate complete directory structure
echo "=== Node Structure Validation ==="
tree .accumulate/

# Validate file permissions
echo "=== Permission Validation ==="
ls -la .accumulate/*/config/priv_validator_key.json

# Validate file sizes (snapshots should be large)
echo "=== File Size Validation ==="
ls -lh .accumulate/*/data/*.snap

# Validate configuration files
echo "=== Configuration Validation ==="
../artifacts/accumulated --work-dir .accumulate/dn --check-config
../artifacts/accumulated --work-dir .accumulate/bvn-cyclops --check-config
```

### Success Criteria
- ✅ Complete dual-node directory structure created
- ✅ All validator keys deployed with 600 permissions
- ✅ Partition snapshots copied (no links) to correct locations
- ✅ Configuration files deployed to all required locations
- ✅ Databases initialized successfully
- ✅ All file duplications handled without links
- ✅ Structure validation passes
- ✅ Configuration validation passes

---

## Phase 3: Node Launch and Validation

### Purpose
Launch the Cyclops validator node and validate successful operation.

### Operations

#### 1. Pre-Launch Validation
Comprehensive validation before node startup:

```bash
cd /tmp/cyclops/node

# Validate all required files exist
echo "=== Pre-Launch File Check ==="
test -f .accumulate/config/accumulate.toml || echo "❌ Missing global config"
test -f .accumulate/config/node_key.json || echo "❌ Missing node key"
test -f .accumulate/dn/config/priv_validator_key.json || echo "❌ Missing DN validator key"
test -f .accumulate/bvn-cyclops/config/priv_validator_key.json || echo "❌ Missing BVN validator key"
test -f .accumulate/dn/data/Directory-partition.snap || echo "❌ Missing DN snapshot"
test -f .accumulate/bvn-cyclops/data/bvn-cyclops-partition.snap || echo "❌ Missing BVN snapshot"

# Validate configuration syntax
echo "=== Configuration Syntax Check ==="
../artifacts/accumulated --work-dir .accumulate --check-config

# Validate network connectivity prerequisites
echo "=== Network Port Check ==="
netstat -tuln | grep -E ':(26656|26657|26658|36656|36657|36658)' || echo "ℹ Ports available"
```

#### 2. Database Restoration
Restore partition snapshots to databases:

```bash
# Restore Directory Node snapshot
echo "=== Restoring Directory Node Database ==="
../artifacts/accumulated restore-snapshot \
  --work-dir .accumulate/dn \
  .accumulate/dn/data/Directory-partition.snap

# Restore BVN snapshot
echo "=== Restoring BVN Database ==="
../artifacts/accumulated restore-snapshot \
  --work-dir .accumulate/bvn-cyclops \
  .accumulate/bvn-cyclops/data/bvn-cyclops-partition.snap
```

#### 3. Node Launch
Launch the Cyclops validator node:

```bash
# Launch node in background with logging
echo "=== Launching Cyclops Validator Node ==="
../artifacts/accumulated run \
  --work-dir .accumulate \
  --log-level info \
  > cyclops-node.log 2>&1 &

# Capture process ID
NODE_PID=$!
echo "Node launched with PID: $NODE_PID"
```

#### 4. Startup Monitoring
Monitor node startup for 60 seconds:

```bash
echo "=== Monitoring Node Startup ==="
for i in {1..12}; do
  sleep 5
  
  # Check if process is still running
  if ! kill -0 $NODE_PID 2>/dev/null; then
    echo "❌ Node process died"
    tail -20 cyclops-node.log
    exit 1
  fi
  
  # Check for successful startup indicators
  if grep -q "Started node" cyclops-node.log; then
    echo "✅ Node startup detected"
    break
  fi
  
  echo "⏳ Waiting for startup... ($((i*5))s)"
done
```

#### 5. Health Validation
Validate node health and connectivity:

```bash
# Test RPC endpoints
echo "=== RPC Endpoint Validation ==="
curl -s http://localhost:26657/status | jq '.result.node_info.network' || echo "❌ RPC not responding"

# Test validator status
echo "=== Validator Status Check ==="
curl -s http://localhost:26657/validators | jq '.result.validators | length' || echo "❌ Validator query failed"

# Check consensus participation
echo "=== Consensus Participation Check ==="
curl -s http://localhost:26657/consensus_state | jq '.result.round_state.height' || echo "❌ Consensus query failed"
```

#### 6. Operational Commands Setup
Provide operational commands for node management:

```bash
echo "=== Node Management Commands ==="
echo "Node PID: $NODE_PID"
echo ""
echo "Monitor logs:"
echo "  tail -f /tmp/cyclops/node/cyclops-node.log"
echo ""
echo "Check status:"
echo "  curl -s http://localhost:26657/status | jq"
echo ""
echo "Stop node:"
echo "  kill $NODE_PID"
echo ""
echo "Restart node:"
echo "  cd /tmp/cyclops/node && ../artifacts/accumulated run --work-dir .accumulate &"
```

### Success Criteria
- ✅ Pre-launch validation passes
- ✅ Database restoration completes successfully
- ✅ Node launches without errors
- ✅ Process remains stable for 60 seconds
- ✅ RPC endpoints respond correctly
- ✅ Validator status queries work
- ✅ Consensus participation detected
- ✅ Operational commands provided

---

## Development Insights from Validator Configuration

### Critical Issues and Solutions

#### 1. Extract Command Memory Allocation Failure
**CRITICAL BUG**: The `analyze extract` command fails with `panic: runtime error: makeslice: len out of range` during snapshot processing at line 121 in `/pkg/database/snapshot/encoding.go`.

**Root Cause**: Memory allocation issue when processing large snapshot sections (1.4GB+ partition snapshots).

**Solution for Phase 1**: 
- Skip the extract step entirely if existing partition snapshots are available
- Use pre-existing `Directory-partition.snap` and `bvn-cyclops-partition.snap` files
- Proceed directly to Phase 2 deployment with existing artifacts
- The consensus sections are already generated and validated

**Implementation**:
```bash
# Check if partition snapshots already exist
if [[ -f "Directory-partition.snap" && -f "bvn-cyclops-partition.snap" ]]; then
    echo "✅ Using existing partition snapshots (skipping extract due to memory issues)"
else
    echo "⚠️  Extract command may fail on large snapshots - consider using pre-built snapshots"
fi
```

#### 2. BPT Restoration Strategy
**Issue**: "cannot modify account - observer is not set" during BPT restoration

**Root Cause**: The `database.Restore()` function calls `batch.UpdateBPT()` to rebuild the BPT from restored accounts, which requires a database observer.

**Solution**: Set database observer before restoration:
```go
// Set database observer for BPT rebuilding during restoration
db.SetObserver(execute.NewDatabaseObserver())

// Use FullRestoreWithOptions - BPT sections are automatically skipped and rebuilt from accounts
err = snapshot.FullRestoreWithOptions(db, file, d.Logger, d.Config.Accumulate.Describe.PartitionUrl(), nil)
```

**BPT Design Principles**:
1. Always ignore missing BPT sections (log warning, don't fail)
2. Always rebuild BPT from all accounts (ensures consistency)
3. Only validate root hash (simple, complete, reliable validation)
4. Handle zero root hash gracefully (normal for partition snapshots)

#### 3. Partition Type Configuration
**CRITICAL**: The most critical configuration error was incorrect partition type specification.

**Correct Configuration**:
```toml
[describe]
  type = "blockValidator"     # For BVN nodes (NOT "directory")
  partition-id = "bvn-cyclops"

# Must be under [describe] section - NOT at root level
```

**Common Error**: Using `type = "directory"` for Cyclops validators causes "unknown partition type PartitionType:0" error.

#### 4. Ed25519 Key Handling
**Issue**: Ed25519 private keys contain 64 bytes (32-byte seed + 32-byte public key), but some functions expect only the 32-byte seed.

**Solution**: Proper length checking and seed extraction:
```go
if len(privKeyBytes) == 64 {
    // Extract 32-byte seed from 64-byte private key
    seed := privKeyBytes[:32]
    key := ed25519.NewKeyFromSeed(seed)
}
```

#### 5. Network JSON Structure Corruption
**CRITICAL**: The `update-network-keys` command was using incomplete struct definition, corrupting network JSON.

**Root Cause**: Missing fields in NetworkConfig struct caused data loss during JSON marshaling.

**Solution**: Complete NetworkConfig struct preserving all fields:
```go
type NetworkConfig struct {
    ID        string      `json:"id"`
    Template  interface{} `json:"template"`
    Oracle    interface{} `json:"oracle"`
    Globals   struct {
        Globals interface{} `json:"globals"`
        Network struct {
            NetworkName string `json:"networkName"`
            Partitions  []interface{} `json:"partitions"`
            Validators  []struct {
                PublicKeyHash string `json:"publicKeyHash"`
                Partitions    []interface{} `json:"partitions"`
            } `json:"validators"`
        } `json:"network"`
        Routing interface{} `json:"routing"`
    } `json:"globals"`
    BVNs interface{} `json:"bvns"`
    DN   interface{} `json:"dn"`
}
```

#### 6. Consensus Generation Format Issues
**Issue**: Mixed base64/hex encoding causing decode errors in consensus generation.

**Solution**: Proper format handling:
- `update-network-keys`: base64 → hex (for storage)
- `generate-consensus-section`: hex → base64 (for CometBFT)

#### 7. File Permissions Security
**Requirement**: Private validator keys must have 600 permissions for security.

**Implementation**: All validator key files must be set to 600 permissions after copying:
```bash
chmod 600 /path/to/priv_validator_key.json
```

### Known Issues and Mitigations

#### 1. Routing Configuration Panics
**Issue**: Missing routing table values cause panic in buildPrefixTree()  
**Mitigation**: Ensure all routing entries have proper "value" fields:
```json
{
  "routes": [
    {"length": 1, "value": 0, "partition": "bvn-cyclops"}
  ]
}
```

#### 2. Memory Issues with Large Snapshots
**Issue**: Extract command fails on snapshots >1GB due to memory allocation
**Mitigation**: Use pre-built partition snapshots, skip extract step

#### 3. Command Interface Consistency
**Issue**: Required inputs implemented as flags instead of positional arguments
**Solution**: All analyze commands now use positional arguments:
```bash
./analyze update-network-keys cyclops-network.json artifacts/
./analyze generate-consensus-section cyclops-network.json bvn-cyclops consensus.json
```

---

## File Size Considerations

### Large Files That Must Be Copied
- **cyclops-genesis.snap**: ~2.0GB
- **Directory-partition.snap**: ~1.3GB  
- **bvn-cyclops-partition.snap**: ~1.4GB
- **Total**: ~4.7GB of snapshot data

### Disk Space Requirements
- **Source artifacts**: ~5GB
- **Test environment**: ~10GB (includes copies and databases)
- **Recommended**: 15GB free space for safe operation

### Copy Performance
- **SSD**: ~30 seconds for full copy
- **HDD**: ~2-3 minutes for full copy
- **Network**: Varies by connection speed

---

## Validation and Testing Strategy

### Phase Validation
Each phase includes comprehensive validation:
- **Phase 0**: File presence and permissions
- **Phase 1**: Artifact generation and JSON syntax
- **Phase 2**: Directory structure and configuration syntax
- **Phase 3**: Node startup and operational health

### Error Handling
- **Fail Fast**: Stop on critical errors
- **Graceful Degradation**: Continue on non-critical issues with warnings
- **Comprehensive Logging**: All operations logged for debugging

### Repeatability
- **Clean Slate**: Each run starts with fresh environment
- **Idempotent**: Phases can be re-run safely
- **Isolated**: No impact on source artifacts

---

## Success Metrics

### Deployment Success
- ✅ All phases complete without critical errors
- ✅ Node launches and remains stable
- ✅ RPC endpoints respond correctly
- ✅ Validator participates in consensus

### Development Success  
- ✅ Repeatable deployment process
- ✅ No corruption of source artifacts
- ✅ Clear separation of concerns
- ✅ Comprehensive validation at each step

### Operational Success
- ✅ Node management commands work
- ✅ Monitoring and logging functional
- ✅ Recovery procedures documented
- ✅ Performance meets expectations

---

## Next Steps

1. **Implement Phase 0 Script**: Create automated environment setup
2. **Implement Phase 1 Script**: Automate artifact preparation
3. **Implement Phase 2 Script**: Automate node structure creation
4. **Implement Phase 3 Script**: Automate node launch and validation
5. **Integration Testing**: Test complete 4-phase workflow
6. **Documentation**: Create operational runbooks
7. **Monitoring**: Implement comprehensive health checks

This development deployment plan provides a robust foundation for Cyclops validator node development and testing with complete isolation and no file link dependencies.
