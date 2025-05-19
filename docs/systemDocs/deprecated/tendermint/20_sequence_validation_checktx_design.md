---
title: Design for Sequence Number Validation in CheckTx
description: Implementation plan for rejecting outdated sequence numbers during the CheckTx phase
tags: [accumulate, sequence-numbers, validation, checktx, optimization, implementation-plan]
created: 2025-05-16
version: 1.0
---

# Design for Sequence Number Validation in CheckTx

## 1. Overview

This document outlines a focused design for implementing sequence number validation during the CheckTx phase to reject transactions with outdated sequence numbers. This optimization will improve system efficiency by preventing already-processed transactions from entering the mempool.

## 2. Current Implementation

Currently, sequence number validation occurs during the transaction processing phase (DeliverTx) in the `isReady` method of `SequencedMessage`:

```go
func (x SequencedMessage) isReady(batch *database.Batch, ctx *MessageContext, seq *messaging.SequencedMessage) (bool, error) {
    // ...
    // If the sequence number is old, mark it already delivered
    if seq.Number <= partitionLedger.Delivered {
        return false, errors.Delivered.WithFormat("%s has been delivered", typ)
    }
    // ...
}
```

However, this check is not performed during the CheckTx phase, allowing duplicate transactions to enter the mempool.

## 3. Design Goals

1. **Focused Scope**: Only implement validation for outdated sequence numbers (seq.Number <= partitionLedger.Delivered)
2. **Minimal Impact**: Make changes with minimal impact on the existing codebase
3. **Performance**: Ensure the validation does not significantly impact CheckTx performance

## 4. Implementation Design

### 4.1 Code Changes

#### 4.1.1 Add Sequence Validation to Executor's Validate Method

```go
// File: internal/core/execute/v2/executor.go

func (x *Executor) Validate(envelope *messaging.Envelope, recheck bool) ([]*protocol.TransactionStatus, error) {
    // Existing validation code...
    
    // Add sequence number validation for sequenced messages
    for _, msg := range envelope.Messages {
        if seq, ok := msg.(*messaging.SequencedMessage); ok {
            err := x.validateSequenceNumber(batch, seq)
            if err != nil {
                // Return appropriate error
                return nil, err
            }
        }
    }
    
    // Continue with existing validation...
}

// New method to validate sequence numbers
func (x *Executor) validateSequenceNumber(batch *database.Batch, seq *messaging.SequencedMessage) error {
    // Load the appropriate ledger based on message type
    var ledger protocol.SequenceLedger
    var isAnchor bool
    
    if seq.Message.Type() == messaging.MessageTypeAnchor {
        // Handle anchor message
        isAnchor = true
        ledger = batch.Account(protocol.AnchorPool).AnchorLedger()
    } else {
        // Handle synthetic transaction
        ledger = batch.Account(protocol.SyntheticLedgerUrl).SyntheticLedger()
    }
    
    // Get the partition ledger for the source
    partitionLedger := ledger.Partition(seq.Source)
    
    // Check if sequence number is outdated
    if seq.Number <= partitionLedger.Delivered {
        typ := "synthetic message"
        if isAnchor {
            typ = "anchor"
        }
        return errors.Delivered.WithFormat("%s has been delivered", typ)
    }
    
    return nil
}
```

#### 4.1.2 Update CheckTx to Handle Sequence Errors

```go
// File: internal/node/abci/accumulator.go

func (app *Accumulator) CheckTx(_ context.Context, req *abci.RequestCheckTx) (rct *abci.ResponseCheckTx, err error) {
    // Existing code...
    
    messages, results, respData, err := executeTransactions(app.logger.With("operation", "CheckTx"), func(envelope *messaging.Envelope) ([]*protocol.TransactionStatus, error) {
        return app.Executor.Validate(envelope, req.Type == abci.CheckTxType_Recheck)
    }, req.Tx)
    
    if err != nil {
        // Check if it's a sequence error
        if errors.Is(err, errors.Delivered) {
            // Create a response with the appropriate error code for already delivered
            var res abci.ResponseCheckTx
            b, _ := err.(*errors.Error).MarshalJSON()
            res.Info = string(b)
            res.Log = string(b)
            res.Code = uint32(protocol.ErrorCodeDelivered) // Use appropriate error code
            return &res, nil
        }
        
        // Handle other errors as before
        b, _ := errors.UnknownError.Wrap(err).(*errors.Error).MarshalJSON()
        var res abci.ResponseCheckTx
        res.Info = string(b)
        res.Log = string(b)
        res.Code = uint32(protocol.ErrorCodeFailed)
        return &res, nil
    }
    
    // Continue with existing code...
}
```

