# CCC Implementation Plan - Concrete Changes

## 1. Outbound Interception (Sending Side)

### Current Flow (BAD):
```go
// internal/core/execute/v2/block/synthetic.go:174
// Messages go directly to dispatcher without validation
func (m *Executor) produceSynthetic(batch *database.Batch, produced []*ProducedMessage, index uint64) error {
    // ...
    dispatcher := m.NewDispatcher()
    for _, p := range produced {
        // PROBLEM: Direct submission without validation
        err = dispatcher.Submit(ctx, p.Destination, envelope)
    }
}
```

### Proposed Change:
```go
// internal/core/execute/v2/block/synthetic.go
func (m *Executor) produceSynthetic(batch *database.Batch, produced []*ProducedMessage, index uint64) error {
    // ...
    for _, p := range produced {
        // NEW: Route through CCC for validation and queueing
        if m.crosschainConductor != nil {
            err = m.crosschainConductor.SubmitOutbound(ctx, p.Destination, envelope, seq)
        } else {
            // Fallback to direct dispatcher
            dispatcher := m.NewDispatcher()
            err = dispatcher.Submit(ctx, p.Destination, envelope)
        }
    }
}
```

### CCC Outbound Handler:
```go
// internal/core/execute/v2/crosschain/conductor.go
func (cc *CrossChainConductor) SubmitOutbound(
    ctx context.Context, 
    dest *url.URL, 
    envelope *messaging.Envelope,
    sequenceNum uint64,
) error {
    // Validate sequence number
    destKey := cc.createDestinationKey(MessageTypeSynthetic, dest)
    queue := cc.getOrCreateDestinationQueue(destKey)
    
    queue.mu.Lock()
    defer queue.mu.Unlock()
    
    // Check if we should send this now
    lastSent := queue.LastSentSequence
    if sequenceNum != lastSent + 1 {
        // Out of order - queue it
        queue.QueuedOutbound = append(queue.QueuedOutbound, &OutboundMessage{
            Envelope: envelope,
            Sequence: sequenceNum,
            Destination: dest,
        })
        cc.logger.Info("Queued out-of-sequence message", 
            "seq", sequenceNum, "expected", lastSent + 1)
        return nil
    }
    
    // In sequence - send it
    if err := cc.dispatcher.Submit(ctx, dest, envelope); err != nil {
        return err
    }
    
    queue.LastSentSequence = sequenceNum
    
    // Check if we can now send queued messages
    cc.processQueuedOutbound(queue)
    
    return nil
}
```

## 2. Inbound Interception (Receiving Side)

### Current Flow (BAD):
```go
// internal/api/v3/tm/submitter.go:49
func (s *Submitter) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
    // PROBLEM: Goes straight to CometBFT without sequence validation
    data, _ := envelope.MarshalBinary()
    res, err := s.local.CheckTx(ctx, data)
    // ...
}
```

### Proposed Change:
```go
// internal/api/v3/tm/submitter.go
func (s *Submitter) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
    // NEW: Pre-validate cross-partition messages
    if s.ccc != nil && isCrossPartitionEnvelope(envelope) {
        validity, err := s.ccc.ValidateInbound(ctx, envelope)
        if err != nil {
            return nil, errors.BadRequest.Wrap(err)
        }
        
        switch validity {
        case ValidationRejected:
            // Sequence too old or invalid
            return nil, errors.Delivered.With("message already processed")
            
        case ValidationQueued:
            // Out of sequence but valid - queue it
            return []*api.Submission{{
                Status: &protocol.TransactionStatus{
                    Code: protocol.ErrorCodePending,
                    Pending: true,
                },
            }}, nil
            
        case ValidationAccepted:
            // Continue with normal submission
        }
    }
    
    // Original submission logic...
    data, _ := envelope.MarshalBinary()
    res, err := s.local.CheckTx(ctx, data)
}
```

