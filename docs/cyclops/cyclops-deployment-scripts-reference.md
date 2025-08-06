# Cyclops Deployment Scripts Reference

**Status**: ✅ **PRODUCTION READY** - Complete deployment automation system  
**Updated**: 2025-07-07 17:00 CDT  
**Source**: `/home/paulsnow/accumulate-network/artifacts2/`  

---

## 🎯 **Overview**

This reference documents all deployment and validation scripts in the Cyclops artifacts collection. These scripts provide complete automation for validator deployment, from initial preparation through final validation.

### **Script Collection**
- **`deploy-cyclops-complete.sh`** - Master deployment orchestrator
- **`phase1-prep.sh`** - Phase 1: Preparation and validation
- **`phase2-deploy.sh`** - Phase 2: Directory structure deployment
- **`phase3-launch.sh`** - Phase 3: Node launch and initialization
- **`phase4-validate.sh`** - Phase 4: Post-deployment validation

---

## 🚀 **Master Deployment Script**

### **`deploy-cyclops-complete.sh`**

**Purpose**: Complete end-to-end Cyclops validator deployment orchestrator

**Usage**:
```bash
./deploy-cyclops-complete.sh [target-directory]
```

**What It Does**:
1. **Copies artifacts** to target directory
2. **Executes Phase 1**: Preparation and validation
3. **Executes Phase 2**: Directory structure deployment
4. **Executes Phase 3**: Node launch and initialization
5. **Provides status** and completion summary

**Key Features**:
- **Colored output** for clear status indication
- **Error handling** with immediate exit on failure
- **Automatic timestamping** for deployment directories
- **Progress tracking** through all phases
- **Comprehensive logging** of all operations

**Default Target**: `/tmp/cyclops-deployment-YYYYMMDD-HHMMSS`

**Example**:
```bash
# Deploy to default timestamped directory
./deploy-cyclops-complete.sh

# Deploy to specific directory
./deploy-cyclops-complete.sh /opt/cyclops-validator
```

---

## 📋 **Phase-Specific Scripts**

### **Phase 1: `phase1-prep.sh`**

**Purpose**: Preparation and artifact validation

**Size**: 25KB (comprehensive preparation logic)

**Key Functions**:
- **Artifact integrity validation**
- **Configuration template processing**
- **Network JSON validation**
- **Dependency checking**
- **Pre-deployment environment setup**

**Validation Checks**:
- ✅ All required artifacts present
- ✅ File sizes and checksums correct
- ✅ Network configuration valid
- ✅ TOML templates processable
- ✅ Target directory permissions

**Output**: Detailed preparation report with pass/fail status

---

### **Phase 2: `phase2-deploy.sh`**

**Purpose**: Directory structure deployment and file placement

**Size**: 14KB (deployment orchestration)

**Key Functions**:
- **Clean directory structure creation**
- **Artifact copying to proper locations**
- **Configuration file generation**
- **Permission setting**
- **Symlink creation**

**Directory Structure Created**:
```
.accumulate/
├── dn/                    # Directory Node
│   ├── config/
│   ├── data/
│   └── snapshots/
├── bvn-cyclops/          # BVN Node
│   ├── config/
│   ├── data/
│   └── snapshots/
└── logs/                 # Centralized logging
```

**File Placement**:
- **Snapshots** → partition-specific data directories
- **Keys** → partition-specific config directories
- **TOML configs** → generated and placed appropriately
- **Binaries** → accessible locations with proper permissions

---

### **Phase 3: `phase3-launch.sh`**

**Purpose**: Node launch and initialization

**Size**: 7KB (launch orchestration)

**Key Functions**:
- **Snapshot restoration**
- **Node process startup**
- **Health check monitoring**
- **Initial synchronization**
- **Service registration**

**Launch Sequence**:
1. **Restore snapshots** to partition data directories
2. **Start Directory Node** with proper configuration
3. **Start BVN Node** with proper configuration
4. **Monitor startup** for successful initialization
5. **Validate connectivity** between partitions
6. **Report operational status**

**Monitoring**:
- **Process health** - Ensure nodes stay running
- **Log monitoring** - Watch for startup errors
- **Port availability** - Verify API endpoints
- **Consensus participation** - Check validator activity

---

### **Phase 4: `phase4-validate.sh`**

**Purpose**: Comprehensive post-deployment validation

**Size**: 11KB (extensive validation logic)

