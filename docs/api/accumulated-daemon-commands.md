<!-- AI_DOCUMENT_TYPE: command_reference -->
<!-- AI_PRIMARY_TOPICS: daemon_commands, init_commands, runtime_commands -->
<!-- AI_COMPLEXITY: high -->
<!-- AI_SPLIT_RECOMMENDED: no -->
<!-- AI_LAST_UPDATED: 2025-01-05 -->

# Accumulate Node Daemon Commands

> **Document Type**: Command reference guide  
> **Scope**: `accumulated` initialization and runtime commands  
> **Target Audience**: Node operators, network administrators

## Quick Reference

| Command | Purpose | Use Case |
|---------|---------|----------|
| `init network` | Complete network initialization | New network deployment |
| `init genesis` | Genesis-only generation | Standalone genesis creation |
| `init prepare-genesis` | Consolidate snapshots | Multi-snapshot genesis |
| `init node` | Single node initialization | Join existing network |
| `init dual` | Dual node setup | Directory + BVN node |
| `run` | Start node daemon | Runtime operation |

---

## Overview
<!-- AI_TAG: daemon_overview -->

The `accumulated` binary is the **Accumulate Node Daemon** - the core server process that runs Accumulate network nodes (Directory Network and Block Validator Networks). This section documents the initialization and runtime commands for production network deployment.

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

### `accumulated init genesis`
<!-- AI_TAG: init_genesis -->

**Purpose:** Generates only genesis files for a network without creating node configurations.

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

### `accumulated init prepare-genesis`
<!-- AI_TAG: init_prepare_genesis -->

**Purpose:** Ingests multiple snapshots and produces a single, consolidated genesis snapshot.

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

## Node Configuration Commands
<!-- AI_TAG: node_config_commands -->

### `accumulated init node`
<!-- AI_TAG: init_node -->

**Purpose:** Initializes a single node to join an existing network.

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

### `accumulated init dual`
<!-- AI_TAG: init_dual -->

**Purpose:** Initializes a dual node (Directory + Block Validator) setup.

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

## Runtime Commands
<!-- AI_TAG: runtime_commands -->

### `accumulated run`
<!-- AI_TAG: run_command -->

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

## Configuration Files
<!-- AI_TAG: config_files -->

### accumulate.toml

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

### Directory Structure

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

## Usage Examples
<!-- AI_TAG: usage_examples -->

### Complete Network Initialization (Production)

```bash
# Step 1: Initialize complete network from config
./accumulated init network cyclops-network.json --work-dir /accumulate-network/nodes

# Step 2: Start individual nodes
./accumulated run --work-dir /accumulate-network/nodes/bvn1-1
./accumulated run --work-dir /accumulate-network/nodes/bvn1-2
```

### Single Node Join (Production)

```bash
# Initialize node to join existing network
./accumulated init node tcp://existing-peer.network:16691 \
  --work-dir /accumulate-network/new-node \
  --public 203.0.113.10 \
  --listen tcp://0.0.0.0:16691

# Start the node
./accumulated run --work-dir /accumulate-network/new-node
```

### Dual Node Setup (Production)

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

### Genesis-Only Generation

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

## Production Network Launch Sequence
<!-- AI_TAG: production_launch -->

For production networks (non-devnet), follow this sequence:

### Step 1: Network Genesis Creation

```bash
# Create network genesis from configuration
./accumulated init network /path/to/network-config.json --work-dir /path/to/network/setup
```

### Step 2: Node Configuration

```bash
# For each validator node, initialize configuration
./accumulated init dual partition.network --work-dir /path/to/node/config
```

### Step 3: Genesis Distribution

```bash
# Copy genesis snapshots to each node
cp /path/to/network/setup/dn-genesis.snap /path/to/node/config/
cp /path/to/network/setup/bvn*-genesis.snap /path/to/node/config/
```

### Step 4: Configuration Validation

```bash
# Verify configuration files exist
ls /path/to/node/config/accumulate.toml
ls /path/to/node/config/*-genesis.snap
```

### Step 5: Network Launch

```bash
# Start each node
./accumulated run --work-dir /path/to/node/config
```

## Best Practices
<!-- AI_TAG: best_practices -->

1. **Always use explicit working directories**: Specify `--work-dir` to avoid conflicts
2. **Validate configurations**: Check `accumulate.toml` and genesis files before starting nodes
3. **Use proper network identifiers**: Follow `partition.network` format for production
4. **Avoid DevNet for production**: Never use `run devnet` for mainnet or production networks
5. **Monitor initialization logs**: Check for errors during `init` commands
6. **Backup configurations**: Keep copies of working configurations and genesis files

## Integration with Partition Snapshots
<!-- AI_TAG: snapshot_integration -->

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

## Node Configuration Deep Dive
<!-- AI_TAG: config_deep_dive -->

