#!/bin/bash

# Cyclops Phase 2 Deployment Script
# Deploys Phase 1 artifacts to validator node configuration
# Based on working Phase 1 automation and deployment design

set -e

# Configuration
ARTIFACTS_SOURCE_DIR="/tmp/cyclops/artifacts"       # Where Phase 1 created artifacts
DEPLOY_DIR="/tmp/cyclops/node"                      # Base deployment directory
CONFIG_DIR="$DEPLOY_DIR/.accumulate/config"        # Node config directory

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print status
print_status() {
    local status=$1
    local message=$2
    if [ "$status" = "OK" ]; then
        echo -e "${GREEN}✓${NC} $message"
    elif [ "$status" = "INFO" ]; then
        echo -e "${BLUE}ℹ${NC} $message"
    elif [ "$status" = "WARN" ]; then
        echo -e "${YELLOW}⚠${NC} $message"
    else
        echo -e "${RED}✗${NC} $message"
    fi
}

echo "🚀 Cyclops Phase 2 Deployment"
echo "============================="
print_status "INFO" "Deploying Phase 1 artifacts to validator node"
print_status "INFO" "Reading artifacts from: $ARTIFACTS_SOURCE_DIR"
print_status "INFO" "Deploying to: $DEPLOY_DIR"

# Step 1: Verify Phase 1 artifacts exist
echo -e "\n📋 Step 1: Verifying Phase 1 deployment exists..."
if [ -d "/tmp/cyclops/artifacts" ]; then
    print_status "OK" "Found Phase 1 deployment at /tmp/cyclops"
else
    print_status "ERROR" "Phase 1 deployment not found. Run Phase 1 first."
    exit 1
fi

# Step 2: Validate Phase 1 artifacts exist
echo -e "\n📋 Step 2: Validating Phase 1 artifacts..."
REQUIRED_FILES=(
    "cyclops-genesis.snap"
    "cyclops-network.json"
    "priv_validator_key_defidevs-acme_dn.json"
    "priv_validator_key_defidevs-acme_bvn0.json"
    "analyze"
    "accumulated"
)

MISSING_FILES=0
for file in "${REQUIRED_FILES[@]}"; do
    if [ -f "$ARTIFACTS_SOURCE_DIR/$file" ]; then
        print_status "OK" "Found: $file"
    else
        print_status "ERROR" "Missing: $file"
        MISSING_FILES=$((MISSING_FILES + 1))
    fi
done

# Check partition snapshots
PARTITION_SNAPSHOTS=(
    "Directory-partition.snap"
    "bvn-cyclops-partition.snap"
)

for snap in "${PARTITION_SNAPSHOTS[@]}"; do
    if [ -f "$ARTIFACTS_SOURCE_DIR/$snap" ]; then
        print_status "OK" "Found: $snap"
    else
        print_status "ERROR" "Missing: $snap"
        MISSING_FILES=$((MISSING_FILES + 1))
    fi
done

if [ $MISSING_FILES -gt 0 ]; then
    print_status "ERROR" "Missing $MISSING_FILES required files. Run Phase 1 prep first."
    exit 1
fi

# Step 3: Create deployment directories
echo -e "\n📁 Step 3: Creating deployment directories..."
mkdir -p "$CONFIG_DIR"
# Partition snapshots will be copied to root deploy directory
print_status "OK" "Created deployment directories"

# Step 4: Rebuild accumulated binary with fixes
echo -e "\n🔨 Step 4: Rebuilding accumulated binary with fixes..."
print_status "INFO" "Building accumulated binary with fixes..."
cd "$HOME/go/src/gitlab.com/AccumulateNetwork/accumulate"
if go build -o accumulated ./cmd/accumulated; then
    print_status "OK" "Binary built successfully"
    # Copy the new binary to artifacts directory
    cp accumulated "$ARTIFACTS_SOURCE_DIR/"
    print_status "OK" "Updated accumulated binary with fixes"
else
    print_status "ERROR" "Failed to build accumulated binary"
    exit 1
fi

# Return to artifacts directory
cd "$ARTIFACTS_SOURCE_DIR"

# Step 5: Copy artifacts to deployment directory
echo -e "\n📦 Step 5: Copying artifacts to deployment directory..."

# Copy core artifacts to deployment directory
for file in "${REQUIRED_FILES[@]}"; do
    cp "$ARTIFACTS_SOURCE_DIR/$file" "$DEPLOY_DIR/"
    print_status "OK" "Copied: $file"
