# Conductor Integration: Implementation Details

## File Changes Required

### 1. Update Original Conductor Structure
**File**: `internal/core/crosschain/conductor.go`

```go
import (
    // ... existing imports ...
    v2cc "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/crosschain"
)

type Conductor struct {
    // ... existing fields ...
    
    // Add CCC reference
    ccc *v2cc.CrossChainConductor
}

// Add setter method
func (c *Conductor) SetCCC(ccc *v2cc.CrossChainConductor) {
    c.ccc = ccc
}
```

### 2. Implement Synthetic Delegation
**File**: `internal/core/crosschain/conductor.go` (line ~145)

```go
func (c *Conductor) willBeginBlock(e execute.WillBeginBlock) error {
    // ... existing code through line 144 ...
    
    // Replace: // TODO Send synthetic transactions
    if err := c.processPendingSynthetics(e.Context, batch); err != nil {
        return errors.UnknownError.WithFormat("process synthetics: %w", err)
    }
    
    return nil
}

// New method for synthetic processing
func (c *Conductor) processPendingSynthetics(ctx context.Context, batch *database.Batch) error {
    // Check if CCC is available
    if c.ccc == nil {
        // No CCC, skip synthetics (maintain current behavior)
        return nil
    }
    
    // Query pending synthetic transactions
    synthetics, err := c.queryPendingSynthetics(batch)
    if err != nil {
        return errors.UnknownError.WithFormat("query synthetics: %w", err)
    }
    
    if len(synthetics) == 0 {
        return nil // Nothing to do
    }
    
    // Delegate to CCC
    for _, syn := range synthetics {
        err := c.ccc.SubmitSynthetic(ctx, syn.Messages, syn.Destination)
        if err != nil {
            // Log but don't fail block
            slog.Error("Failed to submit synthetic via CCC", 
                "destination", syn.Destination,
                "error", err)
        }
    }
    
    return nil
}
```

### 3. Wire CCC into Executor
**File**: `internal/core/execute/v2/executor.go`

```go
func (x *Executor) Initialize(...) error {
    // ... existing initialization ...
    
    // Initialize original conductor
    if x.conductor == nil {
        x.conductor = &crosschain.Conductor{
            Partition:    partition,
            Globals:      globals,
            ValidatorKey: validatorKey,
            Database:     database,
            Querier:      querier,
            Dispatcher:   dispatcher,
        }
    }
    
    // Initialize CCC
    if x.crosschainConductor == nil {
        x.crosschainConductor = v2cc.NewCrossChainConductor(
            partition,
            10000,      // queueSize
            5*time.Second,  // retryInterval
            3,          // maxRetries
        )
    }
    
    // Link them together
    x.conductor.SetCCC(x.crosschainConductor)
    
    // Start both
    if err := x.conductor.Start(bus); err != nil {
        return err
    }
    if err := x.crosschainConductor.Start(); err != nil {
        return err
    }
    
    return nil
}
```

### 4. Fix Collection Proofs
**File**: `internal/core/execute/v2/crosschain/proof_service.go`

```go
type ProofService struct {
    logger         logging.OptionalLogger
    metrics        *ProofMetrics
    merkleManager  database.MerkleManager // ADD THIS
    batchThreshold int
    maxBatchSize   int
    debugMode      bool
}

func NewProofService(logger logging.OptionalLogger, mm database.MerkleManager) *ProofService {
    return &ProofService{
        logger:         logger.With("module", "proof-service").(logging.OptionalLogger),
        metrics:        &ProofMetrics{},
        merkleManager:  mm,  // STORE IT
        batchThreshold: 2,
        maxBatchSize:   100,
    }
}

// Fix line 303
func (ps *ProofService) createCollectionProof(ctx context.Context, req ProofRequest) (*ProofResponse, error) {
    // ... existing code ...
    
    // Get the chain from merkle manager
    chain := ps.merkleManager.GetChain(req.ChainURL)
    if chain == nil {
        return nil, errors.NotFound.WithFormat("chain %s not found", req.ChainURL)
    }
    
    // NOW THIS WORKS:
    receiptList, err := merkle.GetReceiptList(chain, startIdx, endIdx)
    // ... rest of function ...
}
```

