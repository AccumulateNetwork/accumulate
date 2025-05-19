# Error Handling

## Introduction

Error handling is a critical aspect of the healing processes in Accumulate. Proper error handling ensures that the healing processes can gracefully handle various error conditions, provide meaningful error messages, and maintain data integrity. This document details the error handling mechanisms used in the healing processes.

## Error Types

### Structured Error Types

Accumulate uses structured error types to provide more context and information about errors:

```go
// From pkg/errors/errors.go
var (
    // NotFound is returned when a requested resource is not found
    NotFound = Code("not-found")
    
    // BadRequest is returned when a request is malformed or invalid
    BadRequest = Code("bad-request")
    
    // Delivered is returned when a transaction has already been delivered
    Delivered = Code("delivered")
    
    // Pending is returned when a transaction is pending
    Pending = Code("pending")
    
    // ...
)
```

These structured error types allow the healing processes to handle specific error conditions in a consistent way.

### Error Wrapping

The healing processes use error wrapping to provide more context about errors:

```go
// From internal/core/healing/synthetic.go
// Load the synthetic sequence chain entry
b, err := batch.Account(uSrcSynth).SyntheticSequenceChain(si.Destination).Entry(int64(si.Number) - 1)
if err != nil {
    return nil, errors.UnknownError.WithFormat(
        "load synthetic sequence chain entry %d: %w", si.Number, err)
}
```

This error wrapping provides a clear error message that includes the context of the error, making it easier to diagnose and fix issues.

## Error Handling Patterns

### Graceful Degradation

The healing processes use graceful degradation to handle errors in a way that allows the process to continue even if some operations fail:

```go
// From internal/core/healing/anchors.go
// Get a signature from each node that hasn't signed
var gotPartSig bool
var signatures []protocol.Signature
for peer, info := range args.NetInfo.Peers[strings.ToLower(si.Source)] {
    if signed[info.Key] {
        continue
    }
    
    addr := multiaddr.StringCast("/p2p/" + peer.String())
    if len(info.Addresses) > 0 {
        addr = info.Addresses[0].Encapsulate(addr)
    }
    
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()
    
    slog.InfoContext(ctx, "Querying node for its signature", "id", peer)
    res, err := args.Client.ForAddress(addr).Private().Sequence(ctx, srcUrl.JoinPath(protocol.AnchorPool), dstUrl, si.Number, private.SequenceOptions{})
    if err != nil {
        slog.ErrorContext(ctx, "Query failed", "error", err)
        continue
    }
    
    // Process the signature
    // ...
}
```

In this example, if querying a node for its signature fails, the process logs the error and continues with the next node, rather than failing the entire operation.

### Retry Logic

The healing processes include retry logic to handle temporary failures:

```go
// From tools/cmd/debug/heal_anchor.go
func (h *healer) healSingleAnchor(srcId, dstId string, seqNum uint64, txid *url.TxID, txns map[[32]byte]*protocol.Transaction) bool {
    var count int
    
    // Increment the transaction submitted counter
    h.txSubmitted++
    
    // Update pair statistics
    pairKey := fmt.Sprintf("%s:%s", srcId, dstId)
    if pairStat, ok := h.pairStats[pairKey]; ok {
        pairStat.TxSubmitted++
        pairStat.LastUpdated = time.Now()
    }
    
retry:
    err := healing.HealAnchor(h.ctx, healing.HealAnchorArgs{
        Client:  h.C2.ForAddress(nil),
        Source:  srcId,
        Dest:    dstId,
        Number:  seqNum,
        TxnMap:  txns,
        ID:      txid,
    })
    if err == nil {
        // Successfully healed the anchor
        h.txDelivered++
        h.totalHealed++
        
        // Update pair statistics
        if pairStat, ok := h.pairStats[pairKey]; ok {
            pairStat.TxDelivered++
            pairStat.CurrentDelivered = seqNum
            pairStat.LastUpdated = time.Now()
        }
        
        slog.InfoContext(h.ctx, "Successfully healed anchor", 
                "source", srcId, "destination", dstId, "number", seqNum, "id", txid,
                "submitted", h.txSubmitted, "delivered", h.txDelivered)
        return false
    }
    
    // Handle various error conditions
    // ...
    
    count++
    if count >= 10 {
        // Failed after multiple attempts
        h.totalFailed++
        
        // Update pair statistics
        if pairStat, ok := h.pairStats[pairKey]; ok {
            pairStat.TxFailed++
            pairStat.LastUpdated = time.Now()
        }
        
        slog.ErrorContext(h.ctx, "Anchor still pending after multiple attempts, skipping", 
                "source", srcId, "destination", dstId, "number", seqNum, "id", txid,
                "attempts", count, "submitted", h.txSubmitted, "delivered", h.txDelivered)
        return false
    }
    slog.WarnContext(h.ctx, "Anchor still pending, retrying", 
            "source", srcId, "destination", dstId, "number", seqNum, "id", txid,
            "attempt", count, "submitted", h.txSubmitted, "delivered", h.txDelivered)
    goto retry
}
```

