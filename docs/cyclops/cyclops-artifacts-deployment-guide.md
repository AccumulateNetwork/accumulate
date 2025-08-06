# Cyclops Artifacts Deployment Guide

**Status**: ✅ **PRODUCTION READY** - Complete artifacts-based deployment system  
**Updated**: 2025-07-07 16:56 CDT  
**Source**: `/home/paulsnow/accumulate-network/artifacts2/`  

---

## 🎯 **Overview**

This guide covers deploying a Cyclops validator using the pre-built artifacts collection - a complete set of binaries, configurations, keys, and snapshots that enables rapid validator deployment without complex preparation steps.

### **What Are Artifacts?**

The artifacts directory contains the "golden master" collection of all files needed for Cyclops validator deployment:

- **Network Configuration**: Complete network JSON with routing rules
- **Partition Snapshots**: Genesis snapshots for Directory and BVN partitions  
- **Validator Keys**: Ed25519 keys for validator identity and P2P networking
- **Configuration Templates**: TOML files for Accumulate and CometBFT
- **Binaries**: Compiled `accumulated` daemon and utilities
- **Deployment Scripts**: Automated deployment and validation tools

---

## 🚨 **CRITICAL - Artifacts Protection**

### **⚠️ Golden Master Warning**
```
🔒 NEVER MODIFY FILES IN artifacts2/ DIRECTORY
```

**Why This Matters:**
- **Irreplaceable Files**: Some artifacts took hours to generate
- **Configuration DNA**: TOML files define network behavior
- **Deployment Source**: Scripts copy from this directory
- **Backup Strategy**: This IS the backup - protect it!

### **🛡️ Protection Measures**
- **Read-Only Usage**: Only copy FROM artifacts, never modify
- **Backup Strategy**: Regular backups of entire artifacts2 directory
- **Version Control**: Track changes to deployment scripts only
- **Documentation**: Complete regeneration process documented

---

## 📁 **Artifacts Inventory**

### **Core Network Files**
| File | Size | Purpose |
|------|------|---------|
| `cyclops-network.json` | 3.8KB | Network configuration with routing rules |
| `Directory-partition.snap` | 1.3GB | Directory Node genesis snapshot |
| `bvn-cyclops-partition.snap` | 1.4GB | BVN partition genesis snapshot |

### **Validator Identity**
| File | Size | Purpose |
|------|------|---------|
| `priv_validator_key_defidevs-acme_dn.json` | 345B | Directory Node validator key |
| `priv_validator_key_defidevs-acme_bvn0.json` | 345B | BVN validator key |
| `node_key.json` | 144B | P2P networking key |

### **Configuration Templates**
| File | Size | Purpose |
|------|------|---------|
| `toml/accumulate-template-bvn.toml` | 281B | BVN Accumulate config template |
| `toml/accumulate-template-dn.toml` | 89B | DN Accumulate config template |
| `toml/config-template-cometbft.toml` | 15KB | CometBFT consensus config template |
| `toml/tendermint-template.toml` | 179B | Legacy Tendermint config template |

### **Deployment Tools**
| File | Purpose |
|------|---------|
| `phase1-prep.sh` | Phase 1: Artifact preparation and validation |
| `phase2-deploy.sh` | Phase 2: Node directory deployment |
| `phase3-launch.sh` | Phase 3: Validator launch and monitoring |
| `phase4-validate.sh` | Phase 4: Post-deployment validation |

---

## 🚀 **Deployment Workflow**

### **Phase 1: Preparation**
```bash
cd /home/paulsnow/accumulate-network/artifacts2
./phase1-prep.sh
```

**What It Does:**
- Validates all artifacts integrity
- Generates partition-specific configurations from templates
- Creates deployment-ready file structure
- Performs pre-deployment checks

### **Phase 2: Deployment**
```bash
./phase2-deploy.sh
```

**What It Does:**
- Creates clean node directory structure
- Copies artifacts to appropriate locations
- Sets proper file permissions
- Deploys configuration files

### **Phase 3: Launch**
```bash
./phase3-launch.sh
```

**What It Does:**
- Restores partition snapshots
- Starts validator node processes
- Initializes monitoring
- Validates node startup

### **Phase 4: Validation**
```bash
./phase4-validate.sh
```

**What It Does:**
- Verifies node health and connectivity
- Validates configuration integrity
- Checks partition synchronization
- Generates operational status report

