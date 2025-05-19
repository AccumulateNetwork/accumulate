---
title: Snapshot User Experience in Accumulate
description: An analysis of the user experience for working with snapshots in Accumulate
tags: [accumulate, snapshots, user-experience, operations, maintenance]
created: 2025-05-16
version: 1.0
updated: 2025-05-17
---

> **DEPRECATED**: This document has been moved to the new documentation structure. Please refer to the [new location](../new_structure/03_core_components/03_snapshots/04_user_experience.md) for the most up-to-date version.

# Snapshot User Experience in Accumulate

## 1. Introduction

This document analyzes the user experience aspects of working with snapshots in Accumulate. It covers configuration, management, troubleshooting, and best practices for node operators and developers. Understanding the user experience is crucial for effectively leveraging snapshots for node operations and maintenance.

## 2. Configuration Experience

### 2.1 Snapshot Service Configuration

Node operators configure the snapshot service through the node configuration file:

```yaml
snapshots:
  directory: data/snapshots
  schedule: "0 0 * * *"  # Daily at midnight
  retain-count: 4        # Keep 4 snapshots
  enable-indexing: true  # Enable snapshot indexing
```

Configuration options include:

- **Directory**: Where snapshots are stored
- **Schedule**: When snapshots are automatically created (cron format)
- **Retain Count**: How many snapshots to keep
- **Enable Indexing**: Whether to build indices for faster snapshot queries

### 2.2 State Sync Configuration

For nodes using state sync to bootstrap, configuration is done in the CometBFT config:

```toml
[statesync]
enable = true
rpc_servers = "node1.example.com:26657,node2.example.com:26657"
trust_height = 1000000
trust_hash = "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
```

Configuration options include:

- **Enable**: Whether to use state sync
- **RPC Servers**: List of seed nodes to fetch snapshots from
- **Trust Height**: Block height to trust
- **Trust Hash**: Block hash to trust

### 2.3 Configuration Challenges

Users may face challenges with:

1. **Finding trust values**: Obtaining a recent block height and hash
2. **Scheduling**: Setting an appropriate snapshot schedule
3. **Storage planning**: Allocating sufficient disk space for snapshots

## 3. Operational Experience

### 3.1 Creating Snapshots

Snapshots are created:

1. **Automatically**: Based on the configured schedule
2. **Manually**: By creating a `.capture` file in the snapshot directory

Manual snapshot creation:

```bash
touch data/snapshots/.capture
```

This provides flexibility for node operators to create snapshots when needed, such as before upgrades or maintenance.

### 3.2 Listing Snapshots

Users can list available snapshots through the API:

```bash
curl -X GET "http://localhost:26660/v3/snapshots"
```

Example response:

```json
{
  "snapshots": [
    {
      "header": {
        "version": 2,
        "rootHash": "...",
        "systemLedger": {
          "url": "acc://bvn-mainnet.acme",
          "index": 1000000,
          "timestamp": "2025-05-16T10:00:00Z",
          "executorVersion": 1
        }
      },
      "consensusInfo": {
        "block": {
          "header": {
            "height": 1000000,
            "time": "2025-05-16T10:00:00Z",
            "lastBlockId": {
              "hash": "..."
            },
            "appHash": "..."
          }
        }
      }
    }
  ]
}
```

This allows users to see available snapshots and their metadata.

### 3.3 Managing Snapshots

Snapshot management is primarily automatic:

1. **Creation**: Based on schedule or manual trigger
2. **Retention**: Based on retain-count configuration
3. **Pruning**: Automatic removal of old snapshots

However, users can manually manage snapshots by:

1. **Copying**: Moving snapshots to backup storage
2. **Deleting**: Removing unwanted snapshots
3. **Importing**: Placing snapshots in the snapshot directory

### 3.4 Bootstrapping with Snapshots

The user experience for bootstrapping a node with snapshots involves:

1. **Obtaining trust values**: Getting a recent block height and hash
2. **Configuring state sync**: Setting up the CometBFT config
3. **Starting the node**: Launching the node with state sync enabled
4. **Monitoring progress**: Watching logs for state sync progress

Example log output during state sync:

```
INFO [statesync] Discovered new snapshot height=1000000 format=2 hash=... chunks=42
INFO [statesync] Snapshot download started height=1000000 format=2 hash=...
INFO [statesync] Downloading snapshot chunk chunk=1/42
...
INFO [statesync] Applied snapshot chunk chunk=42/42
INFO [statesync] Snapshot restored height=1000000 format=2 hash=...
INFO [statesync] State sync complete
```

## 4. API Experience

### 4.1 Snapshot API Endpoints

Accumulate provides API endpoints for interacting with snapshots:

```
GET /v3/snapshots
```

This endpoint returns a list of available snapshots with their metadata.

### 4.2 API Usage Examples

Listing snapshots with curl:

```bash
curl -X GET "http://localhost:26660/v3/snapshots"
```

Listing snapshots with the Accumulate CLI:

```bash
accumulated api snapshots list
```

### 4.3 API Response Structure

The API response includes detailed information about each snapshot:

