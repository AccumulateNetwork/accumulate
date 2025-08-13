# Gap Request Feature Design

## Overview
The Gap Request feature enables automatic detection and recovery of missing messages (gaps) in cross-partition communication. When a partition detects missing sequence numbers, it immediately requests the missing messages from the source partition.

## Current State Analysis

### Existing Components
1. **SimpleSequenceTracker** (`sequence_tracker_simple.go`)
   - Already detects gaps in sequences
   - Tracks gaps per partition and message type
   - Calls `SendRecoveryRequest()` but implementation is incomplete
   - Drops out-of-order messages (immediately requests recovery)

2. **RecoveryManager** (`recovery_core.go`)
   - Has infrastructure for recovery requests
   - Manages recovery sessions
   - Has `RequestMissingTransactions()` method

3. **BatchProofRecoveryManager** (`types.go`)
   - Handles batch recovery with collection proofs
   - Processes recovery requests asynchronously

### Current Gap Detection Flow
1. Message arrives with sequence N
2. Expected sequence is M (where M < N)
3. Gap detected: [M, N-1]
4. Message N is dropped
5. Recovery request initiated (but not fully implemented)

## Proposed Design

### 1. Gap Request Message Type
Create a new message type for gap requests that can be sent between partitions:

```go
// GapRequest - Request missing messages from a source partition
type GapRequest struct {
    // Requesting partition ID
    Requester string
    
    // Source partition that has the messages
    Source string
    
    // Type of messages missing (Synthetic, Anchor, etc.)
    MessageType MessageType
    
    // Range of missing sequences (inclusive)
    StartSequence uint64
    EndSequence uint64
    
    // Request ID for tracking
    RequestID string
    
    // Timestamp of request
    RequestedAt time.Time
    
    // Priority (higher = more urgent)
    Priority int
}

// GapResponse - Response containing the missing messages
type GapResponse struct {
    // Original request ID
    RequestID string
    
    // Messages found in the gap
    Messages []messaging.Message
    
    // Sequences included
    Sequences []uint64
    
    // If using collection proof
    CollectionProof *protocol.AnnotatedReceipt
    
    // Error if request failed
    Error string
}
```

### 2. Enhanced Gap Request Handler
Add handler in conductor to process incoming gap requests:

```go
// HandleGapRequest processes incoming gap requests from other partitions
func (cc *CrossChainConductor) HandleGapRequest(ctx context.Context, req *GapRequest) error {
    // Validate request
    // Retrieve messages from database
    // Create collection proof if applicable
    // Send response back to requester
}
```

### 3. Gap Request Sender
Enhance the sequence tracker to send proper gap requests:

```go
// SendGapRequest sends a request for missing messages
func (st *SimpleSequenceTracker) SendGapRequest(
    source string, 
    msgType MessageType, 
    gapStart, gapEnd uint64,
) error {
    // Create GapRequest message
    // Send via dispatcher to source partition
    // Track pending request
}
```

### 4. Gap Response Handler
Process responses containing the missing messages:

```go
// HandleGapResponse processes gap response from source partition
func (cc *CrossChainConductor) HandleGapResponse(ctx context.Context, resp *GapResponse) error {
    // Validate response
    // Process messages in order
    // Update sequence tracker
    // Clear gap from tracking
}
```

## Implementation Plan

### Phase 1: Message Types and Infrastructure
1. Define `GapRequest` and `GapResponse` message types
2. Add message type constants
3. Register handlers in conductor

### Phase 2: Gap Request Sending
1. Implement `SendGapRequest` in sequence tracker
2. Add request tracking (timeout, retries)
3. Integration with dispatcher

### Phase 3: Gap Request Handling
1. Implement `HandleGapRequest` in conductor
2. Add database queries to retrieve missing messages
3. Create collection proofs for batches

### Phase 4: Gap Response Processing
1. Implement `HandleGapResponse` 
2. Process messages in correct order
3. Update sequence tracker state
4. Clear resolved gaps

### Phase 5: Testing and Optimization
1. Unit tests for gap detection
2. Integration tests for request/response flow
3. Performance optimization for large gaps
4. Add metrics for gap recovery

## Detailed File Modifications

