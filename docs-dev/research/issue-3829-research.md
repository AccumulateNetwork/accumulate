# Research: Delete exp/tendermint package

## Summary

The `exp/tendermint` package provides Tendermint/CometBFT utilities used for transaction dispatching, peer walking, and deferred client initialization. It is imported by 3 files: `cmd/accumulated/run/consensus.go`, `internal/node/daemon/dispatcher.go`, and `internal/node/daemon/run.go`. The package contains 7 Go files (including 2 test files). Before deletion, all imports must be updated to either inline the functionality, use alternatives, or move necessary code elsewhere.

## Verified Facts

### Fact 1: Package Location and Contents
- **Source**: `exp/tendermint/*.go`
- **Content**: 7 files total:
  - `deferred.go` (207 lines) - DeferredClient implementation
  - `http.go` (432 lines) - HTTPClient for Tendermint RPC
  - `peers.go` (165 lines) - WalkPeers and NewHTTPClientForPeer utilities
  - `dispatcher.go` (296 lines) - Dispatcher for cross-partition messaging
  - `metrics.go` (82 lines) - Prometheus metrics for dispatcher
  - `peers_test.go` (227 lines) - Tests for peer walking
  - `generate_test.go` (55 lines) - Code generation test for DeferredClient
- **Confidence**: HIGH

### Fact 2: Files That Import exp/tendermint
- **Source**: Grep results for `gitlab.com/accumulatenetwork/accumulate/exp/tendermint`
- **Content**: 3 importing files:
  1. `cmd/accumulated/run/consensus.go:41` - imports as `tmlib`
  2. `internal/node/daemon/dispatcher.go:12` - imports as `tendermint`
  3. `internal/node/daemon/run.go:43` - imports as `tendermint`
- **Confidence**: HIGH

### Fact 3: Types Used from exp/tendermint in cmd/accumulated/run/consensus.go
- **Source**: `cmd/accumulated/run/consensus.go:412,464,480`
- **Content**:
  - `tmlib.NewDeferredClient()` at line 412 (in prestart)
  - `tmlib.DispatcherClient` type at lines 456, 464
  - `tmlib.NewDispatcher()` at line 480
- **Confidence**: HIGH

### Fact 4: Types Used from exp/tendermint in internal/node/daemon/run.go
- **Source**: `internal/node/daemon/run.go:85,89,109,207,424`
- **Content**:
  - `*tendermint.DeferredClient` field at line 85 (Daemon.localTm)
  - `map[string]tendermint.DispatcherClient` field at line 89 (Daemon.local)
  - `tendermint.NewDeferredClient()` at line 109
  - `map[string]tendermint.DispatcherClient{}` at line 207
  - `tendermint.NewDispatcher()` at line 424
- **Confidence**: HIGH

### Fact 5: Types Used from exp/tendermint in internal/node/daemon/dispatcher.go
- **Source**: `internal/node/daemon/dispatcher.go:82`
- **Content**: `tendermint.CheckDispatchError(err)` - only one function call
- **Confidence**: HIGH

### Fact 6: CheckDispatchError Function Purpose
- **Source**: `exp/tendermint/dispatcher.go:94-129`
- **Content**: Filters dispatch errors, ignoring "tx already exists in cache" errors. Returns nil for:
  - `mempool.ErrTxInCache`
  - RPC errors matching tx-in-cache
  - `errors.Delivered` error code
- **Confidence**: HIGH

### Fact 7: DeferredClient Purpose
- **Source**: `exp/tendermint/deferred.go:20-28`
- **Content**: Wraps `ioc.PromisedOf[client.Client]` to provide a deferred/lazy CometBFT client that implements `client.Client` and `DispatcherClient` interfaces. Used to defer client initialization until Tendermint node is started.
- **Confidence**: HIGH

### Fact 8: DispatcherClient Interface Definition
- **Source**: `exp/tendermint/dispatcher.go:30-34`
- **Content**:
```go
type DispatcherClient interface {
    tm.SubmitClient
    rpc.NetworkClient
}
```
- **Confidence**: HIGH

