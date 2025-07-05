<!-- AI_DOCUMENT_TYPE: deployment_guide -->
<!-- AI_PRIMARY_TOPICS: deployment_automation, cyclops_network, script_usage -->
<!-- AI_COMPLEXITY: medium -->
<!-- AI_SPLIT_RECOMMENDED: no -->
<!-- AI_LAST_UPDATED: 2025-01-05 -->

# Cyclops Network Deployment Guide

> **Document Type**: Deployment automation guide  
> **Scope**: Cyclops network deployment using automated scripts  
> **Target Audience**: Network operators, deployment engineers

## Quick Start

```bash
cd /home/paul/accumulate-network/artifacts
chmod +x deploy-cyclops-network.sh
./deploy-cyclops-network.sh
```

---

## Automated Deployment Script
<!-- AI_TAG: deployment_script -->

### Script Overview
<!-- AI_TAG: script_overview -->

The `deploy-cyclops-network.sh` script provides complete automation for Cyclops network deployment, including:

- **Cleanup**: Previous deployment artifacts removal
- **Compilation**: Binary building (extract tool and accumulated)
- **Extraction**: Partition snapshot extraction from Cyclops artifacts
- **Initialization**: Network initialization with custom snapshots
- **Configuration**: Node configuration and startup

### Script Location

```bash
/home/paul/accumulate-network/artifacts/deploy-cyclops-network.sh
```

### Script Features
<!-- AI_TAG: script_features -->

#### 1. Environment Setup
- **Working Directory**: `/home/paul/accumulate-network`
- **Artifacts Path**: `/home/paul/accumulate-network/artifacts`
- **Nodes Directory**: `/home/paul/accumulate-network/nodes`
- **Snapshots Output**: `/tmp/partition-snapshots`

#### 2. Cleanup Phase
```bash
# Removes previous deployment artifacts
rm -rf nodes/
rm -rf /tmp/partition-snapshots/
```

#### 3. Binary Compilation
```bash
# Compiles snapshot extraction tool
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate
go build -o /home/paul/accumulate-network/nodes/extract ./tools/cmd/analyze

# Compiles accumulated binary
go build -o /home/paul/accumulate-network/nodes/accumulated ./cmd/accumulated
```

#### 4. Partition Snapshot Extraction
```bash
# Extracts DN and BVN partition snapshots from Cyclops artifacts
./extract \
  --snapshot /home/paul/accumulate-network/artifacts/cyclops-genesis.snap \
  --network /home/paul/accumulate-network/artifacts/network.json \
  --output /tmp/partition-snapshots
```

**Output**:
- `Directory-partition.snap` (~2GB) - DN partition with all accounts, transactions, messages
- `bvn-cyclops-partition.snap` (~0MB) - BVN partition (empty as expected)

#### 5. Network Initialization
```bash
# Creates network genesis with custom snapshots
./accumulated init network /home/paul/accumulate-network/artifacts/network.json \
  --work-dir /home/paul/accumulate-network/nodes \
  --snapshot /tmp/partition-snapshots/Directory-partition.snap \
  --snapshot /tmp/partition-snapshots/bvn-cyclops-partition.snap
```

**Creates**:
- `dn-genesis.snap` - Directory Network genesis
- `bvn1-genesis.snap` - BVN genesis
- Network configuration files

#### 6. Node Configuration
```bash
# Initialize dual node (DN + BVN) configuration
./accumulated init dual Directory.cyclops \
  --work-dir /home/paul/accumulate-network/nodes
```

**Creates**:
- `accumulate.toml` - Node configuration file
- Peer connection settings
- Network participation configuration

#### 7. Network Launch
```bash
# Start the network with logging
./accumulated run --work-dir /home/paul/accumulate-network/nodes 2>&1 | tee network.log
```

## Script Usage
<!-- AI_TAG: script_usage -->

### Prerequisites

1. **Cyclops Artifacts**: Ensure artifacts are in `/home/paul/accumulate-network/artifacts/`:
   - `cyclops-genesis.snap` (original snapshot)
   - `network.json` (network configuration)

2. **Go Environment**: Accumulate source code at `/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate`

### Running the Script

```bash
cd /home/paul/accumulate-network/artifacts
chmod +x deploy-cyclops-network.sh
./deploy-cyclops-network.sh
```

### Expected Output

1. **Cleanup confirmation**: Previous deployments removed
2. **Compilation success**: Binaries built successfully
3. **Extraction statistics**: 
   - DN partition: ~2,974,812 records, ~2GB
   - BVN partition: 0 records, ~0MB
4. **Network initialization**: Genesis files created
5. **Node configuration**: `accumulate.toml` generated
6. **Network startup**: Nodes begin running with log output

## Script Monitoring
<!-- AI_TAG: script_monitoring -->

### Log Files
- `network.log` - Complete network runtime logs
- Console output shows real-time status

### Success Indicators
```bash
# Check for successful partition extraction
ls -lh /tmp/partition-snapshots/
# Should show ~2GB Directory-partition.snap

# Check for genesis files
ls -lh /home/paul/accumulate-network/nodes/*-genesis.snap

# Check configuration
ls /home/paul/accumulate-network/nodes/accumulate.toml
```

## Troubleshooting
<!-- AI_TAG: deployment_troubleshooting -->

