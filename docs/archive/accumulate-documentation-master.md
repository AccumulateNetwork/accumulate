<!-- AI_DOCUMENT_TYPE: comprehensive_reference -->
<!-- AI_PRIMARY_TOPICS: mainnet_config, daemon_commands, deployment_automation -->
<!-- AI_COMPLEXITY: high -->
<!-- AI_SPLIT_RECOMMENDED: yes -->
<!-- AI_LAST_UPDATED: 2025-01-05 -->

# Accumulate Network Documentation

> **Document Status**: Master document pending optimization split  
> **Scope**: MainNet configuration, node daemon commands, deployment automation  
> **Target Audience**: Network operators, developers, deployment engineers

## Document Navigation

- [📋 MainNet Configuration](#mainnet-configuration) - Network specs, ports, validators
- [⚙️ Node Daemon Commands](#node-daemon-commands) - `accumulated` initialization and runtime
- [🚀 Deployment Automation](#deployment-automation) - Cyclops network deployment
- [🔧 Troubleshooting](#troubleshooting) - Common issues and solutions

---

# MainNet Configuration
<!-- AI_TAG: network_reference -->

This section provides the official configuration details for the Accumulate MainNet, including network architecture, port configurations, and validator information.

## Network Architecture
<!-- AI_TAG: network_topology -->

The Accumulate network uses a unique architecture where ADIs (Accumulate Digital Identifiers) are distributed over a set of Tendermint networks. The network consists of:

- **1 Directory Network (DN)**: Serves as the central coordination network
- **3 Block Validator Networks (BVNs)**:
  - Apollo
  - Chandrayaan
  - Yutu

Each network uses the Tendermint consensus protocol for both the Block Validator Networks and the Directory Network.

## Network Ports
<!-- AI_TAG: port_configuration -->

### Directory Network (DN) Ports:
- **16591** - DN P2P (Peer-to-Peer communication)
- **16592** - DN RPC (Remote Procedure Call)
- **16595** - DN RPC JSON (JSON-RPC interface)

### Block Validator Network (BVN) Ports:
- **16691** - BVN P2P (Peer-to-Peer communication)
- **16692** - BVN RPC (Remote Procedure Call)
- **16695** - BVN RPC JSON (JSON-RPC interface)

### Management and SSL Ports:
- **16666** - AccMan (Accumulate Manager)
- **6695** - SSL Client (HTTPS access)

## Network Specifications
<!-- AI_TAG: network_specs -->

### Network Identity
- **Network ID**: MainNet
- **Network Version**: 69

### Network Partitions
<!-- AI_TAG: partition_config -->

The MainNet consists of the following partitions:

| Partition | Type | Role |
|-----------|------|------|
| Directory | directory | Central coordination network |
| Apollo | blockValidator | Block validation network |
| Chandrayaan | blockValidator | Block validation network |
| Yutu | blockValidator | Block validation network |

### Network Validators
<!-- AI_TAG: validator_config -->

The network is maintained by multiple validators operated by different entities:

| Operator | Partitions |
|----------|------------|
| kompendium.acme | Directory, Apollo |
| LunaNova.acme | Directory, Apollo |
| Factoshi.acme | - |
| TurtleBoat.acme | Directory, Chandrayaan |
| Stamp-It.acme | - |
| MusicCityNode.acme | Directory, Yutu |
| ConsensusNetworks.acme | Directory, Chandrayaan |
| defacto.acme | - |
| tfa.acme | Directory, Yutu |
| CodeForj.acme | Directory, Yutu |
| PrestigeIT.acme | Directory, Chandrayaan |
| GOI.acme | - |
| defidevs.acme | Directory, Chandrayaan, Apollo, Yutu |
| Sphereon.acme | Directory, Chandrayaan |
| ACMEMining.acme | Directory, Apollo |
| Inveniam.acme | Directory, Chandrayaan |
| HighStakes.acme | Directory, Yutu |
| FederateThis.acme | Directory, Apollo |
| DetroitLedgerTech.acme | Directory, Yutu |

### Global Parameters

#### Fee Schedule
- Create Identity Sliding Scale: [4800000, 1200000, 350000, 90000, 25000, 7000, 1800]
- Create Sub-Identity: 2500

#### System Limits
- Account Authorities: 20
- Book Pages: 20
- Data Entry Parts: 100
- Identity Accounts: 1000
- Page Entries: 100
- Pending Major Blocks: 28

#### Consensus Parameters
- Major Block Schedule: "0 */12 * * *"
- Operator Accept Threshold: 2/3
- Validator Accept Threshold: 2/3

#### Oracle
- Price: 5000

## Important Notes

- These ports are automatically configured when using the AccMan (Accumulate Manager) tool, which is the recommended method for running nodes on the Accumulate network.
- The network uses separate port ranges for the Directory Network (165xx series) and Block Validator Networks (166xx series).
- Firewall configuration is automatically handled by the Accumulate Manager, though manual iptables configuration is possible.
- These port numbers are specific to the Accumulate blockchain protocol and are essential for proper node operation, peer-to-peer communication, and RPC interactions within the network.

## Special Routing Rules

The network has special routing rules for certain accounts:

| Account | Assigned Partition |
|---------|-------------------|
| acc://staking.acme | Directory |
| acc://ACME | Directory |
| acc://bvn-Apollo.acme | Apollo |
| acc://bvn-Chandrayaan.acme | Chandrayaan |
| acc://bvn-Yutu.acme | Yutu |
| acc://dn.acme | Directory |

## Network Configuration Files

### Kermit Network Configuration

The Kermit network is a testnet configuration with the following structure:

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

Key features of the Kermit network:
- Three BVNs named after the Marx Brothers: Chico, Harpo, and Groucho
- Each BVN has a single validator node
- Base port configuration starts at 16591
- Uses the "v2-vandenberg" executor version
- Simplified fee schedule compared to MainNet

### MainNet Network Configuration

Based on the describe.json and network architecture, here is the equivalent network.json for MainNet:

```json
{
    "id": "MainNet",
    "globals": {
        "executorVersion": "v2-vandenberg",
        "oracle": {
            "price": 5000
        },
        "globals": {
            "feeSchedule": {
                "createIdentitySliding": [
                    4800000,
                    1200000,
                    350000,
                    90000,
                    25000,
                    7000,
                    1800
                ],
                "createSubIdentity": 2500
            },
            "limits": {
                "accountAuthorities": 20,
                "bookPages": 20,
                "dataEntryParts": 100,
                "identityAccounts": 1000,
                "pageEntries": 100,
                "pendingMajorBlocks": 28
            },
            "majorBlockSchedule": "0 */12 * * *",
            "operatorAcceptThreshold": {
                "denominator": 3,
                "numerator": 2
            },
            "validatorAcceptThreshold": {
                "denominator": 3,
                "numerator": 2
            }
        },
        "routing": {
            "routes": [
                {
                    "length": 2,
                    "partition": "Apollo"
                },
                {
                    "length": 2,
                    "partition": "Yutu",
                    "value": 1
                },
                {
                    "length": 2,
                    "partition": "Chandrayaan",
                    "value": 2
                },
                {
                    "length": 3,
                    "partition": "Apollo",
                    "value": 6
                },
                {
                    "length": 4,
                    "partition": "Yutu",
                    "value": 14
                },
                {
                    "length": 4,
                    "partition": "Chandrayaan",
                    "value": 15
                }
            ],
            "overrides": [
                {
                    "account": "acc://staking.acme",
                    "partition": "Directory"
                },
                {
                    "account": "acc://ACME",
                    "partition": "Directory"
                },
                {
                    "account": "acc://bvn-Apollo.acme",
                    "partition": "Apollo"
                },
                {
                    "account": "acc://bvn-Chandrayaan.acme",
                    "partition": "Chandrayaan"
                },
                {
                    "account": "acc://bvn-Yutu.acme",
                    "partition": "Yutu"
                },
                {
                    "account": "acc://dn.acme",
                    "partition": "Directory"
                }
            ]
        }
    },
    "bvns": [
        {
            "id": "Apollo",
            "nodes": [
                {
                    "dnnType": "validator",
                    "bvnnType": "validator",
                    "basePort": 16591,
                    "advertizeAddress": "apollo.accumulate.network"
                }
            ]
        },
        {
            "id": "Chandrayaan",
            "nodes": [
                {
                    "dnnType": "validator",
                    "bvnnType": "validator",
                    "basePort": 16591,
                    "advertizeAddress": "chandrayaan.accumulate.network"
                }
            ]
        },
        {
            "id": "Yutu",
            "nodes": [
                {
                    "dnnType": "validator",
                    "bvnnType": "validator",
                    "basePort": 16591,
                    "advertizeAddress": "yutu.accumulate.network"
                }
            ]
        },
        {
            "id": "Directory",
            "nodes": [
                {
                    "dnnType": "validator",
                    "bvnnType": "none",
                    "basePort": 16591,
                    "advertizeAddress": "directory.accumulate.network"
                }
            ]
        }
    ]
}
```

Note: The MainNet configuration includes placeholder advertizeAddress values. The actual addresses would need to be confirmed with the network operators.

---

# Node Daemon Commands
<!-- AI_TAG: daemon_commands -->

The `accumulated` binary is the **Accumulate Node Daemon** - the core server process that runs Accumulate network nodes (Directory Network and Block Validator Networks). This section documents the initialization and runtime commands for production network deployment.

## Command Overview
<!-- AI_TAG: command_summary -->

The node daemon provides several initialization commands for different deployment scenarios:

| Command | Purpose | Use Case |
|---------|---------|----------|
| `init network` | Complete network initialization | New network deployment |
| `init genesis` | Genesis-only generation | Standalone genesis creation |
| `init prepare-genesis` | Consolidate snapshots | Multi-snapshot genesis |
| `init node` | Single node initialization | Join existing network |
| `init dual` | Dual node setup | Directory + BVN node |
| `run` | Start node daemon | Runtime operation |

## Network Initialization Commands
<!-- AI_TAG: init_commands -->

### `accumulated init network`
<!-- AI_TAG: init_network -->

**Purpose**: Initialize a complete Accumulate network with all nodes, genesis files, and configuration.

```bash
accumulated init network <network-config.json> [flags]
```

**Side Effects:**
- **File Creation**: Multiple node directories under `--work-dir`
- **Key Generation**: Cryptographic keys stored in network config structure
- **Genesis Files**: Binary `.snap` files written per partition
- **Network Topology**: Peer connections configured between all nodes
- **Port Configuration**: Automatic port assignment based on node roles

#### `accumulated init genesis`

**Purpose:** Generates only genesis files for a network without creating node configurations.

**Usage:**
```bash
accumulated init genesis <network-config-file> [flags]
```

**Required Arguments:**
- `<network-config-file>`: Path to network configuration JSON file

**Available Flags:**
- **Inherits ALL flags from parent `init` command**
- Same flags as `init network`: `--genesis-doc`, `--factom-balances`, `--snapshot`, etc.

**Key Difference from `init network`:**
- **Genesis Only**: Creates genesis documents but NO node directories or configurations
- **Dual Format Output**: Produces both binary (`.snap`) and JSON (`.json`) genesis files
- **Standalone Operation**: Can be used independently of node setup

**Actual Implementation Behavior:**
1. **Network Loading**: Calls `loadNetworkConfiguration()` to parse config
2. **Genesis Building**: Calls `buildGenesis()` with same logic as `init network`
3. **File Writing**: For each partition:
   - Writes `<partition>.snap` (binary format)
   - Converts to JSON using `genesis.ConvertSnapshotToJson()`
   - Writes `<partition>.json` (human-readable format)

**Side Effects:**
- **File Creation**: Only genesis files in working directory
- **No Node Setup**: Does NOT create node directories or `accumulate.toml` files
- **Data Processing**: Processes all genesis data sources (snapshots, Factom, faucet)

#### `accumulated init prepare-genesis`

**Purpose:** Ingests multiple snapshots and produces a single, consolidated genesis snapshot.

**Usage:**
```bash
accumulated init prepare-genesis <output-file> <input-snapshot1> [input-snapshot2...]
```

**Required Arguments:**
- `<output-file>`: Path for consolidated output snapshot
- `<input-snapshots>`: One or more input snapshot files to process (minimum 2 args total)

**No Additional Flags** - Uses positional arguments only

**Actual Implementation Behavior:**
1. **In-Memory Database**: Creates `coredb.OpenInMemory()` for data consolidation
2. **Sequential Processing**: For each input snapshot:
   - Opens snapshot file
   - Calls `genesis.Extract()` to load all accounts
   - Retains everything (no filtering)
   - Shows progress with account hash and URL
3. **Collection Phase**: Calls `db.Collect()` to write consolidated snapshot
4. **Progress Reporting**: Real-time updates during processing

**Side Effects:**
- **Memory Usage**: Loads all snapshot data into memory simultaneously
- **File Creation**: Single consolidated snapshot file
- **Progress Output**: Console updates showing processing status
- **Data Retention**: No filtering - retains all accounts and transactions

### Node Configuration Commands

#### `accumulated init node`

**Purpose:** Initializes a single node to join an existing network.

**Usage:**
```bash
accumulated init node <peer-url> [flags]
```

**Required:**
- `--work-dir <directory>`: Working directory for node files (**MANDATORY**)
- `<peer-url>`: URL of existing peer to connect to

**Available Flags:**
- `--follow` / `-f`: Do not participate in voting (follower mode)
- `--genesis-doc <file>`: Genesis document for target network
- `--listen <address>` / `-l`: Address and port to listen on (e.g., tcp://1.2.3.4:5678)
- `--public <ip>` / `-p`: Public IP or URL for external connections
- `--skip-version-check`: Do not enforce version compatibility check
- `--seed <url>`: Fetch network configuration from seed proxy
- `--skip-peer-health-check`: Do not check health of peers
- `--no-prometheus`: Disable Prometheus metrics

**Actual Implementation Behavior:**
1. **Work Directory Check**: Fails if `--work-dir` not specified
2. **Peer Connection**: Chooses between `initNodeFromSeedProxy()` or `initNodeFromPeer()`
3. **Port Calculation**: Determines base port from peer/seed connection
4. **Address Configuration**: 
   - Resolves listen address (defaults to `tcp://0.0.0.0:basePort`)
   - Configures public address if specified
   - Calculates port offsets for different services
5. **Key Management**: Calls `LoadOrGenerateTmPrivKey()` for validator and node keys
6. **File Writing**: Calls `WriteNodeFiles()` to create all configuration files

**Side Effects:**
- **Directory Creation**: Single node directory under `--work-dir`
- **Key Files**: `priv_validator_key.json` and `node_key.json` generated
- **Configuration**: `accumulate.toml` with network-specific settings
- **Port Setup**: Automatic port offset calculation based on network type
- **P2P Configuration**: External address and peer connections configured

#### `accumulated init dual`

**Purpose:** Initializes a dual node (Directory + Block Validator) setup.

**Usage:**
```bash
accumulated init dual <bvn-peer-url> [flags]
```

**Required Arguments:**
- `<bvn-peer-url>`: URL of BVN peer to connect to

**Available Flags:**
- `--follow` / `-f`: Do not participate in voting
- `--skip-version-check`: Do not enforce version check
- `--public <ip>` / `-p`: Public IP or URL (auto-resolved if not specified)
- `--listen <address>` / `-l`: Address to listen on
- `--seed <url>`: Fetch from seed proxy (**NOT CURRENTLY SUPPORTED**)
- `--no-prometheus`: Disable Prometheus metrics
- `--dn-genesis-doc <file>`: Genesis document for Directory Network
- `--bvn-genesis-doc <file>`: Genesis document for Block Validator Network

**Actual Implementation Behavior:**
1. **Public IP Resolution**: Calls `resolvePublicIp()` using ip-api.com if not specified
2. **Port Calculation**: 
   - Extracts BVN port from peer URL
   - Calculates DN port: `dnBasePort = bvnBasePort - PortOffsetBlockValidator`
3. **Dual Node Setup**:
   - **Directory Node**: Calls `initNode()` with DN URL and `--dn-genesis-doc`
   - **BVN Node**: Calls `initNode()` with BVN URL and `--bvn-genesis-doc`
4. **Finalization**:
   - Calls `finalizeBvnn()` to configure BVN node
   - Calls `finalizeDnn()` to configure DN node with BVN partition ID
5. **Key Sharing**: Keys are shared between DN and BVN nodes in dual mode

**Side Effects:**
- **Directory Creation**: Both `dnn` and `bvnn` directories created
- **Shared Keys**: Cryptographic keys shared between both nodes
- **Configuration Files**: Separate `accumulate.toml` for each node
- **Port Coordination**: Automatic port offset calculation for dual operation
- **Partition Linking**: DN node configured with BVN partition ID
- **Network Integration**: Bootstrap peers configured for both networks

### Command Usage Examples

#### Complete Network Initialization (Production)

```bash
# Step 1: Initialize complete network from config
./accumulated init network cyclops-network.json --work-dir /accumulate-network/nodes

# Step 2: Start individual nodes
./accumulated run --work-dir /accumulate-network/nodes/bvn1-1
./accumulated run --work-dir /accumulate-network/nodes/bvn1-2
```

#### Single Node Join (Production)

```bash
# Initialize node to join existing network
./accumulated init node tcp://existing-peer.network:16691 \
  --work-dir /accumulate-network/new-node \
  --public 203.0.113.10 \
  --listen tcp://0.0.0.0:16691

# Start the node
./accumulated run --work-dir /accumulate-network/new-node
```

#### Dual Node Setup (Production)

```bash
# Initialize dual node (DN + BVN)
./accumulated init dual tcp://bvn-peer.network:16691 \
  --work-dir /accumulate-network/dual-node \
  --public 203.0.113.20

# Start Directory Network node
./accumulated run --work-dir /accumulate-network/dual-node/dnn

# Start Block Validator Network node (separate terminal)
./accumulated run --work-dir /accumulate-network/dual-node/bvnn
```

#### Genesis-Only Generation

```bash
# Generate genesis files without node setup
./accumulated init genesis network-config.json \
  --work-dir /genesis-output \
  --snapshot /snapshots/mainnet-latest.snap \
  --factom-addresses /data/factom-addresses.txt

# Results in:
# /genesis-output/Directory.snap
# /genesis-output/Directory.json
# /genesis-output/bvn-apollo.snap
# /genesis-output/bvn-apollo.json
```

### Node Daemon Run Command Documentation

The `accumulated run` command starts the Accumulate node daemon with various configurations:

```bash
accumulated run [flags]
accumulated run [command]
```

#### Standard Node Execution

```bash
accumulated run [flags]
```

**Purpose**: Run a single Accumulate node.

**Prerequisites**:
- Node must be initialized with `accumulated init`
- Configuration files must exist (`accumulate.toml`)
- Genesis snapshots must be available
- Working directory must be properly set up

**Key Flags**:
- `--debug`: Enable debugging features
- `--enable-timing-logs`: Enable core timing analysis logging
- `--json-log-file string`: Write logs to file as JSON
- `--log-file string`: Write logs to file as plain text
- `-n, --node int`: Specify which node to run (for multi-node setups)
- `--pprof string`: Address for profiling server
- `--truncate`: Truncate Badger database if necessary
- `-w, --work-dir string`: Working directory (default: ~/.accumulate)

**Example**:
```bash
./accumulated run --work-dir /path/to/node/config
```

#### DevNet Execution (Development Only)

```bash
accumulated run devnet [flags]
```

**Purpose**: Run a complete local development network.

**⚠️ Important**: DevNet is NOT suitable for mainnet or production use. It takes shortcuts and simplifications that are not appropriate for production networks.

**What DevNet does**:
- Creates simplified network configuration
- Uses development-only shortcuts
- Bypasses security measures appropriate for production
- Automatically configures all network components

**Why DevNet is inappropriate for production**:
- Simplified consensus mechanisms
- Reduced security validations
- Development-only network topology
- Non-production cryptographic settings

### Configuration Files

#### accumulate.toml

The primary configuration file for Accumulate nodes. This file is created during the initialization process and contains:

**Core Configuration Sections**:

```toml
# Network configuration
[network]
id = "MainNet"
type = "validator"  # or "follower"

# Storage configuration
[storage]
type = "leveldb"
path = "data/accumulate.db"

# P2P networking
[p2p]
listen = "tcp://0.0.0.0:16591"
seeds = ["tcp://seed1.accumulate.network:16591"]
persistent_peers = []

# RPC configuration
[rpc]
listen = "tcp://0.0.0.0:16592"

# Consensus configuration
[consensus]
timeout_commit = "1s"
create_empty_blocks = true

# Logging configuration
[logging]
level = "info"
format = "plain"  # or "json"
```

**Configuration Generation**:
- Created automatically by `accumulated init` commands
- Can be manually edited for specific requirements
- Must be present in working directory for `accumulated run`

#### Directory Structure

Proper Accumulate node directory structure:

```
node-directory/
├── accumulate.toml          # Main configuration file
├── data/
│   ├── accumulate.db/       # LevelDB database
│   └── snapshots/           # Snapshot storage
├── genesis/
│   ├── dn-genesis.snap      # Directory Network genesis
│   └── bvn*-genesis.snap    # BVN genesis files
└── logs/
    ├── accumulate.log       # Application logs
    └── consensus.log        # Consensus logs
```

### Production Network Launch Sequence

For production networks (non-devnet), follow this sequence:

#### Step 1: Network Genesis Creation

```bash
# Create network genesis from configuration
./accumulated init network /path/to/network-config.json --work-dir /path/to/network/setup
```

#### Step 2: Node Configuration

```bash
# For each validator node, initialize configuration
./accumulated init dual partition.network --work-dir /path/to/node/config
```

#### Step 3: Genesis Distribution

```bash
# Copy genesis snapshots to each node
cp /path/to/network/setup/dn-genesis.snap /path/to/node/config/
cp /path/to/network/setup/bvn*-genesis.snap /path/to/node/config/
```

#### Step 4: Configuration Validation

```bash
# Verify configuration files exist
ls /path/to/node/config/accumulate.toml
ls /path/to/node/config/*-genesis.snap
```

#### Step 5: Network Launch

```bash
# Start each node
./accumulated run --work-dir /path/to/node/config
```

### Best Practices

1. **Always use explicit working directories**: Specify `--work-dir` to avoid conflicts
2. **Validate configurations**: Check `accumulate.toml` and genesis files before starting nodes
3. **Use proper network identifiers**: Follow `partition.network` format for production
4. **Avoid DevNet for production**: Never use `run devnet` for mainnet or production networks
5. **Monitor initialization logs**: Check for errors during `init` commands
6. **Backup configurations**: Keep copies of working configurations and genesis files

### Integration with Partition Snapshots

When using custom partition snapshots (e.g., from snapshot extraction tools):

1. **Create network genesis first**:
   ```bash
   ./accumulated init network config.json
   ```

2. **Replace genesis snapshots with custom snapshots**:
   ```bash
   cp /path/to/custom/Directory-partition.snap ./dn-genesis.snap
   cp /path/to/custom/bvn-partition.snap ./bvn1-genesis.snap
   ```

3. **Initialize node configurations**:
   ```bash
   ./accumulated init dual partition.network
   ```

4. **Launch network**:
   ```bash
   ./accumulated run --work-dir /path/to/node
   ```

This approach allows integration of custom snapshot data while maintaining proper network initialization procedures.

---

# Deployment Automation
<!-- AI_TAG: deployment_automation -->

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

### Script Usage

#### Prerequisites
1. **Cyclops Artifacts**: Ensure artifacts are in `/home/paul/accumulate-network/artifacts/`:
   - `cyclops-genesis.snap` (original snapshot)
   - `network.json` (network configuration)

2. **Go Environment**: Accumulate source code at `/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate`

#### Running the Script
```bash
cd /home/paul/accumulate-network/artifacts
chmod +x deploy-cyclops-network.sh
./deploy-cyclops-network.sh
```

#### Expected Output
1. **Cleanup confirmation**: Previous deployments removed
2. **Compilation success**: Binaries built successfully
3. **Extraction statistics**: 
   - DN partition: ~2,974,812 records, ~2GB
   - BVN partition: 0 records, ~0MB
4. **Network initialization**: Genesis files created
5. **Node configuration**: `accumulate.toml` generated
6. **Network startup**: Nodes begin running with log output

### Script Monitoring

#### Log Files
- `network.log` - Complete network runtime logs
- Console output shows real-time status

#### Success Indicators
```bash
# Check for successful partition extraction
ls -lh /tmp/partition-snapshots/
# Should show ~2GB Directory-partition.snap

# Check for genesis files
ls -lh /home/paul/accumulate-network/nodes/*-genesis.snap

# Check configuration
ls /home/paul/accumulate-network/nodes/accumulate.toml
```

#### Troubleshooting Script Issues

**Compilation Errors**:
- Ensure Go environment is properly set up
- Check Accumulate source code is at expected path
- Verify all dependencies are available

**Extraction Failures**:
- Verify `cyclops-genesis.snap` exists and is readable
- Check `network.json` format and content
- Ensure sufficient disk space in `/tmp`

**Network Initialization Errors**:
- Verify partition snapshots were created successfully
- Check network configuration JSON syntax
- Ensure working directory permissions are correct

**Node Startup Issues**:
- Verify `accumulate.toml` was created
- Check genesis files exist and are valid
- Monitor `network.log` for specific error messages

### Script Customization

The script can be modified for different environments by updating:

```bash
# Path variables at top of script
ACCUMULATE_REPO="/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate"
WORK_DIR="/home/paul/accumulate-network"
ARTIFACTS_DIR="/home/paul/accumulate-network/artifacts"
NODES_DIR="/home/paul/accumulate-network/nodes"
SNAPSHOT_OUTPUT="/tmp/partition-snapshots"
```

### Integration with Manual Process

The script automates the entire manual process documented above:
1. **Manual Step 1-2**: Automated cleanup and compilation
2. **Manual Step 3**: Automated partition snapshot extraction
3. **Manual Step 4**: Automated network initialization with custom snapshots
4. **Manual Step 5**: Automated node configuration and startup

This provides a reliable, repeatable deployment process that eliminates manual errors and ensures consistent network initialization.

---

# Troubleshooting
<!-- AI_TAG: troubleshooting -->

## Command Issues
<!-- AI_TAG: command_errors -->

### Syntax Errors

**"accepts at most 1 arg(s), received 3"**
- **Cause**: Incorrect command syntax or argument order
- **Solution**: Use `accumulated init network config.json` not `accumulated network init config.json`

**"unknown command"**
- **Cause**: Typo in subcommand name
- **Solution**: Use exact command names: `network`, `genesis`, `node`, `dual`

### Configuration Errors

**"cannot load configuration file for node"**
- **Cause**: Corrupted or missing `accumulate.toml` file
- **Solution**: Re-initialize node with `--reset` flag to recreate configuration

**"node not initialized"**
- **Cause**: Node not properly initialized or wrong working directory
- **Solution**: 
  1. Run appropriate `accumulated init` command first
  2. Verify `--work-dir` points to correct directory
  3. Check that `accumulate.toml` exists in working directory

## Network Issues
<!-- AI_TAG: network_errors -->

### Connection Problems

**"failed to connect to seeds"**
- **Cause**: Network connectivity or DNS resolution issues
- **Solution**: 
  1. Check network connectivity to seed nodes
  2. Verify DNS resolution for seed addresses
  3. Check firewall rules for required ports

**"port already in use"**
- **Cause**: Another process using the same port
- **Solution**: 
  1. Stop conflicting processes
  2. Use different base port with `--base-port` flag
  3. Check port availability with `netstat -tulpn`

## Deployment Issues
<!-- AI_TAG: deployment_errors -->

### Script Failures

**Compilation Errors**:
- Ensure Go environment is properly set up
- Check Accumulate source code is at expected path
- Verify all dependencies are available

**Extraction Failures**:
- Verify `cyclops-genesis.snap` exists and is readable
- Check `network.json` format and content
- Ensure sufficient disk space in `/tmp`

**Network Initialization Errors**:
- Verify partition snapshots were created successfully
- Check network configuration JSON syntax
- Ensure working directory permissions are correct

### Resource Issues

**Insufficient Disk Space**
- **Cause**: Not enough space for snapshots or node data
- **Solution**: 
  1. Free up disk space
  2. Use different directory with more space
  3. Clean up old snapshots and logs

**Memory Issues**
- **Cause**: Insufficient RAM for snapshot processing
- **Solution**: 
  1. Increase available memory
  2. Process snapshots in smaller chunks
  3. Use streaming processing options

## Quick Diagnostics
<!-- AI_TAG: diagnostics -->

### Health Checks

```bash
# Check binary exists and version
./accumulated version

# Check configuration file
ls -la accumulate.toml

# Check genesis files
ls -la *-genesis.snap

# Check network connectivity
telnet <peer-address> <peer-port>

# Check port availability
netstat -tulpn | grep <port>

# Check logs
tail -f network.log
```

### Common File Locations

- **Configuration**: `<work-dir>/accumulate.toml`
- **Genesis Files**: `<work-dir>/*-genesis.snap`
- **Logs**: `<work-dir>/logs/` or console output
- **Keys**: `<work-dir>/config/priv_validator_key.json`
- **Node Data**: `<work-dir>/data/`
