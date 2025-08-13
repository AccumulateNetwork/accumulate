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

# Full deployment
./deploy-cyclops-network.sh

# Skip extraction (if snapshots exist)
./deploy-cyclops-network.sh --skip-extract

# Start from specific step
./deploy-cyclops-network.sh --start-from init

# Show help
./deploy-cyclops-network.sh --help
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

#### 1. Command-Line Options
- `--start-from STEP`: Start from specific step (cleanup|compile|extract|init|boot)
- `--skip-extract`: Skip partition extraction (assumes snapshots exist)
- `--help`: Show usage information and exit

#### 2. Environment Setup
- **Working Directory**: `/home/paul/accumulate-network`
- **Artifacts Path**: `/home/paul/accumulate-network/artifacts`
- **Nodes Directory**: `/home/paul/accumulate-network/nodes`
- **Snapshots Output**: `/tmp/partition-snapshots`

#### 3. Cleanup Phase
```bash
# Removes previous deployment artifacts
rm -rf nodes/
rm -rf /tmp/partition-snapshots/
```

#### 4. Binary Compilation
```bash
# Compiles snapshot extraction tool
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate
go build -o /home/paul/accumulate-network/nodes/extract ./tools/cmd/analyze

# Compiles accumulated binary
go build -o /home/paul/accumulate-network/nodes/accumulated ./cmd/accumulated
```

#### 5. Partition Snapshot Extraction
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

#### 6. Network Initialization
```bash
# Creates network genesis with custom snapshots (CRITICAL: --work-dir flag required)
./accumulated init network --work-dir "$NODES_DIR" \
  "$ARTIFACTS_DIR/cyclops-network.json" \
  --snapshot /tmp/partition-snapshots/Directory-partition.snap \
  --snapshot /tmp/partition-snapshots/bvn-cyclops-partition.snap
```

**⚠️ CRITICAL FIX**: The `--work-dir` flag is **REQUIRED** to ensure configuration files are created in the correct directory. Without this flag, `accumulate.toml` files will be created in the wrong location causing deployment failure.

**Creates**:
- Node directories (e.g., `bvn1-1/`)
- `accumulate.toml` in each node directory
- `directory-genesis.snap` and `bvn-cyclops-genesis.snap` files
- Network configuration files

#### 7. Individual Node Initialization
```bash
# Initialize Tendermint configurations for each node
for node_dir in */; do
    if [ -d "$node_dir" ] && [ -f "${node_dir}accumulate.toml" ]; then
        cd "$NODES_DIR/$node_dir"
        "$NODES_DIR/accumulated" init node --work-dir "$NODES_DIR/$node_dir"
        cd "$NODES_DIR"
    fi
done
```

**Creates**:
- `Node0/config/tendermint.toml` - Tendermint configuration
- `Node0/data/` - Node data directory
- Additional Tendermint configuration files

#### 8. Multi-Node Network Launch
```bash
# Start each node with proper parameters
for node_dir in "${node_dirs[@]}"; do
    cd "$NODES_DIR/$node_dir"
    nohup "$NODES_DIR/accumulated" run --node 0 --work-dir "$NODES_DIR/$node_dir" \
        > "../cyclops-${node_dir}.log" 2>&1 &
    pids+=("$!")
done
```

**⚠️ CRITICAL FIXES**:
- `--node 0` parameter is **REQUIRED** to specify node index
- `--work-dir` parameter is **REQUIRED** for proper configuration location
- Multi-node architecture: Script handles multiple node directories created by `init network`
- Individual log files per node for better debugging

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

### Critical Deployment Fixes

#### 1. Missing accumulate.toml Files
**Error**: `No accumulate.toml found in nodes directory`

**Root Cause**: `accumulated init network` command missing `--work-dir` flag

**Solution**:
```bash
# WRONG (creates files in default location)
./accumulated init network network.json

# CORRECT (creates files in specified directory)
./accumulated init network --work-dir "$NODES_DIR" network.json
```

**Impact**: This was the primary cause of deployment failures. The `--work-dir` flag is **MANDATORY**.

#### 2. Missing Tendermint Configuration
**Error**: `open /path/to/Node0/config/tendermint.toml: no such file or directory`

**Root Cause**: Individual node initialization not performed after network initialization

**Solution**:
```bash
# After network initialization, initialize each node
for node_dir in */; do
    if [ -d "$node_dir" ] && [ -f "${node_dir}accumulate.toml" ]; then
        cd "$NODES_DIR/$node_dir"
        "$NODES_DIR/accumulated" init node --work-dir "$NODES_DIR/$node_dir"
    fi
done
```

**Status**: ⚠️ **KNOWN ISSUE** - This step may still fail. Alternative: manually create directory structure.

#### 3. Node Startup Command Errors
**Error**: `accumulated run` shows help instead of starting

**Root Cause**: Missing required parameters for node startup

**Solution**:
```bash
# WRONG (missing required parameters)
./accumulated run

# CORRECT (with node index and working directory)
./accumulated run --node 0 --work-dir "$NODES_DIR/$node_dir"
```

#### 4. Multi-Node Architecture Issues
**Error**: Script assumes single node but `init network` creates multiple node directories

**Root Cause**: Script used single `NETWORK_PID` variable instead of arrays

**Solution**: Updated script to handle arrays of node directories and PIDs:
```bash
# Global arrays for multi-node support
node_dirs=()
pids=()

# Health validation for multiple nodes
for i in "${!pids[@]}"; do
    if ! kill -0 "${pids[i]}" 2>/dev/null; then
        echo "Node ${node_dirs[i]} died"
    fi
done
```