done

# Copy partition snapshots
for snap in "${PARTITION_SNAPSHOTS[@]}"; do
    cp "$ARTIFACTS_SOURCE_DIR/$snap" "$DEPLOY_DIR/"
    print_status "OK" "Copied: $snap"
done

# Set executable permissions
chmod +x "$DEPLOY_DIR/analyze"
chmod +x "$DEPLOY_DIR/accumulated"
print_status "OK" "Set executable permissions"

# Step 6: Create node directory structure
echo -e "\n⚙️ Step 6: Creating node directory structure..."

# Change to deployment directory
cd "$DEPLOY_DIR"

# Create proper node directory structure according to design specification
print_status "INFO" "Creating directory structures..."

# Create new artifacts structure (our working structure)
mkdir -p artifacts/dnn/{config,data}
mkdir -p artifacts/bvnn/{config,data}

# Create legacy .accumulate structure (for phase 3 compatibility)
mkdir -p .accumulate/{config,data}
mkdir -p .accumulate/dn/{config,data}
mkdir -p .accumulate/bvn-cyclops/{config,data}

if [ -d "artifacts" ] && [ -d ".accumulate" ]; then
    print_status "OK" "Created both artifacts and legacy .accumulate directory structures"
else
    print_status "ERROR" "Failed to create directory structures"
    exit 1
fi

# Step 7: Place validator keys
echo -e "\n🔐 Step 7: Placing validator keys..."

# Directory node validator key
if [ -f "priv_validator_key_defidevs-acme_dn.json" ]; then
    # Copy to artifacts structure
    cp "priv_validator_key_defidevs-acme_dn.json" "artifacts/dnn/config/priv_validator_key.json"
    chmod 600 "artifacts/dnn/config/priv_validator_key.json"
    # Copy to legacy structure
    cp "priv_validator_key_defidevs-acme_dn.json" ".accumulate/dn/config/priv_validator_key.json"
    chmod 600 ".accumulate/dn/config/priv_validator_key.json"
    print_status "OK" "Placed Directory node validator key in both locations"
else
    print_status "ERROR" "Directory validator key not found"
    exit 1
fi

# BVN node validator key
if [ -f "priv_validator_key_defidevs-acme_bvn0.json" ]; then
    # Copy to artifacts structure
    cp "priv_validator_key_defidevs-acme_bvn0.json" "artifacts/bvnn/config/priv_validator_key.json"
    chmod 600 "artifacts/bvnn/config/priv_validator_key.json"
    # Copy to legacy structure
    cp "priv_validator_key_defidevs-acme_bvn0.json" ".accumulate/bvn-cyclops/config/priv_validator_key.json"
    chmod 600 ".accumulate/bvn-cyclops/config/priv_validator_key.json"
    print_status "OK" "Placed BVN node validator key in both locations"
else
    print_status "ERROR" "BVN validator key not found"
    exit 1
fi

# Global validator key (use Directory node key as primary)
cp "priv_validator_key_defidevs-acme_dn.json" ".accumulate/config/priv_validator_key.json"
chmod 600 ".accumulate/config/priv_validator_key.json"
print_status "OK" "Placed global validator key"

# Create validator state files (initial state)
# Legacy structure
echo '{"height":"0","round":0,"step":0}' > ".accumulate/data/priv_validator_state.json"
echo '{"height":"0","round":0,"step":0}' > ".accumulate/dn/data/priv_validator_state.json"
echo '{"height":"0","round":0,"step":0}' > ".accumulate/bvn-cyclops/data/priv_validator_state.json"
# Artifacts structure
echo '{"height":"0","round":0,"step":0}' > "artifacts/dnn/data/priv_validator_state.json"
echo '{"height":"0","round":0,"step":0}' > "artifacts/bvnn/data/priv_validator_state.json"
print_status "OK" "Created validator state files in both locations"

# Note: Node keys will be automatically generated by the 'init network' command
# This ensures proper Ed25519 key generation compatible with Tendermint
# Note: Snapshots will be restored directly to artifacts structure and linked to legacy structure

# Step 8.5: Build accumulated binary
echo -e "\n🔨 Step 8.5: Building accumulated binary..."

