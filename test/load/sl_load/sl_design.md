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
   - **FIXED account** - not generated, uses a predetermined address and key
   - **MUST print** the funding account address and private key at test start
   - Receives ALL faucet calls (consolidates funding in one place)
   - **Balance checking and top-off pattern**:
     - Check current balance before funding
     - Calculate difference needed to reach 110% of requirements
     - Only request the top-off amount from faucet
     - Validate balance settled to target after faucet
   - **Credits top-off pattern**:
     - Check current credits before adding
     - Calculate difference needed to reach 110% of requirements
     - Only add the top-off amount of credits
     - Validate credits settled to target after adding
   - **Distribution**: Must distribute 10% MORE than computed requirements
     - Give each K account 110% of computed ACME needs
     - Add 110% of computed credits to each K account
     - Validate each distribution settles correctly
   - Pays for all account creation, funding, and credits
   - **NO hardcoded values** except 0.001 ACME per transaction

2. **Sender Accounts (k1-kN)**: Accounts that send transactions
   - Each receives 110% of calculated ACME needed for their token transactions
   - Credits added by funding account but paid FROM K account balance
   - Credits in K accounts pay for the load generation transactions
   - Generated fresh each test run to ensure zero starting balance
   - **NO hardcoded ACME amounts** - all dynamically calculated

3. **Receiver Accounts (a1-aN)**: Accounts that receive transactions  
   - Start with 0 balance
   - Only receive from senders during load test
   - Simple to verify exact amounts received

## Configuration System

### Command-Line Flags
The test uses a SINGLE test function (`TestStreamlinedLoad`) controlled entirely by command-line flags:

| Flag | Default | Description |
|------|---------|-------------|
| `-txs` | 1000 | Total number of transactions to send |
| `-k` | 10 | Number of sender (K) accounts |
| `-a` | 10 | Number of receiver (A) accounts |
| `-tps` | 0 | Target transactions per second (0 = unlimited) |
| `-timeout` | 1min | Settlement timeout (resets on progress, max 1 minute) |
| `-verbose` | false | Enable verbose logging for debugging |

### Automatic Calculations
All other parameters are automatically calculated based on the flags:

#### ACME Requirements
- **Total ACME Calculation**: 
  - `total_txs_acme = txs * 0.001` (each tx costs 0.001 ACME - ONLY hardcoded value)
  - Apply 10% buffer: `total_txs_acme * 1.1`
  - Calculate credits needed in ACME (dynamically based on tx count)
  - `total_acme_needed = (total_txs_acme * 1.1) + (credits_acme * 1.1)`
  
- **Faucet Funding Requirements**: 
  - Faucet gives 10 ACME per call (fixed amount)
  - Calculate: `faucet_calls = CEIL(total_acme_needed / 10)`
  - Top-off pattern: Check existing balance first, only request difference
  - Must have full amount BEFORE any distribution begins
  
- **Distribution to K Accounts**:
  - `txs_per_k = txs / k` (transactions each K account will send)
  - `acme_per_k = (txs_per_k * 0.001) * 1.1` (with 10% buffer)
  - Each K gets: `acme_per_k` from funding account
  - Credits: Added by funding account, but ACME deducted from K account balance
  - Credits in K accounts will pay for all load generation transactions

#### Credits Calculation
- **Credits per K Account**: Dynamically calculated based on transaction count
  - Calculate credits needed for expected transactions
  - NO hardcoded credit amounts
  - Apply 10% buffer: `computed_credits * 1.1`
  - Funding account must have 110% of total credits needed
  - Distribute 110% of computed credits to each K account

#### Credit Display Rules
**CRITICAL**: Never display internal credit representations to users!

- **Internal Representation**: Credits are stored as fixed-point integers with `CreditPrecision = 100`
  - 100 internal units = 1.00 credit
  - 1 internal unit = 0.01 credit (smallest unit)
  
