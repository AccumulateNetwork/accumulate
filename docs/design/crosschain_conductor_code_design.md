# CrossChain Conductor - Code Design

**Feature**: CrossChain Conductor  
**Created**: 2025-08-30

## Existing Code Evaluation

### Current Structure (Problems Identified)
```
internal/core/execute/v2/crosschain/
├── conductor.go              # 1200+ lines, does queuing (wrong approach)
├── proof_service.go          # Collection proofs broken (nil merkle state)
├── recovery.go               # Complex recovery logic 
├── unified_transport.go      # Transport layer
├── sequence_tracker_simple.go # Basic sequence tracking
├── types.go                  # Message types
└── 40+ test files           # Excessive test files
```

### Critical Issues with Existing Code
1. **QUEUING IMPLEMENTED** - Current code has `destinationQueues`, `syntheticChan`, `retryChan` (violates NO QUEUING requirement)
2. **COLLECTION PROOFS BROKEN** - `proof_service.go:303` passes `nil` to `merkle.GetReceiptList()`
3. **NO INTERFACE COMPLIANCE** - Doesn't implement our 3 required interfaces
4. **OVERLY COMPLEX** - 40+ files, channels, goroutines (not needed for our design)
5. **WRONG PATTERNS** - Async processing, retry tracking (violates requirements)

## Redesigned Code Structure (Aligned with Test Design)

### New Structure
```go
// Main conductor implementing our interface design
type CrossChainConductor struct {
    chainProvider   ChainProvider    // Get chains and top-of-chain indices
    messageSender   MessageSender    // Send list proofs 
    messageReceiver MessageReceiver  // Receive messages and gap healing
    logger          logging.OptionalLogger
}

// Constructor for dependency injection (testable)
func NewCrossChainConductor(
    chainProvider ChainProvider,
    messageSender MessageSender, 
    messageReceiver MessageReceiver,
    logger logging.OptionalLogger,
) *CrossChainConductor
```

### Core Methods (Matching Test Design)
```go
// Main workflow method
func (cc *CrossChainConductor) ProcessOutboundMessages(ctx context.Context, chainType ChainType, destination string) error {
    // 1. Get chain from ChainProvider
    // 2. Get current top of chain index
    // 3. Collect transactions from last sent index to current top
    // 4. Generate list proof from collected transactions  
    // 5. Send list proof via MessageSender
    // 6. Update top of chain index on success
}

// Gap healing method
func (cc *CrossChainConductor) HandleInboundMessage(ctx context.Context, source string, message *messaging.Envelope) error {
    // 1. Receive message via MessageReceiver  
    // 2. Get last received sequence number
    // 3. If gap detected, request missing messages
    // 4. No queuing - immediate gap healing request
}
```

### Interface Implementations (Production)
```go
// Production ChainProvider implementation
type DatabaseChainProvider struct {
    db database.Beginner
    describe *config.Describe
}
func (dcp *DatabaseChainProvider) GetAnchorChain(ctx context.Context) (Chain, error)
func (dcp *DatabaseChainProvider) GetSyntheticChain(ctx context.Context) (Chain, error)
func (dcp *DatabaseChainProvider) GetTopOfChainIndex(ctx context.Context, chainType ChainType) (uint64, error)
func (dcp *DatabaseChainProvider) SetTopOfChainIndex(ctx context.Context, chainType ChainType, index uint64) error

// Production MessageSender implementation  
type DispatcherMessageSender struct {
    dispatcher execute.Dispatcher
}
func (dms *DispatcherMessageSender) SendListProof(ctx context.Context, destination string, proof *protocol.AnnotatedReceipt) error

// Production MessageReceiver implementation
type APIMessageReceiver struct {
    sequenceTracker *SequenceTracker
}
func (amr *APIMessageReceiver) ReceiveMessage(ctx context.Context, source string, message *messaging.Envelope) (uint64, error)
func (amr *APIMessageReceiver) RequestMissingMessages(ctx context.Context, source string, fromSequence uint64) error
```

## Test Structure Alignment

### Test File Organization (Simplified)
```
internal/core/execute/v2/crosschain/
├── conductor.go                    # Main implementation
├── conductor_test.go              # Core conductor tests
├── chain_provider.go              # ChainProvider implementation  
├── chain_provider_test.go         # ChainProvider tests
├── message_sender.go              # MessageSender implementation
├── message_sender_test.go         # MessageSender tests
├── message_receiver.go            # MessageReceiver implementation
├── message_receiver_test.go       # MessageReceiver tests
└── integration_test.go            # Integration workflow tests
```

### Mock Structure (Test-Only)
```go
// All mocks in *_test.go files only
type MockChainProvider struct { mock.Mock }
type MockMessageSender struct { mock.Mock }  
type MockMessageReceiver struct { mock.Mock }
type MockChain struct { mock.Mock }
type MockTransaction struct { mock.Mock }
```

## Critical Fixes Required

### 1. Collection Proof Fix
**File**: `internal/database/chain.go`
**Add**: 
```go
func (c *Chain) Inner() *MerkleManager {
    return c.merkle
}
```

**File**: `internal/core/execute/v2/crosschain/proof_service.go:303`
**Change**: `merkle.GetReceiptList(nil, ...)` → `merkle.GetReceiptList(req.SourceChain.Inner(), ...)`

### 2. Remove Queue-Based Code
**Remove**:
- All `chan` fields from `CrossChainConductor` 
- `destinationQueues` map
- `goroutine` processing loops
- `retry` mechanisms
- `async` processing

### 3. Simplify to Interface-Based Design
**Replace**: Complex internal structs with simple interface-based dependency injection
**Result**: Testable, mockable, focused implementation

## Implementation Strategy

### Phase 1: Interface Compliance
1. Create interface implementations (ChainProvider, MessageSender, MessageReceiver)
2. Refactor CrossChainConductor to use interfaces
3. Remove queuing and async processing

### Phase 2: Collection Proof Fix  
1. Add `Inner()` method to database.Chain
2. Fix `proof_service.go` nil pointer issue
3. Test collection proof generation

### Phase 3: Integration
1. Hook into executor for outbound processing
2. Hook into API for inbound processing  
3. Remove old conductor code

### Phase 4: Testing
1. Write tests matching test design
2. All mocks in `*_test.go` files only
3. Achieve ≥80% coverage

## Design Benefits

### Testability
- Interface-based design allows easy mocking
- Dependency injection in constructor
- Each component tested independently

### Simplicity  
- No queuing = no complex state management
- No async processing = no goroutine complexity
- Interface compliance = clear contracts

### Correctness
- Fixes collection proof bug
- Removes incorrect queuing behavior
- Aligns with actual requirements (top-of-chain, list proofs, gap healing)