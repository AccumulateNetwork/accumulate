# Testing Funding Credits

## Overview

This package contains tests for the credit funding functionality. Following our testing philosophy, **ALL tests must run against a real devnet instance**, not mocks.

## Test Files

1. **funding_credits_devnet_test.go** - Main test file using real devnet
   - Tests credit top-up for lite accounts
   - Tests credit top-up for key pages
   - Tests error handling with real network failures
   - Tests concurrent operations
   - Includes benchmarks

2. **funding_credits_test.go** - Legacy mock-based tests (deprecated)
   - Should be phased out in favor of devnet tests
   - Kept temporarily for reference

## Prerequisites

### 1. Start Devnet

The devnet must be running before tests can execute:

```bash
# Option 1: Using docker-compose (if available)
cd /path/to/accumulate
docker-compose up -d

# Option 2: Using devnet scripts
cd scripts/devnet
./devnet_manager.sh start

# Option 3: Using accumulated init
accumulated init devnet -w ./devnet
accumulated run -w ./devnet
```

### 2. Verify Devnet is Running

```bash
# Check if devnet is responding
curl -X POST http://localhost:26660/v3 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"network.status","params":{}}'

# Or use the CLI
accumulate -s http://localhost:26660/v3 network status
```

## Running Tests

### Run All Credit Manager Tests
```bash
go test -v -run TestCreditManager ./funding_credits_devnet_test.go
```

### Run Specific Test Cases
```bash
# Test successful top-up
go test -v -run TestCreditManager_TopUpLiteAccount_Devnet/successful_top-up

# Test error handling
go test -v -run TestCreditManager_TopUpLiteAccount_Devnet/error_handling

# Test concurrent operations
go test -v -run TestConcurrentOperations_Devnet
```

### Run with Extended Timeout
```bash
# Some operations may take longer on slow machines
go test -v -timeout 5m -run TestCreditManager ./funding_credits_devnet_test.go
```

### Run Benchmarks
```bash
go test -bench=. -run=^$ ./funding_credits_devnet_test.go
```

## Test Structure

Each test follows this pattern:

1. **Check devnet availability** - Skip if not running
2. **Create real accounts** - Use faucet to fund accounts
3. **Execute real transactions** - Perform actual blockchain operations
4. **Verify on-chain state** - Query blockchain to verify results

Example:
```go
func TestSomething_Devnet(t *testing.T) {
    // Skip if devnet not available
    if !isDevnetAvailable() {
        t.Skip("Devnet not available")
    }
    
    // Create real client
    client := jsonrpc.NewClient(devnetAPIEndpoint)
    
    // Create real accounts
    account := createFundedLiteAccount(t, client, "test")
    
    // Execute real operations
    err := doSomething(account)
    
    // Verify on blockchain
    state := queryOnChainState(t, client, account.URL)
    assert.Equal(t, expected, state)
}
```

## Common Issues and Solutions

### Issue: "Devnet not available"
**Solution**: Start the devnet using one of the methods above

### Issue: "Faucet failed"
**Solution**: The devnet faucet might be empty or rate-limited. Wait and retry.

### Issue: "Transaction timeout"
**Solution**: Increase timeout or check devnet logs for issues

### Issue: "Insufficient credits/ACME"
**Solution**: Tests create new accounts with faucet. If faucet fails, manually fund test accounts.

## Debugging

### View Devnet Logs
```bash
# If using docker
docker logs accumulate-devnet

# If running directly
# Check the console output where accumulated run is running
```

### Query Account State
```bash
# Check account balance
accumulate -s http://localhost:26660/v3 account get acc://[account-url]/ACME

# Check credit balance
accumulate -s http://localhost:26660/v3 account get acc://[account-url]
```

### Monitor Transactions
```bash
# Get transaction status
accumulate -s http://localhost:26660/v3 tx get [txid]
```

## CI/CD Integration

For CI/CD pipelines:

1. Start devnet in the pipeline
2. Wait for devnet to be ready
3. Run tests
4. Collect logs on failure

Example GitHub Actions:
```yaml
- name: Start Devnet
  run: |
    docker-compose up -d
    sleep 30  # Wait for devnet to start
    
- name: Run Tests
  run: go test -v -timeout 5m ./loadgen/wallet/funding_credits_devnet_test.go
  
- name: Collect Logs on Failure
  if: failure()
  run: docker logs accumulate-devnet
```

## Important Notes

1. **Never use mocks** - Real bugs hide in real systems
2. **Tests are slower** - Network I/O takes time
3. **Tests may be flaky** - Network issues can cause intermittent failures
4. **Devnet resets** - Devnet state may reset between test runs
5. **Resource usage** - Running devnet consumes CPU and memory

## Future Improvements

1. Automated devnet startup/shutdown in tests
2. Better transaction status tracking
3. ADI and key page creation helpers
4. Performance profiling tools
5. Chaos testing scenarios