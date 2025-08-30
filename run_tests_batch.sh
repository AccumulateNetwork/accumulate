#!/bin/bash

# Accumulate Project Test Runner - Optimized for faster execution
# Runs tests in smaller batches to avoid timeouts

RESULTS_DIR="./test_results"
mkdir -p "$RESULTS_DIR"

# Initialize report files
PASSED_FILE="$RESULTS_DIR/passed.txt"
FAILED_FILE="$RESULTS_DIR/failed.txt" 
SKIPPED_FILE="$RESULTS_DIR/skipped.txt"
NO_TESTS_FILE="$RESULTS_DIR/no_tests.txt"
ERROR_FILE="$RESULTS_DIR/errors.txt"
SUMMARY_FILE="$RESULTS_DIR/summary.txt"

# Clear previous results
> "$PASSED_FILE"
> "$FAILED_FILE"
> "$SKIPPED_FILE"
> "$NO_TESTS_FILE"
> "$ERROR_FILE"
> "$SUMMARY_FILE"

echo "Starting optimized test run for Accumulate project..."
echo ""

# Test categories for better organization
declare -A test_categories=(
    ["cmd"]="./cmd/..."
    ["exp"]="./exp/..."
    ["internal_api"]="./internal/api/..."
    ["internal_core"]="./internal/core/..."
    ["internal_database"]="./internal/database/..."
    ["internal_node"]="./internal/node/..."
    ["internal_util"]="./internal/util/..."
    ["pkg_api"]="./pkg/api/..."
    ["pkg_client"]="./pkg/client/..."
    ["pkg_database"]="./pkg/database/..."
    ["pkg_types"]="./pkg/types/..."
    ["pkg_other"]="./pkg/accumulate ./pkg/build ./pkg/errors ./pkg/proxy ./pkg/url"
    ["protocol"]="./protocol"
    ["test"]="./test/..."
    ["tools"]="./tools/..."
    ["vdk"]="./vdk/..."
)

# Counters
total_categories=${#test_categories[@]}
processed_categories=0

echo "Testing packages by category..."
echo "Total categories: $total_categories"
echo ""

# Test each category
for category in "${!test_categories[@]}"; do
    processed_categories=$((processed_categories + 1))
    pattern="${test_categories[$category]}"
    
    echo "[$processed_categories/$total_categories] Testing category: $category (pattern: $pattern)"
    
    # Run test for this category with timeout and collect output
    timeout 3m go test -short -v $pattern 2>&1 | while IFS= read -r line; do
        echo "$line"
        
        # Parse test results in real time
        if [[ "$line" =~ ^ok[[:space:]]+([^[:space:]]+) ]]; then
            package="${BASH_REMATCH[1]}"
            echo "$package" >> "$PASSED_FILE"
        elif [[ "$line" =~ ^FAIL[[:space:]]+([^[:space:]]+) ]]; then
            package="${BASH_REMATCH[1]}"
            echo "$package" >> "$FAILED_FILE"
        elif [[ "$line" =~ (\?[[:space:]]+[^[:space:]]+[[:space:]]+\[no[[:space:]]test[[:space:]]files\]) ]]; then
            package=$(echo "$line" | awk '{print $2}')
            echo "$package" >> "$NO_TESTS_FILE"
        elif [[ "$line" =~ SKIP: ]]; then
            echo "$line" >> "$SKIPPED_FILE"
        fi
    done
    
    test_exit_code=$?
    if [ $test_exit_code -eq 124 ]; then
        echo "$category (TIMEOUT)" >> "$ERROR_FILE"
        echo "  → TIMEOUT"
    elif [ $test_exit_code -ne 0 ]; then
        echo "$category (ERROR: $test_exit_code)" >> "$ERROR_FILE"
        echo "  → ERROR"
    else
        echo "  → COMPLETED"
    fi
    echo ""
done

echo "Generating summary..."

# Count results
passed_count=$(wc -l < "$PASSED_FILE" 2>/dev/null || echo 0)
failed_count=$(wc -l < "$FAILED_FILE" 2>/dev/null || echo 0)
no_tests_count=$(wc -l < "$NO_TESTS_FILE" 2>/dev/null || echo 0)
error_count=$(wc -l < "$ERROR_FILE" 2>/dev/null || echo 0)
skipped_lines=$(wc -l < "$SKIPPED_FILE" 2>/dev/null || echo 0)

total_packages=$((passed_count + failed_count + no_tests_count))

# Generate summary
{
    echo "=== ACCUMULATE PROJECT TEST SUMMARY ==="
    echo "Generated on: $(date)"
    echo ""
    echo "RESULTS BY CATEGORY:"
    echo "Total Categories Tested: $processed_categories"
    echo ""
    echo "PACKAGE RESULTS:"
    echo "Total Packages: $total_packages"
    echo "Passed: $passed_count"
    echo "Failed: $failed_count" 
    echo "No Tests: $no_tests_count"
    echo "Errors/Timeouts: $error_count"
    echo "Skipped Test Lines: $skipped_lines"
    echo ""
    
    if [ $total_packages -gt 0 ]; then
        testable_packages=$((passed_count + failed_count))
        if [ $testable_packages -gt 0 ]; then
            success_rate=$(( passed_count * 100 / testable_packages ))
            echo "Success Rate (excluding no-test packages): ${success_rate}%"
        fi
    fi
    
    echo ""
    echo "Results saved to: $RESULTS_DIR"
} > "$SUMMARY_FILE"

echo "Test run completed!"
echo ""
cat "$SUMMARY_FILE"