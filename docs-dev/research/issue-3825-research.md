# Research: Enable DAG-BFT in core_validator.go

## Summary

This research documents the changes required to replace `ConsensusService` with `DAGBFTService` in `core_validator.go`. The `DAGBFTService` is already fully implemented in `dagbft.go` and provides all necessary IOC provisions. The change involves updating the `partOpts.apply()` method to instantiate `DAGBFTService` instead of `ConsensusService`, mapping the existing configuration fields appropriately. Three fields from `ConsensusService` (`Listen`, `BootstrapPeers`, `MetricsNamespace`) are CometBFT-specific and should be omitted for DAG-BFT.

## Verified Facts

### Fact 1: ConsensusService is currently created in partOpts.apply()
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

### Fact 2: DAGBFTService struct fields
- **Source**: `cmd/accumulated/run/dagbft.go:55-76`
- **Content**:
```go
type DAGBFTService struct {
    // Configuration
    NodeDir      string         `json:"nodeDir,omitempty" ...`
    ValidatorKey PrivateKey     `json:"validatorKey,omitempty" ...`
    Genesis      string         `json:"genesis,omitempty" ...`
    Partition    *protocol.PartitionInfo `json:"partition,omitempty" ...`

    // DAG-BFT specific configuration
    NumWorkers       *int `json:"numWorkers,omitempty" ...`
    DAGGCDepth       *int `json:"dagGCDepth,omitempty" ...`
    CommitBufferSize *int `json:"commitBufferSize,omitempty" ...`

    // Executor options
    EnableHealing        *bool `json:"enableHealing,omitempty" ...`
    EnableDirectDispatch *bool `json:"enableDirectDispatch,omitempty" ...`
    MaxEnvelopesPerBlock *uint `json:"maxEnvelopesPerBlock,omitempty" ...`

    // Runtime state (transient)
    service  *dagbft.Service
    eventBus *events.Bus
    globals  chan *network.GlobalValues
}
```
- **Confidence**: HIGH

### Fact 3: DAGBFTService provides equivalent IOC services
- **Source**: `cmd/accumulated/run/dagbft.go:42-51`
- **Content**:
```go
var (
    dagbftProvidesEventBus  = ioc.Provides[*events.Bus](func(s *DAGBFTService) string { return s.Partition.ID })
    dagbftProvidesService   = ioc.Provides[v3.ConsensusService](func(s *DAGBFTService) string { return s.Partition.ID })
    dagbftProvidesSubmitter = ioc.Provides[v3.Submitter](func(s *DAGBFTService) string { return s.Partition.ID })
    dagbftProvidesValidator = ioc.Provides[v3.Validator](func(s *DAGBFTService) string { return s.Partition.ID })
    dagbftProvidesSequencer = ioc.Provides[private.Sequencer](func(s *DAGBFTService) string { return s.Partition.ID })
    dagbftProvidesRouter    = ioc.Provides[routing.Router](func(s *DAGBFTService) string { return s.Partition.ID })

    dagbftNeedsStorage = ioc.Needs[keyvalue.Beginner](func(s *DAGBFTService) string { return s.Partition.ID })
)
```
- **Confidence**: HIGH

### Fact 4: partOpts struct has the required fields for DAGBFTService
- **Source**: `cmd/accumulated/run/core_validator.go:108-116`
- **Content**:
```go
type partOpts struct {
    *CoreValidatorConfiguration
    ID               string
    Type             protocol.PartitionType
    Genesis          string
    Dir              string
    BootstrapPeers   []multiaddr.Multiaddr  // CometBFT-specific
    MetricsNamespace string                  // CometBFT-specific
}
```
- **Confidence**: HIGH

### Fact 5: CoreValidatorConfiguration has the executor options
- **Source**: `cmd/accumulated/run/types_gen.go:326-340`
- **Content**:
```go
type CoreValidatorConfiguration struct {
    Mode                 CoreValidatorMode
    Listen               Multiaddr
    BVN                  string
    ValidatorKey         PrivateKey
    DnGenesis            string
    BvnGenesis           string
    DnBootstrapPeers     []Multiaddr
    BvnBootstrapPeers    []Multiaddr
    EnableHealing        *bool
    EnableDirectDispatch *bool
    EnableSnapshots      *bool
    MaxEnvelopesPerBlock *uint64   // NOTE: uint64, not uint
    StorageType          *StorageType
}
```
- **Confidence**: HIGH

### Fact 8: Type mismatch for MaxEnvelopesPerBlock
- **Source**: `cmd/accumulated/run/types_gen.go:338` and `cmd/accumulated/run/dagbft.go:70`
- **Content**:
  - `CoreValidatorConfiguration.MaxEnvelopesPerBlock` is `*uint64`
  - `DAGBFTService.MaxEnvelopesPerBlock` is `*uint`
  - This type mismatch requires a conversion when assigning the field
- **Confidence**: HIGH