```json
{
  "snapshots": [
    {
      "header": {
        "version": 2,
        "rootHash": "...",
        "systemLedger": {
          "url": "acc://bvn-mainnet.acme",
          "index": 1000000,
          "timestamp": "2025-05-16T10:00:00Z",
          "executorVersion": 1
        }
      },
      "consensusInfo": {
        "block": {
          "header": {
            "height": 1000000,
            "time": "2025-05-16T10:00:00Z",
            "lastBlockId": {
              "hash": "..."
            },
            "appHash": "..."
          }
        }
      }
    }
  ]
}
```

This structure provides users with comprehensive information about each snapshot, including:

- Version information
- Block height and time
- Root hash
- Consensus state

## 5. Troubleshooting Experience

### 5.1 Common Issues

Users may encounter several common issues with snapshots:

1. **Snapshot creation failures**:
   - Insufficient disk space
   - Permission issues
   - Database corruption

2. **State sync failures**:
   - Network connectivity issues
   - Incompatible snapshot versions
   - Incorrect trust values
   - Corrupted snapshot chunks

3. **Restoration failures**:
   - Incompatible database versions
   - Insufficient memory
   - Corrupted snapshots

### 5.2 Diagnostic Tools

Users can diagnose snapshot issues using:

1. **Log analysis**: Examining node logs for error messages
2. **API queries**: Checking snapshot metadata
3. **File inspection**: Examining snapshot files directly

Example log messages for snapshot issues:

```
ERROR [snapshot] Failed to create snapshot: insufficient disk space
ERROR [statesync] Failed to download chunk 5/42: network error
ERROR [statesync] Failed to apply snapshot: BPT hash mismatch
```

### 5.3 Resolution Strategies

For common issues, users can:

1. **Snapshot creation failures**:
   - Free up disk space
   - Check file permissions
   - Verify database integrity

2. **State sync failures**:
   - Check network connectivity
   - Try different seed nodes
   - Verify trust values
   - Restart state sync

3. **Restoration failures**:
   - Ensure compatible software versions
   - Increase available memory
   - Try a different snapshot

## 6. Best Practices

### 6.1 Snapshot Scheduling

Recommended practices for snapshot scheduling:

1. **Regular snapshots**: Create snapshots at regular intervals (daily or weekly)
2. **Pre-upgrade snapshots**: Create snapshots before network upgrades
3. **Post-upgrade snapshots**: Create snapshots after successful upgrades
4. **Staggered scheduling**: Schedule snapshots at different times across nodes

### 6.2 Storage Management

Best practices for snapshot storage:

1. **Sufficient space**: Allocate at least 3-5 times the database size for snapshots
2. **Backup snapshots**: Copy important snapshots to backup storage
3. **Retention policy**: Keep enough snapshots for recovery needs
4. **Monitoring**: Set up alerts for low disk space

### 6.3 Bootstrapping Strategy

Optimal strategy for bootstrapping nodes:

1. **Regular nodes**: Use state sync for fast bootstrapping
2. **Archive nodes**: Use block replay for complete history
3. **Validator nodes**: Use state sync initially, then keep up with blocks
4. **Backup nodes**: Keep recent snapshots ready for quick recovery

### 6.4 Monitoring and Maintenance

Recommended monitoring and maintenance practices:

1. **Regular verification**: Periodically verify snapshot integrity
2. **Log monitoring**: Set up alerts for snapshot errors
3. **API checks**: Regularly query the snapshot API to ensure functionality
4. **Disk cleanup**: Regularly clean up old or unused snapshots

## 7. User Feedback and Improvement Areas

### 7.1 Current Pain Points

Based on analysis, users may experience these pain points:

1. **Trust value acquisition**: Difficulty obtaining trusted block height and hash
2. **Error messages**: Sometimes cryptic or insufficient error messages
3. **Manual intervention**: Occasional need for manual snapshot management
4. **Resource planning**: Difficulty estimating resource requirements

### 7.2 Suggested Improvements

Potential improvements to enhance user experience:

1. **Trust value API**: Add an API endpoint to get recent trusted block values
2. **Enhanced error reporting**: Provide more detailed and actionable error messages
3. **Snapshot management CLI**: Create dedicated CLI tools for snapshot management
4. **Resource calculators**: Provide tools to estimate resource requirements
5. **Snapshot verification**: Add tools to verify snapshot integrity
6. **Differential snapshots**: Implement smaller, incremental snapshots
7. **Snapshot compression**: Add optional compression to reduce storage requirements
8. **Remote storage**: Support storing snapshots on remote storage (S3, etc.)

### 7.3 Documentation Needs

Areas where documentation could be improved:

1. **Troubleshooting guides**: Step-by-step guides for common issues
2. **Configuration examples**: More comprehensive configuration examples
3. **Resource planning**: Guidelines for estimating resource requirements
4. **Operational procedures**: Standard procedures for common operations
5. **API reference**: Comprehensive API documentation with examples

## 8. Conclusion

The snapshot system in Accumulate provides a generally good user experience for node operators and developers. It offers flexibility in configuration, automation for routine tasks, and API access for integration. However, there are opportunities for improvement in error handling, tooling, and documentation.

By addressing the identified pain points and implementing the suggested improvements, the snapshot system could provide an even better user experience, making node operation and maintenance more efficient and reliable.

The key to a positive user experience with snapshots is balancing automation with control, providing clear information and actionable error messages, and ensuring that the system is robust and reliable under various conditions.