**Key Functions**:
- **Directory structure validation**
- **File integrity checking**
- **Configuration validation**
- **Process health verification**
- **Network connectivity testing**

**Validation Categories**:

#### **📁 Directory Structure**
- ✅ All required directories present
- ✅ Proper directory permissions
- ✅ Correct ownership settings
- ✅ Symlinks properly created

#### **📄 File Integrity**
- ✅ All configuration files present
- ✅ TOML syntax validation
- ✅ JSON configuration validation
- ✅ Binary file integrity
- ✅ Key file validation

#### **🔧 Configuration Validation**
- ✅ Network configuration consistency
- ✅ Port allocation correctness
- ✅ Partition ID consistency
- ✅ Validator key matching

#### **🏃 Process Health**
- ✅ Node processes running
- ✅ API endpoints responding
- ✅ Log files being written
- ✅ No critical errors in logs

#### **🌐 Network Connectivity**
- ✅ P2P connectivity established
- ✅ Inter-partition communication
- ✅ External API accessibility
- ✅ Consensus participation

**Command Line Options**:
```bash
# Basic validation
./phase4-validate.sh /path/to/.accumulate

# Verbose output
./phase4-validate.sh --verbose /path/to/.accumulate

# Fix permissions automatically
./phase4-validate.sh --fix-permissions /path/to/.accumulate

# Help information
./phase4-validate.sh --help
```

---

## 🔑 **Utility Scripts**

### **`generate_all_validator_keys.sh`**

**Purpose**: Generate validator key files for all ADIs in the network configuration

**Size**: 1.6KB (key generation utility)

**Key Functions**:
- **Parse network JSON** to extract validator ADIs
- **Generate DN keys** for Directory Network partition
- **Generate BVN keys** for Block Validator Network partition
- **Proper file naming** with ADI-based conventions
- **Cleanup temporary files** after generation

**Prerequisites**:
- `jq` command-line JSON processor
- `analyze` binary in current directory
- `cyclops-network.json` network configuration

**Usage**:
```bash
# Generate keys for all validators in network config
./generate_all_validator_keys.sh

# Output files created:
# - priv_validator_key_defidevs-acme_dn.json
# - priv_validator_key_defidevs-acme_bvn0.json
```

**Key Generation Process**:
1. **Extract ADIs**: Parse `cyclops-network.json` for validator operators
2. **Name Conversion**: Convert ADI format (`acc://defidevs.acme`) to filename format (`defidevs-acme`)
3. **DN Key Generation**: Create Directory Network validator key using `analyze gen-key`
4. **BVN Key Generation**: Create Block Validator Network key using `analyze gen-key`
5. **File Management**: Move keys to proper filenames and cleanup temporary directories

**Output Files**:
- **DN Keys**: `priv_validator_key_{adi_name}_dn.json`
- **BVN Keys**: `priv_validator_key_{adi_name}_bvn0.json`

**Integration**: Used by `phase1-prep.sh` during the preparation phase to generate fresh validator keys

---

## 🛠️ **Script Features**

### **Common Features Across All Scripts**

#### **🎨 Colored Output**
- **Green (✓)**: Success indicators
- **Red (✗)**: Error indicators  
- **Yellow (⚠)**: Warning indicators
- **Blue (ℹ)**: Information indicators
- **Cyan**: Headers and sections

#### **🔍 Error Handling**
- **`set -e`**: Exit immediately on any error
- **Comprehensive logging** of all operations
- **Detailed error messages** with context
- **Cleanup procedures** on failure

#### **📊 Progress Tracking**
- **Step-by-step progress** indicators
- **Time stamps** for all major operations
- **Summary reports** at completion
- **Detailed logs** for troubleshooting

#### **🔧 Configuration**
- **Environment variable** support
- **Command line argument** parsing
- **Default value** handling
- **Flexible path** configuration

---

## 📖 **Usage Patterns**

### **Complete Deployment**
```bash
# One-command complete deployment
cd /home/paulsnow/accumulate-network/artifacts2
./deploy-cyclops-complete.sh

# Monitor progress and check results
tail -f /tmp/cyclops-deployment-*/deployment.log
```

### **Manual Phase Execution**
```bash
# Execute phases individually for debugging
./phase1-prep.sh /target/directory
./phase2-deploy.sh /target/directory  
./phase3-launch.sh /target/directory
./phase4-validate.sh /target/directory
```

### **Validation Only**
```bash
# Validate existing deployment
./phase4-validate.sh --verbose /existing/.accumulate

# Fix permissions and validate
./phase4-validate.sh --fix-permissions /existing/.accumulate
```

