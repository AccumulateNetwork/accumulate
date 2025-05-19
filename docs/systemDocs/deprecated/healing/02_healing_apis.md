# Healing APIs Overview

## Introduction

This document provides an overview of the healing APIs in Accumulate, their purpose, structure, and how they are implemented. The healing APIs are the primary interface for identifying and repairing missing transactions and anchors between partitions.

## API Structure

The healing APIs in Accumulate are primarily implemented in the `internal/core/healing` package and consist of two main components:

1. **Synthetic Transaction Healing API**
   - Primary function: `HealSynthetic`
   - Arguments structure: `HealSyntheticArgs`

2. **Anchor Healing API**
   - Primary function: `HealAnchor`
   - Arguments structure: `HealAnchorArgs`

## Core API Definitions

### Synthetic Healing API

```go
// From internal/core/healing/synthetic.go
type HealSyntheticArgs struct {
    Client      message.AddressedClient
    Querier     api.Querier
    Submitter   api.Submitter
    NetInfo     *NetworkInfo
    Light       *light.Client
    Pretend     bool
    Wait        bool
    SkipAnchors int
}

func (h *Healer) HealSynthetic(ctx context.Context, args HealSyntheticArgs, si SequencedInfo) error {
    // Implementation details
}
```

### Anchor Healing API

```go
// From internal/core/healing/anchors.go
type HealAnchorArgs struct {
    Client  message.AddressedClient
    Querier api.Querier
    Submit  func(...messaging.Message) error
    NetInfo *NetworkInfo
    Known   map[[32]byte]*protocol.Transaction
    Pretend bool
    Wait    bool
}

func HealAnchor(ctx context.Context, args HealAnchorArgs, si SequencedInfo) error {
    // Implementation details
}
```

### Common Structures

Both APIs share common structures for representing sequence information and network state:

```go
// From internal/core/healing/types.go
type SequencedInfo struct {
    Source      string
    Destination string
    Number      uint64
    ID          *url.TxID
}

type NetworkInfo struct {
    Status *protocol.NetworkStatus
    Peers  map[string]map[peer.ID]*network.PeerInfo
}
```

## Command-Line Interface

The healing APIs are exposed through a command-line interface in the `tools/cmd/debug` package:

```go
// From tools/cmd/debug/heal_synth.go
func healSynth(cmd *cobra.Command, args []string) {
    // Implementation details
}

// From tools/cmd/debug/heal_anchor.go
func healAnchor(cmd *cobra.Command, args []string) {
    // Implementation details
}
```

These CLI commands provide a user-friendly interface for running the healing processes:

```bash
# Synthetic healing
./tools/cmd/debug/debug heal synth mainnet --report-interval=1

# Anchor healing
./tools/cmd/debug/debug heal anchor mainnet --report-interval=1
```

## API Design Principles

The healing APIs are designed with several key principles in mind:

1. **Separation of Concerns**: The APIs separate the core healing logic from the CLI implementation, making it easier to test and maintain.

2. **Flexibility**: The APIs support various options (e.g., `Pretend`, `Wait`) to accommodate different use cases.

3. **Version Awareness**: The APIs include version-specific logic to handle different network protocol versions.

4. **Error Handling**: The APIs provide detailed error information to help diagnose and resolve issues.

## Integration with Other APIs

The healing APIs integrate with several other APIs in the Accumulate stack:

1. **Client API**: Used for communication with Accumulate nodes
   ```go
   Client message.AddressedClient
   ```

2. **Query API**: Used for querying transaction and anchor status
   ```go
   Querier api.Querier
   ```

3. **Submit API**: Used for submitting healed transactions and anchors
   ```go
   Submitter api.Submitter
   ```

4. **Light Client API**: Used for accessing chain data efficiently
   ```go
   Light *light.Client
   ```

## API Evolution

The healing APIs have evolved over time to accommodate changes in the Accumulate protocol:

1. **Pre-Baikonur**: The original implementation focused on basic healing functionality.

2. **Baikonur**: Added support for improved transaction formats and validation.

3. **Vandenberg**: Introduced specialized handling for DN-to-BVN anchors and enhanced receipt building.

This evolution is reflected in the version-specific logic throughout the code:

```go
// From internal/core/healing/anchors.go
func HealAnchor(ctx context.Context, args HealAnchorArgs, si SequencedInfo) error {
    if args.NetInfo.Status.ExecutorVersion.V2VandenbergEnabled() &&
        strings.EqualFold(si.Source, protocol.Directory) &&
        !strings.EqualFold(si.Destination, protocol.Directory) {
        return healDnAnchorV2(ctx, args, si)
    }
    return healAnchorV1(ctx, args, si)
}
```

## API Usage Examples

### Synthetic Healing Example

```go
// Create a healer instance
h := &healer{
    ctx:      ctx,
    C1:       client,
    C2:       client,
    net:      networkInfo,
    pairStats: make(map[string]*PartitionPairStats),
    fetcher:  newTxFetcher(client),
}

// Initialize partition pairs
// ...

// Run the healing process
h.heal(args)
```

### Anchor Healing Example

```go
// Create a healer instance
h := &healer{
    ctx:      ctx,
    C1:       client,
    C2:       client,
    net:      networkInfo,
    pairStats: make(map[string]*PartitionPairStats),
}

// Initialize partition pairs
// ...

// Run the healing process
h.heal(args)
```

## Conclusion

The healing APIs in Accumulate provide a robust interface for identifying and repairing missing transactions and anchors between partitions. By understanding these APIs, developers and AI systems can better work with the healing code and contribute to its improvement.

In the following documents, we will explore the implementation details of these APIs, including the synthetic transaction healing process, anchor healing process, database interactions, and more.
