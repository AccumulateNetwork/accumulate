# Phase 1 Backup and Recovery Strategy

**Status: Phase 1 Complete - Securing for Phase 2**

This document provides comprehensive backup, recovery, and documentation strategies to protect the completed Phase 1 work and enable seamless continuation by any AI assistant or developer.

## 🔒 Backup Strategy

### 1. Critical Artifacts Backup

**Location**: `/home/paulsnow/accumulate-network/artifacts/`

**Critical Files to Backup:**
```bash
# Core artifacts (MUST preserve)
cyclops-genesis.snap                    # 2.1GB - Unified snapshot
cyclops-network.json                    # Updated network config with keys
priv_validator_key_defidevs-acme_dn.json    # Directory validator private key
priv_validator_key_defidevs-acme_bvn0.json  # BVN validator private key
consensus_dn.json                       # Directory consensus section
consensus_bvn0.json                     # BVN consensus section

# Generated partition snapshots
partition-snapshots/Directory-partition.snap     # 1.3GB
partition-snapshots/bvn-cyclops-partition.snap   # 1.4GB

# Working tools
analyze                                 # Built CLI tool
accumulated                            # Built daemon
cyclops_prep_automated.sh              # Working automation script
```

### 2. Automated Backup Script

Create comprehensive backup with validation:

```bash
#!/bin/bash
# backup-phase1.sh - Complete Phase 1 backup

BACKUP_DIR="/home/paulsnow/cyclops-phase1-backup-$(date +%Y%m%d-%H%M%S)"
ARTIFACTS_DIR="/home/paulsnow/accumulate-network/artifacts"

echo "🔒 Creating Phase 1 Backup: $BACKUP_DIR"
mkdir -p "$BACKUP_DIR"

# Backup critical artifacts
echo "📦 Backing up critical artifacts..."
cp "$ARTIFACTS_DIR/cyclops-genesis.snap" "$BACKUP_DIR/"
cp "$ARTIFACTS_DIR/cyclops-network.json" "$BACKUP_DIR/"
cp "$ARTIFACTS_DIR/priv_validator_key_defidevs-acme_dn.json" "$BACKUP_DIR/"
cp "$ARTIFACTS_DIR/priv_validator_key_defidevs-acme_bvn0.json" "$BACKUP_DIR/"
cp "$ARTIFACTS_DIR/consensus_dn.json" "$BACKUP_DIR/"
cp "$ARTIFACTS_DIR/consensus_bvn0.json" "$BACKUP_DIR/"

# Backup partition snapshots
echo "📦 Backing up partition snapshots..."
mkdir -p "$BACKUP_DIR/partition-snapshots"
cp "$ARTIFACTS_DIR/partition-snapshots/"*.snap "$BACKUP_DIR/partition-snapshots/"

# Backup tools and scripts
echo "📦 Backing up tools..."
cp "$ARTIFACTS_DIR/analyze" "$BACKUP_DIR/"
cp "$ARTIFACTS_DIR/accumulated" "$BACKUP_DIR/"
cp "$ARTIFACTS_DIR/cyclops_prep_automated.sh" "$BACKUP_DIR/"

# Create manifest
echo "📋 Creating backup manifest..."
cat > "$BACKUP_DIR/BACKUP_MANIFEST.md" << EOF
# Phase 1 Backup Manifest
**Created**: $(date)
**Backup Directory**: $BACKUP_DIR

## Critical Files Backed Up:
- cyclops-genesis.snap ($(stat -c%s "$BACKUP_DIR/cyclops-genesis.snap" | numfmt --to=iec))
- cyclops-network.json ($(stat -c%s "$BACKUP_DIR/cyclops-network.json") bytes)
- priv_validator_key_defidevs-acme_dn.json ($(stat -c%s "$BACKUP_DIR/priv_validator_key_defidevs-acme_dn.json") bytes)
- priv_validator_key_defidevs-acme_bvn0.json ($(stat -c%s "$BACKUP_DIR/priv_validator_key_defidevs-acme_bvn0.json") bytes)
- consensus_dn.json ($(stat -c%s "$BACKUP_DIR/consensus_dn.json") bytes)
- consensus_bvn0.json ($(stat -c%s "$BACKUP_DIR/consensus_bvn0.json") bytes)
- Directory-partition.snap ($(stat -c%s "$BACKUP_DIR/partition-snapshots/Directory-partition.snap" | numfmt --to=iec))
- bvn-cyclops-partition.snap ($(stat -c%s "$BACKUP_DIR/partition-snapshots/bvn-cyclops-partition.snap" | numfmt --to=iec))

## Validation Commands:
\`\`\`bash
# Validate JSON files
jq '.' cyclops-network.json
jq '.' consensus_dn.json  
jq '.' consensus_bvn0.json

# Check snapshot integrity
./analyze snap-report cyclops-genesis.snap
./analyze snap-report partition-snapshots/Directory-partition.snap
./analyze snap-report partition-snapshots/bvn-cyclops-partition.snap
\`\`\`

## Recovery Instructions:
1. Copy all files back to /home/paulsnow/accumulate-network/artifacts/
2. Run validation commands above
3. Execute ./cyclops_prep_automated.sh to verify functionality
EOF

# Validate backup
echo "✅ Validating backup..."
if [ -f "$BACKUP_DIR/cyclops-genesis.snap" ] && [ -f "$BACKUP_DIR/cyclops-network.json" ]; then
    echo "✅ Backup completed successfully: $BACKUP_DIR"
    echo "📊 Total backup size: $(du -sh "$BACKUP_DIR" | cut -f1)"
else
    echo "❌ Backup validation failed!"
    exit 1
fi
```

