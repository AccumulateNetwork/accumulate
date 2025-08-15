# DevNet Load Generator Design

## Executive Summary

The DevNet Load Generator is a continuous transaction generation system designed to create realistic network activity for testing, monitoring, and visualization purposes. It focuses on creating a steady stream of transactions through faucet operations, providing immediate visual feedback in the DevNet dashboard.

**CRITICAL REQUIREMENT: NO SIMULATION** - All operations MUST be real blockchain transactions executed on the running DevNet. The load generator exists to test actual network operators and blockchain functionality. Simulated operations are strictly prohibited.

## Primary Objective

Create continuous, visible transaction activity on the DevNet to:
1. Validate transaction processing and consensus with REAL transactions
2. Test cross-chain message flow with ACTUAL blockchain operations
3. Provide visual feedback in the dashboard from GENUINE network activity
4. Build up account pools through REAL faucet funding for comprehensive testing
5. Test network operators in a REAL running network environment

## NO SIMULATION REQUIREMENT

**This is a fundamental design principle that cannot be compromised:**

1. **ALL operations MUST be REAL blockchain transactions** - No simulated or fake operations
2. **The faucet provides REAL ACME tokens** - These are used to fund all activities
3. **ADI creation uses REAL ACME** - Pay actual tokens to create identities
4. **Credit generation requires REAL ACME** - Convert actual tokens to credits
5. **Token accounts require REAL credits** - Use blockchain credits for account creation
6. **All transfers are REAL transactions** - Move actual tokens between real accounts

**Why NO SIMULATION?**
- We are testing REAL network operators in a running blockchain
- Simulated operations provide no value for testing consensus, transaction processing, or cross-chain messaging
- The entire purpose is to generate ACTUAL load on the DevNet to validate its operation

**Funding Model:**
The DevNet faucet provides unlimited ACME tokens for testing. This enables:
- Continuous account creation and funding
- ADI creation with real token payments
- Credit purchases for account operations
- Token transfers between real accounts

## Architecture

### System Components

```
┌────────────────────────────────────────────────────────────┐
│                    Load Generator System                    │
├────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────────┐         ┌──────────────────┐         │
│  │   Account       │────────►│    Transaction   │         │
│  │   Generator     │         │    Engine        │         │
│  └─────────────────┘         └──────────────────┘         │
│           │                           │                     │
│           ▼                           ▼                     │
│  ┌─────────────────┐         ┌──────────────────┐         │
│  │   Key Store     │         │   JSON-RPC      │         │
│  │   Management    │         │   Client         │         │
│  └─────────────────┘         └──────────────────┘         │
│                                       │                     │
└───────────────────────────────────────┼─────────────────────┘
                                       ▼
                              ┌──────────────────┐
                              │   DevNet API     │
                              │  (Port 26660)    │
                              └──────────────────┘
```

### Core Implementation (Phase 1: Faucet Loop)

```go
type LoadGenerator struct {
    client         *jsonrpc.Client
    accounts       map[string]*Account
    requestCount   uint64
    successCount   uint64
    failCount      uint64
    startTime      time.Time
}

type Account struct {
    URL        *url.URL
    PrivateKey []byte
    Balance    uint64
    Created    time.Time
}
```

## Implementation Phases

### Phase 1: Continuous Faucet Collection with REAL Transactions

**Objective**: Create an infinite loop that continuously generates accounts and requests ACME from the faucet using REAL blockchain transactions.

**Algorithm** (NO SIMULATION - ALL REAL OPERATIONS):
```
1. Initialize JSON-RPC client to DevNet API
2. Start infinite loop:
   a. Generate unique ED25519 key pair
   b. Create lite token address for ACME
   c. Request REAL tokens from faucet (actual blockchain transaction)
   d. Store account information
   e. Update statistics from REAL transaction results
   f. Brief delay (100-500ms)
3. Periodic balance checks via REAL API queries
4. Console output for monitoring ACTUAL blockchain activity
```

**Key Features**:
- Continuous account generation with REAL keys
- Automatic faucet requests via ACTUAL transactions
- Statistics tracking from REAL blockchain responses
- Error resilience for REAL network conditions
- Graceful shutdown

### Phase 2: Token Transfers with REAL Blockchain Transactions

- Transfer ACME between generated accounts using REAL SendTokens transactions
- Create ACTUAL transaction chains on the blockchain
- Test different transaction types with REAL network validation

### Phase 3: ADI Operations with REAL Credits and Transactions

- Create Accumulate Digital Identifiers using REAL ACME from faucet
- Pay ACTUAL ACME to generate credits for ADIs
- Use REAL credits to create token accounts
- All operations are REAL blockchain transactions

### Phase 4: Advanced Patterns with REAL Network Operations

- Data entry transactions with REAL data on-chain
- Multi-signature operations with ACTUAL signature validation
- Token issuance with REAL token creation

## Detailed Design: Continuous Faucet Collector (NO SIMULATION)

### Main Loop Structure - REAL TRANSACTIONS ONLY

