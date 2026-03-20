# Research: Delete internal/node/abci package

## Summary

The `internal/node/abci` package implements the CometBFT ABCI (Application Blockchain Interface) application for Accumulate. It contains the `Accumulator` type which implements `abci.Application` and bridges the CometBFT consensus layer with Accumulate's execution layer. The package is imported by 11 files across cmd, internal/node/daemon, and test packages. To delete this package, all usages must first be updated or removed.

## Verified Facts

### Fact 1: Package Contents
- **Source**: `internal/node/abci/` directory listing
- **Content**: Package contains 7 files:
  - `abci.go` (22 lines) - Package documentation and Version constant
  - `accumulator.go` (846 lines) - Main `Accumulator` type implementing ABCI
  - `execute.go` (83 lines) - Transaction execution helpers including `AdjustStatusIDs` function
  - `snapshot.go` (388 lines) - Snapshot management for state sync
  - `_abci_test.go` - Disabled test file (prefixed with `_`)
  - `e2e_test.go` - End-to-end tests
  - `utils_test.go` - Test utilities
- **Confidence**: HIGH

### Fact 2: Main Exported Types and Functions
- **Source**: `internal/node/abci/abci.go:21`, `internal/node/abci/accumulator.go:49-88`, `internal/node/abci/execute.go:65`, `internal/node/abci/snapshot.go:211`
- **Content**:
  - `const Version uint64 = 0x2` - ABCI application version
  - `type Accumulator struct` - Main ABCI application type
  - `type AccumulatorOptions struct` - Configuration for Accumulator
  - `func NewAccumulator(opts AccumulatorOptions) *Accumulator` - Constructor
  - `func AdjustStatusIDs(messages []messaging.Message, st []*protocol.TransactionStatus)` - ID adjustment helper
  - `func ListSnapshots(dir string) ([]*snapshotInfo, error)` - Snapshot listing utility
- **Confidence**: HIGH

### Fact 3: Files Importing the Package
- **Source**: Grep for `gitlab.com/accumulatenetwork/accumulate/internal/node/abci`
- **Content**: 11 files import this package:
  1. `cmd/accumulated/cmd_reset.go:22` - Uses `abci.Version`
  2. `cmd/accumulated/run/snapshot.go:28` - Uses `abci.ListSnapshots`
  3. `cmd/accumulated/run/consensus.go:51` - Uses `abci.NewAccumulator`, `abci.AccumulatorOptions`
  4. `test/simulator/factory.go:30` - Uses `abci.NewAccumulator`, `abci.AccumulatorOptions`
  5. `test/simulator/consensus/abci.go:15` - Defines `AbciApp` alias to `abci.Accumulator`
  6. `test/e2e/msg_block_anchor_test.go:15` - Uses `abci.AdjustStatusIDs`
  7. `test/e2e/_relaunch_test.go:20` - Uses type assertion to `*abci.Accumulator`
  8. `internal/node/daemon/run.go:54` - Uses `abci.NewAccumulator`, `abci.AccumulatorOptions`
  9. `internal/node/daemon/summary.go:21` - Uses `abci.NewAccumulator`, `abci.AccumulatorOptions`
  10. `internal/node/daemon/snapshots.go:26` - Uses `abci.ListSnapshots`
  11. `test/testing/node.go:26` - Type assertion to `*abci.Accumulator`
- **Confidence**: HIGH

### Fact 4: Usage of `abci.Version`
- **Source**: `cmd/accumulated/cmd_reset.go:101`
- **Content**: `genDoc.ConsensusParams.Version.App = abci.Version`
- **Action Required**: Move `Version` constant elsewhere or inline the value
- **Confidence**: HIGH

### Fact 5: Usage of `abci.ListSnapshots`
- **Source**: `cmd/accumulated/run/snapshot.go:266,321`, `internal/node/daemon/snapshots.go:257`
- **Content**: Used to check if it's time to take a snapshot based on existing snapshots
- **Action Required**: Move `ListSnapshots` function to another package or replace usage
- **Confidence**: HIGH

### Fact 6: Usage of `abci.NewAccumulator` and `abci.AccumulatorOptions`
- **Source**:
  - `cmd/accumulated/run/consensus.go:517-530`
  - `internal/node/daemon/run.go:462-477`
  - `internal/node/daemon/summary.go:149-163`
  - `test/simulator/factory.go:484-493`
- **Content**: Creates new `Accumulator` instances for:
  - CoreConsensusApp start
  - Daemon validator start
  - Summary node start
  - Simulator with ABCI mode
- **Action Required**: Replace with dagbft equivalent or remove entirely
- **Confidence**: HIGH