### Fact 6: DAGBFTService uses libp2p for networking, not CometBFT P2P
- **Source**: `cmd/accumulated/run/dagbft.go:265-296`
- **Content**:
```go
// Create GossipSub for DAG-BFT certificate/batch dissemination
// This enables multi-node consensus networking via libp2p
var ps *pubsub.PubSub
if inst.p2p != nil {
    h := inst.p2p.Host()
    if h != nil {
        ps, err = pubsub.NewGossipSub(inst.context, h,
            pubsub.WithPeerExchange(true),
            pubsub.WithFloodPublish(true),
        )
        ...
    }
}
```
- **Confidence**: HIGH

### Fact 7: DAGBFTService has its own service type but uses ServiceTypeConsensus
- **Source**: `cmd/accumulated/run/dagbft.go:79-82`
- **Content**:
```go
func (s *DAGBFTService) Type() ServiceType {
    // Use a new service type for DAG-BFT
    return ServiceTypeConsensus
}
```
- **Confidence**: HIGH

## Code References

### Primary Implementation Files
| File | Purpose |
|------|---------|
| `cmd/accumulated/run/core_validator.go` | Target file - creates partition services |
| `cmd/accumulated/run/dagbft.go` | DAGBFTService implementation |
| `cmd/accumulated/run/consensus.go` | ConsensusService implementation (CometBFT) |
| `cmd/accumulated/run/types.go` | Service interfaces |
| `internal/node/dagbft/service.go` | Internal DAG-BFT service |

### Key Functions
| Function | Location | Purpose |
|----------|----------|---------|
| `partOpts.apply()` | `core_validator.go:118-177` | Creates partition services |
| `DAGBFTService.start()` | `dagbft.go:121-328` | Starts DAG-BFT consensus |
| `ConsensusService.start()` | `consensus.go:110-250` | Starts CometBFT consensus |

## Field Mapping

| ConsensusService Field | DAGBFTService Field | Notes |
|------------------------|---------------------|-------|
| `NodeDir` | `NodeDir` | Direct mapping |
| `ValidatorKey` | `ValidatorKey` | Direct mapping |
| `Genesis` | `Genesis` | Direct mapping |
| `Listen` | (omit) | CometBFT-specific, libp2p uses P2P config |
| `BootstrapPeers` | (omit) | CometBFT-specific, libp2p uses P2P config |
| `MetricsNamespace` | (omit) | CometBFT-specific |
| `App.Partition` | `Partition` | Direct mapping |
| `App.EnableHealing` | `EnableHealing` | Direct mapping |
| `App.EnableDirectDispatch` | `EnableDirectDispatch` | Direct mapping |
| `App.MaxEnvelopesPerBlock` | `MaxEnvelopesPerBlock` | **Type mismatch**: `*uint64` → `*uint`, requires conversion |

## Required Change

Replace lines 129-147 of `core_validator.go` with:

```go
// Convert MaxEnvelopesPerBlock from *uint64 to *uint
var maxEnvelopes *uint
if p.MaxEnvelopesPerBlock != nil {
    v := uint(*p.MaxEnvelopesPerBlock)
    maxEnvelopes = &v
}

addService(cfg,
    &DAGBFTService{
        NodeDir:              p.Dir,
        ValidatorKey:         p.ValidatorKey,
        Genesis:              p.Genesis,
        Partition:            &protocol.PartitionInfo{
            ID:   p.ID,
            Type: p.Type,
        },
        EnableHealing:        p.EnableHealing,
        EnableDirectDispatch: p.EnableDirectDispatch,
        MaxEnvelopesPerBlock: maxEnvelopes,
    },
    func(s *DAGBFTService) string { return s.Partition.ID })
```

**Note**: The type conversion is required because `CoreValidatorConfiguration.MaxEnvelopesPerBlock` is `*uint64` but `DAGBFTService.MaxEnvelopesPerBlock` is `*uint`. An alternative fix is to update `DAGBFTService` to use `*uint64` for consistency with the schema.

## Open Questions

1. **DAG-BFT specific configuration**: Should `NumWorkers`, `DAGGCDepth`, and `CommitBufferSize` be exposed through `CoreValidatorConfiguration`? Currently they use defaults set in `dagbft.go:126-128`.

2. **Metrics**: DAGBFTService doesn't have a `MetricsNamespace` field. Should DAG-BFT metrics be exposed? (Currently using defaults)

## Contradictions

### Type Mismatch in MaxEnvelopesPerBlock
- **CoreValidatorConfiguration** (schema-generated): `MaxEnvelopesPerBlock *uint64`
- **DAGBFTService** (manually defined): `MaxEnvelopesPerBlock *uint`
- **Resolution options**:
  1. Convert the value in `core_validator.go` when creating `DAGBFTService`
  2. Update `DAGBFTService` to use `*uint64` to match the schema

Otherwise, the DAGBFTService is designed as a drop-in replacement for ConsensusService and provides all the same IOC provisions.

## Success Criteria Verification

From the issue requirements:
- [x] `core_validator.go` will create `DAGBFTService` for each partition (via `partOpts.apply()`)
- [x] Build should succeed (DAGBFTService is fully implemented)
- [x] No references to `ConsensusService` in core_validator.go (after change)
