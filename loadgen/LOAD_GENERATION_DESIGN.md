# Load Generation Design

## Overview
The load generator creates realistic transaction workloads for testing Accumulate network performance, scalability, and reliability. It operates in two modes: blocking (latency-focused) and non-blocking (throughput-focused).

## Architecture

### Core Components

```
┌─────────────────────────────────────────────────────────────┐
│                     Load Generator                           │
├───────────────┬────────────┬─────────────┬─────────────────┤
│    Wallet     │ Transaction │   Metrics   │     Engine      │
│   Manager     │  Generator  │  Collector  │   Controller    │
├───────────────┼────────────┼─────────────┼─────────────────┤
│  - Accounts   │ - Builder   │ - Tracking  │ - Mode Select   │
│  - Keys       │ - Selector  │ - Analysis  │ - Rate Control  │
│  - Balances   │ - Validator │ - Reporting │ - Coordination  │
└───────────────┴────────────┴─────────────┴─────────────────┘
```

## Component Design

### 1. Wallet Manager

#### Purpose
Manages all accounts, keys, and balances required for transaction generation.

#### Responsibilities
- Account inventory management
- Key storage and retrieval
- Balance tracking (optimistic and verified)
- Credit management
- Account relationship tracking

#### Key Structures
```
WalletManager
├── ADIs (map[string]*ADI)
├── LiteAccounts (map[string]*LiteAccount)
├── TokenAccounts (map[string]*TokenAccount)
├── DataAccounts (map[string]*DataAccount)
├── Keys (map[string]*Key)
├── KeyPages (map[string]*KeyPage)
└── Balances (map[string]uint64)
```

#### Operations
- GetFundedAccount(): Returns account with sufficient balance
- GetSigningKey(): Returns appropriate key for transaction
- UpdateBalance(): Updates optimistic balance after transaction
- VerifyBalance(): Confirms actual balance from network
- CreateAccount(): Adds new account to wallet
- RefreshCredits(): Ensures sufficient credits available

### 2. Transaction Generator

#### Purpose
Creates valid transactions based on configured distribution and available resources.

#### Responsibilities
- Transaction type selection
- Transaction building
- Prerequisite validation
- Signature generation
- Transaction serialization

#### Key Structures
```
TransactionGenerator
├── TypeSelector (weighted random selection)
├── Builder (transaction construction)
├── Validator (prerequisite checking)
├── Signer (signature creation)
└── Serializer (encoding)
```

#### Transaction Flow
1. **Selection**: Choose transaction type based on weights
2. **Validation**: Check prerequisites available
3. **Construction**: Build transaction body
4. **Signing**: Add required signatures
5. **Submission**: Send to network

### 3. Metrics Collector

#### Purpose
Tracks all performance metrics and operational statistics.

#### Responsibilities
- Transaction counting
- Latency measurement
- Error tracking
- Resource monitoring
- Report generation

#### Integration Points
- Pre-submission: Record attempt
- Post-submission: Record submission latency
- Post-verification: Record end-to-end latency
- On-error: Record failure details

### 4. Engine Controller

#### Purpose
Orchestrates load generation according to configured mode and parameters.

#### Responsibilities
- Mode management (blocking/non-blocking)
- Rate control
- Worker coordination
- Backpressure handling
- Graceful shutdown

## Operating Modes

### Blocking Mode (Latency-Focused)

#### Characteristics
- Waits for transaction completion before proceeding
- Validates transaction effects (balance changes)
- Accurate end-to-end latency measurement
- Lower throughput
- Deterministic wallet state

#### Operation Flow
```
1. Generate Transaction
2. Submit to Network
3. Poll for Completion
   - Check destination balance
   - Verify transaction status
   - Timeout handling
4. Update Metrics
5. Update Wallet State
6. Continue to Next Transaction
```

#### Configuration
```yaml
mode: blocking
polling:
  interval: 100ms
  timeout: 30s
  method: balance_check
verification:
  required: true
  retry_on_failure: true
concurrency: 10  # Limited concurrent transactions
```

#### Use Cases
- Latency measurement
- Transaction verification
- Consistency testing
- Debugging
- Small-scale testing

### Non-Blocking Mode (Throughput-Focused)

#### Characteristics
- Fire-and-forget submission
- No completion waiting
- Optimistic wallet updates
- Maximum throughput
- Eventual consistency

