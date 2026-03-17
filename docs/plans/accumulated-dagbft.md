# Implementation Plan: accumulated-dagbft Binary

## Goal
Create a standalone `accumulated-dagbft` binary that runs full Accumulate nodes with DAG-BFT consensus, without linking CometBFT.

## Current State Analysis

### CometBFT Dependencies in Accumulate Core

| Package | Dependency | Usage | Files |
|---------|------------|-------|-------|
| `internal/core/execute` | `abcitypes.CommitInfo` | BlockParams.CommitInfo | 1 |
| `internal/core/execute` | `abcitypes.Misbehavior` | BlockParams.Evidence | 1 |
| `internal/*` | `cometbft/libs/log` | Logger interface | 51 |
| `internal/node/abci` | Full CometBFT ABCI | ABCI application | N/A (excluded) |

### What Already Exists
- `internal/node/dagbft/` - Full DAG-BFT service integration
- `pkg/consensus/` - DAG-BFT consensus layer (CometBFT-free)
- `pkg/consensus/adapter/` - ExecutorBridge connecting consensus to executor
- `cmd/accumulated/run/dagbft.go` - DAGBFTService (but run package imports CometBFT)

### The Problem
Even though `DAGBFTService` doesn't use CometBFT, importing the `run` package pulls in CometBFT through:
1. `consensus.go` - CometBFT ConsensusService
2. `types.go` - ConsensusApp interface uses CometBFT types
3. Various packages use `cometbft/libs/log` for logging

---

## Implementation Phases

### Phase 1: Abstract the Logger Interface
**Effort: Medium | Risk: Low | Status: IN PROGRESS**

The `cometbft/libs/log` interface is used in 51 files. Replace with standard `log/slog`.

#### Issues:
1. **Create logging abstraction** - Define `internal/logging/logger.go` interface compatible with both slog and cometbft/libs/log
2. **Migrate internal/database** - Replace cometbft logger with slog
3. **Migrate internal/core/execute** - Replace cometbft logger with slog
4. **Migrate remaining internal packages** - Systematic replacement

#### Progress (2026-03-17):
**Completed:**
- Created `internal/logging/logger.go` with CometBFT-free `Logger` interface
- Created `internal/logging/compat.go` with `FromCometBFT()` and `CometBFTLogger()` conversion functions
- Updated `internal/logging/null.go`, `optional.go`, `recover.go`
- Migrated `pkg/database/indexing`, `pkg/database/merkle`, `pkg/database/bpt`
- Migrated `internal/database` (all core files)
- Migrated `internal/database/snapshot`
- Migrated `internal/core/events`, `internal/core/execute`
- Migrated `internal/api/routing`, `internal/api/v3/*`
- Migrated `internal/node/http`, `internal/node/genesis`, `internal/node/abci` (partial)
- Migrated `internal/bsn`

**Remaining:**
- `internal/node/daemon/run.go` - Needs `FromCometBFT()` wrappers at CometBFT boundary
- `test/simulator/consensus/mempool.go` - Similar boundary conversion needed

#### Acceptance Criteria:
- No `cometbft/libs/log` imports outside `internal/node/abci` and `cmd/accumulated/run/consensus.go`
- All tests pass
- Logging behavior unchanged

---

### Phase 2: Abstract BlockParams from ABCI Types
**Effort: Small | Risk: Low**

`BlockParams` in `internal/core/execute/execute.go` uses CometBFT types for `CommitInfo` and `Evidence`.

#### Issues:
1. **Create consensus-agnostic block params** - Replace ABCI-specific fields with interfaces or make them optional
   ```go
   type BlockParams struct {
       Context    context.Context
       IsLeader   bool
       Index      uint64
       Time       time.Time
       // Make these interface{} or create abstract types
       CommitInfo interface{}  // nil for DAG-BFT
       Evidence   interface{}  // nil for DAG-BFT
   }
   ```

#### Acceptance Criteria:
- `internal/core/execute` compiles without `cometbft/abci/types` import
- DAG-BFT can pass nil for CommitInfo/Evidence
- CometBFT path still works with type assertions

---

### Phase 3: Create DAG-BFT-Only Run Package
**Effort: Medium | Risk: Medium**

Create a parallel run infrastructure that doesn't import CometBFT.

