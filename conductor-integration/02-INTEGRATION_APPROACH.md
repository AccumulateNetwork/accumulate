# Conductor Integration Approach

## Recommended Strategy: Delegation Pattern

### Core Principle
Keep both conductors but make them work together through clear delegation.

## Architecture

```
                    ┌─────────────────────┐
                    │   Block Event       │
                    └──────────┬──────────┘
                               ↓
                    ┌─────────────────────┐
                    │  Original Conductor │ (Coordinator)
                    │                     │
                    │  • Receives events  │
                    │  • Delegates work   │
                    └──────────┬──────────┘
                               ↓
                    ┌─────────────────────┐
                    │     Delegation      │
                    │                     │
                    │  Anchors → Original │
                    │  Synthetics → CCC   │
                    └─────────────────────┘
```

## Implementation Plan

### Phase 1: Add CCC Reference (1 day)
```go
// internal/core/crosschain/conductor.go
type Conductor struct {
    // ... existing fields ...
    
    // Add reference to CCC
    ccc *v2.CrossChainConductor // NEW
}
```

### Phase 2: Delegate Synthetics (2 days)
```go
func (c *Conductor) willBeginBlock(e execute.WillBeginBlock) error {
    // ... existing anchor code ...
    
    // Replace TODO with delegation
    if c.ccc != nil {
        // Delegate synthetic transactions to CCC
        err = c.ccc.ProcessPendingSynthetics(e.Context, batch)
        if err != nil {
            return errors.UnknownError.WithFormat("process synthetics: %w", err)
        }
    }
    
    return nil
}
```

### Phase 3: Optional Anchor Delegation (1 week)
```go
func (c *Conductor) sendBlockAnchor(ctx context.Context, anchor protocol.AnchorBody, seq uint64, dest string) error {
    // Try CCC first for async benefits
    if c.ccc != nil && c.ccc.IsHealthy() {
        err := c.ccc.SubmitAnchor(ctx, anchor, seq, dest)
        if err == nil {
            return nil // Success
        }
        // Log but continue to fallback
        c.log.Warn("CCC anchor submission failed, using original", "error", err)
    }
    
    // Fallback to original implementation
    return c.sendBlockAnchorOriginal(ctx, anchor, seq, dest)
}
```

## Why This Works

### 1. Minimal Changes
- Original conductor stays mostly unchanged
- CCC remains independent
- Just add delegation points

### 2. Clear Responsibilities
- Original: Coordination and anchors
- CCC: Synthetic transactions and queuing
- No overlap or confusion

### 3. Gradual Migration
- Start with synthetics only
- Add anchor delegation later
- Can revert easily if issues

### 4. Maintains Compatibility
- Original conductor still receives events
- Existing anchor logic preserved
- No breaking changes

## Configuration

```yaml
conductor:
  enable_ccc_delegation: true
  
  ccc_features:
    handle_synthetics: true
    handle_anchors: false  # Start false, enable later
    
  fallback:
    always_try_original: true
    log_delegation_errors: true
```

## Success Metrics

### Phase 1 (CCC Reference)
- Compiles successfully
- No runtime errors
- Original functionality unchanged

### Phase 2 (Synthetic Delegation)
- Synthetic transactions processed
- No message loss
- Metrics show CCC handling synthetics

### Phase 3 (Optional Anchor Delegation)
- Anchors sent via CCC
- Fallback works on CCC errors
- Performance improves

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| CCC fails to process | Fallback to original |
| Memory leaks in queues | Monitor and set limits |
| Deadlock between systems | Async delegation, timeouts |
| Performance regression | Measure before/after, feature flags |

## Next Steps
1. Implement Phase 1 (1 day)
2. Test Phase 1 (1 day)
3. Implement Phase 2 (2 days)
4. Test Phase 2 thoroughly (1 week)
5. Consider Phase 3 after stability proven

See `03-IMPLEMENTATION_DETAILS.md` for code changes.