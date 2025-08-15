#!/bin/bash

# Faucet Collection Test Runner
# This script runs the faucet collection test against a DevNet

set -e

echo "🚰 Accumulate Faucet Collection Test Runner"
echo "==========================================="

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

# Run the faucet collection test
echo "🚀 Starting faucet collection test..."
echo
go run faucet_collection_test.go

echo
echo "✅ Test completed successfully!"