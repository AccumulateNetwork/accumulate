# Load Generator Tests

This directory contains comprehensive tests for the Accumulate load generator tool.

## Overview

The load generator (`main.go`) is a performance testing tool that generates transaction load against an Accumulate network. It creates lite token accounts and submits faucet transactions at a configurable rate.

## Test Coverage

The test suite includes:

### Unit Tests (`main_test.go`)
- **Account Creation**: Tests ED25519 key generation and lite token address creation
- **Transaction Initialization**: Tests concurrent transaction execution with mocks
- **Error Handling**: Tests faucet and QueryTx error scenarios
- **Rate Limiting**: Tests TPS control and timing accuracy
- **Worker Coordination**: Tests concurrent worker behavior
- **Metrics Collection**: Tests transaction counting and timing
- **Mock Client**: Thread-safe mock for API testing

### Integration Tests (`integration_test.go`)
- **DataSet Management**: Tests logging and data collection
- **Client Management**: Tests multiple client instances
- **Stress Testing**: Tests account creation under load
- **Configuration**: Tests flag defaults and validation

## Running Tests

```bash
# Run all tests
go test -v

# Run tests with coverage
go test -cover -coverprofile=coverage.out

# View coverage report
go tool cover -html=coverage.out

# Run short tests (skips slow rate limiting test)
go test -short

# Run benchmarks
go test -bench=. -benchmem
```

## Test Structure

### MockClient

The `MockClient` implements the `ClientInterface` for testing:

```go
type MockClient struct {
    FaucetFunc     func(context.Context, *protocol.AcmeFaucet) (*api.TxResponse, error)
    QueryTxFunc    func(context.Context, *api.TxnQuery) (*api.TransactionQueryResponse, error)
    DescribeFunc   func(context.Context) (*api.DescriptionResponse, error)
    // Thread-safe call counters
}
```

Features:
- Configurable behavior via function fields
- Thread-safe call tracking
- Error injection for testing failure paths

### Test Helpers

- `createTestDataSet()`: Creates a DataSet for testing
- `GetFaucetCalls()`: Thread-safe call counter
- `GetQueryTxCalls()`: Thread-safe call counter

## Coverage Report

Current test coverage: **42%** of statements

Covered functions:
- `init()`: 100%
- `initTxs()`: 100% (core transaction logic)
- `createAccount()`: 71.4%

Uncovered functions:
- `main()`: 0% (CLI entry point, tested via integration)
- `initializeClients()`: 0% (network initialization, tested via integration)

## Key Test Cases

### Transaction Execution
- Basic faucet transaction flow
- Concurrent transaction processing (10 parallel)
- Error handling for faucet failures
- Error handling for QueryTx failures
- Transaction hash propagation

### Rate Limiting
- Tests 1-second burst intervals
- Verifies timing accuracy (±200ms tolerance)
- Tests multiple sequential bursts

### Concurrency
- Tests WaitGroup coordination
- Tests mutex-protected state updates
- Tests parallel worker execution
- Tests 100 concurrent mock client calls

### Metrics
- Transaction count tracking
- Settlement time measurement
- Dataset logging and saving
- Client ID uniqueness

## Performance Benchmarks

```bash
$ go test -bench=. -benchmem
BenchmarkCreateAccount-24    107150    11129 ns/op    1313 B/op    30 allocs/op
BenchmarkInitTxs-24           9105    118673 ns/op   19579 B/op   384 allocs/op
```

## Testing Best Practices

1. **Use Mocks**: The `MockClient` allows testing without network access
2. **Test Concurrency**: Use goroutines to verify parallel behavior
3. **Test Errors**: Inject errors to verify error handling
4. **Measure Timing**: Use `time.Since()` for rate limiting tests
5. **Use Subtests**: Group related test cases with `t.Run()`

## Future Improvements

Potential areas for expanded testing:
- Integration tests with a real devnet
- Stress testing with higher concurrency levels
- Memory profiling and leak detection
- Network failure simulation
- Timeout and retry logic