---

## 🔧 **Configuration Details**

### **Network Structure**
```json
{
  "id": "cyclops",
  "partitions": [
    {"id": "Directory", "type": "directory"},
    {"id": "bvn-cyclops", "type": "blockValidator"}
  ],
  "validators": [
    {
      "id": "defidevs.acme",
      "partitions": ["Directory", "bvn-cyclops"]
    }
  ]
}
```

### **Routing Configuration**
- **5 routing rules** distribute accounts across partitions
- **Directory partition** handles system accounts and routing
- **BVN partition** processes user transactions
- **Validator** participates in both partitions

### **Port Allocation**
| Partition | P2P Port | RPC Port | API Port |
|-----------|----------|----------|----------|
| Directory | 26656 | 26657 | 26658 |
| BVN | 36656 | 36657 | 36658 |

---

## 🛠️ **TOML Configuration System**

### **Template Structure**
The artifacts include master TOML templates that are processed during deployment:

- **`accumulate-template-bvn.toml`** → `.accumulate/bvn-cyclops/config/accumulate.toml`
- **`accumulate-template-dn.toml`** → `.accumulate/dn/config/accumulate.toml`
- **`config-template-cometbft.toml`** → `.accumulate/*/config/config.toml`

### **Generation Process**
1. **Phase 1** reads network JSON and templates
2. **Partition-specific configs** generated with proper IDs and ports
3. **Deployment scripts** place configs in correct node directories
4. **Validation** ensures configuration integrity

### **⚠️ TOML File Protection**
- **Never edit templates directly** - they're used by automation
- **Regeneration documented** in `toml/TOML-FILES-DOCUMENTATION.md`
- **Source code references** for manual recovery if needed
- **Backup strategy** includes entire toml/ directory

---

## 🔍 **Troubleshooting**

### **Common Issues**

#### **Missing Artifacts**
```bash
# Verify all required files exist
ls -la /home/paulsnow/accumulate-network/artifacts2/
```

#### **Permission Errors**
```bash
# Fix permissions if deployment fails
chmod +x /home/paulsnow/accumulate-network/artifacts2/*.sh
```

#### **Configuration Errors**
```bash
# Validate TOML syntax
./phase4-validate.sh
```

#### **Snapshot Issues**
```bash
# Verify snapshot integrity
file *.snap
ls -lh *.snap
```

### **Recovery Procedures**

#### **Corrupted Artifacts**
1. **Stop all processes**
2. **Restore from backup** (if available)
3. **Regenerate from source** (see TOML documentation)
4. **Re-run deployment phases**

#### **Failed Deployment**
1. **Clean deployment directory**
2. **Verify artifacts integrity**
3. **Re-run from Phase 1**
4. **Check logs for specific errors**

---

## 📊 **Operational Status**

### **Deployment Metrics**
- **Total Artifacts Size**: ~2.8GB
- **Deployment Time**: ~10-15 minutes
- **Network Startup**: ~2-3 minutes
- **Validation Time**: ~1-2 minutes

### **Success Indicators**
- ✅ All phases complete without errors
- ✅ Node processes running and healthy
- ✅ Partitions synchronized
- ✅ Validator participating in consensus
- ✅ API endpoints responding

---

## 🔗 **Related Documentation**

### **Phase-Specific Guides**
- [**Deployment Phases**](cyclops-deployment-phases.md) - Complete phase documentation
- [**TOML Configuration**](cyclops-toml-configuration.md) - Configuration file details
- [**3-Phase Automation**](cyclops-3-phase-automation-design.md) - System architecture

### **Troubleshooting**
- [**Node Startup Guide**](cyclops-node-startup-and-bpt-guide.md) - Startup procedures
- [**Troubleshooting**](cyclops-node-startup-troubleshooting.md) - Common issues
- [**Fixes Tracking**](cyclops-fixes-tracking.md) - Known issues and solutions

---

## 🎉 **Summary**

The Cyclops artifacts deployment system provides:

- **✅ Rapid Deployment** - Complete validator in 10-15 minutes
- **✅ Reliable Process** - Tested and validated automation
- **✅ Protected Assets** - Golden master artifacts preservation
- **✅ Complete Documentation** - Every step and file documented
- **✅ Recovery Procedures** - Full regeneration capability

**The artifacts-based approach eliminates complex preparation steps and provides a reliable, repeatable deployment process for Cyclops validators.**
