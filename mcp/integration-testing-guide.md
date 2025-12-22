# Integration Testing Guide

This document describes the integration tests for the MCP Accumulate server and how to run them against a local devnet.

## Overview

The integration tests validate that the MCP server can actually interact with a real Accumulate network (devnet). Unlike unit tests which use mocks, these tests make actual API calls and verify real network responses.

## Test Categories

### 1. Basic Network Queries (`integration_test.go`)

**What it tests:**
- Querying accounts (ACME token issuer)
- Querying directories
- Getting node information
- Getting network metrics
- Creating lite account addresses
- Querying chains
- Querying blocks
- Network status queries
- Search operations

**Requirements:**
- Devnet running at `http://127.0.0.1:26660/v2`

**Run with:**
```bash
./run-integration-tests.sh basic
```

### 2. Transaction Operations (`integration_transactions_test.go`)

**What it tests:**
- Sending tokens
- Creating ADIs
- Creating data accounts
- Writing data to data accounts
- Adding credits
- Updating key pages
- Burning tokens
- Full end-to-end workflow

**Requirements:**
- Devnet running with faucet enabled
- Funded accounts for transaction tests
- Set `INTEGRATION_FULL=true` to enable

**Run with:**
```bash
./run-integration-tests.sh transactions
```

### 3. Wallet Operations (`integration_wallet_test.go`)

**What it tests:**
- Wallet initialization
- Key generation
- Key listing
- Vault operations
- Key naming conventions
- Concurrent operations
- Error handling

**Requirements:**
- `ccli` binary available
- Set `CCLI_PATH` environment variable

**Run with:**
```bash
./run-integration-tests.sh wallet
```

## Prerequisites

### 1. Start Devnet

The devnet must be running before running integration tests:

```bash
# Check if devnet is running
ps aux | grep devnet

# If not running, start it
cd ~/go/src/gitlab.com/AccumulateNetwork/devnet
./devnet-mcp
```

Verify devnet is accessible:
```bash
curl -s http://127.0.0.1:26660/v2
# Should return "Method Not Allowed" (this is expected, it means the server is up)
```

### 2. Build ccli (for wallet tests)

```bash
cd ~/go/src/gitlab.com/AccumulateNetwork/wallet
go build -o ccli ./cmd/ccli

# Set environment variable
export CCLI_PATH=~/go/src/gitlab.com/AccumulateNetwork/wallet/ccli
```

## Running Tests

### Quick Start - Basic Tests

Run read-only network queries (no funded accounts needed):

```bash
cd ~/go/src/gitlab.com/AccumulateNetwork/mcp-accumulate
./run-integration-tests.sh basic
```

### Wallet Tests

Test wallet operations (requires ccli):

```bash
export CCLI_PATH=/path/to/ccli
./run-integration-tests.sh wallet
```

### Transaction Tests

Test transaction submission (requires funded accounts):

```bash
# Enable full integration mode and faucet
export INTEGRATION_FULL=true
export DEVNET_FAUCET_ENABLED=true

./run-integration-tests.sh transactions
```

### Full Workflow Test

Run a complete end-to-end test:

```bash
export INTEGRATION_FULL=true
export DEVNET_FAUCET_ENABLED=true

./run-integration-tests.sh full-workflow
```

### All Tests

Run all integration tests:

```bash
export CCLI_PATH=/path/to/ccli
export INTEGRATION_FULL=true
export DEVNET_FAUCET_ENABLED=true

./run-integration-tests.sh all
```

### With Coverage

Generate coverage report:

```bash
./run-integration-tests.sh coverage
```

## Environment Variables

| Variable | Description | Required For |
|----------|-------------|--------------|
| `CCLI_PATH` | Path to ccli binary | Wallet tests |
| `INTEGRATION_FULL` | Enable full transaction tests (`true`/`false`) | Transaction tests |
| `DEVNET_FAUCET_ENABLED` | Enable faucet tests (`true`/`false`) | Faucet tests |
| `TEST_TRANSACTION_ID` | Known transaction ID for query tests | Transaction query tests |
| `TEST_PUBLIC_KEY` | Public key for search tests | Search tests |
| `TEST_ADI_URL` | Existing ADI URL | Data account creation tests |
| `TEST_DATA_ACCOUNT_URL` | Existing data account URL | Data write tests |
| `TEST_PRIVATE_KEY` | Private key for signing | Various transaction tests |
| `TEST_KEY_PAGE_URL` | Key page URL for update tests | Key page tests |
| `TEST_OLD_KEY` | Old key for key page update | Key page tests |
| `TEST_NEW_KEY` | New key for key page update | Key page tests |

## Running Individual Tests

You can run specific tests using Go's `-run` flag:

