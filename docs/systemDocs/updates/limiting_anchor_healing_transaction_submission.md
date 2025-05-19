---
title: Limiting Anchor Transaction Submission During Healing
description: Design for throttling and limiting anchor transaction submission during healing processes
tags: [accumulate, healing, optimization, throttling, anchor-transactions]
created: 2025-05-19
version: 0.1
status: draft
---

# Limiting Anchor Transaction Submission During Healing

## 1. Introduction

This document outlines a design for limiting and throttling anchor transaction submission during healing processes in the Accumulate network. The goal is to optimize network performance, reduce congestion, and ensure anchor healing operations do not overwhelm the network.

## 2. Problem Statement

Currently, the anchor healing process can potentially submit an unlimited number of transactions in rapid succession. This can lead to network congestion, increased resource consumption, and potential performance degradation across the network. As noted in our existing documentation, anchor healing is a critical process that ensures anchors are properly delivered across all partitions. Without proper rate limiting, this process can overwhelm network resources.

## 3. Design Goals

- Limit the rate of anchor transaction submission during healing processes
- Prevent network congestion from anchor healing operations
- Maintain healing effectiveness while reducing resource consumption
- Provide consistent and predictable healing behavior
- Ensure proper error handling without fabricating data (as per critical rule in memory)

## 4. Proposed Solution

For the anchor healing process, implement a strict batching mechanism that:
1. Limits submission to one batch per pass
2. Restricts each batch to a maximum of 20 transactions

### 4.1 Rate Limiting Mechanism

- **Batch Size Limit**: Set a hard limit of 20 anchor transactions per batch
- **Batch Frequency Control**: Only allow one batch to be submitted per healing pass
- **Pass Completion**: A healing pass is considered complete when the batch has been fully processed
- **Error Handling**: Properly categorize errors to determine whether to skip individual transactions or abort the entire batch

### 4.2 Configurable Parameters

- `MaxBatchSize`: Maximum number of anchor transactions in a batch (default: 20)
- `BatchesPerPass`: Number of batches allowed per healing pass (fixed at 1)
- `PassInterval`: Minimum time between healing passes (default: 30 seconds)

### 4.3 Implementation Details

The implementation will focus on modifying the anchor healing process to support batch processing and improved error handling. Below are the key code changes required:

#### 4.3.1 Error Types

First, we need to add new error types to `/pkg/errors/errors.go` (these are the same error types used for synthetic healing):

```go
// Add to pkg/errors/errors.go
var (
    // SkipTransaction indicates that the current transaction should be skipped
    // but batch processing should continue
    SkipTransaction = Define("skip transaction")
    
    // BatchAbort indicates that the current batch should be aborted
    // and processing should skip to the next pass
    BatchAbort = Define("batch abort")
)
```

#### 4.3.2 Core Healing Functions

Modify the `HealAnchor` function in `/internal/core/healing/anchors.go` to handle errors appropriately:

```go
// Modified HealAnchor function in internal/core/healing/anchors.go
func HealAnchor(ctx context.Context, args HealAnchorArgs, si SequencedInfo) error {
    // Determine the anchor type
    var err error
    switch {
    case si.Source == protocol.Directory:
        // DN -> BVN
        err = healDnAnchorV2(ctx, args, si)
    case si.Destination == protocol.Directory:
        // BVN -> DN
        err = healAnchorV1(ctx, args, si)
    default:
        // Log the error
        slog.ErrorContext(ctx, "Unknown anchor direction", 
            "source", si.Source, "destination", si.Destination)
        return errors.SkipTransaction.WithFormat("unknown anchor direction: %s -> %s", si.Source, si.Destination)
    }
    
    // Handle errors based on type
    if err != nil {
        // Check if this is a batch abort error
        if errors.Is(err, errors.BatchAbort) {
            return err
        }
        
        // For other errors, skip just this transaction
        return errors.SkipTransaction.WithFormat("anchor healing failed: %w", err)
    }
    
    return nil
}
```

Modify the `healDnAnchorV2` function to classify errors:

