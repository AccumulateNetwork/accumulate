# Remove CometBFT: Task List

Branch: `issue-3910-remove-cometbft`

Note: The orchestrator script determines victory by checking for
`"github.com/cometbft` imports, go build, go test, and go vet — NOT by
reading this file. This file is for tracking progress only.

Rule: Removing CometBFT must NOT change any wire format, file format,
or serialization. Copy CometBFT struct definitions into local files
when needed to preserve compatibility. Same bytes on disk, same bytes
on wire — just no import.

## Tasks

### Phase 1: Logging

| ID | Issue | Task | Status | Depends | Commit | Notes |
|----|-------|------|--------|---------|--------|-------|
| L1 | #3925 | Make `TendermintZeroLogger.With()` return `Logger` instead of `log.Logger` in `internal/logging/tendermint.go` | DONE | — | 44846d859 | Change return type, update callers |
| L2 | #3925 | Make `NullLogger.With()` return `Logger` in `internal/logging/null.go` | DONE | — | pre-existing | Already returned Logger |
| L3 | #3925 | Change `NewTendermintLogger()` return type from `log.Logger` to `Logger` | DONE | L1 | 44846d859 | Also changed NewTestLogger, ConsoleLoggerForTest return types |
| L4 | #3925 | Remove `cometbft/libs/log` import from `internal/logging/compat.go` | DONE | L3 | 59c341061 | compat.go deleted — CometBFTLogger/FromCometBFT no longer used after InitOpts.Logger changed to logging.Logger |
| L5 | #3925 | Remove `cometbft/libs/log` from remaining logging files | DONE | L3, L4 | 44846d859 | slog.go/null.go never had import; test.go cleaned in L3 commit |
| L6 | #3925 | Fix callers of `NewTendermintLogger` outside logging package | DONE | L3 | 44846d859 | Removed FromCometBFT wrapping at all call sites |

### Phase 2: Key types

Copy CometBFT key structs into local files. Same serialization, no import.

| ID | Issue | Task | Status | Depends | Commit | Notes |
|----|-------|------|--------|---------|--------|-------|
| K1 | #3927 | Copy `privval.FilePV` fields into a local struct in `internal/node/daemon/`. Keep same JSON tags. Replace all `privval.FilePV` usage with the local type. | DONE | L6 | c3b9e077a | Local types in keys.go with identical JSON format |
| K2 | #3927 | Copy `tmp2p.NodeKey` fields into a local struct. Replace usage. | DONE | K1 | c3b9e077a | Included in K1 commit |
| K3 | #3927 | Replace `tmed25519.PubKey`/`crypto.PrivKey` with stdlib `crypto/ed25519` | DONE | K1 | c3b9e077a | Also cleaned daemon/run.go and summary.go |
| K4 | — | Remove `internal/node/daemon/address.go` CometBFT imports | DONE | K2 | c3b9e077a | Cleaned in K1/K2/K3 commit |
| K5 | — | Clean up `internal/node/daemon/dispatcher.go` | DONE | L6 | n/a | No CometBFT imports found |
| KT | — | Write compat tests for key types: create CometBFT `FilePV` and `NodeKey`, marshal to JSON, unmarshal into our local types, verify all fields match. Also test reverse direction. Put tests in `internal/node/daemon/compat_test.go`. | DONE | K1, K2 | c3b9e077a | 5 compat tests all passing |

### Phase 3: Config

Copy used fields from `tm.Config` into a local struct. Same TOML tags.

| ID | Issue | Task | Status | Depends | Commit | Notes |
|----|-------|------|--------|---------|--------|-------|
| CF1 | #3926 | Copy the fields from `tm.Config` that `internal/node/config/config.go` actually reads into a local struct. Same field names, same TOML tags. Remove the embedding. | DONE | L6 | 2300f77a2 | Local TendermintConfig in tendermint.go with all used fields, same mapstructure/toml tags |
| CF2 | #3926 | Update `internal/node/daemon/run.go` to use the local config | DONE | CF1 | n/a | No CometBFT imports found — already clean |
| CF3 | #3926 | Update `internal/node/config/enums_gen.go` — remove tendermintP2P/tendermintRpc if unused | DONE | CF1 | n/a | No CometBFT imports; port offsets still used by callers |
| CFT | — | Write compat tests for config: create CometBFT `tm.Config` with known values, marshal to TOML, unmarshal into our local config struct, verify all used fields match. Put tests in `internal/node/config/compat_test.go`. | DONE | CF1 | 562ed3c74 | 4 compat tests: BaseConfig paths, InstrumentationConfig, TOML roundtrip, defaults match |

### Phase 4: Genesis types

Copy CometBFT genesis structs into local files. Same JSON serialization.

| ID | Issue | Task | Status | Depends | Commit | Notes |
|----|-------|------|--------|---------|--------|-------|
| GN1 | #3928 | Copy `types.GenesisDoc`, `GenesisValidator`, `ConsensusParams` into `pkg/types/cometbft/` or a new local package. Same JSON tags. Remove CometBFT import. | DONE | L6 | 6b62bec08 | Local ConsensusParams/Block structs with protowire encoding; callers updated to field-by-field construction |
| GN2 | #3928 | Update `internal/node/genesis/bootstrap.go` and `provider.go` | DONE | GN1 | 59c341061, a516281a4 | InitOpts.Logger→logging.Logger, ConsensusParams→local type, provider.go dead code removed, local GenesisDocJSON type |
| GN3 | #3929 | Remove or replace `internal/api/v3/tm/` — copy needed RPC response types locally | DONE | GN1 | 8635056b9 | Deleted dead tm package, local NodeStatusResult in network.go |
| GNT | — | Write compat tests for genesis types: create CometBFT `GenesisDoc` with validators and consensus params, marshal to JSON, unmarshal into our local types, verify all fields match. Also test: write a genesis.json with CometBFT, read it with our types. Put tests in `pkg/types/cometbft/compat_test.go` or `internal/node/genesis/compat_test.go`. | DONE | GN1 | b21e6cb16 | 8 compat tests: protobuf encoding, defaults, round-trip, block, JSON format |

