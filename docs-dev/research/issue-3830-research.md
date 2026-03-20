# Research: Remove CometBFT from go.mod and clean up

## Summary

This issue covers the final cleanup of the DAG-BFT integration: removing consensus.go (CometBFT-based consensus service), updating logging infrastructure, and removing CometBFT from go.mod. The codebase has 105 files importing CometBFT packages, primarily for logging (`cometbft/libs/log`), ABCI types, configuration, and consensus. The DAG-BFT service (`dagbft.go`) is already implemented and functional, but still uses the `logging.FromCometBFT()` wrapper for compatibility. Full CometBFT removal requires either: (1) build tags to conditionally exclude CometBFT code, or (2) complete migration away from `cometbft/libs/log` to `log/slog`.

## Verified Facts

### Fact 1: consensus.go imports 15 CometBFT packages
- **Source**: `cmd/accumulated/run/consensus.go:20-34`
- **Content**:
```go
types "github.com/cometbft/cometbft/abci/types"
tmcfg "github.com/cometbft/cometbft/config"
tmcrypto "github.com/cometbft/cometbft/crypto"
tmed25519 "github.com/cometbft/cometbft/crypto/ed25519"
"github.com/cometbft/cometbft/crypto/tmhash"
cmtjson "github.com/cometbft/cometbft/libs/json"
"github.com/cometbft/cometbft/libs/log"
tmnode "github.com/cometbft/cometbft/node"
tmp2p "github.com/cometbft/cometbft/p2p"
tmpv "github.com/cometbft/cometbft/privval"
"github.com/cometbft/cometbft/proxy"
"github.com/cometbft/cometbft/rpc/client"
tmrpc "github.com/cometbft/cometbft/rpc/client"
"github.com/cometbft/cometbft/rpc/client/local"
tmtypes "github.com/cometbft/cometbft/types"
```
- **Confidence**: HIGH

### Fact 2: types.go defines ConsensusApp interface using CometBFT types
- **Source**: `cmd/accumulated/run/types.go:10-16, 55-63`
- **Content**:
```go
import (
	types "github.com/cometbft/cometbft/abci/types"
	tmnode "github.com/cometbft/cometbft/node"
	...
)

type ConsensusApp interface {
	Type() ConsensusAppType
	partition() *protocol.PartitionInfo
	Requires() []ioc.Requirement
	Provides() []ioc.Provided
	prestart(*Instance) error
	start(*Instance, *tendermint) (types.Application, error)  // CometBFT ABCI
	register(*Instance, *tendermint, *tmnode.Node) error       // CometBFT node
}
```
- **Confidence**: HIGH

### Fact 3: DAGBFTService exists and is functional
- **Source**: `cmd/accumulated/run/dagbft.go:55-405`
- **Content**: Complete implementation including:
  - IOC providers (lines 42-51)
  - Service configuration (lines 55-76)
  - `start()` method (lines 121-328)
  - `registerAPIServices()` method (lines 331-398)
- **Confidence**: HIGH

### Fact 4: Logging compatibility layer exists
- **Source**: `internal/logging/compat.go:1-86`
- **Content**:
```go
// CometBFTLogger returns a cometbft/libs/log.Logger from our Logger interface.
func CometBFTLogger(l Logger) log.Logger

// FromCometBFT wraps a cometbft/libs/log.Logger as our Logger interface.
func FromCometBFT(l log.Logger) Logger
```
- **Confidence**: HIGH

### Fact 5: 41 files directly import `cometbft/libs/log`
- **Source**: grep search for `cometbft/cometbft/libs/log`
- **Content**: Files across internal/, test/, tools/, cmd/accumulated/, exp/
- **Confidence**: HIGH

### Fact 6: dagbft.go uses FromCometBFT wrapper
- **Source**: `cmd/accumulated/run/dagbft.go:134, 164, 180, 184, 288`
- **Content**:
```go
s.eventBus = events.NewBus(logging.FromCometBFT(logger.With("module", "events")))
router := routing.NewRouter(routing.RouterOptions{
	Events: s.eventBus,
	Logger: logging.FromCometBFT(logger),
})
db := database.New(store, logging.FromCometBFT(logger))
...
Logger: logging.FromCometBFT(logger.With("module", "dagbft")),
```
- **Confidence**: HIGH

### Fact 7: CometBFT is a direct dependency in go.mod
- **Source**: `go.mod:48`
- **Content**: `github.com/cometbft/cometbft v0.38.21`
- **Confidence**: HIGH

### Fact 8: CometBFT-db is an indirect dependency
- **Source**: `go.mod:133`
- **Content**: `github.com/cometbft/cometbft-db v0.14.1 // indirect`
- **Confidence**: HIGH

### Fact 9: exp/tendermint package has extensive CometBFT usage
- **Source**: `exp/tendermint/*.go`
- **Content**: 7 files with CometBFT imports for dispatcher, deferred client, HTTP, peers, and metrics
- **Confidence**: HIGH

### Fact 10: internal/api/v3/tm package depends on CometBFT
- **Source**: `internal/api/v3/tm/*.go`
- **Content**: 4 files implementing ConsensusService, Submitter, Validator using CometBFT RPC client
- **Confidence**: HIGH