### **Troubleshooting**
```bash
# Verbose output for debugging
./phase4-validate.sh --verbose /path/to/.accumulate

# Check specific components
grep "ERROR\|FAIL" /path/to/.accumulate/logs/*.log
```

---

## 🔍 **Script Internals**

### **Key Functions and Logic**

#### **Path Expansion and Portability**
```bash
# All scripts now use tilde notation for portability
ARTIFACTS_DIR="~/accumulate-network/artifacts"
ACCUMULATE_DIR="~/go/src/gitlab.com/AccumulateNetwork/accumulate"

# Automatic expansion to absolute paths
ARTIFACTS_DIR="${ARTIFACTS_DIR/#\~/$HOME}"
ACCUMULATE_DIR="${ACCUMULATE_DIR/#\~/$HOME}"

# Common utilities provide standardized path handling
source ./common-utils.sh
EXPANDED_PATH=$(expand_path "~/some/path")
```

#### **Artifact Validation**
```bash
# Example validation logic from phase1-prep.sh
validate_artifacts() {
    check_file_exists "cyclops-network.json"
    check_file_size "Directory-partition.snap" "1.3GB"
    check_file_size "bvn-cyclops-partition.snap" "1.4GB"
    validate_json_syntax "cyclops-network.json"
    validate_toml_templates
}
```

#### **Configuration Generation**
```bash
# Example config generation from phase2-deploy.sh
generate_configs() {
    process_toml_template "accumulate-template-dn.toml" "dn"
    process_toml_template "accumulate-template-bvn.toml" "bvn-cyclops"
    generate_cometbft_config "dn" 26656 26657
    generate_cometbft_config "bvn-cyclops" 36656 36657
}
```

#### **Health Monitoring**
```bash
# Example health check from phase3-launch.sh
monitor_node_health() {
    wait_for_port 26657 "Directory Node API"
    wait_for_port 36657 "BVN Node API"
    check_consensus_participation
    validate_inter_partition_communication
}
```

---

## 🚨 **Important Notes**

### **⚠️ Script Safety**
- **Always run from artifacts directory** for proper relative paths
- **Ensure sufficient disk space** (~3GB for snapshots)
- **Check port availability** before deployment
- **Backup existing deployments** before re-running

### **🔒 Security Considerations**
- **Scripts contain validator keys** - protect access
- **Network configuration** includes sensitive routing
- **File permissions** are set automatically
- **Process isolation** through proper directory structure

### **🔧 Customization**
- **Environment variables** can override defaults
- **Configuration templates** can be modified before deployment
- **Port assignments** configurable in network JSON
- **Directory paths** adjustable via command line

---

## 📊 **Performance Metrics**

### **Typical Execution Times**
- **Phase 1 (Prep)**: 30-60 seconds
- **Phase 2 (Deploy)**: 2-3 minutes (snapshot copying)
- **Phase 3 (Launch)**: 1-2 minutes (node startup)
- **Phase 4 (Validate)**: 30-45 seconds
- **Total Complete Deployment**: 4-6 minutes

### **Resource Requirements**
- **Disk Space**: ~3GB (snapshots + binaries)
- **Memory**: 4GB+ recommended for node operation
- **CPU**: 2+ cores recommended
- **Network**: Stable internet for P2P connectivity

---

## 🔗 **Related Documentation**

### **Deployment Guides**
- [**Artifacts Deployment Guide**](cyclops-artifacts-deployment-guide.md) - Complete deployment system
- [**Deployment Phases**](cyclops-deployment-phases.md) - Detailed phase documentation
- [**TOML Configuration**](cyclops-toml-configuration.md) - Configuration system

### **Troubleshooting**
- [**Node Startup Troubleshooting**](cyclops-node-startup-troubleshooting.md) - Common issues
- [**Fixes Tracking**](cyclops-fixes-tracking.md) - Known problems and solutions

---

## 🎉 **Summary**

The Cyclops deployment scripts provide:

- **✅ Complete Automation** - One-command deployment capability
- **✅ Comprehensive Validation** - Extensive pre and post-deployment checks
- **✅ Error Handling** - Robust error detection and reporting
- **✅ Flexible Usage** - Individual phase execution or complete automation
- **✅ Production Ready** - Tested and validated deployment procedures

**These scripts eliminate manual deployment complexity and provide reliable, repeatable Cyclops validator deployment with comprehensive validation and monitoring.**
