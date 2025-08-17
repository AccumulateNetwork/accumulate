# Streamlined Load Test Design Document

## Executive Summary
A modular load testing framework for Accumulate devnet that verifies transaction processing under load with clear accounting and debugging capabilities.

## Architecture Overview

### Core Principles
- **Modularity**: Each file has a single, clear purpose with <250 lines
- **Determinism**: All accounts derived from single timestamp seed for reproducibility  
- **Fail-Fast**: Abort early if setup fails to save debugging time
- **Clear Accounting**: Exact tracking of expected vs actual balances
- **Configurable Modes**: Debug mode for quick testing, production mode for full load

### File Organization

| File | Purpose | Key Responsibilities |
|------|---------|---------------------|
| **sl_types.go** | Core types and structures | LoadTestContext, LiteAccount, LoadConfig, constants, debug/prod mode configuration |
| **sl_accounts.go** | Account management | Account creation, funding, ACME distribution |
| **sl_credits.go** | Credits operations | Adding credits, verification, balance checking |
| **sl_settlement.go** | Settlement verification | Balance verification, transaction waiting, settlement checking |
| **sl_load.go** | Load generation | Transaction sending, TPS measurement, batch operations |
| **sl_report.go** | Reporting functions | Table generation, discrepancy analysis, issue detection |
| **sl_test.go** | Main test entry points | Test orchestration, configuration setup |
| **sl_helpers.go** | Utility functions | Client setup, endpoint discovery, oracle fetching |

## Account Architecture

### Three-Tier Account System
1. **Funding Account**: Central account that funds all test accounts
   - Generated with timestamp-based seed for uniqueness
   - Receives initial ACME from faucet
   - Pays for all account creation and credit operations

2. **Sender Accounts (k1-kN)**: Accounts that send transactions
   - Each receives exactly 100 ACME from funding account
   - Credits paid by funding account (not from sender balance)
   - Clean starting balance for easy verification

3. **Receiver Accounts (a1-aN)**: Accounts that receive transactions  
   - Start with 0 balance
   - Only receive from senders during load test
   - Simple to verify exact amounts received

## Configuration System

### Command-Line Flags
The test accepts three primary configuration flags with sensible defaults:

| Flag | Default | Description |
|------|---------|-------------|
| `-txs` | 1000 | Total number of transactions to send |
| `-k` | 10 | Number of sender (K) accounts |
| `-a` | 10 | Number of receiver (A) accounts |
| `-batch-delay` | 0ms | Delay after every 1000 transactions (0 = no delay) |

### Automatic Calculations
All other parameters are automatically calculated based on the flags:

#### ACME Requirements
- **Per K Account**: `(txs / k) * 0.001 + buffer`
  - Each transaction costs 0.001 ACME
  - Add 0.5 ACME buffer per account for safety
  - Minimum 1 ACME per K account
  
- **Total Funding Needed**: `(k * acmePerK) + (k * creditsPerK) + 10`
  - Sum of all K account ACME needs
  - Plus credits for all K accounts
  - Plus 10 ACME operational buffer

#### Credits Calculation
- **Credits per K Account**: Based on transaction count
  - 100+ txs per K: 0.1 ACME worth of credits
  - 1000+ txs per K: 0.5 ACME worth of credits
  - 5000+ txs per K: 1.0 ACME worth of credits
  
#### Settlement Timeouts
- **Base Settlement Wait**: 15 seconds
- **Progress Timeout**: 10 seconds without progress
- **Max Total Time**: Scales with transaction count
  - < 1000 txs: 3 minutes
  - < 10000 txs: 5 minutes
  - >= 10000 txs: 10 minutes

### Example Configurations

#### Default (1000 txs, 10k, 10a)
```bash
go test -v ./test/load -run TestStreamlinedLoad
# Sends 1000 txs using 10 senders to 10 receivers
# Each sender: 100 txs, needs 1 ACME + credits
# Total funding: ~25 ACME
```

#### High Volume (20000 txs, 10k, 5a)
```bash
go test -v ./test/load -run TestStreamlinedLoad -txs=20000 -k=10 -a=5
# Sends 20000 txs using 10 senders to 5 receivers
# Each sender: 2000 txs, needs 2.5 ACME + credits
# Total funding: ~40 ACME
```