### Fact 11: internal/node/abci is the ABCI application for CometBFT
- **Source**: `internal/node/abci/*.go`
- **Content**: 7 files implementing CometBFT ABCI interface (Accumulator app)
- **Confidence**: HIGH

### Fact 12: test/simulator uses ABCI types for consensus simulation
- **Source**: `test/simulator/consensus/abci.go:14, 39-44`
- **Content**:
```go
"github.com/cometbft/cometbft/abci/types"
// Uses types.CheckTxType_Recheck, types.CheckTxType_New
// Uses types.RequestCheckTx, types.RequestInitChain, etc.
```
- **Confidence**: HIGH

### Fact 13: pkg/types/cometbft wraps CometBFT protobuf types
- **Source**: `pkg/types/cometbft/types.go:1-140`
- **Content**: Wraps `ConsensusParams`, `Block` types with marshaling methods
- **Confidence**: HIGH

### Fact 14: Schema defines CometPrivValFile and CometNodeKeyFile key types
- **Source**: `cmd/accumulated/run/schema.yml:584-633`
- **Content**:
```yaml
CometPrivValFile:
  value: 4
CometNodeKeyFile:
  value: 5
```
- **Confidence**: HIGH

### Fact 15: Implementation plan exists for DAG-BFT binary
- **Source**: `docs/plans/accumulated-dagbft.md:1-213`
- **Content**: Complete implementation plan with phases for:
  - Phase 1: Abstract Logger Interface (IN PROGRESS)
  - Phase 2: Abstract BlockParams from ABCI Types
  - Phase 3: Create DAG-BFT-Only Run Package
  - Phase 4: Create accumulated-dagbft Binary
  - Phase 5: Integration Testing
- **Confidence**: HIGH

## Code References

### Primary Files to Delete (consensus.go removal)
| File | Description |
|------|-------------|
| `cmd/accumulated/run/consensus.go` | CometBFT ConsensusService - 603 lines |

### Files to Modify
| File | Modification |
|------|-------------|
| `cmd/accumulated/run/types.go` | Remove ConsensusApp interface (requires build tags or splitting) |
| `cmd/accumulated/run/core_validator.go` | References ConsensusService |
| `internal/logging/compat.go` | Keep for backward compatibility or remove after full migration |
| `internal/logging/tendermint.go` | Keep for CometBFT logging or remove |
| `internal/logging/slog.go` | Uses `cometbft/libs/log` type in Slogger.With |

### Packages with CometBFT Dependencies (by category)

**Core Consensus (would need build tags or removal):**
- `cmd/accumulated/run/` - ConsensusService, key loading
- `internal/node/abci/` - ABCI application
- `internal/api/v3/tm/` - CometBFT API services
- `exp/tendermint/` - CometBFT utilities

**Logging (widespread, needs migration):**
- `internal/logging/` - compat.go, tendermint.go, slog.go
- 41+ files importing `cometbft/libs/log`

**Test Infrastructure:**
- `test/simulator/` - Uses ABCI types for consensus simulation
- `test/testing/` - Test node setup

**Type Definitions:**
- `pkg/types/cometbft/` - Wrapped CometBFT types for serialization

## Open Questions

1. **Build tags vs full removal**: Should we use build tags to conditionally compile CometBFT code, or should we fully remove CometBFT support? The plan document suggests build tags for gradual migration.

2. **Logging migration strategy**: With 41+ files importing `cometbft/libs/log`, should we:
   - Use the existing `FromCometBFT()` wrapper at boundaries (current approach)
   - Migrate all packages to use `internal/logging.Logger` interface
   - Migrate all packages to use `log/slog` directly

3. **Test simulator**: The test simulator uses ABCI types directly. Should it have a DAG-BFT mode?

4. **Type compatibility**: `pkg/types/cometbft` is used for serialization. Are these types used in stored data that requires backward compatibility?

## Contradictions

None found. The codebase shows a clear pattern:
- DAG-BFT service is implemented and functional
- CometBFT code still exists alongside
- Logging uses compatibility wrappers
- The plan document accurately describes the current state

## Recommended Implementation Approach

Based on the research, the "final cleanup" for issue #3830 should include:

1. **Delete consensus.go** - If DAG-BFT is the only consensus mechanism needed
2. **Modify types.go** - Remove `ConsensusApp` interface and `tendermint` struct, OR add build tags
3. **Update logging** - Continue using `FromCometBFT()` wrapper OR migrate to `slog`
4. **Verify build** - Run `go build ./cmd/accumulated` to verify compilation
5. **Run tests** - Run `go test ./internal/node/dagbft/... ./pkg/consensus/...`

**Note**: Complete CometBFT removal from go.mod may not be achievable in this issue if:
- test/simulator still needs ABCI types
- pkg/types/cometbft is needed for data compatibility
- exp/tendermint is still used

Consider splitting into:
- #3830a: Delete consensus.go and update types.go (core cleanup)
- #3830b: Migrate remaining CometBFT dependencies (logging, tests, utilities)
