# Conductor: Anchor and Synthetic Transaction Management System

## Overview

This document outlines the design for the **Conductor** - a comprehensive transaction management system that runs on all validator nodes to orchestrate anchor and synthetic transaction sequence validation, gap detection, and automatic healing.

## Core Requirements

1. **API-Level Sequence Validation**: Check sequence numbers at transaction submission
2. **Gap Detection and Management**: Hold out-of-order transactions until gaps are filled
3. **Automatic Healing**: If a gap persists for a configurable timeout (default 10 blocks), the Conductor requests missing transactions
4. **Ordered Submission**: Ensure transactions are submitted in correct sequence order
5. **Duplicate Prevention**: Discard already processed transactions
6. **Timeout-Based Recovery**: Request missing transactions after 10 blocks

## Architecture Components

### 1. Request Router

**Location**: API layer and block execution layer
**Responsibility**: Route anchor and synthetic transaction requests/sends to the Conductor for sequence validation and management

```go
type RequestRouter struct {
    conductor *Conductor
    normalProcessor *MessageProcessor
}

// Route incoming API requests
func (rr *RequestRouter) RouteRequest(req *api.Request) {
    switch req.Type {
    case api.AnchorRequest, api.SyntheticTransactionRequest:
        // Route to Conductor for sequence validation
        rr.conductor.ProcessRequest(req)
    default:
        // Route to normal processing
        rr.normalProcessor.ProcessRequest(req)
    }
}

// Route outgoing anchor/synthetic transaction sends
func (rr *RequestRouter) RouteSend(txType TransactionType, envelope *messaging.Envelope) error {
    switch txType {
    case AnchorTransaction, SyntheticTransaction:
        // Route to Conductor for sequence management
        return rr.conductor.SendTransaction(envelope)
    default:
        // Route to normal dispatcher
        return rr.normalProcessor.SendTransaction(envelope)
    }
}
```

### 2. Sequence Validator (API Interceptor)

**Location**: Message processing pipeline before consensus
**Responsibility**: Validate sequence numbers for incoming anchors and synthetic transactions

```go
type SequenceValidator struct {
    partitionStates map[string]*PartitionSequenceState
    conductor *Conductor
}

func (sv *SequenceValidator) ValidateSequence(msg *messaging.Message) ValidationResult {
    // Extract sequence info
    sourcePartition := msg.GetSourcePartition()
    destPartition := msg.GetDestinationPartition()
    sequenceNum := msg.GetSequenceNumber()
    
    state := sv.partitionStates[sourcePartition]
    
    switch {
    case sequenceNum == state.expectedNext:
        return ACCEPT_AND_FORWARD
    case sequenceNum > state.expectedNext:
        return HOLD_FOR_GAP_FILL
    case sequenceNum <= state.lastDelivered:
        return DISCARD_DUPLICATE
    }
}
```

### 3. Gap Tracker

**Responsibility**: Track sequence gaps and manage timeout-based healing triggers

```go
type PartitionSequenceState struct {
    partitionID     string
    lastDelivered   uint64
    expectedNext    uint64
    
    // Gap management
    gaps            map[uint64]*GapInfo
    heldTransactions map[uint64]*messaging.Message
    
    // Block tracking for timeouts
    currentBlock    uint64
}

type GapInfo struct {
    sequenceNumber  uint64
    detectedAtBlock uint64
    healingRequested bool
}
```

### 4. Conductor

**Responsibility**: Coordinate healing requests and manage the healing process

```go
type Conductor struct {
    client      message.AddressedClient
    healer      *healing.Healer
    networkInfo *healing.NetworkInfo
    
    // Active healing requests
    activeRequests map[string]*HealingRequest
}

type HealingRequest struct {
    sourcePartition string
    destPartition   string
    sequenceNumber  uint64
    requestedAtBlock uint64
    status          HealingStatus
}
```

### 5. Transaction Queue Manager

**Responsibility**: Hold and release transactions in correct order for both incoming and outgoing

```go
type TransactionQueue struct {
    // Incoming transaction queues (from API)
    incomingQueues map[string]*PartitionQueue
    
    // Outgoing transaction queues (to network)
    outgoingQueues map[string]*OutgoingQueue
}

type PartitionQueue struct {
    heldTransactions map[uint64]*messaging.Message
    readyToSubmit    []uint64  // Sorted sequence numbers ready for submission
}

type OutgoingQueue struct {
    pendingSends     map[uint64]*messaging.Envelope
    nextSequence     uint64
    readyToSend      []uint64  // Sorted sequence numbers ready to send
}
```

