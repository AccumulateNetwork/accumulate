# CrossChain Conductor - Consolidated Design

**Issue**: #3653  
**Branch**: `3653-add-a-crosschainconductor-process-for-coordinating-partitions`  
**Status**: Implementation Phase

## Core Purpose

The CrossChain Conductor (CCC) provides efficiency optimization for cross-partition message validation. It acts as a protective filter BEFORE consensus validation, reducing network overhead from O(n²) to O(1) without compromising security.

**Key Principle**: Efficiency optimization only - all security comes from consensus validation.

## Design Requirements

### NO QUEUEING Design
- When out-of-order message arrives, request missing messages by returning last received sequence number
- Sender resets to this sequence number, automatically sending missing messages next round
- No memory overhead from growing queues

### Collection Proof Strategy
- Generate collection proofs for outbound message sets
- Use collection proofs to validate inbound message sets
- 90%+ proof size reduction expected
- Failed sends automatically retried if sequence number unchanged

### Critical Fix Required
Collection proofs currently broken due to `nil` merkle state in `proof_service.go:303`:
```go
receiptList, err := merkle.GetReceiptList(nil, startIdx, endIdx) // TODO: Fix this
```
**Fix**: Add `Inner()` method to `database.Chain` to expose underlying `MerkleManager`

## Implementation Phases

1. **Fix Collection Proofs** (4 lines of code + tests)
2. **Add Sequence Tracking** (no queuing, just last received/sent numbers)  
3. **Add Gap Healing** (request missing sets via sequence numbers)
4. **Integration** (executor + API layers)

## Success Criteria
- Collection proofs functional (90%+ size reduction)
- No message queuing (bounded memory usage)
- Network overhead reduction (O(n²) → O(1))
- Automatic retry without tracking
- Security maintained (consensus still validates everything)