# Testing Design Document - Funding Credits

## Core Testing Philosophy

**ALL TESTS MUST USE THE ACTUAL DEVNET**

### Rationale
Mock tests and simulations have repeatedly failed to catch real issues in our distributed system. The complexity of Accumulate's crosschain interactions, consensus mechanisms, and state management cannot be adequately simulated with mocks.

### Requirements

1. **Mandatory Devnet Testing**
   - All tests MUST connect to a running devnet instance
   - Tests MUST perform actual transactions on the devnet
   - Tests MUST verify actual state changes on the blockchain
   - NO mock objects for network interactions

2. **Test Categories**
   - **Unit Tests**: Even unit tests must use devnet for any network operations
   - **Integration Tests**: Full end-to-end flows using devnet
   - **Load Tests**: Stress testing against devnet

3. **Devnet Configuration**
   - Default endpoint: `http://localhost:26660/v3`
   - Timeout: 30 seconds for operations
   - Retry logic for transient failures

4. **Test Setup Requirements**
   ```bash
   # Before running tests:
   ./devnet_manager.sh start
   
   # Verify devnet is running:
   curl http://localhost:26660/v3/status
   ```

5. **Known Limitations**
   - Tests will be slower (network I/O)
   - Tests require devnet to be running
   - Tests may fail due to network issues
   - Tests consume actual credits/ACME

6. **Benefits**
   - Catches real protocol issues
   - Validates actual transaction flow
   - Tests real consensus behavior
   - Verifies actual state transitions
   - Discovers timing/race conditions
   - Tests actual error paths

## Future Considerations

This requirement may be relaxed in the future once:
- The protocol stabilizes
- We have comprehensive devnet coverage
- We develop high-fidelity simulation tools

For now, the cost of false confidence from mock tests far exceeds the inconvenience of requiring devnet.

## Implementation Guidelines

### Test Structure
```go
func TestCreditManager_TopUpLiteAccount_Devnet(t *testing.T) {
    // Skip if devnet not available
    if !isDevnetAvailable() {
        t.Skip("Devnet not available")
    }
    
    // Create real client
    client := jsonrpc.NewClient(devnetAPI)
    
    // Create real accounts with faucet
    fundingAccount := createRealAccount(t, client)
    targetAccount := createRealAccount(t, client)
    
    // Perform actual operations
    cm := NewCreditManager(client, client, realSigner, fundingAccount)
    err := cm.TopUpLiteAccount(ctx, targetAccount)
    
    // Verify actual blockchain state
    verifyCreditsOnChain(t, client, targetAccount)
}
```

### Common Test Helpers
- `setupDevnetAccount()`: Creates real account with faucet funding
- `waitForTransaction()`: Polls for transaction completion
- `verifyOnChainState()`: Queries actual blockchain state
- `cleanupTestAccounts()`: Optional cleanup (devnet resets anyway)

## Test Execution

```bash
# Run all tests (requires devnet)
go test -v ./...

# Run with extended timeouts for slow devnet
go test -v -timeout 5m ./...

# Run specific test suite
go test -v -run TestCreditManager ./funding_credits_test.go
```

## Debugging Failed Tests

1. Check devnet logs: `docker logs accumulate-devnet`
2. Verify account state: `accumulate account get <url>`
3. Check transaction status: `accumulate tx get <txid>`
4. Monitor network partition states
5. Review crosschain conductor logs

## Migration Path

When migrating existing mock-based tests:
1. Replace mock clients with real jsonrpc.Client
2. Create real test accounts using faucet
3. Replace mock expectations with actual state verification
4. Add retry logic for network operations
5. Add proper cleanup/teardown

---

**Remember**: Real bugs hide in real systems. Test against the real thing.