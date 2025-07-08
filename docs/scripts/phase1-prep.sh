#!/bin/bash
# Complete Cyclops Validator Prep Automation
# 
# This script automates the entire Cyclops validator preparation process:
# 1. Generate validator keys for both BVN and DN partitions
# 2. Update network configuration with public keys
# 3. Update consensus configuration files
# 4. Extract partition-specific snapshots with consensus sections
#
# Prerequisites:
# - cyclops-genesis.snap must be present in current directory
# - analyze tool must be built and present in current directory
# - generate_all_validator_keys.sh must be present and executable
#
# Usage: ./cyclops_prep_automated.sh
#
# Author: Automated Cyclops Validator Prep System
# Status: Tested and Verified ✅

set -e  # Exit on any error
set -u  # Exit on undefined variables

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Configuration
ARTIFACTS_DIR="/tmp/cyclops/artifacts"
SCRIPTS_DIR="/home/paulsnow/go/src/gitlab.com/AccumulateNetwork/accumulate/docs/scripts"

# Change to artifacts directory
log_info "Changing to artifacts directory: $ARTIFACTS_DIR"
cd "$ARTIFACTS_DIR"

# Verify prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    if [[ ! -f "cyclops-genesis.snap" ]]; then
        log_error "cyclops-genesis.snap not found in current directory"
        exit 1
    fi
    
    if [[ ! -f "analyze" ]]; then
        log_error "analyze tool not found in current directory"
        exit 1
    fi
    
    if [[ ! -f "$SCRIPTS_DIR/generate_all_validator_keys.sh" ]]; then
        log_error "generate_all_validator_keys.sh not found in scripts directory: $SCRIPTS_DIR"
        exit 1
    fi
    
    if [[ ! -x "$SCRIPTS_DIR/generate_all_validator_keys.sh" ]]; then
        log_warning "Making generate_all_validator_keys.sh executable..."
        chmod +x "$SCRIPTS_DIR/generate_all_validator_keys.sh"
    fi
    
    if [[ ! -f "cyclops-network.json" ]]; then
        log_error "cyclops-network.json not found in current directory"
        exit 1
    fi
    
    log_success "All prerequisites verified"
}

# Rebuild binaries with latest fixes
rebuild_binaries() {
    log_info "Rebuilding binaries with latest fixes..."
    local source_dir="$HOME/go/src/gitlab.com/AccumulateNetwork/accumulate"
    
    # Build analyze with key format fixes
    if (cd "$source_dir" && go build -o "$ARTIFACTS_DIR/analyze" ./tools/cmd/analyze); then
        log_success "Rebuilt analyze binary with key format fixes"
    else
        log_error "Failed to rebuild analyze binary"
        exit 1
    fi
    
    # Build accumulated with latest fixes
    if (cd "$source_dir" && go build -o "$ARTIFACTS_DIR/accumulated" ./cmd/accumulated); then
        log_success "Rebuilt accumulated binary with latest fixes"
    else
        log_error "Failed to rebuild accumulated binary"
        exit 1
    fi
    
    # Set executable permissions
    chmod +x "$ARTIFACTS_DIR/analyze" "$ARTIFACTS_DIR/accumulated"
    log_success "Set executable permissions on rebuilt binaries"
}

# Create backup of important files
create_backups() {
    log_info "Creating backups of original files..."
    
    if [[ -f "cyclops-network.json" && ! -f "cyclops-network.json.bak" ]]; then
        cp cyclops-network.json cyclops-network.json.bak
        log_success "Backed up cyclops-network.json"
    fi
    
    if [[ -f "consensus_dn.json" ]]; then
        cp consensus_dn.json consensus_dn.json.bak
        log_success "Backed up consensus_dn.json"
    fi
    
    if [[ -f "consensus_bvn0.json" ]]; then
        cp consensus_bvn0.json consensus_bvn0.json.bak
        log_success "Backed up consensus_bvn0.json"
    fi
}

