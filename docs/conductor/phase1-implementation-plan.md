# Phase 1: CrosschainCoordinator Implementation Plan

## 🎯 Phase 1 Scope (Simplified)

**Goal**: Create a centralized async processing system for anchor and synthetic transactions with **zero behavior change**.

**Key Principle**: All transactions flow through the CrosschainCoordinator but are processed identically to the current system.

## 🏗️ Implementation Strategy

### 1. Component Naming Resolution
- **Name**: `CrosschainCoordinator` (avoid conflict with existing `crosschain.Conductor`)
- **Package**: `/internal/core/execute/v2/crosschain/`
- **Focus**: Synthetic transactions first (anchors handled by existing conductor)

### 2. Minimal Integration Points

#### A. Synthetic Transaction Integration Only
```go
// In v2/block/block_begin.go - ONLY modify synthetic transaction sending
func (x *Executor) sendSyntheticTransactionsForBlock(...) error {
    // ... existing logic ...
    
    if isLeader {
        if x.crosschainCoordinator != nil {
            // NEW: Route through coordinator
            return x.crosschainCoordinator.SubmitSynthetic(context.Background(), messages, seq.Destination)
        }
        
        // FALLBACK: Existing direct dispatch (unchanged)
        env := &messaging.Envelope{Messages: messages}
        err = x.mainDispatcher.Submit(context.Background(), seq.Destination, env)
        if err != nil {
            return errors.UnknownError.WithFormat("send synthetic transaction %X: %w", hash[:4], err)
        }
    }
    return nil
}
```

#### B. Leave Anchors Unchanged (Phase 1)
- Keep existing `crosschain.Conductor` handling anchors
- No modifications to anchor sending logic
- Avoid naming conflicts and complexity

## 📦 CrosschainCoordinator Structure

### Core Components
```go
package crosschain

// CrosschainCoordinator handles async processing of cross-partition transactions
type CrosschainCoordinator struct {
    // Infrastructure
    dispatcher   execute.Dispatcher
    logger       logging.OptionalLogger
    
    // Async processing
    syntheticChan chan *SyntheticRequest
    stopChan      chan struct{}
    wg            sync.WaitGroup
    
    // Metrics (simple counters)
    syntheticsSent int64
    syntheticsErrors int64
}

// SyntheticRequest represents a synthetic transaction to be sent
type SyntheticRequest struct {
    Messages     []messaging.Message
    Destination  *url.URL
    Context      context.Context
    ResponseChan chan error
}
```

### Key Methods
```go
// NewCrosschainCoordinator creates and starts the coordinator
func NewCrosschainCoordinator(dispatcher execute.Dispatcher, logger logging.OptionalLogger) *CrosschainCoordinator {
    cc := &CrosschainCoordinator{
        dispatcher:    dispatcher,
        logger:        logger.With("module", "crosschain-coordinator"),
        syntheticChan: make(chan *SyntheticRequest, 100),
        stopChan:      make(chan struct{}),
    }
    
    // Start async processor
    cc.wg.Add(1)
    go cc.processSynthetics()
    
    return cc
}

// SubmitSynthetic submits synthetic transactions for async processing
func (cc *CrosschainCoordinator) SubmitSynthetic(ctx context.Context, messages []messaging.Message, destination *url.URL) error {
    responseChan := make(chan error, 1)
    req := &SyntheticRequest{
        Messages:     messages,
        Destination:  destination,
        Context:      ctx,
        ResponseChan: responseChan,
    }
    
    select {
    case cc.syntheticChan <- req:
        return <-responseChan
    case <-ctx.Done():
        return ctx.Err()
    case <-cc.stopChan:
        return errors.InternalError.With("coordinator stopped")
    }
}

// processSynthetics is the main async processing loop
func (cc *CrosschainCoordinator) processSynthetics() {
    defer cc.wg.Done()
    cc.logger.Info("CrosschainCoordinator started")
    
    for {
        select {
        case req := <-cc.syntheticChan:
            cc.processSyntheticRequest(req)
            
        case <-cc.stopChan:
            cc.logger.Info("CrosschainCoordinator stopping")
            // Drain remaining requests
            for {
                select {
                case req := <-cc.syntheticChan:
                    req.ResponseChan <- errors.InternalError.With("coordinator stopping")
                default:
                    return
                }
            }
        }
    }
}

// processSyntheticRequest processes a single synthetic transaction request
func (cc *CrosschainCoordinator) processSyntheticRequest(req *SyntheticRequest) {
    // Phase 1: Direct pass-through to existing dispatcher
    env := &messaging.Envelope{Messages: req.Messages}
    err := cc.dispatcher.Submit(req.Context, req.Destination, env)
    
    // Update metrics
    if err != nil {
        atomic.AddInt64(&cc.syntheticsErrors, 1)
        cc.logger.Error("Synthetic transaction failed", "destination", req.Destination, "error", err)
    } else {
        atomic.AddInt64(&cc.syntheticsSent, 1)
        cc.logger.Debug("Synthetic transaction sent", "destination", req.Destination)
    }
    
    // Send response
    req.ResponseChan <- err
}

// Stop gracefully stops the coordinator
func (cc *CrosschainCoordinator) Stop() {
    close(cc.stopChan)
    cc.wg.Wait()
    cc.logger.Info("CrosschainCoordinator stopped")
}
```

