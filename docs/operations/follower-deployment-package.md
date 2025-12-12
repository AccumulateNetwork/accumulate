# Building a Portable Follower Deployment Package

This document describes exactly how to build a deployment structure that can be zipped, moved to a new computer, and run as a follower in a Docker container.

## Overview

A follower deployment consists of:
- The `accumulated` binary
- Configuration files for both partitions
- LevelDB databases restored from snapshots
- Proper peer configuration

**Total deployment size**: ~29 GB (varies based on snapshot age)

## Snapshot Information

### Source Snapshots (Dec 1, 2025)

| Snapshot | File | Size | SHA256 Hash |
|----------|------|------|-------------|
| Directory | directory.snap | 8.0 GB | `3e1292220bfccd07477061dc32948da70522e0bbe8911c5d3a09e58631335810` |
| Cyclops BVN | cyclops.snap | 14.6 GB | `c85ec89f4f3d7f1b975935ef0e823b335fee17b69398657f9fff99a9634d62f3` |

### Snapshot Contents

Each snapshot contains:
- Block headers and data
- Application state (accounts, transactions, etc.)
- Validator information
- Genesis configuration

## Directory Structure

### Complete Deployment Tree

```
accumulate-follower/                    # Root deployment directory
├── accumulated                         # Binary executable (~50MB)
├── accumulate.toml                     # Root configuration file
├── start.sh                            # Optional startup script
├── logs/                               # Runtime logs directory
│
├── dnn/                                # Directory Node partition
│   ├── config/
│   │   ├── genesis.json               # Chain genesis (555 bytes)
│   │   ├── node_key.json              # P2P identity key (148 bytes)
│   │   ├── priv_validator_key.json    # Validator key (345 bytes, unused for follower)
│   │   ├── tendermint.toml            # CometBFT config (~19KB)
│   │   ├── accumulate.toml            # Partition-specific config (~870 bytes)
│   │   └── addrbook.json              # Peer address book (regenerated)
│   └── data/
│       ├── accumulate.db/             # Accumulate application state (394 MB)
│       ├── blockstore.db/             # Block storage (11 GB)
│       ├── state.db/                  # Consensus state (293 MB)
│       ├── tx_index.db/               # Transaction index (9.3 MB)
│       ├── evidence.db/               # Evidence storage (28 KB)
│       ├── cs.wal/                    # Consensus WAL (regenerated)
│       └── priv_validator_state.json  # Validator state (regenerated)
│
└── bvnn/                               # BVN partition (Cyclops)
    ├── config/
    │   ├── genesis.json               # Chain genesis (553 bytes)
    │   ├── node_key.json              # P2P identity key (148 bytes)
    │   ├── priv_validator_key.json    # Validator key (345 bytes, unused for follower)
    │   ├── tendermint.toml            # CometBFT config (~19KB)
    │   ├── accumulate.toml            # Partition-specific config (~870 bytes)
    │   └── addrbook.json              # Peer address book (regenerated)
    └── data/
        ├── accumulate.db/             # Accumulate application state (4.5 GB)
        ├── blockstore.db/             # Block storage (11 GB)
        ├── state.db/                  # Consensus state (2.6 GB)
        ├── tx_index.db/               # Transaction index (5.4 MB)
        ├── evidence.db/               # Evidence storage (20 KB)
        ├── cs.wal/                    # Consensus WAL (regenerated)
        └── priv_validator_state.json  # Validator state (regenerated)
```

### Database Descriptions

| Database | Description | DN Size | BVN Size |
|----------|-------------|---------|----------|
| accumulate.db | Accumulate application state (accounts, chains, transactions) | 394 MB | 4.5 GB |
| blockstore.db | Raw block data from CometBFT consensus | 11 GB | 11 GB |
| state.db | CometBFT consensus state (validators, commits) | 293 MB | 2.6 GB |
| tx_index.db | Transaction indexing for queries | 9.3 MB | 5.4 MB |
| evidence.db | Byzantine evidence storage | 28 KB | 20 KB |

### Sample Database File Hashes (Dec 1, 2025 Snapshot)

LevelDB databases contain many .ldb files. Sample hashes for verification:

**Directory Node (dnn/data/)**:
| File | SHA256 |
|------|--------|
| state.db/001827.ldb | `562ba558b7d91114f124a6f4800d960141b492705123bd23dd450fc3e3d0c3de` |
| blockstore.db/010927.ldb | `f277d7093a2d5717673e29e1637dc4429dbd22753ec1472b28fd3e1ed943f892` |

