---
title: Sequence Numbers in Accumulate - Processing Flow and Rejection Points
description: Detailed analysis of how sequence numbers are processed and where duplicate and out-of-order transactions are rejected in the current implementation
tags: [accumulate, anchoring, synthetic-transactions, sequence-numbers, cross-network, rejection]
created: 2025-05-16
version: 1.0
---

# Sequence Numbers in Accumulate - Processing Flow and Rejection Points

## Introduction

This document provides a detailed analysis of how sequence numbers are processed in Accumulate's current implementation, with a specific focus on identifying exactly where in the code duplicate and out-of-order transactions are rejected. Understanding these rejection points is crucial for implementing the deterministic anchor transmission system.

## Sequence Number Processing Flow

### 1. Message Reception and Initial Validation

When a sequenced message (anchor or synthetic transaction) is received, it first goes through the validation phase:

```go
// File: internal/core/execute/v2/block/msg_sequenced.go

func (x SequencedMessage) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
    // Check the wrapper
    seq, err := x.check(batch, ctx)
    if err != nil {
        return nil, errors.UnknownError.Wrap(err)
    }

    // Validate the inner message
    _, err = ctx.callMessageValidator(batch, seq.Message)
    return nil, errors.UnknownError.Wrap(err)
}
```

During this initial validation, the system checks that the message has all required fields, including a sequence number:

```go
// File: internal/core/execute/v2/block/msg_sequenced.go

func (x SequencedMessage) check(batch *database.Batch, ctx *MessageContext) (*messaging.SequencedMessage, error) {
    seq, ok := ctx.message.(*messaging.SequencedMessage)
    if !ok {
        return nil, errors.InternalError.WithFormat("invalid message type: expected %v, got %v", messaging.MessageTypeSequenced, ctx.message.Type())
    }

    // Basic validation
    if seq.Message == nil {
        return nil, errors.BadRequest.With("missing message")
    }

    var missing []string
    if seq.Source == nil {
        missing = append(missing, "source")
    }
    if seq.Destination == nil {
        missing = append(missing, "destination")
    }
    if seq.Number == 0 {
        missing = append(missing, "sequence number")
    }
    if len(missing) > 0 {
        return nil, errors.BadRequest.WithFormat("invalid synthetic transaction: missing %s", strings.Join(missing, ", "))
    }

    // Additional validation...
    return seq, nil
}
```

### 2. Sequence Number Verification

The critical sequence number verification happens in the `isReady` method, which is called during message processing:

```go
// File: internal/core/execute/v2/block/msg_sequenced.go

func (x SequencedMessage) isReady(batch *database.Batch, ctx *MessageContext, seq *messaging.SequencedMessage) (bool, error) {
    // Load the ledger
    isAnchor, ledger, err := x.loadLedger(batch, ctx, seq)
    if err != nil {
        return false, errors.UnknownError.Wrap(err)
    }
    partitionLedger := ledger.Partition(seq.Source)

    // If the sequence number is old, mark it already delivered
    typ := "synthetic message"
    if isAnchor {
        typ = "anchor"
    }
    if seq.Number <= partitionLedger.Delivered {
        return false, errors.Delivered.WithFormat("%s has been delivered", typ)
    }

    // If the transaction is out of sequence, mark it pending
    if partitionLedger.Delivered+1 != seq.Number {
        ctx.Executor.logger.Info("Out of sequence message",
            "hash", logging.AsHex(seq.Message.Hash()).Slice(0, 4),
            "seq-got", seq.Number,
            "seq-want", partitionLedger.Delivered+1,
            "source", seq.Source,
            "destination", seq.Destination,
            "type", typ,
            "hash", logging.AsHex(seq.Message.Hash()).Slice(0, 4),
        )
        return false, nil
    }

    return true, nil
}
```

This is the **first key rejection point**:
- If `seq.Number <= partitionLedger.Delivered`, the transaction is identified as a **duplicate** and marked as already delivered.
- If `partitionLedger.Delivered+1 != seq.Number`, the transaction is identified as **out of order** and marked as pending (not ready for processing).

### 3. Message Processing

Once a message passes validation, it moves to the processing phase:

