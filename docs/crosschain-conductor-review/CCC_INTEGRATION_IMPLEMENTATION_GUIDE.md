# CrossChainConductor (CCC) Integration Implementation Guide

## Table of Contents
1. [Overview](#overview)
2. [Current Implementation Points](#current-implementation-points)
3. [Integration Requirements](#integration-requirements)
4. [Phase 1: Minimal Integration](#phase-1-minimal-integration)
5. [Phase 2: Adapter Pattern](#phase-2-adapter-pattern)
6. [Phase 3: Full Unification](#phase-3-full-unification)
7. [Testing Strategy](#testing-strategy)
8. [Rollback Plan](#rollback-plan)

## Overview

This document provides a detailed implementation guide for integrating the Original Conductor with the CrossChainConductor (CCC) to create a unified cross-partition message routing system.

### Current State
- **Original Conductor**: Handles anchors via event subscription
- **CrossChainConductor**: Handles synthetic transactions via direct calls
- Both use the same underlying dispatcher but with different paradigms

## Current Implementation Points

### 1. Original Conductor Entry Points

#### Event Subscription
**File**: `internal/core/crosschain/conductor.go:58-61`
```go
func (c *Conductor) Start(bus *events.Bus) error {
    events.SubscribeSync(bus, c.willBeginBlock)
    events.SubscribeSync(bus, c.willChangeGlobals)
    return nil
}
```

#### Anchor Processing
**File**: `internal/core/crosschain/conductor.go:73-131`
```go
func (c *Conductor) willBeginBlock(e execute.WillBeginBlock) error {
    // Line 73: Entry point for anchor processing
    // Line 98-122: Anchor healing logic
    // Line 124-131: Load ledger and trigger anchor send
}
```

#### Anchor Construction and Sending
**File**: `internal/core/crosschain/conductor.go:133-179`
```go
func (c *Conductor) sendAnchors(e execute.WillBeginBlock, ledger *protocol.SystemLedger) error {
    // Line 147-158: Construct anchor
    // Line 162-176: Route based on partition type
}
```

**File**: `internal/core/crosschain/conductor.go:181-202`
```go
func (c *Conductor) sendBlockAnchor(ctx context.Context, anchor protocol.AnchorBody, sequenceNumber uint64, destPart string) error {
    // Line 191-198: Prepare anchor submission
    // Line 201: Submit via dispatcher
}
```

### 2. CrossChainConductor Entry Points

#### Initialization
**File**: `internal/core/execute/v2/block/executor.go:119-123`
```go
// Initialize crosschain conductor if enabled
if opts.EnableCrosschainCoordinator {
    m.crosschainConductor = crosschain.NewCrossChainConductor(m.mainDispatcher, m.logger)
    m.logger.Info("CrossChainConductor enabled for routing anchor and synthetic transactions")
}
```

#### Synthetic Transaction Routing
**File**: `internal/core/execute/v2/block/block_end.go:581-589`
```go
// Route through crosschain conductor if enabled, otherwise use direct dispatcher
if x.crosschainConductor != nil {
    // Use crosschain conductor for coordinated routing
    err = x.crosschainConductor.SubmitSynthetic(ctx, []messaging.Message{msg}, dest)
    // ...
}
```

#### Inbound Processing
**File**: `internal/core/execute/v2/block/exec_process.go:51-53`
```go
// Route inbound cross-partition messages through crosschain conductor if enabled
if b.Executor.crosschainConductor != nil {
    messages = b.Executor.crosschainConductor.ProcessInbound(b.Params().Context, messages)
}
```

### 3. CCC Core Implementation

#### Constructor
**File**: `internal/core/execute/v2/crosschain/conductor.go:103-125`
```go
func NewCrossChainConductor(dispatcher execute.Dispatcher, logger logging.OptionalLogger) *CrossChainConductor {
    cc := &CrossChainConductor{
        dispatcher:      dispatcher,
        logger:         logger,
        syntheticChan:  make(chan *SyntheticRequest, 1000),
        retryChan:      make(chan *PendingTransmission, 100),
        stopChan:       make(chan struct{}),
        destinations:   make(map[DestinationKey]*DestinationQueue),
        pendingTx:      make(map[string]*PendingTransmission),
        // ...
    }
}
```

#### SubmitAnchor Method (Currently Unused)
**File**: `internal/core/execute/v2/crosschain/conductor.go:865-904`
```go
// SubmitAnchor submits an anchor for transmission
func (cc *CrossChainConductor) SubmitAnchor(req *AnchorRequest) error {
    // Line 867: Create destination key
    // Line 870: Get or create queue
    // Line 873-877: Wrap in synthetic request
    // Line 880-901: Queue or send based on state
}
```

## Integration Requirements

### Required Changes

1. **Add CCC reference to Original Conductor**
2. **Create anchor request converter**
3. **Implement event-to-method bridge**
4. **Preserve anchor healing**
5. **Add configuration flags**
6. **Implement metrics comparison**

## Phase 1: Minimal Integration

### Step 1.1: Add CCC Reference to Original Conductor

**File to modify**: `internal/core/crosschain/conductor.go`

```go
// Add after line 37
type Conductor struct {
    // ... existing fields ...
    
    // NEW: Reference to CrossChainConductor for anchor routing
    CrossChainConductor *v2crosschain.CrossChainConductor
}
```

### Step 1.2: Modify Anchor Submission

**File to modify**: `internal/core/crosschain/conductor.go:201`

Replace:
```go
return c.submit(ctx, destination, env)
```

With:
```go
// NEW: Route through CCC if available
if c.CrossChainConductor != nil {
    // Convert to AnchorRequest
    anchorMsg := &messaging.BlockAnchor{
        Anchor: anchor,
    }
    
    req := &v2crosschain.AnchorRequest{
        Anchor:      anchorMsg,
        Destination: destination,
        SequenceNum: sequenceNumber,
    }
    
    // Submit through CCC for retry and queue management
    err := c.CrossChainConductor.SubmitAnchor(req)
    if err != nil {
        // Fallback to direct submission
        slog.Warn("CCC submission failed, using direct path", "error", err)
        return c.submit(ctx, destination, env)
    }
    return nil
}

// Original path
return c.submit(ctx, destination, env)
```

### Step 1.3: Wire CCC to Original Conductor

**File to modify**: `internal/node/daemon/run.go`

After line 407 where CCC is enabled, add:
```go
// Around line 420, after creating the conductor
conductor := &crosschain.Conductor{
    Partition:    partition,
    Globals:      globals,
    ValidatorKey: validatorKey,
    Database:     database,
    Querier:      client,
    Dispatcher:   dispatcher,
    
    // NEW: Connect CCC if enabled
    CrossChainConductor: execOpts.EnableCrosschainCoordinator ? executor.CrossChainConductor : nil,
}
```

### Step 1.4: Update CCC SubmitAnchor for Signed Envelopes

**File to modify**: `internal/core/execute/v2/crosschain/conductor.go:866`

Enhance the SubmitAnchor method:
```go
func (cc *CrossChainConductor) SubmitAnchor(req *AnchorRequest) error {
    // NEW: Check if anchor is already wrapped in envelope
    var env *messaging.Envelope
    
    switch msg := req.Anchor.(type) {
    case *messaging.Envelope:
        env = msg
    default:
        // Create envelope for unwrapped anchor
        env = &messaging.Envelope{
            Messages: []messaging.Message{req.Anchor},
        }
    }
    
    // Continue with existing logic...
    destKey := cc.createDestinationKey(MessageTypeAnchor, req.Destination)
    // ...
}
```

## Phase 2: Adapter Pattern

### Step 2.1: Create Conductor Adapter

**New file**: `internal/core/crosschain/adapter.go`

```go
package crosschain

import (
    "context"
    "log/slog"
    
    "gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
    v2crosschain "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/crosschain"
    "gitlab.com/accumulatenetwork/accumulate/internal/core/events"
    "gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
    "gitlab.com/accumulatenetwork/accumulate/pkg/url"
    "gitlab.com/accumulatenetwork/accumulate/protocol"
)

// ConductorAdapter bridges the Original Conductor and CCC
type ConductorAdapter struct {
    Original *Conductor
    CCC      *v2crosschain.CrossChainConductor
    
    // Metrics for comparison
    directSubmissions int64
    cccSubmissions    int64
    failovers        int64
}

// Start initializes the adapter
func (a *ConductorAdapter) Start(bus *events.Bus) error {
    // Subscribe to events
    events.SubscribeSync(bus, a.willBeginBlock)
    events.SubscribeSync(bus, a.willChangeGlobals)
    return nil
}

// willBeginBlock handles block events
func (a *ConductorAdapter) willBeginBlock(e execute.WillBeginBlock) error {
    // Let original conductor handle healing
    if err := a.Original.willBeginBlock(e); err != nil {
        return err
    }
    
    // Intercept anchor submissions
    a.Original.Intercept = a.interceptAnchor
    
    return nil
}

// interceptAnchor routes anchors through CCC
func (a *ConductorAdapter) interceptAnchor(ctx context.Context, env *messaging.Envelope) (send bool, err error) {
    // Extract destination from envelope
    // Route through CCC
    // Return false to prevent direct submission
    
    for _, msg := range env.Messages {
        if anchor, ok := msg.(*messaging.BlockAnchor); ok {
            req := &v2crosschain.AnchorRequest{
                Anchor:      env, // Send whole envelope with signatures
                Destination: extractDestination(anchor),
                SequenceNum: extractSequence(anchor),
            }
            
            if err := a.CCC.SubmitAnchor(req); err != nil {
                slog.Warn("CCC submission failed", "error", err)
                a.failovers++
                return true, nil // Allow direct submission
            }
            
            a.cccSubmissions++
            return false, nil // Prevent direct submission
        }
    }
    
    // Not an anchor, allow normal processing
    return true, nil
}
```

### Step 2.2: Replace Original Conductor with Adapter

**File to modify**: `internal/node/daemon/run.go`

Replace conductor initialization with:
```go
// Create original conductor (without starting it)
originalConductor := &crosschain.Conductor{
    Partition:    partition,
    Globals:      globals,
    ValidatorKey: validatorKey,
    Database:     database,
    Querier:      client,
    Dispatcher:   dispatcher,
}

// Create adapter if CCC is enabled
var conductor interface{ Start(*events.Bus) error }
if execOpts.EnableCrosschainCoordinator {
    conductor = &crosschain.ConductorAdapter{
        Original: originalConductor,
        CCC:      executor.CrossChainConductor,
    }
} else {
    conductor = originalConductor
}

// Start the conductor/adapter
if err := conductor.Start(eventBus); err != nil {
    return nil, err
}
```

## Phase 3: Full Unification

### Step 3.1: Merge Conductor Capabilities into CCC

**File to modify**: `internal/core/execute/v2/crosschain/conductor.go`

Add new fields to CrossChainConductor struct:
```go
type CrossChainConductor struct {
    // ... existing fields ...
    
    // NEW: From original conductor
    Partition    *protocol.PartitionInfo
    Globals      atomic.Pointer[network.GlobalValues]
    ValidatorKey ed25519.PrivateKey  // For signing anchors
    Database     database.Beginner   // For healing queries
    Querier      api.Querier2        // For anchor verification
    
    // NEW: Event handling
    eventBus     *events.Bus
    healingEnabled bool
}
```

### Step 3.2: Add Event Subscription to CCC

**File to modify**: `internal/core/execute/v2/crosschain/conductor.go`

Add new methods:
```go
// Start subscribes to events (like original conductor)
func (cc *CrossChainConductor) Start(bus *events.Bus) error {
    cc.eventBus = bus
    events.SubscribeSync(bus, cc.willBeginBlock)
    events.SubscribeSync(bus, cc.willChangeGlobals)
    
    // Start existing workers
    cc.startWorkers()
    
    return nil
}

// willBeginBlock handles block events for anchors
func (cc *CrossChainConductor) willBeginBlock(e execute.WillBeginBlock) error {
    // Check if ready (from original conductor logic)
    if !cc.Globals.Load().ExecutorVersion.V2Enabled() {
        return nil
    }
    
    // Trigger anchor healing if enabled
    if cc.healingEnabled {
        cc.startHealingTask(e)
    }
    
    // Send anchors for current block
    return cc.sendBlockAnchors(e)
}

// sendBlockAnchors constructs and sends anchors
func (cc *CrossChainConductor) sendBlockAnchors(e execute.WillBeginBlock) error {
    // Port logic from original conductor.sendAnchors
    // Use cc.SubmitAnchor for actual submission
    // Benefit from CCC's retry and queue management
}
```

### Step 3.3: Move Anchor Construction to CCC

**File to modify**: `internal/core/execute/v2/crosschain/anchoring.go` (new file)

```go
package crosschain

import (
    "context"
    "crypto/ed25519"
    
    "gitlab.com/accumulatenetwork/accumulate/internal/core/crosschain"
    "gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
    "gitlab.com/accumulatenetwork/accumulate/pkg/url"
    "gitlab.com/accumulatenetwork/accumulate/protocol"
)

// PrepareAnchorSubmission creates a signed anchor envelope
func (cc *CrossChainConductor) PrepareAnchorSubmission(
    ctx context.Context,
    anchor protocol.AnchorBody,
    sequenceNumber uint64,
    destination *url.URL,
) (*messaging.Envelope, error) {
    // Use ValidatorContext from original implementation
    valCtx := crosschain.ValidatorContext{
        Source:       cc.Partition,
        Globals:      cc.Globals.Load(),
        ValidatorKey: cc.ValidatorKey,
    }
    
    env, _, err := valCtx.PrepareAnchorSubmission(ctx, anchor, sequenceNumber, destination)
    return env, err
}
```

## Testing Strategy

### Unit Tests

**File**: `internal/core/execute/v2/crosschain/conductor_integration_test.go`

```go
func TestCCCAnchorSubmission(t *testing.T) {
    // Test anchor submission through CCC
    // Verify queuing behavior
    // Test retry on failure
}

func TestCCCEventIntegration(t *testing.T) {
    // Test event subscription
    // Verify willBeginBlock handling
    // Test anchor construction
}

func TestCCCWithOriginalConductor(t *testing.T) {
    // Test adapter pattern
    // Verify fallback behavior
    // Compare metrics
}
```

### Integration Tests

**File**: `test/e2e/conductor_integration_test.go`

```go
func TestIntegratedConductor(t *testing.T) {
    // Create network with integrated conductor
    // Submit transactions requiring anchors
    // Verify anchors are sent through CCC
    // Test partition failure scenarios
}
```

### Load Tests

**File**: `test/load/conductor_integration_load.go`

```go
func TestConductorIntegrationLoad(t *testing.T) {
    // High volume anchor submission
    // Measure latency with/without CCC
    // Test queue behavior under load
    // Verify no anchor loss
}
```

## Rollback Plan

### Feature Flags

**File to modify**: `internal/core/execute/execute.go:75`

Add new configuration:
```go
type Options struct {
    // ... existing fields ...
    
    EnableCrosschainCoordinator bool  // Existing
    CCCHandlesAnchors          bool  // NEW: Route anchors through CCC
    CCCUnifiedMode             bool  // NEW: Full unification mode
}
```

### Conditional Routing

**File**: `internal/core/crosschain/conductor.go`

```go
func (c *Conductor) sendBlockAnchor(...) error {
    // Check feature flag
    if c.Globals.Load().CCCHandlesAnchors && c.CrossChainConductor != nil {
        // New path through CCC
        return c.submitAnchorViaCCC(...)
    }
    
    // Original path
    return c.submit(ctx, destination, env)
}
```

### Monitoring Points

Add metrics at key integration points:

1. **Anchor submission path** (CCC vs direct)
2. **Retry counts** for anchors
3. **Queue depths** for anchor destinations
4. **Healing trigger frequency**
5. **Submission latency** comparison

**File**: `internal/core/execute/v2/crosschain/metrics.go`

```go
type IntegrationMetrics struct {
    AnchorsCCCRouted     int64
    AnchorsDirectRouted  int64
    AnchorRetries        int64
    AnchorFailovers      int64
    HealingTriggered     int64
}
```

## Configuration

### Example Configuration

**File**: `config/conductor_integration.yaml`

```yaml
accumulate:
  execute:
    enable_crosschain_coordinator: true
    ccc_handles_anchors: true      # Phase 1
    ccc_unified_mode: false         # Phase 3
    
  conductor:
    healing_enabled: true
    retry_policy:
      max_attempts: 3
      initial_delay: 1s
      max_delay: 30s
```

## Implementation Timeline

| Phase | Duration | Risk | Rollback Time |
|-------|----------|------|---------------|
| Phase 1 | 2-3 days | Low | < 1 hour |
| Phase 2 | 1-2 weeks | Medium | < 4 hours |
| Phase 3 | 2-4 weeks | High | < 1 day |

## Success Criteria

### Phase 1
- [ ] Anchors route through CCC when enabled
- [ ] No anchor loss or duplication
- [ ] Metrics show routing distribution
- [ ] Fallback works on CCC failure

### Phase 2
- [ ] Adapter successfully bridges systems
- [ ] Healing continues to work
- [ ] Performance metrics acceptable
- [ ] Clean separation of concerns

### Phase 3
- [ ] Single unified conductor running
- [ ] All tests passing
- [ ] Performance improved or equal
- [ ] Clean shutdown and resource cleanup

## Conclusion

This implementation guide provides a concrete path to integrate the two conductor systems. The phased approach minimizes risk while providing immediate benefits. Each phase builds on the previous one, with clear rollback points and success criteria.

The key insight is that the integration is not just about merging code, but about carefully bridging two different architectural paradigms while maintaining the critical functionality of cross-partition anchor and synthetic transaction delivery.