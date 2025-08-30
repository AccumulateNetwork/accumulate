# Minimal Conductor Implementation Guide

This document provides a step-by-step guide for implementing the **minimal viable Conductor** - a pass-through router that intercepts anchor and synthetic transactions without changing behavior, setting the foundation for full sequence management.

## ⚠️ CRITICAL DISCOVERY: EXISTING CONDUCTOR CONFLICT

**STATUS: DESIGN NEEDS MAJOR REVISION**

During implementation analysis, we discovered that Accumulate already has a `crosschain.Conductor` that handles anchor sending between partitions. This creates a fundamental conflict with our proposed design.

### Existing crosschain.Conductor
- **Location**: `/internal/core/crosschain/conductor.go`
- **Purpose**: Event-driven anchor sending between partitions
- **Scope**: Handles anchors only (not synthetic transactions)
- **Architecture**: Event-driven via `WillBeginBlock` events
- **Integration**: Already integrated and working

### Design Impact
1. **Name Conflict**: Cannot use "Conductor" name
2. **Scope Overlap**: Existing conductor already handles anchors
3. **Architecture Mismatch**: Event-driven vs our proposed API-level interception
4. **Integration Complexity**: Must work with existing anchor system

---

## 🎯 Phase 1 Goals

- **Zero Behavior Change**: Transactions flow exactly as before
- **Async Processing**: Move transaction creation to separate goroutine
- **Foundation**: Establish the Conductor structure for future phases
- **Risk Mitigation**: Prove async transaction creation works without changing behavior

## 📋 Implementation Steps

### Step 1: Create Conductor Structure

**File**: `internal/core/execute/v2/block/conductor/conductor.go`

```go
package conductor

import (
    "context"
    
    "gitlab.com/AccumulateNetwork/accumulate/pkg/types/messaging"
    "gitlab.com/AccumulateNetwork/accumulate/pkg/url"
    "gitlab.com/AccumulateNetwork/accumulate/internal/logging"
    "gitlab.com/AccumulateNetwork/accumulate/pkg/errors"
)

// Dispatcher interface for submitting transactions to partitions
type Dispatcher interface {
    Submit(ctx context.Context, dest *url.URL, envelope *messaging.Envelope) error
}

// TransactionType identifies the type of transaction being routed
type TransactionType int

const (
    AnchorTransaction TransactionType = iota
    SyntheticTransaction
    OtherTransaction
)

// TransactionRequest represents a request to create and send a transaction
type TransactionRequest struct {
    Type        TransactionType
    Destination *url.URL
    Data        interface{} // Transaction-specific data
    Response    chan error  // Response channel for async result
}

// Conductor orchestrates anchor and synthetic transaction flow
// Runs as a separate goroutine to handle transaction creation asynchronously
type Conductor struct {
    mainDispatcher Dispatcher
    logger         logging.OptionalLogger
    
    // Async processing
    requestChan chan *TransactionRequest
    stopChan    chan struct{}
    
    // Metrics for monitoring (optional for Phase 1)
    metricsAnchorsSent    int64
    metricsSyntheticsSent int64
}

// NewConductor creates a new Conductor instance
func NewConductor(dispatcher Dispatcher, logger logging.OptionalLogger) *Conductor {
    c := &Conductor{
        mainDispatcher: dispatcher,
        logger:         logger.With("module", "conductor"),
        requestChan:    make(chan *TransactionRequest, 100), // Buffered channel
        stopChan:       make(chan struct{}),
    }
    
    // Start the conductor goroutine
    go c.run()
    
    return c
}

// RequestTransaction requests async transaction creation and sending
// Phase 1: Move transaction creation to separate goroutine
func (c *Conductor) RequestTransaction(txType TransactionType, destination *url.URL, data interface{}) error {
    // Create request
    req := &TransactionRequest{
        Type:        txType,
        Destination: destination,
        Data:        data,
        Response:    make(chan error, 1),
    }
    
    // Send to conductor goroutine
    select {
    case c.requestChan <- req:
        // Wait for async processing result
        return <-req.Response
    case <-c.stopChan:
        return errors.InternalError.With("conductor stopped")
    }
}

// run is the main conductor goroutine that processes transaction requests
func (c *Conductor) run() {
    c.logger.Info("Conductor started")
    defer c.logger.Info("Conductor stopped")
    
    for {
        select {
        case req := <-c.requestChan:
            c.processTransactionRequest(req)
            
        case <-c.stopChan:
            // Drain remaining requests
            for {
                select {
                case req := <-c.requestChan:
                    req.Response <- errors.InternalError.With("conductor stopping")
                default:
                    return
                }
            }
        }
    }
}

// processTransactionRequest handles a single transaction request
func (c *Conductor) processTransactionRequest(req *TransactionRequest) {
    var err error
    
    // Log the processing for verification
    switch req.Type {
    case AnchorTransaction:
        c.logger.Debug("Processing anchor transaction", "destination", req.Destination)
        err = c.processAnchorTransaction(req)
        c.metricsAnchorsSent++
        
    case SyntheticTransaction:
        c.logger.Debug("Processing synthetic transaction", "destination", req.Destination)
        err = c.processSyntheticTransaction(req)
        c.metricsSyntheticsSent++
        
    default:
        err = errors.InternalError.WithFormat("unknown transaction type: %v", req.Type)
    }
    
    // Send response back
    req.Response <- err
}

// processAnchorTransaction processes an anchor transaction request
func (c *Conductor) processAnchorTransaction(req *TransactionRequest) error {
    // Phase 1: Direct processing - no behavior change
    // In future phases, this will handle sequence management and gap detection
    
    // Extract envelope from request data
    envelope, ok := req.Data.(*messaging.Envelope)
    if !ok {
        return errors.InternalError.With("invalid anchor transaction data")
    }
    
    // Send via existing dispatcher
    return c.mainDispatcher.Submit(context.Background(), req.Destination, envelope)
}

// processSyntheticTransaction processes a synthetic transaction request  
func (c *Conductor) processSyntheticTransaction(req *TransactionRequest) error {
    // Phase 1: Direct processing - no behavior change
    // In future phases, this will handle sequence management and gap detection
    
    // Extract envelope from request data
    envelope, ok := req.Data.(*messaging.Envelope)
    if !ok {
        return errors.InternalError.With("invalid synthetic transaction data")
    }
    
    // Send via existing dispatcher
    return c.mainDispatcher.Submit(context.Background(), req.Destination, envelope)
}

// Stop gracefully stops the conductor
func (c *Conductor) Stop() {
    close(c.stopChan)
}

// GetMetrics returns current routing metrics
func (c *Conductor) GetMetrics() (anchors, synthetics int64) {
    return c.metricsAnchorsSent, c.metricsSyntheticsSent
}
```