```go
// Modified healDnAnchorV2 function in internal/core/healing/anchors.go
func healDnAnchorV2(ctx context.Context, args HealAnchorArgs, si SequencedInfo) error {
    rBVN, err := ResolveSequenced[*messaging.TransactionMessage](ctx, args.Client, args.NetInfo, protocol.Directory, si.Destination, si.Number, true)
    if err != nil {
        // Log the error
        slog.ErrorContext(ctx, "Failed to resolve anchor transaction", 
            "source", protocol.Directory, "destination", si.Destination, 
            "number", si.Number, "error", err)
        
        // Check if this is a network-wide issue that would affect all transactions
        if errors.Is(err, errors.NetworkError) || errors.Is(err, errors.Unavailable) {
            return errors.BatchAbort.WithFormat("network error resolving anchor transaction: %w", err)
        }
        
        // For other errors, skip just this transaction
        return errors.SkipTransaction.WithFormat("failed to resolve anchor transaction: %w", err)
    }
    
    // Rest of the function with similar error handling patterns
    // ...
    
    // Example of checking for database errors
    batch := args.Light.OpenDB(false)
    defer batch.Discard()
    
    // Load the anchor sequence chain entry
    entry, err := batch.Account(uDnAnchor).AnchorSequenceChain(si.Destination).Entry(int64(si.Number) - 1)
    if err != nil {
        // Check if this is a critical error that would affect all transactions
        if errors.Is(err, errors.DatabaseError) {
            // Critical error - abort the entire batch
            return errors.BatchAbort.WithFormat(
                "critical database error loading anchor sequence chain entry: %w", err)
        } else {
            // Non-critical error - skip just this transaction
            return errors.SkipTransaction.WithFormat(
                "load anchor sequence chain entry %d: %w", si.Number, err)
        }
    }
    
    // Rest of the function remains unchanged
    // ...
    
    return nil
}
```

Similarly, modify the `healAnchorV1` function to classify errors:

```go
// Modified healAnchorV1 function in internal/core/healing/anchors.go
func healAnchorV1(ctx context.Context, args HealAnchorArgs, si SequencedInfo) error {
    rDN, err := ResolveSequenced[*messaging.TransactionMessage](ctx, args.Client, args.NetInfo, si.Source, protocol.Directory, si.Number, true)
    if err != nil {
        // Log the error
        slog.ErrorContext(ctx, "Failed to resolve anchor transaction", 
            "source", si.Source, "destination", protocol.Directory, 
            "number", si.Number, "error", err)
        
        // Check if this is a network-wide issue that would affect all transactions
        if errors.Is(err, errors.NetworkError) || errors.Is(err, errors.Unavailable) {
            return errors.BatchAbort.WithFormat("network error resolving anchor transaction: %w", err)
        }
        
        // For other errors, skip just this transaction
        return errors.SkipTransaction.WithFormat("failed to resolve anchor transaction: %w", err)
    }
    
    // Rest of the function with similar error handling patterns
    // ...
    
    return nil
}
```

#### 4.3.3 Command Line Implementation

Update `/tools/cmd/debug/heal_anchor.go` to implement batch processing:

```go
// Define constants for default values
const (
    defaultMaxBatchSize   = 20               // Default maximum transactions per batch
    defaultBatchesPerPass = 1                // Default number of batches per pass
    defaultPassInterval  = 30 * time.Second // Default interval between passes
)

// Batch processing configuration
var (
    maxBatchSize   int           // Maximum transactions per batch
    batchesPerPass int           // Number of batches per pass
    passInterval   time.Duration // Interval between passes when running continuously
    continuous     bool          // Whether to run continuously
)

func init() {
    // Existing code...
    cmdHealAnchor.Flags().IntVar(&maxBatchSize, "batch-size", defaultMaxBatchSize, "Maximum transactions per batch")
    cmdHealAnchor.Flags().IntVar(&batchesPerPass, "batches-per-pass", defaultBatchesPerPass, "Number of batches per pass")
    cmdHealAnchor.Flags().DurationVar(&passInterval, "pass-interval", defaultPassInterval, "Interval between passes when running continuously")
    cmdHealAnchor.Flags().BoolVar(&continuous, "continuous", false, "Run healing continuously with intervals between passes")
}

// Add batch processing to healer struct
type healer struct {
    // Existing fields...
    ctx            context.Context
    C1             api.Client
    C2             api.Client
    net            *healing.NetworkInfo
    light          *light.Client
    skipCurrentPass bool
    batchStats      BatchStats
}

// BatchStats tracks statistics for batch processing
type BatchStats struct {
    BatchesProcessed int
    TxsProcessed     int
    TxsSucceeded     int
    TxsFailed        int
    TxsSkipped       int
    BatchesAborted   int
    LastBatchTime    time.Time
}

// Implement batch processing for anchor healing
func healAnchorBatch(h *healer, src, dst string, anchorsToHeal []AnchorInfo) {
    // Process only up to MaxBatchSize anchors
    batchSize := min(len(anchorsToHeal), maxBatchSize)
    
    slog.InfoContext(h.ctx, "Processing anchor transaction batch", 
        "source", src, "destination", dst, "batch-size", batchSize)
    
    h.batchStats.BatchesProcessed++
    batchStart := time.Now()
    
    for i := 0; i < batchSize; i++ {
        // Check if we should skip the current pass
        if h.skipCurrentPass {
            slog.InfoContext(h.ctx, "Skipping remainder of current pass due to critical error")
            break
        }
        
        anchor := anchorsToHeal[i]
        h.batchStats.TxsProcessed++
        
        success := healSingleAnchor(h, src, dst, anchor.Number, anchor.ID, anchor.Txns)
        
        // Update statistics
        if success {
            h.batchStats.TxsSucceeded++
        }
    }
    
    h.batchStats.LastBatchTime = time.Now()
    
    slog.InfoContext(h.ctx, "Batch processing completed", 
        "duration", time.Since(batchStart),
        "processed", h.batchStats.TxsProcessed,
        "succeeded", h.batchStats.TxsSucceeded,
        "failed", h.batchStats.TxsFailed,
        "skipped", h.batchStats.TxsSkipped)
    
    // If operating under continuous flag, implement delay between passes
    if continuous && h.skipCurrentPass {
        slog.InfoContext(h.ctx, "Entering delay period before next pass", 
            "delay", passInterval)
        time.Sleep(passInterval)
        h.skipCurrentPass = false
    }
}
```

Update the `healSingleAnchor` function to handle the new error types:

```go
// Updated healSingleAnchor function in /tools/cmd/debug/heal_anchor.go
func healSingleAnchor(h *healer, srcId, dstId string, seqNum uint64, txid *url.TxID, txns map[[32]byte]*protocol.Transaction) bool {
    var count int
retry:
    err := healing.HealAnchor(h.ctx, healing.HealAnchorArgs{
        Client:    h.C2.ForAddress(nil),
        NetInfo:   h.net,
        Light:     h.light,
        Pretend:   pretend,
        Wait:      waitForTxn,

        // If an attempt fails, use the next anchor
        SkipAnchors: count,
        Txns:        txns,
    }, healing.SequencedInfo{
        Source:      srcId,
        Destination: dstId,
        Number:      seqNum,
        ID:          txid,
    })
    
    if err == nil {
        return true
    }
    
    if errors.Is(err, errors.Delivered) {
        return true
    }
    
    if errors.Is(err, errors.SkipTransaction) {
        // Log that we're skipping this transaction but continuing the batch
        slog.WarnContext(h.ctx, "Skipping anchor in batch", 
            "source", srcId, "destination", dstId, 
            "number", seqNum, "error", err)
        h.batchStats.TxsSkipped++
        return false
    }
    
    if errors.Is(err, errors.BatchAbort) {
        // Log that we're aborting the entire batch
        slog.ErrorContext(h.ctx, "Aborting batch due to critical error", 
            "source", srcId, "destination", dstId, 
            "error", err)
        
        // Signal to skip to the end of this pass
        h.skipCurrentPass = true
        h.batchStats.BatchesAborted++
        return false
    }
    
    if errors.Is(err, healing.ErrRetry) {
        count++
        if count >= 3 {
            slog.ErrorContext(h.ctx, "Anchor still pending after multiple attempts, skipping", 
                "source", srcId, "destination", dstId, 
                "number", seqNum, "attempts", count)
            h.batchStats.TxsFailed++
            return false
        }
        slog.InfoContext(h.ctx, "Anchor still pending, trying next anchor", 
            "source", srcId, "destination", dstId, 
            "number", seqNum, "attempt", count)
        goto retry
    }
    
    // For other errors, log and continue
    slog.ErrorContext(h.ctx, "Failed to heal anchor", 
        "source", srcId, "destination", dstId, 
        "number", seqNum, "error", err)
    h.batchStats.TxsFailed++
    return false
}
```

## 5. Expected Benefits