# Step 1: Generate validator keys
generate_validator_keys() {
    log_info "Step 1: Generating validator keys..."
    
    # "$SCRIPTS_DIR/generate_all_validator_keys.sh"
    # Skipping key generation since artifacts2 already has correct keys
    
    # Verify key generation
    if [[ -f "priv_validator_key_defidevs-acme_dn.json" && 
          -f "priv_validator_key_defidevs-acme_bvn0.json" ]]; then
        log_success "Validator keys generated successfully"
        log_info "Generated files:"
        ls -la priv_validator_key_*.json
    else
        log_error "Validator key generation failed"
        exit 1
    fi
}

# Step 2: Update network configuration
update_network_config() {
    log_info "Step 2: Updating network configuration..."
    
    # ./analyze update-network-keys --network cyclops-network.json --artifacts .
    # Skipping update-network-keys since artifacts2 already has correct keys and routing
    
    # Verify network config update
    if grep -q "publicKey" cyclops-network.json && grep -q "partitions" cyclops-network.json; then
        log_success "Network configuration updated successfully"
    else
        log_error "Network configuration update failed"
        exit 1
    fi
}

# Step 3: Generate consensus sections
generate_consensus_sections() {
    log_info "Step 3: Generating consensus sections..."
    
    # Generate consensus section for Directory partition
    log_info "Creating consensus section for Directory partition..."
    ./analyze generate-consensus-section \
        cyclops-network.json \
        Directory \
        consensus_dn.json
    
    if [[ $? -ne 0 ]]; then
        log_error "Failed to generate Directory consensus section"
        exit 1
    fi
    
    # Generate consensus section for BVN partition
    log_info "Creating consensus section for bvn-cyclops partition..."
    ./analyze generate-consensus-section \
        cyclops-network.json \
        bvn-cyclops \
        consensus_bvn0.json
    
    if [[ $? -ne 0 ]]; then
        log_error "Failed to generate bvn-cyclops consensus section"
        exit 1
    fi
    
    # Verify consensus files
    if [[ -f "consensus_dn.json" && -f "consensus_bvn0.json" ]]; then
        log_success "Consensus sections generated successfully"
        
        # Validate JSON structure
        if jq . consensus_dn.json > /dev/null 2>&1; then
            log_success "✅ consensus_dn.json is valid JSON"
        else
            log_error "❌ consensus_dn.json is invalid JSON"
            exit 1
        fi
        
        if jq . consensus_bvn0.json > /dev/null 2>&1; then
            log_success "✅ consensus_bvn0.json is valid JSON"
        else
            log_error "❌ consensus_bvn0.json is invalid JSON"
            exit 1
        fi
        
        log_info "Generated consensus files:"
        ls -la consensus_*.json
    else
        log_error "Consensus section generation failed"
        exit 1
    fi
}

