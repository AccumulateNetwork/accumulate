# CrosschainCoordinator Design

## Overview

The **CrosschainCoordinator** is a comprehensive block construction manager that orchestrates all cross-partition communication for a partition. It takes over responsibility for anchor and synthetic transaction generation from the existing fragmented approach, providing centralized, async management of partition-level operations.

## Architecture Vision

### Current State: Fragmented Block Construction
```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   v2 Executor   │    │ crosschain.      │    │   Dispatcher    │
│                 │    │   Conductor      │    │                 │
│ • Synthetic TX  │───▶│ • Anchor Sending │───▶│ • Network Send  │
│   Generation    │    │ • Event-Driven   │    │ • Error Handle  │
│ • Block Logic   │    │ • Healing        │    │                 │
└─────────────────┘    └──────────────────┘    └─────────────────┘
```

### New State: Unified CrosschainCoordinator
```
┌─────────────────────────────────────────────────────────────────┐
│                    CrosschainCoordinator                          │
│                                                                 │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐ │
│  │ Block Manager   │  │ Anchor Manager  │  │ Synthetic TX    │ │
│  │                 │  │                 │  │ Manager         │ │
│  │ • Block Events  │  │ • Anchor Gen    │  │ • TX Generation │ │
│  │ • Coordination  │  │ • Sequence Mgmt │  │ • Routing       │ │
│  │ • State Mgmt    │  │ • Healing       │  │ • Validation    │ │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘ │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │              Async Transaction Router                       │ │
│  │ • Queue Management  • Error Handling  • Metrics           │ │
│  └─────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
                            ┌─────────────────┐
                            │   Dispatcher    │
                            │ • Network Send  │
                            └─────────────────┘
```

## Core Components

### 1. CrosschainCoordinator (Main Orchestrator)
```go
type CrosschainCoordinator struct {
    // Core partition info
    partition    *protocol.PartitionInfo
    globals      atomic.Pointer[network.GlobalValues]
    validatorKey ed25519.PrivateKey
    
    // Infrastructure
    database     database.Beginner
    querier      api.Querier2
    dispatcher   execute.Dispatcher
    eventBus     *events.Bus
    logger       logging.OptionalLogger
    
    // Managers
    blockManager    *BlockManager
    anchorManager   *AnchorManager
    syntheticManager *SyntheticManager
    router          *AsyncRouter
    
    // Control
    ready       func(execute.WillBeginBlock) bool
    runTask     func(func())
    stopChan    chan struct{}
}
```

### 2. BlockManager (Block Construction Coordination)
```go
type BlockManager struct {
    conductor *CrosschainCoordinator
    
    // Block state
    currentBlock   uint64
    previousBlock  uint64
    blockState     *BlockState
    
    // Event handling
    eventHandlers map[string]func(interface{}) error
}

// Responsibilities:
// - Subscribe to WillBeginBlock events
// - Coordinate anchor and synthetic transaction generation
// - Manage block state transitions
// - Trigger async processing
```

### 3. AnchorManager (Anchor Generation & Management)
```go
type AnchorManager struct {
    conductor *CrosschainCoordinator
    
    // Anchor state
    sequenceNumbers map[string]uint64  // Per-destination sequences
    pendingAnchors  map[string]*PendingAnchor
    
    // Healing
    healingEnabled bool
    healingTracker *HealingTracker
}

// Responsibilities:
// - Generate anchors for completed blocks
// - Manage anchor sequence numbers
// - Handle DN->all and BVN->DN routing
// - Coordinate anchor healing
```

### 4. SyntheticManager (Synthetic Transaction Management)
```go
type SyntheticManager struct {
    conductor *CrosschainCoordinator
    
    // Synthetic TX state
    pendingTransactions map[string][]*SyntheticTransaction
    sequenceTracker     *SequenceTracker
    
    // Validation
    validator *SequenceValidator
}

// Responsibilities:
// - Generate synthetic transactions from block execution
// - Validate sequence numbers
// - Queue out-of-order transactions
// - Coordinate with executor
```

### 5. AsyncRouter (Async Transaction Processing)
```go
type AsyncRouter struct {
    conductor *CrosschainCoordinator
    
    // Async processing
    requestChan   chan *TransactionRequest
    workerPool    []*Worker
    
    // Metrics
    metrics *RouterMetrics
}

// Responsibilities:
// - Async processing of anchor and synthetic transactions
// - Queue management and backpressure
// - Error handling and retries
// - Performance metrics
```

## Integration Strategy

### Phase 1: CrosschainCoordinator Foundation (3 weeks)

#### Week 1: Core Structure
1. **Create CrosschainCoordinator Package**
   ```
   /internal/core/execute/v2/partition/
   ├── conductor.go          # Main CrosschainCoordinator
   ├── block_manager.go      # Block coordination
   ├── anchor_manager.go     # Anchor generation
   ├── synthetic_manager.go  # Synthetic TX management
   ├── async_router.go       # Async processing
   └── types.go             # Shared types
   ```

