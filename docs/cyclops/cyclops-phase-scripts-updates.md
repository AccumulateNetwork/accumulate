# Cyclops Phase Scripts Updates for Unified Key Architecture

## Overview

This document outlines the required updates to the Cyclops phase scripts to properly implement the three-file key architecture and ensure consistent handling of validator keys and P2P networking keys throughout the deployment process.

## Key Architecture Summary

The unified key architecture consists of three critical files:

1. **`cyclops-network.json`** - Network configuration with validator public keys
2. **`priv_validator_key.json`** - Single validator private key for both DN and BVN consensus
3. **`node_key.json`** - P2P networking key (separate from validator key)

## Required Updates by Phase

### Phase 0: Environment Setup (`phase0-restart-tests.sh`)

#### Issues Identified:
- Missing `node_key.json` in artifact copy list
- Legacy `priv_validator_key_*.json` pattern handling
- Incorrect source directory path

#### Required Updates:

1. **Fix Source Directory**
   ```bash
   # Current (INCORRECT):
   ARTIFACTS_SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
   
   # Should be:
   ARTIFACTS_SOURCE_DIR="/home/paulsnow/accumulate-network/artifacts2"
   ```

2. **Add Node Key to Copy List**
   ```bash
   # Add to files_to_copy array:
   "node_key.json"              # P2P networking key
   ```

3. **Remove Legacy Key Pattern Handling**
   ```bash
   # REMOVE this entire section (lines ~90-95):
   for key_file in "$artifacts_dest/priv_validator_key_"*.json; do
       if [ -f "$key_file" ]; then
           chmod 600 "$key_file"
           log_success "Set 600 permissions: $(basename "$key_file")"
       fi
   done
   ```

4. **Add Specific P2P Key Permissions**
   ```bash
   # Add after validator key permissions:
   if [ -f "$artifacts_dest/node_key.json" ]; then
       chmod 600 "$artifacts_dest/node_key.json"
       log_success "Set 600 permissions: node_key.json"
   fi
   ```

### Phase 1: Preparation (`phase1-prep.sh`)

#### Issues Identified:
- Legacy wildcard pattern in artifact listing
- Key consistency issues in consensus generation

#### Required Updates:

1. **Fix Artifact Listing Pattern**
   ```bash
   # Line 719 - CHANGE FROM:
   ls -la ./priv_validator_key_*.json | sed 's/^/    /'
   
   # TO:
   ls -la ./priv_validator_key.json | sed 's/^/    /'
   ```

2. **Verify Consensus Key Consistency**
   - Ensure consensus generation uses the unified validator key
   - Validate that generated consensus files contain the correct public key from network JSON
   - Current consensus files show key mismatch that needs investigation

### Phase 2: Deployment (`phase2-deploy.sh`)

#### Status: ✅ Already Updated
- P2P node key placement implemented
- Copies `node_key.json` to all three locations:
  - `.accumulate/config/node_key.json`
  - `.accumulate/dn/config/node_key.json`
  - `.accumulate/bvn-cyclops/config/node_key.json`
- Proper 600 permissions set on all key files

### Phase 3: Launch (`phase3-launch.sh`)

#### Issues Identified:
- Missing comprehensive key validation
- No verification of key consistency

#### Required Updates:

1. **Log Directory Setup**
   ```bash
   LOG_DIR="/tmp/cyclops/logs"
   mkdir -p "$LOG_DIR"
   nohup /tmp/cyclops/artifacts/accumulated run --work-dir . > "$LOG_DIR/cyclops-node.log" 2>&1 &
   echo "$NODE_PID" > "$LOG_DIR/cyclops-node.pid"
   ```

2. **Add Key Validation Step**
   ```bash
   # Add new Step 2.5: Key validation
   echo -e "\n🔑 Step 2.5: Key validation..."
   
   # Validate validator key consistency
   if [ -f "cyclops-network.json" ] && [ -f ".accumulate/config/priv_validator_key.json" ]; then
       NETWORK_PUB_KEY=$(jq -r '.partitions.Directory.validators[0].publicKey' cyclops-network.json)
       VALIDATOR_PUB_KEY=$(jq -r '.pub_key.value' .accumulate/config/priv_validator_key.json)
       # Compare and validate consistency
   fi
   
   # Validate P2P key format
   for key_file in ".accumulate/config/node_key.json" ".accumulate/dn/config/node_key.json" ".accumulate/bvn-cyclops/config/node_key.json"; do
       if [ -f "$key_file" ]; then
           jq empty "$key_file" || echo "ERROR: Invalid P2P key format: $key_file"
       fi
   done
   ```

### Phase 4: Validation (`phase4-validate.sh`)

#### Status: ✅ Already Clean
- Uses only `priv_validator_key.json` references
- No legacy patterns found

## Critical Key Consistency Issues

### Consensus File Key Mismatch
**Problem**: Current consensus files contain different validator keys than the unified architecture:
- Consensus files: `ECZBGdux7MhgShs1SwNTVNHB+XUrv/UmoP46rGnWuLI=`
- Expected from memory: `i7PlCCObVDLrxjNBbwQH7WnnsRyfggcvcqGBN/VOzPw=`

**Solution**: Regenerate consensus files using the correct unified validator key.

### Address Consistency
**Problem**: Validator addresses don't match between consensus and network configuration:
- Consensus: `D6A4E10D68E2C6EB4E907F44CB09BBE50C9D81D2`
- Expected: `8fd3628816fed741e3ce8a845b3ba2137da8a96b`

## Implementation Priority

1. **High Priority**: Phase 0 and Phase 1 updates (foundation for key handling)
2. **Medium Priority**: Phase 3 validation enhancements
3. **Critical**: Resolve key consistency issues in consensus generation

## Validation Checklist

After implementing updates, verify:

- [ ] All scripts reference only `priv_validator_key.json` (no wildcards)
- [ ] `node_key.json` is copied and deployed to all required locations
- [ ] All key files have 600 permissions
- [ ] Consensus files contain the correct unified validator key
- [ ] Validator addresses are consistent across all files
- [ ] P2P keys are properly formatted and validated

## Related Documentation

- [Cyclops Key Management Guide](./cyclops-key-management-guide.md)
- [Cyclops Deployment Guide](./cyclops-deployment.md)
- [Node Startup Troubleshooting](./cyclops-node-startup-troubleshooting.md)

## Testing Recommendations

1. **Clean Environment Test**: Run full phase sequence in clean `/tmp/cyclops` environment
2. **Key Validation**: Verify all three key files are present and properly formatted
3. **Launch Test**: Ensure node starts without key-related errors
4. **Network Connectivity**: Verify P2P networking functions correctly

---

**Note**: This document should be used as a reference for implementing the phase script updates. Each change should be tested individually before proceeding to the next phase.
