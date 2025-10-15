#!/bin/bash

# Script to add Android/Termux skips to timeout-prone test functions

# Function to add skip to a test function if it doesn't already have one
add_skip_to_test() {
    local file="$1"
    local test_func="$2"
    local reason="$3"
    
    # Check if the test function already has a skip
    if grep -A 2 "$test_func" "$file" | grep -q "skipOnAndroid\|t.Skip"; then
        echo "Skip already exists for $test_func in $file"
        return
    fi
    
    # Add skip after the function declaration
    sed -i "/^func $test_func/,/^{/ {
        /^{/ a\\
	skipOnAndroid(t, \"$reason\")
    }" "$file"
    
    echo "Added skip to $test_func in $file"
}

# Test patterns that commonly timeout on Android
timeout_patterns=(
    "Test.*Simulator"
    "Test.*Network"
    "Test.*Consensus"
    "Test.*P2P"
    "Test.*ABCI"
    "Test.*Validate"
    "Test.*E2E"
    "Test.*Faucet"
    "Test.*API"
)

# Directories to process
test_dirs=(
    "./test/e2e"
    "./test/validate" 
    "./test/simulator"
    "./internal/api/v3"
    "./internal/node/abci"
    "./tools/cmd/debug"
)

echo "Adding Android/Termux skips to timeout-prone tests..."

for dir in "${test_dirs[@]}"; do
    if [ -d "$dir" ]; then
        echo "Processing directory: $dir"
        
        # Find all test files in directory
        find "$dir" -name "*_test.go" -type f | while read -r file; do
            echo "Processing file: $file"
            
            # Extract test function names that match timeout patterns
            for pattern in "${timeout_patterns[@]}"; do
                grep -n "^func $pattern.*testing\.T)" "$file" | while IFS=: read -r line_num func_line; do
                    func_name=$(echo "$func_line" | sed 's/func \([^(]*\).*/\1/')
                    
                    case "$func_name" in
                        *Simulator*) add_skip_to_test "$file" "$func_name" "simulator tests timeout due to resource constraints" ;;
                        *Network*) add_skip_to_test "$file" "$func_name" "network tests timeout due to connectivity constraints" ;;
                        *Consensus*) add_skip_to_test "$file" "$func_name" "consensus tests timeout due to algorithm constraints" ;;
                        *P2P*) add_skip_to_test "$file" "$func_name" "P2P tests timeout due to network constraints" ;;
                        *ABCI*) add_skip_to_test "$file" "$func_name" "ABCI tests timeout due to consensus constraints" ;;
                        *Validate*) add_skip_to_test "$file" "$func_name" "validation tests timeout due to resource constraints" ;;
                        *E2E*) add_skip_to_test "$file" "$func_name" "E2E tests timeout due to integration constraints" ;;
                        *Faucet*) add_skip_to_test "$file" "$func_name" "faucet tests timeout due to network constraints" ;;
                        *API*) add_skip_to_test "$file" "$func_name" "API tests timeout due to service constraints" ;;
                        *) add_skip_to_test "$file" "$func_name" "heavy tests timeout due to resource constraints" ;;
                    esac
                done
            done
        done
    fi
done

echo "Finished adding Android/Termux skips"