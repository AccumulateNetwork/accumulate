#!/bin/bash

echo "=== TEST ANALYSIS REPORT ==="
echo ""
echo "Analyzing test results from full_test.log..."
echo ""

# Count total packages
total_packages=$(grep -c "^ok\|^FAIL\|^\?" full_test.log 2>/dev/null || echo "0")

# Count passing packages
passing_packages=$(grep -c "^ok" full_test.log 2>/dev/null || echo "0")

# Count failing packages  
failing_packages=$(grep -c "^FAIL" full_test.log 2>/dev/null || echo "0")

# Get failing package names
echo "=== SUMMARY ==="
echo "Total packages tested: $total_packages"
echo "Passing packages: $passing_packages"
echo "Failing packages: $failing_packages"
echo ""

echo "=== FAILING PACKAGES ==="
grep "^FAIL" full_test.log 2>/dev/null | awk '{print $2}' || echo "None found"
echo ""

echo "=== INDIVIDUAL TEST FAILURES ==="
grep "--- FAIL:" full_test.log 2>/dev/null | head -20 || echo "None found"
echo ""

echo "=== COVERAGE SUMMARY ==="
grep "coverage:" full_test.log 2>/dev/null | grep -v "no statements" | head -20 || echo "No coverage data"
echo ""

echo "=== TEST TIMING (slowest tests) ==="
grep "--- PASS:" full_test.log 2>/dev/null | sort -t'(' -k2 -rn | head -10 || echo "No timing data"
echo ""

# Check if coverage file exists and analyze it
if [ -f coverage_all.out ]; then
    echo "=== OVERALL COVERAGE ==="
    go tool cover -func=coverage_all.out 2>/dev/null | tail -1 || echo "Could not calculate overall coverage"
fi