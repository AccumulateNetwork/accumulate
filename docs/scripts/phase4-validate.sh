#!/bin/bash

# Cyclops Node Structure Validation Script
# Validates deployed node directory structure, files, and permissions
# Based on the Cyclops Node Directory Design specification

set -e

# Configuration
VERBOSE=false
FIX_PERMISSIONS=false
NODE_DIR=""

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

# Function to print verbose info
print_verbose() {
    if [ "$VERBOSE" = true ]; then
        echo -e "${BLUE}  →${NC} $1"
    fi
}

# Usage function
usage() {
    echo "Usage: $0 [OPTIONS] NODE_DIRECTORY"
    echo ""
    echo "Validates Cyclops node directory structure and configuration"
    echo ""
    echo "Arguments:"
    echo "  NODE_DIRECTORY    Path to the .accumulate directory to validate"
    echo ""
    echo "Options:"
    echo "  --verbose         Enable verbose output"
    echo "  --fix-permissions Automatically fix file permissions"
    echo "  --help           Show this help message"
    echo ""
    echo "Examples:"
    echo "  $0 /tmp/cyclops/node/artifacts/.accumulate"
    echo "  $0 --verbose --fix-permissions ~/.accumulate"
    exit 1
}

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --verbose)
            VERBOSE=true
            shift
            ;;
        --fix-permissions)
            FIX_PERMISSIONS=true
            shift
            ;;
        --help)
            usage
            ;;
        -*)
            echo "Unknown option $1"
            usage
            ;;
        *)
            if [ -z "$NODE_DIR" ]; then
                NODE_DIR="$1"
            else
                echo "Multiple directories specified"
                usage
            fi
            shift
            ;;
    esac
done

# Check if node directory is provided
if [ -z "$NODE_DIR" ]; then
    echo "Error: Node directory not specified"
    usage
fi

# Check if node directory exists
if [ ! -d "$NODE_DIR" ]; then
    print_status "ERROR" "Node directory does not exist: $NODE_DIR"
    exit 1
fi

echo "🔍 Cyclops Node Structure Validation"
echo "===================================="
print_status "INFO" "Validating node directory: $NODE_DIR"
echo ""

# Change to node directory
cd "$NODE_DIR"

# Validation counters
ERRORS=0
WARNINGS=0
CHECKS=0

# Function to increment counters
check_result() {
    local result=$1
    CHECKS=$((CHECKS + 1))
    if [ "$result" = "ERROR" ]; then
        ERRORS=$((ERRORS + 1))
    elif [ "$result" = "WARN" ]; then
        WARNINGS=$((WARNINGS + 1))
    fi
}

# Step 1: Directory Structure Validation
echo "📁 Step 1: Directory Structure Validation"
echo "----------------------------------------"

REQUIRED_DIRS=(
    "."
    "config"
    "data"
    "dn"
    "dn/config"
    "dn/data"
    "bvn-cyclops"
    "bvn-cyclops/config"
    "bvn-cyclops/data"
)

for dir in "${REQUIRED_DIRS[@]}"; do
    if [ -d "$dir" ]; then
        print_status "OK" "Directory exists: $dir"
        print_verbose "$(ls -ld "$dir" | awk '{print $1, $3, $4}')"
        check_result "OK"
    else
        print_status "ERROR" "Missing directory: $dir"
        check_result "ERROR"
    fi
done

echo ""

# Step 2: Required Files Validation
echo "📄 Step 2: Required Files Validation"
echo "-----------------------------------"

REQUIRED_FILES=(
    "config/accumulate.toml"
    "config/tendermint.toml"
    "dn/config/priv_validator_key.json"
    "bvn-cyclops/config/priv_validator_key.json"
    "dn/data/Directory-partition.snap"
    "bvn-cyclops/data/bvn-cyclops-partition.snap"
)

for file in "${REQUIRED_FILES[@]}"; do
    if [ -f "$file" ]; then
        size=$(ls -lh "$file" | awk '{print $5}')
        print_status "OK" "File exists: $file ($size)"
        print_verbose "$(ls -l "$file" | awk '{print $1, $3, $4, $6, $7, $8}')"
        check_result "OK"
    else
        print_status "ERROR" "Missing file: $file"
        check_result "ERROR"
    fi
done

echo ""

# Step 3: File Permissions Validation
echo "🔒 Step 3: File Permissions Validation"
echo "-------------------------------------"

KEY_FILES=(
    "dn/config/priv_validator_key.json"
    "bvn-cyclops/config/priv_validator_key.json"
)

for key in "${KEY_FILES[@]}"; do
    if [ -f "$key" ]; then
        perms=$(stat -c "%a" "$key")
        if [ "$perms" = "600" ]; then
            print_status "OK" "Key file secure: $key (600)"
            check_result "OK"
        else
            if [ "$FIX_PERMISSIONS" = true ]; then
                chmod 600 "$key"
                print_status "OK" "Fixed permissions: $key (600)"
                check_result "OK"
            else
                print_status "ERROR" "Incorrect permissions: $key ($perms, should be 600)"
                check_result "ERROR"
            fi
        fi
    else
        print_status "ERROR" "Key file not found: $key"
        check_result "ERROR"
    fi
done

echo ""

# Step 4: File Size Validation
echo "📏 Step 4: File Size Validation"
echo "------------------------------"