### Phase 5: CLI commands

| ID | Issue | Task | Status | Depends | Commit | Notes |
|----|-------|------|--------|---------|--------|-------|
| CL1 | #3932 | Update `cmd/accumulated/cmd_reset.go` — use local key/genesis types | DONE | K1, GN1 | 6978a4e39 | genesis.GenesisDocJSON, SHA256 for address |
| CL2 | #3932 | Update `cmd/accumulated/cmd_init.go` and `cmd_init_network.go` | DONE | K1, CF1, GN1 | 0a206a5bd | Local cometRPCClient, genesis.GenesisDocJSON, stdlib ed25519 |
| CL3 | #3932 | Update `cmd/accumulated/cmd_run.go` and `cmd_run_dual.go` | DONE | CF1 | b6684d84e | Replaced LogFormat constants and ErrAlreadyStopped; also cleaned cmd_run_netsim.go (125131991) |
| CL4 | #3932 | Update `cmd/accumulated/cmd_migrate.go` | DONE | CF1 | n/a | No CometBFT imports found — already clean |
| CL5 | #3932 | Clean up `cmd/accumulated/run/consensus.go` | DONE | CF1, GN3 | n/a | File does not exist — already clean |
| CL6 | #3932 | Clean up `cmd/accumulated/run/key_comet.go` | DONE | K1 | n/a | No CometBFT imports — uses stdlib ed25519 and local types |
| CL7 | — | Delete dead CometBFT state utilities: cmd/init-state, cmd/check-state, cmd/extract-validators | DONE | — | b2732a9eb | Entirely CometBFT-specific tools for state.db — dead after removal |
| CL8 | — | Remove CometBFT imports from `mcp/server/tools_snapshot_restore.go` | DONE | K1 | eb00ef057 | Replaced p2p.LoadOrGenNodeKey and privval.GenFilePV with local types |

### Phase 6: Remaining

| ID | Issue | Task | Status | Depends | Commit | Notes |
|----|-------|------|--------|---------|--------|-------|
| S1 | — | Clean up `internal/core/execute/execute.go` CometBFT import | DONE | L6 | n/a | No CometBFT imports found (already clean) |
| S2 | — | Clean up `pkg/build/parser.go` — use stdlib ed25519 | DONE | K3 | 4bb4e3cee | Removed tmed25519 import, removed CometBFT key type cases |
| S3 | — | Clean up `exp/telemetry/otel_prom.go` | DONE | — | n/a | No CometBFT imports — only string literals in regex |
| S4 | — | Clean up `vdk/node/node.go` | DONE | L6 | a4101c005 | Removed dead CometBFT-dependent functions (package unused) |
| S5 | — | Clean up `pkg/api/v3/types_gen.go` CometBFT ref | DONE | GN1 | n/a | Imports `pkg/types/cometbft` (local package), not github.com/cometbft — already clean |
| S6 | — | Run `go mod tidy` and verify CometBFT removed from go.mod | PENDING | ALL | | Final step |

## Progress Log

| Date | Session | Tasks completed | Notes |
|------|---------|----------------|-------|
| 2026-04-15 | Session 1 | Initial branch setup, ABCI removal, schema fixes, daemon cleanup, dead file removal, logger unification, tool cleanup, test infra, api/v2+MCP | 88 -> 41 CometBFT files |
| 2026-04-16 | Session 2 | L1, L2, L3, L5, L6, S3, S4 | Logger return types changed, callers fixed, vdk cleaned. L4 blocked on Phase 4. |
| 2026-04-16 | Session 3 | K1, K2, K3, K4, K5, KT, S1 + v1 test files | Local key types with compat tests, daemon files cleaned, v1 execute test files cleaned |
| 2026-04-16 | Session 4 | S2, CF1, CFT | pkg/build/parser.go cleaned, local TendermintConfig struct with compat tests |
| 2026-04-16 | Session 5 | GN1, CF2, CF3, CL3 | Local ConsensusParams/Block with protowire encoding, cmd_run*.go cleaned |
| 2026-04-16 | Session 6 | GN2, GNT, L4 | genesis bootstrap/provider cleaned, compat tests, logging/compat.go deleted, local GenesisDocJSON type |
| 2026-04-16 | Session 7 | GN3, CL1, CL2, CL4 | Deleted dead tm/ package, cmd_reset/cmd_init cleaned, local CometBFT RPC client |
| 2026-04-16 | Session 8 | CL5, CL6, CL7, CL8, S5 + 8 test files | Deleted dead cmd tools, MCP snapshot restore cleaned, e2e/bsn/validate/heal test files cleaned. Remaining: 4 compat_test.go (intentional), 1 gold_test.go (//go:build ignore), 9 _prefixed (disabled). 0 source files with CometBFT imports. |
