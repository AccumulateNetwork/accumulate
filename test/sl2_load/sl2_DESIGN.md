# SL2 Load Testing Design

## Overview
SL2 is a redesigned streamlined load testing framework for Accumulate that provides deterministic, reproducible load testing with simplified account management. SL2 executes real transactions against a running devnet - no mocks, no simulations, all real network operations.

## Core Design Principles

### 1. Real Network Testing Only
- **No Mock Accounts**: All accounts exist on the devnet
- **No Simulated Transactions**: Every transaction is submitted to the network
- **Real Consensus**: Transactions go through actual consensus process
- **Real Performance Metrics**: Measurements reflect actual network behavior
- **Devnet Required**: Tests will fail if devnet is not running

### 2. Deterministic Account Generation
- **Funding Account**: Single hardcoded account with private key as SHA256("sl2_load")
  - Consistent across all test runs
  - Easy to fund externally before tests
  - Eliminates faucet dependency issues

- **Test Accounts**: 100 real lite accounts generated deterministically
  - Seed: SHA256(unix_timestamp_nanoseconds)
  - Account[i] key: SHA256(seed || 8_bytes_of_i)
  - Uses binary concatenation, not string formatting
  - Reproducible within same nanosecond
  - Different accounts each test run to avoid state conflicts

### 3. Test Structure
- **SL2Test struct**: Main test orchestrator
  - Funding account reference
  - Array of test accounts
  - Lite client instance (shared across operations)
  - Test configuration parameters
  - Performance metrics tracking

### 4. Test Initialization Flow
1. Generate funding account from hardcoded seed
2. Create timestamp-based seed for test accounts
3. Print configuration (time, seed, funding details)
4. Generate 100 lite accounts deterministically
5. Display account URLs for verification
6. Initialize lite client (lazy initialization on first use)

## Advantages Over Original SL

### Eliminated Dependencies
- No faucet requirement (after initial funding)
- No external account lookups
- No random key generation issues
- No mock infrastructure needed

### Improved Reproducibility
- Funding account always same
- Test accounts deterministic from seed
- Can reproduce exact test by using same nanosecond timestamp
- Binary operations ensure consistent behavior across platforms

### Simplified Debugging
- All account info printed at start
- Clear seed/timestamp tracking
- Predictable account addresses

## Component Architecture

### Faucet Module (sl2_faucet.go)
- **Purpose**: Handle funding account initialization
- **Responsibilities**:
  - Lazy client initialization (create on first use)
  - Call faucet endpoint for funding account
  - Retry logic for faucet failures
  - Balance verification after funding
- **Design Principles**:
  - Client reuse across all faucet operations
  - Singleton pattern for client instance
  - Automatic retry with exponential backoff

## Test Execution Strategy

### Prerequisites
- Devnet must be running (use devnet_config.sh)
- Network endpoints must be accessible
- Initial faucet funding must succeed

### Phase 1: Setup
- Connect to real devnet endpoints
- Multiple faucet funding with settlement verification:
  - Accept parameter N for number of faucet calls (default 1)
  - Record starting balance (may be non-zero from previous runs)
  - Call faucet N times
  - Expected final balance = starting balance + (N * 10 ACME)
  - Settlement verification:
    - Poll every 2 seconds
    - End condition (whichever comes first):
      a) Balance reaches expected total, OR
      b) 1 minute passes with no balance change
    - Reset 1-minute timer on any balance change
    - Track and log each balance change

### Phase 2: Load Generation
- Execute real transactions between accounts
- Monitor actual network success/failure rates
- Track real network performance metrics
- Handle real network delays and timeouts

### Phase 3: Verification
- Query real on-chain balances
- Validate actual transaction counts from network
- Generate report based on real metrics

## Configuration via Arguments

The test accepts positional arguments in order:
1. Total transactions (required)
2. Target TPS (required)
3. Number of accounts (optional, default 100)
4. Timeout in seconds (optional, default 600)
5. Verbose mode (optional, default false)

## Implementation Status

### Completed
- [x] Deterministic funding account
- [x] Timestamp-based seed generation
- [x] 100 lite account creation
- [x] Account information display

### Pending
- [ ] Balance verification
- [ ] Fund distribution logic
- [ ] Transaction execution
- [ ] Performance monitoring
- [ ] Report generation

## Usage Example

Example usage:
- Basic: TestSL2Load

## Key Files

### sl2_test.go
- **Purpose**: Main test implementation and orchestration
- **Contents**:
  - Account struct: Manages private key, public key, and lite URL
  - SL2Test struct: Main test orchestrator with client, accounts, config, and metrics
  - TestSL2Load(): Entry point for the test
  - createAccount(): Generates deterministic accounts from seeds
  - Creates funding account from SHA256("sl2_load")
  - Generates 100 test accounts using nanosecond timestamp with binary concatenation

### sl2_faucet.go  
- **Purpose**: Handles all faucet and funding operations
- **Contents**:
  - FundAccount(): Main funding function with retry logic
  - initializeClient(): Lazy initialization of JSON-RPC client (singleton pattern)
  - callFaucet(): Makes actual faucet API calls to devnet
  - getAccountBalance(): Queries and verifies account balances
  - Implements exponential backoff for retries
  - All operations are real network transactions

### sl2_DESIGN.md
- **Purpose**: Complete design documentation
- **Contents**:
  - Architecture overview and design principles
  - Real network testing approach (no mocks or simulations)
  - Deterministic account generation strategy
  - Test execution phases and prerequisites
  - Configuration options and usage examples

## Future Enhancements
1. Configurable account count
2. Multiple funding accounts for higher throughput
3. Transaction pattern customization
4. Real-time metrics dashboard
5. Automatic retry mechanisms