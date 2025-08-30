# CrosschainCoordinator: Channel-Based Design

## Architecture Overview

**KEY INSIGHT**: The CrosschainCoordinator doesn't need to subscribe to events directly. Instead, the existing `crosschain.Conductor` can delegate anchor and synthetic transaction processing to our CrosschainCoordinator via channels.

## Simplified Architecture

### Current State
```
WillBeginBlock Event
       │
       ▼
crosschain.Conductor.willBeginBlock()
       │
       ▼
sendAnchorForLastBlock()
       │
       ▼
Dispatcher.Submit() [DIRECT]
```

### New State: Channel-Based Delegation
```
WillBeginBlock Event
       │
       ▼
crosschain.Conductor.willBeginBlock()
       │
       ├─▶ AnchorRequest → Channel → CrosschainCoordinator.AnchorManager
       │                                      │
       │                                      ▼
       │                              AsyncRouter.ProcessAnchor()
       │                                      │
       │                                      ▼
       │                              Dispatcher.Submit()
       │
       └─▶ SyntheticRequest → Channel → CrosschainCoordinator.SyntheticManager
                                              │
                                              ▼
                                      AsyncRouter.ProcessSynthetic()
                                              │
                                              ▼
                                      Dispatcher.Submit()
```

## Core Components

### 1. CrosschainCoordinator (Main Orchestrator)
```go
type CrosschainCoordinator struct {
    // Infrastructure
    dispatcher   execute.Dispatcher
    logger       logging.OptionalLogger
    
    // Processing channels
    anchorChan     chan *AnchorRequest
    syntheticChan  chan *SyntheticRequest
    
    // Managers
    anchorManager    *AnchorManager
    syntheticManager *SyntheticManager
    asyncRouter      *AsyncRouter
    
    // Control
    stopChan chan struct{}
    wg       sync.WaitGroup
}
```

### 2. Request Types
```go
type AnchorRequest struct {
    Anchor         protocol.AnchorBody
    SequenceNumber uint64
    Destination    string
    Context        context.Context
    ResponseChan   chan error  // For synchronous error reporting
}

type SyntheticRequest struct {
    Messages     []messaging.Message
    Destination  *url.URL
    Context      context.Context
    ResponseChan chan error  // For synchronous error reporting
}
```

### 3. Modified Existing Conductor
```go
// In crosschain.Conductor
type Conductor struct {
    // ... existing fields ...
    
    // NEW: CrosschainCoordinator integration
    crosschainCoordinator *crosschain.CrosschainCoordinator
}

func (c *Conductor) sendBlockAnchor(ctx context.Context, anchor protocol.AnchorBody, sequenceNumber uint64, destPart string) error {
    if c.crosschainCoordinator != nil {
        // Delegate to CrosschainCoordinator
        return c.crosschainCoordinator.SubmitAnchor(ctx, anchor, sequenceNumber, destPart)
    }
    
    // Fallback to existing logic
    destination := protocol.PartitionUrl(destPart)
    env, _, err := ValidatorContext{
        Source:       c.Partition,
        Globals:      c.Globals.Load(),
        ValidatorKey: c.ValidatorKey,
    }.PrepareAnchorSubmission(ctx, anchor, sequenceNumber, destination)
    if err != nil {
        return errors.UnknownError.Wrap(err)
    }
    return c.submit(ctx, destination, env)
}
```

### 4. V2 Executor Integration
```go
// In v2/block/block_begin.go
func (x *Executor) sendSyntheticTransactionsForBlock(...) error {
    // ... existing logic ...
    
    if isLeader {
        if x.crosschainCoordinator != nil {
            // Delegate to CrosschainCoordinator
            return x.crosschainCoordinator.SubmitSyntheticTransactions(context.Background(), messages, seq.Destination)
        }
        
        // Fallback to existing logic
        env := &messaging.Envelope{Messages: messages}
        err = x.mainDispatcher.Submit(context.Background(), seq.Destination, env)
        if err != nil {
            return errors.UnknownError.WithFormat("send synthetic transaction %X: %w", hash[:4], err)
        }
    }
    return nil
}
```

## Integration Strategy

### Phase 1: CrosschainCoordinator Foundation (1 week)

