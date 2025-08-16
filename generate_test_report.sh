#!/bin/bash

# Comprehensive Test Report Generator for Accumulate
# Tests packages systematically and generates detailed report

RESULTS_DIR="./test_results"
mkdir -p "$RESULTS_DIR"

echo "=== ACCUMULATE PROJECT COMPREHENSIVE TEST REPORT ===" > "$RESULTS_DIR/full_report.txt"
echo "Generated on: $(date)" >> "$RESULTS_DIR/full_report.txt"
echo "" >> "$RESULTS_DIR/full_report.txt"

# Function to test package group and capture results
test_package_group() {
    local group_name="$1" 
    local pattern="$2"
    local output_file="$3"
    
    echo "Testing $group_name packages..."
    echo "=== $group_name PACKAGES ===" >> "$output_file"
    echo "" >> "$output_file"
    
    # Run tests and capture output
    go test -short -v $pattern > "$RESULTS_DIR/${group_name}_raw.txt" 2>&1
    local exit_code=$?
    
    # Parse results
    local passed_packages=()
    local failed_packages=()
    local no_test_packages=()
    local skipped_tests=()
    
    while IFS= read -r line; do
        if [[ "$line" =~ ^ok[[:space:]]+([^[:space:]]+) ]]; then
            passed_packages+=("${BASH_REMATCH[1]}")
        elif [[ "$line" =~ ^FAIL[[:space:]]+([^[:space:]]+) ]]; then
            failed_packages+=("${BASH_REMATCH[1]}")
        elif [[ "$line" =~ ^\?[[:space:]]+([^[:space:]]+)[[:space:]]+\[no[[:space:]]test[[:space:]]files\] ]]; then
            no_test_packages+=("${BASH_REMATCH[1]}")
        elif [[ "$line" =~ SKIP: ]]; then
            skipped_tests+=("$line")
        fi
    done < "$RESULTS_DIR/${group_name}_raw.txt"
    
    # Report results
    echo "PASSED (${#passed_packages[@]} packages):" >> "$output_file"
    if [ ${#passed_packages[@]} -gt 0 ]; then
        printf '  %s\n' "${passed_packages[@]}" >> "$output_file"
    else
        echo "  (none)" >> "$output_file"
    fi
    echo "" >> "$output_file"
    
    echo "FAILED (${#failed_packages[@]} packages):" >> "$output_file"
    if [ ${#failed_packages[@]} -gt 0 ]; then
        printf '  %s\n' "${failed_packages[@]}" >> "$output_file"
    else
        echo "  (none)" >> "$output_file"
    fi
    echo "" >> "$output_file"
    
    echo "NO TESTS (${#no_test_packages[@]} packages):" >> "$output_file"
    if [ ${#no_test_packages[@]} -gt 0 ]; then
        printf '  %s\n' "${no_test_packages[@]}" >> "$output_file"
    else
        echo "  (none)" >> "$output_file"
    fi
    echo "" >> "$output_file"
    
    if [ ${#skipped_tests[@]} -gt 0 ]; then
        echo "SKIPPED TESTS:" >> "$output_file"
        printf '  %s\n' "${skipped_tests[@]}" >> "$output_file"
        echo "" >> "$output_file"
    fi
    
    # Summary for this group
    local total=$((${#passed_packages[@]} + ${#failed_packages[@]} + ${#no_test_packages[@]}))
    local testable=$((${#passed_packages[@]} + ${#failed_packages[@]}))
    
    echo "SUMMARY for $group_name:" >> "$output_file"
    echo "  Total Packages: $total" >> "$output_file"
    echo "  Passed: ${#passed_packages[@]}" >> "$output_file"
    echo "  Failed: ${#failed_packages[@]}" >> "$output_file"
    echo "  No Tests: ${#no_test_packages[@]}" >> "$output_file"
    echo "  Skipped Tests: ${#skipped_tests[@]}" >> "$output_file"
    if [ $testable -gt 0 ]; then
        local success_rate=$((${#passed_packages[@]} * 100 / testable))
        echo "  Success Rate: ${success_rate}%" >> "$output_file"
    fi
    echo "" >> "$output_file"
    echo "================================================" >> "$output_file"
    echo "" >> "$output_file"
    
    # Return counts for global summary
    echo "${#passed_packages[@]} ${#failed_packages[@]} ${#no_test_packages[@]} ${#skipped_tests[@]}"
}

# Test different package groups
echo "Starting comprehensive test run..."

# Initialize global counters
total_passed=0
total_failed=0
total_no_tests=0
total_skipped=0

# Test package groups
echo "1. Testing PKG packages..."
result=$(test_package_group "PKG" "./pkg/..." "$RESULTS_DIR/full_report.txt")
read pkg_passed pkg_failed pkg_no_tests pkg_skipped <<< "$result"

echo "2. Testing INTERNAL packages..."
result=$(test_package_group "INTERNAL" "./internal/..." "$RESULTS_DIR/full_report.txt") 
read int_passed int_failed int_no_tests int_skipped <<< "$result"

echo "3. Testing PROTOCOL packages..."
result=$(test_package_group "PROTOCOL" "./protocol" "$RESULTS_DIR/full_report.txt")
read proto_passed proto_failed proto_no_tests proto_skipped <<< "$result"

echo "4. Testing TEST packages..."
result=$(test_package_group "TEST" "./test/..." "$RESULTS_DIR/full_report.txt")
read test_passed test_failed test_no_tests test_skipped <<< "$result"

echo "5. Testing CMD packages..."
result=$(test_package_group "CMD" "./cmd/..." "$RESULTS_DIR/full_report.txt")
read cmd_passed cmd_failed cmd_no_tests cmd_skipped <<< "$result"

echo "6. Testing EXP packages..."
result=$(test_package_group "EXP" "./exp/..." "$RESULTS_DIR/full_report.txt")
read exp_passed exp_failed exp_no_tests exp_skipped <<< "$result"

echo "7. Testing TOOLS packages..."
result=$(test_package_group "TOOLS" "./tools/..." "$RESULTS_DIR/full_report.txt")
read tools_passed tools_failed tools_no_tests tools_skipped <<< "$result"

echo "8. Testing VDK packages..."
result=$(test_package_group "VDK" "./vdk/..." "$RESULTS_DIR/full_report.txt")
read vdk_passed vdk_failed vdk_no_tests vdk_skipped <<< "$result"

# Calculate totals
total_passed=$((pkg_passed + int_passed + proto_passed + test_passed + cmd_passed + exp_passed + tools_passed + vdk_passed))
total_failed=$((pkg_failed + int_failed + proto_failed + test_failed + cmd_failed + exp_failed + tools_failed + vdk_failed))
total_no_tests=$((pkg_no_tests + int_no_tests + proto_no_tests + test_no_tests + cmd_no_tests + exp_no_tests + tools_no_tests + vdk_no_tests))
total_skipped=$((pkg_skipped + int_skipped + proto_skipped + test_skipped + cmd_skipped + exp_skipped + tools_skipped + vdk_skipped))

total_packages=$((total_passed + total_failed + total_no_tests))
testable_packages=$((total_passed + total_failed))

# Generate overall summary
{
    echo "=== OVERALL PROJECT SUMMARY ==="
    echo ""
    echo "GRAND TOTALS:"
    echo "  Total Packages: $total_packages"
    echo "  Passed: $total_passed"
    echo "  Failed: $total_failed" 
    echo "  No Tests: $total_no_tests"
    echo "  Total Skipped Tests: $total_skipped"
    echo ""
    
    if [ $testable_packages -gt 0 ]; then
        overall_success_rate=$((total_passed * 100 / testable_packages))
        echo "  Overall Success Rate: ${overall_success_rate}%"
    fi
    
    echo ""
    echo "BREAKDOWN BY CATEGORY:"
    echo "  PKG:      P:$pkg_passed F:$pkg_failed N:$pkg_no_tests S:$pkg_skipped"
    echo "  INTERNAL: P:$int_passed F:$int_failed N:$int_no_tests S:$int_skipped"  
    echo "  PROTOCOL: P:$proto_passed F:$proto_failed N:$proto_no_tests S:$proto_skipped"
    echo "  TEST:     P:$test_passed F:$test_failed N:$test_no_tests S:$test_skipped"
    echo "  CMD:      P:$cmd_passed F:$cmd_failed N:$cmd_no_tests S:$cmd_skipped"
    echo "  EXP:      P:$exp_passed F:$exp_failed N:$exp_no_tests S:$exp_skipped"
    echo "  TOOLS:    P:$tools_passed F:$tools_failed N:$tools_no_tests S:$tools_skipped"
    echo "  VDK:      P:$vdk_passed F:$vdk_failed N:$vdk_no_tests S:$vdk_skipped"
    echo ""
    echo "(P=Passed, F=Failed, N=No Tests, S=Skipped Tests)"
    
} >> "$RESULTS_DIR/full_report.txt"

echo ""
echo "=== TEST REPORT COMPLETED ==="
echo "Full report saved to: $RESULTS_DIR/full_report.txt"
echo ""
echo "Quick Summary:"
echo "  Total: $total_packages packages"
echo "  Passed: $total_passed"
echo "  Failed: $total_failed"
echo "  No Tests: $total_no_tests"
if [ $testable_packages -gt 0 ]; then
    overall_success_rate=$((total_passed * 100 / testable_packages))
    echo "  Success Rate: ${overall_success_rate}%"
fi