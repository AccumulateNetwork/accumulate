# Issue Design Document: #3660

## Issue Summary
- **Issue ID**: #3660
- **Title**: Activate Collection Proofs in CrossChain Conductor
- **Status**: Draft
- **Created**: 2025-08-16
- **Last Updated**: 2025-08-16
- **Parent Issue**: #3659 (Master CrossChain Conductor Implementation)

## Problem Statement
The CrossChain Conductor currently processes proofs individually for each transaction, creating significant overhead. Collection proofs can batch multiple transactions into a single proof, reducing overhead by up to 13.2x based on testing. However, the existing implementation has critical issues:
- Race conditions in metrics updates
- Memory leaks in recovery sessions  
- Missing context cancellation
- No configuration flags for enable/disable

## Design Specification

### Architecture Overview
The collection proof system groups transactions by destination and creates a single proof for multiple transactions. This involves:
1. ProofService - Centralized proof construction and validation
2. BatchProofRecoveryManager - Manages batch proof recovery
3. Configuration flags for gradual rollout
4. Metrics tracking for monitoring

### Component Definitions

#### Affected Files
```
internal/core/execute/v2/crosschain/conductor.go           # Add configuration flags
internal/core/execute/v2/crosschain/proof_service.go       # Fix race conditions, add batching
internal/core/execute/v2/crosschain/batch_proof_recovery.go # Fix memory leaks
internal/core/execute/v2/crosschain/types.go              # Add ProofBatch type
```

#### New Components
```
internal/core/execute/v2/crosschain/proof_metrics.go       # Thread-safe metrics
internal/core/execute/v2/crosschain/proof_config.go        # Configuration management
```

### API Contracts

#### Configuration Structure
```go
type ConductorConfig struct {
    // Existing fields...
    
    // Collection Proof Configuration
    EnableCollectionProofs    bool          // Feature flag (default: false)
    CollectionBatchThreshold  int           // Min transactions for collection (default: 2)
    CollectionMaxBatchSize    int           // Max transactions per collection (default: 100)
    CollectionProofTimeout    time.Duration // Timeout for proof generation (default: 5s)
    CollectionRetryAsIndividual bool        // Retry failed collections as individual (default: true)
}
```

#### Core Functions
```go
// Batch transactions for proof creation
func (cc *CrossChainConductor) batchTransactionsForProof(
    messages []messaging.Message,
) []ProofBatch

// Create collection proof with proper synchronization
func (ps *ProofService) CreateCollectionProof(
    ctx context.Context, 
    batch ProofBatch,
) (*ProofResponse, error)

// Process batch recovery with cleanup
func (brm *BatchProofRecoveryManager) processBatchRecovery(
    ctx context.Context,
    req *BatchRecoveryRequest,
) error
```

#### Data Structures
```go
type ProofBatch struct {
    Destination   *url.URL
    Requests      []ProofRequest
    UseCollection bool
}

type ProofResponse struct {
    Proof        *protocol.AnnotatedReceipt
    ProofType    ProofType
    Sequences    []uint64
    IsCollection bool
    ProofSavings int  // Number of individual proofs saved
}

type ProofMetrics struct {
    // Use atomic operations for all counters
    IndividualProofsCreated   atomic.Int64
    CollectionProofsCreated   atomic.Int64
    TransactionsInCollections atomic.Int64
    ProofsSaved               atomic.Int64
}
```

### Data Flow
1. Transactions arrive for cross-partition transmission
2. Group transactions by destination
3. Check if batch size >= threshold for collection proof
4. Generate collection proof (with timeout)
5. On success: transmit collection proof
6. On failure: fallback to individual proofs if configured
7. Update metrics atomically
8. Clean up recovery sessions

### Error Handling
- **Race Condition**: Use atomic operations for all metric updates
- **Memory Leak**: Add defer cleanup for all recovery sessions
- **Context Timeout**: Add context with timeout for proof generation
- **Collection Failure**: Automatic fallback to individual proofs
- **Validation Failure**: Retry with individual proofs

