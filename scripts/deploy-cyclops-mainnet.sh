#!/bin/bash

# Cyclops Mainnet Deployment Script
# Deploys Accumulate Cyclops network with proper key generation and partition extraction

set -e  # Exit on any error

# Configuration
REPO_ROOT="/home/paul/go/src/gitlab.com/AccumulateNetwork/accumulate"
WORK_DIR="${WORK_DIR:-/home/paul/accumulate-mainnet}"
CYCLOPS_SNAPSHOT="${CYCLOPS_SNAPSHOT:-/home/paul/cyclops-genesis.snap}"
NETWORK_JSON="${NETWORK_JSON:-/home/paul/cyclops-network.json}"

# Derived paths
ARTIFACTS_DIR="$WORK_DIR/artifacts"
NODES_DIR="$WORK_DIR/nodes"
KEYS_DIR="$ARTIFACTS_DIR/keys"
ACCUMULATED_BIN="$REPO_ROOT/accumulated"
ANALYZE_BIN="$REPO_ROOT/analyze"
DEBUG_BIN="$REPO_ROOT/debug"
SNAPSHOT_BIN="$REPO_ROOT/snapshot"

# Colors for output
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

# Command line options
STEP="all"
SKIP_BUILD=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --step)
            STEP="$2"
            shift 2
            ;;
        --skip-build)
            SKIP_BUILD=true
            shift
            ;;
        --work-dir)
            WORK_DIR="$2"
            NODES_DIR="$WORK_DIR/nodes"
            ARTIFACTS_DIR="$WORK_DIR/artifacts"
            KEYS_DIR="$ARTIFACTS_DIR/keys"
            shift 2
            ;;
        --cyclops-snapshot)
            CYCLOPS_SNAPSHOT="$2"
            shift 2
            ;;
        --network-json)
            NETWORK_JSON="$2"
            shift 2
            ;;
        --help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --step STEP           Run specific step: setup|keys|extract|init|deploy|start|all"
            echo "  --skip-build          Skip building binaries"
            echo "  --work-dir DIR        Set working directory (default: /home/paul/accumulate-mainnet)"
            echo "  --cyclops-snapshot    Path to cyclops-genesis.snap"
            echo "  --network-json        Path to cyclops-network.json"
            echo "  --help                Show this help"
            echo ""
            echo "Steps:"
            echo "  setup    - Create directories and copy artifacts"
            echo "  keys     - Generate validator keys and update network config"
            echo "  extract  - Extract partition snapshots"
            echo "  init     - Initialize nodes"
            echo "  deploy   - Organize deployment structure"
            echo "  start    - Start the network"
            echo "  all      - Run all steps"
            exit 0
            ;;
        *)
            log_error "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Validate source files
validate_sources() {
    log_info "Validating source files..."
    
    if [[ ! -f "$CYCLOPS_SNAPSHOT" ]]; then
        log_error "Cyclops snapshot not found: $CYCLOPS_SNAPSHOT"
        exit 1
    fi
    
    if [[ ! -f "$NETWORK_JSON" ]]; then
        log_error "Network JSON not found: $NETWORK_JSON"
        exit 1
    fi
    
    log_success "Source files validated"
}

# Build binaries
build_binaries() {
    log_info "Building required binaries..."
    
    cd "$REPO_ROOT"
    
    # Build accumulated (node daemon)
    log_info "Building accumulated..."
    go build -o "$ACCUMULATED_BIN" ./cmd/accumulated
    if [[ $? -eq 0 ]]; then
        log_success "Built accumulated: $ACCUMULATED_BIN"
    else
        log_error "Failed to build accumulated"
        exit 1
    fi
    
    # Build analyze tool
    log_info "Building analyze tool..."
    go build -o "$ANALYZE_BIN" ./tools/cmd/analyze
    if [[ $? -eq 0 ]]; then
        log_success "Built analyze: $ANALYZE_BIN"
    else
        log_error "Failed to build analyze tool"
        exit 1
    fi
    
    # Build debug tool (for snap collect)
    log_info "Building debug tool..."
    go build -o "$DEBUG_BIN" ./tools/cmd/debug
    if [[ $? -eq 0 ]]; then
        log_success "Built debug: $DEBUG_BIN"
    else
        log_error "Failed to build debug tool"
        exit 1
    fi
    
    # Build snapshot tool (for concat)
    log_info "Building snapshot tool..."
    go build -o "$SNAPSHOT_BIN" ./tools/cmd/snapshot
    if [[ $? -eq 0 ]]; then
        log_success "Built snapshot: $SNAPSHOT_BIN"
    else
        log_error "Failed to build snapshot tool"
        exit 1
    fi
}