### 6. Outgoing Transaction Manager

**Responsibility**: Manage sequence numbers for outgoing anchors and synthetic transactions

```go
type OutgoingTransactionManager struct {
    partitionSequences map[string]*OutgoingSequenceState
    dispatcher         Dispatcher
    conductor *Conductor
}

type OutgoingSequenceState struct {
    partitionID      string
    nextSequence     uint64
    pendingTxs       map[uint64]*messaging.Envelope
    lastSent         uint64
    lastConfirmed    uint64
}

func (otm *OutgoingTransactionManager) SendTransaction(envelope *messaging.Envelope) error {
    // Assign sequence number
    destPartition := envelope.GetDestination().String()
    state := otm.partitionSequences[destPartition]
    
    sequenceNum := state.nextSequence
    state.nextSequence++
    
    // Set sequence in envelope
    envelope.SetSequenceNumber(sequenceNum)
    
    // Send via dispatcher with sequence tracking
    err := otm.dispatcher.Submit(context.Background(), envelope.GetDestination(), envelope)
    if err != nil {
        return err
    }
    
    // Track pending transaction
    state.pendingTxs[sequenceNum] = envelope
    state.lastSent = sequenceNum
    
    return nil
}
```

## Process Flow

### Phase 1: Request Routing and API-Level Interception

```
API Request
        ↓
Request Router
        ↓
┌─────────────────────────────────────┐
│ Check request type:                 │
│ - Anchor Request                    │
│ - Synthetic Transaction Request     │
│ - Other Request Types               │
└─────────────────────────────────────┘
        ↓
┌─── Anchor/Synthetic ──────→ Conductor
└─── Other Requests ────────→ Normal Processing

Conductor (Incoming):
Incoming Anchor/Synthetic Transaction
        ↓
Sequence Validator
        ↓
┌─────────────────────────────────────┐
│ Extract: source, dest, sequence     │
│ Check against expected sequence     │
└─────────────────────────────────────┘
        ↓
┌─── Expected Sequence N ────→ Forward to Consensus
├─── Sequence N+X (gap) ────→ Hold + Trigger Gap Detection
└─── Sequence ≤ N ──────────→ Discard (duplicate)
```

### Phase 1b: Outgoing Transaction Routing

```
Block Execution (sendSyntheticTransactionsForBlock/sendBlockAnchor)
        ↓
Request Router (RouteSend)
        ↓
┌─────────────────────────────────────┐
│ Check transaction type:             │
│ - Anchor Transaction                │
│ - Synthetic Transaction             │
│ - Other Transaction Types           │
└─────────────────────────────────────┘
        ↓
┌─── Anchor/Synthetic ──────→ Conductor (Outgoing)
└─── Other Transactions ───→ Normal Dispatcher

Conductor (Outgoing):
Outgoing Anchor/Synthetic Transaction
        ↓
Outgoing Transaction Manager
        ↓
┌─────────────────────────────────────┐
│ Assign sequence number              │
│ Track in outgoing queue             │
│ Send via dispatcher                 │
└─────────────────────────────────────┘
        ↓
Dispatcher → Network
```

### Phase 2: Gap Management

```
Gap Detected
        ↓
┌─────────────────────────────────────┐
│ 1. Record gap in PartitionState     │
│ 2. Hold transaction in queue        │
│ 3. Start gap timer (block counter)  │
│ 4. Update expected sequence         │
└─────────────────────────────────────┘
        ↓
┌─── Gap fills within 10 blocks ──→ Release held transactions in order
└─── Gap persists 10 blocks ──────→ Trigger healing request
```

### Phase 3: Healing Process

```
Gap Timeout (10 blocks)
        ↓
┌─────────────────────────────────────┐
│ 1. Identify source partition        │
│ 2. Create healing request           │
│ 3. Query source for missing tx      │
│ 4. Reconstruct transaction + proof  │
│ 5. Submit to local consensus        │
└─────────────────────────────────────┘
        ↓
┌─── Healing successful ──→ Fill gap + release queued transactions
└─── Healing failed ────→ Retry with exponential backoff
```