### Testing Requirements

#### Local Testing (Against Devnet)
Local tests must run against a real devnet to ensure we test the same code paths as production:
- [ ] Integration tests against local devnet
- [ ] Load tests at 10,000 TPS on devnet
- [ ] Memory leak tests (24-hour run) on devnet
- [ ] Failure scenario tests with real network conditions
- [ ] Performance benchmarks with actual network latency
- [ ] End-to-end transaction flow validation on devnet

#### CI/CD Testing (Simulators and Mocks)
Automated CI tests use simulators and mock networks for speed and consistency:
- [ ] Unit tests with -race flag for race detection
- [ ] Mock-based collection proof tests
- [ ] Simulator-based integration tests
- [ ] Metrics accuracy validation with mocks
- [ ] Context cancellation tests
- [ ] Configuration flag tests
- [ ] Fast failure scenario tests

## Implementation Checklist
- [ ] Fix race conditions with atomic operations
- [ ] Add defer cleanup for recovery sessions
- [ ] Implement context timeout for proof generation
- [ ] Add configuration flags and validation
- [ ] Implement batching logic
- [ ] Create fallback mechanism
- [ ] Add comprehensive metrics
- [ ] Update documentation
- [ ] Add monitoring dashboards

## Acceptance Criteria
1. All race conditions eliminated (verified with -race flag)
2. No memory leaks in 24-hour test
3. 10x performance improvement achieved
4. Collection proof success rate > 99%
5. Automatic fallback works correctly
6. Metrics accurately track all operations
7. Feature flags enable gradual rollout
8. Documentation complete

## Testing Strategy

### Local Development Testing
Local testing uses a real devnet to ensure production parity:

```bash
# Start local devnet (3 validators, 2 partitions)
./scripts/devnet.sh start --validators 3 --partitions 2

# Run integration tests against devnet
go test -tags=devnet ./internal/core/execute/v2/crosschain/... \
  -run TestCollectionProofs \
  -devnet.url=http://localhost:26657

# Run load test against devnet
go test -tags=devnet ./test/load/... \
  -run TestCollectionProofLoad \
  -devnet.url=http://localhost:26657 \
  -load.tps=10000 \
  -load.duration=1h

# Run 24-hour memory test
go test -tags=devnet ./internal/core/execute/v2/crosschain/... \
  -run TestCollectionProofMemory \
  -timeout=24h \
  -memprofile=mem.prof \
  -devnet.url=http://localhost:26657
```

### CI/CD Testing
CI tests use simulators for speed and reproducibility:

```bash
# Unit tests with race detection (no network required)
go test -race ./internal/core/execute/v2/crosschain/...

# Simulator-based integration tests
go test ./test/simulator/... -run TestCollectionProofs

# Mock-based tests
go test ./internal/core/execute/v2/crosschain/... \
  -run TestCollectionProofMock
```

### Test Environment Differences

| Aspect | Local (Devnet) | CI/CD (Simulator) |
|--------|---------------|-------------------|
| Network | Real devnet process | In-memory simulator |
| Latency | Actual network delays | Zero latency |
| Consensus | Full consensus | Simulated consensus |
| Storage | Real database | In-memory storage |
| Speed | Slower (realistic) | Fast (mocked) |
| Purpose | Validate production behavior | Quick feedback |

## Performance Targets
- Collection proof generation: < 100ms for 10 transactions
- Memory usage: Stable over 24 hours
- CPU overhead: < 5% increase
- Success rate: > 99% for collection proofs
- Throughput: Support 10,000 TPS

## Rollout Plan
1. **Testing**: Enable in test environment with threshold=2
2. **Staging**: Run load tests at 5,000 TPS
3. **Production**: 
   - Start with 10% of partitions
   - Monitor for 1 week
   - Gradual increase to 100%

## Change Log
- 2025-08-16: Updated testing strategy: local tests use devnet for production parity, CI tests use simulators for speed

- 2025-08-16: Initial design created
- 2025-08-16: Added detailed specifications from sub-issue analysis