### Step 2: Add Conductor to Executor

**File**: `internal/core/execute/v2/block/executor.go`

```go
// Add import for conductor package
import (
    // ... existing imports ...
    "gitlab.com/AccumulateNetwork/accumulate/internal/core/execute/v2/block/conductor"
)

// Add to Executor struct
type Executor struct {
    // ... existing fields ...
    mainDispatcher Dispatcher
    conductor      *conductor.Conductor  // NEW
}

// Modify NewExecutor function
func NewExecutor(opts ExecutorOptions) (*Executor, error) {
    // ... existing initialization ...
    
    m := &Executor{
        // ... existing fields ...
        mainDispatcher: opts.NewDispatcher(),
    }
    
    // Initialize Conductor (starts its own goroutine)
    m.conductor = conductor.NewConductor(m.mainDispatcher, opts.Logger)
    
    return m, nil
}

// Add cleanup in executor shutdown
func (m *Executor) Stop() {
    // ... existing cleanup ...
    
    // Stop the conductor goroutine
    m.conductor.Stop()
}
```

### Step 3: Route Synthetic Transactions

**File**: `internal/core/execute/v2/block/block_begin.go`

**Find this line** (around line 475):
```go
err = x.mainDispatcher.Submit(context.Background(), seq.Destination, env)
```

**Replace with**:
```go
err = x.conductor.RequestTransaction(conductor.SyntheticTransaction, seq.Destination, env)
```

**Complete context** (lines 472-480):
```go
// Only send synthetic transactions from the leader
if isLeader {
    env := &messaging.Envelope{Messages: messages}
    // OLD: err = x.mainDispatcher.Submit(context.Background(), seq.Destination, env)
    // NEW: Request async processing through Conductor
    err = x.conductor.RequestTransaction(conductor.SyntheticTransaction, seq.Destination, env)
    if err != nil {
        return errors.UnknownError.WithFormat("send synthetic transaction %X: %w", hash[:4], err)
    }
}
```

### Step 4: Route Anchor Transactions

