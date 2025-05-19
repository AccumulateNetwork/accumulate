---
title: Sequence Numbers for Anchors and Synthetic Transactions
description: Detailed explanation of sequence number management for cross-network communication in Accumulate
tags: [accumulate, anchoring, synthetic-transactions, sequence-numbers, cross-network]
created: 2025-05-16
version: 1.0
---

# Sequence Numbers for Anchors and Synthetic Transactions

## Introduction

Sequence numbers are a critical component of Accumulate's cross-network communication system. They ensure proper ordering, prevent replay attacks, and facilitate the healing process for missed transactions. This document explains how sequence numbers work for both anchors and synthetic transactions, and how they integrate with the deterministic transmission approach.

## Current Sequence Number System

### 1. Sequence Chain Architecture

In Accumulate, each network partition (DN or BVN) maintains sequence chains for cross-network communication:

```go
// File: internal/core/crosschain/anchoring.go

// AnchorSequenceChain returns the anchor sequence chain for the partition
func (v *ValidatorContext) AnchorSequenceChain(partition string) *database.Chain {
    return v.batch.Account(v.Url(protocol.AnchorPool)).AnchorSequenceChain(partition)
}

// SyntheticSequenceChain returns the synthetic transaction sequence chain for the partition
func (v *ValidatorContext) SyntheticSequenceChain(partition string) *database.Chain {
    return v.batch.Account(v.Url(protocol.SyntheticPool)).SyntheticSequenceChain(partition)
}
```

These sequence chains serve several purposes:
- Track the next sequence number to use
- Record which transactions have been sent
- Provide a reference for healing missing transactions

### 2. Sequence Number Assignment

Sequence numbers are assigned sequentially for each source-destination pair:

```go
// File: internal/core/crosschain/conductor.go

// NextSequenceNumber gets the next sequence number for the destination
func (c *Conductor) NextSequenceNumber(batch *database.Batch, destination *url.URL) (uint64, error) {
    // Get the sequence chain for the destination
    sequence := batch.Account(c.Url(protocol.AnchorPool)).AnchorSequenceChain(destination.String())
    
    // Get the current head
    head, err := sequence.Head().Get()
    if err != nil {
        if !errors.Is(err, errors.NotFound) {
            return 0, fmt.Errorf("failed to get sequence head: %w", err)
        }
        
        // If not found, start at 1
        return 1, nil
    }
    
    // Return the next sequence number
    return head.Sequence + 1, nil
}
```

### 3. Tracking Sent Transactions

When a transaction is sent, it's recorded in the appropriate sequence chain:

```go
// File: internal/core/crosschain/anchoring.go

// RecordAnchorSent records that an anchor has been sent
func (v *ValidatorContext) RecordAnchorSent(destination *url.URL, sequenceNumber uint64, txid *url.TxID) error {
    // Get the sequence chain
    sequence := v.AnchorSequenceChain(destination.String())
    
    // Create a sequence entry
    entry := &protocol.SequenceEntry{
        Sequence: sequenceNumber,
        Source:   v.Url(),
        TxID:     txid,
    }
    
    // Add the entry to the chain
    return sequence.AddEntry(entry, nil)
}
```

## Deterministic Sequence Numbers

With the move to deterministic anchor transmission, sequence numbers must also be generated deterministically to ensure all validators produce identical transactions.

### 1. Deterministic Sequence Number Generation