## Implementation Details

### Sequence Number Tracking

**Per-Partition State**:
```go
type PartitionSequenceState struct {
    // Current state
    lastDelivered   uint64    // Last successfully processed sequence
    expectedNext    uint64    // Next expected sequence number
    currentBlock    uint64    // Current block height for timeout tracking
    
    // Gap tracking
    gaps            map[uint64]*GapInfo
    gapTimeouts     map[uint64]uint64  // sequence -> block when gap times out
    
    // Transaction holding
    heldTransactions map[uint64]*messaging.Message
    
    // Metrics
    totalGaps       uint64
    healedGaps      uint64
    failedHealing   uint64
}
```

### Gap Detection Logic

```go
func (state *PartitionSequenceState) ProcessTransaction(tx *messaging.Message, currentBlock uint64) ProcessResult {
    seqNum := tx.GetSequenceNumber()
    
    // Update current block
    state.currentBlock = currentBlock
    
    switch {
    case seqNum == state.expectedNext:
        // In order - process immediately
        state.lastDelivered = seqNum
        state.expectedNext = seqNum + 1
        
        // Check if this fills any gaps and releases held transactions
        return state.releaseHeldTransactions()
        
    case seqNum > state.expectedNext:
        // Gap detected - hold transaction
        for i := state.expectedNext; i < seqNum; i++ {
            state.gaps[i] = &GapInfo{
                sequenceNumber:  i,
                detectedAtBlock: currentBlock,
                healingRequested: false,
            }
        }
        
        state.heldTransactions[seqNum] = tx
        return TRANSACTION_HELD
        
    case seqNum <= state.lastDelivered:
        // Duplicate - discard
        return TRANSACTION_DISCARDED
    }
}
```

### Healing Trigger Logic

```go
func (coordinator *Conductor) CheckGapTimeouts(currentBlock uint64) {
    for partitionID, state := range coordinator.partitionStates {
        for seqNum, gap := range state.gaps {
            // Check if gap has persisted for 10 blocks
            if currentBlock-gap.detectedAtBlock >= 10 && !gap.healingRequested {
                coordinator.requestHealing(partitionID, seqNum, currentBlock)
                gap.healingRequested = true
            }
        }
    }
}

func (coordinator *Conductor) requestHealing(sourcePartition string, seqNum uint64, currentBlock uint64) {
    request := &HealingRequest{
        sourcePartition:  sourcePartition,
        destPartition:    coordinator.localPartition,
        sequenceNumber:   seqNum,
        requestedAtBlock: currentBlock,
        status:          HEALING_REQUESTED,
    }
    
    // Use existing healing package
    go coordinator.performHealing(request)
}
```

### Transaction Release Logic

```go
func (state *PartitionSequenceState) releaseHeldTransactions() []ProcessResult {
    var results []ProcessResult
    
    // Check for consecutive transactions starting from expectedNext
    for {
        tx, exists := state.heldTransactions[state.expectedNext]
        if !exists {
            break
        }
        
        // Process the held transaction
        delete(state.heldTransactions, state.expectedNext)
        delete(state.gaps, state.expectedNext)
        
        state.lastDelivered = state.expectedNext
        state.expectedNext++
        
        results = append(results, ProcessResult{
            Transaction: tx,
            Action:      SUBMIT_TO_CONSENSUS,
        })
    }
    
    return results
}
```

## Integration Points

### 1. Request Routing Integration

**Location**: API layer request handling
**Integration**: Route anchor and synthetic transaction requests to Conductor

```go
// In API request handler
func (api *APIHandler) HandleRequest(req *api.Request) (*api.Response, error) {
    // Route requests based on type
    switch req.Type {
    case api.AnchorRequest, api.SyntheticTransactionRequest:
        // Route to Conductor for sequence validation
        return api.conductor.HandleRequest(req)
    default:
        // Route to normal processing
        return api.normalProcessor.HandleRequest(req)
    }
}

// In Conductor
func (c *Conductor) HandleRequest(req *api.Request) (*api.Response, error) {
    // Extract transaction from request
    tx, err := c.extractTransaction(req)
    if err != nil {
        return nil, err
    }
    
    // Validate sequence
    result := c.sequenceValidator.ValidateSequence(tx)
    
    switch result.Action {
    case ACCEPT_AND_FORWARD:
        // Forward to consensus processing
        return c.forwardToConsensus(tx)
        
    case HOLD_FOR_GAP_FILL:
        // Hold transaction and trigger gap detection
        c.gapTracker.RecordGap(tx)
        return &api.Response{Status: "held_for_gap"}, nil
        
    case DISCARD_DUPLICATE:
        // Log and discard
        c.logger.Debug("Discarding duplicate transaction", "seq", tx.GetSequenceNumber())
        return &api.Response{Status: "duplicate_discarded"}, nil
    }
}
```