- **Display Format**: Always divide by `CreditPrecision` and show with 2 decimal places
  - Internal: 110000 → Display: "1100.00 credits"
  - Internal: 100 → Display: "1.00 credit"
  - Internal: 1 → Display: "0.01 credit"

- **Example Code**:
  ```go
  // WRONG - Shows internal representation
  fmt.Printf("Credits: %d\n", creditBalance)  // Shows: Credits: 110000
  
  // CORRECT - Shows proper decimal representation
  fmt.Printf("Credits: %.2f\n", float64(creditBalance)/protocol.CreditPrecision)  // Shows: Credits: 1100.00
  ```

- **Common Mistakes**:
  - Showing raw integer values (110000 instead of 1100.00)
  - Confusing credit precision with ACME precision (they are different!)
  - Mixing internal and display values in calculations

**CAUTION**: The precision used for credits (in credits units) is different from ACME precision. Credits use their own fixed-point representation (CreditPrecision = 100), not the same as ACME's decimal precision (1e8). Do not mix credit amounts with ACME amounts directly.

#### Proper Credit Calculations

**CRITICAL**: Always use the oracle price and proper conversion formulas. Never use rough estimates!

##### Key Constants
```go
// From protocol/protocol.go
AcmePrecision = 1e8              // 100,000,000 (8 decimal places)
CreditPrecision = 1e2            // 100 (2 decimal places)
CreditsPerDollar = 1e2           // 100 credits per dollar (external units)
CreditUnitsPerFiatUnit = 1e4     // 10,000 (CreditsPerDollar * CreditPrecision)
AcmeOraclePrecision = 1e4        // Oracle price precision
```

##### Conversion Formula: ACME to Credits
```go
// CORRECT way to calculate credits from ACME amount
func CalculateCredits(acmeAmount int64, oraclePrice uint64) uint64 {
    // acmeAmount: ACME in internal units (1 ACME = 1e8 internal units)
    // oraclePrice: Price in 1/10000 USD per ACME (e.g., 5000 = $0.50 per ACME)
    
    credits = (acmeAmount * oraclePrice * CreditUnitsPerFiatUnit) / AcmePrecision
    
    // Example: 0.01 ACME at $0.50 per ACME
    // acmeAmount = 0.01 * 1e8 = 1,000,000
    // oraclePrice = 5000 (representing $0.50)
    // credits = (1,000,000 * 5000 * 10,000) / 100,000,000
    // credits = 500,000 internal units = 5000.00 credits
}
```

##### Step-by-Step Credit Calculation Process

1. **Get the Oracle Price**
   ```go
   status, _ := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
   oraclePrice := status.Oracle.Price  // e.g., 5000 = $0.50 per ACME
   ```

2. **Calculate ACME Amount Needed**
   ```go
   // For transaction fees (each transaction costs ~0.001 ACME)
   numTransactions := 1000
   acmePerTx := int64(0.001 * 1e8)  // 100,000 internal units
   totalACME := numTransactions * acmePerTx  // 100,000,000 internal units
   ```

3. **Convert ACME to Credits**
   ```go
   creditsNeeded := CalculateCredits(totalACME, oraclePrice)
   // With oracle at $0.50: 1 ACME = 5000 credits
   // So 1 ACME (1e8 internal) = 500,000 credit internal units
   ```

4. **Display Credits Properly**
   ```go
   // WRONG - Shows internal representation
   fmt.Printf("Credits: %d\n", creditsNeeded)  // Shows: 500000
   
   // CORRECT - Shows decimal representation
   fmt.Printf("Credits: %.2f\n", float64(creditsNeeded)/CreditPrecision)  // Shows: 5000.00
   ```

##### Common Calculation Errors

1. **Using Hardcoded Conversion Factors**
   ```go
   // WRONG - Uses arbitrary conversion factor
   credits := acmeAmount * 1e4  // Arbitrary factor, ignores oracle price
   
   // CORRECT - Uses oracle price
   credits := CalculateCredits(acmeAmount, oraclePrice)
   ```

