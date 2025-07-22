# Accumulate Network Initialization Guide

This document provides a comprehensive guide on how to initialize an Accumulate network using the `accumulated init genesis` command with partition snapshots. It covers the command's implementation, usage, and provides examples using the cyclops network artifacts.

## Table of Contents

1. [Overview](#overview)
2. [Command Syntax](#command-syntax)
3. [Network Configuration File](#network-configuration-file)
4. [Partition Snapshots](#partition-snapshots)
5. [Step-by-Step Usage Guide](#step-by-step-usage-guide)
6. [Technical Implementation Details](#technical-implementation-details)
7. [Examples with Cyclops Network Artifacts](#examples-with-cyclops-network-artifacts)
8. [Initializing a Mainnet Network from Existing Snapshots](#initializing-a-mainnet-network-from-existing-snapshots)
9. [Troubleshooting](#troubleshooting)

## Overview

The `accumulated init genesis` command is used to initialize a new Accumulate network by generating genesis files for each partition in the network. It takes a network configuration file as input and can optionally include partition snapshots to initialize the network state.

The command generates binary snapshot files (`.snap`) and JSON representations (`.json`) for each partition defined in the network configuration. These files are then used by individual nodes to initialize their state.

## Command Syntax

```bash
accumulated init genesis <network configuration file> [flags]
```

### Flags

- `-w, --work-dir <directory>`: Directory where the genesis files will be written (default: current directory)
- `--snapshot <file>`: Path to a snapshot file (can be specified multiple times for different partition snapshots)
- `--factom-addresses <file>`: Path to a file containing Factom addresses (optional)
- `--faucet-seed <seed>`: Seed for generating a faucet account (optional)

## Network Configuration File

The network configuration file is a JSON file that defines the network structure, including partitions, validators, and other network parameters. Here's an example of a network configuration file from the cyclops network:

```json
{
  "id": "cyclops",
  "template": "[[configurations]]\n  type = \"coreValidator\"\n  enable-healing = false\n  enable-snapshots = false\n  storage-type = \"leveldb\"\n\n[storage]\n  type = \"leveldb\"\n  path = \"data/accumulate.db\"",

  "globals": {
    "oracle": {
      "price": 5000
    },
    "globals": {
      "majorBlockSchedule": "0 0 * * *",
      "executorVersion": "v2baikonur",
      "feeSchedule": {
        "createIdentitySliding": [4800000, 1200000, 350000, 90000, 25000, 7000, 1800],
        "createSubIdentity": 2500,
        "createTokenAccount": 10000,
        "createDataAccount": 10000,
        "createKeyPage": 10000,
        "createKeyBook": 10000,
        "sendTokens": 1000,
        "addCredits": 1000,
        "updateKeyPage": 1000,
        "updateKey": 1000,
        "writeData": 1000,
        "writeDataTo": 1000,
        "burnTokens": 1000,
        "issueTokens": 1000,
        "createToken": 50000,
        "updateAccountAuth": 1000,
        "syntheticDepositTokens": 1000,
        "syntheticDepositCredits": 1000,
        "syntheticBurnTokens": 1000,
        "syntheticCreateIdentity": 25000,
        "syntheticCreateTokenAccount": 10000,
        "syntheticWriteData": 1000
      },
      "limits": {
        "accountAuthorities": 20,
        "bookPages": 20,
        "dataEntryParts": 100,
        "dataEntryPartsCompressed": 100,
        "dataEntrySize": 100000,
        "dataEntrySizeCompressed": 100000,
        "identityAccounts": 1000,
        "pageEntries": 100,
        "pendingMajorBlocks": 28
      },
      "operatorAcceptThreshold": {
        "denominator": 3,
        "numerator": 2
      },
      "validatorAcceptThreshold": {
        "denominator": 3,
        "numerator": 2
      },
      "values": {
        "acmeSupply": 500000000000000,
        "acmePrecision": 8,
        "acmeIssuedSupply": 500000000000000
      }
    },
    "network": {
      "networkName": "cyclops",
      "partitions": [
        {
          "id": "bvn-cyclops",
          "type": "blockValidator"
        },
        {
          "id": "Directory",
          "type": "directory"
        }
      ],
      "validators": [
        {
          "operator": "acc://defidevs.acme",
          "partitions": [
            {
              "active": true,
              "id": "Directory"
            },
            {
              "active": true,
              "id": "bvn-cyclops"
            }
          ],
          "publicKey": "b7738ccd1f4cb282333de14a4c85ca5214fe63995a6068062cd36ddb0bd981d6",
          "publicKeyHash": "92aa6717e54b9436794e9280656c99ad951df061dd2c18d60cd23175e7d27ea1"
        }
      ],
      "partitionDuration": 2,
      "blockTime": 1,
      "maxWritesPerBlock": 1000,
      "maxEntriesPerBlock": 5000,
      "maxAnnouncementsPerBlock": 1000,
      "maxTransactionsPerBlock": 1000,
      "maxBlocksPerSecond": 2,
      "maxPendingTransactions": 10000,
      "maxAnchorPoints": 10000,
      "maxAnchorBuilders": 10,
      "maxDirBuilders": 10,
      "maxBlockAnchors": 10000,
      "maxDirectoryAnchors": 10000,
      "maxPendingAnchors": 10000,
      "maxAnchorsPerBlock": 1000,
      "retryLimit": 3
    }
  },
  "bvns": [
    {
      "id": "bvn-cyclops",
      "nodes": [
        {
          "listen": "tcp://127.0.0.1:16591",
          "publicKey": "4953ed885b92ec372f4f5c2aeae223a5e29a50655612421f820fe4d9adb18a3d",
          "publicKeyHash": "0b9849a358dea18962ae52d576a75592887dd6774c8a67dd7781dd1f17bb2e3f"
        }
      ]
    }
  ],
  "dn": {
    "nodes": [
      {
        "listen": "tcp://127.0.0.1:16592",
        "publicKey": "dd4d0a0e5438643036400e627fa987d47ef64f36f27c17a0df9153e2da546755",
        "publicKeyHash": "9a473613bccb2e02ebd34e9dcfdcac71bdc1ae003ea15a60ecfa8c45b258c206"
      }
    ]
  },
  "bootstrap": {
    "accounts": [
      {
        "url": "acc://defidevs.acme",
        "type": "identity",
        "keyBook": {
          "keys": [
            {
              "publicKey": "ad993e23f7774906eb3865ae49b2b896af57b330ebace8c5e895043d19235c56",
              "publicKeyHash": "8c56e7f997090dad989f2b4045693d6693a6b7c6f8b270b1903b9ba8671b813a"
            }
          ]
        },
        "creditBalance": 1000000,
        "tokenAccounts": [
          {
            "url": "acc://defidevs.acme/ACME",
            "tokenUrl": "acc://ACME",
            "balance": 10000000000000
          }
        ]
      }
    ]
  }
}
```

The key components of the network configuration file are:

- `id`: The network identifier
- `globals`: Global network parameters including fee schedules, limits, and network configuration
- `globals.network.partitions`: Defines the partitions in the network (Directory and Block Validator Network partitions)
- `bvns`: Block Validator Network configurations
- `dn`: Directory Network configurations
- `bootstrap`: Initial accounts to be created in the network

## Directory Structure and Artifacts

Understanding the directory structure of an Accumulate network configuration and its artifacts is crucial for successful network initialization. Below is a visual representation of the directory structure before and after running the `accumulated init genesis` command.

### Before Initialization (Input Artifacts)

```
~/accumulate-network/artifacts/
├── accumulated                   # The Accumulate executable
├── cyclops-genesis.snap         # Complete unified snapshot
├── cyclops-network.json         # Network configuration file
├── Directory-partition.snap     # Directory partition snapshot
├── bvn-cyclops-partition.snap   # BVN-Cyclops partition snapshot
└── other configuration files... # Additional configuration files
```

### After Initialization (Output Artifacts)
```
/path/to/artifacts/
├── accumulated                 # Accumulate executable
├── network.json                # Network configuration file
├── Directory-partition.snap    # Directory partition snapshot
└── bvn-cyclops-partition.snap  # BVN partition snapshot
```

### After Initialization

After running `accumulated init genesis`, the following files are generated:

```
/path/to/output/
├── directory-genesis.json      # Directory partition genesis in JSON format
├── directory-genesis.snap      # Directory partition genesis in binary format
├── bvn-cyclops-genesis.json    # BVN partition genesis in JSON format
└── bvn-cyclops-genesis.snap    # BVN partition genesis in binary format
```

### Node Directory Structure

When running `accumulated init network`, a more complex directory structure is created for each node. Looking at the codebase, we can see that each node gets its own directory with both Directory and BVN partition genesis files:

```
/path/to/output/
└── bvn1-1/                     # Node directory (format: bvn{bvn_index}-{node_index})
    ├── accumulate.toml         # Accumulate configuration
    ├── directory-genesis.snap  # Directory partition genesis snapshot
    ├── bvn-cyclops-genesis.snap # BVN partition genesis snapshot (named by BVN ID)
    ├── config/                 # Configuration directory
    │   ├── config.toml         # CometBFT configuration
    │   ├── genesis.json        # CometBFT genesis (not the same as Accumulate genesis)
    │   ├── node_key.json       # Node private key
    │   ├── priv_validator_key.json # Validator private key
    │   └── accumulate.toml     # Accumulate-specific configuration
    └── data/                   # Data directory (created but empty initially)
```

Each node in the network gets both the Directory partition genesis and its respective BVN partition genesis. The CometBFT configuration files (`config.toml`, `genesis.json`, etc.) are created in the `config/` subdirectory of each node's directory.

The `accumulated init network` command:
1. Creates a directory for each node in each BVN
2. Generates node and validator keys if not provided
3. Builds genesis documents for each partition
4. Writes the genesis documents to each node's directory
5. Creates CometBFT configuration files in the `config/` subdirectory
6. Sets up the node configuration in `accumulate.toml`

This structure ensures that each node has all the necessary configuration files and genesis data to participate in the network.

### Key Artifacts Explained

1. **Input Artifacts**:
   - `network.json`: Defines the network structure, partitions, and validators
   - `cyclops-network.json`: Defines the network structure, partitions, and validators
   - `Directory-partition.snap`: Contains accounts, transactions, and state data for the Directory partition
   - `bvn-cyclops-partition.snap`: Contains accounts, transactions, and state data for the BVN-Cyclops partition
   - `cyclops-genesis.snap`: A unified snapshot containing data for all partitions (alternative to partition-specific snapshots)

2. **Output Artifacts**:
   - `Directory.snap` and `Directory.json`: Genesis files for the Directory partition
   - `bvn-cyclops.snap` and `bvn-cyclops.json`: Genesis files for the BVN-Cyclops partition
   - `accumulate.toml`: Node configuration files with references to the genesis files

3. **Node Directory Structure**:
   - Each node has its own directory named according to its role and index
   - Each node directory contains the node configuration and references to the genesis files
   - The node data directory is created when the node starts

This directory structure ensures that each node has access to the genesis files it needs to initialize its state and participate in the network.

## Network Initialization Process Overview

### Fundamental Process

The network initialization process follows these core steps:

1. **Merge all databases together** - Collect snapshots from existing partitions
2. **Generate a new genesis document** from the merged databases
3. **On each node**:
   - Reset Tendermint's state
   - Copy over the new genesis document
   - Reboot

This process allows for network upgrades, migrations, and fresh network deployments using existing state data.

### Network Configuration Structures

The network configuration is defined by Go structs that marshal/unmarshal the `network.json` file. There are currently **two versions** in the codebase:

#### 1. Authoritative NetworkConfig Struct
**Location**: `tools/cmd/analyze/a_extract_network.go`

```go
type NetworkConfig struct {
    ID string `json:"id"`
    Globals struct {
        Oracle struct {
            Price int `json:"price"`
        } `json:"oracle"`
        
        Globals struct {
            // Add other fields as needed
        } `json:"globals"`
        
        Network struct {
            NetworkName string `json:"networkName"`
            Partitions []struct {
                ID   string `json:"id"`
                Type string `json:"type"`
            } `json:"partitions"`
            
            Validators []struct {
                Operator string `json:"operator"`
                PublicKey string `json:"publicKey"` // hex encoded
                Partitions []struct {
                    ID string `json:"id"`
                    Active bool `json:"active"`
                } `json:"partitions"`
            } `json:"validators"`
        } `json:"network"`
    } `json:"globals"`
}
```

#### 2. Enhanced networkConfig Struct (with field preservation)
**Location**: `tools/cmd/analyze/cmd_update_network_keys.go`

```go
type networkConfig struct {
    ID string `json:"id"`
    Template string `json:"template,omitempty"`
    Globals struct {
        Oracle struct {
            Price int `json:"price"`
        } `json:"oracle"`
        
        Globals json.RawMessage `json:"globals,omitempty"`
        
        Network struct {
            NetworkName string `json:"networkName"`
            Partitions []struct {
                ID   string `json:"id"`
                Type string `json:"type"`
            } `json:"partitions"`
            
            Validators []struct {
                Operator string `json:"operator"`
                PublicKey string `json:"publicKey"` // base64 encoded
                Partitions []struct {
                    ID string `json:"id"`
                    Active bool `json:"active"`
                } `json:"partitions"`
            } `json:"validators"`
        } `json:"network"`
        
        Routing json.RawMessage `json:"routing,omitempty"`
    } `json:"globals"`
}
```

**Important**: The second struct uses `json.RawMessage` for field preservation and includes additional fields (`template`, `routing`) that the first struct lacks. This inconsistency can cause data loss when marshaling/unmarshaling network configurations.

## Debug Commands for Network Initialization

### `debug snap collect`

Collects a snapshot from a database with various control options.

**Usage:**
```bash
debug snap collect <source> <destination>
```

**Example:**
```bash
debug snap collect badger:///path/to/accumulate.db /home/paul/work/acc1/unified.snap --indexed
```

**Source formats:**
- `badger:///path/to/accumulate.db` - BadgerDB database
- Other database formats supported

**Key Flags:**
- `--skip-system` - Skip system-specific data
- `--skip-bpt` - Skip BPT (Binary Patricia Tree) data
- `--indexed` - Create indexed snapshot

**Purpose:** Extracts current state from a running node's database into a portable snapshot format.

### `debug genesis ingest`

Ingests multiple partition snapshots and produces a unified database suitable for genesis.

**Usage:**
```bash
debug genesis ingest <output> <inputs...>
```

**Example:**
```bash
debug genesis ingest badger:///home/paul/work/acc1/combined.db bvn0.snap bvn1.snap bvn2.snap dn.snap
```

**Process:**
1. Reads snapshots from the Directory Network (DN) and each Block Validator Network (BVN)
2. Merges all partition data into a unified database
3. Preserves main and scratch chains for user accounts and their transactions
4. Strips system-specific data that shouldn't carry over to new network
5. Optimizes the database structure for genesis initialization

**Relevant Code:**
- `tools/cmd/debug/genesis.go` - Command implementation
- `internal/node/genesis/extract.go` - Core extraction logic

### Complete Snapshot Collection Example

```bash
# Collect unified snapshot with logging
debug snap collect badger:///home/paul/work/acc1/combined.db \
  /home/paul/work/acc1/unified.snap \
  --indexed &> /home/paul/work/acc1/unified.log
```

## Network Initialization Commands

### `accumulated init network`

Creates a new network starting with accounts from a unified snapshot.

**Usage:**
```bash
accumulated init network <config-file> [options]
```

**Example:**
```bash
accumulated init genesis network.json \
  -w dir-for-new-nodes \
  --snapshot unified.snap
```

**Process:**
1. Reads the network configuration file
2. Creates node directories and configurations
3. Generates or reuses validator keys
4. Initializes each node with the unified snapshot
5. Creates CometBFT configuration files
6. Sets up proper genesis documents for each partition

**Configuration Options:**
- **LevelDB Usage**: Passing a template configuration will configure nodes to use LevelDB
- **Healing Disabled**: Built-in healing is disabled as it's currently broken
- **Snapshots Disabled**: Automatic snapshots are disabled due to reliability issues
- **New Configuration Schema**: Automatically uses the latest configuration format

**Post-Creation Notes:**
- Nodes created later with AccMan or manual init commands need manual configuration updates
- All new nodes automatically use the new configuration schema

## Network Configuration Examples

### Kermit Network Configuration

Example of a complete network configuration for the "Kermit" test network:

```json
{
    "id": "Kermit",
    "globals": {
        "executorVersion": "v2-vandenberg",
        "oracle": {
            "price": 500000
        },
        "globals": {
            "feeSchedule": {
                "createIdentitySliding": [
                    400000
                ],
                "createSubIdentity": 10000,
                "bareIdentityDiscount": 10000
            },
            "limits": {
                "identityAccounts": 1000
            }
        },
        "routing": {
            "routes": [
                {
                    "length": 2,
                    "partition": "Chico"
                },
                {
                    "length": 2,
                    "partition": "Harpo",
                    "value": 1
                },
                {
                    "length": 2,
                    "partition": "Groucho",
                    "value": 2
                },
                {
                    "length": 3,
                    "partition": "Chico",
                    "value": 6
                },
                {
                    "length": 4,
                    "partition": "Harpo",
                    "value": 14
                },
                {
                    "length": 4,
                    "partition": "Groucho",
                    "value": 15
                }
            ]
        }
    },
    "bvns": [
        {
            "id": "Chico",
            "nodes": [
                {
                    "dnnType": "validator",
                    "bvnnType": "validator",
                    "basePort": 16591,
                    "advertizeAddress": "kermit-bvn0.accumulate.defidevs.io"
                }
            ]
        },
        {
            "id": "Harpo",
            "nodes": [
                {
                    "dnnType": "validator",
                    "bvnnType": "validator",
                    "basePort": 16591,
                    "advertizeAddress": "kermit-bvn1.accumulate.defidevs.io"
                }
            ]
        },
        {
            "id": "Groucho",
            "nodes": [
                {
                    "dnnType": "validator",
                    "bvnnType": "validator",
                    "basePort": 16591,
                    "advertizeAddress": "kermit-bvn2.accumulate.defidevs.io"
                }
            ]
        }
    ]
}
```

**Key Configuration Elements:**
- **Network ID**: Unique identifier for the network
- **Executor Version**: Specifies the execution engine version
- **Oracle Price**: Sets the oracle price for the network
- **Fee Schedule**: Defines transaction fees and costs
- **Routing Configuration**: Defines how accounts are routed to partitions
- **BVN Definitions**: Specifies Block Validator Networks and their nodes
- **Node Configuration**: Defines validator types, ports, and addresses

## Complete Network Initialization Workflow

### Step 1: Collect Existing State

```bash
# Collect snapshots from existing nodes
debug snap collect badger:///node1/accumulate.db node1.snap --skip-system
debug snap collect badger:///node2/accumulate.db node2.snap --skip-system
debug snap collect badger:///node3/accumulate.db node3.snap --skip-system
```

### Step 2: Create Unified Database

```bash
# Ingest all partition snapshots into unified database
debug genesis ingest badger:///unified/combined.db \
  bvn0.snap bvn1.snap bvn2.snap dn.snap
```

### Step 3: Generate Unified Snapshot

```bash
# Create final unified snapshot for network initialization
debug snap collect badger:///unified/combined.db \
  unified.snap --indexed
```

### Step 4: Initialize New Network

```bash
# Create new network with unified snapshot
accumulated init network network.json \
  -w new-network-nodes \
  --snapshot unified.snap
```

### Step 5: Deploy to Nodes

For each node in the new network:

1. **Reset Tendermint State**:
   ```bash
   tendermint unsafe-reset-all --home /path/to/node/config
   ```

2. **Copy Genesis Documents**:
   ```bash
   cp new-network-nodes/node1/directory-genesis.snap /path/to/node/
   cp new-network-nodes/node1/bvn-genesis.snap /path/to/node/
   ```

3. **Update Configuration**:
   ```bash
   cp new-network-nodes/node1/config/* /path/to/node/config/
   cp new-network-nodes/node1/accumulate.toml /path/to/node/
   ```

4. **Reboot Node**:
   ```bash
   systemctl restart accumulate-node
   ```

## Partition Snapshots and CometBFT Configuration

### Partition Snapshots

Partition snapshots are binary files that contain the state of a specific partition. They are used to initialize the state of a partition during network genesis. The `accumulated init genesis` command can accept multiple snapshot files, each corresponding to a different partition.

In the cyclops network artifacts, we have two partition snapshots:
- `Directory-partition.snap`: Snapshot for the Directory partition
- `bvn-cyclops-partition.snap`: Snapshot for the BVN-Cyclops partition

These snapshots contain accounts, transactions, and other state data specific to each partition. When initializing a network with these snapshots, the command processes each snapshot and filters the accounts based on partition membership using the routing table.

### Relationship Between Partitions and Node Configuration

Each node in an Accumulate network runs two CometBFT instances:

1. **Directory Network (DN) Instance**: Processes transactions for the Directory partition
2. **Block Validator Network (BVN) Instance**: Processes transactions for a specific BVN partition

This is why each node directory contains both the Directory partition genesis snapshot and a BVN partition genesis snapshot. The node needs both to participate in consensus for both networks.

### CometBFT Configuration Files

The CometBFT configuration files are created during the `accumulated init network` process and stored in the `config/` subdirectory of each node's directory. These files include:

- `config.toml`: Contains CometBFT-specific configuration
- `genesis.json`: Contains the initial validator set and other genesis information
- `node_key.json`: Contains the node's private key for P2P communication
- `priv_validator_key.json`: Contains the validator's private key for signing blocks
- `accumulate.toml`: Contains Accumulate-specific configuration including references to the genesis snapshots

### Accumulate Configuration Structure

The `accumulate.toml` file must follow a specific structure to be correctly parsed by the daemon. The most critical part is that the partition type, network ID, and partition ID must be inside a `[describe]` section.

#### Common Configuration Error

This is an **incorrect** configuration structure that will cause the "unknown partition type PartitionType:0" error:

```toml
# Essential Accumulate Node Configuration

# Network type must be one of: directory, blockvalidator, blocksummary, bootstrap
type = "blockvalidator"

# Network and partition identification
network = { id = "cyclops" }
partition-id = "bvn-cyclops"

# Genesis snapshot files
dn-genesis = "directory-genesis.snap"
bvn-genesis = "bvn-cyclops-genesis.snap"

# Storage configuration
[storage]
path = "data/accumulate.db"
type = "badger"
```

The issue with this configuration is that the `type`, `network`, and `partition-id` fields are at the root level, but they need to be inside a `[describe]` section to be properly parsed by the configuration loader.

#### Correct Configuration Structure

Here's the **correct** configuration structure that will work properly:

```toml
# Essential Accumulate Node Configuration

[describe]
# Network type must be one of: directory, blockvalidator, blocksummary, bootstrap
type = "blockvalidator"  # Case-insensitive, both "blockvalidator" and "blockValidator" work

# Network and partition identification
network = { id = "cyclops" }
partition-id = "bvn-cyclops"

# Genesis snapshot files
dn-genesis = "directory-genesis.snap"
bvn-genesis = "bvn-cyclops-genesis.snap"

# Storage configuration (required)
[storage]
path = "data/accumulate.db"
type = "badger"  # Can also be "leveldb"

# Validator key
[validator-key]
address = "AS1214GRkFxJYSz6VUXCRCcjaLTvmevpnaWrqBjgpHxriMXNjxrRG"
type = "raw"
```

#### Technical Explanation

The reason this structure is required is due to how the configuration is parsed in the codebase:

1. The `Describe` struct in the configuration is defined with the TOML tag `toml:"describe"` which means these fields must be in a `[describe]` section
2. The `PartitionTypeByName` function is case-insensitive, so `"blockvalidator"` and `"blockValidator"` are both valid
3. When these fields are at the root level, the daemon reads a zero partition type enum (PartitionType:0), leading to the error

> **IMPORTANT**: The `type`, `network`, and `partition-id` fields MUST be inside the `[describe]` section. If they are at the root level, the daemon will not recognize them and will report an "unknown partition type PartitionType:0" error.

#### Real-World Example

Here's the exact configuration from a working production node:

```toml
# Accumulate Node Configuration

[describe]
type = "directory"
network = { id = "cyclops" }
partition-id = "Directory"

[storage]
type = "badger"
path = "data/accumulate.db"

[validator-key]
type = "raw"
address = "AS1214GRkFxJYSz6VUXCRCcjaLTvmevpnaWrqBjgpHxriMXNjxrRG"

[tendermint]
proxy-app = "tcp://127.0.0.1:26658"
moniker = "directory"
fast-sync = true
db-backend = "goleveldb"
db-dir = "data"
log-level = "info"
log-format = "plain"
genesis-file = "config/genesis.json"
private-key-file = "config/priv_validator_key.json"
node-key-file = "config/node_key.json"
abci = "socket"
filter-peers = false

[tendermint.rpc]
laddr = "tcp://0.0.0.0:26657"
cors-allowed-origins = []
cors-allowed-methods = [ "HEAD", "GET", "POST" ]
cors-allowed-headers = [ "Origin", "Accept", "Content-Type", "X-Requested-With", "X-Server-Time" ]

[tendermint.p2p]
laddr = "tcp://0.0.0.0:26656"
external-address = ""
seed-mode = false
seeds = ""
persistent-peers = "6e9e89f4a5ddb9c2da55e7d6d9b1e4c2a1c4b06e@directory:26656"
upnp = false
addrbook-file = "config/addrbook.json"
addrbook-strict = true
max-num-inbound-peers = 40
max-num-outbound-peers = 10
unconditional-peer-ids = ""
persistent-peers-max-dial-period = "0s"
flush-throttle-timeout = "100ms"
max-packet-msg-payload-size = 1024
send-rate = 5120000
recv-rate = 5120000
pex = true
seed-mode = false
private-peer-ids = ""
allow-duplicate-ip = false
handshake-timeout = "20s"
dial-timeout = "3s"

[tendermint.mempool]
recheck = true
broadcast = true
wal-dir = ""
size = 5000
max-txs-bytes = 1073741824
cache-size = 10000
keep-invalid-txs-in-cache = false
max-tx-bytes = 1048576
max-batch-bytes = 0

[tendermint.fastsync]
version = "v0"

[tendermint.consensus]
wal-file = "data/cs.wal/wal"
timeout-propose = "3s"
timeout-propose-delta = "500ms"
timeout-prevote = "1s"
timeout-prevote-delta = "500ms"
timeout-precommit = "1s"
timeout-precommit-delta = "500ms"
timeout-commit = "1s"
skip-timeout-commit = false
create-empty-blocks = true
create-empty-blocks-interval = "0s"
peer-gossip-sleep-duration = "100ms"
peer-query-maj23-sleep-duration = "2s"

[tendermint.tx-index]
indexer = "kv"

[tendermint.instrumentation]
prometheus = false
prometheus-listen-addr = ":26660"
max-open-connections = 3
namespace = "tendermint"
```

The `WriteNodeFiles` function in the codebase is responsible for creating these files. It:

1. Creates the necessary directories (`config/` and `data/`)
2. Writes the CometBFT configuration files using `config.Store()`
3. Creates or loads the private validator key
4. Creates or loads the node key
5. Writes the genesis file

### How Partition Snapshots Are Used

During network initialization, the partition snapshots are processed as follows:

1. The `accumulated init genesis` command processes each snapshot and filters accounts based on partition membership
2. It generates partition-specific genesis snapshots (e.g., `directory-genesis.snap` and `bvn-cyclops-genesis.snap`)
3. The `accumulated init network` command copies these genesis snapshots to each node's directory
4. When a node starts, it loads the appropriate genesis snapshot based on its role (DN or BVN)
5. The CometBFT instances use their respective genesis files to initialize their state

### Relationship Between DN and BVN Partitions

The Accumulate network architecture consists of two types of partitions that work together:

#### Directory Network (DN)

The Directory Network is a single, global partition that:

- Maintains the global routing table for the entire network
- Stores and manages the ADI (Accumulate Digital Identifier) registry
- Handles cross-partition transactions and anchoring
- Serves as the coordination layer for the entire network
- Contains system accounts and global configuration

#### Block Validator Networks (BVNs)

Block Validator Networks are multiple, independent partitions that:

- Process transactions for specific accounts based on routing rules
- Maintain their own state and consensus
- Anchor their state to the Directory Network for security
- Can be added or removed from the network as needed
- Each BVN handles a subset of the total accounts in the network

#### How They Work Together

1. **Routing**: When a transaction is submitted to the network, the routing table (maintained by the DN) determines which BVN should process it based on the account URL.

2. **Account Distribution**: Accounts are distributed across BVNs based on routing rules. The Directory partition contains system accounts and the ADI registry, while user accounts are distributed across BVNs.

3. **Node Operation**: Each node in the network runs two CometBFT instances - one for the DN and one for its assigned BVN. This is why each node directory contains both genesis snapshots.

4. **Anchoring**: BVNs periodically anchor their state to the DN, creating a hierarchical security model where the security of all BVNs is enhanced by the security of the DN.

5. **Consensus**: Each partition (DN and BVNs) runs its own independent consensus process using CometBFT, but they are coordinated through the anchoring process.

This architecture allows Accumulate to achieve high scalability by distributing transaction processing across multiple BVNs while maintaining security and coordination through the Directory Network.

### Visual Representation

Here's a simplified diagram of how the Directory Network and BVN partitions relate to the node directory structure:

```
                                 Network Initialization
                                          |
                    +---------------------+---------------------+
                    |                                           |
            Directory Partition                          BVN Partitions
         (directory-genesis.snap)                 (bvn-cyclops-genesis.snap)
                    |                                           |
                    +---------------------+---------------------+
                                          |
                                    Node Directory
                                          |
                                     bvn1-1/
                                          |
        +-----------------------------+---+---+-----------------------------+
        |                             |       |                             |
  accumulate.toml          directory-genesis.snap       bvn-cyclops-genesis.snap
        |                             |       |                             |
        |                             |       |                             |
        |                        config/     |                             |
        |                             |       |                             |
        |       +---------------------+       |                             |
        |       |                             |                             |
        |  config.toml                        |                             |
        |  genesis.json                       |                             |
        |  node_key.json                      |                             |
        |  priv_validator_key.json            |                             |
        |  accumulate.toml                    |                             |
        |                                     |                             |
        +---------------------+---------------+-----------------------------+
                              |
                            data/
                         (empty initially)
```

This diagram illustrates how:

1. The network initialization process starts with partition snapshots for both Directory and BVN partitions
2. These snapshots are processed to create genesis files for each partition
3. Each node in the network gets both the Directory and BVN genesis files in its root directory
4. The CometBFT configuration files are stored in the `config/` subdirectory
5. The node data directory is created but initially empty
6. Each node has a single directory structure that contains configuration for both Directory and BVN instances

## Step-by-Step Usage Guide

To initialize a network using the `accumulated init genesis` command with partition snapshots:

1. Prepare the network configuration file (e.g., `cyclops-network.json`)
2. Prepare the partition snapshots (e.g., `Directory-partition.snap` and `bvn-cyclops-partition.snap`)
3. Run the `accumulated init genesis` command:

```bash
accumulated init genesis cyclops-network.json -w dir-for-new-nodes --snapshot Directory-partition.snap --snapshot bvn-cyclops-partition.snap
```

4. The command will generate genesis files for each partition in the specified output directory:
   - `Directory.snap` and `Directory.json` for the Directory partition
   - `bvn-cyclops.snap` and `bvn-cyclops.json` for the BVN-Cyclops partition

5. These files can then be used to initialize individual nodes in the network.

## Technical Implementation Details

The `accumulated init genesis` command works as follows:

1. **Load Network Configuration**: The command loads the network configuration file and parses it into a `NetworkInit` struct.

2. **Process Snapshots**: For each snapshot file specified with the `--snapshot` flag, the command creates a function that opens the file and returns a `SectionReader`.

3. **Build Genesis Documents**: The command calls `BuildGenesisDocs` which:
   - Creates a map to store genesis documents for each partition
   - Adds partitions to the network definition
   - Processes each partition (Directory and BVNs)
   - For each partition, calls `genesis.Init` to initialize the partition state

4. **Initialize Partition State**: For each partition, `genesis.Init`:
   - Builds the routing table
   - Creates a database and router
   - Unpacks snapshots by calling `unpackSnapshots`
   - Creates a genesis executor
   - Executes genesis transactions
   - Collects the state into a snapshot

5. **Unpack Snapshots**: The `unpackSnapshots` function:
   - Opens each snapshot file
   - Calls `Extract` to extract accounts from the snapshot
   - Filters accounts based on partition membership using the router
   - Tracks ACME issued and accounts from snapshots

6. **Extract Accounts**: The `Extract` function:
   - Processes the snapshot to extract accounts
   - Uses the provided filter function to determine which accounts to keep
   - Returns a map of account data

7. **Write Genesis Files**: The command writes the generated genesis documents to the output directory:
   - Binary snapshot files (`.snap`)
   - JSON representations (`.json`)

## Examples with Cyclops Network Artifacts

Using the cyclops network artifacts located at `~/accumulate-network/artifacts`:

```bash
# Initialize a network with Directory and BVN-Cyclops partition snapshots
accumulated init genesis ~/accumulate-network/artifacts/cyclops-network.json -w ~/new-network --snapshot ~/accumulate-network/artifacts/Directory-partition.snap --snapshot ~/accumulate-network/artifacts/bvn-cyclops-partition.snap
```

This command will:
1. Load the cyclops network configuration from `cyclops-network.json`
2. Process the Directory partition snapshot from `Directory-partition.snap`
3. Process the BVN-Cyclops partition snapshot from `bvn-cyclops-partition.snap`
4. Generate genesis files in the `~/new-network` directory:
   - `Directory.snap` and `Directory.json` for the Directory partition
   - `bvn-cyclops.snap` and `bvn-cyclops.json` for the BVN-Cyclops partition

## Initializing a Network from Existing Snapshots: Cyclops Example

This section provides a detailed walkthrough for initializing an Accumulate network using the Cyclops network artifacts. This concrete example demonstrates the exact commands and filenames required to initialize a network from existing snapshots.

### Available Artifacts

The following artifacts are available in the `/home/paul/accumulate-network/artifacts/` directory:

```
total 4.7G
-rwxrwxr-x 1 paul paul  86M Jun 28 13:06 accumulated
-rw-rw-r-- 1 paul paul 1.4G Jul  3 13:47 bvn-cyclops-partition.snap
-rw-rw-r-- 1 paul paul 2.0G Jun 28 10:16 cyclops-genesis.snap
-rw-rw-r-- 1 paul paul 4.3K Jul  2 17:15 cyclops-network.json
-rw-rw-r-- 1 paul paul 1.3G Jul  3 13:47 Directory-partition.snap
```


1. **Network Configuration**: `cyclops-network.json` (4.3 KB) - Defines the Cyclops network topology
   - Network ID: "cyclops"
   - Contains validator configurations, fee schedules, and network parameters
   - Defines both Directory Network and BVN-Cyclops partitions

2. **Partition Snapshots**:
   - `Directory-partition.snap` (1.3 GB) - Directory Network partition snapshot
   - `bvn-cyclops-partition.snap` (1.4 GB) - BVN partition snapshot
   - These contain the actual account data, transactions, and network state

3. **Accumulated Binary**: `accumulated` (86 MB) - The Accumulate node executable

#### 1. Prepare Working Directory

```bash
# Create a working directory
mkdir -p ~/cyclops-network
cd ~/cyclops-network

# Copy network configuration
cp /home/paul/accumulate-network/artifacts/cyclops-network.json .

# Copy the accumulated binary (if needed)
cp /home/paul/accumulate-network/artifacts/accumulated /usr/local/bin/
chmod +x /usr/local/bin/accumulated
```

#### 2. Initialize the Network Structure

Create the node directory structure and configuration files using the network configuration:

```bash
accumulated init network cyclops-network.json -w ./nodes
```

You should see a directory named `bvn1-1` for the single node that will handle both the Directory Network and BVN-Cyclops partitions.

#### 3. Copy Partition Snapshots to Node Directory

Copy the partition snapshots from the artifacts directory to the node directory with the required filenames:

```bash
# Copy Directory partition snapshot
cp /home/paul/accumulate-network/artifacts/Directory-partition.snap ./nodes/bvn1-1/directory-genesis.snap

# Copy BVN-Cyclops partition snapshot
cp /home/paul/accumulate-network/artifacts/bvn-cyclops-partition.snap ./nodes/bvn1-1/bvn-cyclops-genesis.snap
```

Verify the files were copied correctly:

```bash
ls -lh ./nodes/bvn1-1/directory-genesis.snap ./nodes/bvn1-1/bvn-cyclops-genesis.snap
```

#### 4. Verify Node Configuration

Check that the node configuration files reference the correct genesis snapshot filenames:

```bash
# Check Accumulate configuration
grep -A 1 "genesis" ./nodes/bvn1-1/config/accumulate.toml
```

Verify that the following fields are correctly set:
- `dn-genesis = "directory-genesis.snap"`
- `bvn-genesis = "bvn-cyclops-genesis.snap"`

If needed, update the configuration file:

```bash
# Update node configuration
sed -i 's/dn-genesis = .*/dn-genesis = "directory-genesis.snap"/g' "./nodes/bvn1-1/config/accumulate.toml"
sed -i 's/bvn-genesis = .*/bvn-genesis = "bvn-cyclops-genesis.snap"/g' "./nodes/bvn1-1/config/accumulate.toml"
```

Also check the CometBFT configuration to ensure it's properly set up:

```bash
# Check CometBFT configuration
cat ./nodes/bvn1-1/config/config.toml | grep -A 5 "Genesis file"
```

You should see that it references the genesis files in the node directory.

#### 5. Start the Network

Start the node in a terminal session:

```bash
# Start the node (which handles both Directory and BVN)
cd ~/cyclops-network/nodes/bvn1-1
accumulated -w . run
```

You should see output similar to:

```
I[2023-07-04|09:15:23.123] Starting Accumulate node                     module=main version=v2.0.0
I[2023-07-04|09:15:23.456] Initializing validator                       module=main
I[2023-07-04|09:15:23.789] Loading genesis from snapshot                module=main file=directory-genesis.snap
I[2023-07-04|09:15:24.123] Loading genesis from snapshot                module=main file=bvn-cyclops-genesis.snap
I[2023-07-04|09:15:25.123] Starting CometBFT consensus engine          module=main
I[2023-07-04|09:15:25.456] Starting API services                       module=main
```

The node will start up and handle both the Directory Network and BVN-Cyclops partitions as configured.
For a production deployment, you would typically create systemd service files for each node to ensure they start automatically and are properly managed.

After starting the node, wait about 30 seconds for initialization, then verify that the network is running correctly:

#### 6. Verify Network Status

1. **Check Node Status**:
   ```bash
   curl -s http://localhost:26660/v2/status | jq
   ```

   Expected output will include:
   ```json
   {
     "status": "ready",
     "network": "cyclops",
     "networkVersion": "v2-baikonur",
     "executorVersion": "v2baikonur",
     "partitions": [
       {
         "id": "Directory",
         "type": "directory"
       },
       {
         "id": "bvn-cyclops",
         "type": "blockValidator"
       }
     ],
     "ok": true
   }
   ```

2. **Verify Partition State**:
   ```bash
   curl -s http://localhost:26660/v2/describe | jq
   ```

   This will show detailed information about each partition, including:
   ```json
   {
     "network": "cyclops",
     "partitions": [
       {
         "id": "Directory",
         "type": "directory",
         "accounts": 1054598,
         "anchors": 18,
         "height": 1,
         "timestamp": "2023-07-04T09:15:30Z"
       },
       {
         "id": "bvn-cyclops",
         "type": "blockValidator",
         "accounts": 0,
         "anchors": 0,
         "height": 1,
         "timestamp": "2023-07-04T09:15:30Z"
       }
     ]
   }
   ```

3. **Validate Network Topology**:
   ```bash
   curl -s http://localhost:26660/v2/network | jq
   ```

   This will show the network's validator configuration:
   ```json
   {
     "network": "cyclops",
     "partitions": [
       {
         "id": "Directory",
         "type": "directory",
         "validators": [
           {
             "address": "tcp://node0:26656",
             "publicKey": "..."
           },
           {
             "address": "tcp://node1:26656",
             "publicKey": "..."
           }
         ]
       },
       {
         "id": "bvn-cyclops",
         "type": "blockValidator",
         "validators": [
           {
             "address": "tcp://node0:26656",
             "publicKey": "..."
           },
           {
             "address": "tcp://node1:26656",
             "publicKey": "..."
           }
         ]
       }
     ]
   }
   ```

4. **Check Account Data**:
   ```bash
   curl -s http://localhost:26660/v2/accounts?url=acc://system | jq
   ```

   This should return information about the system account, confirming that the network is properly initialized with account data from the snapshots.

### Troubleshooting Cyclops Network Initialization

1. **Snapshot File Not Found**:
   If you see an error like:
   ```
   Error: open Directory-partition.snap: no such file or directory
   ```
   Double-check that you've copied the snapshot files to the correct location and with the exact filenames.

2. **Network ID Mismatch**:
   If you see an error like:
   ```
   Error: network ID mismatch: expected "cyclops", got "mainnet"
   ```
   The network ID in the snapshots doesn't match the one in `cyclops-network.json`. Ensure both use the ID "cyclops".

3. **Memory Issues**:
   If you see errors like:
   ```
   fatal error: runtime: out of memory
   ```
   or the process is killed without an error message, the large Cyclops snapshots (2.7 GB total) require more memory than available. Ensure your system has at least 8GB of RAM, preferably 16GB for processing these large snapshots.

4. **File Size Verification**:
   You can verify the snapshot files were copied correctly by checking their sizes:
   ```bash
   ls -lh Directory-partition.snap bvn-cyclops-partition.snap
   ```
   Directory-partition.snap should be approximately 1.3 GB
   bvn-cyclops-partition.snap should be approximately 1.4 GB

5. **Genesis File Mismatch**:
   If you see an error like:
   ```
   Error: failed to load genesis: failed to load genesis doc: genesis doc file not found
   ```
   Check that the genesis files were correctly copied to each node directory and that the filenames match those specified in the configuration files.

6. **Port Conflicts**:
   If you see an error like:
   ```
   Error: [Tendermint] Failed to start server: address already in use
   ```
   Another process is already using one of the required ports (typically 26656, 26657, or 26660). Stop any existing Accumulate nodes or other services using these ports.

7. **Validator Key Issues**:
   If you see errors related to validator keys:
   ```
   Error: failed to load private validator: failed to load private validator file: key file not found
   ```
   Ensure that the `priv_validator.json` file exists in each node directory and has the correct permissions.

8. **Checking Node Logs**:
   For more detailed troubleshooting, examine the node logs:
   ```bash
   tail -f ./nodes/bvn0-0/data/accumulate.log
   ```
   This will show real-time logs from the node, which can help identify specific issues.

## Troubleshooting

### Common Issues

1. **Snapshot Not Found**: Ensure that the snapshot files exist and are accessible.
   ```
   Error: open /path/to/snapshot.snap: no such file or directory
   ```
   Solution: Check the path to the snapshot files and ensure they exist.

2. **Invalid Network Configuration**: Ensure that the network configuration file is valid JSON.
   ```
   Error: invalid character '}' looking for beginning of object key string
   ```
   Solution: Validate the JSON syntax of the network configuration file.

3. **Partition Not Found in Snapshot**: Ensure that the snapshots contain data for the partitions defined in the network configuration.
   ```
   Error: init Directory: no accounts found for partition
   ```
   Solution: Verify that the snapshots contain data for the specified partitions.

4. **Routing Issues**: If accounts are not being properly routed to partitions, check the routing table in the network configuration.
   ```
   Error: route acc://example.acme: unknown partition
   ```
   Solution: Verify that the routing table is correctly defined in the network configuration.

### Debugging Tips

1. Use the `--verbose` flag to get more detailed output:
   ```bash
   accumulated init genesis network.json -w output-dir --snapshot snapshot.snap --verbose
   ```

2. Check the generated JSON files to verify the content of the genesis state:
   ```bash
   cat output-dir/Directory.json | jq
   ```

3. If you encounter issues with specific accounts, you can use the `analyze` tool to inspect the snapshot:
   ```bash
   accumulated analyze snapshot snapshot.snap
   ```

## See Also

### Related Documentation
- [**network-json-structure.md**](network-json-structure.md) - Network configuration format and validation
- [**consensus-creation-workflow.md**](consensus-creation-workflow.md) - Consensus section generation procedures
- [**p2p-key-generation.md**](p2p-key-generation.md) - P2P key generation for network nodes
- [**debug-app-reference.md**](debug-app-reference.md) - Debug commands for network operations

### Cyclops Validator Documentation
- [**cyclops/cyclops-preparation.md**](cyclops/cyclops-preparation.md) - Cyclops validator preparation workflow
- [**cyclops/cyclops-deployment.md**](cyclops/cyclops-deployment.md) - Cyclops validator deployment procedures
- [**cyclops/cyclops-automation.md**](cyclops/cyclops-automation.md) - Complete automation system

### Technical References
- [**technical/snapshot-format-overview.md**](technical/snapshot-format-overview.md) - Snapshot format introduction
- [**technical/snapshot-format-operations.md**](technical/snapshot-format-operations.md) - Snapshot operations guide
- [**technical/genesis-format.md**](technical/genesis-format.md) - Genesis document format specification
- [**technical/record-format.md**](technical/record-format.md) - Database record format specification

### API Documentation
- [**api/accumulated-daemon-commands.md**](api/accumulated-daemon-commands.md) - `accumulated` command reference
- [**api/analyze-commands.md**](api/analyze-commands.md) - `analyze` tool command reference

### Network References
- [**network/accumulate-mainnet-reference.md**](network/accumulate-mainnet-reference.md) - Mainnet configuration examples
- [**network/network-boot-procedures.md**](network/network-boot-procedures.md) - Network bootstrap procedures

## Related Commands

### Network Initialization
- `accumulated init network` - See [api/accumulated-daemon-commands.md](api/accumulated-daemon-commands.md)
- `debug genesis ingest` - See [debug-app-reference.md](debug-app-reference.md)
- `debug snap collect` - See [debug-app-reference.md](debug-app-reference.md)

### Consensus Management
- `analyze generate-consensus-section` - See [consensus-creation-workflow.md](consensus-creation-workflow.md)
- `analyze update-consensus` - See [api/analyze-commands.md](api/analyze-commands.md)

### Network Analysis
- `debug network scan` - See [debug-app-reference.md](debug-app-reference.md)
- `debug network status` - See [debug-app-reference.md](debug-app-reference.md)

## Source Code References

- `cmd/accumulated/cmd_init_network.go`: Implementation of the `init genesis` command
- `internal/node/daemon/init.go`: Implementation of `BuildGenesisDocs`
- `internal/node/genesis/bootstrap.go`: Implementation of `genesis.Init` and snapshot unpacking
- `internal/node/genesis/extract.go`: Implementation of account extraction from snapshots
- `tools/cmd/debug/genesis.go`: Debug genesis ingest command implementation
- `tools/cmd/analyze/cmd_generate_consensus.go`: Consensus section generation

---
*This document is part of the [Accumulate Network Documentation](README.md) - optimized for AI assistance and developer productivity.*
