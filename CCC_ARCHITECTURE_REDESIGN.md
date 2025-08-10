# CrossChainConductor Architecture Redesign

## Current Problem
The CCC is currently positioned AFTER messages have been accepted by CometBFT, which means:
1. Invalid/out-of-sequence messages still consume network bandwidth
2. Invalid messages enter the mempool and consume consensus resources
3. We can't prevent bad messages from being broadcast to the network

## Proposed Architecture

### Important Note: Defense in Depth
The CCC provides early validation for **efficiency**, not security. Consensus-level validation remains mandatory since nodes can be compromised. The CCC acts as a protective filter to reduce network overhead and centralize queue management, but is NOT a security boundary.

### Sending Side - CCC as Gatekeeper
The CCC should intercept synthetic messages BEFORE they are sent over the network:

```
Transaction Execution
    ↓
produceSynthetic() 
    ↓
**CCC.ValidateOutbound()**  ← NEW: Pre-flight validation
    - Check sequence numbers
    - Validate destination state
    - Queue if destination is not ready
    ↓
dispatcher.Submit() [only if CCC approves]
    ↓
Network transmission
```

### Receiving Side - CCC as Filter
The CCC should intercept messages BEFORE they enter CometBFT:

```
API receives message
    ↓
**CCC.ValidateInbound()**  ← NEW: Pre-consensus validation
    - Verify sequence number is next expected
    - Check if message is duplicate
    - Validate proof/anchor if required
    ↓
Submit to CometBFT [only if valid]
    ↓
CheckTx → DeliverTx → Block execution
```

## Implementation Points

### 1. Modify Dispatcher Integration
Replace direct dispatcher usage with CCC-mediated dispatch:

```go
// Instead of:
dispatcher.Submit(ctx, dest, envelope)

// Use:
ccc.SubmitOutbound(ctx, dest, envelope)
```

### 2. Add API-Level Interceptor
Intercept at the Submitter level before CometBFT submission:

```go
// In internal/api/v3/tm/submitter.go
func (s *Submitter) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
    // NEW: Check with CCC first
    if isCrossPartition(envelope) {
        if err := s.ccc.ValidateInbound(ctx, envelope); err != nil {
            return nil, err // Reject before it hits CometBFT
        }
    }
    
    // Continue with normal submission...
}
```

### 3. CCC State Management
The CCC needs to maintain:

#### On Sending Side:
- **Per-destination sequence tracking**: What sequence number should we send next?
- **Destination readiness**: Is the destination ready for our next message?
- **Outbound queue**: Messages waiting to be sent

#### On Receiving Side:
- **Per-source sequence tracking**: What sequence number do we expect next?
- **Pending messages**: Out-of-sequence messages waiting for gaps to fill
- **Proof validation cache**: Recently validated proofs to avoid re-validation

### 4. Sequence Number Validation

```go
func (cc *CrossChainConductor) ValidateInbound(ctx context.Context, envelope *messaging.Envelope) error {
    messages, _ := envelope.Normalize()
    
    for _, msg := range messages {
        if seq, ok := msg.(*messaging.SequencedMessage); ok {
            // Load expected sequence number
            expected := cc.getExpectedSequence(seq.Source)
            
            // Reject if too old
            if seq.Number <= cc.getDeliveredHeight(seq.Source) {
                return errors.Delivered.WithFormat(
                    "sequence %d already delivered (height: %d)", 
                    seq.Number, cc.getDeliveredHeight(seq.Source))
            }
            
            // Reject if too far ahead (prevent DoS)
            if seq.Number > expected + MAX_SEQUENCE_GAP {
                return errors.BadRequest.WithFormat(
                    "sequence %d too far ahead (expected: %d)", 
                    seq.Number, expected)
            }
            
            // Queue if out of order but within acceptable range
            if seq.Number != expected {
                cc.queueForLater(seq)
                return errors.Pending.WithFormat(
                    "sequence %d queued (expected: %d)", 
                    seq.Number, expected)
            }
        }
    }
    
    return nil // Message is valid and in sequence
}
```

## Benefits of This Design

1. **Network Efficiency**: Invalid messages never leave the source partition (O(n²) → O(1) overhead)
2. **Consensus Efficiency**: Invalid messages never enter CometBFT mempool
3. **Queue Centralization**: Single queue point instead of every node maintaining queues (O(n) → O(1) memory)
4. **Early Rejection**: Problems detected and handled at the earliest possible point
5. **Better DoS Protection**: Can rate-limit and validate before expensive consensus operations
6. **Cleaner Architecture**: Clear separation between validation and execution

**Note**: These are efficiency benefits. Security still requires full consensus validation since nodes can be compromised.

## Migration Path

### Phase 1: Add Outbound Validation (Current Phase)
- CCC observes but doesn't block
- Collect metrics on what would be rejected

### Phase 2: Enforce Outbound Validation
- CCC prevents invalid messages from being sent
- Still accepts all inbound messages

### Phase 3: Add Inbound Pre-Validation
- CCC validates before CometBFT submission
- Reject invalid messages at API level

### Phase 4: Full Enforcement
- Both inbound and outbound validation enforced
- Complete protection against invalid cross-partition messages

## Key Changes Required

1. **New Dispatcher Factory**: 
   - Executor creates CCC-wrapped dispatcher instead of direct dispatcher
   
2. **API Integration**:
   - Submitter service needs CCC reference
   - Check cross-partition messages before CometBFT submission

3. **State Persistence**:
   - CCC needs to persist sequence state across restarts
   - Consider using database for sequence tracking

4. **Recovery Mechanism**:
   - Handle cases where sequences get out of sync
   - Provide manual intervention tools for operators

## Testing Considerations

1. **Sequence Gap Handling**: Test behavior when messages arrive out of order
2. **Partition Failure**: Test recovery when a partition goes down and comes back
3. **Network Partitions**: Test behavior during network splits
4. **Performance**: Ensure CCC doesn't become a bottleneck
5. **State Recovery**: Test CCC state recovery after restart