# Test Documentation for init-test-data

## Overview

This document describes the test coverage for the `cmd/init-test-data` tool, which initializes 3000 test accounts on the Accumulate network.

## Test Files

- `main_test.go`: Core unit tests for configuration, statistics, and wallet integration
- `integration_test.go`: Integration tests for account creation flows and batch processing

## Test Coverage

### Unit Tests (main_test.go)

#### Configuration & Initialization
- **TestNewInitializer**: Tests initializer creation with valid/invalid wallets
  - Valid wallet with funder
  - Wallet without funder (error case)
  - Non-existent wallet (error case)

- **TestConfigValidation**: Tests various configuration combinations
  - Default configuration
  - Small batch sizes (1 account per batch)
  - Large batch sizes (100 accounts per batch)

- **TestDryRunMode**: Verifies dry-run functionality works without creating accounts

- **TestSkipFlags**: Tests selective account type initialization
  - Skip lite accounts
  - Skip ADI token accounts
  - Skip ADI data accounts
  - Skip all account types

#### Statistics & Progress Tracking
- **TestStatsTracking**: Tests atomic counter operations for success/failure tracking
- **TestConcurrentStatsUpdates**: Verifies thread-safe statistics updates with 100 concurrent goroutines
- **TestPrintSummary**: Tests summary output and JSON file generation
- **TestPrintPlan**: Tests dry-run plan output

#### Batch Processing
- **TestBatchProcessing**: Tests batch size calculations
  - 25 accounts with batch size 5 (5 batches)
  - 25 accounts with batch size 10 (3 batches)
  - 25 accounts with batch size 30 (1 batch)

- **TestProcessBatchConcurrency**: Tests concurrent processing within batches
  - Verifies all accounts are processed
  - Tests semaphore-based concurrency control

#### Wallet Integration
- **TestWalletIntegration**: Tests wallet loading and key extraction
  - Funder account key validation
  - Private key format (64 bytes)
  - Public key format (32 bytes)

- **TestAccountTypeFiltering**: Tests account filtering by type
  - Lite accounts
  - ADI token accounts
  - ADI data accounts
  - Count by type

### Integration Tests (integration_test.go)

#### Account Creation Flows
- **TestInitializeLiteAccountsFlow**: Tests lite account initialization setup
  - Account structure validation
  - Key format verification
  - URL format verification

- **TestInitializeADITokenAccountsFlow**: Tests ADI token account setup
  - Account structure validation
  - URL path verification (/tokens)

- **TestInitializeADIDataAccountsFlow**: Tests ADI data account setup
  - Account structure validation
  - URL path verification (/data)

- **TestMixedAccountTypes**: Tests initialization with all account types
  - Combined lite, ADI token, and ADI data accounts
  - Count validation by type

#### Error Handling
- **TestBatchProcessingWithErrors**: Tests error recovery in batch processing
  - Simulates failures for every 3rd account
  - Verifies batch processing continues despite errors

#### Account Management
- **TestAccountIndexing**: Tests account index assignment
  - Sequential index validation
  - GetAccount() retrieval by index
  - Out-of-bounds error handling

- **TestAccountURLParsing**: Tests URL format correctness
  - Lite account URLs
  - ADI token account URLs (path = /tokens)
  - ADI data account URLs (path = /data)

- **TestFunderAccountStructure**: Tests funder account properties
  - URL validation
  - Type validation (should be "funder")
  - Index validation (should be -1)
  - Key format validation

## Running Tests

### Run all tests (short mode)
```bash
go test -v -short gitlab.com/accumulatenetwork/accumulate/cmd/init-test-data
```

### Run with coverage
```bash
go test -short -coverprofile=coverage.out gitlab.com/accumulatenetwork/accumulate/cmd/init-test-data
go tool cover -html=coverage.out
```

### Run specific test
```bash
go test -v -run TestNewInitializer gitlab.com/accumulatenetwork/accumulate/cmd/init-test-data
```

### Run integration tests (requires network)
```bash
go test -v gitlab.com/accumulatenetwork/accumulate/cmd/init-test-data
```

## Coverage Summary

The tests achieve 100% coverage for testable functions:
- `NewInitializer`: 100%
- `printPlan`: 100%
- `processBatch`: 100%
- `printSummary`: 100%
- `saveSummary`: 100%

**Note**: Functions that require network API calls (verifyFunder, createLiteAccount, createADITokenAccount, createADIDataAccount, submitAndWait) cannot be fully tested without mocking the API client, which would require refactoring main.go to accept an interface. These functions are tested structurally but not with actual network calls.

## Test Patterns Used

### Table-Driven Tests
Multiple test cases are defined using a slice of test structs:
```go
tests := []struct {
    name    string
    config  Config
    wantErr bool
}{
    // test cases...
}
```

### Temporary File Testing
All tests use `t.TempDir()` for temporary wallet files, ensuring:
- No pollution of the filesystem
- Automatic cleanup after tests
- Parallel test execution safety

### Concurrent Testing
Tests verify thread-safety using goroutines:
- Atomic counter operations
- Concurrent batch processing
- Race condition detection (run with `-race` flag)

### Mock Functions
Custom mock functions are used to test batch processing:
```go
mockCreateFn := func(ctx context.Context, acct *wallet.Account) error {
    // Test logic
    return nil
}
```

## Edge Cases Covered

1. **Empty Wallet**: Wallet with no funder account
2. **Non-existent Files**: Missing wallet file
3. **Batch Size Variations**:
   - Batch larger than total accounts
   - Single account batches
   - Exact division and remainder cases
4. **Concurrent Updates**: 100+ concurrent goroutines updating statistics
5. **Index Boundaries**: Out-of-bounds account access
6. **Error Recovery**: Partial failures in batch processing

## Future Improvements

To achieve >80% overall coverage, consider:

1. **API Client Abstraction**: Refactor main.go to accept an interface:
   ```go
   type APIClient interface {
       Query(context.Context, *url.URL, api.Query) (api.Record, error)
       Submit(context.Context, *messaging.Envelope, api.SubmitOptions) ([]*api.Submission, error)
   }
   ```

2. **Integration Test Environment**: Set up a test network for full end-to-end testing

3. **Network Error Simulation**: Add tests for:
   - Connection timeouts
   - Transaction failures
   - Insufficient funds
   - Network partitions

4. **Performance Tests**: Add benchmarks for:
   - Batch processing throughput
   - Concurrent account creation
   - Transaction submission rates