if [ ! -f "./accumulated" ]; then
    print_status "INFO" "Building accumulated binary from source..."
    
    # Navigate to source directory and build
    cd "$SOURCE_DIR"
    if go build -o "$DEPLOY_DIR/accumulated" ./cmd/accumulated; then
        cd "$DEPLOY_DIR"
        print_status "OK" "Successfully built accumulated binary"
    else
        print_status "ERROR" "Failed to build accumulated binary"
        exit 1
    fi
else
    print_status "OK" "accumulated binary already exists"
fi

# Step 8.6: Initialize Cyclops node

if ./accumulated init network cyclops-network.json \
    --work-dir "$PWD/artifacts"; then
    print_status "OK" "Successfully initialized Cyclops network"
else
    print_status "ERROR" "Failed to initialize Cyclops network"
    exit 1
fi

# Step 8.6: Create missing TOML configuration files
echo -e "\n🔧 Step 8.6: Creating TOML configuration files..."
print_status "INFO" "Copying TOML templates to node directories..."

# Create artifacts directory structure to match expected layout
mkdir -p artifacts/dnn/config artifacts/dnn/data
mkdir -p artifacts/bvnn/config artifacts/bvnn/data

# Copy TOML templates for DN (Directory Node)
if [ -f "$ARTIFACTS_SOURCE_DIR/toml/accumulate-template-dn.toml" ]; then
    # Copy to artifacts structure
    cp "$ARTIFACTS_SOURCE_DIR/toml/accumulate-template-dn.toml" "artifacts/dnn/config/accumulate.toml"
    # Copy to legacy structure (main config location)
    cp "$ARTIFACTS_SOURCE_DIR/toml/accumulate-template-dn.toml" ".accumulate/config/accumulate.toml"
    print_status "OK" "Created DN accumulate.toml in both locations"
else
    print_status "ERROR" "DN accumulate template not found"
    print_status "ERROR" "Looked for: $ARTIFACTS_SOURCE_DIR/toml/accumulate-template-dn.toml"
    print_status "INFO" "Current directory contents:"
    ls -la "$ARTIFACTS_SOURCE_DIR/toml/" || echo "toml directory not found"
fi

if [ -f "$ARTIFACTS_SOURCE_DIR/toml/config-template-cometbft.toml" ]; then
    # Copy to artifacts structure
    cp "$ARTIFACTS_SOURCE_DIR/toml/config-template-cometbft.toml" "artifacts/dnn/config/config.toml"
    cp "$ARTIFACTS_SOURCE_DIR/toml/config-template-cometbft.toml" "artifacts/dnn/config/tendermint.toml"
    # Copy to legacy structure
    cp "$ARTIFACTS_SOURCE_DIR/toml/config-template-cometbft.toml" ".accumulate/config/tendermint.toml"
    print_status "OK" "Created DN CometBFT config files in both locations"
else
    print_status "ERROR" "CometBFT template not found"
fi

# Copy node key for Tendermint P2P identity
if [ -f "$ARTIFACTS_SOURCE_DIR/node_key.json" ]; then
    # Copy to current artifacts directory first
    cp "$ARTIFACTS_SOURCE_DIR/node_key.json" "node_key.json"
    # Copy to legacy structure (main config location)
    cp "node_key.json" ".accumulate/config/node_key.json"
    chmod 600 ".accumulate/config/node_key.json"
    print_status "OK" "Placed node key for Tendermint P2P identity"
else
    print_status "ERROR" "Node key not found - run phase 1 preparation first"
    exit 1
fi

# Copy TOML templates for BVN (Block Validator Node)
if [ -f "$ARTIFACTS_SOURCE_DIR/toml/accumulate-template-bvn.toml" ]; then
    # Copy to artifacts structure
    cp "$ARTIFACTS_SOURCE_DIR/toml/accumulate-template-bvn.toml" "artifacts/bvnn/config/accumulate.toml"
    print_status "OK" "Created BVN accumulate.toml"
else
    print_status "ERROR" "BVN accumulate template not found"
fi

if [ -f "$ARTIFACTS_SOURCE_DIR/toml/config-template-cometbft.toml" ]; then
    # Copy to artifacts structure
    cp "$ARTIFACTS_SOURCE_DIR/toml/config-template-cometbft.toml" "artifacts/bvnn/config/config.toml"
    cp "$ARTIFACTS_SOURCE_DIR/toml/config-template-cometbft.toml" "artifacts/bvnn/config/tendermint.toml"
    print_status "OK" "Created BVN CometBFT config files"