### 2. API Message Processing

**Location**: After request routing, in Conductor message handlers
**Integration**: Process routed anchor and synthetic transactions

### 3. Block Processing Integration

**Location**: Block finalization process
**Integration**: Route outgoing transactions and check gap timeouts

```go
// In block finalization - route outgoing transactions
func (executor *Executor) sendSyntheticTransactionsForBlock(batch *database.Batch, isLeader bool, blockIndex uint64, anchors []*protocol.TransactionStatus) error {
    // Existing synthetic transaction creation logic...
    
    for _, envelope := range syntheticEnvelopes {
        // Route through healing process instead of direct dispatcher
        err := executor.requestRouter.RouteSend(SyntheticTransaction, envelope)
        if err != nil {
            return err
        }
    }
    
    return nil
}

func (executor *Executor) sendBlockAnchor(batch *database.Batch, anchor *protocol.BlockValidatorAnchor, sequenceNumber uint64, destination *url.URL) error {
    // Existing anchor creation logic...
    
    // Route through healing process instead of direct dispatcher
    err := executor.requestRouter.RouteSend(AnchorTransaction, envelope)
    if err != nil {
        return err
    }
    
    return nil
}

// In block finalization - check gap timeouts
func (executor *Executor) finalizeBlock(block *Block) error {
    // Existing finalization logic...
    
    // Check for gap timeouts and trigger healing
    executor.healingCoordinator.CheckGapTimeouts(block.Index)
    
    // Update sequence states with current block
    executor.sequenceValidator.UpdateBlockHeight(block.Index)
    
    // Check outgoing transaction confirmations
    executor.outgoingTransactionManager.CheckConfirmations(block.Index)
    
    return nil
}
```

### 4. Background Healing Service

**Location**: Background task launcher
**Integration**: Periodic healing checks

```go
// In executor initialization
func (executor *Executor) initializeHealing() {
    if !executor.EnableHealing {
        return
    }
    
    executor.BackgroundTaskLauncher(func() {
        ticker := time.NewTicker(30 * time.Second)
        defer ticker.Stop()
        
        for range ticker.C {
            executor.healingCoordinator.ProcessHealingRequests()
        }
    })
}
```

## Configuration

```go
type HealingConfig struct {
    Enabled              bool          `json:"enabled"`
    GapTimeoutBlocks     int           `json:"gapTimeoutBlocks"`     // Default: 10
    HealingRetryInterval time.Duration `json:"healingRetryInterval"` // Default: 30s
    MaxConcurrentHealing int           `json:"maxConcurrentHealing"` // Default: 5
    MaxHeldTransactions  int           `json:"maxHeldTransactions"`  // Default: 1000
}
```

## Monitoring and Metrics

### Key Metrics
- **Gaps detected per partition**
- **Average gap fill time**
- **Healing success/failure rates**
- **Held transactions count**
- **Timeout-triggered healing requests**

### Logging
- Gap detection events
- Healing request triggers
- Transaction release events
- Healing success/failure
- Performance metrics

## Benefits

1. **Automatic Recovery**: No manual intervention required for sequence gaps
2. **Ordered Processing**: Guarantees transactions are processed in correct order (both incoming and outgoing)
3. **Duplicate Prevention**: Prevents reprocessing of already delivered transactions
4. **Distributed Healing**: All nodes can detect and heal gaps independently
5. **Configurable Timeouts**: Adjustable gap timeout for different network conditions
6. **Existing Infrastructure**: Leverages current healing package and background tasks
7. **Comprehensive Coverage**: Manages both incoming API requests and outgoing block-generated transactions
8. **Sequence Integrity**: Ensures sequence number consistency across all anchor and synthetic transactions
9. **Send Confirmation**: Tracks outgoing transaction delivery and can retry failed sends