### 1. `types.go` - Add Message Types
```go
// Add to MessageType enum
const (
    MessageTypeGapRequest MessageType = iota + 100
    MessageTypeGapResponse
)

// GapRequest structure
type GapRequest struct {
    Requester     string
    Source        string
    MessageType   MessageType
    StartSequence uint64
    EndSequence   uint64
    RequestID     string
    RequestedAt   time.Time
    Priority      int
}

// GapResponse structure
type GapResponse struct {
    RequestID       string
    Messages        []messaging.Message
    Sequences       []uint64
    CollectionProof *protocol.AnnotatedReceipt
    Error           string
}
```

### 2. `sequence_tracker_simple.go` - Enhanced Gap Request
```go
// Replace SendRecoveryRequest with proper implementation
func (st *SimpleSequenceTracker) SendGapRequest(
    source string,
    messageType MessageType,
    gapStart, gapEnd uint64,
) error {
    requestID := fmt.Sprintf("%s-%s-%d-%d-%d", 
        st.conductor.Describe.PartitionId,
        source, messageType, gapStart, time.Now().UnixNano())
    
    req := &GapRequest{
        Requester:     st.conductor.Describe.PartitionId,
        Source:        source,
        MessageType:   messageType,
        StartSequence: gapStart,
        EndSequence:   gapEnd,
        RequestID:     requestID,
        RequestedAt:   time.Now(),
        Priority:      1, // Normal priority
    }
    
    // Send via dispatcher
    envelope := &messaging.Envelope{
        Messages: []messaging.Message{req},
    }
    
    sourceURL := protocol.PartitionUrl(source)
    err := st.conductor.dispatcher.Submit(context.Background(), sourceURL, envelope)
    
    if err != nil {
        st.logger.Error("Failed to send gap request",
            "source", source,
            "gap", fmt.Sprintf("[%d-%d]", gapStart, gapEnd),
            "error", err)
        return err
    }
    
    st.logger.Info("Sent gap request",
        "source", source,
        "type", messageType,
        "gap", fmt.Sprintf("[%d-%d]", gapStart, gapEnd),
        "request_id", requestID)
    
    // Track pending request
    st.trackPendingRequest(requestID, req)
    
    return nil
}
```

### 3. `conductor_recovery.go` - Gap Request Handler
```go
// HandleGapRequest processes incoming gap requests
func (cc *CrossChainConductor) HandleGapRequest(ctx context.Context, req *GapRequest) error {
    cc.logger.Info("Handling gap request",
        "requester", req.Requester,
        "type", req.MessageType,
        "range", fmt.Sprintf("[%d-%d]", req.StartSequence, req.EndSequence))
    
    // Validate request
    if req.EndSequence < req.StartSequence {
        return cc.sendGapErrorResponse(req, "invalid range")
    }
    
    gapSize := req.EndSequence - req.StartSequence + 1
    if gapSize > 100 {
        return cc.sendGapErrorResponse(req, "gap too large (max 100)")
    }
    
    // Retrieve messages from database
    messages, err := cc.retrieveMessages(ctx, req.MessageType, req.StartSequence, req.EndSequence)
    if err != nil {
        cc.logger.Error("Failed to retrieve messages for gap",
            "error", err,
            "request_id", req.RequestID)
        return cc.sendGapErrorResponse(req, err.Error())
    }
    
    // Create response
    resp := &GapResponse{
        RequestID: req.RequestID,
        Messages:  messages,
        Sequences: make([]uint64, len(messages)),
    }
    
    // Extract sequences
    for i, msg := range messages {
        if seqMsg, ok := msg.(*messaging.SequencedMessage); ok {
            resp.Sequences[i] = seqMsg.Number
        }
    }
    
    // Create collection proof if beneficial
    if len(messages) >= 2 && cc.proofService != nil {
        proof, err := cc.createCollectionProofForGap(ctx, messages, req)
        if err == nil {
            resp.CollectionProof = proof
        }
    }
    
    // Send response back to requester
    return cc.sendGapResponse(req.Requester, resp)
}

// HandleGapResponse processes gap response from source partition
func (cc *CrossChainConductor) HandleGapResponse(ctx context.Context, resp *GapResponse) error {
    cc.logger.Info("Received gap response",
        "request_id", resp.RequestID,
        "messages", len(resp.Messages),
        "has_proof", resp.CollectionProof != nil)
    
    // Check for error
    if resp.Error != "" {
        cc.logger.Error("Gap request failed",
            "request_id", resp.RequestID,
            "error", resp.Error)
        return errors.UnknownError.With(resp.Error)
    }
    
    // Validate and process messages in order
    for i, msg := range resp.Messages {
        // Validate sequence if available
        if i < len(resp.Sequences) {
            expectedSeq := resp.Sequences[i]
            if seqMsg, ok := msg.(*messaging.SequencedMessage); ok {
                if seqMsg.Number != expectedSeq {
                    cc.logger.Warn("Sequence mismatch in gap response",
                        "expected", expectedSeq,
                        "actual", seqMsg.Number)
                    continue
                }
            }
        }
        
        // Process the message
        err := cc.processRecoveredMessage(ctx, msg, resp.CollectionProof)
        if err != nil {
            cc.logger.Error("Failed to process recovered message",
                "error", err,
                "sequence", resp.Sequences[i])
        }
    }
    
    // Update sequence tracker to clear the gap
    if cc.sequenceTracker != nil {
        cc.sequenceTracker.ClearResolvedGap(resp.RequestID)
    }
    
    // Update metrics
    atomic.AddInt64(&cc.syntheticsRetried, int64(len(resp.Messages)))
    
    return nil
}
```

