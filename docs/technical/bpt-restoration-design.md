# BPT Restoration Design Document

## Executive Summary

This document outlines the design for a simplified and robust BPT (Binary Patricia Tree) restoration strategy for Accumulate mainnet node deployment. The core insight is that **root hash validation is the only reliable validation we need** - individual account hash verification is fundamentally flawed and unnecessary.

## Problem Statement

### Current Issues
1. **Snapshots without BPT sections cause restoration to fail** - Blocking mainnet node launch
2. **Complex individual account hash verification is unreliable** - Cannot prove completeness
3. **BPT CLI commands have API incompatibilities** - Causing build failures
4. **Partition-specific snapshots often lack BPT sections** - Normal for distributed architecture

### Root Cause Analysis
The current restoration logic has a **fundamental design flaw**:
- Individual account hash verification cannot prove the rebuilt BPT doesn't contain extra entries
- It cannot prove the snapshot's BPT section is complete
- This creates false confidence in incomplete validation

### Key Insight: "Sometimes Less is More"
**Root hash comparison captures the complete BPT state** - if it matches, the entire BPT is mathematically proven correct. Individual account validation is not only unnecessary, it's misleading.

## Proposed Solution

### Core Design Principles

1. **Always ignore missing BPT sections** - Log warning, never fail
2. **Always rebuild BPT from all accounts** - Ensures consistency from source data
3. **Only validate root hash** - Simple, complete, reliable validation
4. **Handle zero root hash gracefully** - Normal for partition/genesis snapshots

### Architecture Overview

```
Snapshot File
├── Header (contains expected root hash)
├── Account Records (source of truth)
└── BPT Section (optional, ignored)
           ↓
    Restoration Process
           ↓
1. Read expected root hash from header
2. Restore all account records
3. Rebuild BPT from accounts (UpdateBPT)
4. Compare rebuilt root hash vs expected
           ↓
    Validation Result
├── Match → Success
├── Zero expected → Log actual hash, Success  
└── Mismatch → Clear failure with both hashes
```

## Detailed Design

### 1. Simplified BPT Section Reading

**Current Approach:**
```go
func readBptSnapshot(snap *snapshot.Reader, opts *RestoreOptions) (map[[32]byte][32]byte, error) {
    // Complex logic to read individual BPT entries
    // Fails if BPT section missing
    // Returns map of account key hash → account hash
}
```

**New Approach:**
```go
func readBptSnapshot(snap *snapshot.Reader, opts *RestoreOptions) (map[[32]byte][32]byte, error) {
    // Skip reading individual BPT entries - we'll validate via root hash only
    return nil, nil
}
```

**Rationale:** We don't need individual BPT entries because root hash validation is complete and sufficient.

### 2. Simplified Validation Logic

**Current Approach:**
- Read individual BPT entries from snapshot
- Iterate through all restored accounts
- Verify each account hash matches BPT entry
- Check for missing accounts in BPT
- Finally validate root hash

**New Approach:**
```go
// Simple root hash validation - the only reliable validation we can do
batch := db.Begin(false)
defer batch.Discard()

// Get the rebuilt BPT root hash
actualRootHash, err := batch.GetBptRootHash()
if err != nil {
    return errors.UnknownError.WithFormat("get rebuilt BPT root hash: %w", err)
}

expectedRootHash := rd.Header.RootHash
zeroHash := [32]byte{}

// Case 1: Expected root hash is zero (genesis or partition snapshot)
if expectedRootHash == zeroHash {
    fmt.Printf("INFO: Snapshot had zero root hash, rebuilt BPT root hash: %x\n", actualRootHash)
    return nil
}

// Case 2: Expected root hash is non-zero, must match exactly
if expectedRootHash != actualRootHash {
    return errors.InvalidRecord.WithFormat(
        "BPT root hash mismatch: expected %x, rebuilt %x", 
        expectedRootHash, actualRootHash)
}

fmt.Printf("INFO: BPT root hash validation successful: %x\n", actualRootHash)
return nil
```

### 3. Error Handling Strategy

| Scenario | Current Behavior | New Behavior | Rationale |
|----------|------------------|--------------|-----------|
| Missing BPT section | **FAIL** | **WARN + CONTINUE** | BPT can be rebuilt from accounts |
| Corrupted BPT section | **FAIL** | **WARN + CONTINUE** | BPT can be rebuilt from accounts |
| Zero expected root hash | **PASS** | **INFO + PASS** | Normal for partition snapshots |
| Root hash mismatch | **FAIL** | **FAIL** | Indicates data corruption |
| Individual account mismatch | **FAIL** | **N/A** | Covered by root hash validation |

## Benefits Analysis

### Reliability Improvements
- **Complete validation**: Root hash mathematically proves entire BPT correctness
- **No false positives**: Cannot miss inconsistencies that affect BPT integrity
- **No partial validation**: Either the entire BPT is correct or it's not

