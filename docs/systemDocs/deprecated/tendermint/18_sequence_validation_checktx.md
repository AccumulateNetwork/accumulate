---
title: Sequence Number Validation in CheckTx Phase
description: Analysis of whether sequence number validation occurs during the CheckTx phase and recommendations for optimization
tags: [accumulate, anchoring, synthetic-transactions, sequence-numbers, checktx, optimization]
created: 2025-05-16
version: 1.0
---

# Sequence Number Validation in CheckTx Phase

## Introduction

For optimal performance in a blockchain system, invalid transactions should be rejected as early as possible in the processing pipeline. This document analyzes whether sequence number validation for anchors and synthetic transactions occurs during the CheckTx phase in Accumulate, and provides recommendations for optimization.

## Current Implementation Analysis

### CheckTx Flow in Accumulate

When a transaction is received by a node, it first goes through the CheckTx phase before being added to the mempool. The CheckTx implementation in Accumulate is found in `internal/node/abci/accumulator.go`:

```go
// File: internal/node/abci/accumulator.go

func (app *Accumulator) CheckTx(_ context.Context, req *abci.RequestCheckTx) (rct *abci.ResponseCheckTx, err error) {
    // ...
    messages, results, respData, err := executeTransactions(app.logger.With("operation", "CheckTx"), func(envelope *messaging.Envelope) ([]*protocol.TransactionStatus, error) {
        return app.Executor.Validate(envelope, req.Type == abci.CheckTxType_Recheck)
    }, req.Tx)
    // ...
}
```

The key function here is `app.Executor.Validate()`, which is called to validate the transaction envelope.

### Validation Process

The validation process follows these steps:

1. The envelope is parsed and basic validation is performed
2. Each message in the envelope is validated individually
3. For sequenced messages (including synthetic transactions and anchors), the validation includes:
   - Basic field validation (source, destination, sequence number)
   - Signature verification
   - Inner message validation

However, the critical sequence number ordering check (whether the sequence number is the next expected one) is **not** performed during the CheckTx phase. This check is deferred until the actual processing phase (DeliverTx).

The sequence number ordering check occurs in the `isReady` method of `SequencedMessage`, which is called during processing, not validation:

```go
// File: internal/core/execute/v2/block/msg_sequenced.go

func (x SequencedMessage) isReady(batch *database.Batch, ctx *MessageContext, seq *messaging.SequencedMessage) (bool, error) {
    // ...
    // If the sequence number is old, mark it already delivered
    if seq.Number <= partitionLedger.Delivered {
        return false, errors.Delivered.WithFormat("%s has been delivered", typ)
    }

    // If the transaction is out of sequence, mark it pending
    if partitionLedger.Delivered+1 != seq.Number {
        ctx.Executor.logger.Info("Out of sequence message", ...)
        return false, nil
    }
    // ...
}
```

This method is called from the `process` method, not the `Validate` method:

```go
// File: internal/core/execute/v2/block/msg_sequenced.go

func (x SequencedMessage) process(batch *database.Batch, ctx *MessageContext, seq *messaging.SequencedMessage) (bool, error) {
    // Check if the message is ready to be processed
    ready, err := x.isReady(batch, ctx, seq)
    // ...
}
```

### Implications

This means that:

1. Duplicate transactions (with sequence numbers that have already been processed) are not rejected during CheckTx
2. Out-of-order transactions are not rejected during CheckTx
3. These transactions are added to the mempool and only rejected during the DeliverTx phase

This can lead to inefficiencies:
- The mempool may contain transactions that will eventually be rejected
- Network bandwidth is wasted propagating transactions that will be rejected
- Node resources are consumed storing and processing transactions that will never be committed

## Optimization Opportunity

Moving sequence number validation to the CheckTx phase would provide several benefits:

1. **Reduced Mempool Pollution**: Only valid transactions with the correct sequence numbers would enter the mempool
2. **Bandwidth Savings**: Invalid transactions would not be propagated to other nodes
3. **Resource Efficiency**: Node resources would not be wasted on transactions that will be rejected
4. **Faster Healing**: The healing process would be more efficient as it would not need to handle as many out-of-order transactions

## Implementation Recommendation

To implement sequence number validation during CheckTx, the following changes would be needed:

```go
// File: internal/core/execute/v2/block/msg_sequenced.go

func (x SequencedMessage) Validate(batch *database.Batch, ctx *MessageContext) (*protocol.TransactionStatus, error) {
    // Check the wrapper
    seq, err := x.check(batch, ctx)
    if err != nil {
        return nil, errors.UnknownError.Wrap(err)
    }

    // Check sequence number ordering
    isAnchor, ledger, err := x.loadLedger(batch, ctx, seq)
    if err != nil {
        return nil, errors.UnknownError.Wrap(err)
    }
    partitionLedger := ledger.Partition(seq.Source)

    // If the sequence number is old, reject it as a duplicate
    if seq.Number <= partitionLedger.Delivered {
        typ := "synthetic message"
        if isAnchor {
            typ = "anchor"
        }
        return nil, errors.Delivered.WithFormat("%s has been delivered", typ)
    }

    // If the transaction is out of sequence, reject it unless it's the next expected one
    if partitionLedger.Delivered+1 != seq.Number {
        return nil, errors.OutOfOrder.WithFormat(
            "expected sequence number %d, got %d", 
            partitionLedger.Delivered+1, seq.Number)
    }

    // Validate the inner message
    _, err = ctx.callMessageValidator(batch, seq.Message)
    return nil, errors.UnknownError.Wrap(err)
}
```

This change would reject duplicate and out-of-order transactions during the CheckTx phase, preventing them from entering the mempool.

## Considerations for Deterministic Anchor Transmission

For the deterministic anchor transmission system, early sequence number validation is particularly important:

1. **Reduced Network Load**: Since all validators will be generating and submitting transactions, early rejection of duplicates is critical to prevent network congestion
2. **Optimized Signature Collection**: Early validation ensures that only transactions with the correct sequence numbers are considered for signature collection
3. **Simplified Healing**: The healing process becomes simpler as fewer out-of-order transactions need to be handled

## Conclusion

The current implementation of Accumulate does not perform sequence number validation during the CheckTx phase, which can lead to inefficiencies in the system. Moving this validation earlier in the processing pipeline would provide significant benefits in terms of resource utilization and system performance.

For the deterministic anchor transmission system, implementing early sequence number validation should be considered a priority to ensure optimal performance and reliability.

## References

1. [Accumulate Cross-Network Communication](../implementation/02_cross_network_communication.md)
2. [Sequence Numbers Current Implementation](16_sequence_numbers_current_implementation.md)
3. [Sequence Numbers Processing Flow](17_sequence_numbers_processing_flow.md)
4. [Deterministic Anchor Transmission](12_deterministic_anchor_transmission.md)
