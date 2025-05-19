# Error Handling - V2 Implementation

```yaml
# AI-METADATA
document_type: implementation_detail
project: accumulate_network
component: error_handling
version: v2
related_files:
  - ../../concepts/error-handling.md
  - ./transaction-healing-process.md
  - ./caching.md
```

## Overview

This document details the error handling approach in the V2 implementation of the Accumulate Network debug tools. The V2 implementation significantly improves upon the basic error handling in V1, providing more robust mechanisms for handling and recovering from errors during the healing process.

## Error Handling Philosophy

The core philosophy of the V2 error handling approach is that **every transaction must eventually be healed - there are no permanent failures**. If the automated healing process cannot resolve issues, developers must take direct actions to fix the network. This approach ensures the network's integrity is maintained even in challenging conditions.

As stated in the transaction healing process documentation:

> The core philosophy of the healing process is that every transaction must eventually be healed - there are no permanent failures. If the automated healing process cannot resolve issues, developers must take direct actions to fix the network. This approach ensures the network's integrity is maintained even in challenging conditions.

## Error Categories

The V2 implementation categorizes errors into several types to apply appropriate handling strategies:

### 1. Network Errors

Network errors occur when communication with a node fails due to connectivity issues, timeouts, or node unavailability.

**Handling Strategy**:
- Record the failure in the node performance metrics
- Select an alternative node for future requests
- Retry the operation in the next healing cycle
- Log detailed information about the failure

```go
// Example of network error handling
if err != nil && isNetworkError(err) {
    h.nodeManager.RecordFailure(node, err)
    h.logger.Debug("Network error when querying node", 
        zap.String("node", node.String()),
        zap.Error(err))
    return nil, err
}
```

### 2. Routing Errors

Routing errors occur when a URL cannot be properly routed to a partition, often due to inconsistent URL construction.

**Handling Strategy**:
- Normalize the URL using standardized URL construction
- Add routing overrides for problematic URLs
- Track routing errors for specific URL patterns
- Log detailed information about the URL and routing attempt

```go
// Example of routing error handling
if err != nil && strings.Contains(err.Error(), "cannot route message") {
    normalizedUrl := normalizeUrl(url)
    h.logger.Debug("Routing error, attempting with normalized URL", 
        zap.String("original", url.String()),
        zap.String("normalized", normalizedUrl.String()))
    return h.queryWithNormalizedUrl(ctx, normalizedUrl)
}
```

### 3. Transaction Submission Errors

Transaction submission errors occur when a transaction cannot be submitted to the network, either due to validation failures, duplicate transactions, or other issues.

**Handling Strategy**:
- Record the failure in the transaction tracker
- Increment the submission count
- Try alternative submission approaches in future cycles
- Consider creating a synthetic alternative after multiple failures

```go
// Example of transaction submission error handling
if err != nil {
    tx.SubmissionCount++
    tx.LastAttemptTime = time.Now()
    h.txTracker.UpdateTransactionStatus(tx, TxStatusFailed)
    h.logger.Debug("Transaction submission failed", 
        zap.String("txid", tx.Hash.String()),
        zap.Error(err))
    
    if tx.SubmissionCount >= maxSubmissionAttempts {
        return h.createSyntheticAlternative(ctx, src, dst, gapNum)
    }
    return false, nil
}
```

### 4. Data Consistency Errors

Data consistency errors occur when there are inconsistencies in the data retrieved from different nodes, such as different chain heights or missing entries.

**Handling Strategy**:
- Compare data from multiple nodes to identify inconsistencies
- Use majority consensus to determine the correct data
- Flag significant inconsistencies for manual review
- Log detailed information about the inconsistencies

```go
// Example of data consistency error handling
if heightA != heightB {
    h.logger.Warn("Chain height inconsistency detected", 
        zap.String("chain", chainUrl.String()),
        zap.String("nodeA", nodeA.String()),
        zap.Uint64("heightA", heightA),
        zap.String("nodeB", nodeB.String()),
        zap.Uint64("heightB", heightB))
    
    // Use the higher height as it likely represents more recent data
    return math.Max(heightA, heightB), nil
}
```