### Compilation Errors
- **Issue**: Go build failures
- **Solutions**:
  - Ensure Go environment is properly set up
  - Check Accumulate source code is at expected path
  - Verify all dependencies are available

### Extraction Failures
- **Issue**: Partition snapshot extraction fails
- **Solutions**:
  - Verify `cyclops-genesis.snap` exists and is readable
  - Check `network.json` format and content
  - Ensure sufficient disk space in `/tmp`

### Network Initialization Errors
- **Issue**: `accumulated init network` fails
- **Solutions**:
  - Verify partition snapshots were created successfully
  - Check network configuration JSON syntax
  - Ensure working directory permissions are correct

### Node Startup Issues
- **Issue**: `accumulated run` fails to start
- **Solutions**:
  - Verify `accumulate.toml` was created
  - Check genesis files exist and are valid
  - Monitor `network.log` for specific error messages

## Script Customization
<!-- AI_TAG: script_customization -->

The script can be modified for different environments by updating:

```bash
# Path variables at top of script
ACCUMULATE_REPO="/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate"
WORK_DIR="/home/paul/accumulate-network"
ARTIFACTS_DIR="/home/paul/accumulate-network/artifacts"
NODES_DIR="/home/paul/accumulate-network/nodes"
SNAPSHOT_OUTPUT="/tmp/partition-snapshots"
```

### Environment Variables

| Variable | Purpose | Default Value |
|----------|---------|---------------|
| `ACCUMULATE_REPO` | Source code location | `/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate` |
| `WORK_DIR` | Base working directory | `/home/paul/accumulate-network` |
| `ARTIFACTS_DIR` | Cyclops artifacts location | `/home/paul/accumulate-network/artifacts` |
| `NODES_DIR` | Node configuration output | `/home/paul/accumulate-network/nodes` |
| `SNAPSHOT_OUTPUT` | Partition snapshot output | `/tmp/partition-snapshots` |

## Manual Deployment Process
<!-- AI_TAG: manual_deployment -->

For environments where the automated script cannot be used, follow these manual steps:

### Step 1: Environment Preparation

```bash
# Create working directories
mkdir -p /home/paul/accumulate-network/nodes
mkdir -p /tmp/partition-snapshots

# Clean previous deployments
rm -rf /home/paul/accumulate-network/nodes/*
rm -rf /tmp/partition-snapshots/*
```

### Step 2: Binary Compilation

```bash
# Navigate to source directory
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate

# Build extraction tool
go build -o /home/paul/accumulate-network/nodes/extract ./tools/cmd/analyze

# Build accumulated daemon
go build -o /home/paul/accumulate-network/nodes/accumulated ./cmd/accumulated
```

### Step 3: Partition Extraction

```bash
# Navigate to nodes directory
cd /home/paul/accumulate-network/nodes

# Extract partition snapshots
./extract \
  --snapshot /home/paul/accumulate-network/artifacts/cyclops-genesis.snap \
  --network /home/paul/accumulate-network/artifacts/network.json \
  --output /tmp/partition-snapshots
```

### Step 4: Network Initialization

```bash
# Initialize network with custom snapshots
./accumulated init network /home/paul/accumulate-network/artifacts/network.json \
  --work-dir /home/paul/accumulate-network/nodes \
  --snapshot /tmp/partition-snapshots/Directory-partition.snap \
  --snapshot /tmp/partition-snapshots/bvn-cyclops-partition.snap
```

### Step 5: Node Configuration

```bash
# Configure dual node setup
./accumulated init dual Directory.cyclops \
  --work-dir /home/paul/accumulate-network/nodes
```

### Step 6: Network Launch

```bash
# Start the network
./accumulated run --work-dir /home/paul/accumulate-network/nodes 2>&1 | tee network.log
```

## Integration Benefits
<!-- AI_TAG: integration_benefits -->

### Automation Advantages

1. **Consistency**: Eliminates manual configuration errors
2. **Repeatability**: Same process every deployment
3. **Speed**: Automated execution reduces deployment time
4. **Reliability**: Tested sequence of operations
5. **Monitoring**: Built-in logging and status reporting

### Manual Process Integration

The script automates the entire manual process:
1. **Manual Step 1-2**: Automated cleanup and compilation
2. **Manual Step 3**: Automated partition snapshot extraction
3. **Manual Step 4**: Automated network initialization with custom snapshots
4. **Manual Step 5**: Automated node configuration and startup

This provides a reliable, repeatable deployment process that eliminates manual errors and ensures consistent network initialization.

## Production Considerations
<!-- AI_TAG: production_considerations -->

### Security
- Review script contents before execution
- Ensure proper file permissions on artifacts
- Validate network configuration before deployment

### Performance
- Monitor disk space during extraction (~2GB+ required)
- Consider SSD storage for better I/O performance
- Allocate sufficient memory for snapshot processing

### Monitoring
- Set up log rotation for `network.log`
- Monitor node health after deployment
- Implement alerting for deployment failures

### Backup
- Backup original artifacts before deployment
- Keep copies of working configurations
- Document any customizations made to the script

---

## Related Documentation

- [MainNet Reference](../network/accumulate-mainnet-reference.md) - Network specifications and configuration
- [Node Daemon Commands](../api/accumulated-daemon-commands.md) - `accumulated` command reference
- [Network Glossary](../network/accumulate-network-glossary.md) - Terminology definitions