## 🔧 Integration Steps

### Step 1: Create CrosschainCoordinator Package
```
/internal/core/execute/v2/crosschain/
├── coordinator.go          # Main CrosschainCoordinator implementation
├── types.go               # Request types and interfaces
└── coordinator_test.go    # Unit tests
```

### Step 2: Add to V2 Executor
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
    
    // Initialize CrosschainCoordinator (feature flagged)
    if opts.EnableCrosschainCoordinator {
        m.crosschainCoordinator = crosschain.NewCrosschainCoordinator(
            m.mainDispatcher,
            opts.Logger,
        )
    }
    
    return m, nil
}
```

### Step 3: Update ExecutorOptions
```go
// In execute/execute.go
type ExecutorOptions struct {
    // ... existing fields ...
    EnableCrosschainCoordinator bool  // Feature flag for Phase 1
}
```

### Step 4: Modify Synthetic Transaction Sending
Only modify the synthetic transaction sending in `v2/block/block_begin.go` as shown above.

## ✅ Success Criteria for Phase 1

1. **Zero Behavior Change**: All synthetic transactions processed identically
2. **Async Processing**: Transaction creation moved to separate goroutine
3. **Feature Flag**: Can be enabled/disabled without code changes
4. **Logging**: Clear logging of async processing
5. **Metrics**: Basic counters for sent/failed transactions
6. **Graceful Shutdown**: Proper cleanup on node shutdown

## 📊 Implementation Effort

- **New Code**: ~300 lines (coordinator + tests)
- **Modified Code**: ~15 lines (executor integration + synthetic transaction routing)
- **Risk Level**: **LOW** (feature flagged with fallback)
- **Implementation Time**: 2-3 days

## 🧪 Testing Strategy

1. **Unit Tests**: Test coordinator async processing
2. **Integration Tests**: Test with feature flag enabled/disabled
3. **Performance Tests**: Verify no throughput degradation
4. **Network Tests**: Test on devnet with synthetic transactions

## 🚀 Next Steps

1. Implement CrosschainCoordinator package
2. Add feature flag to ExecutorOptions
3. Integrate with V2 executor
4. Add unit tests
5. Test on devnet
6. Deploy with feature flag disabled initially
7. Enable feature flag and monitor

This focused Phase 1 approach:
- **Minimizes risk** by only touching synthetic transactions
- **Avoids conflicts** with existing anchor system
- **Provides foundation** for Phase 2 error handling
- **Enables gradual rollout** with feature flags
- **Maintains zero behavior change** requirement
