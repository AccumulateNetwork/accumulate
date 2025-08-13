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

### Receiving Side - API Routes to CCC
The destination partition's API routes anchor/synth transactions through CCC:

```
API receives transaction
    ↓
Check transaction type
    ↓
If Anchor or Synthetic:
    ↓
    **Route to CCC.ValidateInbound()**
        - Verify sequence number is next expected
        - Check if message is duplicate  
        - Validate proof/anchor signatures
        - Validate merkle proofs
    ↓
    CCC submits to CometBFT [only if valid]
Else:
    ↓
    Submit directly to CometBFT
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

### 2. API-Level Routing to CCC
Route anchor and synthetic transactions through CCC at the API layer:

```go
// In internal/api/v3/tm/submitter.go
func (s *Submitter) Submit(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
    // Check if this is an anchor or synthetic transaction
    if isAnchorOrSynthetic(envelope) {
        // Route to CCC for validation and submission
        return s.ccc.ProcessInbound(ctx, envelope, opts)
    }
    
    // Regular transactions go directly to CometBFT
    return s.submitToCometBFT(ctx, envelope, opts)
}

// CCC handles validation and submission
func (ccc *CrossChainConductor) ProcessInbound(ctx context.Context, envelope *messaging.Envelope, opts api.SubmitOptions) ([]*api.Submission, error) {
    // Validate the anchor/synthetic transaction
    if err := ccc.ValidateInbound(ctx, envelope); err != nil {
        return nil, err // Reject invalid transactions
    }
    
    // If valid, CCC submits to CometBFT
    return ccc.submitToCometBFT(ctx, envelope, opts)
}
```

### 3. CCC State Management
The CCC needs to maintain:

#### On Sending Side:
- **Per-destination sequence tracking**: What sequence number should we send next?
- **Destination readiness**: Is the destination ready for our next message?
- **NO QUEUES**: Messages are sent immediately or rejected

#### On Receiving Side:
- **Per-source sequence tracking**: What sequence number do we expect next?
- **NO QUEUES**: Out-of-sequence messages are rejected
- **Query mechanism**: When gaps detected, query for missing transaction sets using list proofs
- **Proof validation cache**: Recently validated proofs to avoid re-validation

### 4. Sequence Number Validation (No Queueing)

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
            
            // Reject if out of sequence - NO QUEUEING
            if seq.Number != expected {
                // Trigger query for missing transactions
                cc.queryMissingTransactions(seq.Source, expected, seq.Number)
                
                return errors.BadRequest.WithFormat(
                    "sequence %d out of order (expected: %d), querying for missing", 
                    seq.Number, expected)
            }
        }
    }
    
    return nil // Message is valid and in sequence
}

// Query for missing transaction sets using list proofs
func (cc *CrossChainConductor) queryMissingTransactions(source string, expectedSeq, receivedSeq uint64) {
    // Query the source partition for the complete set of missing transactions
    // Use list proofs to efficiently validate the entire set
    // This avoids the overhead of mixing missing txs with ones we already have
    
    // IMPORTANT: Batch to avoid transaction size limits
    gap := receivedSeq - expectedSeq
    if gap > MAX_BATCH_SIZE {
        // Split into multiple queries to respect transaction size limits
        for batch := expectedSeq; batch < receivedSeq; batch += MAX_BATCH_SIZE {
            end := batch + MAX_BATCH_SIZE - 1
            if end >= receivedSeq {
                end = receivedSeq - 1
            }
            go cc.fetchMissingBatch(source, batch, end)
        }
    } else {
        go cc.fetchMissingSet(source, expectedSeq, receivedSeq-1)
    }
}
```

## Collection Proof Formatting

### Structure
Collection proofs are highly efficient - ONE merkle proof for many transactions:

```go
type CollectionProof struct {
    // Single merkle state proof for the source partition
    StateRoot      []byte    // Current state root of source partition
    ProofPath      [][]byte  // Single merkle path to the transaction list
    
    // List of transaction hashes at this state
    TxHashes       [][]byte  // Just the hashes (32 bytes each)
    
    // Actual transactions
    Transactions   []Transaction  // The full transaction data
    
    // Sequence range
    StartSequence  uint64
    EndSequence    uint64
}
```

### Size Efficiency
- **Single Proof**: ONE merkle proof validates entire transaction set
- **Hash List**: Small - just 32 bytes per transaction hash
- **Massive Savings**: Collection proof for 1000 txs is barely larger than individual proof for 1 tx
- **Example**: 
  - Individual proofs: 1000 txs × ~1KB proof = ~1MB just for proofs
  - Collection proof: 1 proof (~1KB) + 1000 hashes (32KB) + tx data = huge savings