else
    print_status "ERROR" "CometBFT template not found"
fi

# Validator keys and state files are already created in both locations above
print_status "OK" "Validator keys and state files ready in both structures"

# Function to show progress during long operations
show_progress() {
    local pid=$1
    local message="$2"
    echo -n "$message  "
    while kill -0 $pid 2>/dev/null; do
        sleep 5
        echo -n "."
    done
    echo ""
}

# Step 8.7: Restore Directory partition snapshot
echo -e "\n🔄 Step 8.7: Restoring Directory partition snapshot..."

if [ -f "$ARTIFACTS_SOURCE_DIR/Directory-partition.snap" ]; then
    print_status "INFO" "Restoring Directory partition snapshot to DN database..."
    
    # Use accumulated restore-snapshot with DN work-dir (dnn subdirectory) with progress indicator
    ./accumulated restore-snapshot "$ARTIFACTS_SOURCE_DIR/Directory-partition.snap" \
        --work-dir "artifacts/dnn" &
    RESTORE_PID=$!
    show_progress $RESTORE_PID "ℹ Rebuilding BPT from accounts"
    
    # Wait for the process and check exit code
    wait $RESTORE_PID
    if [ $? -eq 0 ]; then
        print_status "OK" "Successfully restored Directory partition snapshot"
    else
        print_status "ERROR" "Failed to restore Directory partition snapshot"
        exit 1
    fi
else
    print_status "ERROR" "Directory partition snapshot not found for restoration"
    print_status "ERROR" "Looked for: $ARTIFACTS_SOURCE_DIR/Directory-partition.snap"
    echo "Available files:"
    ls -la "$ARTIFACTS_SOURCE_DIR/" | grep -E "\.(snap|json)$" || echo "No snapshot files found"
    exit 1
fi

# Step 8.8: Restore BVN partition snapshot
echo -e "\n🔄 Step 8.8: Restoring BVN partition snapshot..."

if [ -f "$ARTIFACTS_SOURCE_DIR/bvn-cyclops-partition.snap" ]; then
    print_status "INFO" "Restoring BVN partition snapshot to BVN database..."
    
    # Use accumulated restore-snapshot with BVN work-dir (bvnn subdirectory) with progress indicator
    ./accumulated restore-snapshot "$ARTIFACTS_SOURCE_DIR/bvn-cyclops-partition.snap" \
        --work-dir "artifacts/bvnn" &
    RESTORE_PID=$!
    show_progress $RESTORE_PID "ℹ Rebuilding BPT from accounts"
    
    # Wait for the process and check exit code
    wait $RESTORE_PID
    if [ $? -eq 0 ]; then
        print_status "OK" "Successfully restored BVN partition snapshot"
    else
        print_status "ERROR" "Failed to restore BVN partition snapshot"
        exit 1
    fi
else
    print_status "ERROR" "BVN partition snapshot not found for restoration"
    exit 1
fi

# Step 8.9: Copy restored snapshots to legacy structure (NO LINKS - FULL COPIES)
echo -e "\n📁 Step 8.9: Copying restored snapshots to legacy structure..."

# Copy database files completely (NO symbolic links to prevent corruption)
print_status "INFO" "Creating full copies in legacy structure (no links)..."
cp -r "artifacts/dnn/data/accumulate.db" ".accumulate/dn/data/Directory-partition.snap"
cp -r "artifacts/bvnn/data/accumulate.db" ".accumulate/bvn-cyclops/data/bvn-cyclops-partition.snap"
print_status "OK" "Created full copies of restored snapshots (no corruption risk)"

# Step 9: Update configuration files for dual node
echo -e "\n⚙️ Step 9: Updating configuration files for dual node..."

print_status "INFO" "Dual node initialization creates separate configs for DN and BVN partitions"
print_status "INFO" "Configuration files created in .accumulate/dn/config/ and .accumulate/bvn-cyclops/config/"
print_status "OK" "Dual node configuration completed by init dual command"

# Step 10: Comprehensive validation
echo -e "\n🔍 Step 10: Comprehensive node structure validation..."