**BVN Node (bvnn/data/)**:
| File | SHA256 |
|------|--------|
| state.db/000010.ldb | `e338ffa3393af236e554723d414a52e53df1b6aafe2020adcf3a7ca46212da8b` |
| blockstore.db/020087.ldb | `3b858c085089b5feea64f67f9b021f2100405f3f0874828f7552949ce7eabeaf` |

Note: Individual .ldb file names vary based on compaction. Use snapshot hashes for primary verification.

### Files That Must Be Removed Before Packaging

These are regenerated at startup and can cause issues if stale:

```bash
# Address books (rebuilt from persistent_peers)
rm -f dnn/config/addrbook.json
rm -f bvnn/config/addrbook.json

# Consensus WAL (rebuilt on startup)
rm -f dnn/data/cs.wal/wal
rm -f bvnn/data/cs.wal/wal

# Validator state (rebuilt for follower)
rm -f dnn/data/priv_validator_state.json
rm -f bvnn/data/priv_validator_state.json

# Atomic write temp files
rm -f dnn/config/write-file-atomic-*
rm -f bvnn/config/write-file-atomic-*
```

## Configuration Files

### Root accumulate.toml

This is the main configuration file that controls the follower:

```toml
network = "mainnet"

[[configurations]]
  type = "follower"
  mode = "dual"
  bvn = "Cyclops"
  listen = "/ip4/0.0.0.0/tcp/16591"
  storage-type = "leveldb"
  enable-healing = false
  enable-snapshots = false
  dn-genesis = "dnn/config/genesis.json"
  bvn-genesis = "bvnn/config/genesis.json"

  # CRITICAL: Empty arrays prevent overwriting tendermint.toml on startup
  dn-bootstrap-peers = []
  bvn-bootstrap-peers = []

[logging]
  format = "plain"
  [[logging.rules]]
    level = "info"
```

**CRITICAL**: The `dn-bootstrap-peers = []` and `bvn-bootstrap-peers = []` MUST be empty arrays. If populated, the node will overwrite `tendermint.toml` with these peers on every startup, undoing your peer configuration.

### Genesis File Format

Each partition has a genesis.json that defines the chain:

**Directory (dnn/config/genesis.json)**:
```json
{
  "genesis_time": "2025-11-24T06:46:32.800831907Z",
  "chain_id": "MainNet.Directory",
  "initial_height": "1",
  "consensus_params": {
    "block": { "max_bytes": "22020096", "max_gas": "-1" },
    "evidence": {
      "max_age_num_blocks": "100000",
      "max_age_duration": "172800000000000",
      "max_bytes": "1048576"
    },
    "validator": { "pub_key_types": ["ed25519"] },
    "version": { "app": "0" },
    "abci": { "vote_extensions_enable_height": "0" }
  },
  "app_hash": ""
}
```

**BVN (bvnn/config/genesis.json)**:
```json
{
  "genesis_time": "2025-11-24T06:45:34.977186017Z",
  "chain_id": "MainNet.Cyclops",
  "initial_height": "1",
  ...
}
```

### Node Key Format

The `node_key.json` contains the P2P identity:

```json
{
  "priv_key": {
    "type": "tendermint/PrivKeyEd25519",
    "value": "base64-encoded-64-byte-ed25519-private-key"
  }
}
```

The node ID is derived from the public key (first 20 bytes of SHA256 hash, hex encoded).

### Priv Validator Key Format

The `priv_validator_key.json` (unused for followers but required):

```json
{
  "address": "BA52C9CEF0AEEA5ED43E560BEB63D8EBE60BDEAB",
  "pub_key": {
    "type": "tendermint/PubKeyEd25519",
    "value": "base64-encoded-32-byte-public-key"
  },
  "priv_key": {
    "type": "tendermint/PrivKeyEd25519",
    "value": "base64-encoded-64-byte-private-key"
  }
}
```

### Complete tendermint.toml Configuration Files

These are the actual complete configuration files used in the deployment.

**Directory Node (dnn/config/tendermint.toml)**:

<details>
<summary>Click to expand complete DN tendermint.toml (463 lines)</summary>

