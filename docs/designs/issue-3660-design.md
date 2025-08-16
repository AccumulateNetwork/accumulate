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
- No configuration flags for enable/disable

## Design Specification

### Architecture Overview
The collection proof system groups ALL transactions by destination and creates a single proof for multiple transactions. This involves:
1. ProofService - Centralized proof construction and validation
2. All transactions use collection proofs (no threshold checking)
3. Recovery handled by destination gap request (no recovery sessions)
4. Metrics tracking for monitoring

### Component Definitions

#### Affected Files
```
internal/core/execute/v2/crosschain/conductor.go           # Enable collection proofs
internal/core/execute/v2/crosschain/proof_service.go       # Fix race conditions, add batching
internal/core/execute/v2/crosschain/types.go              # Add ProofBatch type
```

#### New Components
```
internal/core/execute/v2/crosschain/proof_metrics.go       # Thread-safe metrics
```

### API Contracts

#### Configuration Structure
```go
type ConductorConfig struct {
    // Existing fields...
    
    // Collection Proof Configuration (simplified)
    EnableCollectionProofs    bool          // Always true - all transactions use collection proofs
    CollectionMaxBatchSize    int           // Max transactions per collection (default: 100)
}
```

#### Core Functions
```go
// Batch ALL transactions for proof creation (no threshold)
func (cc *CrossChainConductor) batchTransactionsForProof(
    messages []messaging.Message,
) []ProofBatch

// Create collection proof (no timeout needed)
func (ps *ProofService) CreateCollectionProof(
    batch ProofBatch,
) (*ProofResponse, error)

// Transmission with recovery via gap detection
func (cc *CrossChainConductor) transmitWithGapRecovery(
    proof *ProofResponse,
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
2. Group ALL transactions by destination (no threshold check)
3. Generate collection proof (no timeout)
4. Transmit collection proof
5. On transmission failure: do NOT update last index sent (enables recovery)
6. Destination gap request handles recovery automatically
7. Update metrics atomically

### Error Handling
- **Race Condition**: Use atomic operations for all metric updates
- **Transmission Failure**: Do NOT update last index sent, allows automatic recovery
- **Gap Detection**: Destination handles recovery via gap requests
- **No fallback**: All transactions use collection proofs

### Testing Requirements

#### Local Testing (Against Devnet)
Local tests must run against a real devnet to ensure we test the same code paths as production:
- [ ] Integration tests against local devnet
- [ ] Load tests at 100 TPS on devnet
- [ ] Track both cross-chain and intra-chain transactions
- [ ] Verify gap recovery mechanism works
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
- [ ] Enable collection proofs for ALL transactions
- [ ] Implement batching logic (no threshold)
- [ ] Handle transmission failures without updating last index
- [ ] Implement gap recovery mechanism
- [ ] Add comprehensive metrics
- [ ] Update documentation
- [ ] Add monitoring dashboards

## Acceptance Criteria
1. All race conditions eliminated (verified with -race flag)
2. All transactions use collection proofs (no exceptions)
3. Gap recovery mechanism works automatically
4. Collection proof success rate > 99%
5. Metrics accurately track all operations
6. Documentation complete
7. 100 TPS sustained on devnet

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

# Run load test against devnet (100 TPS target)
go test -tags=devnet ./test/load/... \
  -run TestCollectionProofLoad \
  -devnet.url=http://localhost:26657 \
  -load.tps=100 \
  -load.duration=1h \
  -track.crosschain=true \
  -track.intrachain=true

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
- Collection proof generation: < 100ms for batch
- Memory usage: Stable over 24 hours
- CPU overhead: < 5% increase
- Success rate: > 99% for collection proofs
- Throughput: Support 100 TPS sustained

## Rollout Plan
1. **Testing**: Enable in test environment (all transactions use collection proofs)
2. **Staging**: Run load tests at 100 TPS
3. **Production**: 
   - Full rollout at 100% (no gradual increase)
   - Monitor for 1 week
   - New protocol fully enabled from start

## Change Log
- 2025-08-16: Simplified design: ALL transactions use collection proofs, no thresholds, no fallback, recovery via gap detection, 100 TPS target

- 2025-08-16: Updated testing strategy: local tests use devnet for production parity, CI tests use simulators for speed

- 2025-08-16: Initial design created
- 2025-08-16: Added detailed specifications from sub-issue analysis