# Step 1: Setup directories and copy artifacts
setup_directories() {
    log_info "Setting up directories and copying artifacts..."
    
    # Create directory structure
    mkdir -p "$NODES_DIR"
    mkdir -p "$ARTIFACTS_DIR"
    mkdir -p "$KEYS_DIR"
    
    # Copy source artifacts
    log_info "Copying cyclops snapshot..."
    cp "$CYCLOPS_SNAPSHOT" "$ARTIFACTS_DIR/cyclops-genesis.snap"
    
    log_info "Copying network configuration..."
    cp "$NETWORK_JSON" "$ARTIFACTS_DIR/cyclops-network.json"
    
    log_success "Directories created and artifacts copied"
}

# Step 2: Generate validator keys and update network config
generate_keys() {
    log_info "Generating validator keys and updating network config..."
    
    # Create keys directory
    mkdir -p "$KEYS_DIR"
    
    # Parse validators from network JSON
    local validators
    validators=$(jq -r '.globals.network.validators[].operator' "$ARTIFACTS_DIR/cyclops-network.json" 2>/dev/null)
    
    if [[ -z "$validators" ]]; then
        log_error "No validators found in network configuration"
        exit 1
    fi
    
    log_info "Found validators: $(echo "$validators" | tr '\n' ' ')"
    
    # Generate partition-specific keys for each validator
    local validator_count=0
    while IFS= read -r validator; do
        if [[ -n "$validator" ]]; then
            validator_count=$((validator_count + 1))
            local validator_key_dir="$KEYS_DIR/validator-$validator_count"
            mkdir -p "$validator_key_dir"
            
            log_info "Generating partition-specific keys for validator $validator_count: $validator"
            
            # Generate key for Directory Network (DN) partition
            log_info "Generating DN key for $validator"
            if ! "$ANALYZE_BIN" generate-key "$validator" "$validator_key_dir" --partition "Directory"; then
                log_error "Failed to generate DN key for $validator"
                exit 1
            fi
            
            # Rename to partition-specific filename
            if [[ -f "$validator_key_dir/priv_validator_key.json" ]]; then
                mv "$validator_key_dir/priv_validator_key.json" "$validator_key_dir/priv_validator_key_dn.json"
                log_success "Generated DN key: priv_validator_key_dn.json"
            fi
            
            # Generate key for Block Validator Network (BVN) partition  
            log_info "Generating BVN key for $validator"
            if ! "$ANALYZE_BIN" generate-key "$validator" "$validator_key_dir" --partition "bvn-cyclops"; then
                log_error "Failed to generate BVN key for $validator"
                exit 1
            fi
            
            # Rename to partition-specific filename
            if [[ -f "$validator_key_dir/priv_validator_key.json" ]]; then
                mv "$validator_key_dir/priv_validator_key.json" "$validator_key_dir/priv_validator_key_bvn.json"
                log_success "Generated BVN key: priv_validator_key_bvn.json"
            fi
            
            log_success "Generated partition-specific keys for $validator"
        fi
    done <<< "$validators"
    
    if [[ $validator_count -eq 0 ]]; then
        log_error "No validator keys were generated"
        exit 1
    fi
    
    log_success "Generated $validator_count validator keys"
    
    # Update network configuration with partition-specific public keys
    log_info "Updating network configuration with partition-specific public keys..."
    
    validator_count=0
    while IFS= read -r validator; do
        if [[ -n "$validator" ]]; then
            validator_count=$((validator_count + 1))
            local validator_key_dir="$KEYS_DIR/validator-$validator_count"
            
            log_info "Updating network config for $validator with partition keys"
            
            # Update network config with DN partition key
            if [[ -f "$validator_key_dir/priv_validator_key_dn.json" ]]; then
                log_info "Updating DN partition key for $validator"
                if ! "$ANALYZE_BIN" update "$validator" "$ARTIFACTS_DIR/cyclops-network.json" "$validator_key_dir/priv_validator_key_dn.json" --partition "Directory"; then
                    log_error "Failed to update DN partition key for $validator"
                    exit 1
                fi
                log_success "Updated DN partition key for $validator"
            fi
            
            # Update network config with BVN partition key
            if [[ -f "$validator_key_dir/priv_validator_key_bvn.json" ]]; then
                log_info "Updating BVN partition key for $validator"
                if ! "$ANALYZE_BIN" update "$validator" "$ARTIFACTS_DIR/cyclops-network.json" "$validator_key_dir/priv_validator_key_bvn.json" --partition "bvn-cyclops"; then
                    log_error "Failed to update BVN partition key for $validator"
                    exit 1
                fi
                log_success "Updated BVN partition key for $validator"
            fi
            
            log_success "Updated network config for $validator with both partition keys"
        fi
    done <<< "$validators"
    
    log_success "Network configuration updated with all public keys"
    
    # Transform network JSON for accumulated compatibility (CRITICAL: Do this after key updates)
    local transformed_json="$ARTIFACTS_DIR/cyclops-network-transformed.json"
    log_info "Transforming network JSON structure for accumulated compatibility..."
    transform_network_json "$ARTIFACTS_DIR/cyclops-network.json" "$transformed_json"
    
    # Replace original with transformed version to ensure consistency throughout pipeline
    log_info "Using transformed JSON as primary network configuration..."
    mv "$ARTIFACTS_DIR/cyclops-network.json" "$ARTIFACTS_DIR/cyclops-network-original.json"
    mv "$transformed_json" "$ARTIFACTS_DIR/cyclops-network.json"
    
    log_success "Network JSON transformed and set as primary configuration"
}