```toml
# This is a TOML config file.
# For more information, see https://github.com/toml-lang/toml

# NOTE: Any path below can be absolute (e.g. "/var/myawesomeapp/data") or
# relative to the home directory (e.g. "data"). The home directory is
# "$HOME/.cometbft" by default, but could be changed via $CMTHOME env variable
# or --home cmd flag.

# The version of the CometBFT binary that created or
# last modified the config file. Do not modify this.
version = "0.38.0-rc3"

#######################################################################
###                   Main Base Config Options                      ###
#######################################################################

# TCP or UNIX socket address of the ABCI application,
# or the name of an ABCI application compiled in with the CometBFT binary
proxy_app = ""

# A custom human readable name for this node
moniker = "76-fun"

# Database backend: goleveldb | cleveldb | boltdb | rocksdb | badgerdb
db_backend = "goleveldb"

# Database directory
db_dir = "data"

# Output level for logging, including package level options
log_level = "error;statesync=info;snapshot=info;restore=info;executor=info;synthetic=info;website=info"

# Output format: 'plain' (colored text) or 'json'
log_format = "plain"

##### additional base config options #####

# Path to the JSON file containing the initial validator set and other meta data
genesis_file = "config/genesis.json"

# Path to the JSON file containing the private key to use as a validator in the consensus protocol
priv_validator_key_file = "config/priv_validator_key.json"

# Path to the JSON file containing the last sign state of a validator
priv_validator_state_file = "data/priv_validator_state.json"

# TCP or UNIX socket address for CometBFT to listen on for
# connections from an external PrivValidator process
priv_validator_laddr = ""

# Path to the JSON file containing the private key to use for node authentication in the p2p protocol
node_key_file = "config/node_key.json"

# Mechanism to connect to the ABCI application: socket | grpc
abci = "socket"

# If true, query the ABCI app on connecting to a new peer
# so the app can decide if we should keep the connection or not
filter_peers = false


#######################################################################
###                 Advanced Configuration Options                  ###
#######################################################################

#######################################################
###       RPC Server Configuration Options          ###
#######################################################
[rpc]

# TCP or UNIX socket address for the RPC server to listen on
laddr = "tcp://127.0.0.1:16592"

# A list of origins a cross-domain request can be executed from
cors_allowed_origins = []

# A list of methods the client is allowed to use with cross-domain requests
cors_allowed_methods = ["HEAD", "GET", "POST", ]

# A list of non simple headers the client is allowed to use with cross-domain requests
cors_allowed_headers = ["Origin", "Accept", "Content-Type", "X-Requested-With", "X-Server-Time", ]

# TCP or UNIX socket address for the gRPC server to listen on
grpc_laddr = ""

# Maximum number of simultaneous connections.
grpc_max_open_connections = 900

# Activate unsafe RPC commands like /dial_seeds and /unsafe_flush_mempool
unsafe = false

# Maximum number of simultaneous connections (including WebSocket).
max_open_connections = 900

# Maximum number of unique clientIDs that can /subscribe
max_subscription_clients = 100

# Maximum number of unique queries a given client can /subscribe to
max_subscriptions_per_client = 5

# Experimental parameter to specify the maximum number of events a node will buffer
experimental_subscription_buffer_size = 200

# Experimental parameter to specify the maximum number of RPC responses that can be buffered
experimental_websocket_write_buffer_size = 200

# Enabling this experimental parameter will cause the WebSocket connection to be closed if it cannot read fast enough
experimental_close_on_slow_client = false

# How long to wait for a tx to be committed during /broadcast_tx_commit.
timeout_broadcast_tx_commit = "10s"

# Maximum size of request body, in bytes
max_body_bytes = 1000000

# Maximum size of request header, in bytes
max_header_bytes = 1048576

# The path to a file containing certificate that is used to create the HTTPS server.
tls_cert_file = ""

# The path to a file containing matching private key that is used to create the HTTPS server.
tls_key_file = ""

# pprof listen address (https://golang.org/pkg/net/http/pprof)
pprof_laddr = ""

#######################################################
###           P2P Configuration Options             ###
#######################################################
[p2p]

# Address to listen for incoming connections
laddr = "tcp://0.0.0.0:16591"

# Address to advertise to peers for them to dial
external_address = ""

# Comma separated list of seed nodes to connect to
seeds = ""

# Comma separated list of nodes to keep persistent connections to
# CRITICAL: These must be Directory partition peers on port 16591
persistent_peers = "ebb29bee942723271a39217bd0ed62f7827245de@144.76.105.23:16591,b006ca4bca16d89a808492ce15e0acfec4b3e94a@23.22.212.106:16591,412d5573ffe3c581801f6c56217f438d5020b277@23.22.212.106:16591,3029240e829e58e399bc7b6115bb6bc947cc24c7@23.22.212.106:16591"

# UPNP port forwarding
upnp = false

# Path to address book
addr_book_file = "config/addrbook.json"

# Set true for strict address routability rules
addr_book_strict = true

# Maximum number of inbound peers
max_num_inbound_peers = 40

# Maximum number of outbound peers to connect to, excluding persistent peers
max_num_outbound_peers = 10

# List of node IDs, to which a connection will be (re)established ignoring any existing limits
unconditional_peer_ids = ""

# Maximum pause when redialing a persistent peer (if zero, exponential backoff is used)
persistent_peers_max_dial_period = "0s"

# Time to wait before flushing messages out on the connection
flush_throttle_timeout = "100ms"

# Maximum size of a message packet payload, in bytes
max_packet_msg_payload_size = 1024

# Rate at which packets can be sent, in bytes/second
send_rate = 5120000

# Rate at which packets can be received, in bytes/second
recv_rate = 5120000

# Set true to enable the peer-exchange reactor
pex = true

# Seed mode, in which node constantly crawls the network and looks for peers.
seed_mode = false

# Comma separated list of peer IDs to keep private (will not be gossiped to other peers)
private_peer_ids = ""

# Toggle to disable guard against peers connecting from the same ip.
allow_duplicate_ip = false

# Peer connection configuration.
handshake_timeout = "20s"
dial_timeout = "3s"

#######################################################
###          Mempool Configuration Option          ###
#######################################################
[mempool]

recheck = true
broadcast = true
wal_dir = ""
size = 5000
max_txs_bytes = 1073741824
cache_size = 10000
keep-invalid-txs-in-cache = false
max_tx_bytes = 1048576
max_batch_bytes = 0

#######################################################
###         State Sync Configuration Options        ###
#######################################################
[statesync]
# State sync is DISABLED for snapshot-restored nodes
enable = false

# RPC servers for light client verification (not used when enable = false)
rpc_servers = ""
trust_height = 0
trust_hash = ""
trust_period = "168h0m0s"

# Time to spend discovering snapshots before initiating a restore.
discovery_time = "15s"

# Temporary directory for state sync snapshot chunks
temp_dir = ""

# The timeout duration before re-requesting a chunk
chunk_request_timeout = "10s"

# The number of concurrent chunk fetchers to run
chunk_fetchers = "4"

#######################################################
###       Block Sync Configuration Options          ###
#######################################################
[blocksync]

version = "v0"

#######################################################
###         Consensus Configuration Options         ###
#######################################################
[consensus]

wal_file = "data/cs.wal/wal"

# How long we wait for a proposal block before prevoting nil
timeout_propose = "3s"
timeout_propose_delta = "500ms"
timeout_prevote = "1s"
timeout_prevote_delta = "500ms"
timeout_precommit = "1s"
timeout_precommit_delta = "500ms"
timeout_commit = "1s"

double_sign_check_height = 0
skip_timeout_commit = false
create_empty_blocks = true
create_empty_blocks_interval = "0s"

peer_gossip_sleep_duration = "100ms"
peer_query_maj23_sleep_duration = "2s"

#######################################################
###         Storage Configuration Options           ###
#######################################################
[storage]

discard_abci_responses = false

#######################################################
###   Transaction Indexer Configuration Options     ###
#######################################################
[tx_index]

indexer = "kv"
psql-conn = ""

#######################################################
###       Instrumentation Configuration Options     ###
#######################################################
[instrumentation]

prometheus = true
prometheus_listen_addr = ":26660"
max_open_connections = 3
namespace = "consensus_Directory"
```

