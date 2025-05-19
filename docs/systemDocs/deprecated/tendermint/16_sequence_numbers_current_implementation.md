---
title: Sequence Numbers in Accumulate - Current Implementation
description: Analysis of how sequence numbers are currently implemented for anchors and synthetic transactions, with focus on out-of-order transaction handling
tags: [accumulate, anchoring, synthetic-transactions, sequence-numbers, cross-network]
created: 2025-05-16
version: 1.0
---

# Sequence Numbers in Accumulate - Current Implementation

## Introduction

This document analyzes how sequence numbers are currently implemented in Accumulate for anchors and synthetic transactions. It focuses particularly on how the system handles out-of-order transactions and maintains consistency across network partitions.

## Sequence Number Architecture

### Sequence Chains

In Accumulate, each network partition (DN or BVN) maintains dedicated sequence chains for cross-network communication:

1. **Anchor Sequence Chains**: Track anchors sent to other partitions
2. **Synthetic Sequence Chains**: Track synthetic transactions sent to other partitions

These chains are maintained per destination, allowing each source-destination pair to have its own sequence numbering.

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

### Sequence Number Assignment

Sequence numbers are assigned sequentially for each source-destination pair. The current implementation gets the next sequence number by retrieving the head of the sequence chain and incrementing it:

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

### Recording Sent Transactions

When a transaction is sent, it's recorded in the appropriate sequence chain with its sequence number:

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

## Handling Out-of-Order Transactions

### Current Approach

Accumulate's current implementation handles out-of-order transactions through a combination of:

1. **Sequence Numbers**: Each transaction carries a sequence number
2. **Transaction Processing**: The executor checks sequence numbers during processing
3. **Healing Mechanism**: A separate process detects and resubmits missing transactions

### Transaction Processing Logic

When a synthetic transaction or anchor is received, the system checks its sequence number against the expected next sequence number:

```go
// File: internal/core/execute/v2/block/msg_synthetic.go (conceptual implementation)

func processSyntheticMessage(batch *database.Batch, msg *messaging.SyntheticMessage) error {
    // Get the source and destination
    source := msg.Source()
    destination := msg.Destination()
    
    // Get the sequence chain
    sequence := batch.Account(destination.JoinPath(protocol.SyntheticPool)).
        SyntheticSequenceChain(source.String())
    
    // Get the expected sequence number
    head, err := sequence.Head().Get()
    if err != nil && !errors.Is(err, errors.NotFound) {
        return err
    }
    
    expectedSeq := uint64(1)
    if head != nil {
        expectedSeq = head.Sequence + 1
    }
    
    // Check the sequence number
    if msg.SequenceNumber < expectedSeq {
        // This is a duplicate, ignore it
        return nil
    }
    
    if msg.SequenceNumber > expectedSeq {
        // This is out of order, queue it for later processing
        // In practice, the transaction is rejected and will be resubmitted by the healing process
        return errors.OutOfOrder.WithFormat(
            "expected sequence number %d, got %d", expectedSeq, msg.SequenceNumber)
    }
    
    // Process the transaction normally
    // ...
    
    // Record the sequence number
    entry := &protocol.SequenceEntry{
        Sequence: msg.SequenceNumber,
        Source:   source,
        TxID:     msg.ID(),
    }
    return sequence.AddEntry(entry, nil)
}
```

### Queuing Mechanism

The current Accumulate implementation does not maintain an explicit queue for out-of-order transactions. Instead, it relies on the healing mechanism to resubmit missing transactions. When a transaction with a higher-than-expected sequence number is received, it is rejected, and the healing process will eventually resubmit it in the correct order.

## Healing Mechanism

The healing mechanism is responsible for detecting and resubmitting missing transactions. It's a critical component for handling out-of-order transactions.

### Detecting Missing Transactions

The healing process scans sequence chains to identify gaps in sequence numbers:

```go
// File: internal/core/healing/anchors.go (conceptual implementation)

func FindMissingAnchors(ctx context.Context, batch *database.Batch, source, destination *url.URL) ([]uint64, error) {
    // Get the sequence chain
    sequence := batch.Account(destination.JoinPath(protocol.AnchorPool)).
        AnchorSequenceChain(source.String())
    
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

### Resubmitting Missing Transactions

When a missing transaction is identified, the healing process retrieves it from the source partition and resubmits it:

```go
// File: internal/core/healing/synthetic.go

