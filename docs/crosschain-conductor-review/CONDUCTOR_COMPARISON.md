# Comparison: Original Conductor vs CrossChainConductor (CCC)

## Timeline & Context

Both conductors are **NEW in v1.5** - neither existed in v1.4.x:

1. **Original Conductor** (`internal/core/crosschain/conductor.go`)
   - Introduced: November 5, 2023 (commit ffe38e11f)
   - Purpose: Push-style anchor healing (#3423)
   - Author: Ethan Reesor

2. **CrossChainConductor (CCC)** (`internal/core/execute/v2/crosschain/conductor.go`)
   - Introduced: August 8, 2025 (commit 01289e4b2)
   - Purpose: Partition failure handling & async processing
   - Author: Paul Snow (with Claude)

## Architecture Comparison

### Original Conductor (November 2023)

```go
// Simple, synchronous design
type Conductor struct {
    Partition    *protocol.PartitionInfo
    Globals      atomic.Pointer[network.GlobalValues]
    ValidatorKey ed25519.PrivateKey
    Database     database.Beginner
    Querier      api.Querier2
    Dispatcher   execute.Dispatcher
    
    // Optional hooks
    Ready        func(execute.WillBeginBlock) bool
    RunTask      func(func())
    Intercept    interceptor
}
```

**Key Features:**
- Event-driven via bus subscription
- Synchronous anchor submission
- Direct dispatcher calls
- Minimal state tracking
- Focus on anchor healing

**Flow:**
```
Event Bus → willBeginBlock → sendBlockAnchor → Dispatcher.Submit
```

### CrossChainConductor (August 2025)

```go
// Complex, async design with queuing
type CrossChainConductor struct {
    dispatcher execute.Dispatcher
    logger     logging.OptionalLogger
    
    // Async processing channels
    syntheticChan chan *SyntheticRequest
    retryChan     chan *PendingTransmission
    
    // State management
    destinations    map[DestinationKey]*DestinationQueue
    pendingTx       map[string]*PendingTransmission
    
    // Metrics
    syntheticsSent     int64
    syntheticsQueued   int64
    syntheticsErrors   int64
    transmissionErrors int64
}
```

**Key Features:**
- Async processing with worker pools
- Queue management per destination
- Retry mechanisms with exponential backoff
- Partition blocking/unblocking
- Comprehensive metrics
- Error recovery

**Flow:**
```
Submit → Queue → Worker Pool → Dispatcher → Monitor Errors → Retry/Unblock
```

## Feature Comparison

| Feature | Original Conductor | CrossChainConductor |
|---------|-------------------|---------------------|
| **Purpose** | Anchor healing & routing | Full cross-partition orchestration |
| **Design** | Synchronous, event-driven | Asynchronous, channel-based |
| **Scope** | Anchors primarily | Anchors + Synthetics |
| **Error Handling** | Basic, returns errors | Comprehensive retry with backoff |
| **State Management** | Minimal | Per-destination queues |
| **Partition Failure** | No handling | Blocking/unblocking logic |
| **Metrics** | None | Detailed counters |
| **Complexity** | ~225 lines | ~1000+ lines |
| **Dependencies** | Events bus | Worker pools |

## Transaction Routing

### Original Conductor
```go
func (c *Conductor) sendBlockAnchor(...) error {
    // Prepare envelope
    env, _, err := ValidatorContext{...}.PrepareAnchorSubmission(...)
    
    // Direct submission
    return c.Dispatcher.Submit(ctx, destination, env)
}
```

### CrossChainConductor
```go
func (cc *CrossChainConductor) SubmitSynthetic(...) error {
    // Create request
    req := &SyntheticRequest{...}
    
    // Queue or send based on state
    if queue.IsBlocked {
        queue.QueuedRequests = append(queue.QueuedRequests, req)
    } else {
        cc.syntheticChan <- req  // Async processing
    }
}
```

## Use Cases

### Original Conductor Handles:
1. **Anchor broadcasting** (DN → all BVNs, BVN → DN)
2. **Anchor healing** (recovery of missing anchors)
3. **Basic routing** with validator signatures

### CrossChainConductor Handles:
1. **Synthetic transaction routing** with queuing
2. **Partition failure scenarios** with blocking
3. **Automatic retry** with exponential backoff
4. **Load distribution** across worker pools
5. **Error recovery** and monitoring
6. **Anchor submission** (has method but not fully integrated)

## Integration Points

### Where They're Used:

**Original Conductor:**
- Started in `daemon/run.go` as a service
- Subscribes to block events
- Handles anchor submission at block boundaries

**CrossChainConductor:**
- Created in `block/executor.go` when `EnableCrosschainCoordinator: true`
- Used in `block_end.go` for synthetic transactions
- Used in `exec_process.go` for inbound processing

## Current State (as of codebase)

```go
// internal/node/daemon/run.go:407
EnableCrosschainCoordinator: true  // CCC is enabled by default

// Both conductors are running:
// 1. Original conductor handles anchors via events
// 2. CCC handles synthetic transactions via direct calls
```

## Key Differences Summary

1. **Philosophy**: Original is simple/synchronous, CCC is complex/async
2. **Scope**: Original focuses on anchors, CCC on full cross-partition flow
3. **Error Handling**: Original fails fast, CCC retries with recovery
4. **State**: Original is stateless, CCC maintains queues and metrics
5. **Integration**: Original uses events, CCC uses direct method calls

## Conclusion

Both conductors are **new in v1.5** and serve different purposes:
- **Original Conductor**: Lightweight anchor routing and healing
- **CrossChainConductor**: Comprehensive cross-partition orchestration

They currently **work together** in the system, with the original handling anchor events and CCC handling synthetic transaction routing with advanced error recovery.