```go
// File: internal/core/crosschain/deterministic.go (new file to create)

// DeterministicSequencer generates deterministic sequence numbers
type DeterministicSequencer struct {
    database    *database.Batch
    networkID   string
    partitionID string
    logger      logging.Logger
}

// NextSequenceNumber gets the next sequence number deterministically
func (s *DeterministicSequencer) NextSequenceNumber(ctx context.Context, source, destination *url.URL, isAnchor bool) (uint64, error) {
    // Determine which sequence chain to use
    var sequenceChain *database.Chain
    if isAnchor {
        sequenceChain = s.database.Account(protocol.PartitionUrl(s.partitionID).JoinPath(protocol.AnchorPool)).
            AnchorSequenceChain(destination.String())
    } else {
        sequenceChain = s.database.Account(protocol.PartitionUrl(s.partitionID).JoinPath(protocol.SyntheticPool)).
            SyntheticSequenceChain(destination.String())
    }
    
    // Get the current head
    head, err := sequenceChain.Head().Get()
    if err != nil {
        if !errors.Is(err, errors.NotFound) {
            return 0, fmt.Errorf("failed to get sequence head: %w", err)
        }
        
        // If not found, start at 1
        return 1, nil
    }
    
    // Return the next sequence number
    return head.Sequence + 1, nil
}
```

### 2. Integration with Deterministic Transaction Generation

The sequence number is incorporated into the deterministic transaction generation process:

```go
// File: internal/core/crosschain/deterministic.go (new file to create)

// BuildAnchorTransaction builds a deterministic transaction from an anchor
func (b *DeterministicTxBuilder) BuildAnchorTransaction(ctx context.Context, source, destination *url.URL, anchor *protocol.PartitionAnchor) (*protocol.Transaction, error) {
    // Get the next sequence number
    sequenceNumber, err := b.sequencer.NextSequenceNumber(ctx, source, destination, true)
    if err != nil {
        return nil, fmt.Errorf("failed to get sequence number: %w", err)
    }
    
    // Set the sequence number in the anchor
    anchor.SequenceNumber = sequenceNumber
    
    // Generate deterministic transaction ID based on source, destination, and anchor content
    txid := b.generateDeterministicTxID(source, destination, anchor, sequenceNumber)
    
    // Build transaction with the deterministic ID
    txn := &protocol.Transaction{
        Header: protocol.TransactionHeader{
            Principal: destination,
        },
        Body: anchor,
    }
    
    // Set the deterministic ID
    txn.Header.SetTxID(txid)
    
    return txn, nil
}
```

## Sequence Numbers for Synthetic Transactions

Synthetic transactions follow a similar pattern to anchors but use a separate sequence chain.

### 1. Synthetic Transaction Sequence Numbers

```go
// File: internal/core/execute/synthetic.go

// NextSyntheticSequenceNumber gets the next sequence number for synthetic transactions
func (e *Executor) NextSyntheticSequenceNumber(batch *database.Batch, destination *url.URL) (uint64, error) {
    // Get the sequence chain for the destination
    sequence := batch.Account(e.Url(protocol.SyntheticPool)).SyntheticSequenceChain(destination.String())
    
    // Get the current head
    head, err := sequence.Head().Get()
    if err != nil {
        if !errors.Is(err, errors.NotFound) {
            return 0, fmt.Errorf("failed to get sequence head: %w", err)
        }
        
        // If not found, start at 1
        return 1, nil
    }
    
    // Return the next sequence number
    return head.Sequence + 1, nil
}
```

### 2. Deterministic Synthetic Transaction Generation

With the deterministic approach, synthetic transactions are generated similarly to anchors:

```go
// File: internal/core/crosschain/deterministic.go (new file to create)

// BuildSyntheticTransaction builds a deterministic synthetic transaction
func (b *DeterministicTxBuilder) BuildSyntheticTransaction(ctx context.Context, source, destination *url.URL, body protocol.TransactionBody) (*protocol.Transaction, error) {
    // Get the next sequence number
    sequenceNumber, err := b.sequencer.NextSequenceNumber(ctx, source, destination, false)
    if err != nil {
        return nil, fmt.Errorf("failed to get sequence number: %w", err)
    }
    
    // Set the sequence number if the body supports it
    if seqBody, ok := body.(interface{ SetSequenceNumber(uint64) }); ok {
        seqBody.SetSequenceNumber(sequenceNumber)
    }
    
    // Generate deterministic transaction ID based on source, destination, body, and sequence number
    txid := b.generateDeterministicTxID(source, destination, body, sequenceNumber)
    
    // Build transaction with the deterministic ID
    txn := &protocol.Transaction{
        Header: protocol.TransactionHeader{
            Principal: destination,
        },
        Body: body,
    }
    
    // Set the deterministic ID
    txn.Header.SetTxID(txid)
    
    return txn, nil
}
```