### CCC Inbound Validator:
```go
// internal/core/execute/v2/crosschain/conductor.go
type ValidationResult int
const (
    ValidationAccepted ValidationResult = iota
    ValidationQueued
    ValidationRejected
)

func (cc *CrossChainConductor) ValidateInbound(
    ctx context.Context, 
    envelope *messaging.Envelope,
) (ValidationResult, error) {
    messages, _ := envelope.Normalize()
    
    for _, msg := range messages {
        switch msg := msg.(type) {
        case *messaging.SyntheticMessage:
            seq, ok := msg.Message.(*messaging.SequencedMessage)
            if !ok {
                continue
            }
            
            // Get current state from database
            batch := cc.db.Begin(false)
            defer batch.Discard()
            
            var ledger *protocol.SyntheticLedger
            account := batch.Account(cc.describe.Synthetic())
            account.Main().GetAs(&ledger)
            
            partLedger := ledger.Partition(seq.Source)
            
            // Check sequence number
            if seq.Number <= partLedger.Delivered {
                cc.logger.Info("Rejecting old sequence",
                    "seq", seq.Number, 
                    "delivered", partLedger.Delivered)
                return ValidationRejected, nil
            }
            
            if seq.Number > partLedger.Delivered + 1 {
                // Out of sequence - queue it internally
                cc.queueInbound(seq)
                cc.logger.Info("Queueing out-of-sequence message",
                    "seq", seq.Number,
                    "expected", partLedger.Delivered + 1)
                return ValidationQueued, nil
            }
            
            // Sequence is correct
            return ValidationAccepted, nil
            
        case *messaging.BlockAnchor:
            // Similar validation for anchors
            return cc.validateAnchor(msg)
        }
    }
    
    return ValidationAccepted, nil
}
```

## 3. Integration Points

### A. Executor Initialization
```go
// internal/core/execute/v2/block/executor.go
func NewExecutor(opts ExecutorOptions) (*Executor, error) {
    x := &Executor{
        // ...
    }
    
    // NEW: Create CCC with dispatcher reference
    if opts.Router != nil {
        dispatcher := NewDispatcher(opts.Network, opts.Router, opts.Dialer)
        x.crosschainConductor = crosschain.NewCrossChainConductor(
            dispatcher, 
            x.logger,
            x.Describe,
            x.Database,
        )
    }
}
```

### B. API Service Initialization
```go
// internal/api/v3/tm/node.go or similar
func NewNode(config Config) (*Node, error) {
    // ...
    
    // NEW: Pass CCC to submitter
    submitter := NewSubmitter(local, config.CCC)
    
    // ...
}
```

## 4. State Management

### New Database Schema
```go
// internal/database/crosschain_state.go
type CrossChainState struct {
    // Outbound tracking
    OutboundSequences map[string]uint64  // destination -> last sent seq
    OutboundQueues    map[string][]QueuedMessage
    
    // Inbound tracking  
    InboundExpected   map[string]uint64  // source -> next expected seq
    InboundQueued     map[string][]QueuedMessage
    
    // Health metrics
    LastSuccessful    map[string]time.Time
    FailureCounts     map[string]int
}
```

## 5. Testing Strategy

### Unit Tests
```go
// internal/core/execute/v2/crosschain/conductor_test.go

func TestCCC_RejectsOldSequence(t *testing.T) {
    // Setup CCC with delivered height 100
    // Try to submit sequence 99
    // Verify rejection
}

func TestCCC_QueuesOutOfOrderSequence(t *testing.T) {
    // Setup CCC expecting sequence 101
    // Submit sequence 103
    // Verify it's queued
    // Submit sequence 101, 102
    // Verify all three are processed in order
}

func TestCCC_PreventsOutboundGaps(t *testing.T) {
    // Try to send sequence 105 when last sent was 100
    // Verify it's queued
    // Send 101-104
    // Verify 105 is automatically sent
}
```

## 6. Rollout Phases

### Phase 2.1: Outbound Logging Only
- CCC logs what it would reject but still sends
- Collect metrics for 1-2 weeks

### Phase 2.2: Outbound Enforcement
- CCC prevents out-of-sequence outbound messages
- Monitor for any issues

### Phase 2.3: Inbound Logging
- CCC logs what it would reject on inbound
- Continue collecting metrics

### Phase 2.4: Full Enforcement
- Both inbound and outbound fully enforced
- Complete protection enabled

## 7. Monitoring & Alerts

### Key Metrics to Track:
- Messages rejected (by type and reason)
- Queue depths (inbound and outbound)
- Sequence gaps detected
- Recovery operations triggered
- Network overhead saved (bytes not sent)

### Alerts:
- Queue depth > threshold
- Sequence gap > threshold  
- Destination unreachable > timeout
- Recovery failure