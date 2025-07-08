# Phase 0: Environment Setup - Cyclops Development Deployment

## Overview

Phase 0 establishes a clean, isolated test environment for Cyclops validator deployment with complete file independence and corruption prevention. This phase is critical for ensuring deployment reliability and preventing file corruption issues that can occur with linked files.

## Objectives

- **Create isolated test environment** under `/tmp/cyclops`
- **Ensure complete file independence** with no symbolic or hard links
- **Prevent file corruption** through proper copying procedures
- **Establish restart capability** for development iterations
- **Validate security permissions** on sensitive files
- **Test environment integrity** before proceeding to deployment phases

## Critical Requirements

### No File Links Policy
- **NEVER use `ln -s` or `ln` commands** - these cause file corruption
- **Always use `cp` commands** for complete file duplication
- **Verify file independence** using inode comparison
- **Detect and prevent** any linking operations

### File Independence
- Each copied file must have a unique inode
- Modifications to copies must not affect source files
- Complete isolation between test environment and source artifacts
- No shared underlying data structures

## Directory Structure

```
/tmp/cyclops/                           # Isolated test environment
└── node/                               # Node deployment directory
    └── artifacts/                      # Copied artifacts (NO LINKS)
        ├── cyclops-genesis.snap        # Unified snapshot (2.1GB)
        ├── cyclops-network-full.json   # Complete network configuration
        ├── priv_validator_key_*.json   # Validator private keys (600 perms)
        ├── accumulated                 # Node binary
        ├── analyze                     # Analysis binary
        └── ...                         # Other required artifacts
```

## Phase 0 Procedures

### Step 1: Environment Cleanup
```bash
# Remove any existing test environment
if [ -d "/tmp/cyclops" ]; then
    rm -rf "/tmp/cyclops"
fi
```

**Purpose**: Ensure clean starting state with no residual files or configurations.

### Step 2: Directory Creation
```bash
# Create base test environment
mkdir -p "/tmp/cyclops/node/artifacts"

# Create backup directory for restart testing
mkdir -p "/tmp/cyclops/backup"
```

**Purpose**: Establish required directory structure for deployment phases.

### Step 3: Artifact Copying (Critical - No Links)
```bash
# Copy all artifacts using FULL FILE COPIES (NO LINKS)
cp "$SOURCE/cyclops-genesis.snap" "/tmp/cyclops/node/artifacts/"
cp "$SOURCE/cyclops-network-full.json" "/tmp/cyclops/node/artifacts/"
cp "$SOURCE/priv_validator_key_defidevs-acme_dn.json" "/tmp/cyclops/node/artifacts/"
cp "$SOURCE/priv_validator_key_defidevs-acme_bvn0.json" "/tmp/cyclops/node/artifacts/"
cp "$SOURCE/accumulated" "/tmp/cyclops/node/artifacts/"
cp "$SOURCE/analyze" "/tmp/cyclops/node/artifacts/"
```

**Critical**: Never use `ln -s` or `ln` commands - these create links that cause corruption.

### Step 4: Security Permissions
```bash
# Set secure permissions on validator keys
chmod 600 "/tmp/cyclops/node/artifacts/priv_validator_key_defidevs-acme_dn.json"
chmod 600 "/tmp/cyclops/node/artifacts/priv_validator_key_defidevs-acme_bvn0.json"

# Set executable permissions on binaries
chmod +x "/tmp/cyclops/node/artifacts/accumulated"
chmod +x "/tmp/cyclops/node/artifacts/analyze"
```

**Purpose**: Ensure proper security for sensitive files and executable permissions for binaries.

### Step 5: File Independence Verification
```bash
# Verify files are independent (different inodes)
test_inode=$(stat -c %i "/tmp/cyclops/node/artifacts/cyclops-network-full.json")
source_inode=$(stat -c %i "$SOURCE/cyclops-network-full.json")

if [ "$test_inode" = "$source_inode" ]; then
    echo "ERROR: Files share same inode - LINK DETECTED"
    exit 1
fi
```

**Purpose**: Detect any accidental linking that could cause corruption.

## Validation Criteria

### File Independence Tests
- ✅ All copied files have unique inodes
- ✅ No symbolic links detected (`[ ! -L "$file" ]`)
- ✅ No hard links detected (different inode numbers)
- ✅ File modifications don't affect source files

### Security Validation
- ✅ Validator keys have 600 permissions
- ✅ Binaries have executable permissions
- ✅ Directory structure is properly created
- ✅ No world-readable sensitive files