SNAPSHOT_FILES=(
    "dn/data/Directory-partition.snap"
    "bvn-cyclops/data/bvn-cyclops-partition.snap"
)

for snap in "${SNAPSHOT_FILES[@]}"; do
    if [ -f "$snap" ]; then
        size=$(stat -c%s "$snap")
        size_mb=$((size / 1024 / 1024))
        if [ "$size" -gt 100000000 ]; then  # Greater than 100MB
            print_status "OK" "Snapshot size valid: $snap (${size_mb}MB)"
            check_result "OK"
        else
            print_status "WARN" "Snapshot seems small: $snap (${size_mb}MB)"
            check_result "WARN"
        fi
    else
        print_status "ERROR" "Snapshot not found: $snap"
        check_result "ERROR"
    fi
done

echo ""

# Step 5: JSON Structure Validation
echo "🔧 Step 5: JSON Structure Validation"
echo "-----------------------------------"

# Check if jq is available
if command -v jq >/dev/null 2>&1; then
    for key in "${KEY_FILES[@]}"; do
        if [ -f "$key" ]; then
            if jq -e '.priv_key.type == "tendermint/PrivKeyEd25519"' "$key" >/dev/null 2>&1; then
                print_status "OK" "Valid validator key format: $key"
                check_result "OK"
            else
                print_status "ERROR" "Invalid validator key format: $key"
                check_result "ERROR"
            fi
            
            # Check if key has required fields
            if jq -e '.priv_key.value' "$key" >/dev/null 2>&1; then
                print_status "OK" "Key has private key value: $key"
                print_verbose "Key type: $(jq -r '.priv_key.type' "$key")"
                check_result "OK"
            else
                print_status "ERROR" "Key missing private key value: $key"
                check_result "ERROR"
            fi
        fi
    done
else
    print_status "WARN" "jq not available, skipping JSON validation"
    check_result "WARN"
fi

echo ""

# Step 6: Configuration File Validation
echo "⚙️  Step 6: Configuration File Validation"
echo "----------------------------------------"

CONFIG_FILES=(
    "config/accumulate.toml"
    "config/tendermint.toml"
)

for config in "${CONFIG_FILES[@]}"; do
    if [ -f "$config" ]; then
        # Basic syntax check - look for common TOML patterns
        if grep -q "^\[" "$config" && ! grep -q "^[[:space:]]*#.*\[" "$config"; then
            print_status "OK" "Configuration file format: $config"
            print_verbose "Sections: $(grep "^\[" "$config" | tr '\n' ' ')"
            check_result "OK"
        else
            print_status "WARN" "Configuration file may have issues: $config"
            check_result "WARN"
        fi
        
        # Check configuration files
        if [[ "$config" == *"accumulate.toml" ]]; then
            if grep -q "\[network\]" "$config" && grep -q "id.*=.*cyclops" "$config"; then
                print_status "OK" "Accumulate config has network ID: cyclops"
                check_result "OK"
            else
                print_status "WARN" "Accumulate config missing network ID"
                check_result "WARN"
            fi
        elif [[ "$config" == *"tendermint.toml" ]]; then
            if grep -q "\[p2p\]" "$config" && grep -q "laddr.*=.*:26656" "$config"; then
                print_status "OK" "Tendermint config has P2P port: 26656"
                check_result "OK"
            else
                print_status "WARN" "Tendermint config missing P2P port"
                check_result "WARN"
            fi
        fi
    fi
done

echo ""

# Step 7: Directory Permissions Check
echo "🛡️  Step 7: Directory Permissions Check"
echo "--------------------------------------"

for dir in "${REQUIRED_DIRS[@]}"; do
    if [ -d "$dir" ]; then
        perms=$(stat -c "%a" "$dir")
        if [ "$perms" -ge "755" ]; then
            print_status "OK" "Directory permissions: $dir ($perms)"
            check_result "OK"
        else
            print_status "WARN" "Directory permissions may be restrictive: $dir ($perms)"
            check_result "WARN"
        fi
    fi
done

echo ""

# Summary
echo "📊 Validation Summary"
echo "===================="
echo "Total checks performed: $CHECKS"
echo -e "Passed: ${GREEN}$((CHECKS - ERRORS - WARNINGS))${NC}"
echo -e "Warnings: ${YELLOW}$WARNINGS${NC}"
echo -e "Errors: ${RED}$ERRORS${NC}"

echo ""

if [ "$ERRORS" -eq 0 ]; then
    if [ "$WARNINGS" -eq 0 ]; then
        print_status "OK" "🎉 Node structure validation PASSED - Ready for deployment!"
        echo ""
        echo "Next steps:"
        echo "1. Test node startup: ./accumulated run --work-dir $(basename "$NODE_DIR")"
        echo "2. Monitor logs for proper initialization"
        echo "3. Verify both partitions are running correctly"
        exit 0
    else
        print_status "WARN" "⚠️  Node structure validation PASSED with warnings"
        echo ""
        echo "Consider addressing the warnings before deployment."
        exit 0
    fi
else
    print_status "ERROR" "❌ Node structure validation FAILED"
    echo ""
    echo "Please fix the errors before proceeding with deployment."
    if [ "$FIX_PERMISSIONS" = false ]; then
        echo "Tip: Use --fix-permissions to automatically fix permission issues."
    fi
    exit 1
fi