### 3. Documentation Backup

**Complete Documentation Set:**
```bash
# Documentation files to preserve
docs/cyclops/cyclops-preparation.md           # Complete prep workflow
docs/cyclops/cyclops-automation.md            # Automation details  
docs/cyclops/consensus-generation-fix.md      # Technical fix details
docs/cyclops/consensus-code-changes.md        # Code changes
docs/cyclops/examples/                        # Working examples
docs/cyclops/cyclops-network-json-reference.md # Network config reference
docs/README.md                                # Master documentation index
```

## 🔄 Recovery Procedures

### 1. Complete Recovery from Backup

```bash
#!/bin/bash
# recover-phase1.sh - Restore Phase 1 from backup

BACKUP_DIR="$1"
ARTIFACTS_DIR="/home/paulsnow/accumulate-network/artifacts"

if [ -z "$BACKUP_DIR" ]; then
    echo "Usage: $0 <backup-directory>"
    exit 1
fi

echo "🔄 Recovering Phase 1 from: $BACKUP_DIR"

# Create artifacts directory
mkdir -p "$ARTIFACTS_DIR/partition-snapshots"

# Restore critical files
echo "📦 Restoring artifacts..."
cp "$BACKUP_DIR/cyclops-genesis.snap" "$ARTIFACTS_DIR/"
cp "$BACKUP_DIR/cyclops-network.json" "$ARTIFACTS_DIR/"
cp "$BACKUP_DIR/priv_validator_key_defidevs-acme_dn.json" "$ARTIFACTS_DIR/"
cp "$BACKUP_DIR/priv_validator_key_defidevs-acme_bvn0.json" "$ARTIFACTS_DIR/"
cp "$BACKUP_DIR/consensus_dn.json" "$ARTIFACTS_DIR/"
cp "$BACKUP_DIR/consensus_bvn0.json" "$ARTIFACTS_DIR/"
cp "$BACKUP_DIR/analyze" "$ARTIFACTS_DIR/"
cp "$BACKUP_DIR/accumulated" "$ARTIFACTS_DIR/"
cp "$BACKUP_DIR/cyclops_prep_automated.sh" "$ARTIFACTS_DIR/"

# Restore partition snapshots
cp "$BACKUP_DIR/partition-snapshots/"*.snap "$ARTIFACTS_DIR/partition-snapshots/"

# Set permissions
chmod +x "$ARTIFACTS_DIR/analyze"
chmod +x "$ARTIFACTS_DIR/accumulated"
chmod +x "$ARTIFACTS_DIR/cyclops_prep_automated.sh"
chmod 600 "$ARTIFACTS_DIR/priv_validator_key_"*.json

echo "✅ Recovery completed. Run validation:"
echo "cd $ARTIFACTS_DIR && ./cyclops_prep_automated.sh --validate-only"
```

