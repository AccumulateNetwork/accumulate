# Research: Remove ConsensusService from types.go

## Summary

The issue requests removing `ConsensusService` and `CoreConsensusApp` type registrations from `types.go` after `core_validator.go` is updated. However, research reveals that **`core_validator.go` has NOT been updated yet** — it still actively uses both `ConsensusService` and `CoreConsensusApp`. Additionally, these types are used in tests and are generated from `schema.yml`. Removing them now would break the build. The prerequisite work (updating `core_validator.go` to use `DAGBFTService` instead) must be completed first.

## Verified Facts

### Fact 1: ConsensusService and CoreConsensusApp are still actively used in core_validator.go
- **Source**: `cmd/accumulated/run/core_validator.go:129-147`
- **Content**:
  ```go
  addService(cfg,
      &ConsensusService{
          NodeDir:          p.Dir,
          ValidatorKey:     p.ValidatorKey,
          Genesis:          p.Genesis,
          Listen:           applyAddrTransforms(p.Listen, offset),
          BootstrapPeers:   p.BootstrapPeers,
          MetricsNamespace: p.MetricsNamespace,
          App: &CoreConsensusApp{
              EnableHealing:        p.EnableHealing,
              EnableDirectDispatch: p.EnableDirectDispatch,
              MaxEnvelopesPerBlock: p.MaxEnvelopesPerBlock,
              Partition: &protocol.PartitionInfo{
                  ID:   p.ID,
                  Type: p.Type,
              },
          },
      },
      func(c *ConsensusService) string { return c.App.partition().ID })
  ```
- **Confidence**: HIGH

### Fact 2: Types are defined in schema.yml (lines 311-356)
- **Source**: `cmd/accumulated/run/schema.yml:311-356`
- **Content**:
  - `ConsensusService` is defined as a union member of `Service` (line 311)
  - `CoreConsensusApp` is defined as a union member of `ConsensusApp` (line 342)
  - Both are used for JSON marshaling/unmarshaling of configuration files
- **Confidence**: HIGH

### Fact 3: Types are used in instance_test.go
- **Source**: `cmd/accumulated/run/instance_test.go:122-143`
- **Content**:
  ```go
  &ConsensusService{
      NodeDir: "node-1/dnn",
      Genesis: "node-1/dn-genesis.snap",
      App: &CoreConsensusApp{
          EnableHealing: Ptr(true),
          Partition: &protocol.PartitionInfo{
              ID:   protocol.Directory,
              Type: protocol.PartitionTypeDirectory,
          },
      },
  },
  ```
- **Confidence**: HIGH

### Fact 4: consensus.go implements ConsensusService methods
- **Source**: `cmd/accumulated/run/consensus.go:63-601`
- **Content**:
  - IOC providers for ConsensusService: `consensusProvidesEventBus`, `consensusProvidesService`, etc. (lines 63-66)
  - IOC providers for CoreConsensusApp: `coreConsensusNeedsStorage`, `coreConsensusProvidesSequencer`, etc. (lines 68-71)
  - Method implementations: `ConsensusService.Requires()`, `ConsensusService.Provides()`, `ConsensusService.start()`, etc.
  - CoreConsensusApp implementations: `partition()`, `Requires()`, `Provides()`, `prestart()`, `start()`, `register()`
- **Confidence**: HIGH

### Fact 5: DAGBFTService exists as an alternative
- **Source**: `cmd/accumulated/run/dagbft.go:55-76`
- **Content**:
  ```go
  type DAGBFTService struct {
      NodeDir      string
      ValidatorKey PrivateKey
      Genesis      string
      Partition    *protocol.PartitionInfo
      // ... DAG-BFT specific configuration
  }
  ```
  - DAGBFTService is intended to replace ConsensusService for DAG-BFT consensus
  - It implements the same Service interface
- **Confidence**: HIGH

### Fact 6: Generated code in types_gen.go and schema_gen.go
- **Source**: `cmd/accumulated/run/types_gen.go:265-324` and `cmd/accumulated/run/schema_gen.go:304-343`
- **Content**:
  - `ConsensusService` struct with fields: NodeDir, ValidatorKey, Genesis, Listen, BootstrapPeers, MetricsNamespace, App
  - `CoreConsensusApp` struct with fields: Partition, EnableHealing, EnableDirectDispatch, MaxEnvelopesPerBlock
  - Both have Copy(), Equal(), MarshalJSON(), UnmarshalJSON() methods
  - Schema methods: `sConsensusService`, `sCoreConsensusApp`
