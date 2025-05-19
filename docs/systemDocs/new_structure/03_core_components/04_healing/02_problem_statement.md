# Healing Problem Definition

## Introduction

In Accumulate's multi-chain architecture, maintaining data consistency across partitions is a critical challenge. This document defines the healing problem in Accumulate and outlines the approach to solving it.

## Multi-Chain Architecture

Accumulate uses a unique multi-chain architecture consisting of:

1. **Directory Network (DN)**: A single partition that maintains the global state and routing information
2. **Blockchain Validation Networks (BVNs)**: Multiple partitions that process transactions for specific accounts

This architecture provides scalability and performance benefits, but introduces the challenge of ensuring data consistency across partitions.

## The Healing Problem

### Problem Statement

In the normal operation of Accumulate, two types of data must be consistently synchronized across partitions:

1. **Synthetic Transactions**: Protocol-generated transactions that need to be delivered across partitions
2. **Anchors**: Cryptographic commitments that link blocks across different partitions

Due to network conditions, node failures, or other issues, these transactions or anchors may sometimes fail to be delivered, creating inconsistencies in the distributed ledger.

### Valid Partition Pairs

A key aspect of the healing problem is understanding which partition pairs are valid for data synchronization:

1. **Synthetic Transactions**: Can flow between any pair of partitions
2. **Anchors**: Can only flow between the Directory Network and BVNs (DN→BVN or BVN→DN)

Importantly, anchors do not flow directly between BVNs (no BVN→BVN anchors).

```go
// From internal/core/execute/v1/chain/anchor.go
// This code shows how anchors are only processed between DN and BVNs

func (x *Executor) ProcessAnchor(batch *database.Batch, envHash [32]byte, txn *protocol.Transaction) error {
    // ...
    
    // Verify the source and destination are valid
    if src.Type != protocol.PartitionTypeDirectory && dst.Type != protocol.PartitionTypeDirectory {
        return errors.BadRequest.WithFormat("anchor source or destination must be the directory")
    }
    
    // ...
}
```

### Challenges

The healing problem encompasses several specific challenges:

1. **Identifying Missing Data**: Determining which transactions or anchors are missing between partitions
2. **Efficient Discovery**: Using algorithms that can efficiently find missing data without excessive network traffic
3. **Versioning**: Handling different network protocol versions with appropriate healing strategies
4. **Validation**: Ensuring that healed transactions or anchors are cryptographically valid
5. **Monitoring**: Tracking the progress and success rate of healing operations

## Solution Approach

Accumulate's healing system addresses these challenges through:

1. **Binary Search Algorithm**: Efficiently identifies missing transactions by recursively dividing the search space
2. **Version-Specific Logic**: Implements different healing strategies based on the network protocol version
3. **Signature Collection**: For anchors, collects signatures from network nodes to ensure validity
4. **Detailed Reporting**: Provides comprehensive metrics and statistics about the healing process
5. **Partition Pair Tracking**: Monitors the status of each valid partition pair separately

## Implementation Location

The healing implementation is primarily located in two areas of the codebase:

1. **Core Implementation**: `internal/core/healing/` directory contains the core healing logic
   ```
   internal/core/healing/
   ├── anchors.go      # Anchor healing implementation
   ├── scan.go         # Scanning for missing data
   ├── sequenced.go    # Handling sequenced messages
   ├── synthetic.go    # Synthetic transaction healing
   ├── types.go        # Type definitions
   └── types_gen.go    # Generated type code
   ```

2. **Command-Line Interface**: `tools/cmd/debug/` directory contains the CLI implementation
   ```
   tools/cmd/debug/
   ├── heal_anchor.go  # Anchor healing CLI
   ├── heal_common.go  # Common healing infrastructure
   ├── heal_synth.go   # Synthetic healing CLI
   └── heal_test.go    # Testing code
   ```

## Related Components

The healing problem is related to several other components in Accumulate:

1. **Sequencing**: The process of assigning sequence numbers to transactions between partitions
   ```go
   // From internal/core/execute/v1/chain/sequence.go
   func (x *Executor) ProcessSequence(batch *database.Batch, envHash [32]byte, txn *protocol.Transaction) error {
       // ...
   }
   ```

2. **Anchoring**: The process of creating and validating cryptographic commitments between partitions
   ```go
   // From internal/core/execute/v1/chain/anchor.go
   func (x *Executor) ProcessDirectoryAnchor(batch *database.Batch, envHash [32]byte, txn *protocol.Transaction) error {
       // ...
   }
   ```

3. **Network Status**: Provides information about the network topology and state
   ```go
   // From internal/node/config/network.go
   func (n *Network) Status(ctx context.Context) (*protocol.NetworkStatus, error) {
       // ...
   }
   ```

## Conclusion

The healing problem in Accumulate is a critical aspect of maintaining data consistency in a multi-chain architecture. By understanding the problem and its solution approach, developers and AI systems can better work with the healing code and contribute to its improvement.
