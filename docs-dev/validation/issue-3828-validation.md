# Validation Report: Delete internal/api/v3/tm package

## Overall Status: PASS

The implementation has been completed successfully. The `internal/api/v3/tm` package has been deleted and its CometBFT API implementations have been relocated to `exp/tendermint/api.go`.

## Implementation Summary

| Item | Status | Notes |
|------|--------|-------|
| Package deleted | Yes | `internal/api/v3/tm/` directory no longer exists |
| Code relocated | Yes | Moved to `exp/tendermint/api.go` |
| Imports updated | Yes | All 4 files now import from `exp/tendermint` |
| Build succeeds | Yes | `go build ./cmd/accumulated` passes |
| Tests pass | Yes | `./internal/node/dagbft/...` and `./pkg/consensus/...` tests pass |

## Code Reference Verification

| Reference from Research | Valid? | Notes |
|-------------------------|--------|-------|
| `internal/api/v3/tm/consensus.go` | N/A | File deleted as expected |
| `internal/api/v3/tm/submitter.go` | N/A | File deleted as expected |
| `internal/api/v3/tm/validator.go` | N/A | File deleted as expected |
| `internal/api/v3/tm/consensus_test.go` | N/A | File deleted as expected |
| `internal/node/dagbft/api.go:38` | Valid | `ConsensusAPIService` implements `api.ConsensusService` |
| `internal/node/dagbft/api.go:135` | Valid | `SubmitterService` implements `api.Submitter` |
| `internal/node/dagbft/api.go:198` | Valid | `ValidatorService` implements `api.Validator` |
| `cmd/accumulated/run/consensus.go` import | Valid | Now uses `tmapi "exp/tendermint"` |
| `internal/node/daemon/run.go` import | Valid | Now uses `tendermint "exp/tendermint"` |
| `internal/node/daemon/summary.go` import | Valid | Now uses `tm "exp/tendermint"` |
| `exp/tendermint/dispatcher.go` | Valid | Uses `SubmitClient` from same package |

## Completeness Checklist

This was a deletion/relocation task rather than an algorithmic implementation task. The standard checklist for algorithmic specifications doesn't fully apply. Instead:

- [x] Identified all files to delete (4 files)
- [x] Identified all files requiring import updates (4 files)
- [x] Identified relocation target (`exp/tendermint/api.go`)
- [x] Verified DAG-BFT equivalents exist with same interfaces
- [x] Implementation matches research findings

## Completeness Score: 5/5 (for deletion task)

All identified requirements have been addressed:
1. tm package deleted
2. Code relocated to exp/tendermint
3. Imports updated in all consuming files
4. Build compiles
5. Tests pass

## Ambiguity Issues

The research document contained minor ambiguities in the "Open Questions" section:
- Use of "should" in question about daemon files
- Use of "may" in answer about future daemon removal

These were appropriate for the research phase and did not affect implementation. The implementation addressed the open questions by:
1. Relocating CometBFT code to `exp/tendermint` (experimental package), keeping it available for CometBFT mode
2. Updating daemon imports to use the relocated code
3. Not removing daemon files (they still serve CometBFT-based node implementation)

## Implementation Approach

The implementation chose to **relocate** rather than **delete outright**:
- CometBFT API implementations moved to `exp/tendermint/api.go`
- This keeps CometBFT-specific code co-located in the `exp/tendermint` package
- Allows continued use of CometBFT mode while DAG-BFT is developed
- Cleaner architecture: CometBFT code isolated in experimental package

## Required Changes

None. Implementation is complete.

## Test Results

```
go build ./cmd/accumulated: SUCCESS
go test ./internal/node/dagbft/... ./pkg/consensus/... -v -short: PASS
go test ./exp/tendermint/... -v -short: PASS
```

## Files Changed

| File | Change |
|------|--------|
| `internal/api/v3/tm/consensus.go` | Deleted |
| `internal/api/v3/tm/submitter.go` | Deleted |
| `internal/api/v3/tm/validator.go` | Deleted |
| `internal/api/v3/tm/consensus_test.go` | Deleted |
| `exp/tendermint/api.go` | Created (322 lines) |
| `cmd/accumulated/run/consensus.go` | Import updated |
| `internal/node/daemon/run.go` | Import updated |
| `internal/node/daemon/summary.go` | Import updated |
| `exp/tendermint/dispatcher.go` | Uses local `SubmitClient` interface |