This section provides detailed technical documentation of the Accumulate node configuration generation process, CometBFT integration, and cryptographic key management based on actual implementation analysis.

### Configuration File Generation Process
<!-- AI_TAG: config_generation -->

#### `accumulate.toml` Structure and Generation

The `accumulate.toml` file is generated by serializing a structured `Config` object that embeds CometBFT's Tendermint configuration and Accumulate-specific settings.

**Configuration Structure:**
```go
type Config struct {
    tm.Config      // Embedded CometBFT configuration
    Accumulate Accumulate
}

type Accumulate struct {
    Describe              // Network description (type, partition ID)
    SummaryNetwork        string
    DisableDirectDispatch bool
    MaxEnvelopesPerBlock  int
    Healing     Healing     // Healing configuration
    Snapshots   Snapshots   // Snapshot configuration
    Storage     Storage     // Database storage settings
    P2P         P2P         // P2P networking configuration
    API         API         // API server configuration
    AnalysisLog AnalysisLog // Analysis logging
    Logging     Logging     // Loki logging integration
}
```

**Default Configuration Generation:**
```go
func Default(netName string, net protocol.PartitionType, nodeType NodeType, partitionId string) *Config {
    c := new(Config)
    c.Accumulate.Network.Id = netName
    c.Accumulate.NetworkType = net
    c.Accumulate.PartitionId = partitionId
    c.Accumulate.API.TxMaxWaitTime = 10 * time.Minute
    c.Accumulate.API.ConnectionLimit = 500
    c.Accumulate.Storage.Type = BadgerStorage
    c.Accumulate.Storage.Path = filepath.Join("data", "accumulate.db")
    c.Accumulate.Snapshots.Enable = false
    c.Accumulate.Snapshots.Directory = "snapshots"
    c.Accumulate.Snapshots.RetainCount = 10
    c.Config = *tm.DefaultConfig()  // Embed CometBFT defaults
    c.LogLevel = DefaultLogLevels
    c.Instrumentation.Prometheus = true
    return c
}
```

**TOML File Writing Process:**
```go
func StoreAcc(config *Config, dir string) error {
    return writeTomlFile(config.Accumulate, filepath.Join(dir, "accumulate.toml"))
}

func writeTomlFile(v any, file string) error {
    f, err := os.Create(file)
    if err != nil {
        return err
    }
    defer f.Close()
    return toml.NewEncoder(f).Encode(v)  // Uses github.com/pelletier/go-toml
}
```

**Key Configuration Sections:**

1. **Network Description (`[describe]`)**:
   ```toml
   [describe]
   network-type = "block-validator"  # or "directory"
   partition-id = "BVN0"
   
   [describe.network]
   id = "MainNet"
   ```

2. **Storage Configuration (`[storage]`)**:
   ```toml
   [storage]
   type = "badger"  # or "leveldb", "memory"
   path = "data/accumulate.db"
   ```

3. **API Configuration (`[api]`)**:
   ```toml
   [api]
   listen-address = "tcp://0.0.0.0:26660"
   tx-max-wait-time = "10m0s"
   connection-limit = 500
   read-header-timeout = "10s"
   debug-jsonrpc = false
   ```

4. **P2P Configuration (`[p2p]`)**:
   ```toml
   [p2p]
   listen = ["/tcp/16591"]
   bootstrap-peers = ["tcp://bootstrap.accumulate.io:16591"]
   ```

5. **Snapshots Configuration (`[snapshots]`)**:
   ```toml
   [snapshots]
   enable = false
   enable-indexing = false
   directory = "snapshots"
   retain = 10
   schedule = "0 */4"
   ```

### CometBFT Configuration Integration
<!-- AI_TAG: cometbft_config -->

#### `tendermint.toml` Generation and Management

CometBFT configuration is handled through embedded `tm.Config` within the main `Config` struct and written as a separate `tendermint.toml` file.