# Step 3: Build consensus sections for each partition
build_consensus_sections() {
    log_info "Building CometBFT-compatible consensus sections for each partition..."
    
    # Validate that we have the transformed network JSON with keys
    if [[ ! -f "$ARTIFACTS_DIR/cyclops-network.json" ]]; then
        log_error "Network JSON not found: $ARTIFACTS_DIR/cyclops-network.json"
        exit 1
    fi
    
    # Verify JSON structure is transformed and has validator keys
    if ! jq -e '.network.validators[0].publicKey' "$ARTIFACTS_DIR/cyclops-network.json" >/dev/null 2>&1; then
        log_error "Network JSON missing validator public keys - ensure generate_keys step completed successfully"
        exit 1
    fi
    
    log_info "Generating partition-specific consensus sections..."
    cd "$ARTIFACTS_DIR"
    
    # Extract partition list from network configuration
    local partitions
    partitions=$(jq -r '.network.partitions[].id' cyclops-network.json)
    
    if [[ -z "$partitions" ]]; then
        log_error "No partitions found in network configuration"
        exit 1
    fi
    
    log_info "Found partitions: $(echo $partitions | tr '\n' ' ')"
    
    # Generate consensus sections for each partition
    for partition in $partitions; do
        log_info "Generating consensus section for partition: $partition"
        
        # Create consensus section JSON file for this partition
        local consensus_file="consensus-${partition}.json"
        
        # Use analyze tool to generate consensus section
        # This creates a standalone consensus JSON that can be embedded in snapshots
        if "$ANALYZE_BIN" generate-consensus-section \
            --network-config cyclops-network.json \
            --partition "$partition" \
            --output "$consensus_file"; then
            
            log_success "Generated consensus section: $consensus_file"
            
            # Validate the generated consensus section
            if jq -e '.chainId and .validators and (.validators | length > 0)' "$consensus_file" >/dev/null 2>&1; then
                local validator_count
                validator_count=$(jq '.validators | length' "$consensus_file")
                log_info "Consensus section for $partition contains $validator_count validators"
            else
                log_error "Generated consensus section for $partition is invalid"
                exit 1
            fi
        else
            log_error "Failed to generate consensus section for partition: $partition"
            exit 1
        fi
    done
    
    # Update network JSON to include consensus sections
    log_info "Embedding consensus sections into network configuration..."
    
    # Create updated network JSON with embedded consensus sections
    local network_with_consensus="cyclops-network-with-consensus.json"
    
    # Start with the current network JSON
    cp cyclops-network.json "$network_with_consensus"
    
    # Embed each consensus section into the appropriate partition
    for partition in $partitions; do
        local consensus_file="consensus-${partition}.json"
        
        if [[ -f "$consensus_file" ]]; then
            log_info "Embedding consensus section for partition: $partition"
            
            # Use jq to embed the consensus section into the partition configuration
            local temp_file="temp-network.json"
            if jq --argjson consensus "$(cat "$consensus_file")" \
                --arg partition "$partition" \
                '(.network.partitions[] | select(.id == $partition)).consensus = $consensus' \
                "$network_with_consensus" > "$temp_file"; then
                
                mv "$temp_file" "$network_with_consensus"
                log_success "Embedded consensus section for $partition"
            else
                log_error "Failed to embed consensus section for partition: $partition"
                exit 1
            fi
        else
            log_error "Consensus file not found: $consensus_file"
            exit 1
        fi
    done
    
    # Replace the network JSON with the version containing consensus sections
    log_info "Updating network configuration with embedded consensus sections..."
    mv cyclops-network.json cyclops-network-no-consensus.json
    mv "$network_with_consensus" cyclops-network.json
    
    # Validate the final network JSON
    if jq -e '.network.partitions[0].consensus.validators' cyclops-network.json >/dev/null 2>&1; then
        log_success "Network configuration updated with consensus sections"
        
        # Report consensus section statistics
        for partition in $partitions; do
            local validator_count
            validator_count=$(jq -r --arg partition "$partition" \
                '(.network.partitions[] | select(.id == $partition)).consensus.validators | length' \
                cyclops-network.json)
            log_info "Partition $partition: $validator_count validators in consensus section"
        done
    else
        log_error "Failed to validate network JSON with consensus sections"
        exit 1
    fi
    
    log_success "Consensus sections built and embedded successfully"
}