### 4. `conductor_inbound.go` - Message Routing
```go
// Add to ProcessInboundMessage
func (cc *CrossChainConductor) ProcessInboundMessage(ctx context.Context, msg messaging.Message) error {
    switch m := msg.(type) {
    case *GapRequest:
        return cc.HandleGapRequest(ctx, m)
    case *GapResponse:
        return cc.HandleGapResponse(ctx, m)
    // ... existing cases
    }
}
```

### 5. Tests - `gap_request_test.go`
```go
func TestGapRequestFlow(t *testing.T) {
    // Test gap detection triggers request
    // Test gap request handling
    // Test gap response processing
    // Test gap closure in sequence tracker
    // Test collection proof in gap response
    // Test error cases (invalid range, too large, etc.)
}
```

## Configuration Parameters

Add configurable parameters:
```go
type GapRequestConfig struct {
    // Maximum gap size to request (larger = potential attack)
    MaxGapSize int
    
    // Timeout for gap requests
    RequestTimeout time.Duration
    
    // Maximum retries for failed requests
    MaxRetries int
    
    // Delay between retries
    RetryDelay time.Duration
    
    // Enable collection proofs in responses
    UseCollectionProofs bool
}
```

## Metrics to Track

1. `gap_requests_sent` - Total gap requests sent
2. `gap_requests_received` - Total gap requests received
3. `gap_responses_sent` - Total gap responses sent
4. `gap_responses_received` - Total gap responses received
5. `gaps_recovered` - Successfully recovered gaps
6. `gap_recovery_time` - Time to recover gaps
7. `gap_recovery_failures` - Failed gap recoveries
8. `messages_recovered` - Total messages recovered through gaps

## Security Considerations

1. **Rate Limiting**: Limit gap requests per source to prevent DoS
2. **Gap Size Limits**: Maximum 100 messages per gap request
3. **Authentication**: Verify requester is valid partition
4. **Timeout**: Abandon gap requests after timeout
5. **Deduplication**: Prevent duplicate gap requests

## Performance Optimizations

1. **Batch Requests**: Combine multiple small gaps into one request
2. **Collection Proofs**: Use for 2+ messages in response
3. **Caching**: Cache recent gap responses for quick re-sends
4. **Priority Queue**: Process high-priority gaps first
5. **Async Processing**: Non-blocking gap request/response handling

## Testing Strategy

1. **Unit Tests**:
   - Gap detection logic
   - Request/response message creation
   - Sequence validation

2. **Integration Tests**:
   - Full gap request/response flow
   - Multiple partition scenarios
   - Network failure recovery

3. **Load Tests**:
   - High volume gap scenarios
   - Concurrent gap requests
   - Large gap recovery

4. **Chaos Tests**:
   - Random message drops
   - Partition failures during recovery
   - Malformed gap requests

## Rollout Plan

1. **Phase 1**: Deploy with feature flag disabled
2. **Phase 2**: Enable in test environment
3. **Phase 3**: Enable for small gaps only (< 10 messages)
4. **Phase 4**: Gradually increase gap size limit
5. **Phase 5**: Full production deployment

## Success Criteria

- 95% of gaps recovered within 30 seconds
- No message loss due to gaps
- < 1% overhead from gap recovery traffic
- Zero deadlocks or race conditions
- Successful recovery from network partitions