</details>

**BVN Node (bvnn/config/tendermint.toml)**:

<details>
<summary>Click to expand complete BVN tendermint.toml (463 lines)</summary>

```toml
# This is a TOML config file.
# For more information, see https://github.com/toml-lang/toml

# NOTE: Any path below can be absolute (e.g. "/var/myawesomeapp/data") or
# relative to the home directory (e.g. "data"). The home directory is
# "$HOME/.cometbft" by default, but could be changed via $CMTHOME env variable
# or --home cmd flag.

# The version of the CometBFT binary that created or
# last modified the config file. Do not modify this.
version = "0.38.0-rc3"

#######################################################################
###                   Main Base Config Options                      ###
#######################################################################

# TCP or UNIX socket address of the ABCI application,
# or the name of an ABCI application compiled in with the CometBFT binary
proxy_app = ""

# A custom human readable name for this node
moniker = "76-fun"

# Database backend: goleveldb | cleveldb | boltdb | rocksdb | badgerdb
db_backend = "goleveldb"

# Database directory
db_dir = "data"

# Output level for logging, including package level options
log_level = "error;statesync=info;snapshot=info;restore=info;executor=info;synthetic=info;website=info"

# Output format: 'plain' (colored text) or 'json'
log_format = "plain"

##### additional base config options #####

# Path to the JSON file containing the initial validator set and other meta data
genesis_file = "config/genesis.json"

# Path to the JSON file containing the private key to use as a validator in the consensus protocol
priv_validator_key_file = "config/priv_validator_key.json"

# Path to the JSON file containing the last sign state of a validator
priv_validator_state_file = "data/priv_validator_state.json"

# TCP or UNIX socket address for CometBFT to listen on for
# connections from an external PrivValidator process
priv_validator_laddr = ""

# Path to the JSON file containing the private key to use for node authentication in the p2p protocol
node_key_file = "config/node_key.json"

# Mechanism to connect to the ABCI application: socket | grpc
abci = "socket"

# If true, query the ABCI app on connecting to a new peer
# so the app can decide if we should keep the connection or not
filter_peers = false


#######################################################################
###                 Advanced Configuration Options                  ###
#######################################################################

#######################################################
###       RPC Server Configuration Options          ###
#######################################################
[rpc]

# TCP or UNIX socket address for the RPC server to listen on
laddr = "tcp://127.0.0.1:16692"

# A list of origins a cross-domain request can be executed from
cors_allowed_origins = []

# A list of methods the client is allowed to use with cross-domain requests
cors_allowed_methods = ["HEAD", "GET", "POST", ]

# A list of non simple headers the client is allowed to use with cross-domain requests
cors_allowed_headers = ["Origin", "Accept", "Content-Type", "X-Requested-With", "X-Server-Time", ]

# TCP or UNIX socket address for the gRPC server to listen on
grpc_laddr = ""

# Maximum number of simultaneous connections.
grpc_max_open_connections = 900

# Activate unsafe RPC commands like /dial_seeds and /unsafe_flush_mempool
unsafe = false

# Maximum number of simultaneous connections (including WebSocket).
max_open_connections = 900

# Maximum number of unique clientIDs that can /subscribe
max_subscription_clients = 100

# Maximum number of unique queries a given client can /subscribe to
max_subscriptions_per_client = 5

# Experimental parameter to specify the maximum number of events a node will buffer
experimental_subscription_buffer_size = 200

# Experimental parameter to specify the maximum number of RPC responses that can be buffered
experimental_websocket_write_buffer_size = 200

# Enabling this experimental parameter will cause the WebSocket connection to be closed if it cannot read fast enough
experimental_close_on_slow_client = false

# How long to wait for a tx to be committed during /broadcast_tx_commit.
timeout_broadcast_tx_commit = "10s"

# Maximum size of request body, in bytes
max_body_bytes = 1000000

# Maximum size of request header, in bytes
max_header_bytes = 1048576

# The path to a file containing certificate that is used to create the HTTPS server.
tls_cert_file = ""

# The path to a file containing matching private key that is used to create the HTTPS server.
tls_key_file = ""

# pprof listen address (https://golang.org/pkg/net/http/pprof)
pprof_laddr = ""

#######################################################
###           P2P Configuration Options             ###
#######################################################
[p2p]

# Address to listen for incoming connections
laddr = "tcp://0.0.0.0:16691"

# Address to advertise to peers for them to dial
external_address = ""

# Comma separated list of seed nodes to connect to
seeds = ""

# Comma separated list of nodes to keep persistent connections to
# CRITICAL: These must be Cyclops BVN peers on port 16691
persistent_peers = "3029240e829e58e399bc7b6115bb6bc947cc24c7@23.22.212.106:16691"

# UPNP port forwarding
upnp = false

# Path to address book
addr_book_file = "config/addrbook.json"

# Set true for strict address routability rules
addr_book_strict = true

# Maximum number of inbound peers
max_num_inbound_peers = 40

# Maximum number of outbound peers to connect to, excluding persistent peers
max_num_outbound_peers = 10

# List of node IDs, to which a connection will be (re)established ignoring any existing limits
unconditional_peer_ids = ""

# Maximum pause when redialing a persistent peer (if zero, exponential backoff is used)
persistent_peers_max_dial_period = "0s"

# Time to wait before flushing messages out on the connection
flush_throttle_timeout = "100ms"

# Maximum size of a message packet payload, in bytes
max_packet_msg_payload_size = 1024

# Rate at which packets can be sent, in bytes/second
send_rate = 5120000

# Rate at which packets can be received, in bytes/second
recv_rate = 5120000

# Set true to enable the peer-exchange reactor
pex = true

# Seed mode, in which node constantly crawls the network and looks for peers.
seed_mode = false

# Comma separated list of peer IDs to keep private (will not be gossiped to other peers)
private_peer_ids = ""

# Toggle to disable guard against peers connecting from the same ip.
allow_duplicate_ip = false

# Peer connection configuration.
handshake_timeout = "20s"
dial_timeout = "3s"

#######################################################
###          Mempool Configuration Option          ###
#######################################################
[mempool]

recheck = true
broadcast = true
wal_dir = ""
size = 5000
max_txs_bytes = 1073741824
cache_size = 10000
keep-invalid-txs-in-cache = false
max_tx_bytes = 1048576
max_batch_bytes = 0

#######################################################
###         State Sync Configuration Options        ###
#######################################################
[statesync]
# State sync parameters (for reference - enable = false for snapshot-restored nodes)
enable = false

# RPC servers for state sync (example configuration)
rpc_servers = "http://apollo-mainnet.accumulate.defidevs.io:16692,http://apollo-mainnet.accumulate.defidevs.io:16692"
trust_height = 12249000
trust_hash = "2FEF2659DC1858636B1F02065754A981EE587399FF59AFA48F36E50B2F71AEB7"
trust_period = "168h0m0s"

# Time to spend discovering snapshots before initiating a restore.
discovery_time = "15s"

# Temporary directory for state sync snapshot chunks
temp_dir = ""

# The timeout duration before re-requesting a chunk
chunk_request_timeout = "10s"

# The number of concurrent chunk fetchers to run
chunk_fetchers = "4"

#######################################################
###       Block Sync Configuration Options          ###
#######################################################
[blocksync]

version = "v0"

#######################################################
###         Consensus Configuration Options         ###
#######################################################
[consensus]

wal_file = "data/cs.wal/wal"

# How long we wait for a proposal block before prevoting nil
timeout_propose = "3s"
timeout_propose_delta = "500ms"
timeout_prevote = "1s"
timeout_prevote_delta = "500ms"
timeout_precommit = "1s"
timeout_precommit_delta = "500ms"
timeout_commit = "1s"

double_sign_check_height = 0
skip_timeout_commit = false
create_empty_blocks = true
create_empty_blocks_interval = "0s"

peer_gossip_sleep_duration = "100ms"
peer_query_maj23_sleep_duration = "2s"

#######################################################
###         Storage Configuration Options           ###
#######################################################
[storage]

discard_abci_responses = false

#######################################################
###   Transaction Indexer Configuration Options     ###
#######################################################
[tx_index]

indexer = "kv"
psql-conn = ""

#######################################################
###       Instrumentation Configuration Options     ###
#######################################################
[instrumentation]

prometheus = true
prometheus_listen_addr = ":26670"
max_open_connections = 3
namespace = "consensus_Cyclops"
```