### 5. Add Health Check to CCC
**File**: `internal/core/execute/v2/crosschain/conductor.go`

```go
// Add health check method
func (cc *CrossChainConductor) IsHealthy() bool {
    cc.mu.RLock()
    defer cc.mu.RUnlock()
    
    // Check if queues are not overflowing
    for _, queue := range cc.destinationQueues {
        if len(queue.QueuedRequests) > cc.maxQueueSize*0.9 {
            return false // Queue almost full
        }
    }
    
    // Check if conductor is running
    select {
    case <-cc.stopChan:
        return false // Stopped
    default:
        return true // Running
    }
}

// Add method for CCC to handle anchors
func (cc *CrossChainConductor) SubmitAnchor(ctx context.Context, anchor protocol.AnchorBody, seq uint64, dest string) error {
    // Convert anchor to message format
    destURL := protocol.PartitionUrl(dest)
    
    txn := &protocol.Transaction{
        Header: protocol.TransactionHeader{
            Principal: destURL.JoinPath(protocol.AnchorPool),
        },
        Body: anchor,
    }
    
    msg := &messaging.SequencedMessage{
        Message:     &messaging.TransactionMessage{Transaction: txn},
        Source:      cc.partition.Url(),
        Destination: destURL,
        Number:      seq,
    }
    
    // Submit through synthetic channel (reuse existing logic)
    return cc.SubmitSynthetic(ctx, []messaging.Message{msg}, destURL)
}
```

## Testing Plan

### Unit Tests

```go
// conductor_integration_test.go
func TestConductorWithCCC(t *testing.T) {
    // Setup
    conductor := &Conductor{...}
    ccc := NewCrossChainConductor(...)
    conductor.SetCCC(ccc)
    
    // Test synthetic delegation
    err := conductor.processPendingSynthetics(ctx, batch)
    require.NoError(t, err)
    
    // Verify CCC received synthetics
    metrics := ccc.GetMetrics()
    require.Greater(t, metrics.SyntheticsProcessed, 0)
}

func TestConductorWithoutCCC(t *testing.T) {
    // Setup conductor without CCC
    conductor := &Conductor{...}
    // ccc is nil
    
    // Should not fail
    err := conductor.processPendingSynthetics(ctx, batch)
    require.NoError(t, err)
}
```

### Integration Tests

```go
func TestFullIntegration(t *testing.T) {
    // Setup full system
    executor := NewExecutor(...)
    executor.Initialize(...)
    
    // Submit transactions that create synthetics
    // ...
    
    // Trigger block event
    event := execute.WillBeginBlock{...}
    executor.conductor.willBeginBlock(event)
    
    // Verify synthetics processed by CCC
    time.Sleep(100*time.Millisecond) // Let async processing happen
    
    metrics := executor.crosschainConductor.GetMetrics()
    require.Greater(t, metrics.SyntheticsProcessed, 0)
}
```

## Rollback Plan

If issues arise, disable CCC delegation:

```go
// Quick disable via configuration
if config.DisableCCCDelegation {
    x.conductor.SetCCC(nil)
}
```

Or remove the delegation code:
```go
// Revert line 145 to original
// TODO Send synthetic transactions
```

## Monitoring

Add metrics to track delegation:
```go
type ConductorMetrics struct {
    SyntheticsDelegated   int64
    DelegationErrors      int64
    FallbacksUsed         int64
}
```

Log key events:
```
INFO: Delegating 5 synthetics to CCC
WARN: CCC delegation failed, using fallback
INFO: Successfully sent anchor via CCC
```

## Timeline

| Task | Duration | Owner |
|------|----------|-------|
| Add CCC reference | 2 hours | Core team |
| Implement delegation | 4 hours | Core team |
| Unit tests | 4 hours | Test team |
| Integration tests | 1 day | Test team |
| Deploy to testnet | 1 day | DevOps |
| Monitor | 1 week | All |
| Production deploy | - | After validation |