# Unified Transport Layer Design

## Problem Statement
Currently, anchors and synthetic transactions use separate transport mechanisms:
- Synthetic transactions: Use `produceSynthetic()` with collection proof support via ProofService
- Anchors: Use `prepareAnchor()` without collection proof support

This separation leads to:
1. Code duplication
2. Inconsistent proof optimization (only synthetics have collection proofs)
3. Maintenance overhead
4. Different code paths for similar crosschain operations

## Proposed Solution: Unified Transport Layer

### Core Concept
Create a single transport layer that handles both anchors and synthetic transactions, with collection proof support for both types.

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Block Executor                        │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  produceSynthetic() ─┐                                  │
│                      ├──► UnifiedTransport.Send()       │
│  prepareAnchor() ────┘                                  │
│                                                          │
└─────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────┐
│                  Unified Transport Layer                 │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  • Message batching by destination                      │
│  • Collection proof generation for all types            │
│  • Consistent sequencing                                │
│  • Unified metrics and monitoring                       │
│                                                          │
└─────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────┐
│                      ProofService                        │
├─────────────────────────────────────────────────────────┤
│                                                          │
│  • Collection proofs for batches                        │
│  • Individual proofs for single messages                │
│  • Proof validation                                     │
│                                                          │
└─────────────────────────────────────────────────────────┘
```

## Implementation Plan

### Phase 1: Create Unified Message Interface
```go
// CrossChainMessage represents any message crossing partitions
type CrossChainMessage interface {
    GetDestination() *url.URL
    GetSequence() uint64
    GetType() MessageType  // Anchor or Synthetic
    GetPayload() messaging.Message
}

// MessageType identifies the crosschain message type
type MessageType int

const (
    MessageTypeSynthetic MessageType = iota
    MessageTypeAnchor
)
```

### Phase 2: Unified Transport Service
```go
type UnifiedTransport struct {
    proofService *ProofService
    conductor    *CrossChainConductor
    logger       logging.OptionalLogger
    metrics      *TransportMetrics
}

// Send handles both anchors and synthetic transactions
func (ut *UnifiedTransport) Send(ctx context.Context, messages []CrossChainMessage) error {
    // 1. Group by destination
    grouped := ut.groupByDestination(messages)
    
    // 2. Create collection proofs for each group
    for dest, group := range grouped {
        if len(group) >= collectionThreshold {
            proof := ut.createCollectionProof(group)
        } else {
            proof := ut.createIndividualProof(group[0])
        }
    }
    
    // 3. Send to destination
    return ut.sendToPartitions(grouped)
}
```

### Phase 3: Refactor Existing Code

1. **Update Block Executor**:
   - Modify `produceSynthetic()` to use UnifiedTransport
   - Modify `prepareAnchor()` to use UnifiedTransport
   - Remove duplicate proof generation code

2. **Update ProofService**:
   - Add support for anchor proofs in collection batches
   - Ensure ProofTypeUnified handles both types

3. **Update CrossChainConductor**:
   - Replace separate methods with unified `ProcessCrossChainMessages()`
   - Maintain backward compatibility with deprecation notices

### Phase 4: Testing Strategy

1. **Unit Tests**:
   - Test unified message batching
   - Test collection proof generation for mixed types
   - Test fallback to individual proofs

2. **Integration Tests**:
   - Test anchor + synthetic in same batch
   - Test cross-partition delivery
   - Test proof validation

3. **Performance Tests**:
   - Benchmark collection proof savings
   - Measure latency improvements
   - Test under high load

## Benefits

1. **Code Simplification**:
   - Single code path for all crosschain messages
   - Reduced maintenance burden
   - Easier to understand and debug

2. **Performance Improvements**:
   - Collection proofs for anchors (new capability)
   - Better batching across message types
   - Reduced network overhead

3. **Consistency**:
   - Same proof generation for all types
   - Unified metrics and monitoring
   - Consistent error handling

4. **Extensibility**:
   - Easy to add new message types
   - Pluggable proof strategies
   - Future protocol upgrades simplified

## Migration Path

1. **Phase 1** (Current PR): 
   - Collection proofs for synthetic transactions ✅
   - ProofService foundation ✅

2. **Phase 2** (Next PR):
   - Implement UnifiedTransport interface
   - Add anchor support to ProofService
   - Create unified Send() method

3. **Phase 3** (Following PR):
   - Migrate block executor to use UnifiedTransport
   - Deprecate old methods
   - Update tests

4. **Phase 4** (Future):
   - Remove deprecated code
   - Optimize based on production metrics
   - Add advanced batching strategies

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Breaking existing functionality | Maintain backward compatibility with deprecation path |
| Performance regression | Comprehensive benchmarking before deployment |
| Complex migration | Phased rollout with feature flags |
| Protocol compatibility | Ensure wire format remains unchanged |

## Success Metrics

- **Code Reduction**: 30-40% less code in transport layer
- **Performance**: 50%+ reduction in proof overhead for anchors
- **Reliability**: No increase in crosschain failures
- **Maintainability**: Single unified code path

## Conclusion

The unified transport layer will significantly simplify crosschain message handling while providing performance benefits through collection proofs for all message types. The phased implementation approach ensures a smooth transition without disrupting existing functionality.