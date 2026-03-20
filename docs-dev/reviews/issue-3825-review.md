# Review Report: Enable DAG-BFT in core_validator.go

## Decision: APPROVED

The implementation correctly replaces `ConsensusService` with `DAGBFTService` in `core_validator.go`. The validation document accurately identifies implementation details and corrects outdated information in the research document.

## Fresh Eyes Test

### Points of Confusion
- **Minor**: The review process references a specification file (`docs-dev/specifications/issue-3825-spec.md`) that doesn't exist. The validation document serves as the effective specification and is comprehensive.
- **None in validation**: The validation document is clear and self-contained, explaining exactly what was changed and why.

### Unstated Assumptions
- **DAGBFTService is schema-generated**: The struct is defined in `types_gen.go`, meaning the field types (`*uint64`, `*int64`) are determined by the schema, not manual definition. This is important because the research document incorrectly stated `MaxEnvelopesPerBlock` was `*uint` (it's actually `*uint64`).
- **IOC service compatibility**: The validation assumes the IOC services provided by `DAGBFTService` are functionally equivalent to those from `ConsensusService`. This is verified by the successful build and test results.

## Alternative Interpretations

| Step | Could Be Misread As | Clarification Needed |
|------|---------------------|---------------------|
| "Create DAGBFTService" | "Modify existing ConsensusService" | No - validation clearly states replacement, not modification |
| Line numbers 122-135 | Different file or location | No - file path explicitly stated: `core_validator.go` |
| "No ConsensusService references" | "Delete consensus.go entirely" | No - only references in core_validator.go are affected; a comment explaining the replacement is appropriate |
| Field mapping from `p.Dir` to `NodeDir` | Use full `p.CoreValidatorConfiguration.Dir` | No - embedded struct field access is correct Go idiom |

## Known Pitfalls Coverage

### Checked Against Common Go/Accumulate Pitfalls
- [x] **Type mismatches**: Validation correctly identifies that research doc had outdated type info (`*uint` vs `*uint64`). Actual code uses matching types.
- [x] **Service type confusion**: Validation correctly notes `DAGBFTService.Type()` returns `ServiceTypeDAGBFT`, not `ServiceTypeConsensus` as research stated.
- [x] **IOC registration**: All 6 IOC provisions documented and verified.
- [x] **Field mapping completeness**: All 7 required fields mapped, 3 CometBFT-specific fields correctly omitted.

### Not Applicable to This Issue
- **Error log check**: No `docs-dev/errors/error-log.md` exists (no prior errors recorded for this issue).
- **CLAUDE.md common errors**: No CLAUDE.md exists in this repository.

## Verification Results

| Check | Result |
|-------|--------|
| `go build ./cmd/accumulated` | PASS |
| `go test ./internal/node/dagbft/... ./pkg/consensus/... -v -short` | PASS |
| DAGBFTService struct fields match documentation | PASS |
| No ConsensusService creation in core_validator.go | PASS |
| IOC providers at dagbft.go:46-54 | PASS |

## Code Consistency

### Implementation Matches Documentation

**core_validator.go:122-135** (Verified)
```go
addService(cfg,
    &DAGBFTService{
        NodeDir:      p.Dir,
        ValidatorKey: p.ValidatorKey,
        Genesis:      p.Genesis,
        Partition: &protocol.PartitionInfo{
            ID:   p.ID,
            Type: p.Type,
        },
        EnableHealing:        p.EnableHealing,
        EnableDirectDispatch: p.EnableDirectDispatch,
        MaxEnvelopesPerBlock: p.MaxEnvelopesPerBlock,
    },
    func(s *DAGBFTService) string { return s.Partition.ID })
```

### Research Document Corrections (Validated)
The validation document correctly identified 3 errors in the research:
1. **Line numbers**: Research cited 129-147 (now 122-135 with DAGBFTService)
2. **ServiceType**: Research said `ServiceTypeConsensus`, actual is `ServiceTypeDAGBFT`
3. **Type mismatch**: Research said `*uint` vs `*uint64`, actual is both `*uint64`

## Final Checklist

- [x] Self-contained (no external knowledge needed)
- [x] All examples verified (code matches documentation)
- [x] No high-risk ambiguities
- [x] Ready for human review
- [x] Build passes
- [x] Tests pass
- [x] IOC provisions documented
- [x] Field mapping complete

## Required Changes Before Approval

None. The implementation is complete and correct.

## Summary

This implementation successfully enables DAG-BFT consensus in `core_validator.go` by:

1. Replacing `ConsensusService` creation with `DAGBFTService` in `partOpts.apply()`
2. Mapping all required fields from `CoreValidatorConfiguration` and `partOpts`
3. Correctly omitting CometBFT-specific fields (`Listen`, `BootstrapPeers`, `MetricsNamespace`)
4. Maintaining full IOC service compatibility

The validation document accurately corrects three outdated facts from the research document, demonstrating proper verification practices.
