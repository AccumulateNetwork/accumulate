# Cyclops 3-Phase Deployment Reference

**Quick Reference for Cyclops Validator Deployment**

---

## 🚀 Complete Deployment (One Command)

```bash
cd /home/paulsnow/accumulate-network/artifacts2
./deploy-cyclops-complete.sh [target-directory]
```

**This runs all 3 phases automatically with full validation and monitoring.**

---

## 📋 Individual Phase Execution

### **Phase 1: Preparation** 
**Script**: `phase1-prep.sh`  
**Purpose**: Generate and validate all deployment artifacts

```bash
./phase1-prep.sh
```

**What it does**:
- ✅ Generates Ed25519 validator keys for both partitions
- ✅ Updates network JSON with validator public keys  
- ✅ Creates consensus sections for each partition
- ✅ Extracts partition-specific snapshots with embedded consensus
- ✅ Generates proper node configuration files
- ✅ Validates all artifacts

**Outputs**:
- Updated `cyclops-network.json` with validator keys
- Partition snapshots: `Directory-partition.snap`, `bvn-cyclops-partition.snap`
- Validator keys: `priv_validator_key_defidevs-acme_*.json`
- Configuration templates: `accumulate.toml`, `config.toml`

**Duration**: ~2-3 minutes  
**Status Check**: All files present and validated

---

### **Phase 2: Deployment**
**Script**: `phase2-deploy.sh`  
**Purpose**: Deploy artifacts to target validator node directory

```bash
./phase2-deploy.sh
```

**What it does**:
- ✅ Creates proper dual-node directory structure (`.accumulate/`)
- ✅ Places artifacts in correct locations with proper permissions
- ✅ Sets up configuration files for both partitions (DN + BVN)
- ✅ Validates deployment structure

**Directory Structure Created**:
```
.accumulate/
├── config/                    # Global configuration
│   ├── accumulate.toml       # Accumulate daemon config
│   ├── config.toml           # CometBFT config  
│   └── node_key.json         # P2P networking key
├── data/                     # Global data
├── dn/                       # Directory Node partition
│   ├── config/priv_validator_key.json (600 perms)
│   └── data/Directory-partition.snap
└── bvn-cyclops/              # BVN partition
    ├── config/priv_validator_key.json (600 perms)
    └── data/bvn-cyclops-partition.snap
```

**Duration**: ~30 seconds  
**Status Check**: Directory structure validation passes

---

### **Phase 3: Launch**
**Script**: `phase3-launch.sh`  
**Purpose**: Launch the validator node with monitoring

```bash
./phase3-launch.sh
```

**What it does**:
- ✅ Pre-launch validation of all files and configuration
- ✅ Network connectivity checks (ports 26656, 26657, 26658)
- ✅ Configuration syntax validation (`--check-config`)
- ✅ Node startup with background execution
- ✅ 30-second startup monitoring with health checks
- ✅ RPC endpoint availability testing
- ✅ Operational command setup

**Monitoring Process**:
1. **Process Health**: Checks every 5 seconds for 30 seconds
2. **RPC Availability**: Tests `http://localhost:26657/status`
3. **Network Validation**: Confirms network ID = "cyclops"
4. **Block Height**: Verifies node is processing blocks

**Outputs**:
- **PID File**: `cyclops-node.pid` (process management)
- **Log File**: `cyclops-node.log` (all node output)
- **Status Report**: Network info and operational commands

**Duration**: ~2-5 minutes (includes snapshot restoration)  
**Status Check**: RPC endpoint responding, logs show successful startup

---

### **Phase 4: Validation**
**Script**: `phase4-validate.sh`  
**Purpose**: Comprehensive validation of deployed node structure

```bash
./phase4-validate.sh .accumulate
```

**What it does**:
- ✅ Validates complete directory structure
- ✅ Checks all required files are present
- ✅ Verifies file permissions (especially private keys)
- ✅ Tests configuration file syntax
- ✅ Validates JSON structure of keys and configs
- ✅ Checks file sizes and integrity

**Validation Categories**:
1. **Directory Structure**: Ensures `.accumulate/`, `dn/`, `bvn-cyclops/` hierarchy
2. **Required Files**: Validates presence of all config files and snapshots
3. **Permissions**: Confirms private keys have 600 permissions
4. **File Integrity**: Checks file sizes and JSON validity
5. **Configuration**: Tests TOML syntax and required fields

**Duration**: ~10-30 seconds  
**Status Check**: All validation tests pass with detailed report

---

## 🔍 Phase Status Validation

### **After Phase 1**
```bash
# Check all artifacts present
ls -la cyclops-network.json *.snap priv_validator_key_*.json

# Validate network JSON
jq . cyclops-network.json

# Check snapshot sizes (should be ~1.3-1.4GB each)
du -h *.snap
```

### **After Phase 2**
```bash
# Validate directory structure
./phase4-validate.sh .accumulate

# Check permissions
find .accumulate -name "priv_validator_key.json" -exec ls -la {} \;
```

### **After Phase 3**
```bash
# Check node process
ps aux | grep accumulated

# Test RPC endpoint
curl http://localhost:26657/status | jq

# View recent logs
tail -20 cyclops-node.log

# Check network connectivity
curl http://localhost:26657/net_info | jq '.result.peers | length'
```

---

## ⚠️ Common Issues & Solutions

### **Phase 1 Issues**
- **Missing binaries**: Ensure `accumulated` and `analyze` are present and executable
- **Network JSON errors**: Validate JSON syntax with `jq`
- **Key generation fails**: Check file permissions and disk space

### **Phase 2 Issues**
- **Permission errors**: Ensure scripts are executable (`chmod +x *.sh`)
- **Directory creation fails**: Check disk space and write permissions
- **Validation fails**: Run `./validate-node-structure.sh .accumulate` for details

### **Phase 3 Issues**
- **Port conflicts**: Check if ports 26656-26658 are already in use
- **Configuration errors**: Run `./accumulated run --work-dir .accumulate --check-config`
- **Startup timeout**: Check logs for specific error messages
- **RPC not responding**: Verify node process is running and ports are open

---

## 🛠️ Operational Commands

### **Node Management**
```bash
# Status check
curl http://localhost:26657/status | jq

# View logs
tail -f cyclops-node.log

# Stop node
kill $(cat cyclops-node.pid)

# Restart node  
./phase3-launch.sh
```

### **Health Monitoring**
```bash
# Validator info
curl http://localhost:26657/validators | jq

# Network peers
curl http://localhost:26657/net_info | jq '.result.peers'

# Block height
curl http://localhost:26657/status | jq '.result.sync_info.latest_block_height'
```

---

## 📊 Expected Performance

### **Resource Usage**
- **Disk Space**: ~3GB (snapshots + databases)
- **Memory**: ~2-4GB during startup, ~1-2GB steady state
- **CPU**: Moderate during startup, low steady state
- **Network**: Ports 26656 (P2P), 26657 (RPC), 26658 (gRPC)

### **Timing**
- **Phase 1**: 2-3 minutes (key generation + consensus creation)
- **Phase 2**: 30 seconds (directory setup)
- **Phase 3**: 2-5 minutes (snapshot restoration + startup)
- **Total**: 5-9 minutes for complete deployment

---

**🎯 All three phases are working and production-ready!**
