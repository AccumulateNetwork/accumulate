---
title: Snapshots in Accumulate - Overview
description: A comprehensive overview of the snapshot system in Accumulate
tags: [accumulate, snapshots, state-synchronization, cometbft]
created: 2025-05-16
version: 1.0
---

# Snapshots in Accumulate - Overview

## 1. Introduction

Snapshots in Accumulate provide a mechanism for capturing, storing, and restoring the state of the blockchain. They are essential for state synchronization between nodes, fast bootstrapping of new nodes, and disaster recovery. This document provides a comprehensive overview of the snapshot system in Accumulate, including its architecture, components, and usage.

## 2. Key Concepts

### 2.1 What is a Snapshot?

A snapshot in Accumulate is a point-in-time capture of the blockchain state, including:

- Account data (stored in the Binary Prefix Tree)
- Transaction data
- Signature data
- Chain data (including Merkle trees and mark points)
- System ledger information
- Consensus state (in v2 snapshots)

Snapshots are stored as structured files with a specific format that allows for efficient storage, retrieval, and validation.

### 2.2 Snapshot Versions

Accumulate supports two snapshot versions:

1. **Version 1 (Legacy)**: The original snapshot format, which includes basic blockchain state.
2. **Version 2 (Current)**: An enhanced format that adds support for consensus state and other improvements.

### 2.3 Snapshot Components

A snapshot consists of several sections:

1. **Header**: Contains metadata about the snapshot, including version, timestamp, and root hash.
2. **Accounts**: Contains the state of all accounts in the system.
3. **Transactions**: Contains transaction data referenced by accounts.
4. **Signatures**: Contains signature data referenced by transactions.
5. **Consensus**: (Version 2 only) Contains CometBFT consensus state.

## 3. Snapshot Architecture

### 3.1 System Components

The snapshot system in Accumulate consists of several key components:

1. **Snapshot Collection**: Responsible for capturing and storing snapshots.
2. **Snapshot Restoration**: Responsible for loading and applying snapshots.
3. **CometBFT Integration**: Provides integration with CometBFT's state sync protocol.
4. **Snapshot Service**: Exposes snapshot functionality via API.

### 3.2 Data Flow

The snapshot data flow follows this pattern:

1. **Collection**: State is collected from the database and written to a snapshot file.
2. **Storage**: Snapshots are stored in a designated directory.
3. **Distribution**: Snapshots can be shared between nodes via CometBFT's state sync protocol.
4. **Restoration**: Snapshots can be loaded to restore a node's state.

### 3.3 Integration with CometBFT

Accumulate integrates with CometBFT's state sync protocol through four ABCI functions:

1. **ListSnapshots**: Returns a list of available snapshots.
2. **LoadSnapshotChunk**: Loads a chunk of a snapshot for distribution.
3. **OfferSnapshot**: Receives a snapshot offer from another node.
4. **ApplySnapshotChunk**: Applies a chunk of a received snapshot.

## 4. Snapshot Implementation

### 4.1 Snapshot Collection

Snapshots are collected by the `snapshotCollector` component, which:

1. Monitors block commits via the event system.
2. Determines when to create a snapshot based on a schedule or manual trigger.
3. Collects state from the database.
4. Writes the state to a snapshot file.

```go
func (c *snapshotCollector) collect(batch *coredb.Batch, blockTime time.Time, majorBlock, minorBlock uint64) {
    if !c.isTimeForSnapshot(blockTime) {
        return
    }
    
    // Collection logic...
}
```

### 4.2 Snapshot Storage

Snapshots are stored in a designated directory with filenames based on block height:

```
<snapshot-dir>/snapshot-<height>.acc
```

The snapshot file format is a structured binary format with sections for different types of data.

### 4.3 Snapshot Restoration

Snapshots are restored by the `RestoreVisitor` component, which:

1. Reads the snapshot file.
2. Restores transactions and signatures first.
3. Restores accounts and their chains.
4. Rebuilds indices as needed.

```go
func (v *RestoreVisitor) VisitAccount(acct *Account, i int) error {
    // Restoration logic...
}
```

## 5. User Experience

### 5.1 Snapshot Configuration

Snapshots can be configured through the node configuration:

```yaml
snapshots:
  directory: data/snapshots
  schedule: "0 0 * * *"  # Daily at midnight
  retain-count: 4        # Keep 4 snapshots
  enable-indexing: true  # Enable snapshot indexing
```

### 5.2 Manual Snapshot Triggering

Snapshots can be triggered manually by creating a `.capture` file in the snapshot directory:

```bash
touch data/snapshots/.capture
```

### 5.3 Snapshot Listing

Users can list available snapshots via the API:

```bash
curl -X GET "http://localhost:26660/v3/snapshots"
```

## 6. Conclusion

The snapshot system in Accumulate provides a robust mechanism for capturing, storing, and restoring blockchain state. It is essential for state synchronization, fast bootstrapping, and disaster recovery. Understanding the snapshot system is crucial for operating and maintaining Accumulate nodes effectively.

In the following documents, we will explore each component of the snapshot system in more detail, including the collection process, file format, restoration process, and CometBFT integration.