### Batching Strategy Options

#### Option 1: Historical State Proofs (Simpler for CometBFT)
```go
// Use earlier state snapshots to limit batch size
const MAX_BATCH_SIZE = 1000

func CreateHistoricalBatch(startSeq, endSeq uint64) CollectionProof {
    // Find a historical state that includes exactly MAX_BATCH_SIZE txs
    // Create proof from that earlier state
    // Self-contained proof that CometBFT can validate directly
}
```
**Pros:**
- Self-contained proofs, simple for CometBFT validation
- Each proof is independent and complete
- No complex multi-message coordination

**Cons:**
- Need to maintain historical states
- May not align perfectly with batch boundaries

#### Option 2: Complete Proof + Transaction Batches (More Efficient)
```go
// Send complete proof once, then transaction batches
type CompleteProof struct {
    StateRoot    []byte
    ProofPath    [][]byte  
    CompleteHashList [][]byte  // ALL transaction hashes
}

type TransactionBatch struct {
    ProofID      []byte  // References the complete proof
    StartIndex   int     // Index in hash list
    Transactions []Transaction
}
```
**Pros:**
- Most efficient - proof sent once
- Can send unlimited transactions in batches
- Minimal overhead

**Cons:**
- CometBFT must track proof-to-batch relationships
- More complex validation logic
- State management across multiple messages

### Selected Design: Option 1 with Async Healing
**Using Historical State Proofs with separate envelopes:**
1. Each healing transaction gets its own envelope
2. CCC runs asynchronously from normal transaction flow
3. CometBFT validation remains simple - each envelope is self-contained
4. No interference with regular transaction processing

```go
// Each healing batch is a separate envelope
type HealingEnvelope struct {
    Type          string  // "healing"
    CollectionProof CollectionProof
    // Self-contained proof with ~1000 transactions
}

// CCC processes healing asynchronously
func (ccc *CrossChainConductor) ProcessHealing() {
    // Runs independently of normal transaction flow
    // Submits healing envelopes at its own pace
    // No blocking of regular transactions
}
```

### Proof Validation
```go
func ValidateCollectionProof(proof *CollectionProof) error {
    // For Option 1: Simple, self-contained validation
    // 1. Verify the merkle proof against historical state
    // 2. Verify transaction hashes match the list
    // 3. Process all transactions in the batch
    
    // For Option 2: Would require stateful validation
    // tracking complete proofs and matching batches
}
```

## Critical Implementation Requirement: Chain Access

### Source CCC Needs
The source CCC must have efficient access to chains of transactions organized by destination and type:
```go
// Source partition needs indexed access to:
// - Synthetic transactions destined for each partition
// - Anchor transactions destined for each partition
type SourceIndex struct {
    // Key: destination/type (e.g., "BVN1/synthetic", "DN/anchor")
    // Value: Chain of transactions in sequence order
    DestinationChains map[string]*TransactionChain
}

type TransactionChain struct {
    // Efficient access to any historical state
    GetTransactionsFrom(startSeq, endSeq uint64) []Transaction
    GetStateAt(sequence uint64) StateProof
}
```

### Destination CCC Needs
The destination CCC must track what it has received from each source:
```go
// Destination partition needs indexed access to:
// - What sequence number expected from each source/type
type DestinationIndex struct {
    // Key: source/type (e.g., "BVN0/synthetic", "BVN2/anchor")
    // Value: Sequence tracking
    SourceTracking map[string]*SequenceTracker
}

type SequenceTracker struct {
    LastDelivered   uint64
    ExpectedNext    uint64
    // Quick lookup of what we've already processed
}
```

### Database/Indexing Requirements
This design heavily relies on:
1. **Efficient chain indexing** by destination/type pairs
2. **Historical state access** for creating collection proofs
3. **Fast sequence number lookups** for validation
4. **Partition-aware transaction routing** tables

Without these indexes, the CCC cannot efficiently:
- Create collection proofs for healing
- Validate incoming sequence numbers
- Query for missing transaction sets

## Benefits of This Design

1. **Network Efficiency**: Invalid messages never leave the source partition (O(n²) → O(1) overhead)
2. **Consensus Efficiency**: Invalid messages never enter CometBFT mempool
3. **No Queue Complexity**: No message queues to manage - use list proofs to query missing sets
4. **Early Rejection**: Problems detected and handled at the earliest possible point
5. **Better DoS Protection**: Can rate-limit and validate before expensive consensus operations
6. **Cleaner Architecture**: Clear separation between validation and execution
7. **Efficient Recovery**: List proofs allow fetching complete missing sets without mixing with existing transactions

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