### Fact 9: Dispatcher Purpose
- **Source**: `exp/tendermint/dispatcher.go:36-63`
- **Content**: Implements `execute.Dispatcher` for routing envelopes to different partitions via direct Tendermint RPC calls. Uses peer walking to discover remote partition nodes.
- **Confidence**: HIGH

### Fact 10: golangci-lint Exception
- **Source**: `.golangci.yml:83`
- **Content**: `path: ^(test/util/goroutine_leaks\.go|exp/tendermint/http\.go|exp/telemetry/translate\.go)$` - lint exception for http.go
- **Confidence**: HIGH

### Fact 11: DAG-BFT Does Not Use exp/tendermint
- **Source**: `cmd/accumulated/run/dagbft.go`, `internal/node/dagbft/service.go`
- **Content**: The DAG-BFT service (`DAGBFTService`) and node implementation do not import or use `exp/tendermint`. They use `accumulated.NewDispatcher` from `internal/node/daemon` instead.
- **Confidence**: HIGH

### Fact 12: WalkPeers and HTTPClient Are Only Used in Dispatcher
- **Source**: `exp/tendermint/dispatcher.go:248,252,286`
- **Content**: `WalkPeers`, `NewHTTPClientForPeer`, and related functions are only used internally within the dispatcher's `getClients` method to discover partition nodes.
- **Confidence**: HIGH

## Code References

### Primary Implementation Files
- `exp/tendermint/deferred.go` - DeferredClient (ioc.Promised wrapper for client.Client)
- `exp/tendermint/dispatcher.go` - Dispatcher (execute.Dispatcher implementation)
- `exp/tendermint/http.go` - HTTPClient (Tendermint RPC client)
- `exp/tendermint/peers.go` - WalkPeers, NewHTTPClientForPeer
- `exp/tendermint/metrics.go` - Prometheus metrics

### Consumer Files (Need Updates)
- `cmd/accumulated/run/consensus.go:412,464,480` - Uses DeferredClient, DispatcherClient, NewDispatcher
- `internal/node/daemon/run.go:85,89,109,207,424` - Uses DeferredClient, DispatcherClient, NewDispatcher
- `internal/node/daemon/dispatcher.go:82` - Uses CheckDispatchError

### Config File (Needs Update)
- `.golangci.yml:83` - Lint exception for exp/tendermint/http.go

## Open Questions

1. **Where should CheckDispatchError be moved?** Options:
   - Inline it in `internal/node/daemon/dispatcher.go` (only consumer)
   - Move to a shared package like `internal/node/dispatch` or `pkg/errors`

2. **Where should DeferredClient be moved?** Options:
   - Keep in `exp/ioc` since it uses `ioc.PromisedOf`
   - Create new `internal/node/tm` or `internal/node/comet` package
   - Move to `internal/api/v3/tm` which already has Tendermint-related code

3. **Where should Dispatcher be moved?** Options:
   - `internal/node/daemon` (already has dispatcher.go for API-based dispatch)
   - Create `internal/node/tm` package for all CometBFT-related code

4. **Should WalkPeers/HTTPClient be preserved?**
   - They are only used by the Tendermint Dispatcher
   - If Dispatcher is moved, these should move with it

## Contradictions

None found. All imports and usages are consistent with the documented interfaces.

## Recommended Approach

Based on the research, the recommended approach for deletion is:

1. **Move `CheckDispatchError`** to `internal/node/daemon/dispatcher.go` (inline, since it's the only consumer)

2. **Move `DeferredClient`** and `DispatcherClient` interface to `internal/api/v3/tm` package (which already has CometBFT-related API code like `tm.SubmitClient`)

3. **Move `Dispatcher`, `HTTPClient`, `WalkPeers`, `NewHTTPClientForPeer`, and metrics** to a new `internal/node/comet` or `internal/node/tm` package, or inline in `internal/node/daemon` if only used there

4. **Update `.golangci.yml`** to remove the `exp/tendermint/http.go` exception

5. **Delete `exp/tendermint/`** directory after all imports are updated

This approach minimizes code duplication while maintaining the existing functionality for CometBFT-based consensus (used alongside DAG-BFT).