2. **Basic Integration Points**
   - Event bus subscription
   - Dispatcher integration
   - Database access patterns
   - Logger setup

#### Week 2: Block Manager Implementation
1. **Event Handling**
   ```go
   func (bm *BlockManager) handleWillBeginBlock(e execute.WillBeginBlock) error {
       // Coordinate anchor generation for previous block
       err := bm.conductor.anchorManager.GenerateAnchors(e)
       if err != nil {
           return err
       }
       
       // Coordinate synthetic transaction processing
       err = bm.conductor.syntheticManager.ProcessPendingTransactions(e)
       if err != nil {
           return err
       }
       
       return nil
   }
   ```

2. **State Management**
   - Block transition tracking
   - State consistency validation
   - Error recovery

#### Week 3: Basic Anchor Management
1. **Anchor Generation**
   ```go
   func (am *AnchorManager) GenerateAnchors(e execute.WillBeginBlock) error {
       // Construct anchor for previous block
       anchor, sequenceNumber, err := crosschain.ConstructLastAnchor(
           e.Context, e.Batch, am.conductor.partition.URL())
       if err != nil {
           return err
       }
       
       // Route based on partition type
       return am.routeAnchor(anchor, sequenceNumber)
   }
   ```

2. **Routing Logic**
   - DN → all partitions
   - BVN → DN only
   - Sequence number management

### Phase 2: Migration from Existing Systems (4 weeks)

#### Week 1: Parallel Operation Setup
1. **Feature Flags**
   ```go
   type CrosschainCoordinatorConfig struct {
       EnableAnchorManagement    bool
       EnableSyntheticManagement bool
       EnableAsyncProcessing     bool
   }
   ```

2. **Dual Operation**
   - CrosschainCoordinator handles synthetic transactions
   - Existing crosschain.Conductor continues anchors
   - Validation and comparison

#### Week 2: Anchor Migration
1. **Gradual Migration**
   ```go
   func (pc *CrosschainCoordinator) handleAnchors(e execute.WillBeginBlock) error {
       if pc.config.EnableAnchorManagement {
           return pc.anchorManager.GenerateAnchors(e)
       }
       // Fall back to existing conductor
       return nil
   }
   ```

2. **Validation**
   - Compare anchor generation results
   - Verify sequence number consistency
   - Monitor for discrepancies

#### Week 3: Synthetic Transaction Migration
1. **Executor Integration**
   ```go
   // In v2 executor block_begin.go
   func (x *Executor) sendSyntheticTransactionsForBlock(...) error {
       if x.partitionConductor.IsEnabled() {
           return x.CrosschainCoordinator.SubmitSyntheticTransactions(messages, seq.Destination)
       }
       // Existing direct dispatcher logic
       return x.mainDispatcher.Submit(context.Background(), seq.Destination, env)
   }
   ```

2. **Async Processing**
   - Queue synthetic transactions
   - Async submission via router
   - Error handling and retries

#### Week 4: Legacy Removal
1. **Remove crosschain.Conductor**
   - Update node startup code
   - Remove event subscriptions
   - Clean up imports

2. **Integration Points**
   - Update daemon/run.go
   - Update consensus.go
   - Update simulator factory

### Phase 3: Enhanced Features (3 weeks)

#### Week 1: Sequence Management
1. **Sequence Validation**
   ```go
   type SequenceValidator struct {
       expectedSequences map[string]uint64
       gapTracker       *GapTracker
       timeoutManager   *TimeoutManager
   }
   ```

2. **Gap Detection**
   - Track sequence gaps
   - Queue out-of-order transactions
   - Timeout-based healing triggers

#### Week 2: Healing Integration
1. **Unified Healing**
   ```go
   func (pc *CrosschainCoordinator) healMissingTransactions() error {
       // Heal missing anchors
       err := pc.anchorManager.HealMissingAnchors()
       if err != nil {
           return err
       }
       
       // Heal missing synthetic transactions
       return pc.syntheticManager.HealMissingTransactions()
   }
   ```

2. **Background Healing**
   - Automatic gap detection
   - Peer querying for missing transactions
   - Retry logic with exponential backoff

#### Week 3: Performance Optimization
1. **Async Processing Optimization**
   - Worker pool tuning
   - Queue size optimization
   - Backpressure handling

2. **Metrics and Monitoring**
   - Transaction throughput metrics
   - Error rate monitoring
   - Healing success rates

## Detailed Implementation Plan

### 1. Node Integration Points

#### Current Integration (to be replaced)
```go
// In daemon/run.go:431
conductor := &crosschain.Conductor{
    Partition:    &protocol.PartitionInfo{...},
    ValidatorKey: execOpts.Key,
    Database:     execOpts.Database,
    Querier:      v3.Querier2{Querier: client},
    Dispatcher:   execOpts.NewDispatcher(),
    RunTask:      execOpts.BackgroundTaskLauncher,
    EnableAnchorHealing: &no,
    Ready: func(execute.WillBeginBlock) bool { ... },
}
err := conductor.Start(d.eventBus)
```