- Reduced network congestion during anchor healing operations
- More predictable resource consumption
- Improved overall network stability
- Better monitoring and control of anchor healing processes
- Reduced risk of transaction flooding

## 6. Potential Drawbacks and Mitigations

- **Slower Healing**: Limiting batch size may increase total healing time
  - *Mitigation*: Optimize batch processing and adjust batch size based on network conditions
  
- **Incomplete Healing**: Critical anchors might be delayed
  - *Mitigation*: Implement priority-based selection for anchors within the batch limit

## 7. Error Handling

Error handling is a critical aspect of the anchor healing process. The current implementation often aborts execution on errors, which can lead to incomplete healing and require manual intervention.

### 7.1 Error Classification

We will classify errors into two categories:

1. **Skip Transaction Errors**: Errors that allow processing to continue by skipping the current anchor
2. **Batch Abort Errors**: Critical errors that abort the entire batch and move to the next pass

### 7.2 Current Error Handling Issues

**File**: `/internal/core/healing/anchors.go`

1. **HealAnchor function** (lines 44-201):
   - Error handling for anchor resolution aborts the entire process
   ```go
   // Determine the anchor type
   var err error
   switch {
   case si.Source == protocol.Directory:
       // DN -> BVN
       err = healDnAnchorV2(ctx, args, si)
   case si.Destination == protocol.Directory:
       // BVN -> DN
       err = healAnchorV1(ctx, args, si)
   default:
       return fmt.Errorf("unknown anchor direction: %s -> %s", si.Source, si.Destination)
   }
   
   if err != nil {
       return err  // <-- Aborts execution instead of continuing with next anchor
   }
   ```

2. **healDnAnchorV2 function** (lines 426-533):
   - Multiple error checks that return immediately instead of logging and continuing
   - Example:
   ```go
   rBVN, err := ResolveSequenced[*messaging.TransactionMessage](ctx, args.Client, args.NetInfo, protocol.Directory, si.Destination, si.Number, true)
   if err != nil {
       return err  // <-- Aborts execution instead of continuing with next anchor
   }
   ```

### 7.3 Proposed Error Handling

The proposed solution will update error handling to classify errors and handle them appropriately:

1. **Network Errors**: Abort the entire batch since they likely affect all transactions
2. **Database Errors**: Abort the batch if critical, skip transaction if non-critical
3. **Transaction-specific Errors**: Skip the current transaction and continue with the batch

## 8. Implementation Examples

### 8.1 Error Types

```go
// Add to pkg/errors/errors.go
var (
    // SkipTransaction indicates that the current transaction should be skipped
    // but batch processing should continue
    SkipTransaction = Define("skip transaction")
    
    // BatchAbort indicates that the current batch should be aborted
    // and processing should skip to the next pass
    BatchAbort = Define("batch abort")
)
```

### 8.2 Core Functions

#### 1. `/internal/core/healing/anchors.go`

```go
// Modified HealAnchor function in internal/core/healing/anchors.go
func HealAnchor(ctx context.Context, args HealAnchorArgs, si SequencedInfo) error {
    // Determine the anchor type
    var err error
    switch {
    case si.Source == protocol.Directory:
        // DN -> BVN
        err = healDnAnchorV2(ctx, args, si)
    case si.Destination == protocol.Directory:
        // BVN -> DN
        err = healAnchorV1(ctx, args, si)
    default:
        // Log the error
        slog.ErrorContext(ctx, "Unknown anchor direction", 
            "source", si.Source, "destination", si.Destination)
        return errors.SkipTransaction.WithFormat("unknown anchor direction: %s -> %s", si.Source, si.Destination)
    }
    
    // Handle errors based on type
    if err != nil {
        // Check if this is a batch abort error
        if errors.Is(err, errors.BatchAbort) {
            return err
        }
        
        // For other errors, skip just this transaction
        return errors.SkipTransaction.WithFormat("anchor healing failed: %w", err)
    }
    
    return nil
}
```

### 8.3 Command Line Tools

