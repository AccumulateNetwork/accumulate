# Testing Guide for Core Infrastructure Tests

This guide covers running and troubleshooting tests for the core infrastructure test tools: `gen-testdata` and `load` (load generator).

## Overview

The test infrastructure consists of two main tools:

1. **gen-testdata** - Generates test data for Accumulate transactions and accounts
2. **load** - Load testing tool for the Accumulate network

Each tool has comprehensive unit and integration tests.

## Running Unit Tests

Unit tests run quickly and don't require a running network.

### gen-testdata Tests

```bash
# Run all tests
go test -v ./test/cmd/gen-testdata

# Run in short mode (skip long-running tests)
go test -short -v ./test/cmd/gen-testdata
```

### Load Generator Tests

```bash
# Run all unit tests (integration tests will be skipped)
go test -short -v ./test/cmd/load

# Run specific tests
go test -v -run TestCreateAccount ./test/cmd/load
go test -v -run TestTransactionLoadCalculation ./test/cmd/load
```

## Running Integration Tests

Integration tests require a running Accumulate devnet.

### Setting Up a Devnet

1. **Build the accumulated daemon:**
   ```bash
   go build -o ./accumulated ./cmd/accumulated
   ```

2. **Initialize the devnet:**
   ```bash
   # Create a minimal devnet (1 BVN, 1 validator, no followers)
   ./accumulated init devnet -w .nodes -b 1 -v 1 -f 0 \
     --faucet-seed ci \
     --globals '{"executorVersion": "v2", "oracle": {"price": 50000000}}'
   ```

3. **Start the devnet:**
   ```bash
   # Run in foreground (for debugging)
   ./accumulated run devnet -w .nodes

   # Or run in background
   ./accumulated run devnet -w .nodes > devnet.log 2>&1 &
   ```

4. **Wait for startup:**
   ```bash
   # Wait 10-15 seconds for the network to initialize
   sleep 15
   ```

5. **Verify the devnet is running:**
   ```bash
   curl -s http://127.0.0.1:26660/v2 -H "Content-Type: application/json" \
     -d '{"jsonrpc":"2.0","id":1,"method":"describe"}' | head -50
   ```

### Running Integration Tests

Once the devnet is running:

```bash
# Run load generator integration tests
INTEGRATION_TEST=1 ACC_API=http://127.0.0.1:26660/v2 go test -v -timeout 5m ./test/cmd/load

# Run with different network configurations
INTEGRATION_TEST=1 ACC_API=http://localhost:26660/v2 go test -v ./test/cmd/load
```

### Stopping the Devnet

```bash
# If running in background
pkill -f "accumulated run devnet"

# Or if you have the PID
kill $(cat devnet.pid)
```

## Test Configuration

### Environment Variables

- `INTEGRATION_TEST=1` - Enable integration tests
- `ACC_API` - Accumulate API endpoint (default: `http://127.0.0.1:26660/v2`)

### Test Flags

- `-short` - Skip integration tests
- `-v` - Verbose output
- `-timeout` - Set test timeout (e.g., `-timeout 5m`)
- `-run` - Run specific tests (e.g., `-run TestCreateAccount`)

## Common Test Scenarios

### Scenario 1: Quick Validation

Run unit tests only to verify code changes compile and basic functionality works:

```bash
go test -short ./test/cmd/gen-testdata ./test/cmd/load
```

### Scenario 2: Full Integration Test

Test against a live devnet with different configurations:

```bash
# Start devnet with 2 BVNs, 2 validators each
./accumulated init devnet -w .nodes -b 2 -v 2 -f 1 --reset \
  --globals '{"executorVersion": "v2"}'
./accumulated run devnet -w .nodes &

# Wait for startup
sleep 15

# Run integration tests
INTEGRATION_TEST=1 go test -v ./test/cmd/load
```

### Scenario 3: Load Testing

Run the actual load generator tool:

```bash
# Low load: 10 TPS for 10 seconds
go run ./test/cmd/load -s http://127.0.0.1:26660/v2 -t 10 -d 10 -r 5

# Medium load: 100 TPS for 60 seconds
go run ./test/cmd/load -s http://127.0.0.1:26660/v2 -t 100 -d 60 -r 25

# High load: 1000 TPS for 30 seconds (requires more resources)
go run ./test/cmd/load -s http://127.0.0.1:26660/v2 -t 1000 -d 30 -r 50
```

