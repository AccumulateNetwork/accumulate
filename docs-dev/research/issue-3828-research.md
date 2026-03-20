# Research: Delete internal/api/v3/tm package

## Summary

The `internal/api/v3/tm` package contains CometBFT-specific API implementations (`ConsensusService`, `Submitter`, `Validator`) that depend on CometBFT RPC types. DAG-BFT equivalents exist in `internal/node/dagbft/api.go` implementing the same interfaces (`api.ConsensusService`, `api.Submitter`, `api.Validator`). The tm package is imported by 4 files that need migration before deletion.

## Verified Facts

### Fact 1: tm package contains 3 API service implementations
- **Source**: `internal/api/v3/tm/consensus.go:31-42`, `internal/api/v3/tm/submitter.go:27-32`, `internal/api/v3/tm/validator.go:24-29`
- **Content**:
  - `ConsensusService` implements `api.ConsensusService` (line 42: `var _ api.ConsensusService = (*ConsensusService)(nil)`)
  - `Submitter` implements `api.Submitter` (line 32: `var _ api.Submitter = (*Submitter)(nil)`)
  - `Validator` implements `api.Validator` (line 29: `var _ api.Validator = (*Validator)(nil)`)
- **Confidence**: HIGH

### Fact 2: tm package has CometBFT dependencies
- **Source**: `internal/api/v3/tm/consensus.go:15`, `internal/api/v3/tm/submitter.go:13-14`, `internal/api/v3/tm/validator.go:12-13`
- **Content**: All files import CometBFT types (`coretypes "github.com/cometbft/cometbft/rpc/core/types"`, `"github.com/cometbft/cometbft/types"`)
- **Confidence**: HIGH

### Fact 3: DAG-BFT equivalents exist with same interfaces
- **Source**: `internal/node/dagbft/api.go:38`, `internal/node/dagbft/api.go:135`, `internal/node/dagbft/api.go:198`
- **Content**:
  - `ConsensusAPIService` implements `api.ConsensusService` (line 38: `var _ api.ConsensusService = (*ConsensusAPIService)(nil)`)
  - `SubmitterService` implements `api.Submitter` (line 135: `var _ api.Submitter = (*SubmitterService)(nil)`)
  - `ValidatorService` implements `api.Validator` (line 198: `var _ api.Validator = (*ValidatorService)(nil)`)
- **Confidence**: HIGH

### Fact 4: 4 files import the tm package
- **Source**: grep search for `internal/api/v3/tm`
- **Content**:
  1. `cmd/accumulated/run/consensus.go:45` - uses `tmapi "gitlab.com/accumulatenetwork/accumulate/internal/api/v3/tm"`
  2. `internal/node/daemon/run.go:46` - uses `"gitlab.com/accumulatenetwork/accumulate/internal/api/v3/tm"`
  3. `internal/node/daemon/summary.go:17` - uses `"gitlab.com/accumulatenetwork/accumulate/internal/api/v3/tm"`
  4. `exp/tendermint/dispatcher.go:22` - uses `"gitlab.com/accumulatenetwork/accumulate/internal/api/v3/tm"`
- **Confidence**: HIGH

### Fact 5: cmd/accumulated/run/consensus.go uses tm package for CometBFT consensus
- **Source**: `cmd/accumulated/run/consensus.go:547-580`
- **Content**: Creates `tmapi.NewConsensusService`, `tmapi.NewSubmitter`, `tmapi.NewValidator` in `CoreConsensusApp.register()` method for CometBFT-based consensus
- **Confidence**: HIGH

### Fact 6: internal/node/daemon/run.go uses tm package for validator services
- **Source**: `internal/node/daemon/run.go:556-590`
- **Content**: In `startServices()`, creates `tm.NewConsensusService`, `tm.NewSubmitter`, `tm.NewValidator` for the Daemon's CometBFT-based validator mode
- **Confidence**: HIGH