**Configuration Loading Process:**
```go
func (c *ConsensusService) start(inst *Instance) error {
    // Load CometBFT config
    d.config = tmcfg.DefaultConfig()
    d.config.SetRoot(inst.path(c.NodeDir))
    
    _, err = os.Stat(inst.path(c.NodeDir, "config", "tendermint.toml"))
    switch {
    case err == nil:
        // Load existing configuration with Viper
        v := viper.New()
        v.SetConfigFile(filepath.Join(nodeDir, "config", "tendermint.toml"))
        err = v.ReadInConfig()
        err = v.Unmarshal(d.config)
        
    case errors.Is(err, fs.ErrNotExist):
        // Generate new configuration with defaults
        d.config.NodeKey = ""
        d.config.PrivValidatorKey = ""
        d.config.Genesis = filepath.Join("..", c.Genesis)
        d.config.Mempool.MaxTxBytes = 4194304
        
        // Configure instrumentation
        d.config.Instrumentation.Prometheus = true
        d.config.Instrumentation.PrometheusListenAddr = listenHostPort(c.Listen, defaultHost, portMetrics)
        d.config.Instrumentation.Namespace = c.MetricsNamespace
        
        // Configure P2P
        d.config.P2P.ListenAddress = listenUrl(c.Listen, defaultHost, useTCP{}, portCmtP2P)
        d.config.RPC.ListenAddress = listenUrl(c.Listen, defaultHost, useTCP{}, portCmtRPC)
        d.config.P2P.AllowDuplicateIP = false
        
        // Set persistent peers from bootstrap peers
        for i, peer := range c.BootstrapPeers {
            id, err := cmtPeerAddress(peer)
            if i > 0 {
                d.config.P2P.PersistentPeers += ","
            }
            d.config.P2P.PersistentPeers += id
        }
        
        // Write configuration file
        tmcfg.WriteConfigFile(inst.path(c.NodeDir, "config", "tendermint.toml"), d.config)
    }
}
```

**Key CometBFT Configuration Parameters:**

1. **Consensus Settings**:
   ```toml
   [consensus]
   timeout_commit = "1s"
   create_empty_blocks = true
   create_empty_blocks_interval = "0s"
   ```

2. **P2P Network Settings**:
   ```toml
   [p2p]
   laddr = "tcp://0.0.0.0:26656"
   persistent_peers = "node_id@host:port,node_id2@host2:port2"
   allow_duplicate_ip = false
   addr_book_strict = true
   ```

3. **RPC Configuration**:
   ```toml
   [rpc]
   laddr = "tcp://0.0.0.0:26657"
   grpc_laddr = ""
   grpc_max_open_connections = 900
   ```

4. **Mempool Settings**:
   ```toml
   [mempool]
   recheck = true
   broadcast = true
   wal_dir = ""
   size = 5000
   max_txs_bytes = 4194304
   ```

5. **Instrumentation (Metrics)**:
   ```toml
   [instrumentation]
   prometheus = true
   prometheus_listen_addr = ":26660"
   max_open_connections = 3
   namespace = "consensus_BVN0"
   ```

#### Port Assignment Strategy

Ports are automatically assigned based on partition type and node role:

```go
const PortOffsetDirectory = 0
const PortOffsetBlockValidator = 100
const PortOffsetBlockSummary = 200

// Port calculation example:
// Directory Network: Base port + 0
// Block Validator: Base port + 100
// Block Summary: Base port + 200
```

**Standard Port Assignments:**
- **CometBFT P2P**: 26656 (+ partition offset)
- **CometBFT RPC**: 26657 (+ partition offset)
- **Accumulate API**: 26660 (+ partition offset)
- **Prometheus Metrics**: 26661 (+ partition offset)

### Cryptographic Key Management
<!-- AI_TAG: key_management -->

#### Private Validator Key (`priv_validator_key.json`)

**Purpose**: Ed25519 key used for consensus signing by the validator node.

**File Format**:
```json
{
  "address": "A3C204C8B9B9B9B9B9B9B9B9B9B9B9B9B9B9B9B9",
  "pub_key": {
    "type": "tendermint/PubKeyEd25519",
    "value": "base64-encoded-public-key"
  },
  "priv_key": {
    "type": "tendermint/PrivKeyEd25519",
    "value": "base64-encoded-private-key"
  }
}
```

**Key Loading and Generation Process**:
```go
func (c *ConsensusService) loadPrivVal(inst *Instance, config *tmcfg.Config, key PrivateKey) (*tmpv.FilePV, error) {
    key2, err := convertKeyToComet(inst, key)
    if err != nil {
        return nil, err
    }
    
    // Create FilePV with key and state file
    pv := tmpv.NewFilePV(key2, "", config.PrivValidatorStateFile())
    
    // Load or create validator state
    b, err := os.ReadFile(config.PrivValidatorStateFile())
    switch {
    case err == nil:
        err = cmtjson.Unmarshal(b, &pv.LastSignState)
        return pv, err
    case !errors.Is(err, fs.ErrNotExist):
        return nil, err
    }
    
    // Write initial state file
    b, err = cmtjson.MarshalIndent(pv.LastSignState, "", "  ")
    if err != nil {
        return nil, err
    }
    err = os.WriteFile(config.PrivValidatorStateFile(), b, 0600)
    return pv, err
}
```

**Private Validator State File (`priv_validator_state.json`)**:
```json
{
  "height": "0",
  "round": 0,
  "step": 0
}
```

#### Node Key (`node_key.json`)

**Purpose**: Ed25519 key used for P2P networking identification and secure communication.

**File Format**:
```json
{
  "priv_key": {
    "type": "tendermint/PrivKeyEd25519",
    "value": "base64-encoded-private-key"
  }
}
```

