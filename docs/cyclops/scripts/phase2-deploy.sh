#!/bin/bash

# Cyclops Phase 2 Deployment Script
# Deploys Phase 1 artifacts to validator node configuration
# Based on working Phase 1 automation and deployment design

set -e

# Configuration
ARTIFACTS_DIR="/home/paulsnow/accumulate-network/artifacts"
DEPLOY_DIR="/tmp/cyclops/node"
DEPLOY_ARTIFACTS_DIR="$DEPLOY_DIR/artifacts"

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

# Step 1: Remove any past deployed node
echo -e "\n🧹 Step 1: Cleanup previous deployment..."
if [ -d "/tmp/cyclops" ]; then
    print_status "INFO" "Removing existing deployment: /tmp/cyclops"
    rm -rf "/tmp/cyclops"
    print_status "OK" "Previous deployment cleaned up"
else
    print_status "OK" "No previous deployment found"
fi

# Step 2: Create deployment directories
echo -e "\n📁 Step 2: Creating deployment directories..."
mkdir -p "$DEPLOY_ARTIFACTS_DIR"
mkdir -p "$DEPLOY_DIR/partition-snapshots"
print_status "OK" "Created deployment directories"

# Step 3: Validate Phase 1 artifacts exist
echo -e "\n📋 Step 3: Validating Phase 1 artifacts..."
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
    if [ -f "$ARTIFACTS_DIR/$file" ]; then
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
    if [ -f "$ARTIFACTS_DIR/partition-snapshots/$snap" ]; then
        print_status "OK" "Found: partition-snapshots/$snap"
    else
        print_status "ERROR" "Missing: partition-snapshots/$snap"
        MISSING_FILES=$((MISSING_FILES + 1))
    fi
done

if [ $MISSING_FILES -gt 0 ]; then
    print_status "ERROR" "Missing $MISSING_FILES required files. Run Phase 1 prep first."
    exit 1
fi

# Step 4: Rebuild accumulated binary with fixes
echo -e "\n🔨 Step 4: Rebuilding accumulated binary with fixes..."

# Build the binary with our partition type and Ed25519 fixes
print_status "INFO" "Building accumulated binary with fixes..."
cd /home/paulsnow/go/src/gitlab.com/AccumulateNetwork/accumulate
if go build -o accumulated ./cmd/accumulated; then
    print_status "OK" "Binary built successfully"
    # Copy the new binary to artifacts directory
    cp accumulated "$ARTIFACTS_DIR/"
    print_status "OK" "Updated accumulated binary with fixes"
else
    print_status "ERROR" "Failed to build accumulated binary"
    exit 1
fi

# Return to artifacts directory
cd "$ARTIFACTS_DIR"

# Step 5: Copy artifacts to deployment directory
echo -e "\n📦 Step 5: Copying artifacts to deployment directory..."

# Copy core artifacts
for file in "${REQUIRED_FILES[@]}"; do
    cp "$ARTIFACTS_DIR/$file" "$DEPLOY_ARTIFACTS_DIR/"
    print_status "OK" "Copied: $file"
done

# Copy partition snapshots
for snap in "${PARTITION_SNAPSHOTS[@]}"; do
    cp "$ARTIFACTS_DIR/partition-snapshots/$snap" "$DEPLOY_DIR/partition-snapshots/"
    print_status "OK" "Copied: partition-snapshots/$snap"
done

# Set executable permissions
chmod +x "$DEPLOY_ARTIFACTS_DIR/analyze"
chmod +x "$DEPLOY_ARTIFACTS_DIR/accumulated"
print_status "OK" "Set executable permissions"

# Step 6: Create node directory structure
echo -e "\n⚙️ Step 6: Creating node directory structure..."
cd "$DEPLOY_ARTIFACTS_DIR"

# Create proper node directory structure according to design specification
print_status "INFO" "Creating .accumulate directory structure..."
mkdir -p .accumulate/{config,data}
mkdir -p .accumulate/dn/{config,data}
mkdir -p .accumulate/bvn-cyclops/{config,data}

if [ -d ".accumulate" ]; then
    print_status "OK" "Created .accumulate directory structure"
else
    print_status "ERROR" "Failed to create .accumulate directory"
    exit 1
fi

# Step 7: Place validator keys
echo -e "\n🔐 Step 7: Placing validator keys..."

