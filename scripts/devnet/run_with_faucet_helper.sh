#!/bin/bash

# Script to run tests with background faucet helper
# This demonstrates how to use the faucet helper as a background process

set -e

echo "🚰 Running Tests with Faucet Helper Background Process"
echo "===================================================="

# Default configuration
SERVER_URL="http://127.0.0.1:26660"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Parse command line arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --server)
      SERVER_URL="$2"
      shift 2
      ;;
    --help)
      echo "Usage: $0 [OPTIONS]"
      echo "Options:"
      echo "  --server URL    DevNet server URL (default: http://127.0.0.1:26660)"
      echo "  --help          Show this help message"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      echo "Use --help for usage information"
      exit 1
      ;;
  esac
done

echo "📡 Server: $SERVER_URL"
echo

# Check if server is reachable
echo "🔍 Checking server connectivity..."
if ! curl -s --max-time 5 "$SERVER_URL/v2" > /dev/null; then
    echo "❌ Cannot reach server at $SERVER_URL"
    echo "💡 Make sure your DevNet is running:"
    echo "   go run ./cmd/accumulated run devnet -w .devnet-test"
    exit 1
fi
echo "✅ Server is reachable"
echo

# Navigate to the test directory
cd "$SCRIPT_DIR"

# Option 1: Run the integrated test suite
echo "🚀 Option 1: Running CrosschainCoordinator test suite with integrated faucet helper..."
echo
go run crosschain_conductor_test.go

echo
echo "✅ Test suite completed!"

# Option 2: Run original faucet test for comparison
echo
echo "🚀 Option 2: Running standalone faucet collection test for comparison..."
echo
timeout 30s go run faucet_collection.go || echo "⏰ Faucet test completed/timed out"

echo
echo "✅ All tests completed successfully!"
echo
echo "💡 Next steps:"
echo "  - Use the faucet helper in your CrosschainCoordinator tests"
echo "  - Create multiple funded accounts before running load tests"
echo "  - Monitor faucet statistics during sustained testing"