```go
// Updated healSingleAnchor function in /tools/cmd/debug/heal_anchor.go
func healSingleAnchor(h *healer, srcId, dstId string, seqNum uint64, txid *url.TxID, txns map[[32]byte]*protocol.Transaction) bool {
    var count int
retry:
    err := healing.HealAnchor(h.ctx, healing.HealAnchorArgs{
        Client:    h.C2.ForAddress(nil),
        NetInfo:   h.net,
        Light:     h.light,
        Pretend:   pretend,
        Wait:      waitForTxn,

        // If an attempt fails, use the next anchor
        SkipAnchors: count,
        Txns:        txns,
    }, healing.SequencedInfo{
        Source:      srcId,
        Destination: dstId,
        Number:      seqNum,
        ID:          txid,
    })
    
    if err == nil {
        return true
    }
    
    if errors.Is(err, errors.Delivered) {
        return true
    }
    
    if errors.Is(err, errors.SkipTransaction) {
        // Log that we're skipping this transaction but continuing the batch
        slog.WarnContext(h.ctx, "Skipping anchor in batch", 
            "source", srcId, "destination", dstId, 
            "number", seqNum, "error", err)
        h.batchStats.TxsSkipped++
        return false
    }
    
    if errors.Is(err, errors.BatchAbort) {
        // Log that we're aborting the entire batch
        slog.ErrorContext(h.ctx, "Aborting batch due to critical error", 
            "source", srcId, "destination", dstId, 
            "error", err)
        
        // Signal to skip to the end of this pass
        h.skipCurrentPass = true
        h.batchStats.BatchesAborted++
        return false
    }
    
    // Rest of error handling...
    // ...
}
```

## 9. Configuration and Flags

### 9.1 Default Values

Default values are defined as constants to ensure consistency throughout the codebase:

```go
// Define constants for default values
const (
    defaultMaxBatchSize   = 20               // Default maximum transactions per batch
    defaultBatchesPerPass = 1                // Default number of batches per pass
    defaultPassInterval  = 30 * time.Second // Default interval between passes
)
```

### 9.2 Command Line Flags

These flags will be added to the anchor healing command:

```go
func init() {
    // Existing code...
    cmdHealAnchor.Flags().IntVar(&maxBatchSize, "batch-size", defaultMaxBatchSize, "Maximum transactions per batch")
    cmdHealAnchor.Flags().IntVar(&batchesPerPass, "batches-per-pass", defaultBatchesPerPass, "Number of batches per pass")
    cmdHealAnchor.Flags().DurationVar(&passInterval, "pass-interval", defaultPassInterval, "Interval between passes when running continuously")
    cmdHealAnchor.Flags().BoolVar(&continuous, "continuous", false, "Run healing continuously with intervals between passes")
}
```

### 9.3 Batch Statistics

Add a structure to track batch processing statistics:

```go
// BatchStats tracks statistics for batch processing
type BatchStats struct {
    BatchesProcessed int
    TxsProcessed     int
    TxsSucceeded     int
    TxsFailed        int
    TxsSkipped       int
    BatchesAborted   int
    LastBatchTime    time.Time
}
```

### 9.4 Testing Strategy

1. **Unit Tests**:
   - Add tests for the new error handling in `/internal/core/healing/anchors_test.go`
   - Test different error scenarios and verify proper handling
   - Ensure no data is fabricated during testing (as per critical rule in memory)

2. **Integration Tests**:
   - Test batch processing with various batch sizes
   - Verify that errors are properly logged and don't abort the entire process
   - Test continuous mode with pass intervals
   - Validate that on-demand transaction fetching follows the reference test code exactly

3. **Performance Tests**:
   - Measure the impact of batch processing on network load
   - Compare anchor healing performance before and after the changes
   - Test with different batch sizes to find optimal configuration

## 10. Implementation Plan

1. **Phase 1: Core Error Handling**
   - Add new error types to `/pkg/errors/errors.go`
   - Update error handling in anchor healing functions
   - Add unit tests for the new error handling

2. **Phase 2: Batch Processing**
   - Add batch processing types and configuration
   - Implement batch processing in anchor healing command line tools
   - Update reporting to include batch statistics

3. **Phase 3: Testing and Validation**
   - Test with various network conditions and transaction volumes
   - Validate error handling and batch processing
   - Optimize batch size and pass interval based on testing results

4. **Phase 4: Deployment**
   - Deploy to development environment
   - Monitor and gather feedback
   - Deploy to production in stages

## 11. Related Documents

- [Anchor Healing Documentation](../new_structure/03_core_components/04_healing/04_anchor_healing.md)
- [Transaction Exchange Improvements](../new_structure/08_future_development/03_transaction_exchange.md)
