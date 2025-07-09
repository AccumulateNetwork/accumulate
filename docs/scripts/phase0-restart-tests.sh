#!/bin/bash

# Phase 0: Environment Setup for Cyclops Development Deployment
# Removes test directory, recreates it, and copies artifacts
# Part of the 4-phase development deployment plan

set -euo pipefail

# Color codes for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
TEST_ENV_DIR="/tmp/cyclops"
ARTIFACTS_SOURCE_DIR="$HOME/accumulate-network/artifacts2"
ARTIFACTS_TARGET_DIR="$TEST_ENV_DIR/artifacts"

# Logging functions
log_info() {
    echo -e "${BLUE}ℹ️  $1${NC}"
}

log_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

# Main execution
main() {
    echo "🧪 Phase 0: Environment Setup for Cyclops Development Deployment"
    echo "================================================================"
    echo "Test Environment: $TEST_ENV_DIR"
    echo "Source Artifacts: $ARTIFACTS_SOURCE_DIR"
    echo ""
    
    # Remove existing test environment
    if [ -d "$TEST_ENV_DIR" ]; then
        log_info "Removing existing test environment: $TEST_ENV_DIR"
        rm -rf "$TEST_ENV_DIR"
        log_success "Test environment cleaned up"
    else
        log_info "No existing test environment found"
    fi
    
    # Create directory structure
    log_info "Creating directory structure"
    mkdir -p "$ARTIFACTS_TARGET_DIR"
    log_success "Created directory: $ARTIFACTS_TARGET_DIR"
    
    # Copy artifacts by name
    local artifacts_dest="$ARTIFACTS_TARGET_DIR"
    local files_to_copy=(
        "config.toml"
        "accumulate.toml"
        "tendermint.toml"
        "cyclops-network.json"
        "node_key.json"
        "priv_validator_key.json"
        "accumulated"
        "analyze"
        "cyclops-genesis.snap"
        "Directory-partition.snap"
        "bvn-cyclops-partition.snap"
        "Directory.toml"
        "bvn-cyclops.toml"
    )
    
    log_info "Copying artifacts..."
    for file in "${files_to_copy[@]}"; do
        if [ -f "$ARTIFACTS_SOURCE_DIR/$file" ]; then
            cp "$ARTIFACTS_SOURCE_DIR/$file" "$artifacts_dest/"
            log_success "Copied: $file"
        else
            log_info "Skipped (not found): $file"
        fi
    done
        
    # Add default API listen address if missing
    if ! grep -q listen-address "$artifacts_dest/accumulate.toml"; then
        printf "\n[api]\nlisten-address = \"tcp://0.0.0.0:9900\"\n" >> "$artifacts_dest/accumulate.toml"
    fi

    # Set secure permissions on all key files
    log_info "Setting secure permissions on key files"
    for key_file in "$artifacts_dest/priv_validator_key_"*.json; do
        if [ -f "$key_file" ]; then
            chmod 600 "$key_file"
            log_success "Set 600 permissions: $(basename "$key_file")"
        fi
    done
    
    # Set permissions on node key
    if [ -f "$artifacts_dest/node_key.json" ]; then
        chmod 600 "$artifacts_dest/node_key.json"
        log_success "Set 600 permissions: node_key.json"
    fi
    
    # Set executable permissions on binaries
    for binary in "accumulated" "analyze"; do
        if [ -f "$artifacts_dest/$binary" ]; then
            chmod +x "$artifacts_dest/$binary"
            log_success "Set executable permissions: $binary"
        fi
    done
    
    echo ""
    log_success "Phase 0 Environment Setup Complete!"
    log_info "Test environment ready at: $TEST_ENV_DIR"
}

# Execute main function
main "$@"