```go
func (lg *LoadGenerator) Run(ctx context.Context) error {
    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            lg.generateAndFund() // REAL faucet transaction
        }
    }
}

func (lg *LoadGenerator) generateAndFund() {
    // 1. Generate unique account with REAL key
    seed := fmt.Sprintf("load-%d-%d", 
        atomic.AddUint64(&lg.requestCount, 1),
        time.Now().UnixNano())
    privKey := GenerateKey(seed)
    
    // 2. Create lite address for REAL account
    addr, _ := protocol.LiteTokenAddress(
        privKey[32:], "ACME", 
        protocol.SignatureTypeED25519)
    
    // 3. Request REAL tokens from faucet
    // This submits an ACTUAL blockchain transaction
    ctx, cancel := context.WithTimeout(
        context.Background(), 10*time.Second)
    defer cancel()
    
    sub, err := lg.client.Faucet(ctx, addr, 
        api.FaucetOptions{})
    
    // 4. Track REAL transaction results
    if err != nil {
        atomic.AddUint64(&lg.failCount, 1)
        return
    }
    
    atomic.AddUint64(&lg.successCount, 1)
    
    // 5. Store account with REAL blockchain address
    lg.mu.Lock()
    lg.accounts[addr.String()] = &Account{
        URL:        addr,
        PrivateKey: privKey,
        Created:    time.Now(),
    }
    lg.mu.Unlock()
}
```

### Configuration Parameters

```yaml
load_generator:
  # API endpoint
  api_endpoint: "http://127.0.0.1:26660/v3"
  
  # Generation rate
  accounts_per_second: 2
  
  # Batch size for parallel generation
  batch_size: 5
  
  # Balance check interval
  balance_check_interval: 30s
  
  # Statistics display interval
  stats_interval: 5s
  
  # Maximum accounts to maintain
  max_accounts: 10000
  
  # Retry configuration
  retry_attempts: 3
  retry_delay: 1s
```

### Console Output Format

```
================================================================================
                    DEVNET LOAD GENERATOR - FAUCET COLLECTOR
================================================================================
Status: RUNNING | Uptime: 00:05:23 | API: http://127.0.0.1:26660

STATISTICS:
  Total Requests:     523
  Successful:         520 (99.4%)
  Failed:            3 (0.6%)
  Rate:              2.1 req/s
  
ACCOUNTS:
  Total Created:      520
  Total Balance:      5,200 ACME
  Avg Balance:        10 ACME
  Active Accounts:    520

RECENT ACTIVITY:
  10:23:45 - Created acc://a3f2... - Faucet: SUCCESS (10 ACME)
  10:23:44 - Created acc://b7e1... - Faucet: SUCCESS (10 ACME)
  10:23:43 - Created acc://c9d4... - Faucet: SUCCESS (10 ACME)
  
NETWORK METRICS:
  Latest Block:       #1,234
  TPS:               15.3
  Pending TXs:        2
================================================================================
Press Ctrl+C to stop
```

### Error Handling

1. **Connection Failures**:
   - Exponential backoff retry
   - Maximum retry attempts
   - Graceful degradation

2. **Faucet Exhaustion**:
   - Detect empty faucet
   - Alert and pause
   - Resume when refilled

3. **Network Congestion**:
   - Adaptive rate limiting
   - Queue management
   - Timeout handling

### Integration with Dashboard

The load generator provides real-time metrics that the dashboard can display:

1. **Transaction Metrics**:
   - Transactions per second
   - Success/failure rates
   - Queue depths

2. **Account Metrics**:
   - Total accounts created
   - Active accounts
   - Balance distribution

3. **Network Metrics**:
   - Block progression
   - Partition activity
   - Cross-chain messages

### Command-Line Interface

```bash
# Start load generator with default settings
./load-generator

# Custom configuration
./load-generator --rate 5 --endpoint http://127.0.0.1:26660/v3

# Verbose output
./load-generator --verbose

# JSON output for automation
./load-generator --output json

# Run for specific duration
./load-generator --duration 10m

# Save accounts to file
./load-generator --save-accounts accounts.json
```

### Performance Considerations

1. **Resource Usage**:
   - Memory: ~100MB for 10,000 accounts
   - CPU: < 5% for standard load
   - Network: ~10 KB/s at 2 req/s

2. **Scalability**:
   - Supports multiple instances
   - Partition-aware distribution
   - Rate limiting per partition

3. **Optimization**:
   - Connection pooling
   - Batch operations
   - Async processing

## Testing Strategy

### Unit Tests

- Account generation uniqueness
- Faucet request formatting
- Statistics calculation
- Error handling paths

### Integration Tests

- DevNet API connectivity
- Faucet operation success
- Balance verification
- Multi-partition distribution

### Load Tests

- Sustained operation (hours/days)
- High rate generation
- Memory leak detection
- Connection stability

## Future Enhancements

### Near-term (Phase 2)
- Token transfer operations
- Account-to-account transactions
- Balance redistribution

### Medium-term (Phase 3)
- ADI creation and management
- Token issuance
- Multi-signature operations

### Long-term (Phase 4)
- Smart contract interactions
- Complex transaction chains
- Chaos engineering scenarios

## Security Considerations

1. **Key Management**:
   - Keys stored in memory only
   - Optional encryption at rest
   - Secure key generation

2. **API Security**:
   - Rate limiting respect
   - Connection encryption (HTTPS)
   - Authentication support

3. **Resource Protection**:
   - Memory limits
   - Connection limits
   - Graceful degradation

## Conclusion

The Load Generator provides essential transaction activity for DevNet testing and visualization. Starting with a simple faucet collection loop, it establishes the foundation for comprehensive load testing capabilities while providing immediate value through visible network activity in the dashboard.