#### Create CrosschainCoordinator Package
```
/internal/core/execute/v2/crosschain/
├── conductor.go              # Main CrosschainCoordinator
├── anchor_manager.go         # Anchor processing
├── synthetic_manager.go      # Synthetic TX processing
├── async_router.go           # Async processing
├── types.go                  # Request types
└── README.md                 # Documentation
```

#### Basic Implementation
```go
func (cc *CrosschainCoordinator) Start() error {
    // Start worker goroutines
    cc.wg.Add(2)
    go cc.processAnchors()
    go cc.processSynthetics()
    return nil
}

func (cc *CrosschainCoordinator) processAnchors() {
    defer cc.wg.Done()
    for {
        select {
        case req := <-cc.anchorChan:
            err := cc.anchorManager.ProcessAnchor(req)
            if req.ResponseChan != nil {
                req.ResponseChan <- err
            }
        case <-cc.stopChan:
            return
        }
    }
}

func (cc *CrosschainCoordinator) SubmitAnchor(ctx context.Context, anchor protocol.AnchorBody, sequenceNumber uint64, destination string) error {
    responseChan := make(chan error, 1)
    req := &AnchorRequest{
        Anchor:         anchor,
        SequenceNumber: sequenceNumber,
        Destination:    destination,
        Context:        ctx,
        ResponseChan:   responseChan,
    }
    
    select {
    case cc.anchorChan <- req:
        return <-responseChan
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

### Phase 2: Existing Conductor Integration (1 week)

#### Modify crosschain.Conductor
```go
// In /internal/core/crosschain/conductor.go
func NewConductor(opts ConductorOptions) *Conductor {
    c := &Conductor{
        // ... existing initialization ...
    }
    
    // Initialize CrosschainCoordinator if enabled
    if opts.EnableCrosschainCoordinator {
        c.crosschainConductor = crosschain.NewConductor(crosschain.ConductorOptions{
            Dispatcher: opts.Dispatcher,
            Logger:     opts.Logger,
        })
        c.crosschainConductor.Start()
    }
    
    return c
}
```

#### Update Node Startup
```go
// In daemon/run.go, consensus.go, factory.go
conductor := &crosschain.Conductor{
    // ... existing fields ...
    
    // NEW: Enable CrosschainCoordinator
    EnableCrosschainCoordinator: true,
}
```

### Phase 3: V2 Executor Integration (1 week)

#### Add CrosschainCoordinator to Executor
```go
// In v2/block/executor.go
type Executor struct {
    // ... existing fields ...
    crosschainCoordinator *crosschain.CrosschainCoordinator
}

func NewExecutor(opts execute.ExecutorOptions) (*Executor, error) {
    m := &Executor{
        // ... existing initialization ...
    }
    
    // Initialize CrosschainCoordinator
    if opts.EnableCrosschainCoordinator {
        m.crosschainConductor = crosschain.NewConductor(crosschain.ConductorOptions{
            Dispatcher: m.mainDispatcher,
            Logger:     opts.Logger,
        })
        m.crosschainConductor.Start()
    }
    
    return m, nil
}
```

#### Update Synthetic Transaction Handling
```go
// In v2/block/block_begin.go
func (x *Executor) sendSyntheticTransactionsForBlock(...) error {
    // ... existing logic ...
    
    if isLeader {
        if x.crosschainConductor != nil {
            return x.crosschainConductor.SubmitSyntheticTransactions(context.Background(), messages, seq.Destination)
        }
        
        // Fallback to existing direct dispatch
        env := &messaging.Envelope{Messages: messages}
        err = x.mainDispatcher.Submit(context.Background(), seq.Destination, env)
        if err != nil {
            return errors.UnknownError.WithFormat("send synthetic transaction %X: %w", hash[:4], err)
        }
    }
    return nil
}
```

### Phase 4: Enhanced Features (2 weeks)

#### Add Sequence Management
```go
type SequenceManager struct {
    sequences map[string]uint64  // Per-destination sequence tracking
    gaps      map[string][]uint64 // Gap tracking
    mutex     sync.RWMutex
}

