# Cyclops Key Management Guide

**Status**: ✅ **PRODUCTION READY** - Unified key architecture implemented

**Last Updated**: 2025-07-08

---

## Overview

The Cyclops validator network uses a **three-file key architecture** that separates concerns between network configuration, consensus validation, and P2P networking. Understanding this architecture is critical for successful deployment and operation.

## 🔑 Three-File Key Architecture

### 1. Network Configuration: `cyclops-network.json`
**Purpose**: Defines the network structure and validator public keys for consensus

**Contains**:
- Validator **public keys** for consensus signing
- Network configuration (partitions, fees, limits)
- Global network settings
- Routing configuration

**Key Role**: 
- Used to generate consensus sections in snapshots
- Provides configuration information for network initialization
- Contains the **authoritative** validator public keys that nodes must match

**Location**: `/home/paulsnow/accumulate-network/artifacts2/cyclops-network.json`

**Example Validator Entry**:
```json
{
  "operator": "acc://defidevs.acme",
  "publicKey": "8bb3e508239b5432ebc633416f0407ed69e7b11c9f82072f72a18137f54eccfc",
  "publicKeyHash": "8fd3628816fed741e3ce8a845b3ba2137da8a96b3c9000da7dbb728ffec5fadd",
  "partitions": ["Directory", "bvn-cyclops"]
}
```

### 2. Consensus Validation: `priv_validator_key.json`
**Purpose**: Contains the private key for consensus signing

**Contains**:
- Ed25519 **private key** for signing consensus messages
- Corresponding **public key** (must match network JSON)
- Validator address (derived from public key)

**Key Role**:
- Used by consensus engine to sign blocks and votes
- **Must derive to the public key in the network JSON**
- Single key used for both DN and BVN partitions

**Location**: `/home/paulsnow/accumulate-network/artifacts2/priv_validator_key.json`

**Format**:
```json
{
  "address": "8fd3628816fed741e3ce8a845b3ba2137da8a96b",
  "pub_key": {
    "type": "tendermint/PubKeyEd25519",
    "value": "i7PlCCObVDLrxjNBbwQH7WnnsRyfggcvcqGBN/VOzPw="
  },
  "priv_key": {
    "type": "tendermint/PrivKeyEd25519",
    "value": "[64-byte Ed25519 private key in base64]"
  }
}
```

### 3. P2P Networking: `node_key.json`
**Purpose**: Contains the private key for P2P networking and node discovery

**Contains**:
- Ed25519 **private key** for P2P authentication
- Used for network discovery and peer communication
- **Separate and different** from validator key

**Key Role**:
- Enables nodes to discover and authenticate with each other
- Essential for network connectivity and bootstrap process
- **Not stored in network JSON** - managed separately per node

**Location**: `/home/paulsnow/accumulate-network/artifacts2/node_key.json`

**Format**:
```json
{
  "type": "tendermint/PrivKeyEd25519",
  "value": "[64-byte Ed25519 private key in base64 - includes both seed and public key]"
}
```

---

## 🎯 Key Relationships and Dependencies

### Critical Relationships
1. **Network JSON ↔ Validator Key**: Public keys must match exactly
2. **Validator Key ↔ Consensus**: Private key must derive to network JSON public key
3. **Node Key ↔ P2P**: Independent key used only for networking

### Validation Chain
```
Network JSON Public Key → Must Match → Validator Private Key → Signs Consensus Messages
Node Key → Independent → Used for P2P Networking Only
```

---

## 🚨 Critical DevOps Requirements

### For Consensus (Validator Keys)
- **Single Key Architecture**: One `priv_validator_key.json` for both DN and BVN
- **Key Matching**: Private key must derive to public key in network JSON
- **Address Consistency**: Validator address must match across all files
- **No Key Confusion**: Don't mix up validator and node keys

### For P2P Networking (Node Keys)
- **Essential for Connectivity**: Missing node key = no network connectivity
- **Unique Per Node**: Each node needs its own P2P key
- **Not in Network JSON**: P2P keys are managed separately
- **Bootstrap Critical**: Required for initial network discovery

### For Network Configuration
- **Authoritative Source**: Network JSON is the single source of truth
- **Snapshot Generation**: Used to create consensus sections
- **Configuration Source**: Provides all network settings
- **Validator Registry**: Contains all validator public keys

---

## 🔧 Deployment Checklist

### Pre-Deployment Validation
- [ ] Network JSON contains correct validator public key
- [ ] Validator private key derives to network JSON public key
- [ ] Validator address matches across all files
- [ ] Node key exists and is properly formatted
- [ ] All three files are present in artifacts directory

### Post-Deployment Validation
- [ ] Node starts without key-related errors
- [ ] P2P connectivity established
- [ ] Consensus participation confirmed
- [ ] No key mismatch errors in logs

---

## 🐛 Common Issues and Solutions

### Issue: "Validator key mismatch"
**Cause**: Private validator key doesn't match network JSON public key
**Solution**: Regenerate validator key or update network JSON

### Issue: "P2P connection failed"
**Cause**: Missing or corrupted node key
**Solution**: Generate new node key with proper Ed25519 format

### Issue: "Consensus not participating"
**Cause**: Validator key not matching network configuration
**Solution**: Verify key derivation and address consistency

### Issue: "Node startup panic"
**Cause**: Incorrect key format (32-byte vs 64-byte)
**Solution**: Ensure proper Ed25519 key format in all files

---

## 📝 Key Generation Commands

### Generate Validator Key
```bash
# Using analyze tool (recommended)
analyze generate key --type validator --output priv_validator_key.json

# Verify key matches network JSON
analyze validate keys --network cyclops-network.json --validator priv_validator_key.json
```

### Generate Node Key
```bash
# Using analyze tool
analyze generate key --type node --output node_key.json

# Manual verification
analyze validate key --file node_key.json --type node
```

---

## 🔒 Security Considerations

### Validator Keys
- **High Security**: Controls consensus participation
- **Backup Critical**: Loss prevents consensus participation
- **Access Control**: Restrict to validator operators only

### Node Keys
- **Medium Security**: Controls network identity
- **Replaceable**: Can be regenerated without consensus impact
- **Unique**: Each node must have different P2P key

### Network JSON
- **Configuration Security**: Changes affect entire network
- **Version Control**: Track all changes with backups
- **Validation**: Always validate after modifications

---

## 📚 Related Documentation

- [Cyclops Deployment Guide](cyclops-deployment.md)
- [Network JSON Reference](cyclops-network-json-reference.md)
- [P2P Key Generation](../technical/p2p-key-generation.md)
- [Troubleshooting Guide](cyclops-node-startup-troubleshooting.md)

---

*This guide provides the definitive reference for Cyclops key management. All procedures have been tested and validated in the production environment.*