```go
// File: internal/core/execute/v2/block/msg_sequenced.go

func (x SequencedMessage) Process(batch *database.Batch, ctx *MessageContext) (_ *protocol.TransactionStatus, err error) {
    batch = batch.Begin(true)
    defer func() { commitOrDiscard(batch, &err) }()

    // Check if the message has already been processed
    status, err := ctx.checkStatus(batch)
    if err != nil || status.Delivered() {
        return status, err
    }

    // Process the message
    seq, err := x.check(batch, ctx)
    var delivered bool
    if err == nil {
        delivered, err = x.process(batch, ctx, seq)
    }

    // Record status and return
    // ...
}
```

The actual processing happens in the `process` method:

```go
// File: internal/core/execute/v2/block/msg_sequenced.go

func (x SequencedMessage) process(batch *database.Batch, ctx *MessageContext, seq *messaging.SequencedMessage) (bool, error) {
    // Check if the message is ready to be processed
    ready, err := x.isReady(batch, ctx, seq)
    if err != nil {
        return false, errors.UnknownError.Wrap(err)
    }

    var st *protocol.TransactionStatus
    if ready {
        // Process the message within
        st, err = ctx.callMessageExecutor(batch, msg)
    } else {
        // Mark the message as pending
        ctx.Executor.logger.Debug("Pending sequenced message", "hash", logging.AsHex(seq.Message.Hash()).Slice(0, 4), "module", "synthetic")
        st, err = ctx.childWith(seq.Message).recordPending(batch)
    }
    // ...
}
```

This is the **second key rejection point**:
- If the message is not ready (as determined by `isReady`), it is marked as pending instead of being processed.

### 4. Ledger Update

After processing, the ledger is updated to reflect the new state:

```go
// File: internal/core/execute/v2/block/msg_sequenced.go

func (x SequencedMessage) updateLedger(batch *database.Batch, ctx *MessageContext, seq *messaging.SequencedMessage, pending bool) (*protocol.PartitionSyntheticLedger, error) {
    // Load the ledger
    isAnchor, ledger, err := x.loadLedger(batch, ctx, seq)
    if err != nil {
        return nil, errors.UnknownError.Wrap(err)
    }
    partLedger := ledger.Partition(seq.Source)

    // This should never happen, but if it does Add will panic
    if pending && seq.Number <= partLedger.Delivered {
        msg := "synthetic messages"
        if isAnchor {
            msg = "anchors"
        }
        return nil, errors.FatalError.WithFormat("%s processed out of order: delivered %d, processed %d", msg, partLedger.Delivered, seq.Number)
    }

    // The ledger's Delivered number needs to be updated if the transaction
    // succeeds or fails
    if partLedger.Add(!pending, seq.Number, seq.ID()) {
        err = batch.Account(ledger.GetUrl()).Main().Put(ledger)
        if err != nil {
            return nil, errors.UnknownError.WithFormat("store synthetic transaction ledger: %w", err)
        }
    }

    return partLedger, nil
}
```

This is the **third key rejection point**:
- There's a safety check that will cause a fatal error if a transaction with a sequence number less than or equal to the last delivered sequence number is somehow marked as pending.

### 5. Queuing Next Transaction

After a transaction is successfully processed, the system checks if the next transaction in the sequence is available and queues it for processing:

```go
// File: internal/core/execute/v2/block/msg_sequenced.go

func (x SequencedMessage) process(batch *database.Batch, ctx *MessageContext, seq *messaging.SequencedMessage) (bool, error) {
    // ... processing logic ...

    // Queue the next transaction in the sequence
    next, ok := ledger.Get(seq.Number + 1)
    if ok {
        ctx.queueAdditional(&internal.MessageIsReady{TxID: next})
    }

    return true, nil
}
```

This mechanism ensures that transactions are processed in sequence order, even if they are received out of order.

## Handling of Out-of-Order Transactions

### 1. Initial Reception

When a transaction is received with a sequence number higher than expected:
1. It passes initial validation (it has a valid sequence number)
2. It fails the `isReady` check because its sequence number is not the next expected one
3. It is marked as pending rather than being processed