2. **Mixing Precisions**
   ```go
   // WRONG - Treats credit internal units as credits
   minCredits := 100  // Thinks this is 100 credits, but it's actually 1.00 credit
   
   // CORRECT - Accounts for precision
   minCredits := 100 * CreditPrecision  // 10,000 internal units = 100.00 credits
   ```

3. **Ignoring Oracle Price Changes**
   ```go
   // WRONG - Assumes fixed ACME to credit ratio
   creditsPerACME := 10000  // Assumes 1 ACME = 10,000 credits always
   
   // CORRECT - Fetches current oracle price
   oraclePrice := getCurrentOraclePrice()
   creditsPerACME := CalculateCredits(1e8, oraclePrice)
   ```

##### Real-World Example

**Scenario**: Fund 50 accounts to send 10,000 transactions total

```go
// Step 1: Calculate ACME needed
txPerAccount := 10000 / 50  // 200 transactions per account
acmePerTx := int64(0.001 * 1e8)  // 100,000 internal units
acmePerAccount := txPerAccount * acmePerTx  // 20,000,000 internal units (0.2 ACME)

// Step 2: Get oracle price
oraclePrice := uint64(5000)  // $0.50 per ACME

// Step 3: Calculate credits needed per account
// Assume 1 credit per transaction (this varies by transaction type)
creditsPerTx := 1 * CreditPrecision  // 100 internal units
creditsNeededPerAccount := txPerAccount * creditsPerTx  // 20,000 internal units

// Step 4: Calculate ACME to spend on credits
// At $0.50 per ACME: 100 credits cost $1.00 = 2 ACME
// So 200 credits (20,000 internal) need:
acmeForCredits := (creditsNeededPerAccount * AcmePrecision) / CalculateCredits(1e8, oraclePrice)
// = (20,000 * 100,000,000) / 500,000 = 4,000,000 internal units (0.04 ACME)

// Step 5: Total per account
totalPerAccount := acmePerAccount + acmeForCredits  // 0.24 ACME
```

##### Validation Checks

Always validate credit calculations:
```go
// After adding credits, verify the balance
actualCredits := GetCreditsBalance(account)
expectedCredits := CalculateCredits(acmeSpentOnCredits, oraclePrice)

// Allow for small rounding differences
tolerance := uint64(10 * CreditPrecision)  // 10.00 credits tolerance
if abs(actualCredits - expectedCredits) > tolerance {
    // Credit calculation or conversion error!
}
```

#### Transaction Fee Table

**CRITICAL**: Accurate fee calculation is essential for proper load test funding. Always use the actual fee table from the network.

##### Getting the Fee Table

The fee table can be obtained from:
1. **Network Description Query**:
   ```go
   desc, err := client.Describe(ctx)
   feeTable := desc.Network.FeeTable
   ```

2. **Network Status Query** (may include fee information):
   ```go
   status, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
   ```

##### Standard Transaction Fees

| Transaction Type | Credit Cost | Notes |
|-----------------|-------------|-------|
| SendTokens | 3.00 credits | Token transfers between accounts |
| AddCredits | 3.00 credits | Adding credits to an account |
| CreateTokenAccount | 25.00 credits | Creating new token accounts |
| CreateIdentity | 100.00 credits | Creating ADI identity |
| CreateDataAccount | 25.00 credits | Creating data accounts |
| WriteData | 0.01 credits/byte | Writing data (minimum 1 credit) |
| UpdateKeyPage | 3.00 credits | Updating key pages |
| CreateKeyPage | 25.00 credits | Creating key pages |
| Burn | 3.00 credits | Burning tokens |
| Issue | 3.00 credits | Issuing tokens |

##### Using Fee Table for Load Calculations