</details>

### Key Configuration Differences Between DN and BVN

| Setting | Directory Node (DN) | BVN Node (Cyclops) |
|---------|---------------------|-------------------|
| RPC port | 16592 | 16692 |
| P2P port | 16591 | 16691 |
| Prometheus port | 26660 | 26670 |
| Namespace | consensus_Directory | consensus_Cyclops |
| Persistent peers | Directory peers on 16591 | BVN peers on 16691 |

## Port Reference

| Partition | Service | Port | Description |
|-----------|---------|------|-------------|
| Directory | P2P | 16591 | Peer-to-peer consensus |
| Directory | RPC | 16592 | Tendermint RPC API |
| Cyclops BVN | P2P | 16691 | Peer-to-peer consensus |
| Cyclops BVN | RPC | 16692 | Tendermint RPC API |
| Accumulate | API | 16593 | Accumulate JSON-RPC API |

## Peer Configuration (CRITICAL)

### Finding Working Peers

Query the mainnet API:

```bash
# Get all peers from network status
curl -s https://mainnet.accumulatenetwork.io/v3 -X POST \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"network-status","params":{}}' \
  | jq '.result.network.partitions'
```

### Verifying Peer Identity

**CRITICAL**: Always verify the node ID and partition before adding a peer:

```bash
# Check peer status (RPC port = P2P port + 1)
curl -s http://PEER_IP:PORT/status | jq '{
  node_id: .result.node_info.id,
  network: .result.node_info.network,
  height: .result.sync_info.latest_block_height
}'
```