#### Many Accounts (5000 txs, 50k, 20a)
```bash
go test -v ./test/load -run TestStreamlinedLoad -txs=5000 -k=50 -a=20
# Sends 5000 txs using 50 senders to 20 receivers
# Each sender: 100 txs, needs 1 ACME + credits
# Total funding: ~70 ACME
```

#### With Throttling (10000 txs with delays)
```bash
go test -v ./test/load -run TestStreamlinedLoad -txs=10000 -batch-delay=100ms
# Sends 10000 txs with 100ms delay after every 1000 txs
# Reduces peak TPS to avoid overwhelming the network
```

## Test Flow Specification

### Phase 1: Setup
1. Create unique accounts using timestamp seed
2. Fund the funding account via faucet
3. Add credits to funding account for self-transactions
4. Distribute ACME to sender accounts
5. Add credits to sender accounts (funding account pays)
6. Verify all sender accounts have expected balance

### Phase 2: Load Generation
1. Calculate transactions per sender
2. Send transactions in round-robin pattern
3. Track expected spend per sender
4. Track expected receive per receiver
5. Measure transactions per second

### Phase 3: Verification
1. Poll balances every second
2. Compare actual vs expected for all accounts
3. Exit early if all balances correct
4. Generate detailed report if discrepancies found
5. Identify specific failure patterns

## Verification Requirements

### Success Criteria
- All sender balances = initial - (txCount * txAmount) ± 0.0001 ACME
- All receiver balances = txCount * txAmount ± 0.0001 ACME
- Total ACME conserved (sum of all accounts unchanged)
- All transactions either fully processed or clearly failed

### Failure Detection Patterns
- **NOT DEBITED**: Sender balance unchanged despite accepted transactions
- **NOT CREDITED**: Receiver balance zero despite sent transactions
- **PARTIAL PROCESSING**: Some but not all transactions processed
- **ACME CREATION/DESTRUCTION**: Total ACME in system changed

## Reporting Requirements

### Sender Account Report
- Account identifier
- Expected balance after transactions
- Actual balance
- Difference
- Status indicator (✓, ⚠️ NOT DEBITED, ❌ WRONG)

### Receiver Account Report
- Account identifier  
- Expected received amount
- Actual balance
- Difference
- Status indicator (✓, ❌ MISSING, ❌ WRONG)

### Summary Statistics
- Total expected sent vs actual sent
- Total expected received vs actual received
- Sender discrepancy total
- Receiver discrepancy total
- Transaction success rate
- Measured TPS

## Implementation Guidelines

### Global Test Context Pattern
- Single LoadTestContext struct shared across all operations
- Contains client, accounts, configuration, oracle price
- Passed to all functions for stateless operation
- Enables function composition and reusability

### Error Handling Strategy
- Return errors immediately, don't continue on failure
- Log specific failure reasons with context
- Fail fast during setup to save time
- Provide clear error messages for debugging

### Deterministic Account Generation
- Base seed from timestamp hash
- Combine seed with index and prefix for each account
- Ensures unique accounts per test run
- Allows reproduction with same timestamp

## Known Issues to Handle

### Devnet Behavior Under Load
- API accepts transactions beyond processing capacity
- No backpressure or rate limiting signals
- Transactions silently dropped without error
- Need verification loop to detect dropped transactions

### Settlement Timing
- Transactions may take variable time to settle
- Need configurable retry loops with timeouts
- Must distinguish between slow and failed transactions
- Balance checking must account for partial settlement

## Testing Strategy

### Unit Testing Approach
- Test each module independently
- Mock network responses for predictable testing
- Verify calculation functions with known inputs/outputs
- Test error handling paths

### Integration Testing Approach
- Start with minimal configuration (1 sender, 1 receiver, 10 txs)
- Gradually increase load to find breaking point
- Test with both debug and production configurations
- Verify recovery after failures

### Performance Benchmarks
- Baseline TPS with single sender/receiver
- Maximum sustainable TPS before drops
- Settlement time under various loads
- Resource usage patterns

## Future Enhancements

### Adaptive Load Testing
- Start with low transaction rate
- Gradually increase until failures detected
- Find maximum sustainable TPS automatically
- Report performance curve

### Advanced Monitoring
- Track individual transaction latencies
- Monitor validator consensus during test
- Detect network partitions or failures
- Correlate failures with network events

### Recovery Testing
- Verify system recovers after overload
- Test transaction replay mechanisms
- Validate eventual consistency
- Check for permanent failures