**Step 1: Identify Transaction Mix**
```go
// Example: Load test with mixed transactions
type TransactionMix struct {
    SendTokens   int  // 90% of transactions
    AddCredits   int  // 5% of transactions  
    CreateTokenAccount int  // 5% of transactions
}

mix := TransactionMix{
    SendTokens: 9000,
    AddCredits: 500,
    CreateTokenAccount: 500,
}
```

**Step 2: Calculate Total Credits Needed**
```go
func calculateTotalCredits(mix TransactionMix, feeTable FeeTable) int64 {
    total := int64(0)
    
    // SendTokens: 9000 * 3 credits
    total += int64(mix.SendTokens) * feeTable.SendTokens
    
    // AddCredits: 500 * 3 credits
    total += int64(mix.AddCredits) * feeTable.AddCredits
    
    // CreateTokenAccount: 500 * 25 credits
    total += int64(mix.CreateTokenAccount) * feeTable.CreateTokenAccount
    
    // Add 10% buffer for safety
    return total + (total / 10)
}

// Example: 9000*3 + 500*3 + 500*25 = 27000 + 1500 + 12500 = 41000 credits
// With buffer: 45100 credits needed
```

**Step 3: Convert Credits to ACME**
```go
func creditsToACME(credits int64, oraclePrice uint64) int64 {
    // credits = (acmeAmount * oraclePrice * CreditUnitsPerFiatUnit) / AcmePrecision
    // So: acmeAmount = (credits * AcmePrecision) / (oraclePrice * CreditUnitsPerFiatUnit)
    
    creditsInternal := credits * protocol.CreditPrecision
    acmeNeeded := (creditsInternal * protocol.AcmePrecision) / 
                  (oraclePrice * protocol.CreditUnitsPerFiatUnit)
    
    // Ensure minimum
    if acmeNeeded < int64(0.01 * 1e8) {
        acmeNeeded = int64(0.01 * 1e8)
    }
    
    return acmeNeeded
}
```

##### Dynamic Fee Discovery

**IMPORTANT**: Never hardcode fee values. Always fetch from the network:

```go
func fetchCurrentFees(client *jsonrpc.Client) (*FeeSchedule, error) {
    // Get network description
    desc, err := client.Describe(context.Background())
    if err != nil {
        return nil, fmt.Errorf("failed to get network description: %w", err)
    }
    
    fees := &FeeSchedule{
        SendTokens:         getFeeForType(desc, protocol.TransactionTypeSendTokens),
        AddCredits:        getFeeForType(desc, protocol.TransactionTypeAddCredits),
        CreateTokenAccount: getFeeForType(desc, protocol.TransactionTypeCreateTokenAccount),
        // ... other transaction types
    }
    
    return fees, nil
}

func getFeeForType(desc *api.Description, txType protocol.TransactionType) int64 {
    // Look up fee in description's fee table
    if fee, ok := desc.Network.FeeTable[txType]; ok {
        return fee
    }
    // Return default if not found
    return 300 // 3.00 credits default
}
```

##### Load Test Fee Calculation Example

**Scenario**: 10,000 transactions across 50 accounts

```go
// Step 1: Get fee table from network
fees, _ := fetchCurrentFees(client)

// Step 2: Calculate per-account needs
txPerAccount := 10000 / 50  // 200 transactions
creditsPerAccount := txPerAccount * fees.SendTokens  // 200 * 3 = 600 credits

// Step 3: Convert to ACME (at $0.50/ACME oracle price)
acmeForCredits := creditsToACME(creditsPerAccount, 5000)
// 600 credits = 0.012 ACME at $0.50/ACME

// Step 4: Add transaction ACME costs (0.001 ACME per tx)
acmeForTxs := txPerAccount * int64(0.001 * 1e8)  // 0.2 ACME

// Step 5: Total per account with 10% buffer
totalPerAccount := (acmeForCredits + acmeForTxs) * 110 / 100
// (0.012 + 0.2) * 1.1 = 0.2332 ACME per account
```

##### Common Fee Calculation Errors