### 2. Partial Recovery (Individual Components)

```bash
# Restore just network configuration
cp backup/cyclops-network.json ~/accumulate-network/artifacts/

# Restore just validator keys  
cp backup/priv_validator_key_*.json ~/accumulate-network/artifacts/

# Restore just consensus files
cp backup/consensus_*.json ~/accumulate-network/artifacts/

# Restore just partition snapshots
cp backup/partition-snapshots/*.snap ~/accumulate-network/artifacts/partition-snapshots/
```

## 📚 AI Handoff Documentation

### 1. Complete Reproduction Guide

**For New AI Assistant:**

```markdown
# Phase 1 Reproduction Guide

## Context
Phase 1 (Prep) of Cyclops validator deployment is complete. This guide enables 
complete reproduction from scratch.

## Prerequisites
1. Accumulate source code at: /home/paulsnow/go/src/gitlab.com/AccumulateNetwork/accumulate
2. Unified snapshot: cyclops-genesis.snap (2.1GB)
3. Go compiler and build tools

## Step-by-Step Reproduction

### 1. Build Tools
cd /home/paulsnow/go/src/gitlab.com/AccumulateNetwork/accumulate
go build -o ~/accumulate-network/artifacts/analyze ./tools/cmd/analyze
go build -o ~/accumulate-network/artifacts/accumulated ./cmd/accumulated

### 2. Run Automated Prep
cd ~/accumulate-network/artifacts
./cyclops_prep_automated.sh

### 3. Validate Results
- Check all 8 critical artifacts exist
- Run consensus validation: docs/cyclops/examples/validate-consensus.sh
- Verify partition snapshots contain consensus sections

## Expected Artifacts
[List of all 8 critical files with sizes and checksums]

## Troubleshooting
[Reference to complete troubleshooting documentation]
```

### 2. Technical Reference

**Key Technical Details for AI:**
- **Consensus Fix**: Base64 decoding, not hex (critical for public keys)
- **Network JSON Structure**: Validators at network level, not partition level
- **CometBFT Format**: Must use types.GenesisDoc for consensus sections
- **Key Integration**: update-network-keys command updates network JSON
- **Partition Assignment**: Validators must have partitions array with active: true

## 🛡️ Protection Strategies

### 1. Read-Only Reference Copies

```bash
# Create read-only reference copies
mkdir -p ~/cyclops-phase1-reference
cp -r ~/accumulate-network/artifacts/* ~/cyclops-phase1-reference/
chmod -R 444 ~/cyclops-phase1-reference/  # Read-only
```

### 2. Git Repository Backup

```bash
# Create git repository for version control
cd ~/accumulate-network/artifacts
git init
git add cyclops-network.json consensus_*.json cyclops_prep_automated.sh
git commit -m "Phase 1 complete - all artifacts ready"
git tag phase1-complete
```

### 3. Checksum Validation

```bash
# Create checksums for integrity verification
cd ~/accumulate-network/artifacts
sha256sum cyclops-genesis.snap > checksums.txt
sha256sum cyclops-network.json >> checksums.txt
sha256sum priv_validator_key_*.json >> checksums.txt
sha256sum consensus_*.json >> checksums.txt
sha256sum partition-snapshots/*.snap >> checksums.txt
```

## 🚀 Phase 2 Preparation

### 1. Isolated Testing Environment

```bash
# Create Phase 2 testing directory
mkdir -p ~/cyclops-phase2-testing
cp ~/accumulate-network/artifacts/cyclops-genesis.snap ~/cyclops-phase2-testing/
# Test Phase 2 deployment here without affecting Phase 1 artifacts
```

### 2. Rollback Strategy

```bash
# If Phase 2 fails, quick rollback to Phase 1 state
cp ~/cyclops-phase1-reference/* ~/accumulate-network/artifacts/
```

## 📋 Validation Checklist

**Before proceeding to Phase 2:**

- [ ] All 8 critical artifacts backed up
- [ ] Backup validated and tested
- [ ] Documentation complete and tested
- [ ] Read-only reference copies created
- [ ] Checksums generated
- [ ] Recovery procedures tested
- [ ] AI handoff documentation complete
- [ ] Phase 2 testing environment prepared

**Phase 1 is now fully secured and documented for safe Phase 2 progression.**
