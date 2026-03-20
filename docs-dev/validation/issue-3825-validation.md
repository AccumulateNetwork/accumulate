# Validation Report: Enable DAG-BFT in core_validator.go

## Overall Status: PASS

The implementation has been completed and verified. The `core_validator.go` file now creates `DAGBFTService` instead of `ConsensusService` for partition consensus.

## Implementation Verification

### Code Change Verified

| Location | Expected | Actual | Status |
|----------|----------|--------|--------|
| `core_validator.go:122-135` | Create DAGBFTService | DAGBFTService created | PASS |
| `core_validator.go` | No ConsensusService references | Confirmed removed | PASS |

### Current Implementation (lines 122-135)

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

## Research Document Corrections

The research document contains several facts that are now outdated or inaccurate after implementation:

### Fact 1: Line Numbers (OUTDATED)
- **Research stated**: `core_validator.go:129-147` for ConsensusService creation
- **Current state**: Lines 122-135 now contain DAGBFTService creation
- **Status**: Research is outdated; implementation is complete

### Fact 7: ServiceType (CORRECTED)
- **Research stated**: `DAGBFTService.Type()` returns `ServiceTypeConsensus`
- **Actual (types_gen.go:412)**: `DAGBFTService.Type()` returns `ServiceTypeDAGBFT`
- **Status**: Research was incorrect; DAGBFTService has its own service type

### Fact 8: Type Mismatch (RESOLVED)
- **Research stated**: Type mismatch between `CoreValidatorConfiguration.MaxEnvelopesPerBlock` (`*uint64`) and `DAGBFTService.MaxEnvelopesPerBlock` (`*uint`)
- **Actual (types_gen.go:409)**: `DAGBFTService.MaxEnvelopesPerBlock` is `*uint64`
- **Status**: No type mismatch exists; both use `*uint64`

## Field Mapping Verification

| CoreValidatorConfiguration | DAGBFTService | Mapped? |
|---------------------------|---------------|---------|
| `Dir` | `NodeDir` | YES |
| `ValidatorKey` | `ValidatorKey` | YES |
| `Genesis` | `Genesis` | YES |
| `ID` + `Type` | `Partition` | YES |
| `EnableHealing` | `EnableHealing` | YES |
| `EnableDirectDispatch` | `EnableDirectDispatch` | YES |
| `MaxEnvelopesPerBlock` | `MaxEnvelopesPerBlock` | YES |
| `Listen` | (not needed) | N/A |
| `BootstrapPeers` | (not needed) | N/A |
| `MetricsNamespace` | (not needed) | N/A |

Note: `Listen`, `BootstrapPeers`, and `MetricsNamespace` are CometBFT-specific and correctly omitted for DAG-BFT.

## IOC Provisions Verification

The DAGBFTService provides all required IOC services (from `dagbft.go:45-53`):

| Service | Provider Variable | Status |
|---------|-------------------|--------|
| `*events.Bus` | `dagbftProvidesEventBus` | PROVIDED |
| `v3.ConsensusService` | `dagbftProvidesService` | PROVIDED |
| `v3.Submitter` | `dagbftProvidesSubmitter` | PROVIDED |
| `v3.Validator` | `dagbftProvidesValidator` | PROVIDED |
| `private.Sequencer` | `dagbftProvidesSequencer` | PROVIDED |
| `routing.Router` | `dagbftProvidesRouter` | PROVIDED |

## Build Verification

- **Command**: `go build ./cmd/accumulated`
- **Result**: SUCCESS

## Test Verification

- **Command**: `go test ./internal/node/dagbft/... ./pkg/consensus/... -v -short -timeout 5m`
- **Result**: SUCCESS

## Success Criteria Verification

From the issue requirements:

- [x] `core_validator.go` creates `DAGBFTService` for each partition (via `partOpts.apply()`)
- [x] Build succeeds
- [x] No references to `ConsensusService` creation in `core_validator.go`
- [x] All required fields are properly mapped
- [x] Tests pass

## Completeness Checklist

Since this was a straightforward service replacement task (not an algorithmic change), the specification format differs from typical algorithm specifications:

- [x] Input fields documented (CoreValidatorConfiguration, partOpts)
- [x] Operation documented (service replacement)
- [x] Output documented (DAGBFTService instance)
- [x] Field mapping documented
- [x] Implementation verified against research

## Ambiguity Scan

Searched for problematic terms in research document:
- "usually": 0 occurrences
- "typically": 0 occurrences
- "should": 1 occurrence (in context: "should be omitted" - correctly definitive)
- "may": 0 occurrences

No ambiguity issues found.

## Required Changes

None. The implementation is complete and correct.

## Open Questions Status

From research document:

1. **DAG-BFT specific configuration**: `NumWorkers`, `DAGGCDepth`, and `CommitBufferSize` are defined in `DAGBFTService` (types_gen.go:404-406) and use defaults from `dagconfig`. These are optional and not exposed through `CoreValidatorConfiguration`, which is acceptable for initial implementation.

2. **Metrics**: DAG-BFT uses standard metrics infrastructure. MetricsNamespace was CometBFT-specific and not needed.