**Key Conversion Process**:
```go
func convertNodeKey(inst *Instance) (*tmp2p.NodeKey, error) {
    var key PrivateKey
    if inst.config.P2P != nil {
        key = inst.config.P2P.Key
    }
    key2, err := convertKeyToComet(inst, key)
    if err != nil {
        return nil, err
    }
    return &tmp2p.NodeKey{PrivKey: key2}, nil
}

func convertKeyToComet(inst *Instance, key PrivateKey) (tmcrypto.PrivKey, error) {
    addr, err := key.get(inst)
    if err != nil {
        return nil, err
    }
    
    sk, ok := addr.GetPrivateKey()
    if !ok {
        return nil, errors.BadRequest.With("not a private key")
    }
    
    switch addr.GetType() {
    case protocol.SignatureTypeED25519:
        return tmed25519.PrivKey(sk), nil
    default:
        return nil, errors.BadRequest.WithFormat("unsupported key type %v", addr.GetType())
    }
}
```

#### Key Security and Permissions

**File Permissions**:
- **Private keys**: `0600` (read/write owner only)
- **Configuration files**: `0644` (readable by group)
- **Directories**: `0700` (accessible by owner only)

**Key Validation Process**:
1. **Existence Check**: Verify key files exist in expected locations
2. **Format Validation**: Parse JSON structure and validate key types
3. **Cryptographic Validation**: Verify key pair consistency
4. **Permission Check**: Ensure appropriate file permissions

**Key Storage Locations**:
```
node-directory/
├── config/
│   ├── priv_validator_key.json     # Consensus signing key
│   ├── priv_validator_state.json   # Validator state tracking
│   ├── node_key.json               # P2P networking key
│   ├── accumulate.toml             # Accumulate configuration
│   └── tendermint.toml             # CometBFT configuration
├── data/                           # Blockchain data
└── genesis.json                    # Genesis document
```

### Complete Node Initialization Workflow
<!-- AI_TAG: init_workflow -->

#### Step-by-Step Initialization Process

1. **Directory Structure Creation**:
   ```go
   err := os.MkdirAll(inst.path(c.NodeDir, "config"), 0700)
   err = os.MkdirAll(inst.path(c.NodeDir, "data"), 0700)
   ```

2. **Configuration Generation**:
   - Generate default `tm.Config` from CometBFT
   - Create Accumulate-specific configuration
   - Merge configurations into unified `Config` struct

3. **Key Generation or Loading**:
   - Check for existing private validator key
   - Generate new Ed25519 key if missing
   - Create or load node key for P2P networking
   - Set appropriate file permissions

4. **Configuration File Writing**:
   ```go
   // Write CometBFT configuration
   tmcfg.WriteConfigFile(inst.path(c.NodeDir, "config", "tendermint.toml"), d.config)
   
   // Write Accumulate configuration
   err = StoreAcc(config, filepath.Join(config.RootDir, configDir))
   ```

5. **Genesis Document Handling**:
   - Load or generate genesis document
   - Validate genesis consistency
   - Write genesis file to node directory

6. **Validation and Cleanup**:
   - Validate all configuration files
   - Verify key file integrity
   - Clean up temporary files on error

#### Error Handling and Recovery

**Common Initialization Errors**:
1. **Permission Errors**: Insufficient file system permissions
2. **Key Corruption**: Invalid or corrupted key files
3. **Configuration Conflicts**: Incompatible configuration parameters
4. **Network Errors**: Unable to connect to bootstrap peers
5. **Genesis Mismatches**: Inconsistent genesis documents

**Recovery Procedures**:
1. **Clean Initialization**: Remove corrupted files and reinitialize
2. **Key Recovery**: Restore from backup or regenerate keys
3. **Configuration Reset**: Revert to default configuration
4. **Network Resync**: Clear data directory and resync from network

#### Production Deployment Considerations

**Security Best Practices**:
1. **Key Management**: Store private keys securely with restricted access
2. **Network Security**: Use firewalls to restrict P2P and API access
3. **Monitoring**: Implement comprehensive logging and metrics
4. **Backup Strategy**: Regular backup of keys and configuration
5. **Update Procedures**: Planned maintenance and upgrade processes

**Performance Optimization**:
1. **Storage Configuration**: Choose appropriate database backend
2. **Memory Settings**: Configure mempool and cache sizes
3. **Network Tuning**: Optimize P2P connection limits
4. **Metrics Collection**: Enable Prometheus for monitoring
5. **Log Management**: Configure appropriate log levels and rotation

---

## Related Documentation

- [MainNet Reference](../network/accumulate-mainnet-reference.md) - Network specifications and configuration
- [Deployment Guide](../deployment/cyclops-deployment-guide.md) - Cyclops network deployment automation
- [Network Glossary](../network/accumulate-network-glossary.md) - Terminology definitions