Expected output:
- **Directory**: `network: "MainNet"` or `"MainNet.Directory"`
- **Cyclops BVN**: `network: "MainNet.Cyclops"`

### Common Peer Mistakes

1. **Wrong partition**: A Directory peer on port 16591 won't help your BVN on port 16691
2. **ID mismatch**: The configured ID must match the peer's actual node ID
3. **Stale peers**: Old peers may be offline or have changed IDs

### Current Working Peers (Dec 2025)

**Directory (DN) - Port 16591**:
```
ebb29bee942723271a39217bd0ed62f7827245de@144.76.105.23:16591
3029240e829e58e399bc7b6115bb6bc947cc24c7@23.22.212.106:16591
```

**Cyclops (BVN) - Port 16691**:
```
3029240e829e58e399bc7b6115bb6bc947cc24c7@23.22.212.106:16691
```

**WARNING**: The peer `ba238200737bad88d4e9407fec6858fdc05d6dca@144.76.105.23:16691` serves DIRECTORY data on port 16691, NOT Cyclops BVN. Always verify!

## Build Process

### Step 1: Create Base Structure

```bash
WORK_DIR=/path/to/accumulate-follower
DN_SNAPSHOT=/path/to/directory.snap
BVN_SNAPSHOT=/path/to/cyclops.snap

./deploy-follower \
  --work-dir "$WORK_DIR" \
  --dn-snapshot "$DN_SNAPSHOT" \
  --bvn-snapshot "$BVN_SNAPSHOT" \
  --accumulated ./accumulated \
  --network mainnet \
  --bvn Cyclops
```