# Step 4: Extract partition snapshots
extract_partitions() {
    log_info "Extracting partition snapshots..."
    
    # Validate that we have the transformed network JSON
    if [[ ! -f "$ARTIFACTS_DIR/cyclops-network.json" ]]; then
        log_error "Network JSON not found: $ARTIFACTS_DIR/cyclops-network.json"
        exit 1
    fi
    
    # Verify JSON structure is transformed (has top-level 'network' field)
    if ! jq -e '.network' "$ARTIFACTS_DIR/cyclops-network.json" >/dev/null 2>&1; then
        log_error "Network JSON does not have expected transformed structure (missing top-level 'network' field)"
        exit 1
    fi
    
    log_info "Using transformed network JSON for partition extraction"
    
    # Extract partition snapshots using the transformed network configuration
    cd "$ARTIFACTS_DIR"
    
    log_info "Running partition extraction with transformed JSON..."
    "$ANALYZE_BIN" extract cyclops-network.json cyclops-genesis.snap --partition-snapshots "$ARTIFACTS_DIR"
    
    if [[ $? -eq 0 ]]; then
        log_success "Partition snapshots extracted successfully using transformed JSON"
        
        # List generated partition snapshots
        log_info "Generated partition snapshots:"
        ls -la *-partition.snap 2>/dev/null || log_warning "No partition snapshots found"
    else
        log_error "Failed to extract partition snapshots"
        exit 1
    fi
}

# Function to transform network JSON structure for accumulated binary compatibility
transform_network_json() {
    local input_json="$1"
    local output_json="$2"
    
    log_info "Transforming network JSON structure for accumulated compatibility..."
    
    # Use jq to transform the JSON structure
    # Move globals.network to top-level network field
    if ! jq '{
        network: .globals.network,
        globals: (.globals | del(.network)),
        oracle: .globals.oracle
    }' "$input_json" > "$output_json"; then
        log_error "Failed to transform network JSON structure"
        exit 1
    fi
    
    log_success "Network JSON transformed successfully"
}

