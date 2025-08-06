# Automated Cyclops Network Deployment

This document describes the automated deployment process for the Cyclops network, including partition snapshot extraction and network initialization.

## Quick Start - Automated Deployment

The fastest way to deploy the Cyclops network is using the automated deployment script:

```bash
cd ~/accumulate-network/artifacts
./deploy-cyclops-network.sh
```

This single command will:
1. Clean up any previous deployment attempts
2. Recreate directories and initialize the network structure
3. Compile the latest version of the extract tool
4. Compile the accumulated binary
5. Extract partition snapshots from the unified Cyclops genesis snapshot
6. Initialize the network with the partition snapshots
7. Boot the Cyclops network

## What the Script Does

### Step 1: Cleanup Previous Deployments
- Removes existing `~/accumulate-network/nodes` directory
- Removes existing `/tmp/partition-snapshots` directory  
- Stops any running accumulated processes
- Ensures a clean deployment environment

### Step 2: Directory Initialization
- Creates the nodes directory structure
- Copies or prepares the accumulated binary
- Sets up the deployment environment

### Step 3: Compilation
- Builds the latest extract tool from source: `go build -o analyze ./tools/cmd/analyze`
- Builds the accumulated binary: `go build -o accumulated ./cmd/accumulated`
- Ensures all tools are up-to-date with the latest fixes

### Step 4: Partition Snapshot Extraction
- Uses the fixed extract tool to create partition-specific snapshots
- Processes the unified `cyclops-genesis.snap` (2.1GB) from artifacts
- Creates `Directory-partition.snap` (~1.3GB) and `bvn-cyclops-partition.snap` if applicable
- Validates that snapshots contain the expected data (not empty)

### Step 5: Network Initialization
- Runs `accumulated network init` with the Cyclops network configuration
- Copies partition snapshots to appropriate locations:
  - `Directory-partition.snap` → `dn-genesis.snap`
  - `bvn-cyclops-partition.snap` → `bvn1-genesis.snap`
- Configures the network for the Cyclops topology

### Step 6: Network Boot
- Starts the Cyclops network using `accumulated run`
- Runs in background with logging to `cyclops-network.log`
- Provides network status checking and monitoring information

## Manual Steps (Alternative)

If you prefer to run the steps manually or need to troubleshoot:

### 1. Clean Previous Attempts
```bash
# Remove previous deployment
rm -rf ~/accumulate-network/nodes
rm -rf /tmp/partition-snapshots
pkill -f accumulated
```

### 2. Compile Tools
```bash
cd /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate

# Build extract tool
go build -o analyze ./tools/cmd/analyze

# Build accumulated
go build -o accumulated ./cmd/accumulated
```

### 3. Extract Partition Snapshots
```bash
# Create partition snapshots from unified snapshot
./analyze extract ~/accumulate-network/artifacts/cyclops-network.json ~/accumulate-network/artifacts/cyclops-genesis.snap

# Verify snapshots were created
ls -lh /tmp/partition-snapshots/
```

### 4. Initialize Network
```bash
cd ~/accumulate-network/nodes
cp /home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate/accumulated ./

# Initialize with Cyclops configuration
./accumulated network init ~/accumulate-network/artifacts/cyclops-network.json

# Copy partition snapshots
cp /tmp/partition-snapshots/Directory-partition.snap ./dn-genesis.snap
cp /tmp/partition-snapshots/bvn-cyclops-partition.snap ./bvn1-genesis.snap  # if exists
```

### 5. Start Network
```bash
cd ~/accumulate-network/nodes
./accumulated run
```

## Monitoring and Management

### Check Network Status
```bash
# API status check
curl http://127.0.0.1:26660/v2/status

# View logs
tail -f ~/accumulate-network/nodes/cyclops-network.log
```

### Stop Network
```bash
# Stop the network
pkill -f accumulated

# Or if you have the PID
kill $(cat ~/accumulate-network/nodes/cyclops-network.pid)
```

## Troubleshooting

### Empty Partition Snapshots
The extract tool has been fixed to resolve the empty partition snapshot issue. If you encounter empty snapshots:
- Ensure you're using the latest version of the extract tool
- Check that the consensus section writing is using the simple approach (not the complex JSON/Binary fallback)
- Verify the unified snapshot file exists and is not corrupted

### Network Startup Issues
- Check the log file: `~/accumulate-network/nodes/cyclops-network.log`
- Verify configuration files were created properly
- Ensure partition snapshots are in the correct locations with proper sizes

### Port Conflicts
If you get port binding errors:
- Check for existing accumulated processes: `pgrep -f accumulated`
- Stop conflicting processes: `pkill -f accumulated`
- Verify ports 26656, 26657, 26660 are available

## Key Improvements

This automated deployment process includes several key improvements:

1. **Fixed Partition Snapshot Extraction**: The extract tool now properly creates partition snapshots with the expected data instead of empty files
2. **Automated Cleanup**: Ensures clean deployment environment by removing previous attempts
3. **Comprehensive Validation**: Checks for required files and validates snapshot creation
4. **Background Execution**: Network runs in background with proper logging and PID management
5. **Status Monitoring**: Provides tools and commands for monitoring network health

## Network Configuration

The deployment uses the Cyclops network configuration from `~/accumulate-network/artifacts/cyclops-network.json` which defines:
- Network ID: "cyclops"
- Directory Network (DN) partition
- BVN "bvn-cyclops" partition  
- Validator configurations
- Network topology

The partition snapshots ensure that each partition starts with the correct subset of accounts and data from the unified genesis snapshot.