## Error Recovery Mechanisms

The V2 implementation includes several mechanisms for recovering from errors:

### 1. Transaction Tracking and Reuse

The V2 implementation tracks transactions across healing cycles, allowing for reuse of previously created transactions:

```go
// Find an existing transaction for a gap
existingTx := h.txTracker.FindTransactionForGap(src.ID, dst.ID, gapNum)
if existingTx != nil {
    if existingTx.SubmissionCount < maxSubmissionAttempts {
        existingTx.SubmissionCount++
        existingTx.LastAttemptTime = time.Now()
        h.submit <- existingTx
        h.txTracker.UpdateTransactionStatus(existingTx, TxStatusPending)
        return true, nil
    }
    return h.createSyntheticAlternative(ctx, src, dst, gapNum)
}
```

This approach:
- Reduces the overhead of recreating transactions
- Maintains consistent transaction IDs across attempts
- Provides a history of submission attempts
- Enables intelligent decision-making based on past attempts

### 2. Node Performance Tracking

The V2 implementation tracks node performance to select the most reliable nodes for operations:

```go
// Select the best node for an operation
bestNode := h.nodeManager.GetBestNode(partition)
if bestNode == nil {
    return nil, fmt.Errorf("no available nodes for partition %s", partition)
}

// Record success or failure
if err != nil {
    h.nodeManager.RecordFailure(bestNode, err)
} else {
    h.nodeManager.RecordSuccess(bestNode)
}
```

This approach:
- Adapts to changing network conditions
- Avoids repeatedly using unreliable nodes
- Balances load across available nodes
- Improves overall success rates

### 3. URL Normalization

The V2 implementation normalizes URLs to ensure consistent routing:

```go
// Normalize a URL for consistent routing
func normalizeUrl(u *url.URL) *url.URL {
    if u == nil {
        return nil
    }
    
    // Convert anchor pool URL to partition URL
    if strings.EqualFold(u.Authority, protocol.Directory) && strings.HasPrefix(u.Path, "/anchors/") {
        partitionID := strings.TrimPrefix(u.Path, "/anchors/")
        return protocol.PartitionUrl(partitionID)
    }
    
    // Already in the correct format
    return u
}
```

This approach:
- Reduces "cannot route message" errors
- Ensures consistent key formats for caching
- Improves compatibility between different code components
- Standardizes on the raw partition URL format

### 4. Synthetic Alternatives

After multiple failed attempts with a transaction, the V2 implementation creates synthetic alternatives:

```go
// Create a synthetic alternative after multiple failed attempts
func (h *Healer) createSyntheticAlternative(ctx context.Context, src, dst *PartitionInfo, gapNum uint64) (bool, error) {
    h.logger.Info("Creating synthetic alternative after multiple failed attempts",
        zap.String("source", src.ID),
        zap.String("destination", dst.ID),
        zap.Uint64("sequence", gapNum))
    
    // Implementation of synthetic alternative creation
    // ...
    
    return true, nil
}
```

This approach:
- Provides an alternative path for healing when normal methods fail
- Ensures no permanent failures in the healing process
- Adapts to different failure scenarios
- Maintains network integrity even in challenging conditions

## Error Logging and Reporting

The V2 implementation includes comprehensive error logging and reporting:

### 1. Structured Logging

All errors are logged with structured metadata for easy parsing and analysis:

```go
h.logger.Error("Failed to heal gap",
    zap.String("source", src.ID),
    zap.String("destination", dst.ID),
    zap.Uint64("sequence", gapNum),
    zap.Error(err),
    zap.String("errorType", categorizeError(err)),
    zap.Int("attemptCount", tx.SubmissionCount))
```

This approach:
- Provides context-rich error information
- Enables automated analysis of error patterns
- Facilitates troubleshooting and debugging
- Supports monitoring and alerting systems

### 2. Error Metrics

Key error metrics are tracked and exposed:

```go
// Update error metrics
h.metrics.ErrorCount.WithLabelValues(src.ID, dst.ID, categorizeError(err)).Inc()
h.metrics.FailedHealing.WithLabelValues(src.ID, dst.ID).Inc()
```

This approach:
- Provides quantitative insights into error patterns
- Enables trend analysis over time
- Supports performance optimization efforts
- Facilitates capacity planning and resource allocation

### 3. Detailed Error Reports

The V2 implementation generates detailed error reports:

```go
// Generate a detailed error report
func (h *Healer) generateErrorReport() *ErrorReport {
    report := &ErrorReport{
        Time:            time.Now(),
        NetworkErrors:   h.metrics.ErrorCount.WithLabelValues("", "", "network").Value(),
        RoutingErrors:   h.metrics.ErrorCount.WithLabelValues("", "", "routing").Value(),
        SubmissionErrors: h.metrics.ErrorCount.WithLabelValues("", "", "submission").Value(),
        ConsistencyErrors: h.metrics.ErrorCount.WithLabelValues("", "", "consistency").Value(),
        ErrorsByPartition: make(map[string]int),
    }
    
    // Populate error details
    // ...
    
    return report
}
```

This approach:
- Provides comprehensive error information
- Enables root cause analysis
- Supports continuous improvement efforts
- Facilitates communication with stakeholders

## Error Handling Improvements from V1

The V2 implementation includes several significant improvements over the V1 error handling approach:

### 1. Structured Error Categorization

V1 had minimal error categorization, often treating all errors the same. V2 categorizes errors into specific types (network, routing, submission, consistency) for targeted handling.

### 2. Persistent Transaction Tracking

V1 did not track transactions across healing cycles, requiring recreation of transactions for each attempt. V2 maintains a transaction tracker that persists across cycles, enabling reuse and history-based decision-making.

### 3. Node Performance Metrics

V1 did not track node performance, often repeatedly using unreliable nodes. V2 maintains detailed performance metrics for each node, selecting the most reliable nodes for operations.

### 4. URL Normalization

V1 had inconsistent URL construction, leading to routing errors. V2 standardizes on the raw partition URL format and includes normalization logic to handle legacy formats.

### 5. Comprehensive Logging

V1 had basic logging with limited context. V2 includes structured logging with rich context, enabling better troubleshooting and analysis.

### 6. Synthetic Alternatives

V1 had limited options when transactions failed. V2 creates synthetic alternatives after multiple failed attempts, ensuring no permanent failures.

### 7. Error Metrics and Reporting

V1 had minimal error tracking and reporting. V2 includes comprehensive metrics and detailed reports, providing insights into error patterns and trends.

## Best Practices for Error Handling in V2

When working with the V2 implementation, consider these best practices:

1. **Use Structured Logging**: Always include relevant context in error logs using structured fields.

2. **Categorize Errors**: Properly categorize errors to apply appropriate handling strategies.

3. **Track Node Performance**: Record successes and failures for each node to build reliable performance metrics.

4. **Normalize URLs**: Always normalize URLs before routing to ensure consistency.

5. **Implement Graceful Degradation**: Design systems to continue functioning with reduced capabilities when errors occur.

6. **Monitor Error Metrics**: Regularly review error metrics to identify patterns and trends.

7. **Update Error Handling Strategies**: Continuously refine error handling strategies based on observed patterns.

## Conclusion

The V2 implementation of the Accumulate Network debug tools includes a comprehensive error handling approach that significantly improves upon the V1 implementation. By categorizing errors, implementing recovery mechanisms, and providing detailed logging and reporting, the V2 implementation ensures that the healing process can handle a wide range of error scenarios while maintaining network integrity.

The core philosophy that "every transaction must eventually be healed" guides the error handling approach, ensuring that the system continues to function even in challenging conditions. This resilience is critical for maintaining the overall health and integrity of the Accumulate Network.

## See Also

- [Error Handling Approaches](../../concepts/error-handling.md): Comprehensive explanation of error handling approaches
- [Transaction Healing Process](./transaction-healing-process.md): Detailed explanation of the V2 transaction healing process
- [Caching](./caching.md): Explanation of the caching system in V2