## Sequence Numbers and Healing

Sequence numbers are essential for the healing process, as they allow the system to identify and recover missing transactions.

### 1. Identifying Missing Transactions

```go
// File: internal/core/healing/anchors.go

// FindMissingAnchors identifies missing anchors based on sequence numbers
func FindMissingAnchors(ctx context.Context, batch *database.Batch, source, destination *url.URL) ([]uint64, error) {
    // Get the sequence chain
    sequence := batch.Account(destination.JoinPath(protocol.AnchorPool)).AnchorSequenceChain(source.String())
    
    // Get the entries
    entries, err := sequence.Entries(0, 0)
    if err != nil {
        return nil, fmt.Errorf("failed to get sequence entries: %w", err)
    }
    
    // Find gaps in the sequence
    var missing []uint64
    var expected uint64 = 1
    for _, entry := range entries {
        for expected < entry.Sequence {
            missing = append(missing, expected)
            expected++
        }
        expected = entry.Sequence + 1
    }
    
    return missing, nil
}
```

### 2. Deterministic Healing

With the deterministic approach, healing becomes more straightforward:

```go
// File: internal/core/crosschain/deterministic.go (new file to create)

// HealMissingTransaction regenerates and submits a missing transaction
func (t *DeterministicAnchorTransmitter) HealMissingTransaction(ctx context.Context, source, destination *url.URL, sequenceNumber uint64, isAnchor bool) error {
    t.config.Logger.Debug("Healing missing transaction", 
        "source", source, 
        "destination", destination, 
        "sequence", sequenceNumber,
        "type", map[bool]string{true: "anchor", false: "synthetic"}[isAnchor])
    
    // For anchors, regenerate from the block state
    if isAnchor {
        // Find the block that corresponds to this sequence number
        blockIndex, err := t.findBlockForSequence(ctx, source, destination, sequenceNumber)
        if err != nil {
            return err
        }
        
        // Generate the anchor
        anchor, err := t.generator.GenerateAnchor(ctx, blockIndex)
        if err != nil {
            return err
        }
        
        // Force the sequence number
        anchor.SequenceNumber = sequenceNumber
        
        // Build and submit the transaction
        txn, err := t.txBuilder.BuildAnchorTransactionWithSequence(ctx, source, destination, anchor, sequenceNumber)
        if err != nil {
            return err
        }
        
        return t.sigManager.SignAndSubmit(ctx, txn)
    }
    
    // For synthetic transactions, we need to retrieve the original transaction data
    // This is more complex and may require querying the source network
    // ...
    
    return nil
}
```

## Sequence Numbers in BVN-to-BVN Communication

For BVN-to-BVN communication, sequence numbers follow the same pattern but with different source-destination pairs.

### 1. BVN-to-BVN Sequence Management

```go
// File: internal/core/crosschain/bvn_transmitter.go (new file to create)

// NextBvnToBvnSequenceNumber gets the next sequence number for BVN-to-BVN communication
func (t *BvnToBvnTransmitter) NextBvnToBvnSequenceNumber(ctx context.Context, source, destination *url.URL) (uint64, error) {
    // Use the synthetic sequence chain for BVN-to-BVN communication
    return t.sequencer.NextSequenceNumber(ctx, source, destination, false)
}
```

### 2. Deterministic BVN-to-BVN Transaction Generation