func (h *Healer) HealSynthetic(ctx context.Context, args HealSyntheticArgs, si SequencedInfo) error {
    // Query the synthetic transaction
    r, err := ResolveSequenced[messaging.Message](ctx, args.Client, args.NetInfo, si.Source, si.Destination, si.Number, false)
    if err != nil {
        return err
    }
    si.ID = r.ID

    // Query the status
    Q := api.Querier2{Querier: args.Querier}
    if s, err := Q.QueryMessage(ctx, r.ID, nil); err == nil &&
        // Has it already been delivered?
        s.Status.Delivered() &&
        // Does the sequence info match?
        s.Sequence != nil &&
        s.Sequence.Source.Equal(protocol.PartitionUrl(si.Source)) &&
        s.Sequence.Destination.Equal(protocol.PartitionUrl(si.Destination)) &&
        s.Sequence.Number == si.Number {
        // If it's been delivered, skip it
        return errors.Delivered
    }

    // Build the receipt
    var receipt *merkle.Receipt
    if args.NetInfo.Status.ExecutorVersion.V2VandenbergEnabled() {
        receipt, err = h.buildSynthReceiptV2(ctx, args, si)
    } else {
        receipt, err = h.buildSynthReceiptV1(ctx, args, si)
    }
    if err != nil {
        return err
    }

    // Submit the synthetic transaction with proof
    msg := &messaging.SyntheticMessage{
        Message: r.Sequence,
        Proof: &protocol.AnnotatedReceipt{
            Receipt: receipt,
            Anchor: &protocol.AnchorMetadata{
                Account: protocol.DnUrl(),
            },
        },
    }
    
    // Add signature
    for _, sigs := range r.Signatures.Records {
        for _, sig := range sigs.Signatures.Records {
            sig, ok := sig.Message.(*messaging.SignatureMessage)
            if !ok {
                continue
            }
            ks, ok := sig.Signature.(protocol.KeySignature)
            if !ok {
                continue
            }
            msg.Signature = ks
        }
    }
    
    // Submit the message
    env := new(messaging.Envelope)
    env.Messages = []messaging.Message{msg}
    
    // Submit to the destination partition
    return args.Submitter.Submit(ctx, env, nil)
}
```

## Sequence Number Verification

When a transaction is received, its sequence number is verified as part of the validation process:

```go
// File: internal/core/execute/v2/chain/synthetic.go (conceptual implementation)

func verifySyntheticTransaction(batch *database.Batch, tx *protocol.Transaction) error {
    // Get the synthetic transaction
    synth, ok := tx.Body.(*protocol.SyntheticTransaction)
    if !ok {
        return errors.BadRequest.WithFormat("expected synthetic transaction, got %T", tx.Body)
    }
    
    // Get the source and destination
    source := synth.Source
    destination := tx.Header.Principal
    
    // Get the sequence chain
    sequence := batch.Account(destination.JoinPath(protocol.SyntheticPool)).
        SyntheticSequenceChain(source.String())
    
    // Get the expected sequence number
    head, err := sequence.Head().Get()
    if err != nil && !errors.Is(err, errors.NotFound) {
        return err
    }
    
    expectedSeq := uint64(1)
    if head != nil {
        expectedSeq = head.Sequence + 1
    }
    
    // Check the sequence number
    if synth.SequenceNumber != expectedSeq {
        return errors.BadRequest.WithFormat(
            "invalid sequence number: expected %d, got %d", expectedSeq, synth.SequenceNumber)
    }
    
    return nil
}
```

## Sequence Numbers in BVN-to-BVN Communication

For BVN-to-BVN communication, sequence numbers follow the same pattern but with different source-destination pairs:

```go
// File: internal/core/execute/synthetic.go (conceptual implementation)

func sendSyntheticTransaction(batch *database.Batch, source, destination *url.URL, body protocol.TransactionBody) error {
    // Get the sequence chain
    sequence := batch.Account(source.JoinPath(protocol.SyntheticPool)).
        SyntheticSequenceChain(destination.String())
    
    // Get the next sequence number
    head, err := sequence.Head().Get()
    if err != nil && !errors.Is(err, errors.NotFound) {
        return err
    }
    
    nextSeq := uint64(1)
    if head != nil {
        nextSeq = head.Sequence + 1
    }
    
    // Set the sequence number
    if seqBody, ok := body.(interface{ SetSequenceNumber(uint64) }); ok {
        seqBody.SetSequenceNumber(nextSeq)
    }
    
    // Build and submit the transaction
    // ...
    
    // Record the sequence
    entry := &protocol.SequenceEntry{
        Sequence: nextSeq,
        Source:   source,
        TxID:     txid,
    }
    return sequence.AddEntry(entry, nil)
}
```

## Limitations of the Current Approach

The current approach has several limitations:

1. **No Explicit Queuing**: Out-of-order transactions are rejected rather than queued
2. **Reliance on Healing**: The system relies on the healing mechanism to handle out-of-order transactions
3. **Potential Delays**: There can be delays between when a transaction is rejected and when it's resubmitted by the healing process
4. **Network Traffic**: Rejected transactions generate additional network traffic when they're resubmitted

## Conclusion

Accumulate's current sequence number implementation provides a robust mechanism for ensuring transaction ordering across network partitions. While it doesn't explicitly queue out-of-order transactions, the healing mechanism effectively handles these cases by detecting and resubmitting missing transactions.

The sequence chains provide a reliable record of which transactions have been sent and received, enabling the healing process to identify and recover from any inconsistencies. This approach ensures that even in the presence of network delays or failures, all transactions will eventually be processed in the correct order.

## References

1. [Cross-Network Communication](../implementation/02_cross_network_communication.md)
2. [Anchor Proofs and Receipts](../implementation/03_anchor_proofs_and_receipts.md)
3. [Deterministic Anchor Transmission](12_deterministic_anchor_transmission.md)
