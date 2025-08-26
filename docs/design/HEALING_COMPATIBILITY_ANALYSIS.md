# Healing Compatibility Analysis for v1.5.0-experimental Upgrade

## Executive Summary
After analyzing the healing mechanisms in the current version and comparing them with the new ProofService/CrossChainConductor architecture in v1.5.0-experimental, I've identified several compatibility concerns and cleanup requirements that need to be addressed during upgrade.

## Current Healing Mechanisms (v1.4.x)

### 1. Anchor Healing (`internal/core/healing/anchors.go`)
- **Purpose**: Recovers missing or undelivered anchor transactions between partitions
- **Key Operations**:
  - Collects signatures from validators that haven't signed
  - Rebuilds anchor transactions with collected signatures
  - Two versions: healAnchorV1 (legacy) and healDnAnchorV2 (Vandenberg+)
  - Submits signatures as `BlockAnchor` messages

### 2. Synthetic Transaction Healing (`internal/core/healing/synthetic.go`)
- **Purpose**: Resubmits synthetic transactions that failed delivery
- **Key Operations**:
  - Builds Merkle receipts using `buildSynthReceiptV1` or `buildSynthReceiptV2`
  - Creates `SyntheticMessage` with proof and signature
  - Resubmits directly to destination partition
  - Falls back to `BadSyntheticMessage` for pre-Baikonur versions

## ProofService Changes in v1.5.0-experimental

### Key Differences
1. **Collection Proofs**: New system batches multiple transactions into single proofs
2. **Centralized Proof Management**: All proof creation goes through ProofService
3. **NO CACHING Design**: Current implementation has no caching (for easier testing)
4. **Automatic Batching**: Transactions to same destination are automatically batched (threshold: 2)

## Compatibility Issues Identified

### 1. Receipt Format Incompatibility
**Issue**: Healing mechanisms build traditional individual receipts, while ProofService may expect collection proofs for batched transactions.

**Impact**:
- Healed synthetic transactions with individual proofs may fail validation if ProofService expects collection proofs
- Mixed proof formats during transition period could cause validation failures

**Current Healing Receipt Building**:
```go
// buildSynthReceiptV2 creates individual receipts
receipt, err := batch.Account(uSrcSynth).MainChain().Receipt(seqEntry.Source, mainIndex.Source)
```

**ProofService Expectation**:
```go
// ProofService creates collection proofs for 2+ transactions
if len(batch.Requests) >= ps.batchThreshold {
    return ps.createCollectionProof(ctx, mergedRequest)
}
```

### 2. Mempool Cleanup During Upgrade
**Issue**: If mempool contains pending transactions during upgrade, they may not be compatible with new proof validation.

**Specific Concerns**:
- Synthetic transactions in flight with old proof format
- Anchors pending with old signature format
- Transactions that need re-proving with collection proofs

### 3. State Cleanup Requirements
**Issue**: Partially delivered transactions may leave orphaned state.

**Areas Requiring Cleanup**:
1. **Synthetic Sequence Chains**: May have gaps if healing was interrupted
2. **Pending Signatures**: Old format signatures that need conversion
3. **Anchor Chains**: Incomplete anchor sequences that need completion

## Migration Recommendations

### Pre-Upgrade Checklist
1. **Complete All Healing Operations**
   ```bash
   # Check for pending healing operations
   accumulate healing status
   
   # Complete any in-progress healing
   accumulate healing complete --wait
   ```

2. **Clear Mempool**
   ```bash
   # Ensure mempool is empty before upgrade
   accumulate mempool status
   accumulate mempool clear --force  # If necessary
   ```

3. **Verify Partition Synchronization**
   ```bash
   # Check all partitions are synchronized
   accumulate partition status --all
   ```

### During Upgrade

1. **Phased Rollout** (NOT RECOMMENDED for v1.5.0-experimental)
   - Due to breaking changes, all nodes must upgrade simultaneously
   - Cannot run mixed versions in same network