### Fact 7: internal/node/daemon/summary.go uses tm package for summary services
- **Source**: `internal/node/daemon/summary.go:170-186`
- **Content**: In `startSummaryServices()`, creates `tm.NewConsensusService`, `tm.NewSubmitter`, `tm.NewValidator` for block summary partition
- **Confidence**: HIGH

### Fact 8: exp/tendermint/dispatcher.go uses tm.Submitter for direct dispatch
- **Source**: `exp/tendermint/dispatcher.go:31-33`, `exp/tendermint/dispatcher.go:195-197`
- **Content**:
  - Defines `DispatcherClient` interface that embeds `tm.SubmitClient` (line 31-33)
  - Creates `tm.NewSubmitter` to submit envelopes to other partitions (line 195-197)
- **Confidence**: HIGH

### Fact 9: tm.SubmitClient interface is used by dispatcher
- **Source**: `internal/api/v3/tm/submitter.go:22-25`
- **Content**: `SubmitClient` interface requires `BroadcastTxAsync` and `BroadcastTxSync` methods from CometBFT RPC
- **Confidence**: HIGH

### Fact 10: Test file exists for tm package
- **Source**: `internal/api/v3/tm/consensus_test.go`
- **Content**: Test file with `TestConsensusStatus` that tests the consensus service with mock CometBFT client
- **Confidence**: HIGH

## Code References

### tm Package Files (to be deleted)
- `internal/api/v3/tm/consensus.go` - CometBFT ConsensusService implementation
- `internal/api/v3/tm/submitter.go` - CometBFT Submitter implementation
- `internal/api/v3/tm/validator.go` - CometBFT Validator implementation
- `internal/api/v3/tm/consensus_test.go` - Tests for ConsensusService

### DAG-BFT Equivalent (already exists)
- `internal/node/dagbft/api.go` - Contains `ConsensusAPIService`, `SubmitterService`, `ValidatorService`

### Files Requiring Migration
1. `cmd/accumulated/run/consensus.go:533-583` - `CoreConsensusApp.register()` method
2. `internal/node/daemon/run.go:550-637` - `startServices()` function
3. `internal/node/daemon/summary.go:167-209` - `startSummaryServices()` function
4. `exp/tendermint/dispatcher.go:31-33,195-197` - `DispatcherClient` interface and usage

## Open Questions

1. **What happens to exp/tendermint/dispatcher.go?**
   - This file uses `tm.SubmitClient` interface which requires CometBFT RPC methods (`BroadcastTxAsync`, `BroadcastTxSync`). When DAG-BFT replaces CometBFT entirely, this dispatcher will likely need to be removed or significantly rewritten. Currently it's used for direct dispatch to other partitions.

2. **Should internal/node/daemon/* files be migrated or removed?**
   - The daemon package appears to be the older CometBFT-based node implementation. If DAG-BFT is the future, these files may eventually be removed entirely rather than migrated.

3. **Is cmd/accumulated/run/consensus.go still used with DAG-BFT?**
   - This file contains `CoreConsensusApp` which is the CometBFT-based consensus app. Need to verify if this code path is still active when using DAG-BFT.

## Contradictions

None found. The tm package and dagbft/api.go implement the same interfaces but for different consensus backends (CometBFT vs DAG-BFT).

## Implementation Notes for Next Stage

To delete the tm package, the following changes are required:

1. **Remove imports** from all 4 files that import `internal/api/v3/tm`
2. **Update or remove** code that instantiates `tm.NewConsensusService`, `tm.NewSubmitter`, `tm.NewValidator`
3. **Handle exp/tendermint/dispatcher.go** - either remove `tm.SubmitClient` from `DispatcherClient` interface or migrate to use DAG-BFT equivalent
4. **Delete the tm package directory** after all imports are removed
5. **Run tests** to ensure nothing breaks

The DAG-BFT equivalents in `internal/node/dagbft/api.go` are ready to use as replacements where applicable.
