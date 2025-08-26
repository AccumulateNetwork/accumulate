# Conductor Systems: Current State Analysis

## Executive Summary
Two conductor systems exist with fundamentally different architectures that need to work together.

## Original Conductor
**Location**: `internal/core/crosschain/conductor.go`
**Created**: November 2023
**Architecture**: Event-driven, synchronous

### What It Does:
- ✅ Sends anchors at block boundaries
- ✅ Heals missing anchors (re-sends)
- ❌ NO synthetic transaction support (has TODO comment)
- ❌ NO inbound message processing

### How It Works:
```go
Block Event → willBeginBlock() → Send Anchors → Done
```

### Key Characteristics:
- Stateless operation
- Runs only at block boundaries
- Simple, proven, reliable
- Limited scope

## CrossChainConductor (CCC)
**Location**: `internal/core/execute/v2/crosschain/conductor.go`
**Created**: August 2025
**Architecture**: Channel-based, asynchronous

### What It Does:
- ✅ Processes synthetic transactions
- ✅ Per-destination queue management
- ✅ Retry logic with exponential backoff
- ✅ Filters inbound messages (ProcessInbound)
- ❌ Collection proofs broken (nil pointer bug)

### How It Works:
```go
Continuous Loop → Check Queues → Process Messages → Retry if Failed
```

### Key Characteristics:
- Stateful with persistent queues
- Runs continuously in background
- Complex retry and blocking logic
- Broader scope

## Architecture Mismatch

| Aspect | Original | CCC |
|--------|----------|-----|
| **Trigger** | Block events | Continuous |
| **State** | Stateless | Stateful queues |
| **Scope** | Anchors only | Synthetics + filtering |
| **Error Handling** | Return error | Async retry |
| **Complexity** | Simple | Complex |

## Current Problems

### 1. No Synthetic Support in Original
- Line 145: `// TODO Send synthetic transactions`
- Never implemented
- Blocks full cross-partition messaging

### 2. Collection Proofs Broken in CCC
- Line 303 in proof_service.go: `GetReceiptList(nil, ...)`
- Nil pointer guarantees failure
- Falls back to individual proofs
- No performance benefit

### 3. No Coordination
- Both systems run independently
- No shared state
- Potential for conflicts
- Duplicate efforts possible

## Integration Challenges

### Technical
1. **Event-driven vs Channel-based** - Fundamentally different models
2. **Stateless vs Stateful** - Different data management
3. **Synchronous vs Asynchronous** - Different error handling
4. **No common interface** - Can't easily swap or delegate

### Operational
1. **Which handles what?** - No clear responsibility boundaries
2. **How to migrate?** - Can't just replace one with other
3. **Backwards compatibility?** - Must maintain existing behavior
4. **Performance impact?** - Adding layers could slow things

## Next Steps
See `02-INTEGRATION_APPROACH.md` for the proposed solution.