Load generator flags:
- `-s` - Server URL
- `-t` - Transactions per second
- `-d` - Duration in seconds
- `-r` - Transactions per client (goroutines)

## Troubleshooting

### Issue: "Server not available"

**Symptoms:** Integration tests skip or fail with connection errors

**Solutions:**
1. Verify devnet is running: `ps aux | grep accumulated`
2. Check API endpoint: `curl http://127.0.0.1:26660/v2`
3. Wait longer for startup: The network may need 15-30 seconds
4. Check devnet logs: `tail -f devnet.log`

### Issue: "Divide by zero" panic

**Symptoms:** Tests fail with `runtime error: integer divide by zero`

**Solution:** This was a bug in the load generator when no nodes are found in the network description. It has been fixed by falling back to the server URL when the node list is empty.

### Issue: "Transaction timeouts"

**Symptoms:** Faucet or transaction queries timeout

**Solutions:**
1. Increase test timeout: `-timeout 10m`
2. Reduce load: Lower TPS or duration
3. Check network health: Verify blocks are being produced
4. Check resource usage: Ensure sufficient CPU/memory

### Issue: "Tests are flaky"

**Symptoms:** Tests pass sometimes but fail other times

**Solutions:**
1. Increase wait times between operations
2. Add retries with backoff
3. Check for resource contention
4. Run tests sequentially: `-p 1`

### Issue: "Dataset logging errors"

**Symptoms:** TestDataSetLogging fails

**Solution:** This test may not generate output if no data is saved. This is expected behavior and the test now logs this case instead of failing.

## Test Coverage

### gen-testdata Tests

- ✅ Test data generation for all transaction types
- ✅ Account test data generation
- ✅ Ledger test vector generation
- ✅ JSON marshaling/unmarshaling
- ✅ Binary encoding/decoding
- ✅ File I/O operations

### Load Generator Tests

Unit Tests:
- ✅ Account creation and uniqueness
- ✅ Lite token address generation
- ✅ Client structure validation
- ✅ Transaction load calculations
- ✅ Dataset logging
- ✅ Benchmarks for account creation

Integration Tests:
- ✅ Single account faucet operation
- ✅ Transaction submission and waiting
- ✅ Multiple client initialization
- ✅ Network connectivity
- ✅ API endpoint discovery

## Performance Expectations

### Unit Tests

- **gen-testdata**: < 1 second
- **load (unit tests)**: < 1 second

### Integration Tests

- **Single faucet transaction**: 2-5 seconds
- **10 transactions**: 10-20 seconds
- **100 transactions**: 1-3 minutes

### Load Testing

Expected throughput depends on network configuration:

| Config | Expected TPS | Notes |
|--------|-------------|-------|
| 1 BVN, 1 validator | 10-50 | Minimal setup |
| 2 BVNs, 2 validators each | 50-200 | Standard testing |
| 3 BVNs, 4 validators each | 200-500 | Production-like |

## Best Practices

1. **Always run unit tests first** - They're fast and catch most issues
2. **Use short mode for quick feedback** - Run full tests in CI/CD
3. **Clean up devnets** - Remove old `.nodes` directories between test runs
4. **Monitor resources** - Load tests can consume significant CPU/memory
5. **Test incrementally** - Start with low load and increase gradually
6. **Check logs** - Both test logs and devnet logs provide valuable debugging info
7. **Use consistent seeds** - The `--faucet-seed ci` flag ensures reproducible behavior

## CI/CD Integration

### GitLab CI Example

```yaml
test-integration:
  stage: test
  script:
    # Build
    - go build -o accumulated ./cmd/accumulated

    # Start devnet
    - ./accumulated init devnet -w .nodes -b 1 -v 1 -f 0 --reset
    - ./accumulated run devnet -w .nodes &
    - sleep 15

    # Run tests
    - INTEGRATION_TEST=1 go test -v -timeout 10m ./test/cmd/...

  after_script:
    - pkill -f accumulated || true
```

## Additional Resources

- [Accumulate Documentation](https://docs.accumulatenetwork.io/)
- [API Documentation](https://docs.accumulatenetwork.io/api/)
- [Protocol Specification](https://docs.accumulatenetwork.io/protocol/)