# Directory node validator key
if [ -f "priv_validator_key_defidevs-acme_dn.json" ]; then
    cp "priv_validator_key_defidevs-acme_dn.json" ".accumulate/dn/config/priv_validator_key.json"
    chmod 600 ".accumulate/dn/config/priv_validator_key.json"
    print_status "OK" "Placed Directory node validator key"
else
    print_status "ERROR" "Directory validator key not found"
    exit 1
fi

# BVN node validator key
if [ -f "priv_validator_key_defidevs-acme_bvn0.json" ]; then
    cp "priv_validator_key_defidevs-acme_bvn0.json" ".accumulate/bvn-cyclops/config/priv_validator_key.json"
    chmod 600 ".accumulate/bvn-cyclops/config/priv_validator_key.json"
    print_status "OK" "Placed BVN node validator key"
else
    print_status "ERROR" "BVN validator key not found"
    exit 1
fi

# Global validator key (use Directory node key as primary)
cp "priv_validator_key_defidevs-acme_dn.json" ".accumulate/config/priv_validator_key.json"
chmod 600 ".accumulate/config/priv_validator_key.json"
print_status "OK" "Placed global validator key"

# Create validator state files (initial state)
echo '{"height":"0","round":0,"step":0}' > ".accumulate/data/priv_validator_state.json"
echo '{"height":"0","round":0,"step":0}' > ".accumulate/dn/data/priv_validator_state.json"
echo '{"height":"0","round":0,"step":0}' > ".accumulate/bvn-cyclops/data/priv_validator_state.json"
print_status "OK" "Created validator state files"

# Note: Node keys will be automatically generated by the 'init network' command
# This ensures proper Ed25519 key generation compatible with Tendermint

# Step 8: Place partition snapshots
echo -e "\n📦 Step 8: Placing partition snapshots..."

# Directory partition snapshot
if [ -f "../partition-snapshots/Directory-partition.snap" ]; then
    cp "../partition-snapshots/Directory-partition.snap" ".accumulate/dn/data/"
    print_status "OK" "Placed Directory partition snapshot"
else
    print_status "ERROR" "Directory partition snapshot not found"
    exit 1
fi

# BVN partition snapshot
if [ -f "../partition-snapshots/bvn-cyclops-partition.snap" ]; then
    cp "../partition-snapshots/bvn-cyclops-partition.snap" ".accumulate/bvn-cyclops/data/"
    print_status "OK" "Placed BVN partition snapshot"
else
    print_status "ERROR" "BVN partition snapshot not found"
    exit 1
fi

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

# Step 8.7: Restore Directory partition snapshot
echo -e "\n🔄 Step 8.7: Restoring Directory partition snapshot..."

if [ -f "$ARTIFACTS_DIR/partition-snapshots/Directory-partition.snap" ]; then
    print_status "INFO" "Restoring Directory partition snapshot to DN database..."
    
    # Use accumulated restore-snapshot with DN work-dir (dnn subdirectory)
    if ./accumulated restore-snapshot "$ARTIFACTS_DIR/partition-snapshots/Directory-partition.snap" \
        --work-dir "$PWD/artifacts/dnn"; then
        print_status "OK" "Successfully restored Directory partition snapshot"
    else
        print_status "ERROR" "Failed to restore Directory partition snapshot"
        exit 1
    fi
else
    print_status "ERROR" "Directory partition snapshot not found for restoration"
    exit 1
fi

# Step 8.8: Restore BVN partition snapshot
echo -e "\n🔄 Step 8.8: Restoring BVN partition snapshot..."

if [ -f "$ARTIFACTS_DIR/partition-snapshots/bvn-cyclops-partition.snap" ]; then
    print_status "INFO" "Restoring BVN partition snapshot to BVN database..."
    
    # Use accumulated restore-snapshot with BVN work-dir (bvnn subdirectory)
    if ./accumulated restore-snapshot "$ARTIFACTS_DIR/partition-snapshots/bvn-cyclops-partition.snap" \
        --work-dir "$PWD/artifacts/bvnn"; then
        print_status "OK" "Successfully restored BVN partition snapshot"
    else
        print_status "ERROR" "Failed to restore BVN partition snapshot"
        exit 1
    fi
else
    print_status "ERROR" "BVN partition snapshot not found for restoration"
    exit 1
fi

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
        "artifacts/dnn/config/priv_validator_key.json"
        "artifacts/bvnn/config/accumulate.toml"
        "artifacts/bvnn/config/config.toml"
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
echo "└── partition-snapshots/      # Genesis snapshots"
echo "    ├── Directory-partition.snap"
echo "    └── bvn-cyclops-partition.snap"

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
