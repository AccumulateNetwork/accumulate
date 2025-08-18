# Accumulate Protocol v1.5.0-experimental Release Notes

## Release Date: August 9, 2025

## ⚠️ EXPERIMENTAL RELEASE - BREAKING CHANGES

## Overview
This experimental release introduces the CrossChainConductor, a major architectural change to Accumulate's cross-partition transaction processing. The new system provides centralized proof management, automatic collection proof optimization, and robust partition failure handling.

**WARNING: This release contains breaking changes and should be thoroughly tested before production use.**

## Major Features

### 🚀 CrossChainConductor
A new orchestration layer for managing cross-partition transactions with:
- **Asynchronous Processing**: Non-blocking transaction submission
- **Per-Destination Queue Management**: Independent queue for each partition
- **Circuit Breaker Pattern**: Automatic failure detection and recovery
- **Comprehensive Metrics**: Real-time monitoring and performance tracking

### ⚡ Centralized ProofService
Revolutionary proof management system featuring:
- **Collection Proof Optimization**: Automatic batching with 13.2x performance improvement
- **95% Memory Reduction**: Efficient proof aggregation for large transaction batches
- **NO CACHING Design**: Simplified testing and validation (caching can be added later)
- **Threshold-Based Batching**: Automatic collection proofs for 2+ transactions

### 🛡️ Partition Failure Handling
Robust failure management including:
- **Health Monitoring**: Real-time partition health tracking
- **Automatic Recovery**: Transaction recovery for failed partitions
- **Visual Monitoring**: Real-time lag visualization
- **Transaction Drop Policy**: Prevents queue buildup during failures

## Performance Improvements

### Collection Proof Metrics
- **Average Speedup**: 13.2x faster than individual proofs
- **Memory Usage**: 95% reduction (500 txs: 361KB → 17KB)
- **Proof Savings**: Thousands of individual proofs eliminated

### Load Test Results
- **Extended Load Test**: 100 requests, 100% success rate @ 46.87 req/sec
- **Sustained Load Test**: 2,135 requests over 2 minutes with 0 failures
- **Throughput**: 17.79 requests/second sustained

## Technical Details

### New Components
1. **CrossChainConductor** (`internal/core/execute/v2/crosschain/conductor.go`)
   - Central orchestrator for cross-partition messaging
   - Manages async processing and retry logic

2. **ProofService** (`internal/core/execute/v2/crosschain/proof_service.go`)
   - Centralized proof construction and validation
   - Automatic optimization by destination

3. **BatchProofRecoveryManager** (`test/load/batch_proof_recovery.go`)
   - Collection proof support for recovery operations
   - Efficient batch transaction recovery

### Configuration
```go
// ProofService Configuration
batchThreshold: 2     // Use collection proofs for 2+ transactions
maxBatchSize: 100     // Maximum transactions per collection
caching: DISABLED     // No caching for easier testing
debugMode: ENABLED    // Detailed logging for debugging
```

### API Changes
- New `CreateProofsForSyntheticTransactions()` method in CrossChainConductor
- New `ValidateIncomingProof()` for centralized validation
- New `RequestMissingTransactionsWithBatchProof()` for batch recovery

## Migration Guide

### For Node Operators
1. No configuration changes required - features are automatically enabled
2. Monitor new metrics for performance insights
3. Collection proofs activate automatically for 2+ transactions

### For Developers
1. Use CrossChainConductor for cross-partition messaging
2. Leverage ProofService for optimized proof generation
3. Monitor partition health via new APIs

## Testing
Comprehensive test suite including:
- Unit tests for all new components
- Integration tests with DevNet
- Performance benchmarks
- Visual monitoring tools
- Extended load testing (2x duration)

## Documentation
- [Design Document](test/load/PROOF_CENTRALIZATION_DESIGN_NO_CACHE.md)
- [Run Instructions](test/load/RUN_INSTRUCTIONS.md)
- [Visual Monitoring Guide](test/load/HOW_TO_RUN_VISUAL_TESTS.md)
- [Code Review Findings](test/load/CODE_REVIEW_COLLECTION_PROOFS.md)

## 🚨 BREAKING CHANGES

### Architecture Changes
1. **New CrossChainConductor Required**: The CrossChainConductor is now required for cross-partition messaging
2. **Proof Format Changes**: Collection proofs change the proof format for batched transactions
3. **API Changes**: New methods replace deprecated proof generation functions
4. **Message Flow**: Asynchronous message processing changes timing characteristics

### Migration Required
- Nodes must upgrade to support new proof formats
- Existing proof validation logic needs updates
- Client libraries require updates for new API methods

### Compatibility
- **NOT backward compatible** with v1.4.x nodes
- All nodes in a network must upgrade together
- Client applications may need updates

## Bug Fixes
- Fixed import cycles in proof validation
- Resolved connection exhaustion in V3 client
- Fixed race conditions in metrics collection

## Known Issues
- Visual monitor may timeout after 30 seconds (by design)
- DevNet required for full test suite execution

## Contributors
- Paul (Lead Developer)
- Claude AI Assistant (Development Support)

## Acknowledgments
Special thanks to the Accumulate community for their feedback and testing support.

---

## Upgrade Instructions

### From v1.4.x
```bash
# Pull the latest changes
git fetch origin
git checkout v1.5.0

# Build the new version
make build

# Run tests
make test

# Deploy
make deploy
```

### Verification
```bash
# Verify ProofService is active
curl -X POST http://localhost:26660/v2 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"metrics","params":{},"id":1}'

# Check for collection_proofs_created metric
```

## Support
For issues or questions:
- GitLab Issues: https://gitlab.com/accumulatenetwork/accumulate/issues
- Documentation: https://docs.accumulate.defi

---

**Full Changelog**: https://gitlab.com/accumulatenetwork/accumulate/compare/v1.4.3...v1.5.0