This retry logic allows the healing process to handle temporary failures by retrying the operation a limited number of times before giving up.

### Error Classification

The healing processes classify errors to handle them appropriately:

```go
// From tools/cmd/debug/heal_synth.go
if errors.Is(err, errors.Delivered) {
    // Transaction already delivered
    h.txDelivered++
    
    // Update pair statistics
    if pairStat, ok := h.pairStats[pairKey]; ok {
        pairStat.TxDelivered++
        pairStat.CurrentDelivered = number
        pairStat.LastUpdated = time.Now()
    }
    
    slog.InfoContext(h.ctx, "Transaction already delivered", 
        "source", source, "destination", destination, "number", number, "id", id,
        "submitted", h.txSubmitted, "delivered", h.txDelivered)
    return false
} else if errors.Is(err, errors.Pending) {
    // Transaction pending
    slog.InfoContext(h.ctx, "Transaction pending", 
        "source", source, "destination", destination, "number", number, "id", id,
        "submitted", h.txSubmitted, "delivered", h.txDelivered)
    return true
} else {
    // Other error
    h.totalFailed++
    
    // Update pair statistics
    if pairStat, ok := h.pairStats[pairKey]; ok {
        pairStat.TxFailed++
        pairStat.LastUpdated = time.Now()
    }
    
    slog.ErrorContext(h.ctx, "Failed to heal transaction", 
        "source", source, "destination", destination, "number", number, "id", id,
        "error", err, "submitted", h.txSubmitted, "delivered", h.txDelivered)
    return false
}
```

This classification allows the healing process to handle different types of errors in different ways, providing a more nuanced approach to error handling.

## Error Logging

### Structured Logging

The healing processes use structured logging to provide more context about errors:

```go
// From tools/cmd/debug/heal_synth.go
slog.ErrorContext(h.ctx, "Failed to heal transaction", 
    "source", source, "destination", destination, "number", number, "id", id,
    "error", err, "submitted", h.txSubmitted, "delivered", h.txDelivered)
```

This structured logging provides a clear error message that includes the context of the error, making it easier to diagnose and fix issues.

### Log Levels

The healing processes use different log levels to indicate the severity of errors:

```go
// From tools/cmd/debug/heal_anchor.go
slog.InfoContext(h.ctx, "Successfully healed anchor", 
        "source", srcId, "destination", dstId, "number", seqNum, "id", txid,
        "submitted", h.txSubmitted, "delivered", h.txDelivered)

slog.WarnContext(h.ctx, "Anchor still pending, retrying", 
        "source", srcId, "destination", dstId, "number", seqNum, "id", txid,
        "attempt", count, "submitted", h.txSubmitted, "delivered", h.txDelivered)

slog.ErrorContext(h.ctx, "Anchor still pending after multiple attempts, skipping", 
        "source", srcId, "destination", dstId, "number", seqNum, "id", txid,
        "attempts", count, "submitted", h.txSubmitted, "delivered", h.txDelivered)
```

These log levels allow operators to filter logs based on severity, making it easier to focus on critical errors.

## Error Handling for Specific Cases

### Transaction Not Found

The healing processes handle the case where a transaction is not found:

```go
// From internal/core/healing/synthetic.go
// Fetch the transaction and signatures
var sigSets []*api.SignatureSetRecord
Q := api.Querier2{Querier: args.Querier}
res, err := Q.QueryMessage(ctx, si.ID, nil)
switch {
case err == nil:
    if res.Status.Delivered() {
        slog.InfoContext(ctx, "Synthetic message has been delivered", "id", si.ID, "source", si.Source, "destination", si.Destination, "number", si.Number)
        return errors.Delivered
    }
    sigSets = res.Signatures.Records
    txn = res.Message.(*messaging.TransactionMessage).Transaction

case !errors.Is(err, errors.NotFound):
    return err
}
```

In this example, if the transaction is not found (errors.NotFound), the process continues with an empty signature set, allowing it to proceed with healing the transaction.