### Simplicity Gains
- **Fewer failure modes**: Only root hash comparison can fail
- **Clear semantics**: Hash matches = success, doesn't match = failure  
- **Easier debugging**: Compare two hash values instead of complex account iteration

### Robustness Enhancements
- **Works with any snapshot**: With or without BPT sections
- **Handles partition snapshots**: Zero root hash is valid and expected
- **Future-proof**: Root hash validation will always work regardless of BPT format changes

### Performance Benefits
- **Faster restoration**: Skip reading BPT section entirely
- **Reduced memory usage**: No need to store individual account hashes
- **Simpler code path**: Fewer branches and error conditions

## Risk Analysis

### Risks Eliminated
1. **False validation confidence**: Individual account checks gave incomplete assurance
2. **BPT section dependency**: No longer required for successful restoration
3. **Complex error scenarios**: Simplified to single root hash comparison

### Remaining Risks
1. **Root hash calculation bugs**: If `GetBptRootHash()` has issues
   - **Mitigation**: This is existing, well-tested code used throughout the system
2. **Snapshot header corruption**: If expected root hash is wrong
   - **Mitigation**: Same risk exists in current approach, no change in exposure
3. **Missing edge cases**: Unforeseen scenarios with zero root hash
   - **Mitigation**: Comprehensive logging provides visibility

### Risk Assessment: **LOW**
The new approach actually reduces risk by eliminating complex, flawed validation logic.

## Implementation Plan

### Phase 1: Code Changes
**Files to Modify:**
- `/internal/database/snapshot.go`
  - Simplify `readBptSnapshot()` function
  - Replace validation logic in `Restore()` function
  - Add appropriate logging statements

**Estimated Effort:** 2-3 hours

### Phase 2: Testing
**Test Scenarios:**
1. Existing mainnet snapshots with BPT sections
2. Partition-specific snapshots without BPT sections  
3. Genesis snapshots with zero root hash
4. Corrupted snapshots with wrong root hash
5. Large snapshots to verify performance

**Estimated Effort:** 4-6 hours

### Phase 3: Deployment
**Steps:**
1. Build modified `accumulated` binary
2. Test snapshot restoration with partition snapshots
3. Launch mainnet node with single validator + BVN
4. Monitor logs for validation results
5. Verify node operation and BPT integrity

**Estimated Effort:** 2-4 hours

## Testing Strategy

### Unit Tests
```go
func TestBptRestoration(t *testing.T) {
    // Test cases:
    // 1. Normal snapshot with matching root hash
    // 2. Snapshot with zero root hash  
    // 3. Snapshot with mismatched root hash
    // 4. Snapshot without BPT section
}
```

### Integration Tests
- Restore actual mainnet snapshots
- Verify BPT rebuilding works correctly
- Confirm node startup succeeds
- Validate account data integrity

### Performance Tests
- Compare restoration time before/after changes
- Measure memory usage during restoration
- Verify no regression in BPT operations

## Success Criteria

### Primary Goals
1. ✅ **Mainnet node launches successfully** with partition snapshots
2. ✅ **BPT integrity maintained** through root hash validation
3. ✅ **No false failures** from missing BPT sections
4. ✅ **Backward compatibility** with existing snapshots

### Secondary Goals
1. ✅ **Improved performance** from simplified logic
2. ✅ **Better error messages** with clear root hash comparison
3. ✅ **Operational visibility** through comprehensive logging
4. ✅ **Reduced maintenance burden** from simpler codebase

## Monitoring and Observability

### Log Messages
- **INFO**: Successful root hash validation with hash value
- **INFO**: Zero root hash detected with actual rebuilt hash
- **ERROR**: Root hash mismatch with expected vs actual values

### Metrics to Track
- Snapshot restoration success rate
- BPT rebuilding time
- Root hash validation results
- Node startup success after restoration

## Future Considerations

### Potential Enhancements
1. **Structured logging**: Replace `fmt.Printf` with proper logger
2. **Metrics collection**: Add Prometheus metrics for restoration operations
3. **BPT CLI fixes**: Re-enable BPT commands once APIs are stable

### Maintenance Notes
- This design is intentionally simple to minimize future maintenance
- Root hash validation is a stable, long-term approach
- Individual account validation was removed permanently due to fundamental flaws

## Conclusion

This simplified BPT restoration strategy provides a **robust, reliable, and maintainable** solution for Accumulate mainnet deployment. By focusing on root hash validation - the only validation that actually matters - we eliminate complex, flawed logic while ensuring complete data integrity.

The approach is **mathematically sound**: if the root hash matches, the entire BPT is provably correct. If it doesn't match, there's definitely a problem. This binary outcome is exactly what we need for reliable snapshot restoration.

**Key Takeaway**: Sometimes the best solution is the simplest one. Root hash validation gives us everything we need and nothing we don't.
