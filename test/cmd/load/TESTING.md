# Load Generator Testing Guide

This document describes the testing approach for the Accumulate load generator.

## Test Structure

The test suite is organized into multiple files:

- `main_test.go` - Core unit tests with mocked API client
- `integration_test.go` - Dataset and infrastructure tests

## Test Coverage

### Functions Tested

#### ✅ `createAccount()` - 71.4% coverage
- Tests account creation
- Verifies uniqueness of generated accounts
- Validates account URL format
- Stress tests with 1000+ accounts

#### ✅ Transaction Logic (via mock)
The transaction logic from `initTxs()` is tested through a mock implementation (`testInitTxs()`) that:
- Tests concurrent transaction execution
- Verifies faucet and QueryTx call patterns
- Tests error handling for API failures
- Validates transaction count tracking
- Tests rate limiting behavior

#### ✅ Client Structure
- Tests Client struct initialization
- Validates field access and modification
- Tests multiple client instances
- Verifies client ID uniqueness

#### ✅ Dataset Operations
- Tests DataSet initialization
- Validates concurrent dataset access
- Tests saving transaction metrics
- Verifies multiple dataset instances

### Functions Requiring Live Server

The following functions require a running Accumulate API server and are not unit-testable:

- `main()` - Entry point, tested via manual/integration testing
- `initTxs()` - Requires real API client (tested via mock wrapper)
- `initializeClients()` - Requires network describe call

## Running Tests

### Run all tests
```bash
go test -v
```

### Run with coverage
```bash
go test -cover -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Run benchmarks
```bash
go test -bench=. -benchmem
```

### Skip long-running tests
```bash
go test -short
```

## Mock Patterns

### MockClient

The `MockClient` type implements the API client interface for testing:

```go
mock := &MockClient{
    FaucetFunc: func(ctx context.Context, req *protocol.AcmeFaucet) (*api.TxResponse, error) {
        // Custom implementation
        return &api.TxResponse{...}, nil
    },
}
```

### Features:
- Thread-safe call counting
- Customizable behavior via function fields
- Default implementations for all methods
- Call count tracking for verification

## Test Examples

### Testing Transaction Generation

```go
func TestInitTxsBasic(t *testing.T) {
    mock := &MockClient{}
    ds := createTestDataSet()

    client := &Client{
        DataSet: ds,
        Client:  nil,
        Id:      0,
        TxCount: 0,
    }

    err := testInitTxs(0.0, 5, client, mock)
    require.NoError(t, err)
    require.Equal(t, 5, client.TxCount)
}
```

### Testing Concurrency

```go
func TestInitTxsConcurrency(t *testing.T) {
    mock := &MockClient{
        FaucetFunc: func(ctx context.Context, req *protocol.AcmeFaucet) (*api.TxResponse, error) {
            time.Sleep(10 * time.Millisecond)
            return &api.TxResponse{...}, nil
        },
    }

    start := time.Now()
    err := testInitTxs(0.0, 10, client, mock)
    elapsed := time.Since(start)

    // Should complete much faster than sequential execution
    require.Less(t, elapsed, 100*time.Millisecond)
}
```

### Testing Error Handling

```go
func TestInitTxsErrorHandling(t *testing.T) {
    mock := &MockClient{
        FaucetFunc: func(ctx context.Context, req *protocol.AcmeFaucet) (*api.TxResponse, error) {
            return nil, errors.New("faucet error")
        },
    }

    err := testInitTxs(0.0, 3, client, mock)
    require.NoError(t, err) // Should not propagate error
    require.Equal(t, 0, client.TxCount) // Should not increment on failure
}
```

## Benchmarks

### BenchmarkCreateAccount
Tests the performance of ED25519 key generation and account creation.

### BenchmarkInitTxs
Tests the overhead of transaction initialization with mock client.

## Integration Testing

For full integration testing with a live devnet:

```bash
# Start a local devnet
devnet start

# Run the load generator
go run main.go -s http://127.0.1.1:26660/v2 -t 10 -d 5 -r 5

# Monitor results in ./load_tester/
```

## Test Coverage Goals

- ✅ Account generation: >70%
- ✅ Mock client patterns: 100%
- ✅ Dataset operations: Full coverage via integration tests
- ⚠️ Network-dependent functions: Require integration testing

## Known Limitations

1. **initTxs() not directly tested**: This function requires a real API client. We test equivalent logic through `testInitTxs()` with mocks.

2. **initializeClients() not tested**: This function calls `Describe()` which requires network access. The client initialization logic is tested indirectly.

3. **main() not tested**: Entry point tested via manual execution.

## Future Improvements

1. Add HTTP mock server for end-to-end testing
2. Create integration test suite with dockerized devnet
3. Add property-based testing for transaction generation
4. Add chaos testing for error scenarios