### Fact 7: Usage of `abci.AdjustStatusIDs`
- **Source**: `test/e2e/msg_block_anchor_test.go:187`
- **Content**: `abci.AdjustStatusIDs([]messaging.Message{captured[1]}, st)` - corrects transaction IDs for block anchors
- **Action Required**: Move function or update test
- **Confidence**: HIGH

### Fact 8: Type Assertion to `*abci.Accumulator`
- **Source**:
  - `test/testing/node.go:233`
  - `test/e2e/_relaunch_test.go:39,95`
  - `test/simulator/consensus/abci.go:20,23,44,74,120,144`
- **Content**: Tests access `OnFatal` method and call ABCI methods directly
- **Action Required**: Update tests to use new consensus mechanism
- **Confidence**: HIGH

### Fact 9: AbciApp Alias in Simulator
- **Source**: `test/simulator/consensus/abci.go:20`
- **Content**: `type AbciApp abci.Accumulator` - Wraps Accumulator for simulator
- **Action Required**: Remove or replace with dagbft equivalent
- **Confidence**: HIGH

### Fact 10: internal/node/node.go Reference
- **Source**: `internal/node/node.go:21,27`
- **Content**:
  - `type AppFactory func(*privval.FilePV) (abci.Application, error)` - Uses CometBFT abci types
  - `ABCI   abci.Application` - Field in Node struct
- **Action Required**: This references `github.com/cometbft/cometbft/abci/types`, not our package, but Node struct stores ABCI app
- **Confidence**: HIGH

## Code References

### Primary Implementation Files
- `internal/node/abci/abci.go` - Package doc and Version constant
- `internal/node/abci/accumulator.go` - Main Accumulator implementation
- `internal/node/abci/execute.go` - Transaction execution helpers
- `internal/node/abci/snapshot.go` - Snapshot management

### Key Functions/Types to Relocate or Replace
1. `Version` constant (line 21 of abci.go)
2. `Accumulator` type (line 49 of accumulator.go)
3. `AccumulatorOptions` type (line 73 of accumulator.go)
4. `NewAccumulator` function (line 90 of accumulator.go)
5. `AdjustStatusIDs` function (line 65 of execute.go)
6. `ListSnapshots` function (line 211 of snapshot.go)

### Dependent Files (Must be Updated)
| File | Usage |
|------|-------|
| `cmd/accumulated/cmd_reset.go` | `abci.Version` |
| `cmd/accumulated/run/snapshot.go` | `abci.ListSnapshots` |
| `cmd/accumulated/run/consensus.go` | `abci.NewAccumulator`, `abci.AccumulatorOptions` |
| `test/simulator/factory.go` | `abci.NewAccumulator`, `abci.AccumulatorOptions` |
| `test/simulator/consensus/abci.go` | `type AbciApp abci.Accumulator`, method calls |
| `test/e2e/msg_block_anchor_test.go` | `abci.AdjustStatusIDs` |
| `test/e2e/_relaunch_test.go` | Type assertion `*abci.Accumulator` |
| `internal/node/daemon/run.go` | `abci.NewAccumulator`, `abci.AccumulatorOptions` |
| `internal/node/daemon/summary.go` | `abci.NewAccumulator`, `abci.AccumulatorOptions` |
| `internal/node/daemon/snapshots.go` | `abci.ListSnapshots` |
| `test/testing/node.go` | Type assertion `*abci.Accumulator` |

## Open Questions

1. **Where should `ListSnapshots` be relocated?** - Currently used for snapshot scheduling in both run package and daemon. Could potentially be moved to `internal/database/snapshot` or a new utility package.

2. **Where should `AdjustStatusIDs` be relocated?** - Used in tests for block anchor ID adjustment. Could potentially be moved to `pkg/types/messaging` or a test helper package.

3. **What replaces the Accumulator type?** - The dagbft integration presumably has an equivalent consensus application. The implementation plan should specify what takes its place.

4. **Should `Version` constant be preserved?** - Used in consensus params. May need to be defined in protocol or a consensus-related package.

5. **What happens to tests using type assertion to `*abci.Accumulator`?** - Tests in `test/testing/node.go` and `test/e2e/_relaunch_test.go` use `OnFatal` callback. Need equivalent mechanism in new consensus.

## Contradictions

None found. All sources consistently reference the same package structure and usage patterns.

## Recommended Deletion Order

1. **Phase 1 - Relocate utility functions**:
   - Move `ListSnapshots` to appropriate snapshot package
   - Move `AdjustStatusIDs` to messaging or test helper package
   - Move `Version` constant to protocol or consensus package

2. **Phase 2 - Update consumers**:
   - Update all import statements
   - Replace `abci.NewAccumulator` calls with dagbft equivalent
   - Update test type assertions

3. **Phase 3 - Delete package**:
   - Remove `internal/node/abci/` directory
   - Verify build succeeds
   - Run tests to verify functionality