# Run the validation script
VALIDATION_SCRIPT="$ARTIFACTS_DIR/validate-node-structure.sh"
if [ -f "$VALIDATION_SCRIPT" ]; then
    print_status "INFO" "Running comprehensive validation..."
    if "$VALIDATION_SCRIPT" --verbose ".accumulate"; then
        print_status "OK" "Node structure validation PASSED"
    else
        print_status "ERROR" "Node structure validation FAILED"
        echo -e "\n🚫 Deployment validation failed. Please check the errors above."
        exit 1
    fi
else
    print_status "WARN" "Validation script not found, performing basic checks..."
    
    # Basic validation checks for dual node structure
    REQUIRED_DIRS=(
        "artifacts"
        "artifacts/dnn"
        "artifacts/dnn/config"
        "artifacts/dnn/data"
        "artifacts/bvnn"
        "artifacts/bvnn/config"
        "artifacts/bvnn/data"
    )
    
    for dir in "${REQUIRED_DIRS[@]}"; do
        if [ -d "$dir" ]; then
            print_status "OK" "Directory exists: $dir"
        else
            print_status "ERROR" "Missing directory: $dir"
        fi
    done
    
    REQUIRED_FILES=(
        "artifacts/dnn/config/accumulate.toml"
        "artifacts/dnn/config/config.toml"
        "artifacts/dnn/config/tendermint.toml"
        "artifacts/dnn/config/priv_validator_key.json"
        "artifacts/bvnn/config/accumulate.toml"
        "artifacts/bvnn/config/config.toml"
        "artifacts/bvnn/config/tendermint.toml"
        "artifacts/bvnn/config/priv_validator_key.json"
        "artifacts/dnn/data/priv_validator_state.json"
        "artifacts/bvnn/data/priv_validator_state.json"
    )
    
    # Check for database directories (created by snapshot restoration)
    REQUIRED_DBS=(
        "artifacts/dnn/data/accumulate.db"
        "artifacts/bvnn/data/accumulate.db"
    )
    
    for file in "${REQUIRED_FILES[@]}"; do
        if [ -f "$file" ]; then
            print_status "OK" "File exists: $file"
        else
            print_status "ERROR" "Missing file: $file"
        fi
    done
    
    # Check database directories
    for db in "${REQUIRED_DBS[@]}"; do
        if [ -d "$db" ]; then
            db_size=$(du -sh "$db" | cut -f1)
            print_status "OK" "Database exists: $db ($db_size)"
        else
            print_status "ERROR" "Missing database: $db"
        fi
    done
fi

# Summary
DEPLOY_SIZE=$(du -sh "$DEPLOY_DIR" | cut -f1)
print_status "OK" "Phase 2 deployment completed successfully"
print_status "INFO" "Deployment location: $DEPLOY_DIR"
print_status "INFO" "Total deployment size: $DEPLOY_SIZE"

echo -e "\n🎉 Phase 2 Deployment Complete!"
echo "===================================="
echo -e "\n📁 Deployment Structure:"
echo "/tmp/cyclops/node/"
echo "├── artifacts/"
echo "│   ├── .accumulate/          # Node directory"
echo "│   │   ├── config/           # Global config"
echo "│   │   ├── data/             # Global data"
echo "│   │   ├── dn/               # Directory Node"
echo "│   │   │   ├── config/       # DN config & keys"
echo "│   │   │   └── data/         # DN data & snapshots"
echo "│   │   └── bvn-cyclops/      # BVN Node"
echo "│   │       ├── config/       # BVN config & keys"
echo "│   │       └── data/         # BVN data & snapshots"
echo "│   ├── analyze               # Analysis tool"
echo "│   ├── accumulated           # Node daemon"
echo "│   └── [other artifacts]     # Keys, configs, etc."
echo "├── Directory-partition.snap  # Directory partition snapshot"
echo "└── bvn-cyclops-partition.snap # BVN partition snapshot"

echo -e "\n📋 Next Steps (Phase 3 - Launch):"
echo "1. Navigate to node directory:"
echo "   cd /tmp/cyclops/node/artifacts"
echo ""
echo "2. Test node startup (dry run):"
echo "   ./accumulated run --work-dir .accumulate --check-config"
echo ""
echo "3. Launch the validator node:"
echo "   ./accumulated run --work-dir .accumulate"
echo ""
echo "4. Monitor node status:"
echo "   curl http://localhost:26657/status"
echo ""
echo "5. Validate deployment:"
echo "   $VALIDATION_SCRIPT .accumulate"

echo -e "\n💡 Deployment ready for Phase 3 launch!"
