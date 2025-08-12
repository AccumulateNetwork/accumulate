#!/bin/bash
# Wrapper script to run debug sequence mainnet without redirection issues

echo "Running: debug sequence mainnet"
echo "==============================="
echo ""

# Run with a timeout to prevent hanging
timeout 30 ./debug sequence mainnet --debug --verbose

exit_code=$?
if [ $exit_code -eq 124 ]; then
    echo ""
    echo "Command timed out after 30 seconds"
    echo "This may indicate network connectivity issues"
fi

exit $exit_code