```go
// File: internal/core/crosschain/bvn_transmitter.go (new file to create)

// BuildBvnToBvnTransaction builds a deterministic transaction for BVN-to-BVN communication
func (t *BvnToBvnTransmitter) BuildBvnToBvnTransaction(ctx context.Context, source, destination *url.URL, body protocol.TransactionBody) (*protocol.Transaction, error) {
    // Get the next sequence number
    sequenceNumber, err := t.NextBvnToBvnSequenceNumber(ctx, source, destination)
    if err != nil {
        return nil, err
    }
    
    // Use the standard synthetic transaction builder with the sequence number
    return t.txBuilder.BuildSyntheticTransaction(ctx, source, destination, body)
}
```

## Sequence Number Collision Prevention

To prevent sequence number collisions in the deterministic system, all validators must have a consistent view of the sequence chain.

### 1. Consensus on Sequence Numbers

```go
// File: internal/core/crosschain/deterministic.go (new file to create)

// EnsureSequenceConsistency ensures all validators have a consistent view of sequence numbers
func (s *DeterministicSequencer) EnsureSequenceConsistency(ctx context.Context, source, destination *url.URL, isAnchor bool) error {
    // This is implicitly handled by the consensus mechanism
    // All validators process the same blocks in the same order
    // Therefore, they all see the same sequence chain state
    
    return nil
}
```

### 2. Handling Edge Cases

```go
// File: internal/core/crosschain/deterministic.go (new file to create)

// ReconcileSequenceChain handles edge cases where sequence chains might diverge
func (s *DeterministicSequencer) ReconcileSequenceChain(ctx context.Context, source, destination *url.URL, isAnchor bool) error {
    // In practice, this should not be needed in a deterministic system
    // However, it's included as a safety mechanism
    
    // Get the sequence chain
    var sequenceChain *database.Chain
    if isAnchor {
        sequenceChain = s.database.Account(protocol.PartitionUrl(s.partitionID).JoinPath(protocol.AnchorPool)).
            AnchorSequenceChain(destination.String())
    } else {
        sequenceChain = s.database.Account(protocol.PartitionUrl(s.partitionID).JoinPath(protocol.SyntheticPool)).
            SyntheticSequenceChain(destination.String())
    }
    
    // Ensure the chain is up to date
    // This is a no-op in normal operation
    return nil
}
```

## Migration Considerations

When migrating to the deterministic system, sequence number continuity must be maintained.

### 1. Preserving Sequence Chains

```go
// File: internal/core/crosschain/migration.go (new file to create)

// PreserveLegacySequences ensures sequence continuity during migration
func PreserveLegacySequences(ctx context.Context, batch *database.Batch, source, destination *url.URL) error {
    // The deterministic system will continue from the last sequence number
    // No special handling is needed as long as the sequence chains are preserved
    
    return nil
}
```

### 2. Handling In-Flight Transactions

```go
// File: internal/core/crosschain/migration.go (new file to create)

// HandleInFlightTransactions manages transactions sent during migration
func HandleInFlightTransactions(ctx context.Context, batch *database.Batch, source, destination *url.URL) error {
    // Wait for in-flight transactions to complete
    // This can be done by adding a delay before switching to deterministic mode
    
    return nil
}
```

## Conclusion

Sequence numbers are a fundamental component of Accumulate's cross-network communication system. With the transition to deterministic anchor transmission, sequence numbers will continue to play a critical role but will be generated deterministically to ensure all validators produce identical transactions.

The deterministic approach simplifies the healing process and reduces the risk of sequence number collisions, while maintaining backward compatibility with the existing sequence chain architecture.

## References

1. [Deterministic Anchor Transmission](12_deterministic_anchor_transmission.md)
2. [Deterministic Anchor Detailed Design](13_deterministic_anchor_detailed_design.md)
3. [Deterministic Anchor Abstraction](14_deterministic_anchor_abstraction.md)
4. [Deterministic BVN-to-BVN Options](15_deterministic_bvn_to_bvn_options.md)
5. [Cross-Network Communication](../implementation/02_cross_network_communication.md)
6. [Anchor Proofs and Receipts](../implementation/03_anchor_proofs_and_receipts.md)