### Step 2: Configure Root accumulate.toml

```bash
cat > "$WORK_DIR/accumulate.toml" << 'EOF'
network = "mainnet"

[[configurations]]
  type = "follower"
  mode = "dual"
  bvn = "Cyclops"
  listen = "/ip4/0.0.0.0/tcp/16591"
  storage-type = "leveldb"
  enable-healing = false
  enable-snapshots = false
  dn-genesis = "dnn/config/genesis.json"
  bvn-genesis = "bvnn/config/genesis.json"
  dn-bootstrap-peers = []
  bvn-bootstrap-peers = []

[logging]
  format = "plain"
  [[logging.rules]]
    level = "info"
EOF
```

### Step 3: Configure Peers

Edit `dnn/config/tendermint.toml` line ~214:
```toml
persistent_peers = "ebb29bee942723271a39217bd0ed62f7827245de@144.76.105.23:16591,3029240e829e58e399bc7b6115bb6bc947cc24c7@23.22.212.106:16591"
```

Edit `bvnn/config/tendermint.toml` line ~214:
```toml
persistent_peers = "3029240e829e58e399bc7b6115bb6bc947cc24c7@23.22.212.106:16691"
```

### Step 4: Clean Up

```bash
cd "$WORK_DIR"

# Remove regenerated files
rm -f dnn/config/addrbook.json bvnn/config/addrbook.json
rm -f dnn/data/cs.wal/wal bvnn/data/cs.wal/wal
rm -f dnn/data/priv_validator_state.json bvnn/data/priv_validator_state.json
rm -f dnn/config/write-file-atomic-* bvnn/config/write-file-atomic-*
```

### Step 5: Create Package

```bash
cd /path/to
tar -czvf accumulate-follower.tar.gz accumulate-follower/
```

## Docker Deployment

### Dockerfile

```dockerfile
FROM ubuntu:22.04

RUN apt-get update && apt-get install -y \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /accumulate
COPY . .
RUN chmod +x accumulated

# DN: P2P=16591, RPC=16592
# BVN: P2P=16691, RPC=16692
EXPOSE 16591 16592 16691 16692

HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
  CMD curl -sf http://localhost:16592/status || exit 1

CMD ["./accumulated", "run-dual", "dnn", "bvnn"]
```