### Transaction Already Delivered

The healing processes handle the case where a transaction has already been delivered:

```go
// From internal/core/healing/synthetic.go
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
    slog.InfoContext(ctx, "Synthetic message has been delivered", "id", si.ID, "source", si.Source, "destination", si.Destination, "number", si.Number)
    return errors.Delivered
}
```

In this example, if the transaction has already been delivered, the process returns a specific error (errors.Delivered) to indicate that healing is not needed.

### Transaction Pending

The healing processes handle the case where a transaction is pending:

```go
// From tools/cmd/debug/heal_synth.go
if errors.Is(err, errors.Pending) {
    // Transaction pending
    slog.InfoContext(h.ctx, "Transaction pending", 
        "source", source, "destination", destination, "number", number, "id", id,
        "submitted", h.txSubmitted, "delivered", h.txDelivered)
    return true
}
```

In this example, if the transaction is pending, the process logs the status and returns true to indicate that the transaction should be retried later.

### Network Errors

The healing processes handle network errors:

```go
// From internal/core/healing/anchors.go
slog.InfoContext(ctx, "Querying node for its signature", "id", peer)
res, err := args.Client.ForAddress(addr).Private().Sequence(ctx, srcUrl.JoinPath(protocol.AnchorPool), dstUrl, si.Number, private.SequenceOptions{})
if err != nil {
    slog.ErrorContext(ctx, "Query failed", "error", err)
    continue
}
```

In this example, if querying a node for its signature fails due to a network error, the process logs the error and continues with the next node, rather than failing the entire operation.

## Error Reporting

### Error Statistics

The healing processes track and report error statistics:

```go
// From tools/cmd/debug/heal_common.go
func (h *healer) printDetailedReport() {
    // ...
    
    // Print delivery statistics
    fmt.Println("\n* DELIVERY STATUS:")
    fmt.Printf("   Transactions Submitted: %d\n", h.txSubmitted)
    fmt.Printf("   Transactions Delivered: %d\n", h.txDelivered)
    fmt.Printf("   Transactions Failed: %d\n", h.totalFailed)
    
    // ...
}
```

These statistics provide a high-level view of the healing process, including the number of failed operations.

### Error Details

The healing processes provide detailed information about errors:

```go
// From tools/cmd/debug/heal_synth.go
slog.ErrorContext(h.ctx, "Failed to heal transaction", 
    "source", source, "destination", destination, "number", number, "id", id,
    "error", err, "submitted", h.txSubmitted, "delivered", h.txDelivered)
```

This detailed information helps operators diagnose and fix issues.

## Data Integrity Rules

The healing processes follow strict data integrity rules to ensure that they do not fabricate or fake any data:

```go
// From internal/core/healing/synthetic.go
// Query the synthetic transaction
r, err := ResolveSequenced[messaging.Message](ctx, args.Client, args.NetInfo, si.Source, si.Destination, si.Number, false)
if err != nil {
    return err // Return the error, don't fabricate data
}
```

These rules are critical for maintaining the integrity of the healing process and ensuring that it does not mask errors by fabricating data.

## Recent Changes

Recent changes to the error handling mechanisms include:

1. **Improved Error Classification**: Enhanced error classification to better handle specific error conditions
2. **Structured Logging**: Improved structured logging to provide more context about errors
3. **Retry Logic**: Enhanced retry logic to handle temporary failures

## Best Practices

When working with error handling in the healing processes, it's important to follow these best practices:

1. **Use Structured Error Types**: Use structured error types to provide more context about errors
2. **Wrap Errors**: Wrap errors to provide more context about the error
3. **Use Graceful Degradation**: Use graceful degradation to handle errors in a way that allows the process to continue
4. **Include Retry Logic**: Include retry logic to handle temporary failures
5. **Follow Data Integrity Rules**: Never fabricate or fake data that isn't available from the network

```go
// Example of following data integrity rules
// From internal/core/healing/synthetic.go
// Query the synthetic transaction
r, err := ResolveSequenced[messaging.Message](ctx, args.Client, args.NetInfo, si.Source, si.Destination, si.Number, false)
if err != nil {
    return err // Return the error, don't fabricate data
}
```

## Conclusion

Error handling is a critical aspect of the healing processes in Accumulate. By using structured error types, error wrapping, graceful degradation, retry logic, and error classification, the healing processes can handle various error conditions in a consistent and reliable way.

In the next document, we will explore the testing and verification mechanisms used to ensure the correctness of the healing processes.
