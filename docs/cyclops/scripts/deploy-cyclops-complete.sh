#!/bin/bash

# Cyclops Complete Deployment Script
# Executes all 3 phases of Cyclops validator deployment
# 
# Usage: ./deploy-cyclops-complete.sh [target-directory]
#
# This script will:
# 1. Copy artifacts to target directory
# 2. Execute Phase 1: Preparation
# 3. Execute Phase 2: Deployment  
# 4. Execute Phase 3: Launch
#
# Created: 2025-07-07
# Status: Production Ready

set -e

# Color codes for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
ARTIFACTS_SOURCE="/home/paulsnow/accumulate-network/artifacts2"
DEFAULT_TARGET="/tmp/cyclops-deployment-$(date +%Y%m%d-%H%M%S)"

# Functions
print_header() {
    echo -e "${CYAN}================================${NC}"
    echo -e "${CYAN}$1${NC}"
    echo -e "${CYAN}================================${NC}"
}

print_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Parse arguments
TARGET_DIR="${1:-$DEFAULT_TARGET}"

print_header "CYCLOPS COMPLETE DEPLOYMENT"
echo -e "Source: ${ARTIFACTS_SOURCE}"
echo -e "Target: ${TARGET_DIR}"
echo -e "Time: $(date)"
echo ""

# Validate source directory
if [ ! -d "$ARTIFACTS_SOURCE" ]; then
    print_error "Source artifacts directory not found: $ARTIFACTS_SOURCE"
    exit 1
fi

# Check required files
REQUIRED_FILES=(
    "cyclops-network.json"
    "Directory-partition.snap"
    "bvn-cyclops-partition.snap"
    "priv_validator_key_defidevs-acme_dn.json"
    "priv_validator_key_defidevs-acme_bvn0.json"
    "node_key.json"
    "accumulated"
    "analyze"
    "phase1-prep.sh"
    "phase2-deploy.sh"
    "phase3-launch.sh"
)

print_step "Validating source artifacts..."
for file in "${REQUIRED_FILES[@]}"; do
    if [ ! -f "$ARTIFACTS_SOURCE/$file" ]; then
        print_error "Required file missing: $file"
        exit 1
    fi
done
print_success "All required artifacts present"

# Create target directory
print_step "Creating target directory: $TARGET_DIR"
mkdir -p "$TARGET_DIR"

# Copy artifacts
print_step "Copying artifacts to target directory..."
cp -r "$ARTIFACTS_SOURCE"/* "$TARGET_DIR/"
print_success "Artifacts copied successfully"

# Make scripts executable
print_step "Setting script permissions..."
chmod +x "$TARGET_DIR"/*.sh
chmod +x "$TARGET_DIR"/accumulated
chmod +x "$TARGET_DIR"/analyze
print_success "Permissions set"

# Change to target directory
cd "$TARGET_DIR"

# Phase 1: Preparation
print_header "PHASE 1: PREPARATION"
print_step "Executing phase1-prep.sh..."
if ./phase1-prep.sh; then
    print_success "Phase 1 completed successfully"
else
    print_error "Phase 1 failed"
    exit 1
fi

# Phase 2: Deployment
print_header "PHASE 2: DEPLOYMENT"
print_step "Executing phase2-deploy.sh..."
if ./phase2-deploy.sh; then
    print_success "Phase 2 completed successfully"
else
    print_error "Phase 2 failed"
    exit 1
fi

# Phase 3: Launch
print_header "PHASE 3: LAUNCH"
print_step "Executing phase3-launch.sh..."
if ./phase3-launch.sh; then
    print_success "Phase 3 completed successfully"
else
    print_error "Phase 3 failed"
    exit 1
fi

# Final status
print_header "DEPLOYMENT COMPLETE"
print_success "Cyclops validator node deployed and launched successfully!"
echo ""
echo -e "${PURPLE}Deployment Directory:${NC} $TARGET_DIR"
echo -e "${PURPLE}Node Directory:${NC} $TARGET_DIR/.accumulate"
echo -e "${PURPLE}Log File:${NC} $TARGET_DIR/cyclops-node.log"
echo -e "${PURPLE}PID File:${NC} $TARGET_DIR/cyclops-node.pid"
echo ""

# Display operational commands
print_header "OPERATIONAL COMMANDS"
echo -e "${CYAN}Status Check:${NC}"
echo "  curl http://localhost:26657/status | jq"
echo ""
echo -e "${CYAN}View Logs:${NC}"
echo "  tail -f $TARGET_DIR/cyclops-node.log"
echo ""
echo -e "${CYAN}Stop Node:${NC}"
echo "  kill \$(cat $TARGET_DIR/cyclops-node.pid)"
echo ""
echo -e "${CYAN}Restart Node:${NC}"
echo "  cd $TARGET_DIR && ./phase3-launch.sh"
echo ""
echo -e "${CYAN}Validate Structure:${NC}"
echo "  cd $TARGET_DIR && ./phase4-validate.sh .accumulate"
echo ""

print_success "Cyclops validator is now running and ready for operation!"