### docker-compose.yml

```yaml
version: '3.8'

services:
  accumulate-follower:
    build: .
    container_name: accumulate-follower
    restart: unless-stopped
    ports:
      - "16591:16591"
      - "16592:16592"
      - "16691:16691"
      - "16692:16692"
    volumes:
      - ./dnn/data:/accumulate/dnn/data
      - ./bvnn/data:/accumulate/bvnn/data
    logging:
      driver: "json-file"
      options:
        max-size: "100m"
        max-file: "5"
```

### Run Commands

```bash
# Extract
tar -xzvf accumulate-follower.tar.gz
cd accumulate-follower

# Option A: Direct
./accumulated run-dual dnn bvnn

# Option B: Docker Compose
docker-compose up -d

# Option C: Docker manual
docker build -t accumulate-follower .
docker run -d --name accumulate-follower \
  -p 16591:16591 -p 16592:16592 \
  -p 16691:16691 -p 16692:16692 \
  -v $(pwd)/dnn/data:/accumulate/dnn/data \
  -v $(pwd)/bvnn/data:/accumulate/bvnn/data \
  accumulate-follower
```

## Verification

### Check Sync Status

```bash
# Directory status
curl -s http://localhost:16592/status | jq '{
  height: .result.sync_info.latest_block_height,
  catching_up: .result.sync_info.catching_up,
  peers: .result.n_peers
}'

# BVN status
curl -s http://localhost:16692/status | jq '{
  height: .result.sync_info.latest_block_height,
  catching_up: .result.sync_info.catching_up,
  peers: .result.n_peers
}'
```

### Expected Behavior

- `catching_up: true` - Node is syncing blocks from peers
- `catching_up: false` with height matching network - Node is synced
- `catching_up: false` with height far behind - **PROBLEM** - check peers

### Check Peer Connections

```bash
# DN peers
curl -s http://localhost:16592/net_info | jq '.result.n_peers, [.result.peers[].node_info.id]'

# BVN peers
curl -s http://localhost:16692/net_info | jq '.result.n_peers, [.result.peers[].node_info.id]'
```

## Troubleshooting

### BVN Stuck with `catching_up: false` But Far Behind

This is the most common issue. The blocksync reactor thinks it's caught up.

**Cause**: Wrong peers - either wrong partition or wrong node IDs.

**Fix**:
1. Verify peer serves the correct partition:
   ```bash
   curl -s http://PEER_IP:16692/status | jq '.result.node_info.network'
   # Must show "MainNet.Cyclops" for BVN
   ```
2. Update `persistent_peers` with correct peer
3. Clear addrbook and restart

### Config Keeps Getting Reset

**Cause**: `accumulate.toml` has populated bootstrap-peers arrays.

**Fix**: Ensure these are empty:
```toml
dn-bootstrap-peers = []
bvn-bootstrap-peers = []
```

### Auth Failure Errors

```
auth failure: conn.ID (xxx) dialed ID (yyy) mismatch
```

**Cause**: The peer at that IP has a different node ID than configured.

**Fix**: Query the peer's actual ID and update config:
```bash
curl -s http://PEER_IP:PORT/status | jq '.result.node_info.id'
```

### No Peers Connecting

1. Check firewall allows outbound on 16591/16691
2. Test connectivity: `nc -zv PEER_IP PORT`
3. Check peers are online: `curl -s http://PEER_IP:PORT/status`

## Checklist

Before packaging:
- [ ] Deploy-follower completed successfully
- [ ] Root accumulate.toml has empty bootstrap-peers arrays
- [ ] DN tendermint.toml has correct persistent_peers (port 16591)
- [ ] BVN tendermint.toml has correct persistent_peers (port 16691)
- [ ] Verified peer IDs match actual node IDs
- [ ] Verified peers serve correct partitions
- [ ] Removed addrbook.json files
- [ ] Removed cs.wal/wal files
- [ ] Removed priv_validator_state.json files
- [ ] Removed write-file-atomic-* temp files

After starting:
- [ ] DN shows `catching_up: true` or synced
- [ ] BVN shows `catching_up: true` or synced
- [ ] Both partitions have peer connections
- [ ] Heights are advancing