```bash
# Run only the account query test
go test -v -tags=integration -run TestDevnetQuery

# Run only wallet initialization test
go test -v -tags=integration -run TestWalletIntegration/InitWallet

# Run all tests matching a pattern
go test -v -tags=integration -run "TestDevnet.*Query"
```

## Test Structure

### Integration Test Build Tag

All integration tests use the build tag `integration`:

```go
// +build integration
```

This prevents them from running during normal `go test ./...` commands. To run integration tests, you must explicitly specify the tag:

```bash
go test -tags=integration ./...
```

### Test Timeouts

Integration tests use longer timeouts than unit tests:

- Basic queries: 30 seconds
- Transaction operations: 60 seconds
- Full workflow: 60+ seconds
- Wallet operations: 30 seconds

You can adjust the global timeout with the `-timeout` flag:

```bash
go test -v -tags=integration -timeout 15m
```

## Expected Results

### Basic Tests (Passing)

✅ TestDevnetQuery - Query ACME account
✅ TestDevnetQueryDirectory - Query root directory
✅ TestDevnetNodeInfo - Get node information
✅ TestDevnetQueryMetrics - Get network metrics
✅ TestDevnetCreateLiteAccount - Create lite account address
✅ TestDevnetChainQuery - Query chain entries
✅ TestDevnetBlockQuery - Query block data
✅ TestDevnetNetworkStatus - Get network status

### Wallet Tests (Passing with ccli)

✅ TestWalletIntegration/InitWallet - Initialize wallet
✅ TestWalletIntegration/GenerateKey - Generate key pair
✅ TestWalletIntegration/ListKeys - List wallet keys
✅ TestWalletIntegration/GenerateMultipleKeys - Generate multiple keys
✅ TestWalletKeyNaming - Test key naming conventions

### Transaction Tests (May Skip or Fail)

⚠️ TestDevnetTokenSend - Requires funded account
⚠️ TestDevnetCreateADI - Requires funded account
⚠️ TestDevnetCreateDataAccount - Requires existing ADI
⚠️ TestDevnetWriteData - Requires data account and key
⚠️ TestDevnetFullWorkflow - Requires faucet access

Many transaction tests will be skipped unless proper environment variables are set and accounts are funded.

## Troubleshooting

### Devnet Not Accessible

```
ERROR: Devnet is not accessible at http://127.0.0.1:26660/v2
```

**Solution:** Start the devnet:
```bash
cd ~/go/src/gitlab.com/AccumulateNetwork/devnet
./devnet-mcp
```

### ccli Not Found

```
⚠ ccli not found (wallet tests will be skipped)
```

**Solution:** Build and export ccli path:
```bash
cd ~/go/src/gitlab.com/AccumulateNetwork/wallet
go build -o ccli ./cmd/ccli
export CCLI_PATH=$(pwd)/ccli
```

### Transaction Tests Failing

```
Token send failed (expected if account not funded): insufficient balance
```

**Solution:** This is expected. Transaction tests require:
1. Faucet access to fund accounts
2. Setting `INTEGRATION_FULL=true` and `DEVNET_FAUCET_ENABLED=true`
3. Waiting for faucet transactions to be confirmed

### Test Timeouts

```
panic: test timed out after 2m0s
```

**Solution:** Increase timeout:
```bash
go test -v -tags=integration -timeout 15m
```

## Coverage

Integration tests complement unit tests by validating:

1. **Actual Network Interaction**: Real API calls vs mocked responses
2. **End-to-End Workflows**: Complete operations from key generation to transaction submission
3. **Error Handling**: Real network errors and edge cases
4. **Wallet Integration**: Actual ccli binary execution
5. **Transaction Lifecycle**: Submit, wait for confirmation, query results

## What's NOT Tested

Integration tests do NOT cover:

1. **Mainnet/Testnet**: Only devnet is tested
2. **Performance**: No load testing or benchmarks
3. **Long-running Operations**: Tests complete in minutes, not hours/days
4. **Network Failures**: Simulated network outages not tested
5. **Consensus Edge Cases**: Advanced network scenarios not covered

## Next Steps

After integration tests pass:

1. **Test on Testnet**: Validate against public testnet
2. **Test on Mainnet**: Careful validation with real ACME
3. **Add Monitoring**: Set up continuous integration testing
4. **Document Workflows**: Create user-facing documentation
5. **Security Audit**: Professional review before production use

## Contributing

When adding new MCP tools:

1. Add corresponding integration test
2. Document required environment variables
3. Add test to appropriate category (basic/transactions/wallet)
4. Update this guide with new test information
5. Update `run-integration-tests.sh` if needed

## Resources

- **Accumulate SDK Docs**: https://docs.accumulatenetwork.io/
- **MCP Protocol**: https://modelcontextprotocol.io/
- **Go Testing**: https://golang.org/pkg/testing/
- **Devnet Setup**: `~/go/src/gitlab.com/AccumulateNetwork/devnet/README.md`
