#!/bin/bash

# Integration Test Runner for MCP Accumulate
# This script runs integration tests against a local devnet

set -e

echo "==================================="
echo "MCP Accumulate Integration Tests"
echo "==================================="
echo

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if devnet is running (try v3 API)
echo "Checking devnet connectivity..."
if ! curl -s -X POST http://127.0.0.1:26660/v3 -H "Content-Type: application/json" -d '{"jsonrpc":"2.0","id":1,"method":"query","params":{}}' > /dev/null 2>&1; then
    echo -e "${RED}ERROR: Devnet is not accessible at http://127.0.0.1:26660/v3${NC}"
    echo "Please start the devnet before running integration tests."
    echo
    echo "You can start devnet with:"
    echo "  cd ~/go/src/gitlab.com/AccumulateNetwork/devnet"
    echo "  ./devnet-mcp"
    exit 1
fi
echo -e "${GREEN}✓ Devnet is accessible${NC}"
echo

# Check if ccli is available
echo "Checking for ccli binary..."
CCLI_PATH=""
if [ -f "./ccli" ]; then
    CCLI_PATH="./ccli"
elif [ -f "../wallet/ccli" ]; then
    CCLI_PATH="../wallet/ccli"
elif command -v ccli &> /dev/null; then
    CCLI_PATH=$(which ccli)
fi

if [ -n "$CCLI_PATH" ]; then
    echo -e "${GREEN}✓ Found ccli at: $CCLI_PATH${NC}"
    export CCLI_PATH
else
    echo -e "${YELLOW}⚠ ccli not found (wallet tests will be skipped)${NC}"
fi
echo

# Run tests based on arguments
TEST_LEVEL=${1:-basic}

case $TEST_LEVEL in
    basic)
        echo "Running BASIC integration tests (queries, network info)..."
        echo "This tests read-only operations that don't require funded accounts."
        echo
        go test -v -tags=integration -run "TestDevnet(Query|NodeInfo|CreateLiteAccount|NetworkStatus|ChainQuery|BlockQuery|Directory)" -timeout 5m
        ;;

    wallet)
        echo "Running WALLET integration tests..."
        echo "This tests wallet operations using ccli."
        echo
        if [ -z "$CCLI_PATH" ]; then
            echo -e "${RED}ERROR: ccli not found. Set CCLI_PATH or ensure ccli is in PATH.${NC}"
            exit 1
        fi
        go test -v -tags=integration -run "TestWallet" -timeout 5m
        ;;

    transactions)
        echo "Running TRANSACTION integration tests..."
        echo "⚠ WARNING: These tests require funded accounts and may fail without proper setup."
        echo
        echo "Prerequisites:"
        echo "  1. Devnet must be running with faucet enabled"
        echo "  2. Set INTEGRATION_FULL=true to enable full tests"
        echo "  3. Optionally set TEST_ADI_URL, TEST_DATA_ACCOUNT_URL, TEST_PRIVATE_KEY for specific tests"
        echo
        read -p "Continue? (y/n) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            echo "Aborted."
            exit 0
        fi
        export INTEGRATION_FULL=true
        export DEVNET_FAUCET_ENABLED=true
        go test -v -tags=integration -run "TestDevnet(TokenSend|CreateADI|CreateDataAccount|WriteData|AddCredits|BurnTokens|UpdateKeyPage)" -timeout 10m
        ;;

    full-workflow)
        echo "Running FULL WORKFLOW integration test..."
        echo "This runs a complete end-to-end workflow test."
        echo
        export INTEGRATION_FULL=true
        export DEVNET_FAUCET_ENABLED=true
        go test -v -tags=integration -run "TestDevnetFullWorkflow" -timeout 10m
        ;;

    all)
        echo "Running ALL integration tests..."
        echo
        if [ -z "$CCLI_PATH" ]; then
            echo -e "${YELLOW}⚠ ccli not found. Wallet tests will be skipped.${NC}"
        fi
        echo -e "${YELLOW}⚠ WARNING: Transaction tests may fail without funded accounts.${NC}"
        echo
        export INTEGRATION_FULL=true
        export DEVNET_FAUCET_ENABLED=true
        go test -v -tags=integration -timeout 15m
        ;;

    coverage)
        echo "Running integration tests with coverage..."
        echo
        go test -v -tags=integration -coverprofile=integration_coverage.out -timeout 10m
        echo
        echo "Coverage report:"
        go tool cover -func=integration_coverage.out
        ;;

    *)
        echo "Usage: $0 [basic|wallet|transactions|full-workflow|all|coverage]"
        echo
        echo "Test levels:"
        echo "  basic          - Read-only network queries (default)"
        echo "  wallet         - Wallet operations using ccli"
        echo "  transactions   - Transaction submission tests (requires funded accounts)"
        echo "  full-workflow  - Complete end-to-end workflow"
        echo "  all            - Run all integration tests"
        echo "  coverage       - Run with coverage report"
        echo
        echo "Environment variables:"
        echo "  CCLI_PATH              - Path to ccli binary"
        echo "  INTEGRATION_FULL       - Enable full integration tests (set to 'true')"
        echo "  DEVNET_FAUCET_ENABLED  - Enable faucet tests (set to 'true')"
        echo "  TEST_TRANSACTION_ID    - Known transaction ID for query tests"
        echo "  TEST_PUBLIC_KEY        - Public key for search tests"
        echo "  TEST_ADI_URL           - Existing ADI for data account tests"
        echo "  TEST_DATA_ACCOUNT_URL  - Existing data account for write tests"
        echo "  TEST_PRIVATE_KEY       - Private key for transaction signing"
        exit 1
        ;;
esac

echo
echo -e "${GREEN}==================================="
echo "Integration Tests Complete"
echo -e "===================================${NC}"
