# Improved Gap Recovery Design

## Current Problem

The current `SimpleSequenceTracker` implementation has a critical flaw:
- When message #5 arrives but we're expecting #3, we DROP message #5
- We request recovery for messages #3-4
- Message #5 needs to be re-sent after #3-4 are recovered
- This causes unnecessary message drops and retransmissions

## Better Approach: Buffer Out-of-Order Messages

### Option 1: Accept and Buffer (Recommended)
```go
// When gap detected (e.g., received #5, expecting #3):
1. ACCEPT message #5 and buffer it
2. Set LastSyntheticDelivered = 5 (advance the index)
3. Mark sequences 3-4 as "missing" in a gap tracker
4. Request recovery for the missing messages
5. When #3 and #4 arrive via recovery, mark gap as filled
```

**Advantages:**
- No message drops
- No retransmissions needed
- System continues processing future messages
- Recovery happens in background

**Implementation:**
```go
func (st *SimpleSequenceTracker) ValidateAndTrackSynthetic(msg *messaging.SequencedMessage) (valid bool, reason string, requestRecovery bool) {
    // ...
    
    // Gap detected - but ACCEPT the message
    if msg.Number > expectedNext {
        gapStart := expectedNext
        gapEnd := msg.Number - 1
        
        // Track the gap
        gap := &SimpleSequenceGap{
            Start:      gapStart,
            End:        gapEnd,
            DetectedAt: time.Now(),
        }
        state.SyntheticGaps[gapStart] = gap
        
        // ADVANCE the sequence number (key difference!)
        state.LastSyntheticDelivered = msg.Number
        
        // Request recovery in background
        go st.SendRecoveryRequest(source, "synthetic", gapStart, gapEnd)
        
        // ACCEPT the message
        return true, "accepted with gap", true
    }
}
```

### Option 2: Sliding Window Buffer
```go
type SequenceBuffer struct {
    BaseSequence uint64              // Lowest undelivered sequence
    Window       map[uint64]Message  // Buffered messages
    MaxWindow    int                 // Max buffer size (e.g., 1000)
}

// Accept any message within window
func (sb *SequenceBuffer) Accept(seq uint64, msg Message) bool {
    if seq < sb.BaseSequence {
        return false // Already delivered
    }
    
    if seq > sb.BaseSequence + sb.MaxWindow {
        return false // Too far ahead
    }
    
    sb.Window[seq] = msg
    
    // Advance base if we can deliver consecutive messages
    for {
        if msg, ok := sb.Window[sb.BaseSequence]; ok {
            deliver(msg)
            delete(sb.Window, sb.BaseSequence)
            sb.BaseSequence++
        } else {
            break
        }
    }
    
    return true
}
```

### Option 3: Hybrid Approach
- Accept messages up to N sequences ahead
- Drop messages that are too far ahead (potential attack)
- Buffer reasonable out-of-order messages
- Deliver buffered messages once gaps are filled

## Why Current Approach is Problematic

1. **Cascading Drops**: If recovery is slow, many valid messages get dropped
2. **Wasted Bandwidth**: Dropped messages must be retransmitted
3. **Recovery Storms**: Multiple partitions requesting same dropped messages
4. **Latency**: Can't process future messages until gap is filled

## Recommended Solution

Modify `SimpleSequenceTracker` to:

1. **Accept out-of-order messages** (within reasonable bounds)
2. **Advance LastSyntheticDelivered** to the highest accepted sequence
3. **Track gaps** separately from delivered sequences
4. **Request recovery** for gaps without blocking future messages
5. **Mark gaps as filled** when recovery completes

This matches how TCP handles out-of-order packets - accept and buffer them rather than dropping them.

## Implementation Priority

This should be HIGH PRIORITY because:
- Current implementation will cause performance issues under load
- Network jitter will trigger unnecessary recovery requests
- Collection proofs make out-of-order delivery more likely
- Devnet testing will likely expose these issues