2. **Atomic Upgrade** (RECOMMENDED)
   - Stop all nodes
   - Upgrade all nodes to v1.5.0-experimental
   - Start all nodes together
   - Monitor for validation errors

### Post-Upgrade Verification

1. **Monitor Healing Operations**
   ```bash
   # Check if healing works with new ProofService
   accumulate healing test-synthetic
   accumulate healing test-anchor
   ```

2. **Verify Proof Validation**
   ```bash
   # Check both old and new proof formats work
   accumulate proof validate --format=individual
   accumulate proof validate --format=collection
   ```

3. **Clean Orphaned State**
   ```bash
   # Run cleanup for any orphaned state
   accumulate cleanup orphaned-state --dry-run
   accumulate cleanup orphaned-state --execute
   ```

## Specific Cleanup Tasks Required

### 1. Convert Pending Proofs
Transactions with old proof format need conversion to collection proofs where applicable:
```go
// Pseudo-code for conversion
for txn in pending_transactions {
    if txn.destination == same && count >= 2 {
        newProof = ProofService.CreateCollectionProof(transactions)
        replace(txn.proof, newProof)
    }
}
```

### 2. Re-heal Failed Transactions
Some transactions may need re-healing with new proof format:
```go
// Re-heal with ProofService
failedTxns := findFailedTransactions()
for batch in groupByDestination(failedTxns) {
    proof := ProofService.OptimizeForDestinations(batch)
    resubmit(batch, proof)
}
```

### 3. Update Signature Format
BlockAnchor signatures may need format updates for compatibility:
```go
// Update signature format if needed
for sig in pending_signatures {
    if sig.version == old {
        newSig = convertToNewFormat(sig)
        replace(sig, newSig)
    }
}
```

## Risk Assessment

### High Risk
1. **Mixed Proof Formats**: Validation failures during transition
2. **Mempool Incompatibility**: Pending transactions may fail
3. **Breaking Change**: All nodes must upgrade together

### Medium Risk
1. **Healing Interruption**: In-progress healing may fail
2. **State Orphaning**: Incomplete transactions may leave orphaned state
3. **Performance Impact**: Initial proof conversion overhead

### Low Risk
1. **Metrics Disruption**: Temporary metrics inconsistency
2. **Log Verbosity**: Increased logging during transition

## Recommendations

### CRITICAL Actions
1. **DO NOT upgrade with transactions in mempool**
2. **DO NOT upgrade during active healing operations**
3. **DO ensure all partitions are synchronized before upgrade**
4. **DO upgrade all nodes simultaneously (breaking change)**

### Best Practices
1. **Schedule Maintenance Window**: Plan for 2-4 hours
2. **Backup State**: Create full backup before upgrade
3. **Test in Staging**: Validate upgrade process in test environment
4. **Monitor Post-Upgrade**: Watch for validation errors for 24 hours

### Monitoring Commands
```bash
# Monitor proof validation
watch -n 5 'accumulate metrics | grep proof'

# Monitor healing operations  
watch -n 10 'accumulate healing status'

# Monitor mempool
watch -n 2 'accumulate mempool status'

# Monitor partition sync
watch -n 30 'accumulate partition lag'
```

## Conclusion

The upgrade to v1.5.0-experimental requires careful coordination due to breaking changes in proof format and validation. The main concerns are:

1. **Proof Format Compatibility**: Healing mechanisms use individual proofs while ProofService uses collection proofs
2. **Mempool State**: Must be empty before upgrade to avoid validation failures
3. **Simultaneous Upgrade**: All nodes must upgrade together (breaking change)

With proper preparation and the cleanup procedures outlined above, the upgrade can be completed successfully. The key is ensuring no transactions are in flight during the upgrade and all healing operations are complete.

## Next Steps
1. Implement pre-upgrade validation script
2. Create rollback procedure documentation
3. Test upgrade procedure in staging environment
4. Schedule maintenance window for production upgrade
5. Prepare monitoring dashboards for post-upgrade verification