# Step 4: Create genesis snapshot and initialize nodes
prepare_genesis_and_init() {
    log_info "Preparing genesis snapshot and initializing nodes..."
    
    # Create genesis snapshot from partition snapshots
    cd "$ARTIFACTS_DIR"
    
    # Find all partition snapshots
    local partition_snapshots
    partition_snapshots=$(find . -name "*-partition.snap" -type f | sort)
    
    if [[ -z "$partition_snapshots" ]]; then
        log_error "No partition snapshots found for genesis creation"
        exit 1
    fi
    
    log_info "Creating genesis snapshot from partition snapshots:"
    echo "$partition_snapshots" | while read -r snap; do
        log_info "  - $snap"
    done
    
    # Create genesis snapshot by concatenating partition snapshots
    log_info "Concatenating partition snapshots into genesis..."
    "$SNAPSHOT_BIN" concat cyclops-genesis.snap $partition_snapshots
    
    if [[ $? -eq 0 ]]; then
        log_success "Genesis snapshot created successfully"
    else
        log_error "Failed to create genesis snapshot"
        exit 1
    fi
    
    # Validate that we have the transformed network JSON (should already be done in generate_keys)
    if [[ ! -f "$ARTIFACTS_DIR/cyclops-network.json" ]]; then
        log_error "Transformed network JSON not found: $ARTIFACTS_DIR/cyclops-network.json"
        exit 1
    fi
    
    # Verify JSON structure is transformed (has top-level 'network' field)
    if ! jq -e '.network' "$ARTIFACTS_DIR/cyclops-network.json" >/dev/null 2>&1; then
        log_error "Network JSON is not in transformed format (missing top-level 'network' field)"
        exit 1
    fi
    
    log_info "Using already transformed network JSON for node initialization"
    
    # Initialize nodes using accumulated with the transformed JSON
    log_info "Initializing nodes with accumulated..."
    mkdir -p "$NODES_DIR"
    
    # Use the already transformed network JSON for node initialization
    "$ACCUMULATED_BIN" init genesis \
        --work-dir "$NODES_DIR" \
        --network "$ARTIFACTS_DIR/cyclops-network.json" \
        --genesis "$ARTIFACTS_DIR/cyclops-genesis.snap" \
        --bvn-count 1 \
        --validator-count 1
    
    if [[ $? -eq 0 ]]; then
        log_success "Nodes initialized successfully with consistent JSON structure"
    else
        log_error "Failed to initialize nodes"
        exit 1
    fi
    
    # Copy validator keys to node directories
    log_info "Copying validator keys to node directories..."
    
    # Find node directories
    local node_dirs
    node_dirs=$(find "$NODES_DIR" -name "bvn*" -type d)
    
    if [[ -z "$node_dirs" ]]; then
        log_error "No BVN node directories found"
        exit 1
    fi
    
    # Copy keys to each node directory
    local validator_count=0
    echo "$node_dirs" | while read -r node_dir; do
        validator_count=$((validator_count + 1))
        local validator_key_dir="$KEYS_DIR/validator-$validator_count"
        
        if [[ -f "$validator_key_dir/priv_validator_key.json" ]]; then
            cp "$validator_key_dir/priv_validator_key.json" "$node_dir/"
            log_success "Copied validator key to $node_dir"
        else
            log_warning "Validator key not found: $validator_key_dir/priv_validator_key.json"
        fi
    done
    
    log_success "Genesis preparation and node initialization completed with consistent JSON structure"
}