### Environment Integrity
- ✅ All required artifacts present
- ✅ File sizes match source artifacts
- ✅ Checksums match (optional verification)
- ✅ Sufficient disk space available

## Environment Reset Capability

### Clean Environment Reset
```bash
# Clean existing environment
rm -rf "/tmp/cyclops"

# Recreate directory structure
mkdir -p "/tmp/cyclops/node/artifacts"

# Re-copy artifacts from source
cp "$SOURCE/"* "/tmp/cyclops/node/artifacts/"
```

**Purpose**: Enable quick environment reset for development iterations without backup overhead.

## Running Phase 0 Tests

```bash
# Navigate to scripts directory
cd /home/paulsnow/go/src/gitlab.com/AccumulateNetwork/accumulate/docs/scripts

# Run Phase 0 environment setup
./phase0-restart-tests.sh
```

### Test Coverage
1. **Environment Cleanup** - Removes existing test environment
2. **Directory Creation** - Creates required directory structure
3. **Artifact Copying** - Tests full file copying (no links)
4. **File Independence** - Verifies no linking corruption
5. **Environment Reset** - Tests clean environment recreation capability
6. **Permissions** - Validates security permissions

## Common Issues and Solutions

### Issue: File Corruption from Links
**Symptom**: Changes to test files affect source files
**Cause**: Use of `ln -s` or `ln` commands creating links
**Solution**: Use only `cp` commands for all file operations

### Issue: Permission Denied Errors
**Symptom**: Cannot access validator keys or execute binaries
**Cause**: Incorrect file permissions
**Solution**: Set 600 permissions on keys, +x on binaries



## File Size Requirements

| File | Size | Purpose |
|------|------|---------|
| cyclops-genesis.snap | ~2.1GB | Unified network snapshot |
| Directory-partition.snap | ~1.3GB | Directory partition snapshot |
| bvn-cyclops-partition.snap | ~1.4GB | BVN partition snapshot |
| accumulated | ~88MB | Node binary |
| analyze | ~87MB | Analysis binary |
| Network JSON | ~2KB | Network configuration |
| Validator keys | ~345B each | Private validator keys |

**Total**: ~4.9GB minimum space required

## Security Considerations

### File Permissions
- **Validator keys**: 600 (owner read/write only)
- **Configuration files**: 644 (owner read/write, group/other read)
- **Binaries**: 755 (owner read/write/execute, group/other read/execute)
- **Directories**: 755 (standard directory permissions)

### Access Control
- Test environment isolated under `/tmp/cyclops`
- No network access required for Phase 0
- No external dependencies
- Self-contained artifact copying

## Performance Metrics

### Typical Execution Times
- Environment cleanup: < 5 seconds
- Directory creation: < 1 second
- Artifact copying: 30-60 seconds (depends on disk I/O)
- Permission setting: < 1 second
- Validation tests: 5-10 seconds

### Resource Usage
- **Disk I/O**: High during copying phase
- **Memory**: Minimal (< 100MB)
- **CPU**: Low (file operations only)
- **Network**: None required

## Next Steps

After successful Phase 0 completion:

1. **Proceed to Phase 1**: Artifact preparation and key generation
2. **Run Phase 1 tests**: Validate preparation procedures
3. **Continue to Phase 2**: Node directory creation and deployment
4. **Monitor environment**: Watch for any corruption or issues

## Troubleshooting

### Debug Commands
```bash
# Check file independence
stat -c "%i %n" /tmp/cyclops/node/artifacts/* | sort

# Verify no symbolic links
find /tmp/cyclops -type l

# Check permissions
ls -la /tmp/cyclops/node/artifacts/priv_validator_key_*
```

### Log Analysis
- Check for "LINK DETECTED" errors
- Verify all validation checkmarks (✅)
- Look for permission denied errors

## Related Documentation

- [Phase 1: Artifact Preparation](phase1-artifact-preparation.md)
- [Phase 2: Node Deployment](phase2-node-deployment.md)
- [Phase 3: Node Launch](phase3-node-launch.md)
- [Cyclops Development Deployment Plan](cyclops-development-deployment-plan.md)
- [File Corruption Prevention Guide](../technical/file-corruption-prevention.md)

---

**Status**: Production Ready ✅  
**Last Updated**: 2025-01-08  
**Version**: 1.0  
**Critical**: No file links policy must be strictly enforced