**File**: `internal/core/execute/v1/block/block_begin.go` (if using v1)
**OR**: Create similar routing in v2 anchor sending code

**Find this line** (around line 489):
```go
err = x.mainDispatcher.Submit(context.Background(), destPartUrl, env)
```

**Replace with**:
```go
err = x.conductor.RequestTransaction(conductor.AnchorTransaction, destPartUrl, env)
```

**Complete context**:
```go
// Only send anchors from a validator
if x.isValidator {
    // OLD: err = x.mainDispatcher.Submit(context.Background(), destPartUrl, env)
    // NEW: Request async processing through Conductor
    err = x.conductor.RequestTransaction(conductor.AnchorTransaction, destPartUrl, env)
    if err != nil {
        return errors.UnknownError.Wrap(err)
    }
}
```

### Step 5: Add Conductor to v1 Executor (if needed)

**File**: `internal/core/execute/v1/block/executor.go`

```go
// Add import for conductor package
import (
    // ... existing imports ...
    "gitlab.com/AccumulateNetwork/accumulate/internal/core/execute/v2/block/conductor"
)

// Add to Executor struct
type Executor struct {
    // ... existing fields ...
    mainDispatcher Dispatcher
    conductor      *conductor.Conductor  // NEW
}

// Add to initialization
func (m *Executor) EnableTimers() {
    // ... existing code ...
    
    // Initialize Conductor
    m.conductor = conductor.NewConductor(m.mainDispatcher, m.logger)
}
```

## 🧪 Testing the Minimal Conductor

### Test 1: Verify Async Processing

**Add logging to verify transactions are being processed asynchronously:**

```bash
# Look for Conductor logs in validator output
grep "Conductor started" /path/to/validator/logs
grep "Processing anchor transaction" /path/to/validator/logs
grep "Processing synthetic transaction" /path/to/validator/logs
```

### Test 2: Verify No Behavior Change

**Run existing tests to ensure no regression:**

```bash
# Run block execution tests
go test ./internal/core/execute/v2/block/...

# Run integration tests
go test ./test/e2e/...
```

### Test 3: Check Metrics

**Add a simple metrics endpoint or log output:**

```go
// Add to Conductor
func (c *Conductor) LogMetrics() {
    anchors, synthetics := c.GetMetrics()
    c.logger.Info("Conductor metrics", 
        "anchors_routed", anchors,
        "synthetics_routed", synthetics)
}

// Success Criteria

// Phase 1 Complete When:

1. **✅ All anchor transactions** go through `conductor.RequestTransaction(conductor.AnchorTransaction, ...)`
2. **✅ All synthetic transactions** go through `conductor.RequestTransaction(conductor.SyntheticTransaction, ...)`  
3. **✅ Conductor goroutine** is running and processing requests asynchronously
4. **✅ Zero behavior change** - all transactions still reach their destinations
5. **✅ Logging confirms** async processing is working
6. **✅ All tests pass** - no regressions introduced
7. **✅ Metrics show** transaction counts match expected volume
8. **✅ Graceful shutdown** - conductor stops cleanly when executor stops

// Verification Commands

# Compile and test
go build ./internal/core/execute/v2/block/conductor/
go test ./internal/core/execute/v2/block/conductor/
go build ./internal/core/execute/v2/block/
go test ./internal/core/execute/v2/block/

# Check for compilation errors
go build ./cmd/accumulated/

# Run validator and check logs
./accumulated run devnet --work-dir /tmp/accumulate-test
```

## 🔄 Next Steps

Once Phase 1 is complete and verified:

1. **Phase 2**: Add sequence number tracking
2. **Phase 3**: Add gap detection and holding logic  
3. **Phase 4**: Add timeout-based healing
4. **Phase 5**: Add monitoring and optimization

## 🚨 Rollback Plan

If issues arise, **easily rollback** by:

1. **Remove Conductor calls**:
   ```go
   // Change back to:
   err = x.mainDispatcher.Submit(context.Background(), destination, env)
   ```

2. **Remove Conductor field** from Executor struct

3. **Remove conductor.Stop()** from executor shutdown

4. **Delete conductor.go** file

The minimal implementation is designed to be **completely reversible** with no lasting changes to the codebase.

## 📊 Estimated Effort

- **Implementation**: 4-6 hours
- **Testing**: 2-3 hours  
- **Documentation**: 1 hour
- **Total**: **1 day**

This minimal implementation provides the foundation for all future Conductor functionality while maintaining zero risk to existing operations.