1. **Hardcoding Fee Values**
   ```go
   // WRONG - Fees may change
   creditsNeeded := txCount * 3
   
   // CORRECT - Use fee table
   creditsNeeded := txCount * feeTable.SendTokens
   ```

2. **Wrong Transaction Type Assumptions**
   ```go
   // WRONG - Assumes all transactions cost the same
   totalCredits := totalTxCount * 3
   
   // CORRECT - Calculate based on transaction mix
   totalCredits := sendCount * fees.SendTokens + 
                   createCount * fees.CreateTokenAccount
   ```

3. **Ignoring Minimum Fees**
   ```go
   // WRONG - WriteData has minimum fee
   writeCredits := dataSize * 0.01
   
   // CORRECT - Ensure minimum
   writeCredits := max(dataSize * 0.01, 1.0)
   ```

##### Fee Table Integration in Load Tests

The load test should:
1. **Query fee table at startup** - Get current fees from network
2. **Calculate credits per transaction type** - Use actual fees, not estimates
3. **Adjust for transaction mix** - Different transaction types have different costs
4. **Add safety buffer** - Always add 10% extra for fee variations
5. **Validate sufficient credits** - Check credit balance before starting load
6. **Monitor credit consumption** - Track actual vs expected credit usage

```go
// In test setup
func setupLoadTest(config LoadConfig) error {
    // 1. Fetch current fees
    fees, err := fetchCurrentFees(client)
    if err != nil {
        return fmt.Errorf("failed to get fees: %w", err)
    }
    
    // 2. Calculate credits needed
    creditsNeeded := calculateCreditsForLoad(config, fees)
    
    // 3. Ensure accounts have sufficient credits
    if err := fundAccountsWithCredits(accounts, creditsNeeded); err != nil {
        return fmt.Errorf("failed to fund credits: %w", err)
    }
    
    // 4. Validate before starting
    if !validateSufficientCredits(accounts, creditsNeeded) {
        return fmt.Errorf("insufficient credits after funding")
    }
    
    return nil
}
```
  
#### Settlement Timeouts
- **Settlement Timeout**: 1 minute maximum
- **Progress Detection**: Timeout resets to 1 minute whenever progress is detected
- **No Scaling**: Same timeout for any number of transactions
- **Progress Definition**: Any balance change in any account being monitored

### Example Configurations

#### Default (1000 txs, 10k, 10a)
```bash
go test -v -run TestStreamlinedLoad
# Sends 1000 txs using 10 senders to 10 receivers
# No rate limiting, runs at maximum speed
```

#### Rate Limited (5000 txs at 100 TPS)
```bash
go test -v -run TestStreamlinedLoad -txs=5000 -tps=100
# Sends 5000 txs at 100 TPS (takes ~50 seconds)
# Prevents overwhelming the network
```

#### High Volume (20000 txs, 10k, 5a)
```bash
go test -v -run TestStreamlinedLoad -txs=20000 -k=10 -a=5 -tps=200
# Sends 20000 txs using 10 senders to 5 receivers at 200 TPS
# Each sender: 2000 txs, needs 2.5 ACME + credits
```

#### Many Accounts (5000 txs, 50k, 20a)
```bash
go test -v -run TestStreamlinedLoad -txs=5000 -k=50 -a=20
# Sends 5000 txs using 50 senders to 20 receivers
# Tests account scaling
```

#### Debug Mode (100 txs with verbose output)
```bash
go test -v -run TestStreamlinedLoad -txs=100 -verbose
# Sends 100 txs with detailed logging
# Useful for troubleshooting issues
```

## Test Flow Specification