## Considerations

1. **Memory Usage**: Held transactions consume memory - need limits and cleanup
2. **Network Load**: Healing requests generate additional network traffic
3. **Coordination**: Multiple nodes may heal the same gap - need deduplication
4. **Performance**: Sequence validation adds processing overhead
5. **Edge Cases**: Handle network partitions, validator changes, and restart scenarios

## Refactor Complexity Analysis

### **DIFFICULTY: MODERATE** 

Based on analysis of the current codebase, this refactor is **more manageable than initially expected**.

### **✅ Advantages We Have**

#### **1. Clean Transaction Construction**
The code already has **well-structured transaction construction**:

```go
// Synthetic transactions are already built with all components
messages := []messaging.Message{
    &messaging.SyntheticMessage{
        Message:   seq,           // SequencedMessage with source/dest/number
        Proof:     receipt,       // AnnotatedReceipt with merkle proofs
        Signature: keySig,        // Key signature from validator
    },
}

// Anchors are built via shared utility
env, err := shared.PrepareBlockAnchor(nodeUrl, network, key, anchor, sequenceNumber, destUrl)
```

#### **2. Single Dispatcher Interface**
All transactions go through **one dispatcher interface**:
```go
// Current pattern - easy to intercept
err = x.mainDispatcher.Submit(context.Background(), destination, envelope)
```

#### **3. Clear Send Points**
Only **2 main functions** send anchor/synthetic transactions:
- `sendSyntheticTransactionsForBlock()` - Line 475 in block_begin.go
- `sendBlockAnchor()` - Line 489 in block_begin.go

#### **4. Access to All Internals**
We have **complete access** to:
- ✅ Transaction construction logic
- ✅ Sequence number assignment  
- ✅ Signature creation (`x.signTransaction()`)
- ✅ Merkle proof generation
- ✅ Dispatcher interface
- ✅ Database access for sequence tracking

### **🔶 Refactor Requirements**

#### **1. Minimal Code Changes**
**Replace 2 dispatcher calls:**
```go
// Current:
err = x.mainDispatcher.Submit(ctx, destination, envelope)

// New:
err = x.conductor.RouteSend(TransactionType, envelope)
```

#### **2. Add Conductor**
**Single new component** in executor:
```go
type Executor struct {
    mainDispatcher Dispatcher
    conductor      *Conductor  // NEW
    // ... existing fields
}
```

#### **3. Sequence State Tracking**
**Add per-partition sequence tracking:**
```go
// Already have partition info in:
// - seq.Destination (for synthetic txs)
// - destPartUrl (for anchors)

// Just need to track:
outgoingSequences map[string]uint64  // partition -> next sequence
```

### **📊 Effort Estimate**

| Phase | Effort | Risk | 
|-------|--------|------|
| **Minimal Router** | 1-2 days | Low |
| **Sequence Management** | 2-3 days | Low-Medium |
| **Gap Detection** | 3-5 days | Medium |
| **Full Integration** | 1-2 days | Low |
| **Testing & Debugging** | 2-3 days | Medium |
| **Total** | **9-15 days** | **Medium** |

### **🎯 Why This Is Manageable**

1. **Clean existing architecture** - dispatcher pattern makes interception easy
2. **Limited touch points** - only 2 functions need modification  
3. **Complete access** - we can construct, sign, and track all transactions
4. **Incremental approach** - can implement in phases without breaking changes
5. **Existing patterns** - healing, background tasks, sequence tracking already exist

The refactor is **significantly easier** than building from scratch because the transaction construction, signing, and dispatch infrastructure is already well-designed and accessible.

**The Conductor acts like a train conductor** - orchestrating the flow of anchor and synthetic transactions, ensuring they arrive in the right order, and coordinating with other partitions when things go missing.

## Implementation Priority

1. **Phase 1**: Minimal viable router (pass-through)
2. **Phase 2**: Add sequence management and tracking
3. **Phase 3**: Add incoming sequence validation and gap detection
4. **Phase 4**: Add timeout-based healing integration
5. **Phase 5**: Monitoring, metrics, and optimization
6. **Phase 6**: Advanced features (retry logic, performance tuning)