### Command-Line Options for Debugging

#### Skip Time-Consuming Steps
```bash
# Skip 58-second extraction step if snapshots exist
./deploy-cyclops-network.sh --skip-extract

# Start from network initialization (skip compilation)
./deploy-cyclops-network.sh --start-from init

# Start from boot only (for testing node startup)
./deploy-cyclops-network.sh --start-from boot
```

#### Deployment Step Breakdown
- `cleanup`: Remove previous deployment artifacts
- `compile`: Build extract tool and accumulated binary
- `extract`: Create partition snapshots (58 seconds)
- `init`: Initialize network and node configurations
- `boot`: Start network nodes

### Common Issues and Solutions

#### Extraction Failures
**Issue**: Partition snapshot extraction fails

**Solutions**:
- Verify `cyclops-genesis.snap` exists and is readable
- Check `network.json` format and content
- Ensure sufficient disk space in `/tmp` (~4GB required)
- Check extraction tool compilation: `ls -la nodes/extract`

#### Network Initialization Errors
**Issue**: `accumulated init network` fails

**Solutions**:
- **CRITICAL**: Always use `--work-dir` flag
- Verify partition snapshots were created successfully
- Check network configuration JSON syntax
- Ensure working directory permissions are correct
- Verify accumulated binary exists: `ls -la nodes/accumulated`

#### Node Startup Problems
**Issue**: Network fails to start or nodes crash immediately

**Diagnostic Steps**:
1. Check individual node logs: `tail -20 cyclops-*.log`
2. Verify Tendermint configuration exists: `ls -la */Node0/config/`
3. Check node startup command parameters
4. Verify port availability (default: 26656, 26657)
5. Ensure proper file permissions on configuration files

**Solutions**:
- Use correct `accumulated run` parameters with `--node` and `--work-dir`
- Create missing Tendermint directories manually if needed
- Check system resources (memory, disk space)
- Review node-specific log files for detailed error messages

#### Health Check Failures
**Issue**: Nodes die during startup validation

**Diagnostic Commands**:
```bash
# Check if node processes are running
ps aux | grep accumulated

# Check last lines of node logs
tail -10 /home/paul/accumulate-network/nodes/cyclops-*.log

# Check network connectivity
curl -s http://localhost:26657/status || echo "API not responding"
```

### Log File Locations
- **Node Logs**: `/home/paul/accumulate-network/nodes/cyclops-{node-name}.log`
- **Extraction Logs**: Console output during extraction step
- **Network Logs**: Individual per-node log files

### Performance Considerations
- **Extraction Time**: ~58 seconds for 2GB snapshot
- **Memory Usage**: ~2-4GB during extraction
- **Disk Space**: ~6GB total (snapshots + nodes + artifacts)
- **Startup Time**: 30-60 seconds for network health validations

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
# Initialize network with custom snapshots (CRITICAL: --work-dir flag required)
./accumulated init network --work-dir /home/paul/accumulate-network/nodes \
  /home/paul/accumulate-network/artifacts/cyclops-network.json \
  --snapshot /tmp/partition-snapshots/Directory-partition.snap \
  --snapshot /tmp/partition-snapshots/bvn-cyclops-partition.snap
```

**⚠️ CRITICAL**: The `--work-dir` flag must come **before** the network configuration file path.

### Step 5: Individual Node Initialization

```bash
# Navigate to nodes directory
cd /home/paul/accumulate-network/nodes

# Initialize Tendermint configurations for each node
for node_dir in */; do
    if [ -d "$node_dir" ] && [ -f "${node_dir}accumulate.toml" ]; then
        node_name="${node_dir%/}"
        echo "Initializing node: $node_name"
        cd "$node_name"
        ../accumulated init node --work-dir "/home/paul/accumulate-network/nodes/$node_name" || {
            echo "Node init failed, creating basic structure"
            mkdir -p Node0/config Node0/data
        }
        cd ..
    fi
done
```

### Step 6: Multi-Node Network Launch

```bash
# Start each node with proper parameters
cd /home/paul/accumulate-network/nodes

# Find all node directories
node_dirs=()
for dir in */; do
    if [ -d "$dir" ] && [ -f "${dir}accumulate.toml" ]; then
        node_dirs+=("${dir%/}")
    fi
done

# Start each node
pids=()
for node_dir in "${node_dirs[@]}"; do
    echo "Starting node: $node_dir"
    cd "$node_dir"
    nohup ../accumulated run --node 0 --work-dir "/home/paul/accumulate-network/nodes/$node_dir" \
        > "../cyclops-${node_dir}.log" 2>&1 &
    pids+=("$!")
    echo "Node $node_dir started (PID: $!)" 
    cd ..
done

# Monitor node health
echo "Monitoring ${#pids[@]} nodes..."
for i in "${!pids[@]}"; do
    if kill -0 "${pids[i]}" 2>/dev/null; then
        echo "✓ Node ${node_dirs[i]} running (PID: ${pids[i]})"
    else
        echo "✗ Node ${node_dirs[i]} failed to start"
        echo "Last 10 lines of log:"
        tail -10 "cyclops-${node_dirs[i]}.log"
    fi
done
```

**⚠️ CRITICAL PARAMETERS**:
- `--node 0`: Specifies node index (required)
- `--work-dir`: Specifies working directory (required)
- Individual log files per node for debugging

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
