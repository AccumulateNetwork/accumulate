#!/bin/bash
# TDD Complete Validation Script
# Runs all TDD validation checks in sequence

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Get script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo -e "${BLUE}🧪 TDD Complete Validation Suite${NC}"
echo -e "${BLUE}==================================${NC}"
echo ""

# Function to print colored output
print_status() {
    local status=$1
    local message=$2
    case $status in
        "pass") echo -e "${GREEN}✅ $message${NC}" ;;
        "fail") echo -e "${RED}❌ $message${NC}" ;;
        "warn") echo -e "${YELLOW}⚠️  $message${NC}" ;;
        "info") echo -e "ℹ️  $message" ;;
        "header") echo -e "${BLUE}📋 $message${NC}" ;;
    esac
}

# Track validation results
TOTAL_CHECKS=0
PASSED_CHECKS=0
FAILED_CHECKS=0

run_validation() {
    local check_name=$1
    local script_path=$2
    local description=$3
    
    TOTAL_CHECKS=$((TOTAL_CHECKS + 1))
    
    print_status "header" "Running: $check_name"
    print_status "info" "$description"
    echo ""
    
    if [ -x "$script_path" ]; then
        if "$script_path"; then
            PASSED_CHECKS=$((PASSED_CHECKS + 1))
            print_status "pass" "$check_name completed successfully"
        else
            FAILED_CHECKS=$((FAILED_CHECKS + 1))
            print_status "fail" "$check_name failed"
        fi
    else
        print_status "fail" "Script not found or not executable: $script_path"
        FAILED_CHECKS=$((FAILED_CHECKS + 1))
    fi
    
    echo ""
    echo "----------------------------------------"
    echo ""
}

# Start validation suite
print_status "info" "Starting TDD validation suite..."
print_status "info" "Timestamp: $(date)"
echo ""

# 1. Mock Detection
run_validation \
    "Mock Detection" \
    "$SCRIPT_DIR/detect_mocks.sh" \
    "Validates that mocks are only used in *_test.go files"

# 2. Test Coverage
run_validation \
    "Test Coverage" \
    "$SCRIPT_DIR/verify_coverage.sh" \
    "Verifies that test coverage meets ≥80% requirement"

# 3. Build Validation
print_status "header" "Build Validation"
print_status "info" "Verifies that all packages compile without errors"
TOTAL_CHECKS=$((TOTAL_CHECKS + 1))

if go build ./...; then
    PASSED_CHECKS=$((PASSED_CHECKS + 1))
    print_status "pass" "All packages compile successfully"
else
    FAILED_CHECKS=$((FAILED_CHECKS + 1))
    print_status "fail" "Compilation errors found"
fi
echo ""
echo "----------------------------------------"
echo ""

# 4. Test Execution
print_status "header" "Test Execution"
print_status "info" "Runs all tests to ensure they pass"
TOTAL_CHECKS=$((TOTAL_CHECKS + 1))

if go test ./... -v > test_results.txt 2>&1; then
    PASSED_CHECKS=$((PASSED_CHECKS + 1))
    print_status "pass" "All tests pass"
else
    FAILED_CHECKS=$((FAILED_CHECKS + 1))
    print_status "fail" "Some tests are failing"
    print_status "info" "Check test_results.txt for details"
fi
echo ""
echo "----------------------------------------"
echo ""

# 5. Go Vet
print_status "header" "Static Analysis (go vet)"
print_status "info" "Runs go vet to check for suspicious code"
TOTAL_CHECKS=$((TOTAL_CHECKS + 1))

if go vet ./... > vet_results.txt 2>&1; then
    PASSED_CHECKS=$((PASSED_CHECKS + 1))
    print_status "pass" "No vet warnings found"
else
    FAILED_CHECKS=$((FAILED_CHECKS + 1))
    print_status "fail" "Go vet found issues"
    print_status "info" "Check vet_results.txt for details"
fi
echo ""
echo "----------------------------------------"
echo ""

# Final Results
print_status "header" "TDD Validation Results"
echo ""
print_status "info" "Total Checks: $TOTAL_CHECKS"
print_status "pass" "Passed: $PASSED_CHECKS"

if [ $FAILED_CHECKS -gt 0 ]; then
    print_status "fail" "Failed: $FAILED_CHECKS"
else
    print_status "info" "Failed: $FAILED_CHECKS"
fi

echo ""

if [ $FAILED_CHECKS -eq 0 ]; then
    print_status "pass" "🎉 ALL TDD VALIDATIONS PASSED!"
    echo ""
    echo -e "${GREEN}Your code follows TDD best practices:${NC}"
    echo "• ✅ No mocks in production code"
    echo "• ✅ Test coverage ≥80%"
    echo "• ✅ All code compiles"
    echo "• ✅ All tests pass"
    echo "• ✅ No static analysis issues"
    echo ""
    echo -e "${BLUE}Ready for code review and merge! 🚀${NC}"
    exit 0
else
    print_status "fail" "❌ TDD VALIDATION FAILED"
    echo ""
    echo -e "${RED}Issues found that need to be resolved:${NC}"
    
    if [ $FAILED_CHECKS -gt 0 ]; then
        echo "• $FAILED_CHECKS validation checks failed"
        echo "• Review the output above for specific issues"
        echo "• Fix all issues and re-run this script"
    fi
    
    echo ""
    echo -e "${YELLOW}Next Steps:${NC}"
    echo "1. Review the failed checks above"
    echo "2. Fix the identified issues"
    echo "3. Re-run: $0"
    echo "4. Ensure all checks pass before submitting PR"
    
    exit 1
fi