#### New Integration
```go
// In daemon/run.go
partitionConductor := &partition.Conductor{
    Partition:    &protocol.PartitionInfo{...},
    ValidatorKey: execOpts.Key,
    Database:     execOpts.Database,
    Querier:      v3.Querier2{Querier: client},
    Dispatcher:   execOpts.NewDispatcher(),
    EventBus:     d.eventBus,
    Logger:       d.Logger,
    RunTask:      execOpts.BackgroundTaskLauncher,
    
    Config: &partition.ConductorConfig{
        EnableAnchorManagement:    true,
        EnableSyntheticManagement: true,
        EnableAsyncProcessing:     true,
        EnableHealing:            true,
    },
    
    Ready: func(execute.WillBeginBlock) bool { ... },
}
err := partitionConductor.Start()
```

### 2. Executor Integration

#### Current Synthetic Transaction Handling
```go
// In block_begin.go:475
if isLeader {
    env := &messaging.Envelope{Messages: messages}
    err = x.mainDispatcher.Submit(context.Background(), seq.Destination, env)
    if err != nil {
        return errors.UnknownError.WithFormat("send synthetic transaction %X: %w", hash[:4], err)
    }
}
```

#### New Synthetic Transaction Handling
```go
// In block_begin.go
if isLeader {
    err = x.partitionConductor.SubmitSyntheticTransactions(messages, seq.Destination)
    if err != nil {
        return errors.UnknownError.WithFormat("send synthetic transaction %X: %w", hash[:4], err)
    }
}
```

### 3. Event Flow

#### Current Event Flow
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
Dispatcher.Submit()
```

#### New Event Flow
```
WillBeginBlock Event
       │
       ▼
CrosschainCoordinator.BlockManager.handleWillBeginBlock()
       │
       ├─▶ AnchorManager.GenerateAnchors()
       │   │
       │   ▼
       │   AsyncRouter.SubmitAnchor()
       │
       └─▶ SyntheticManager.ProcessPending()
           │
           ▼
           AsyncRouter.SubmitSynthetic()
```

## Benefits of CrosschainCoordinator Approach

### 1. Unified Architecture
- ✅ Single component manages all cross-partition communication
- ✅ Centralized state management and coordination
- ✅ Consistent error handling and retry logic
- ✅ Unified metrics and monitoring

### 2. Improved Async Processing
- ✅ Dedicated async processing for both anchors and synthetic transactions
- ✅ Queue management and backpressure handling
- ✅ Worker pool optimization
- ✅ Better resource utilization

### 3. Enhanced Sequence Management
- ✅ Unified sequence tracking across transaction types
- ✅ Gap detection and automatic healing
- ✅ Out-of-order transaction handling
- ✅ Sequence validation before consensus

### 4. Better Testability
- ✅ Clear component boundaries
- ✅ Mockable interfaces
- ✅ Isolated testing of each manager
- ✅ Integration test capabilities

### 5. Operational Benefits
- ✅ Centralized configuration
- ✅ Unified logging and metrics
- ✅ Better debugging capabilities
- ✅ Simplified deployment and monitoring

## Success Criteria

### Phase 1 Success
- ✅ CrosschainCoordinator handles basic anchor generation
- ✅ Event integration working correctly
- ✅ No regression in anchor sending functionality
- ✅ Basic async processing operational

### Phase 2 Success
- ✅ Complete migration from crosschain.Conductor
- ✅ Synthetic transactions route through CrosschainCoordinator
- ✅ Legacy code cleanly removed
- ✅ Performance metrics within 5% of baseline

### Phase 3 Success
- ✅ Sequence management and healing operational
- ✅ Out-of-order transaction handling working
- ✅ Automatic healing reducing manual intervention
- ✅ Performance improvements measurable

## Risk Mitigation

### High Risk: Anchor Sequence Integrity
- **Mitigation**: Parallel operation with validation during migration
- **Rollback**: Feature flags allow immediate reversion
- **Testing**: Comprehensive sequence number validation

### Medium Risk: Event Timing
- **Mitigation**: Maintain exact event handling timing
- **Rollback**: Keep existing event subscription patterns
- **Testing**: Event ordering and timing tests

### Low Risk: Performance Impact
- **Mitigation**: Async processing should improve performance
- **Rollback**: Performance monitoring with automatic rollback
- **Testing**: Load testing and benchmarking

## Conclusion

The CrosschainCoordinator represents a significant architectural improvement that unifies cross-partition communication under a single, well-designed component. By taking over block construction responsibilities from the fragmented current approach, it provides better async processing, sequence management, and healing capabilities while maintaining the exact behavior of the existing system during migration.