### Phase 1: Setup with Balance Validation
1. **MUST PRINT** funding account address and private key (hex format) for debugging
2. **MUST PRINT** generation seed (hex format) for K and A accounts for debugging
3. Create unique K and A accounts using timestamp seed
4. **Check funding account current balance** (may have existing ACME/credits)
5. Calculate total ACME and credits needed (dynamically, no hardcoded values)
6. **Top off funding account** via faucet (only request difference to reach 110% of needs)
7. **Validate funding account** has target balance after faucet (wait for settlement)
8. **Check funding account credits** and calculate top-off amount needed
9. **Top off credits** for funding account to reach 110% of computed needs
10. **Validate funding account credits** settled to target value
11. **Check all K account balances** (should be zero, but don't fail if not)
12. **Check all A account balances** (should be zero, but don't fail if not)
13. Distribute 110% of computed ACME from funding account to each K account
14. **Validate each K account** balance settled to expected amount
15. Add 110% of computed credits to K accounts (funding initiates, K balance pays)
16. **Validate each K account** credits settled to expected amount

### Phase 2: Load Generation
1. Calculate transactions per sender
2. Send transactions in round-robin pattern
3. **Track exact ACME sent to each A account** (maintain running total)
4. Track expected spend per sender (K accounts)
5. Track expected receive per receiver (A accounts)
6. Measure transactions per second
7. Print progress status throughout generation

### Progress Reporting Requirements
- **Print progress status** at regular intervals during load generation
- **Frequency**: Every 10% of total transactions OR every 30 seconds, whichever comes first
- **Progress should include**:
  - Number of transactions sent so far
  - Percentage complete
  - Current measured TPS
  - Elapsed time
  - Estimated time remaining (if rate-limited)

### Phase 3: Verification After Load Generation
1. **Validate all A accounts** have received exact tracked amounts
2. Poll A account balances every second until settled
3. Compare actual vs expected for all A accounts
4. **Validate all K accounts** have correct remaining balance
5. Poll K account balances every second until settled
6. Compare actual vs expected for all K accounts
7. Exit early if all balances correct
8. Generate detailed report if discrepancies found
9. Identify specific failure patterns
10. **Ensure total ACME conserved** (sum of all accounts matches expected)

## Verification Requirements

### Success Criteria
- All sender balances = initial - (txCount * txAmount) ± 0.0001 ACME
- All receiver balances = **exact tracked amount sent to each A account**
- Each A account balance matches the sum of all transactions sent to it
- Total ACME conserved (sum of all accounts unchanged)
- All transactions either fully processed or clearly failed
- **Settlement validation**: Wait for and verify settlement at each step

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

## Required Output at Test Start

The test MUST print the following information at startup for debugging purposes:

1. **Funding Account Information**:
   ```
   === FUNDING ACCOUNT ===
   Address: acc://[address]/ACME
   Private Key (hex): [64-character hex string]
   =======================
   ```

2. **Generation Seed Information**:
   ```
   === GENERATION SEED ===
   Seed (hex): [64-character hex string]
   =======================
   ```

This allows post-test debugging by:
- Accessing the funding account directly
- Regenerating the exact K and A accounts used in the test
- Investigating any issues with specific accounts

## Implementation Guidelines

### Global Test Context Pattern
- Single LoadTestContext struct shared across all operations
- Contains client, accounts, configuration, oracle price
- Passed to all functions for stateless operation
- Enables function composition and reusability

### Context Usage Restrictions
- **NO context.Context structs**: Do not use Go's context package or ctx structs anywhere in the streamlined load generator
- **NO waitgroups**: Avoid sync.WaitGroup - use simpler synchronization patterns
- A search for "ctx" in sl_*.go files should return zero results
- Use simple sequential or concurrent patterns without complex synchronization

### Error Handling Strategy
- Return errors immediately, don't continue on failure
- Log specific failure reasons with context
- Fail fast during setup to save time
- Provide clear error messages for debugging

### Deterministic Account Generation
- Base seed from timestamp hash for K and A accounts
- **MUST print** the generation seed at test start for debugging
- Combine seed with index and prefix for each account
- Ensures unique accounts per test run
- Allows reproduction with same timestamp
- Enables post-test debugging by knowing exact accounts used

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