```go
// File: internal/core/execute/v2/block/msg_sequenced.go

// If the transaction is out of sequence, mark it pending
if partitionLedger.Delivered+1 != seq.Number {
    ctx.Executor.logger.Info("Out of sequence message",
        "hash", logging.AsHex(seq.Message.Hash()).Slice(0, 4),
        "seq-got", seq.Number,
        "seq-want", partitionLedger.Delivered+1,
        "source", seq.Source,
        "destination", seq.Destination,
        "type", typ,
        "hash", logging.AsHex(seq.Message.Hash()).Slice(0, 4),
    )
    return false, nil
}
```

### 2. Storage in Pending State

Out-of-order transactions are stored in the system in a pending state:

```go
// Mark the message as pending
ctx.Executor.logger.Debug("Pending sequenced message", "hash", logging.AsHex(seq.Message.Hash()).Slice(0, 4), "module", "synthetic")
st, err = ctx.childWith(seq.Message).recordPending(batch)
```

### 3. Processing When Ready

When the preceding transaction in the sequence is processed, it triggers the processing of the next transaction:

```go
// Queue the next transaction in the sequence
next, ok := ledger.Get(seq.Number + 1)
if ok {
    ctx.queueAdditional(&internal.MessageIsReady{TxID: next})
}
```

This mechanism allows out-of-order transactions to be stored and processed in the correct order once the preceding transactions are available.

## Handling of Duplicate Transactions

### 1. Initial Check

When a transaction is received with a sequence number that has already been processed:

```go
// If the sequence number is old, mark it already delivered
if seq.Number <= partitionLedger.Delivered {
    return false, errors.Delivered.WithFormat("%s has been delivered", typ)
}
```

The transaction is immediately identified as already delivered and not processed further.

### 2. Status Check

Additionally, there's a status check at the beginning of processing:

```go
// Check if the message has already been processed
status, err := ctx.checkStatus(batch)
if err != nil || status.Delivered() {
    return status, err
}
```

This ensures that even if a duplicate somehow passes the sequence number check, it will be caught by the status check.

## Missing Transaction Detection and Healing

The system also includes mechanisms to detect and heal missing transactions:

```go
// File: internal/core/execute/v2/block/block_end.go

func (x *Executor) requestMissingSyntheticTransactions(blockIndex uint64, synthLedger *protocol.SyntheticLedger, anchorLedger *protocol.AnchorLedger) {
    // For each partition
    for _, partition := range x.partitions {
        // Skip our own partition
        if strings.EqualFold(partition.ID, x.describe.PartitionId) {
            continue
        }

        // Request missing synthetic transactions
        x.requestMissingTransactionsFromPartition(blockIndex, synthLedger, partition.ID, false)

        // Request missing anchors
        x.requestMissingTransactionsFromPartition(blockIndex, anchorLedger, partition.ID, true)
    }
}
```

This healing process runs periodically to identify and request any missing transactions based on gaps in the sequence numbers.

## Conclusion

In Accumulate's current implementation, sequence number validation and rejection of duplicate or out-of-order transactions happens at multiple points:

1. **Primary Rejection Point**: In the `isReady` method, which:
   - Rejects duplicates by checking if the sequence number is less than or equal to the last delivered sequence number
   - Marks out-of-order transactions as pending if the sequence number is not the next expected one

2. **Secondary Checks**:
   - Status check at the beginning of processing
   - Safety check during ledger update

3. **Handling Mechanism**:
   - Out-of-order transactions are stored in a pending state
   - When the preceding transaction is processed, the next transaction in the sequence is queued for processing
   - A healing process runs periodically to identify and request missing transactions

This multi-layered approach ensures that transactions are processed in the correct order, even if they are received out of order, while efficiently rejecting duplicates.

## References

1. [Accumulate Cross-Network Communication](../implementation/02_cross_network_communication.md)
2. [Anchor Proofs and Receipts](../implementation/03_anchor_proofs_and_receipts.md)
3. [Deterministic Anchor Transmission](12_deterministic_anchor_transmission.md)
4. [Sequence Numbers Current Implementation](16_sequence_numbers_current_implementation.md)
