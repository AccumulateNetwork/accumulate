# Gap Recovery Design - Simple Index-Based Approach

## Overview

The CrossChain Conductor (CCC) uses a simple, elegant approach to gap recovery that leverages collection proofs and index tracking. There is no complex buffering or gap management - just a single index per destination that tracks what has been successfully sent.

## Core Concept

Gap recovery is achieved by simply resetting the send index when a gap is detected. The normal batch sending mechanism handles everything else automatically.

## Architecture

### Source Partition (Sender)

Each source partition maintains per-destination state:
```go
type DestinationState struct {
    SentTxIndex    uint64  // Last successfully sent sequence number
    CurrentTxIndex uint64  // Current/latest sequence number available
    // ... other fields
}
```

#### Sending Process:
1. **Batch Creation**: When sending to a destination, create a batch from `SentTxIndex + 1` to `CurrentTxIndex`
2. **Collection Proof**: Create a single collection proof for all messages in the batch
3. **Send Attempt**: Send the batch with collection proof
4. **Success**: If successful, update `SentTxIndex = CurrentTxIndex`
5. **Failure**: If failed, leave `SentTxIndex` unchanged (next attempt will include same messages plus any new ones)

### Destination Partition (Receiver)

Each destination tracks what it has received:
```go
type SourceState struct {
    LastReceivedSeq uint64  // Highest consecutive sequence received
}
```

#### Gap Detection:
1. Receives message with sequence N
2. Expects sequence M (where M < N)
3. Detects gap of sequences [M, N-1]
4. Sends `GapRequest` with `LastKnownSequence = M-1`

### Gap Request Handling

When source receives a `GapRequest`:
```go
func handleGapRequest(request GapRequest) {
    state := getDestinationState(request.Source)
    
    // Simply reset the send index to what the destination has
    state.SentTxIndex = request.LastKnownSequence
    
    // Next regular send will include everything from LastKnownSequence+1
}
```

## Why This Works

### Self-Healing Property
- Failed sends automatically get retried with cumulative data
- No special recovery logic needed - the normal send path handles everything
- Network issues resolve themselves once connectivity returns

### Efficiency with Collection Proofs
- Even if resending 100 messages, it's just one collection proof
- Bandwidth overhead is minimal compared to individual message proofs
- Destination can quickly validate the entire batch

### Simplicity
- No complex buffer management
- No gap tracking data structures
- No special recovery sessions
- Just one index per destination

## Example Scenarios

### Scenario 1: Normal Operation
```
Source has messages 1-10 for destination
SentTxIndex = 0, CurrentTxIndex = 10

1. Send batch [1-10] with collection proof
2. Success! Update SentTxIndex = 10
3. New messages 11-15 arrive
4. Send batch [11-15] with collection proof  
5. Success! Update SentTxIndex = 15
```

### Scenario 2: Send Failure
```
Source has messages 1-10 for destination
SentTxIndex = 0, CurrentTxIndex = 10

1. Send batch [1-10] with collection proof
2. FAILURE! SentTxIndex remains 0
3. New messages 11-12 arrive, CurrentTxIndex = 12
4. Retry: Send batch [1-12] with collection proof
5. Success! Update SentTxIndex = 12
```

### Scenario 3: Gap Detection and Recovery
```
Destination has received messages 1-5
Source sends message 8 (maybe 6-7 were lost)

1. Destination detects gap, sends GapRequest{LastKnownSequence: 5}
2. Source receives request, sets SentTxIndex = 5
3. Source's next send includes [6-CurrentTxIndex] with collection proof
4. Destination receives all missing messages in one batch
```

### Scenario 4: Multiple Gaps
```
Destination has 1-3, receives 7

1. Destination sends GapRequest{LastKnownSequence: 3}
2. Source sets SentTxIndex = 3
3. Source sends [4-10] (assuming CurrentTxIndex = 10)
4. All gaps filled in one batch!
```

## Implementation Details

### Per-Destination Tracking
```go
type CrossChainConductor struct {
    destinationStates map[string]*DestinationSendState
    // ...
}

type DestinationSendState struct {
    Destination    *url.URL
    SentTxIndex    uint64      // Last successfully sent
    CurrentTxIndex uint64      // Latest available
    LastSendTime   time.Time
    SendInProgress bool
    mu             sync.RWMutex
}
```

### Batch Send Function
```go
func (cc *CrossChainConductor) sendBatchToDestination(dest *url.URL) error {
    state := cc.getDestinationState(dest)
    state.mu.Lock()
    defer state.mu.Unlock()
    
    // Nothing new to send?
    if state.SentTxIndex >= state.CurrentTxIndex {
        return nil
    }
    
    // Collect messages from SentTxIndex+1 to CurrentTxIndex
    messages := cc.collectMessages(dest, state.SentTxIndex+1, state.CurrentTxIndex)
    
    // Create collection proof for entire batch
    proof := cc.proofService.CreateCollectionProof(messages)
    
    // Send batch
    err := cc.dispatcher.Submit(dest, messages, proof)
    if err != nil {
        // Failed - SentTxIndex unchanged, will retry everything next time
        return err
    }
    
    // Success - advance SentTxIndex
    state.SentTxIndex = state.CurrentTxIndex
    return nil
}
```

### Gap Request Handler
```go
func (cc *CrossChainConductor) HandleGapRequest(req *messaging.GapRequest) {
    state := cc.getDestinationState(req.RequestingPartition)
    state.mu.Lock()
    defer state.mu.Unlock()
    
    // Reset send index to what destination has
    if req.LastKnownSequence < state.SentTxIndex {
        cc.logger.Info("Resetting send index for gap recovery",
            "destination", req.RequestingPartition,
            "was", state.SentTxIndex,
            "now", req.LastKnownSequence)
        
        state.SentTxIndex = req.LastKnownSequence
    }
    
    // Trigger immediate send to fill the gap
    go cc.sendBatchToDestination(req.RequestingPartition)
}
```

## Benefits

1. **Simplicity**: No complex state machines or buffer management
2. **Reliability**: Self-healing under network issues
3. **Efficiency**: Collection proofs minimize overhead
4. **Scalability**: O(1) memory per destination regardless of gap size
5. **Correctness**: No messages lost, no duplicates processed

## Testing Considerations

1. **Unit Tests**: Test index advancement and reset logic
2. **Failure Tests**: Verify SentTxIndex doesn't advance on failure
3. **Gap Tests**: Verify gap requests correctly reset index
4. **Integration Tests**: Full end-to-end gap recovery scenarios

## Monitoring

Key metrics to track:
- `sent_tx_index` per destination
- `current_tx_index` per destination  
- `gap_size` (CurrentTxIndex - SentTxIndex)
- `gap_requests_received`
- `gap_requests_sent`
- `collection_proof_size` (messages per batch)