- **Confidence**: HIGH

### Fact 7: The project plan documents Phase 3 dependency
- **Source**: `docs/plans/accumulated-dagbft.md:93-113`
- **Content**:
  - Phase 3 "Create DAG-BFT-Only Run Package" includes "Split types.go - Move ConsensusApp interface to comet-only file"
  - This requires build tags or separate packages to properly decouple CometBFT code
- **Confidence**: HIGH

## Code References

### Primary Implementation Files
| File | Function | Description |
|------|----------|-------------|
| `cmd/accumulated/run/schema.yml:311-356` | N/A | Type definitions for ConsensusService and CoreConsensusApp |
| `cmd/accumulated/run/types_gen.go:265-324` | Generated types | Struct definitions and serialization methods |
| `cmd/accumulated/run/schema_gen.go:31-32, 304-343` | Generated schema | Schema methods for ConsensusService and CoreConsensusApp |
| `cmd/accumulated/run/consensus.go:63-601` | Multiple | Service implementation methods |
| `cmd/accumulated/run/core_validator.go:129-147` | `partOpts.apply()` | Creates ConsensusService with CoreConsensusApp |
| `cmd/accumulated/run/types.go:55-63` | `ConsensusApp` interface | Interface that CoreConsensusApp implements |
| `cmd/accumulated/run/instance_test.go:122-143` | `TestRun()` | Test using ConsensusService and CoreConsensusApp |

### Usage Sites (cmd/accumulated/run only)
- `core_validator.go:130` - Creates ConsensusService
- `core_validator.go:137` - Creates CoreConsensusApp
- `core_validator.go:147` - References ConsensusService in lambda
- `consensus.go:63` - consensusProvidesEventBus references ConsensusService
- `consensus.go:68-71` - IOC providers reference CoreConsensusApp
- `consensus.go:83` - var _ prestarter = (*ConsensusService)(nil)
- `consensus.go:85-110` - ConsensusService methods
- `consensus.go:264` - ConsensusService.loadPrivVal
- `consensus.go:316` - ConsensusService.genesisDocProvider
- `consensus.go:392-533` - CoreConsensusApp methods
- `instance_test.go:122-143` - Test creates ConsensusService with CoreConsensusApp

## Dependencies for Removal

To safely remove ConsensusService and CoreConsensusApp, the following must happen first:

1. **Update core_validator.go** - Replace ConsensusService/CoreConsensusApp usage with DAGBFTService
2. **Update instance_test.go** - Update test to use DAGBFTService or add build tags
3. **Update schema.yml** - Remove ConsensusService and CoreConsensusApp definitions
4. **Regenerate types_gen.go and schema_gen.go** - Run code generators
5. **Update consensus.go** - Remove or guard ConsensusService/CoreConsensusApp implementations with build tags
6. **Update types.go** - Remove ConsensusApp interface or guard with build tags

## Open Questions

1. **Should removal use build tags?** - The plan (docs/plans/accumulated-dagbft.md) suggests using build tags to allow both CometBFT and DAG-BFT builds. This would mean keeping the types but only compiling them for CometBFT builds.

2. **Is core_validator.go being deprecated entirely?** - If CoreValidatorConfiguration is only for CometBFT consensus, it may need build tags rather than removal.

3. **What about backwards compatibility?** - Existing configuration files may reference ConsensusService. How will migration be handled?

## Contradictions

No contradictions found. The issue title states "after core_validator.go is updated" but core_validator.go has not been updated yet. This is a sequencing issue, not a contradiction in the codebase.

## Recommendation

This issue cannot be completed until the prerequisite work is done:
1. **First**: Update `core_validator.go` to use `DAGBFTService` instead of `ConsensusService/CoreConsensusApp`
2. **Then**: Remove the type registrations from `schema.yml`
3. **Finally**: Regenerate and clean up related code

Alternatively, if the goal is to support both CometBFT and DAG-BFT:
1. Add build tags to `consensus.go` and related files
2. Keep `ConsensusService/CoreConsensusApp` for CometBFT builds only
3. Update `core_validator.go` to be CometBFT-only with build tags