func (sm *SequenceManager) ValidateSequence(destination string, sequence uint64) error {
    sm.mutex.Lock()
    defer sm.mutex.Unlock()
    
    expected := sm.sequences[destination] + 1
    if sequence == expected {
        sm.sequences[destination] = sequence
        return nil
    }
    
    if sequence > expected {
        // Gap detected, queue for later
        sm.gaps[destination] = append(sm.gaps[destination], sequence)
        return ErrSequenceGap
    }
    
    // Duplicate or old sequence
    return ErrDuplicateSequence
}
```

#### Add Healing Integration
```go
func (cc *CrosschainCoordinator) healMissingTransactions() {
    // Use existing healing package
    for destination, gaps := range cc.sequenceManager.GetGaps() {
        for _, sequence := range gaps {
            cc.healingManager.RequestTransaction(destination, sequence)
        }
    }
}
```

## Benefits of Channel-Based Approach

### 1. **Minimal Risk**
- ✅ No changes to event system
- ✅ No changes to node startup complexity
- ✅ Existing conductor continues to work
- ✅ Easy rollback via feature flags

### 2. **Clean Separation**
- ✅ CrosschainCoordinator focuses on async processing
- ✅ Existing conductor handles events and coordination
- ✅ Clear interface via channels
- ✅ Independent testing of each component

### 3. **Gradual Migration**
- ✅ Can enable CrosschainCoordinator per partition
- ✅ Can enable per transaction type (anchors vs synthetics)
- ✅ Performance comparison between approaches
- ✅ Easy A/B testing

### 4. **Enhanced Capabilities**
- ✅ Async processing with worker pools
- ✅ Queue management and backpressure
- ✅ Sequence validation and gap detection
- ✅ Automatic healing integration
- ✅ Comprehensive metrics and monitoring

## Code Impact Analysis

### Files Modified (Minimal Changes)
1. **`/internal/core/crosschain/conductor.go`** - Add CrosschainCoordinator integration (~20 lines)
2. **`/internal/core/execute/v2/block/executor.go`** - Add CrosschainCoordinator field (~15 lines)
3. **`/internal/core/execute/v2/block/block_begin.go`** - Route synthetic transactions (~10 lines)
4. **Node startup files** - Add feature flag (~5 lines each × 3 files)

### Files Created (New Implementation)
1. **CrosschainCoordinator package** - ~1500 lines total
2. **Test files** - ~800 lines
3. **Documentation** - ~500 lines

### Total Impact
- **Modified Code**: ~65 lines across 6 files
- **New Code**: ~2800 lines
- **Risk Level**: LOW (feature flagged, fallback to existing logic)

## Migration Timeline

### Week 1: Foundation
- Create CrosschainCoordinator package
- Implement basic channel processing
- Add anchor and synthetic managers
- Unit tests

### Week 2: Integration
- Modify existing conductor to delegate via channels
- Add feature flags to node startup
- Integration testing
- Performance validation

### Week 3: V2 Executor
- Add CrosschainCoordinator to v2 executor
- Route synthetic transactions via channels
- End-to-end testing
- Performance comparison

### Week 4: Enhancement
- Add sequence management
- Implement gap detection
- Healing integration
- Comprehensive testing

### Week 5: Production
- Feature flag rollout
- Monitoring and metrics
- Performance optimization
- Documentation

## Success Criteria

### Phase 1 Success
- ✅ CrosschainCoordinator processes anchor requests via channels
- ✅ Zero behavior change when disabled
- ✅ Performance within 5% when enabled
- ✅ All unit tests passing

### Phase 2 Success
- ✅ Existing conductor delegates to CrosschainCoordinator
- ✅ Feature flags working correctly
- ✅ Fallback logic functional
- ✅ Integration tests passing

### Phase 3 Success
- ✅ Synthetic transactions route via CrosschainCoordinator
- ✅ V2 executor integration working
- ✅ End-to-end tests passing
- ✅ Performance improvements measurable

### Phase 4 Success
- ✅ Sequence management operational
- ✅ Gap detection and healing working
- ✅ Production deployment successful
- ✅ Monitoring and alerting functional

## Conclusion

The channel-based approach provides a **low-risk, high-value** path to implementing the CrosschainCoordinator. By leveraging the existing conductor for event handling and using channels for delegation, we achieve:

- **Minimal code changes** to existing systems
- **Easy rollback** via feature flags
- **Independent testing** of new functionality
- **Gradual migration** with performance validation
- **Enhanced capabilities** without disrupting existing flows

This approach respects the existing architecture while providing a clean path to async processing and sequence management improvements.
