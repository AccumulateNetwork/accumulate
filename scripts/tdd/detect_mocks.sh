#!/bin/bash
# TDD Mock Detection Script
# Validates that mocks are only used in *_test.go files

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "🔍 TDD Mock Detection"
echo "===================="

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

# Directories to check (production code only)
PRODUCTION_DIRS=("cmd/" "internal/impl/" "pkg/")
VIOLATIONS_FOUND=false
REPORT_FILE="mock_violations_report.txt"

# Clean up previous reports
rm -f "$REPORT_FILE"
echo "Mock Detection Report - $(date)" > "$REPORT_FILE"
echo "=================================" >> "$REPORT_FILE"

print_status "info" "Checking for mock usage in production code directories..."

for dir in "${PRODUCTION_DIRS[@]}"; do
    if [ -d "$dir" ]; then
        print_status "info" "Scanning $dir"
        
        # Search for files containing "Mock" that are NOT test files
        MOCK_FILES=$(find "$dir" -name "*.go" ! -name "*_test.go" -exec grep -l "Mock" {} \; 2>/dev/null || true)
        
        if [ -n "$MOCK_FILES" ]; then
            print_status "fail" "Mock violations found in $dir:"
            echo "" >> "$REPORT_FILE"
            echo "Violations in $dir:" >> "$REPORT_FILE"
            
            while IFS= read -r file; do
                if [ -n "$file" ]; then
                    echo "  📁 $file" | tee -a "$REPORT_FILE"
                    
                    # Show the specific lines containing "Mock"
                    MOCK_LINES=$(grep -n "Mock" "$file" 2>/dev/null || true)
                    if [ -n "$MOCK_LINES" ]; then
                        while IFS= read -r line; do
                            if [ -n "$line" ]; then
                                echo "    🔴 Line $line" | tee -a "$REPORT_FILE"
                            fi
                        done <<< "$MOCK_LINES"
                    fi
                fi
            done <<< "$MOCK_FILES"
            
            VIOLATIONS_FOUND=true
        else
            print_status "pass" "No mock violations in $dir"
        fi
    else
        print_status "warn" "Directory $dir not found"
    fi
done

echo "" >> "$REPORT_FILE"

# Additional check: Look for specific mock patterns
print_status "info" "Checking for common mock patterns..."

MOCK_PATTERNS=(
    "type.*Mock.*struct"
    "func.*Mock.*"
    "mock\.Mock"
    "testify/mock"
    "gomock"
    "*Mock.*"
)

for pattern in "${MOCK_PATTERNS[@]}"; do
    for dir in "${PRODUCTION_DIRS[@]}"; do
        if [ -d "$dir" ]; then
            PATTERN_MATCHES=$(find "$dir" -name "*.go" ! -name "*_test.go" -exec grep -l "$pattern" {} \; 2>/dev/null || true)
            
            if [ -n "$PATTERN_MATCHES" ]; then
                echo "Pattern '$pattern' found in production code:" >> "$REPORT_FILE"
                while IFS= read -r file; do
                    if [ -n "$file" ]; then
                        echo "  - $file" >> "$REPORT_FILE"
                        VIOLATIONS_FOUND=true
                    fi
                done <<< "$PATTERN_MATCHES"
            fi
        fi
    done
done

# Check for proper mock usage in test files
print_status "info" "Verifying proper mock usage in test files..."

TEST_MOCK_COUNT=$(find . -name "*_test.go" -exec grep -l "Mock" {} \; 2>/dev/null | wc -l || echo "0")
echo "Test files with mocks: $TEST_MOCK_COUNT" >> "$REPORT_FILE"

if [ "$TEST_MOCK_COUNT" -gt 0 ]; then
    print_status "pass" "Found $TEST_MOCK_COUNT test files properly using mocks"
    echo "Test files properly using mocks:" >> "$REPORT_FILE"
    find . -name "*_test.go" -exec grep -l "Mock" {} \; 2>/dev/null | while read file; do
        echo "  ✅ $file" >> "$REPORT_FILE"
    done
else
    print_status "warn" "No test files found using mocks"
fi

echo "" >> "$REPORT_FILE"

# Final result
if [ "$VIOLATIONS_FOUND" = true ]; then
    echo "VIOLATIONS DETECTED:" >> "$REPORT_FILE"
    echo "- Mocks found in production code directories" >> "$REPORT_FILE"
    echo "- This violates TDD principles" >> "$REPORT_FILE"
    echo "- Move all mocks to *_test.go files" >> "$REPORT_FILE"
    
    print_status "fail" "TDD Mock validation FAILED"
    print_status "info" "Violations report saved to: $REPORT_FILE"
    echo ""
    echo "🚨 REQUIRED ACTIONS:"
    echo "1. Move all Mock types to *_test.go files"
    echo "2. Ensure mocks are only used in test code"
    echo "3. Use interfaces for dependency injection in production code"
    echo "4. Re-run this script after fixes"
    
    exit 1
else
    echo "VALIDATION PASSED:" >> "$REPORT_FILE"
    echo "- No mocks found in production code" >> "$REPORT_FILE"
    echo "- TDD mock usage principles followed" >> "$REPORT_FILE"
    
    print_status "pass" "TDD Mock validation PASSED"
    print_status "info" "Report saved to: $REPORT_FILE"
    echo ""
    echo "🎉 EXCELLENT TDD COMPLIANCE:"
    echo "• No mocks in cmd/, internal/impl/, pkg/ directories"
    echo "• Mocks properly contained in test files"
    echo "• Production code follows dependency injection patterns"
fi

# Additional recommendations
echo ""
print_status "info" "TDD Best Practice Recommendations:"
echo "• Use interfaces for all external dependencies"
echo "• Create mocks in *_test.go files using testify/mock"
echo "• Prefix all mock types with 'Mock' for clarity"
echo "• Keep production code testable without mocks"