#### Operation Flow
```
1. Generate Transaction
2. Submit to Network
3. Record Submission
4. Update Metrics (submission only)
5. Optimistically Update Wallet
6. Immediately Continue
```

#### Configuration
```yaml
mode: non-blocking
submission:
  batch_size: 100
  queue_depth: 10000
  workers: 50
verification:
  required: false
  sample_rate: 0.01  # Verify 1% for validation
concurrency: unlimited
```

#### Use Cases
- Stress testing
- Maximum load generation
- Network saturation testing
- Large-scale testing
- Performance limits exploration

## Mode Comparison

| Aspect | Blocking Mode | Non-Blocking Mode |
|--------|--------------|-------------------|
| **Throughput** | Low (10-100 TPS) | High (1000+ TPS) |
| **Latency Accuracy** | Exact end-to-end | Submission only |
| **Wallet Accuracy** | Always accurate | Eventually consistent |
| **Resource Usage** | High (polling) | Low (fire-forget) |
| **Complexity** | High | Low |
| **Debugging** | Easy | Difficult |
| **Use Case** | Accuracy | Volume |

## Hybrid Mode

### Adaptive Mode Switching
```yaml
mode: hybrid
primary: non-blocking
sampling:
  rate: 0.05  # 5% in blocking mode
  strategy: random
fallback:
  on_error_rate: 0.10  # Switch to blocking if >10% errors
  on_queue_depth: 5000  # Switch to blocking if queue too deep
```

### Benefits
- Balance between accuracy and throughput
- Continuous latency sampling
- High load with verification
- Adaptive to network conditions

## Transaction Pipeline

### Pipeline Stages

#### Stage 1: Generation
```
Input: Transaction Type
Process:
  - Gather prerequisites
  - Validate resources
  - Build transaction
Output: Unsigned Transaction
```

#### Stage 2: Signing
```
Input: Unsigned Transaction
Process:
  - Identify required signatures
  - Retrieve signing keys
  - Generate signatures
Output: Signed Transaction
```

#### Stage 3: Submission
```
Input: Signed Transaction
Process:
  - Serialize transaction
  - Submit to network
  - Record submission time
Output: Transaction ID
```

#### Stage 4: Verification (Blocking Only)
```
Input: Transaction ID
Process:
  - Poll for completion
  - Verify effects
  - Record completion time
Output: Verification Result
```

#### Stage 5: Metrics
```
Input: Transaction Result
Process:
  - Update counters
  - Calculate latencies
  - Track errors
Output: Metrics Update
```

## Rate Control

### Token Bucket Algorithm
```
Bucket Capacity: burst_size
Refill Rate: target_tps
Operation:
  - Take token for each transaction
  - Block when bucket empty
  - Refill at constant rate
```

### Adaptive Rate Control
```
Initial Rate: 10 TPS
Increase: +10% every minute if success > 95%
Decrease: -50% immediately if success < 80%
Maximum: configured_max_tps
Minimum: 1 TPS
```

### Backpressure Handling
```
Queue Depth Monitoring:
  - Slow down if queue > 80% full
  - Pause if queue full
  - Resume when queue < 50% full

Network Response:
  - Reduce rate on timeouts
  - Backoff on errors
  - Circuit breaker on failures
```

## Worker Pool Design

### Blocking Mode Workers
```
Worker Count: Fixed (10-50)
Task: Complete transaction lifecycle
Queue: Bounded (prevent overflow)
Coordination: Semaphore for completion
```

### Non-Blocking Mode Workers
```
Worker Count: Dynamic (50-500)
Task: Submit transactions only
Queue: Large (10000+)
Coordination: Channel-based
```

### Worker Lifecycle
```
1. Initialize
   - Establish connections
   - Prepare resources
2. Process
   - Fetch from queue
   - Execute transaction
   - Report results
3. Shutdown
   - Drain queue
   - Close connections
   - Final reporting
```

## Error Handling

### Error Categories

#### Transient Errors
- Network timeouts
- Temporary unavailability
- Rate limiting
**Action**: Retry with backoff

#### Permanent Errors
- Insufficient balance
- Invalid transaction
- Account not found
**Action**: Skip and log

#### Critical Errors
- Network unreachable
- All accounts exhausted
- Configuration error
**Action**: Stop generator

