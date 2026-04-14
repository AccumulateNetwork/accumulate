# Bug Report: DAG-BFT Integration Remaining Issues

## Overview
Three remaining bugs identified after logger and halt controller fixes. These prevent full integration validation.

---

## Bug 1: TestIntegration_ThreeNodes Timeout

**Issue ID**: DAG-BFT-001  
**Severity**: HIGH  
**Category**: Consensus Logic  
**Status**: OPEN

### Symptoms
- Test times out waiting for 3-node consensus to progress
- Only 1 header created across all rounds
- 0 certificates created
- Nodes receive headers via gossip but fail to aggregate into quorum certificates

### Root Cause
Vote aggregation or quorum calculation in DAG-BFT consensus layer. Likely in:
- Certificate formation logic
- Vote signature verification
- Quorum threshold calculation

### Affected Code
- `internal/node/dagbft/service.go` — consensus round progression
- `test/simulator/consensus/` — consensus simulator (test harness)
- `pkg/consensus/` — consensus adapter interface

### Evidence
```
Headers created: 1
Certificates created: 0
```

### Impact
- Prevents multi-node consensus validation
- Blocks production readiness testing
- 3-node deployments non-functional

### Test Location
`test/integration/dagbft_test.go::TestIntegration_ThreeNodes`

### Expected Fix Effort
High — requires deep consensus protocol debugging

---

## Bug 2: TestExecutorConsistency JSON Serialization

**Issue ID**: DAG-BFT-002  
**Severity**: MEDIUM  
**Category**: State Serialization  
**Status**: OPEN

### Symptoms
- JSON marshaling fails with error about missing `"$epilogue"` field
- Occurs during state consistency validation
- Test fails before completing block-level validation

### Root Cause
Unknown — likely mismatch between BlockState structure and expected JSON schema, or missing field in serialization.

### Affected Code
- `internal/core/execute/` — BlockState serialization
- Test: `test/integration/dagbft_test.go::TestExecutorConsistency`

### Impact
- State consistency validation unavailable
- Cannot verify executor correctness across blocks

### Test Location
`test/integration/dagbft_test.go::TestExecutorConsistency`

### Expected Fix Effort
Low to Medium — likely a missing field or schema mismatch

---

## Bug 3: tools/cmd/export-snapshot Build Failure

**Issue ID**: DAG-BFT-003  
**Severity**: MEDIUM  
**Category**: Build/Tooling  
**Status**: OPEN

### Symptoms
- `tools/cmd/export-snapshot` fails to build
- Likely logger-related (similar pattern to earlier fixes)

### Root Cause
Unknown — probably logger interface mismatch in snapshot export tool

### Affected Code
- `tools/cmd/export-snapshot/` — snapshot export command

### Impact
- Snapshot export functionality unavailable
- Snapshot creation/export workflows broken

### Expected Fix Effort
Low — probably similar logger fix pattern as earlier (5-10 minutes)

---

## Execution Plan

### Phase 1: Code Review
1. Review TestIntegration_ThreeNodes test harness and consensus simulator
2. Review JSON serialization in BlockState
3. Review export-snapshot command structure

### Phase 2: Bug Fixes (in priority order)
1. **Bug 3 (export-snapshot)** — Quick win, clears build issues
2. **Bug 2 (JSON serialization)** — Medium complexity, likely straightforward
3. **Bug 1 (3-node consensus)** — Highest complexity, requires protocol debugging

### Phase 3: Validation
- Build succeeds: `go build ./...`
- All three buggy tests pass
- Full test suite runs without regressions

---

## Test Commands

```bash
# Run individual buggy tests
go test -run TestIntegration_ThreeNodes ./test/integration
go test -run TestExecutorConsistency ./test/integration
go build ./tools/cmd/export-snapshot

# Full integration test suite
go test ./test/integration -v -timeout 10m

# Build all tools
go build ./...
```

---

## Files to Review
- `test/simulator/consensus/node.go` — consensus simulator
- `test/simulator/factory.go` — logger patterns
- `internal/core/execute/block_state.go` — BlockState serialization
- `tools/cmd/export-snapshot/main.go` — snapshot export
- `internal/node/dagbft/service.go` — consensus round progression