# Step 5: Deploy structure - organize files for mainnet deployment
deploy_structure() {
    log_info "Organizing deployment structure..."
    
    # Parse validators from network.json
    local validators
    validators=$(jq -r '.globals.network.validators[].operator' "$ARTIFACTS_DIR/cyclops-network.json")
    
    # Find node directories
    local node_dirs
    node_dirs=$(find "$NODES_DIR" -maxdepth 1 -type d -name "bvn*" | sort)
    
    if [[ -z "$node_dirs" ]]; then
        log_error "No node directories found in $NODES_DIR"
        exit 1
    fi
    
    log_info "Found node directories:"
    echo "$node_dirs" | while read -r node_dir; do
        log_info "  - $(basename "$node_dir")"
    done
    
    # Copy validator keys to node directories
    local validator_count=0
    local node_array=($node_dirs)
    
    echo "$validators" | while read -r validator; do
        if [[ -n "$validator" ]]; then
            validator_count=$((validator_count + 1))
            local validator_key_dir="$KEYS_DIR/validator-$validator_count"
            
            # Use modulo to cycle through available nodes if more validators than nodes
            local node_index=$(( (validator_count - 1) % ${#node_array[@]} ))
            local target_node_dir="${node_array[$node_index]}"
            
            if [[ -n "$target_node_dir" && -d "$target_node_dir" ]]; then
                log_info "Copying validator key for $validator to $(basename "$target_node_dir")"
                
                # Copy the private validator key
                if [[ -f "$validator_key_dir/priv_validator_key.json" ]]; then
                    cp "$validator_key_dir/priv_validator_key.json" "$target_node_dir/"
                    log_success "Copied validator key to $(basename "$target_node_dir")"
                else
                    log_warning "Validator key not found at $validator_key_dir/priv_validator_key.json"
                fi
            fi
        fi
    done
    
    log_success "Deployment structure organized"
}

# Step 6: Start the network
start_network() {
    log_info "Starting Cyclops mainnet..."
    
    # Find node directories
    local node_dirs
    node_dirs=$(find "$NODES_DIR" -maxdepth 1 -type d -name "bvn*" | sort)
    
    if [[ -z "$node_dirs" ]]; then
        log_error "No node directories found"
        exit 1
    fi
    
    # Start each node
    local pids=()
    local node_array=($node_dirs)
    
    for node_dir in "${node_array[@]}"; do
        if [[ -n "$node_dir" ]]; then
            local node_name
            node_name=$(basename "$node_dir")
            
            log_info "Starting node: $node_name"
            
            # Check if accumulate.toml exists
            if [[ ! -f "$node_dir/accumulate.toml" ]]; then
                log_warning "No accumulate.toml found in $node_dir, skipping"
                continue
            fi
            
            # Start the node in background
            "$ACCUMULATED_BIN" run --work-dir "$node_dir" > "$node_dir/node.log" 2>&1 &
            local pid=$!
            pids+=($pid)
            
            log_success "Started node $node_name (PID: $pid)"
            
            # Give node time to start
            sleep 2
        fi
    done
    
    if [[ ${#pids[@]} -eq 0 ]]; then
        log_error "No nodes were started"
        exit 1
    fi
    
    log_success "Started ${#pids[@]} nodes"
    log_info "Node PIDs: ${pids[*]}"
    log_info "Check node logs in respective node directories"
    
    # Wait for user input to stop
    echo ""
    log_info "Press Ctrl+C to stop all nodes"
    
    # Set up signal handler to clean up
    trap 'log_info "Stopping all nodes..."; kill ${pids[*]} 2>/dev/null; exit 0' INT TERM
    
    # Wait indefinitely
    while true; do
        sleep 1
    done
}

# Main execution
main() {
    log_info "Starting Cyclops Mainnet Deployment"
    log_info "Working directory: $WORK_DIR"
    log_info "Step: $STEP"
    
    # Validate sources first
    validate_sources
    
    # Build binaries
    build_binaries
    
    case "$STEP" in
        "setup")
            setup_directories
            ;;
        "keys")
            generate_keys
            ;;
        "consensus")
            build_consensus_sections
            ;;
        "extract")
            extract_partitions
            ;;
        "init")
            prepare_genesis_and_init
            ;;
        "deploy")
            deploy_structure
            ;;
        "start")
            start_network
            ;;
        "all")
            setup_directories
            generate_keys
            build_consensus_sections
            extract_partitions
            prepare_genesis_and_init
            deploy_structure
            start_network
            ;;
        *)
            log_error "Unknown step: $STEP"
            log_info "Valid steps: setup, keys, consensus, extract, init, deploy, start, all"
            exit 1
            ;;
    esac
    
    log_success "Deployment step '$STEP' completed successfully"
}

# Run main function
main "$@"