### Recovery Strategies

#### Exponential Backoff
```
Initial: 100ms
Maximum: 30s
Multiplier: 2
Jitter: ±20%
```

#### Circuit Breaker
```
Threshold: 10 consecutive failures
Open Duration: 30 seconds
Half-Open Test: 1 transaction
Recovery: 5 successful transactions
```

## State Management

### Wallet State

#### Optimistic Updates (Non-Blocking)
- Immediately deduct balance
- Mark account as used
- Update nonce
- Risk: Overdraft

#### Verified Updates (Blocking)
- Wait for confirmation
- Query actual balance
- Sync with network
- Guarantee: Accuracy

### Recovery from Inconsistency
```
1. Detect divergence
2. Pause generation
3. Query all accounts
4. Reconcile balances
5. Resume generation
```

## Configuration Schema

### Complete Configuration
```yaml
load_generator:
  # Mode configuration
  mode: blocking|non-blocking|hybrid
  
  # Target performance
  target_tps: 100
  burst_size: 1000
  ramp_up_time: 60s
  test_duration: 3600s
  
  # Transaction mix
  transaction_weights:
    send_tokens: 0.50
    create_account: 0.20
    write_data: 0.20
    other: 0.10
  
  # Blocking mode settings
  blocking:
    poll_interval: 100ms
    poll_timeout: 30s
    verification_method: balance
    max_concurrent: 10
  
  # Non-blocking mode settings
  non_blocking:
    queue_size: 10000
    worker_count: 100
    batch_size: 100
    optimistic_updates: true
  
  # Rate control
  rate_control:
    algorithm: token_bucket
    adaptive: true
    min_tps: 1
    max_tps: 10000
    increase_rate: 0.1
    decrease_rate: 0.5
  
  # Error handling
  error_handling:
    max_retries: 3
    backoff_multiplier: 2
    circuit_breaker: true
    error_threshold: 0.1
  
  # Resource limits
  limits:
    max_accounts: 10000
    max_pending: 5000
    max_workers: 500
    memory_limit: 4GB
  
  # Metrics
  metrics:
    interval: 1s
    detailed: true
    export_format: json
    export_path: ./metrics
```

## Performance Expectations

### Blocking Mode
- **TPS**: 10-100
- **Latency**: Accurate to ms
- **CPU**: Moderate (polling)
- **Memory**: Low
- **Network**: High (polling queries)

### Non-Blocking Mode
- **TPS**: 1000-10000
- **Latency**: Submission only
- **CPU**: High (generation)
- **Memory**: High (queue)
- **Network**: Moderate (submissions)

## Monitoring & Observability

### Key Metrics
- Transactions per second
- Success rate
- Latency percentiles
- Queue depth
- Worker utilization
- Error rate
- Resource usage

### Health Checks
- Wallet account availability
- Network connectivity
- Queue capacity
- Memory usage
- Worker pool status

### Alerting
- TPS below target
- Error rate spike
- Queue overflow
- Resource exhaustion
- Network issues

## Testing Strategy

### Unit Testing
- Transaction generation
- Wallet operations
- Rate control
- Error handling

### Integration Testing
- Network submission
- Balance verification
- Cross-partition transactions
- Recovery scenarios

### Load Testing
- Gradual ramp-up
- Sustained load
- Burst testing
- Failure injection

### Validation
- Balance consistency
- Transaction uniqueness
- Metrics accuracy
- State recovery

## Future Enhancements

### Planned Features
1. **Smart Load Distribution**: ML-based transaction selection
2. **Predictive Scaling**: Anticipate resource needs
3. **Chaos Engineering**: Automated failure injection
4. **Multi-Region**: Distributed load generation
5. **Real-Time Analytics**: Stream processing of metrics
6. **Auto-Tuning**: Self-adjusting parameters
7. **Replay Mode**: Reproduce specific scenarios
8. **Comparative Analysis**: A/B testing support

### Optimization Opportunities
- Connection pooling
- Transaction batching
- Parallel verification
- Memory pooling
- Zero-copy operations
- SIMD acceleration

## Summary
The load generator provides flexible, scalable transaction generation with two primary modes optimized for different testing scenarios. Blocking mode ensures accuracy and verification, while non-blocking mode maximizes throughput. The modular design allows easy extension and customization for various testing requirements.