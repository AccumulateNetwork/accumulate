#!/bin/bash
# TDD Coverage Verification Script
# Verifies that test coverage meets TDD requirements (≥80%)

set -euo pipefail

# Configuration
MIN_COVERAGE=80
COVERAGE_FILE="coverage.out"
REPORT_FILE="coverage_report.txt"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "🧪 TDD Coverage Verification"
echo "=========================="

# Function to print colored output
print_status() {
    local status=$1
    local message=$2
    case $status in
        "pass") echo -e "${GREEN}✅ $message${NC}" ;;
        "fail") echo -e "${RED}❌ $message${NC}" ;;
        "warn") echo -e "${YELLOW}⚠️  $message${NC}" ;;
        "info") echo -e "ℹ️  $message" ;;
    esac
}

# Function to run coverage for a specific package
run_package_coverage() {
    local package=$1
    print_status "info" "Running coverage for $package"
    
    if go test -coverprofile="${package//\//_}_coverage.out" "./$package" 2>/dev/null; then
        if [ -f "${package//\//_}_coverage.out" ]; then
            local coverage=$(go tool cover -func="${package//\//_}_coverage.out" | grep "total:" | awk '{print $3}' | sed 's/%//')
            if [ -n "$coverage" ]; then
                echo "$package: ${coverage}%" >> "$REPORT_FILE"
                if (( $(echo "$coverage >= $MIN_COVERAGE" | bc -l) )); then
                    print_status "pass" "$package: ${coverage}% coverage"
                else
                    print_status "fail" "$package: ${coverage}% coverage (below ${MIN_COVERAGE}%)"
                    return 1
                fi
            else
                print_status "warn" "$package: No coverage data"
                echo "$package: No coverage data" >> "$REPORT_FILE"
            fi
            rm -f "${package//\//_}_coverage.out"
        fi
    else
        print_status "warn" "$package: Test execution failed"
        echo "$package: Test execution failed" >> "$REPORT_FILE"
    fi
    return 0
}

# Clean up previous reports
rm -f "$COVERAGE_FILE" "$REPORT_FILE"
echo "Coverage Report - $(date)" > "$REPORT_FILE"
echo "=============================" >> "$REPORT_FILE"

# Run full test suite with coverage
print_status "info" "Running full test suite with coverage..."
if go test -coverprofile="$COVERAGE_FILE" ./...; then
    print_status "pass" "Test suite completed"
else
    print_status "fail" "Test suite failed"
    exit 1
fi

# Check overall coverage
if [ -f "$COVERAGE_FILE" ]; then
    TOTAL_COVERAGE=$(go tool cover -func="$COVERAGE_FILE" | grep "total:" | awk '{print $3}' | sed 's/%//')
    echo "" >> "$REPORT_FILE"
    echo "Overall Coverage: ${TOTAL_COVERAGE}%" >> "$REPORT_FILE"
    
    if (( $(echo "$TOTAL_COVERAGE >= $MIN_COVERAGE" | bc -l) )); then
        print_status "pass" "Overall coverage: ${TOTAL_COVERAGE}% (meets ${MIN_COVERAGE}% requirement)"
    else
        print_status "fail" "Overall coverage: ${TOTAL_COVERAGE}% (below ${MIN_COVERAGE}% requirement)"
        echo ""
        print_status "fail" "TDD Coverage verification FAILED"
        exit 1
    fi
else
    print_status "fail" "No coverage file generated"
    exit 1
fi

# Check specific critical packages
print_status "info" "Checking critical package coverage..."
CRITICAL_PACKAGES=(
    "internal/core/execute/v2/crosschain"
    "internal/core/execute"
    "pkg/api/v3"
)

FAILED_PACKAGES=()
for package in "${CRITICAL_PACKAGES[@]}"; do
    if [ -d "$package" ]; then
        if ! run_package_coverage "$package"; then
            FAILED_PACKAGES+=("$package")
        fi
    fi
done

echo "" >> "$REPORT_FILE"
if [ ${#FAILED_PACKAGES[@]} -gt 0 ]; then
    echo "Failed Packages:" >> "$REPORT_FILE"
    for package in "${FAILED_PACKAGES[@]}"; do
        echo "  - $package" >> "$REPORT_FILE"
    done
    print_status "fail" "${#FAILED_PACKAGES[@]} critical packages below coverage threshold"
else
    echo "All critical packages meet coverage requirements" >> "$REPORT_FILE"
    print_status "pass" "All critical packages meet coverage requirements"
fi

# Generate HTML report
if command -v go &> /dev/null; then
    go tool cover -html="$COVERAGE_FILE" -o coverage.html
    print_status "info" "HTML coverage report generated: coverage.html"
fi

echo ""
print_status "info" "Coverage report saved to: $REPORT_FILE"
print_status "pass" "TDD Coverage verification COMPLETED"

# Exit with error if any critical packages failed
if [ ${#FAILED_PACKAGES[@]} -gt 0 ]; then
    exit 1
fi