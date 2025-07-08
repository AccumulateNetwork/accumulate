# Cyclops Easy Deployment Guide

## Overview
This guide provides the simplest path to configure and launch a Cyclops validator node. All the complex troubleshooting steps have been automated into a single script.

## Quick Start (5 Minutes)

### Prerequisites
```bash
# 1. Navigate to your artifacts directory
cd /path/to/accumulate-network/artifacts

# 2. Ensure you have the required files:
ls -la accumulated analyze cyclops-init-network.json
ls -la ../partition-snapshots/bvn-cyclops-partition.snap
```

### One-Command Deployment
```bash
# Run the automated deployment script
./cyclops_node_startup_automated.sh
```

### Start Your Node
```bash
# After the script completes successfully:
cd artifacts
./accumulated run --work-dir .accumulate/bvn1-1
```

That's it! Your Cyclops validator node should be running.

## What the Automation Script Does

The script automates all the manual steps we discovered during troubleshooting:

### ✅ Step 1: Initialize Network Structure
- Runs `accumulated init network` with proper configuration
- Creates `.accumulate/bvn1-1/` directory structure
- Sets up basic configuration files

### ✅ Step 2: Generate Node Key
- Uses `analyze generate-node-key` to create proper Ed25519 keys
- Sets correct 600 permissions for security
- Places key in correct location: `.accumulate/bvn1-1/config/node_key.json`

### ✅ Step 3: Create Proper Configuration
- Generates `accumulate.toml` with correct structure:
  ```toml
  [describe]
    type = "blockValidator"
    partition-id = "bvn-cyclops"
  
  [storage]
    type = "leveldb"
    path = "data/accumulate.db"
  ```
- Avoids TOML structure conflicts
- Uses proper partition type for BVN nodes

### ✅ Step 4: Restore Partition Snapshot
- Attempts to restore `bvn-cyclops-partition.snap`
- Handles BPT restoration issues gracefully
- Creates database directory structure

### ✅ Step 5: Comprehensive Validation
- Checks all required files exist
- Validates file permissions (600 for keys)
- Verifies database directory creation
- Reports any issues found

## Configuration Options

### Environment Variables
```bash
# Customize the deployment
export NETWORK_ID="cyclops"           # Network identifier
export PARTITION_ID="bvn-cyclops"     # Partition name
export PARTITION_TYPE="blockValidator" # Node type
export WORK_DIR="$PWD/artifacts"      # Working directory

./cyclops_node_startup_automated.sh
```

### Directory Node Deployment
```bash
# For Directory nodes instead of BVN
export PARTITION_TYPE="directory"
export PARTITION_ID="Directory"
./cyclops_node_startup_automated.sh
```

## Troubleshooting

### Script Validation Only
```bash
# Check setup without making changes
./cyclops_node_startup_automated.sh --validate-only
```

### Common Issues

#### 1. Missing Prerequisites
**Error**: "accumulated binary not found"
**Solution**: Ensure binaries are built and in artifacts directory

#### 2. Snapshot Not Found
**Error**: "BVN partition snapshot not found"
**Solution**: Ensure `../partition-snapshots/bvn-cyclops-partition.snap` exists

#### 3. BPT Restoration Failure
**Error**: "cannot modify account - observer is not set"
**Status**: Known issue, node may still start successfully

#### 4. Permission Issues
**Error**: Key files have wrong permissions
**Solution**: Script automatically sets 600 permissions

### Manual Verification
```bash
# Check node structure
tree artifacts/.accumulate/bvn1-1/

# Verify configuration
cat artifacts/.accumulate/bvn1-1/config/accumulate.toml

# Check key permissions
ls -la artifacts/.accumulate/bvn1-1/config/*.json
```

## Advanced Usage

### Custom Network Configuration
```bash
# Use different network configuration
cp my-network.json cyclops-init-network.json
./cyclops_node_startup_automated.sh
```

### Multiple Node Deployment
```bash
# Deploy multiple nodes with different configurations
PARTITION_ID="bvn-node1" ./cyclops_node_startup_automated.sh
PARTITION_ID="bvn-node2" ./cyclops_node_startup_automated.sh
```

## File Structure After Deployment

```
artifacts/
├── .accumulate/bvn1-1/           # Node directory
│   ├── config/
│   │   ├── accumulate.toml       # ✅ Main configuration
│   │   ├── node_key.json         # ✅ P2P key (600 perms)
│   │   ├── priv_validator_key.json # ✅ Validator key (600 perms)
│   │   └── priv_validator_state.json # ✅ Validator state
│   └── data/
│       ├── accumulate.db/        # ✅ Restored database
│       └── priv_validator_state.json
├── accumulated                   # Node binary
├── analyze                      # Analysis tool
└── cyclops-init-network.json   # Network config
```

## Validation Commands

### Check Node Status
```bash
# After starting the node
curl http://localhost:26657/status
```

### Monitor Logs
```bash
# Start with debug logging
./accumulated run --work-dir .accumulate/bvn1-1 --log-level debug
```

### Verify Database
```bash
# Check database size
du -sh artifacts/.accumulate/bvn1-1/data/accumulate.db/
```

## Next Steps After Deployment

1. **Monitor Node Health**: Watch logs for consensus participation
2. **Network Connectivity**: Verify peer connections
3. **Validator Participation**: Check if node is signing blocks
4. **Backup Keys**: Secure backup of validator keys
5. **Monitoring Setup**: Implement health checks and alerts

## Production Considerations

### Security
- Keep validator keys secure (already set to 600 permissions)
- Use firewall to restrict access to node ports
- Regular security updates

### Performance
- Monitor disk space for database growth
- Ensure adequate RAM and CPU resources
- Network bandwidth for peer synchronization

### Maintenance
- Regular backups of validator state
- Node software updates
- Network upgrade participation

## Integration with Existing Workflows

### CI/CD Integration
```bash
# Add to deployment pipeline
./cyclops_node_startup_automated.sh
if [ $? -eq 0 ]; then
    echo "Node deployment successful"
    ./accumulated run --work-dir .accumulate/bvn1-1 &
else
    echo "Node deployment failed"
    exit 1
fi
```

### Docker Integration
```dockerfile
# Add to Dockerfile
COPY cyclops_node_startup_automated.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/cyclops_node_startup_automated.sh
CMD ["/usr/local/bin/cyclops_node_startup_automated.sh"]
```

## Support and Documentation

### Related Documentation
- [Cyclops Node Startup Troubleshooting](cyclops-node-startup-troubleshooting.md)
- [Cyclops Network Configuration](cyclops-network-configuration.md)
- [BPT Restoration Design](../technical/bpt-restoration-design.md)

### Getting Help
1. Check the troubleshooting guide first
2. Run validation: `./cyclops_node_startup_automated.sh --validate-only`
3. Review logs with `--log-level debug`
4. Consult the detailed troubleshooting documentation

## Summary

This automation script eliminates the complexity of manual Cyclops node deployment by:

- ✅ **Automating all manual steps** discovered during troubleshooting
- ✅ **Handling configuration correctly** (proper TOML structure, partition types)
- ✅ **Managing file permissions** (600 for sensitive keys)
- ✅ **Providing comprehensive validation** (file existence, permissions, structure)
- ✅ **Graceful error handling** (continues on non-critical failures)
- ✅ **Clear documentation** (what each step does and why)

The result is a **one-command deployment** that takes you from zero to a running Cyclops validator node in minutes instead of hours of troubleshooting.

---

*This guide is based on validated troubleshooting steps from 2025-07-07 and incorporates all discovered fixes and best practices.*
