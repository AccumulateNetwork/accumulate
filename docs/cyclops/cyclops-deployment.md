# Cyclops Deployment: Deploy Phase

**Status**: ✅ **UPDATED** - Reflects unified single-key architecture

This document details the steps for deploying prepared artifacts to the validator node. These steps assume the Prep phase has been completed and all artifacts are present in `~/accumulate-network/artifacts2` on the build/ops machine.

## 🔑 Key Architecture Overview

Cyclops uses a **three-file key system**:
- **`cyclops-network.json`** - Network configuration with validator public keys
- **`priv_validator_key.json`** - Single private validator key for both DN and BVN
- **`node_key.json`** - P2P networking key (separate from validator key)

## Steps

1. **Clean Previous Deployment**
   - Remove any previous deployment at `/tmp/cyclops/` on the validator node.

2. **Create Artifacts Directory**
   - Create the directory `/tmp/cyclops/artifacts` on the validator node.

3. **Copy Core Artifacts**
   - Copy the following files from `~/accumulate-network/artifacts2` to `/tmp/cyclops/artifacts`:
     - `cyclops-genesis.snap` - Genesis snapshot
     - `priv_validator_key.json` - **Single validator key for both partitions**
     - `node_key.json` - P2P networking key
     - `cyclops-network.json` - Network configuration
     - `consensus_dn.json` - Directory consensus configuration
     - `consensus_bvn0.json` - BVN consensus configuration

4. **Key Validation** ⚠️ **CRITICAL**
   - Verify validator key matches network JSON:
     ```bash
     # Check that priv_validator_key.json public key matches cyclops-network.json
     analyze validate keys --network cyclops-network.json --validator priv_validator_key.json
     ```
   - Verify node key format:
     ```bash
     # Check P2P key format
     analyze validate key --file node_key.json --type node
     ```

5. **Node Configuration Construction**
   - Use the artifacts in `/tmp/cyclops/artifacts` to construct the node configuration
   - **Single validator key** will be used for both DN and BVN partitions
   - **Node key** will be used for P2P networking only

6. **Initialize Node**
   - Run `accumulated init node` using the artifacts from `/tmp/cyclops`
   - The single `priv_validator_key.json` will be copied to both partition directories

7. **Verify Deployment**
   - Ensure TOML configuration files are generated and placed correctly
   - Ensure **single** `priv_validator_key.json` is present in both partition directories
   - Ensure `node_key.json` is present for P2P networking
   - Ensure partition snapshots are present and correct
   - **Verify no dangling DN/BVN-specific key files exist**

---

## 🎯 Deployment Validation Checklist

### Required Files Present
- [ ] `cyclops-genesis.snap` - Genesis snapshot
- [ ] `priv_validator_key.json` - Single validator key
- [ ] `node_key.json` - P2P networking key  
- [ ] `cyclops-network.json` - Network configuration
- [ ] Consensus JSON files for both partitions

### Key Validation
- [ ] Validator public key matches network JSON
- [ ] Validator address is consistent across files
- [ ] Node key is properly formatted for P2P
- [ ] No dangling DN/BVN-specific key files

### Architecture Compliance
- [ ] Single validator key used for both partitions
- [ ] P2P key separate from validator key
- [ ] Network JSON contains authoritative public keys

**Result:**
The validator node at `/tmp/cyclops` is configured with the unified key architecture and ready for launch.

---

## 📚 Related Documentation

- **[Key Management Guide](cyclops-key-management-guide.md)** - Complete key architecture reference
- **[Network JSON Reference](cyclops-network-json-reference.md)** - Network configuration details
- **[Troubleshooting Guide](cyclops-node-startup-troubleshooting.md)** - Common key-related issues