#### Option A: Build Tags (Recommended)
Add build tags to split CometBFT and DAG-BFT code paths:
- `consensus.go` → `//go:build !dagbft`
- `types.go` → Split into `types_common.go` and `types_comet.go`
- `dagbft.go` → `//go:build dagbft || !comet`

#### Option B: Separate Package
Create `cmd/accumulated/run-dagbft/` with only DAG-BFT services.

#### Issues:
1. **Split types.go** - Move ConsensusApp interface to comet-only file
2. **Add build tags to consensus.go** - Exclude from dagbft builds
3. **Create dagbft-only configuration** - DAGBFTValidatorConfiguration parallel to CoreValidatorConfiguration
4. **Update schema generation** - Handle build tag variants

---

### Phase 4: Create accumulated-dagbft Binary
**Effort: Small | Risk: Low**

#### Issues:
1. **Create cmd/accumulated-dagbft/main.go** - Entry point using DAG-BFT run package
2. **Create init command** - Initialize DAG-BFT node configuration
3. **Create run command** - Start DAG-BFT node
4. **Add to build system** - Makefile targets, CI/CD

#### Acceptance Criteria:
- `go build ./cmd/accumulated-dagbft` produces binary without CometBFT
- `go list -m` shows no cometbft dependency for dagbft binary
- Can run devnet with DAG-BFT consensus

---

### Phase 5: Integration Testing
**Effort: Medium | Risk: Medium**

#### Issues:
1. **Create DAG-BFT devnet configuration** - Multi-node test network
2. **Verify database-backed replay protection** - No memory leaks
3. **State hash consensus verification** - All nodes agree on state
4. **Performance benchmarking** - Compare to CometBFT baseline
5. **Failover testing** - Node crashes, network partitions

---

## Issue Breakdown

### Immediate (Phase 1-2)
| # | Title | Phase | Effort |
|---|-------|-------|--------|
| 1 | Replace cometbft/libs/log with slog in internal/database | 1 | S |
| 2 | Replace cometbft/libs/log with slog in internal/core/execute | 1 | S |
| 3 | Replace cometbft/libs/log in remaining internal packages | 1 | M |
| 4 | Abstract BlockParams.CommitInfo and Evidence from ABCI types | 2 | S |

### Core Implementation (Phase 3-4)
| # | Title | Phase | Effort |
|---|-------|-------|--------|
| 5 | Add build tags to separate CometBFT code in run package | 3 | M |
| 6 | Create DAGBFTValidatorConfiguration | 3 | M |
| 7 | Create cmd/accumulated-dagbft binary | 4 | S |
| 8 | Add accumulated-dagbft to build system | 4 | S |

### Validation (Phase 5)
| # | Title | Phase | Effort |
|---|-------|-------|--------|
| 9 | Create DAG-BFT devnet configuration | 5 | M |
| 10 | Integration tests for DAG-BFT consensus | 5 | L |
| 11 | Performance benchmarks vs CometBFT | 5 | M |

---

## Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Logger migration breaks logging behavior | Low | Medium | Comprehensive test coverage |
| Build tags increase maintenance burden | Medium | Low | Clear documentation, CI for both builds |
| DAG-BFT executor adapter has subtle bugs | Medium | High | Extensive integration testing |
| Performance regression vs CometBFT | Low | Medium | Benchmark before/after |

---

## Success Metrics

1. **Binary size**: `accumulated-dagbft` should be smaller than `accumulated`
2. **No CometBFT linkage**: `go list -m all` shows no cometbft for dagbft build
3. **Test parity**: All existing tests pass with both consensus backends
4. **Memory stability**: No unbounded growth under sustained load
5. **State consensus**: All nodes agree on state hash

---

## Estimated Total Effort

| Phase | Effort | Dependencies |
|-------|--------|--------------|
| Phase 1 | 2-3 days | None |
| Phase 2 | 0.5 day | None |
| Phase 3 | 2-3 days | Phase 1, 2 |
| Phase 4 | 1 day | Phase 3 |
| Phase 5 | 3-5 days | Phase 4 |

**Total: ~9-13 days of focused work**

---

## Quick Win Alternative

If full decoupling is too much work initially, accept CometBFT linkage but:
1. Add `--consensus=dagbft` flag to existing `accumulated`
2. Runtime switch between ConsensusService and DAGBFTService
3. Binary still links CometBFT but doesn't use it when running DAG-BFT

This gets DAG-BFT running in production faster, with full decoupling as follow-up work.