### 4.2 Database Considerations

The implementation requires read-only access to the ledger state during CheckTx. To minimize performance impact:

1. **Use Read-Only Batch**: Ensure the database batch used in CheckTx is read-only to prevent any state changes
2. **Consider Caching**: For frequently accessed ledger states, implement a simple cache to reduce database reads

```go
// Example of using a read-only batch
func (x *Executor) Validate(envelope *messaging.Envelope, recheck bool) ([]*protocol.TransactionStatus, error) {
    // Create a read-only batch
    batch := x.database.Begin(true)
    defer batch.Discard()
    
    // Validation logic...
}
```

## 5. Testing Strategy

### 5.1 Unit Tests

1. **Sequence Validation Tests**: Create unit tests for the `validateSequenceNumber` method
   - Test with sequence numbers below, equal to, and above the delivered number
   - Test with different message types (anchor, synthetic)

2. **CheckTx Integration Tests**: Test the CheckTx method with various sequence scenarios
   - Test with already delivered sequence numbers
   - Test with valid sequence numbers
   - Test with mixed message types

### 5.2 Integration Tests

1. **Network Tests**: Test the behavior in a multi-node network
   - Verify that duplicate transactions are rejected at CheckTx
   - Measure the impact on network traffic

2. **Performance Tests**: Measure the performance impact
   - Benchmark CheckTx with and without sequence validation
   - Test with high transaction volumes

## 6. Rollout Plan

1. **Development**: Implement the changes in a feature branch
2. **Code Review**: Conduct thorough code review focusing on correctness and performance
3. **Testing**: Execute the testing strategy outlined above
4. **Staging Deployment**: Deploy to a staging environment for further testing
5. **Production Deployment**: Roll out to production with monitoring

## 7. Development Time Estimate

| Task | Description | Estimated Time |
|------|-------------|----------------|
| Design Review | Review and finalize design with team | 1 day |
| Implementation | Code changes to Executor and CheckTx | 2 days |
| Unit Testing | Create and run unit tests | 1 day |
| Integration Testing | Create and run integration tests | 2 days |
| Performance Testing | Benchmark and optimize | 1 day |
| Documentation | Update documentation | 1 day |
| Code Review & Revisions | Address feedback | 2 days |
| **Total** | | **10 days** |

### 7.1 Assumptions

- The developer has good knowledge of the Accumulate codebase
- No major architectural changes are discovered during implementation
- Testing environments are readily available

### 7.2 Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Performance degradation in CheckTx | Medium | Implement caching, optimize database access |
| False rejections | High | Thorough testing with various sequence scenarios |
| Increased complexity | Low | Keep changes focused, add comprehensive documentation |

## 8. Future Enhancements

After this initial implementation is successful, consider these future enhancements:

1. **Reject Far-Future Sequences**: Reject transactions with sequence numbers too far ahead
2. **Transaction Sorting**: Implement sorting in PrepareProposal
3. **Enhanced Metrics**: Add metrics to track rejected transactions by type

## 9. Conclusion

This focused implementation of sequence number validation in CheckTx will provide immediate benefits by rejecting already-processed transactions early in the pipeline. With an estimated development time of 10 days, this represents a high-value, relatively low-effort improvement to the Accumulate protocol.

## 10. References

1. [Comprehensive Analysis of Sequence Numbers](19_sequence_numbers_comprehensive_analysis.md)
2. [Sequence Numbers Processing Flow](17_sequence_numbers_processing_flow.md)
3. [Sequence Validation in CheckTx](18_sequence_validation_checktx.md)
