# Long-Running Load Test Design

## CRITICAL REQUIREMENT: NO SIMULATIONS

**ALL TRANSACTIONS MUST BE REAL**
- ❌ NO simulated transactions
- ❌ NO mock transactions  
- ❌ NO fake success/failure rates
- ✅ ALL transactions are sent to the actual devnet
- ✅ ALL results are verified using devnet APIs
- ✅ ALL metrics are based on real network behavior

## Overview
This document describes the design for long-running load tests that provide progressive metrics and performance analysis over extended periods using REAL DEVNET TRANSACTIONS.

## Architecture

### Transaction Flow
1. **Account Setup**: Create real accounts on devnet with actual ACME tokens
2. **Transaction Submission**: Send real transactions to devnet nodes
3. **Verification**: Query devnet APIs to verify transaction settlement
4. **Metrics Collection**: Measure actual network performance

### NO SIMULATIONS Policy
Every test MUST:
- Connect to a running devnet instance
- Create real accounts with real keys
- Fund accounts with actual ACME tokens from faucet
- Send real transactions between accounts
- Verify balances using devnet API queries
- Measure actual network latency and throughput

## Key Features

### 1. Progressive Reporting with Real Data
- Reports are generated at configurable intervals (default: every 10,000 transactions)
- Each report includes:
  - ACTUAL transaction counts from devnet responses
  - REAL success/failure rates from network
  - MEASURED send and settlement durations
  - TRUE TPS metrics based on confirmed transactions
  - ACTUAL cumulative lag measurements
  - Progress percentage of real transactions

### 2. Command-Line Flags
```bash
# Run 100k REAL transactions with reports every 10k
go test -v -run TestProgressiveLoad -args \
  -total-txs=100000 \
  -report-interval=10000 \
  -batch-size=1000 \
  -batch-delay-ms=1000 \
  -settle-time-ms=5000
```

#### Available Flags:
- `-total-txs`: Total number of REAL transactions to send
- `-report-interval`: REAL transactions between reports
- `-batch-size`: REAL transactions per batch
- `-batch-delay-ms`: Delay between batches (for rate limiting)
- `-settle-time-ms`: Time to wait for REAL settlement verification

### 3. Real Metrics Calculation

#### Transaction Rate (TPS) - Based on Actual Network Performance
- **Send TPS**: Successfully submitted transactions / actual send duration
- **End-to-end TPS**: Confirmed transactions / (send + actual settlement duration)

#### Lag Calculation - Based on Real Network Behavior
- **Expected baseline**: Network's theoretical maximum throughput
- **Actual time**: Real measured time from devnet
- **Lag**: Actual network time - Expected baseline time
- **Cumulative lag**: Sum of all real segment lags

### 4. Real Performance Analysis

#### Per-Segment Metrics (From Actual Devnet)
Each segment tracks REAL:
- Transaction submission timestamps
- Network response times
- Actual success/failure from devnet
- Real settlement confirmation times
- Actual TPS rates from network
- Cumulative network lag

#### Final Summary (All Real Data)
After all transactions complete:
- Total REAL transaction statistics
- Overall TPS from ACTUAL network performance
- Average performance metrics from REAL data
- TPS trend analysis from ACTUAL measurements
- Lag analysis from REAL network behavior

### 5. Devnet Integration Requirements

#### Pre-test Setup
1. Ensure devnet is running and healthy
2. Verify faucet has sufficient ACME
3. Check network connectivity to all nodes
4. Confirm API endpoints are responsive

#### During Test Execution
1. Create real accounts with proper keys
2. Fund accounts from faucet with real ACME
3. Send actual transactions to devnet
4. Query devnet APIs for transaction status
5. Verify account balances via API
6. Monitor network health metrics

#### Post-test Verification
1. Query final account balances
2. Verify all transactions settled correctly
3. Check for any network errors or issues
4. Validate metrics against devnet logs

## Implementation Requirements

### MANDATORY: Real Transaction Implementation
```go
// CORRECT: Real transaction to devnet
func SendTransaction(from, to Account, amount int64) error {
    // Build real transaction
    tx := BuildRealTransaction(from, to, amount)
    
    // Send to actual devnet node
    response := devnetClient.Submit(tx)
    
    // Check real response
    if response.Error != nil {
        return response.Error
    }
    
    // Return actual result
    return nil
}

// WRONG: Simulated transaction
func SimulatedTransaction() error {
    // NEVER DO THIS
    if rand.Float() < 0.66 {
        return nil // Fake success
    }
    return errors.New("simulated failure")
}
```

### Settlement Verification
```go
// CORRECT: Query real devnet for settlement
func VerifySettlement(txID string) bool {
    // Query actual devnet API
    status := devnetClient.GetTransactionStatus(txID)
    
    // Check real settlement status
    return status.State == "settled"
}

// WRONG: Simulated settlement
func SimulatedSettlement() bool {
    time.Sleep(5 * time.Second) // NEVER DO THIS
    return true // Fake settlement
}
```

## Test Validation Checklist

Before running any load test, verify:
- [ ] Devnet is running (`./accumulated run devnet`)
- [ ] All transactions use real devnet client
- [ ] No hardcoded success/failure rates
- [ ] No `time.Sleep()` for simulating settlement
- [ ] All metrics come from actual API responses
- [ ] Account balances are verified via devnet queries
- [ ] Transaction IDs are real and can be queried
- [ ] Network errors are actual devnet errors

## Benefits of Real Testing

1. **Accurate Performance Metrics**: Real network behavior, not simulations
2. **True Bottleneck Discovery**: Find actual system limitations
3. **Valid Stress Testing**: Test real network capacity
4. **Meaningful Results**: Data that reflects actual system performance
5. **Problem Detection**: Discover real issues, not simulated ones

## Common Mistakes to Avoid

❌ **NEVER**: Use random number generators for success/failure
❌ **NEVER**: Sleep to simulate network delay
❌ **NEVER**: Return hardcoded transaction results
❌ **NEVER**: Calculate metrics from simulated data
❌ **NEVER**: Skip actual balance verification
❌ **NEVER**: Assume transaction success without checking

✅ **ALWAYS**: Send real transactions to devnet
✅ **ALWAYS**: Query actual API endpoints
✅ **ALWAYS**: Verify real account balances
✅ **ALWAYS**: Measure actual network timing
✅ **ALWAYS**: Report real success/failure rates
✅ **ALWAYS**: Use actual settlement confirmation

## Debugging Real Tests

When tests fail with real transactions:
1. Check devnet logs for errors
2. Verify account funding succeeded
3. Confirm network connectivity
4. Check transaction signatures
5. Verify API endpoint availability
6. Review actual error messages from devnet
7. Examine account state on devnet

## Future Enhancements

All enhancements must maintain REAL transaction policy:
1. **Multi-node Testing**: Send to different devnet nodes
2. **Partition Testing**: Test with network partitions
3. **Recovery Testing**: Test transaction recovery scenarios
4. **Load Distribution**: Spread load across multiple nodes
5. **Performance Profiling**: Profile actual devnet performance
6. **Network Monitoring**: Real-time devnet health metrics