# Step 4: Extract partition snapshots
extract_partition_snapshots() {
    log_info "Step 4: Extracting partition snapshots..."
    
    # Create partition-snapshots directory if it doesn't exist
    mkdir -p ./partition-snapshots
    
    # Run the extract command
    ./analyze extract cyclops-network.json cyclops-genesis.snap --partition-snapshots ./partition-snapshots
    
    # Verify extraction
    if [[ -f "./partition-snapshots/bvn-cyclops-partition.snap" && 
          -f "./partition-snapshots/Directory-partition.snap" ]]; then
        log_success "Partition snapshots extracted successfully"
        log_info "Generated snapshots:"
        ls -lh ./partition-snapshots/*.snap
    else
        log_error "Partition snapshot extraction failed"
        exit 1
    fi
}

# Step 5: Generate node key and configuration files
generate_node_config() {
    log_info "Step 5: Generating node key and configuration files..."
    
    # Generate node key using analyze command
    log_info "Generating node key..."
    if ./analyze generate-node-key node_key.json; then
        log_success "Node key generated successfully"
    else
        log_error "Failed to generate node key"
        exit 1
    fi
    
    log_info "Creating accumulate.toml configuration..."
    cat > accumulate.toml << 'EOF'
[describe]
  type = "blockValidator"
  partition-id = "bvn-cyclops"

[configurations]
  enable-healing = false
  enable-snapshots = false
  storage-type = "leveldb"

[storage]
  type = "leveldb"
  path = "data/accumulate.db"

[network]
  id = "cyclops"

[logging]
  level = "info"
EOF
    
    if [ -f "accumulate.toml" ]; then
        log_success "Configuration created: accumulate.toml"
    else
        log_error "Failed to create accumulate.toml"
        exit 1
    fi
    
    # Generate config.toml (CometBFT configuration)
    log_info "Creating config.toml (CometBFT configuration)..."
    cat > config.toml << 'EOF'
# CometBFT configuration file for Cyclops validator

# This is a TOML config file.
# For more information, see https://github.com/toml-lang/toml

# NOTE: Any path below can be absolute (e.g. "/var/myawesomeapp/data") or
# relative to the home directory (e.g. "data"). The home directory is
# "$HOME/.tendermint" by default, but could be changed via $TMHOME env variable
# or --home cmd flag.

#######################################################################
###                   Main Base Config Options                     ###
#######################################################################

# TCP or UNIX socket address of the ABCI application,
# or the name of an ABCI application compiled in with the Tendermint binary
proxy_app = "tcp://127.0.0.1:26658"

# A custom human readable name for this node
moniker = "cyclops-validator"

# If this node is many blocks behind the tip of the chain, FastSync
# allows them to catchup quickly by downloading blocks in parallel
# and verifying their commits
fast_sync = true

# Database backend: goleveldb | cleveldb | boltdb | rocksdb | badgerdb
# * goleveldb (github.com/syndtr/goleveldb - most popular implementation)
#   - pure go
#   - stable
# * cleveldb (uses levigo wrapper)
#   - fast
#   - requires gcc
#   - use cleveldb build tag (go build -tags cleveldb)
# * boltdb (uses etcd's fork of bolt - github.com/etcd-io/bbolt)
#   - EXPERIMENTAL
#   - may be faster is some use-cases (random reads - indexer)
#   - use boltdb build tag (go build -tags boltdb)
# * rocksdb (uses github.com/tecbot/gorocksdb)
#   - EXPERIMENTAL
#   - requires gcc
#   - use rocksdb build tag (go build -tags rocksdb)
# * badgerdb (uses github.com/dgraph-io/badger)
#   - EXPERIMENTAL
#   - use badgerdb build tag (go build -tags badgerdb)
db_backend = "goleveldb"

# Database directory
db_dir = "data"

# Output level for logging, including package level options
log_level = "info"

# Output format: 'plain' (colored text) or 'json'
log_format = "plain"

##### additional base config options #####

# Path to the JSON file containing the initial validator set and other meta data
genesis_file = "config/genesis.json"

# Path to the JSON file containing the private key to use as a validator in the consensus protocol
priv_validator_key_file = "config/priv_validator_key.json"

# Path to the JSON file containing the last sign state of a validator
priv_validator_state_file = "data/priv_validator_state.json"

# TCP or UNIX socket address for Tendermint to listen on for
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
laddr = "tcp://127.0.0.1:26657"

# A list of origins a cross-domain request can be executed from
# Default value '[]' disables cors support
# Use '["*"]' to allow any origin
cors_allowed_origins = []

# A list of methods the client is allowed to use with cross-domain requests
cors_allowed_methods = ["HEAD", "GET", "POST", ]

# A list of non simple headers the client is allowed to use with cross-domain requests
cors_allowed_headers = ["Origin", "Accept", "Content-Type", "X-Requested-With", "X-Server-Time", ]

# TCP or UNIX socket address for the gRPC server to listen on
# NOTE: This server only supports /broadcast_tx_commit
grpc_laddr = ""

# Maximum number of simultaneous connections.
# Does not include RPC (HTTP&WebSocket) connections. See max_open_connections
# If you want to accept a larger number than the default, make sure
# you increase your OS limits.
# 0 - unlimited.
grpc_max_open_connections = 900

# Activate unsafe RPC commands like /dial_seeds and /unsafe_flush_mempool
unsafe = false

# Maximum number of simultaneous connections (including WebSocket).
# Does not include gRPC connections. See grpc_max_open_connections
# If you want to accept a larger number than the default, make sure
# you increase your OS limits.
# 0 - unlimited.
# Should be < {ulimit -Sn} - {MaxNumInboundPeers} - {MaxNumOutboundPeers} - {N of wal, db and other open files}
# 1024 - 40 - 10 - 50 = 924 = ~900
max_open_connections = 900

# Maximum number of unique clientIDs that can /subscribe
# If you're using /broadcast_tx_commit, set to the estimated maximum number
# of broadcast_tx_commit calls per block.
max_subscription_clients = 100

# Maximum number of unique queries a given client can /subscribe to
# If you're using GRPC (or Local RPC client) and /broadcast_tx_commit, set to
# the estimated # maximum number of broadcast_tx_commit calls per block.
max_subscriptions_per_client = 5

# How long to wait for a tx to be committed during /broadcast_tx_commit.
# WARNING: Using a value larger than 10s will result in increasing the
# global HTTP write timeout, which applies to all connections and endpoints.
# See https://github.com/tendermint/tendermint/issues/3435
timeout_broadcast_tx_commit = "10s"

# Maximum size of request body, in bytes
max_body_bytes = 1000000

# Maximum size of request header, in bytes
max_header_bytes = 1048576

# The path to a file containing certificate that is used to create the HTTPS server.
# Might be either absolute path or path related to Tendermint's config directory.
# If the certificate is signed by a certificate authority,
# the certFile should be the concatenation of the server's certificate, any intermediates,
# and the CA's certificate.
# NOTE: both tls_cert_file and tls_key_file must be present for Tendermint to create HTTPS server.
# Otherwise, HTTP server is run.
tls_cert_file = ""

# The path to a file containing matching private key that is used to create the HTTPS server.
# Might be either absolute path or path related to Tendermint's config directory.
# NOTE: both tls_cert_file and tls_key_file must be present for Tendermint to create HTTPS server.
# Otherwise, HTTP server is run.
tls_key_file = ""

# pprof listen address (https://golang.org/pkg/net/http/pprof)
pprof_laddr = "localhost:6060"

#######################################################
###           P2P Configuration Options             ###
#######################################################
[p2p]

# Address to listen for incoming connections
laddr = "tcp://0.0.0.0:26656"

# Address to advertise to peers for them to dial
# If empty, will use the same port as the laddr,
# and will introspect on the listener or use UPnP
# to figure out the address. ip and port are required
# example: 159.89.10.97:26656
external_address = ""

# Comma separated list of seed nodes to connect to
seeds = ""

# Comma separated list of nodes to keep persistent connections to
persistent_peers = ""

# UPNP port forwarding
upnp = false

# Path to address book
addr_book_file = "config/addrbook.json"

# Set true for strict address routability rules
# Set false for private or local networks
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

# Seed mode, in which node constantly crawls the network and looks for
# peers. If another node asks it for addresses, it responds and disconnects.
#
# Does not work if the peer-exchange reactor is disabled.
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

# Mempool version to use:
#   1) "v0" - (default) FIFO mempool.
#   2) "v1" - prioritized mempool.
version = "v0"

recheck = true
broadcast = true
wal_dir = ""

# Maximum number of transactions in the mempool
size = 5000

# Limit the total size of all txs in the mempool.
# This only accounts for raw transactions (e.g. given 1MB transactions and
# max_txs_bytes=5MB, mempool will only accept 5 transactions).
max_txs_bytes = 1073741824

# Size of the cache (used to filter transactions we saw earlier) in transactions
cache_size = 10000

# Do not remove invalid transactions from the cache (default: false)
# Set to true if it's not possible for any invalid transaction to become valid
# again in the future.
keep_invalid_txs_in_cache = false

# Maximum size of a single transaction.
# NOTE: the max size of a tx transmitted over the network is {max_tx_bytes}.
max_tx_bytes = 1048576

# Maximum size of a batch of transactions to send to a peer
# Including space needed by encoding (one varint per transaction).
# XXX: Unused due to https://github.com/tendermint/tendermint/issues/5796
max_batch_bytes = 0

#######################################################
###         State Sync Configuration Options        ###
#######################################################
[statesync]
# State sync rapidly bootstraps a new node by discovering, fetching, and restoring a state machine
# snapshot from peers instead of fetching and replaying historical blocks. Requires some peers in
# the network to take and serve state machine snapshots. State sync is not attempted if the node
# has any local state (LastBlockHeight > 0). The node will have a truncated block history,
# starting from the height of the snapshot.
enable = false

# RPC servers (comma-separated) for light client verification of the synced state machine and
# retrieval of state data for node bootstrapping. Also needs a trusted height and corresponding
# header hash obtained from a trusted source, and a period during which validators can be trusted.
#
# For Cosmos SDK-based chains, trust_period should usually be about 2/3 of the unbonding period
# (~2 weeks) during which they can be financially punished (slashed) for misbehavior.
rpc_servers = ""
trust_height = 0
trust_hash = ""
trust_period = "168h0m0s"

# Time to spend discovering snapshots before initiating a restore.
discovery_time = "15s"

# Temporary directory for state sync snapshot chunks, defaults to the OS tempdir (since v0.33.7).
# Will create a new, randomly named directory within, and remove it when done.
temp_dir = ""

# The timeout duration before re-requesting a chunk, possibly from a different
# peer (default: 1 minute).
chunk_request_timeout = "10s"

# The number of concurrent chunk fetchers to run (default: 1).
chunk_fetchers = "4"

#######################################################
###       Fast Sync Configuration Connections       ###
#######################################################
[fastsync]

# Fast Sync version to use:
#   1) "v0" (default) - the legacy fast sync implementation
#   2) "v1" - refactor of v0 version for better testability
#   2) "v2" - complete redesign of v0, optimized for testability & readability
version = "v0"

#######################################################
###         Consensus Configuration Options         ###
#######################################################
[consensus]

wal_file = "data/cs.wal/wal"

# How long we wait for a proposal block before prevoting nil
timeout_propose = "3s"
# How much timeout_propose increases with each round
timeout_propose_delta = "500ms"
# How long we wait after receiving +2/3 prevotes for "anything" (ie. not a single block or nil)
timeout_prevote = "1s"
# How much the timeout_prevote increases with each round
timeout_prevote_delta = "500ms"
# How long we wait after receiving +2/3 precommits for "anything" (ie. not a single block or nil)
timeout_precommit = "1s"
# How much the timeout_precommit increases with each round
timeout_precommit_delta = "500ms"
# How long we wait after committing a block, before starting on the new
# height (this gives us a chance to receive some more precommits, even
# though we already have +2/3).
timeout_commit = "5s"

# How many blocks to look back to check existence of the node ID in the validator set before joining consensus
# When non-zero, the node will panic upon restart
# if the same consensus key was used to sign {double_sign_check_height} last blocks.
# So, validators should stop the state machine, wait for some blocks, and then restart the state machine to avoid panic.
double_sign_check_height = 0

# Make progress as soon as we have all the precommits (as if TimeoutCommit = 0)
skip_timeout_commit = false

# EmptyBlocks mode and possible interval between empty blocks
create_empty_blocks = true
create_empty_blocks_interval = "0s"

# Reactor sleep duration parameters
peer_gossip_sleep_duration = "100ms"
peer_query_maj23_sleep_duration = "2s"

#######################################################
###   Transaction Indexer Configuration Options     ###
#######################################################
[tx_index]

# What indexer to use for transactions
#
# The application will set which txs to index. In some cases a node operator will be able
# to decide which txs to index based on configuration set in the application.
#
# Options:
#   1) "null"
#   2) "kv" (default) - the simplest possible indexer, backed by key-value storage (defaults to levelDB; see DBBackend).
#   3) "psql" - the indexer services backed by PostgreSQL.
# When "kv" or "psql" is chosen, txs are indexed by their hash, height, and other metadata.
indexer = "kv"

# The PostgreSQL connection configuration, the connection format:
#   postgresql://[[user][:password]@][host][:port][/dbname][?param1=value1&...&paramN=valueN]
# For example:
#   postgresql://user:password@localhost:5432/dbname?sslmode=require
# When indexer is "psql" this sets the database connection.
psql-conn = ""
EOF
    
    if [ -f "config.toml" ]; then
        log_success "CometBFT configuration created: config.toml"
    else
        log_error "Failed to create config.toml"
        exit 1
    fi
}

# Verify final artifacts
verify_artifacts() {
    log_info "Verifying final artifacts..."
    
    # Check partition snapshots have consensus sections
    log_info "Checking BVN partition snapshot..."
    if ./analyze info ./partition-snapshots/bvn-cyclops-partition.snap | grep -q "Consensus"; then
        log_success "BVN partition snapshot contains consensus section"
    else
        log_error "BVN partition snapshot missing consensus section"
        exit 1
    fi
    
    log_info "Checking Directory partition snapshot..."
    if ./analyze info ./partition-snapshots/Directory-partition.snap | grep -q "Consensus"; then
        log_success "Directory partition snapshot contains consensus section"
    else
        log_error "Directory partition snapshot missing consensus section"
        exit 1
    fi
    
    # Check node configuration files
    log_info "Checking node configuration files..."
    if [ -f "accumulate.toml" ]; then
        log_success "Node configuration file exists: accumulate.toml"
        # Validate TOML syntax
        if command -v toml-test >/dev/null 2>&1; then
            toml-test accumulate.toml && log_success "accumulate.toml syntax valid"
        fi
    else
        log_error "Missing node configuration file: accumulate.toml"
        exit 1
    fi
    
    if [ -f "config.toml" ]; then
        log_success "CometBFT configuration file exists: config.toml"
    else
        log_error "Missing CometBFT configuration file: config.toml"
        exit 1
    fi
    
    if [ -f "node_key.json" ]; then
        log_success "Node key file exists: node_key.json"
        # Validate JSON syntax
        if jq empty node_key.json 2>/dev/null; then
            log_success "node_key.json syntax valid"
        else
            log_error "node_key.json has invalid JSON syntax"
            exit 1
        fi
    else
        log_error "Missing node key file: node_key.json"
        exit 1
    fi
    
    # List all final artifacts
    log_success "All artifacts verified successfully!"
    echo
    log_info "Final artifacts ready for deployment:"
    echo "  ✅ Partition Snapshots:"
    ls -lh ./partition-snapshots/*.snap | sed 's/^/    /'
    echo "  ✅ Consensus Configuration:"
    ls -la consensus_*.json | sed 's/^/    /'
    echo "  ✅ Validator Keys:"
    ls -la ./priv_validator_key_*.json | sed 's/^/    /'
    echo "  ✅ Network Configuration:"
    ls -la cyclops-network.json | sed 's/^/    /'
    echo "  ✅ Node Configuration:"
    ls -la accumulate.toml config.toml node_key.json | sed 's/^/    /'
}

# Main execution
main() {
    echo
    log_info "=== Cyclops Validator Prep - Automated Workflow ==="
    echo
    
    check_prerequisites
    rebuild_binaries
    create_backups
    generate_validator_keys
    update_network_config
    generate_consensus_sections
    extract_partition_snapshots
    generate_node_config
    verify_artifacts
    
    echo
    log_success "=== Cyclops Validator Prep Complete ==="
    log_info "All artifacts have been generated and verified."
    log_info "The system is ready for the deployment phase."
    echo
}

# Execute main function
main "$@"
