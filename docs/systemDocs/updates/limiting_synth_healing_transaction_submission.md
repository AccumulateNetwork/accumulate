---
title: Limiting Synthetic Transaction Submission During Healing
description: Design for throttling and limiting synthetic transaction submission during healing processes
tags: [accumulate, healing, optimization, throttling, synthetic-transactions]
created: 2025-05-19
version: 0.1
status: draft
---

# Limiting Synthetic Transaction Submission During Healing

## 1. Introduction

This document outlines a design for limiting and throttling synthetic transaction submission during healing processes in the Accumulate network. The goal is to optimize network performance, reduce congestion, and ensure synthetic healing operations do not overwhelm the network.

## 2. Problem Statement

Currently, the synthetic healing process can potentially submit an unlimited number of transactions in rapid succession. This can lead to network congestion, increased resource consumption, and potential performance degradation across the network. As noted in our existing documentation, synthetic healing is a critical process that ensures protocol-generated transactions are properly delivered across all partitions. Without proper rate limiting, this process can overwhelm network resources.

## 3. Design Goals

- Limit the rate of synthetic transaction submission during healing processes
- Prevent network congestion from synthetic healing operations
- Maintain healing effectiveness while reducing resource consumption
- Provide consistent and predictable healing behavior
- Ensure proper error handling without fabricating data (as per critical rule in memory)

## 4. Proposed Solution

For the synthetic healing process, implement a strict batching mechanism that:
1. Limits submission to one batch per pass
2. Restricts each batch to a maximum of 20 transactions

### 4.1 Rate Limiting Mechanism

- **Batch Size Limit**: Set a hard limit of 20 synthetic transactions per batch
- **Batch Frequency Control**: Only allow one batch to be submitted per healing pass
- **Pass Completion**: A healing pass is considered complete when the batch has been fully processed
- **Error Handling**: Properly categorize errors to determine whether to skip individual transactions or abort the entire batch

### 4.2 Configurable Parameters

- `MaxBatchSize`: Maximum number of synthetic transactions in a batch (default: 20)
- `BatchesPerPass`: Number of batches allowed per healing pass (fixed at 1)
- `PassInterval`: Minimum time between healing passes (default: 30 seconds)

### 4.3 Implementation Details

The implementation will focus on modifying the synthetic healing process to support batch processing and improved error handling. Below are the key code changes required:

#### 4.3.1 Error Types

First, we need to add new error types to `/pkg/errors/errors.go`:

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

Modify the `HealSynthetic` function in `/internal/core/healing/synthetic.go` to handle errors appropriately:

```go
// Modified HealSynthetic function in internal/core/healing/synthetic.go
func (h *Healer) HealSynthetic(ctx context.Context, args HealSyntheticArgs, si SequencedInfo) error {
    if args.Querier == nil {
        args.Querier = args.Client
    }
    if args.Submitter == nil {
        args.Submitter = args.Client
    }

    // Query the synthetic transaction
    r, err := ResolveSequenced[messaging.Message](ctx, args.Client, args.NetInfo, si.Source, si.Destination, si.Number, false)
    if err != nil {
        // Log the error
        slog.ErrorContext(ctx, "Failed to resolve synthetic transaction", 
            "source", si.Source, "destination", si.Destination, 
            "number", si.Number, "error", err)
        
        // Check if this is a network-wide issue that would affect all transactions
        if errors.Is(err, errors.NetworkError) || errors.Is(err, errors.Unavailable) {
            return errors.BatchAbort.WithFormat("network error resolving transaction: %w", err)
        }
        
        // For other errors, skip just this transaction
        return errors.SkipTransaction.WithFormat("failed to resolve transaction: %w", err)
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
        slog.InfoContext(ctx, "Synthetic message has been delivered", "id", si.ID, "source", si.Source, "destination", si.Destination, "number", si.Number)
        return errors.Delivered
    }

    slog.InfoContext(ctx, "Resubmitting", "source", si.Source, "destination", si.Destination, "number", si.Number, "id", r.Message.ID())

    // Build the receipt
    var receipt *merkle.Receipt
    if args.NetInfo.Status.ExecutorVersion.V2VandenbergEnabled() {
        receipt, err = h.buildSynthReceiptV2(ctx, args, si)
    } else {
        receipt, err = h.buildSynthReceiptV1(ctx, args, si)
    }
    if err != nil {
        // Check if this is a batch abort error
        if errors.Is(err, errors.BatchAbort) {
            return err
        }
        
        // Log the error
        slog.ErrorContext(ctx, "Failed to build receipt for synthetic transaction", 
            "source", si.Source, "destination", si.Destination, 
            "number", si.Number, "error", err)
        
        // Return a special error that indicates this transaction should be skipped
        return errors.SkipTransaction.WithFormat("receipt building failed: %w", err)
    }
    
    // Rest of the function remains unchanged
    // ...
}
```

Modify the `buildSynthReceiptV2` function to classify errors:

```go
// Modified buildSynthReceiptV2 function in internal/core/healing/synthetic.go
func (h *Healer) buildSynthReceiptV2(_ context.Context, args HealSyntheticArgs, si SequencedInfo) (*merkle.Receipt, error) {
    batch := args.Light.OpenDB(false)
    defer batch.Discard()
    uSrc := protocol.PartitionUrl(si.Source)
    uSrcSys := uSrc.JoinPath(protocol.Ledger)
    uSrcSynth := uSrc.JoinPath(protocol.Synthetic)
    uDn := protocol.DnUrl()
    uDnSys := uDn.JoinPath(protocol.Ledger)
    uDnAnchor := uDn.JoinPath(protocol.AnchorPool)

    // Load the synthetic sequence chain entry
    b, err := batch.Account(uSrcSynth).SyntheticSequenceChain(si.Destination).Entry(int64(si.Number) - 1)
    if err != nil {
        // Check if this is a critical error that would affect all transactions
        if errors.Is(err, errors.DatabaseError) {
            // Critical error - abort the entire batch
            return nil, errors.BatchAbort.WithFormat(
                "critical database error loading synthetic sequence chain entry: %w", err)
        } else {
            // Non-critical error - skip just this transaction
            return nil, errors.SkipTransaction.WithFormat(
                "load synthetic sequence chain entry %d: %w", si.Number, err)
        }
    }
    
    // Rest of the function with similar error handling patterns
    // ...
}
```

#### 4.3.3 Command Line Implementation

Update `/tools/cmd/debug/heal_synth.go` to implement batch processing:

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
    cmdHealSynth.Flags().IntVar(&maxBatchSize, "batch-size", defaultMaxBatchSize, "Maximum transactions per batch")
    cmdHealSynth.Flags().IntVar(&batchesPerPass, "batches-per-pass", defaultBatchesPerPass, "Number of batches per pass")
    cmdHealSynth.Flags().DurationVar(&passInterval, "pass-interval", defaultPassInterval, "Interval between passes when running continuously")
    cmdHealSynth.Flags().BoolVar(&continuous, "continuous", false, "Run healing continuously with intervals between passes")
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

// Implement batch processing for synthetic healing
func healSynthBatch(h *healer, src, dst string, txsToHeal []TransactionInfo) {
    // Process only up to MaxBatchSize transactions
    batchSize := min(len(txsToHeal), maxBatchSize)
    
    slog.InfoContext(h.ctx, "Processing synthetic transaction batch", 
        "source", src, "destination", dst, "batch-size", batchSize)
    
    h.batchStats.BatchesProcessed++
    batchStart := time.Now()
    
    for i := 0; i < batchSize; i++ {
        // Check if we should skip the current pass
        if h.skipCurrentPass {
            slog.InfoContext(h.ctx, "Skipping remainder of current pass due to critical error")
            break
        }
        
        tx := txsToHeal[i]
        h.batchStats.TxsProcessed++
        
        success := healSingleSynth(h, src, dst, tx.Number, tx.ID)
        
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

Update the `healSingleSynth` function to handle the new error types:

```go
// Updated healSingleSynth function in /tools/cmd/debug/heal_synth.go
func healSingleSynth(h *healer, source, destination string, number uint64, id *url.TxID) bool {
    var count int
retry:
    err := h.HealSynthetic(h.ctx, healing.HealSyntheticArgs{
        Client:    h.C2.ForAddress(nil),
        Querier:   h.C2,
        Submitter: h.C2,
        NetInfo:   h.net,
        Light:     h.light,
        Pretend:   pretend,
        Wait:      waitForTxn,

        // If an attempt fails, use the next anchor
        SkipAnchors: count,
    }, healing.SequencedInfo{
        Source:      source,
        Destination: destination,
        Number:      number,
        ID:          id,
    })
    
    if err == nil {
        return false
    }
    
    if errors.Is(err, errors.Delivered) {
        return true
    }
    
    if errors.Is(err, errors.SkipTransaction) {
        // Log that we're skipping this transaction but continuing the batch
        slog.WarnContext(h.ctx, "Skipping transaction in batch", 
            "source", source, "destination", destination, 
            "number", number, "error", err)
        h.batchStats.TxsSkipped++
        return false
    }
    
    if errors.Is(err, errors.BatchAbort) {
        // Log that we're aborting the entire batch
        slog.ErrorContext(h.ctx, "Aborting batch due to critical error", 
            "source", source, "destination", destination, 
            "error", err)
        
        // Signal to skip to the end of this pass
        h.skipCurrentPass = true
        h.batchStats.BatchesAborted++
        return false
    }
    
    if errors.Is(err, healing.ErrRetry) {
        count++
        if count >= 3 {
            slog.ErrorContext(h.ctx, "Message still pending after multiple attempts, skipping", 
                "source", source, "destination", destination, 
                "number", number, "attempts", count)
            h.batchStats.TxsFailed++
            return false
        }
        slog.InfoContext(h.ctx, "Message still pending, trying next anchor", 
            "source", source, "destination", destination, 
            "number", number, "attempt", count)
        goto retry
    }
    
    // For other errors, log and continue
    slog.ErrorContext(h.ctx, "Failed to heal transaction", 
        "source", source, "destination", destination, 
        "number", number, "error", err)
    h.batchStats.TxsFailed++
    return false
}
```

## 5. Expected Benefits

- Reduced network congestion during healing operations
- More predictable resource consumption
- Improved overall network stability
- Better monitoring and control of healing processes
- Reduced risk of transaction flooding

## 6. Potential Drawbacks and Mitigations

- **Slower Healing**: Limiting batch size may increase total healing time
  - *Mitigation*: Optimize batch processing and adjust batch size based on network conditions
  
- **Incomplete Healing**: Critical transactions might be delayed
  - *Mitigation*: Implement priority-based selection for transactions within the batch limit

## 7. Error Handling

Error handling is a critical aspect of the synthetic healing process. The current implementation often aborts execution on errors, which can lead to incomplete healing and require manual intervention.

### 7.1 Error Classification

We will classify errors into two categories:

1. **Skip Transaction Errors**: Errors that allow processing to continue by skipping the current transaction
2. **Batch Abort Errors**: Critical errors that abort the entire batch and move to the next pass

### 7.2 Current Error Handling Issues

**File**: `/internal/core/healing/synthetic.go`

1. **HealSynthetic function** (lines 44-201):
   - Error handling for receipt building aborts the entire process
   ```go
   // Build the receipt
   var receipt *merkle.Receipt
   if args.NetInfo.Status.ExecutorVersion.V2VandenbergEnabled() {
       receipt, err = h.buildSynthReceiptV2(ctx, args, si)
   } else {
       receipt, err = h.buildSynthReceiptV1(ctx, args, si)
   }
   if err != nil {
       return err  // <-- Aborts execution instead of continuing with next transaction
   }
   ```

2. **buildSynthReceiptV2 function** (lines 426-533):
   - Multiple error checks that return immediately instead of logging and continuing
   - Example:
   ```go
   // Locate the synthetic ledger main chain index entry
   mainIndex, err := batch.Index().Account(uSrcSynth).Chain("main").SourceIndex().FindIndexEntryAfter(seqEntry.Source)
   if err != nil {
       return nil, errors.UnknownError.WithFormat(
           "locate synthetic ledger main chain index entry after %d: %w", seqEntry.Source, err)
   }
   ```

### 7.2 Anchor Healing Error Handling

**File**: `/internal/core/healing/anchors.go`

1. **healDnAnchorV2 function** (lines 49-110):
   - Error handling for resolving anchors aborts the entire process
   ```go
   // Resolve the anchor sent to the BVN
   rBVN, err := ResolveSequenced[*messaging.TransactionMessage](ctx, args.Client, args.NetInfo, protocol.Directory, si.Destination, si.Number, true)
   if err != nil {
       return err  // <-- Aborts execution instead of continuing
   }
   ```

2. **healAnchorV1 function** (lines 112-345):
   - Multiple error checks that return immediately instead of logging and continuing

### 7.3 Command Line Implementation

**File**: `/tools/cmd/debug/heal_synth.go`

1. **healSingleSynth function** (lines 90-128):
   - Limited retry logic (only retries 3 times) before aborting
   ```go
   count++
   if count >= 3 {
       slog.Error("Message still pending, skipping", "attempts", count)
       return false
   }
   ```

**File**: `/tools/cmd/debug/heal_anchor.go`

1. **healSingleAnchor method** (lines 72-114):
   - Limited retry logic (only retries 10 times) before aborting
   ```go
   count++
   if count >= 10 {
       slog.Error("Anchor still pending, skipping", "attempts", count)
       return false
   }
   ```

## 8. Error Handling Approach

For each identified error case, we need to implement a specific approach to log the error and maintain smooth processing. The approach will vary based on the error's scope and impact:

### 8.1 Synthetic Healing Error Handling Approach

#### 1. Core Healing Functions

**HealSynthetic function**:

```go
// Updated approach for receipt building error handling
var receipt *merkle.Receipt
if args.NetInfo.Status.ExecutorVersion.V2VandenbergEnabled() {
    receipt, err = h.buildSynthReceiptV2(ctx, args, si)
} else {
    receipt, err = h.buildSynthReceiptV1(ctx, args, si)
}
if err != nil {
    // Log the error
    slog.ErrorContext(ctx, "Failed to build receipt for synthetic transaction", 
        "source", si.Source, "destination", si.Destination, 
        "number", si.Number, "error", err)
    
    // Return a special error that indicates this transaction should be skipped
    // but batch processing should continue
    return errors.SkipTransaction.WithFormat("receipt building failed: %w", err)
}
```

**buildSynthReceiptV2 function**:

- Convert immediate returns to error collection and reporting
- Implement a mechanism to distinguish between critical errors (that should abort the entire batch) and non-critical errors (that should only skip the current transaction)

```go
// Example approach for handling errors in receipt building
mainIndex, err := batch.Index().Account(uSrcSynth).Chain("main").SourceIndex().FindIndexEntryAfter(seqEntry.Source)
if err != nil {
    // Check if this is a critical error that would affect all transactions
    if errors.Is(err, errors.DatabaseError) || errors.Is(err, errors.FatalError) {
        // Critical error - abort the entire batch
        return nil, errors.BatchAbort.WithFormat(
            "critical error locating synthetic ledger main chain index: %w", err)
    } else {
        // Non-critical error - skip just this transaction
        return nil, errors.SkipTransaction.WithFormat(
            "locate synthetic ledger main chain index entry after %d: %w", seqEntry.Source, err)
    }
}
```

#### 2. Command Line Implementation

**healSingleSynth function**:

```go
// Updated approach for handling errors in healSingleSynth
err := h.HealSynthetic(h.ctx, healing.HealSyntheticArgs{...})
if err != nil {
    if errors.Is(err, errors.Delivered) {
        return true
    }
    
    if errors.Is(err, errors.SkipTransaction) {
        // Log that we're skipping this transaction but continuing the batch
        slog.WarnContext(h.ctx, "Skipping transaction in batch", 
            "source", source, "destination", destination, 
            "number", number, "error", err)
        return false
    }
    
    if errors.Is(err, errors.BatchAbort) {
        // Log that we're aborting the entire batch
        slog.ErrorContext(h.ctx, "Aborting batch due to critical error", 
            "source", source, "destination", destination, 
            "error", err)
        
        // Signal to skip to the end of this pass
        h.skipCurrentPass = true
        return false
    }
    
    // For other errors, log and continue
    slog.ErrorContext(h.ctx, "Failed to heal transaction", 
        "source", source, "destination", destination, 
        "number", number, "error", err)
    return false
}
```

### 8.2 Anchor Healing Error Handling Approach

#### 1. Core Healing Functions

**healDnAnchorV2 function**:

```go
// Updated approach for handling errors in anchor resolution
rBVN, err := ResolveSequenced[*messaging.TransactionMessage](ctx, args.Client, args.NetInfo, protocol.Directory, si.Destination, si.Number, true)
if err != nil {
    // Log the error
    slog.ErrorContext(ctx, "Failed to resolve anchor for BVN", 
        "source", si.Source, "destination", si.Destination, 
        "number", si.Number, "error", err)
    
    // Check if this is a network-wide issue that would affect all anchors
    if errors.Is(err, errors.NetworkError) || errors.Is(err, errors.Unavailable) {
        return errors.BatchAbort.WithFormat("network error resolving anchor: %w", err)
    }
    
    // For other errors, skip just this anchor
    return errors.SkipTransaction.WithFormat("failed to resolve anchor: %w", err)
}
```

**healAnchorV1 function**:

- Apply similar error handling patterns as in healDnAnchorV2
- Distinguish between critical errors (affecting all anchors) and non-critical errors (affecting just the current anchor)

#### 2. Command Line Implementation

**healSingleAnchor method**:

```go
// Updated approach for handling errors in healSingleAnchor
err := healing.HealAnchor(h.ctx, healing.HealAnchorArgs{...})
if err != nil {
    if errors.Is(err, errors.Delivered) {
        return true
    }
    
    if errors.Is(err, errors.SkipTransaction) {
        // Log that we're skipping this anchor but continuing the batch
        slog.WarnContext(h.ctx, "Skipping anchor in batch", 
            "source", srcId, "destination", dstId, 
            "number", seqNum, "error", err)
        return false
    }
    
    if errors.Is(err, errors.BatchAbort) {
        // Log that we're aborting the entire batch
        slog.ErrorContext(h.ctx, "Aborting batch due to critical error", 
            "source", srcId, "destination", dstId, 
            "error", err)
        
        // Signal to skip to the end of this pass
        h.skipCurrentPass = true
        return false
    }
    
    // For retryable errors, implement limited retry logic
    if errors.Is(err, healing.ErrRetry) {
        count++
        if count >= 3 {
            slog.WarnContext(h.ctx, "Anchor still pending after multiple attempts, skipping", 
                "source", srcId, "destination", dstId, 
                "number", seqNum, "attempts", count)
            return false
        }
        slog.InfoContext(h.ctx, "Anchor still pending, retrying", 
            "source", srcId, "destination", dstId, 
            "number", seqNum, "attempt", count)
        goto retry
    }
    
    // For other errors, log and continue
    slog.ErrorContext(h.ctx, "Failed to heal anchor", 
        "source", srcId, "destination", dstId, 
        "number", seqNum, "error", err)
    return false
}
```

### 8.3 Batch Processing Implementation

To implement the batch processing approach with proper error handling:

1. **Add New Error Types**:
   ```go
   // Add to pkg/errors/errors.go
   var (
       SkipTransaction = Define("skip transaction")
       BatchAbort      = Define("batch abort")
   )
   ```

2. **Implement Batch Processing Logic**:
   ```go
   // Pseudocode for batch processing in heal_synth.go
   func healSynthBatch(h *healer, src, dst string, txsToHeal []TransactionInfo) {
       // Process only up to MaxBatchSize transactions
       batchSize := min(len(txsToHeal), MaxBatchSize)
       
       for i := 0; i < batchSize; i++ {
           // Check if we should skip the current pass
           if h.skipCurrentPass {
               slog.InfoContext(h.ctx, "Skipping remainder of current pass due to critical error")
               break
           }
           
           tx := txsToHeal[i]
           success := healSingleSynth(h, src, dst, tx.Number, tx.ID)
           
           // Update statistics
           if success {
               h.txDelivered++
           }
       }
       
       // If operating under continuous flag, implement delay between passes
       if continuous && h.skipCurrentPass {
           slog.InfoContext(h.ctx, "Entering delay period before next pass")
           time.Sleep(PassInterval)
           h.skipCurrentPass = false
       }
   }
   ```

3. **Update Main Healing Functions**:
   - Modify `healSynth` and `healAnchor` to use batch processing
   - Add batch statistics reporting
   - Implement pass interval delays

## 9. Implementation Details

### 9.1 Files Requiring Modification

The following files need to be modified to implement the batch processing and error handling improvements:

1. **Core Healing Files**:
   - `/internal/core/healing/synthetic.go` - Update synthetic healing functions
   - `/internal/core/healing/anchors.go` - Update anchor healing functions
   - `/internal/core/healing/types.go` - Add batch processing types and parameters

2. **Command Line Implementation Files**:
   - `/tools/cmd/debug/heal_synth.go` - Implement batch processing for synthetic healing
   - `/tools/cmd/debug/heal_anchor.go` - Implement batch processing for anchor healing
   - `/tools/cmd/debug/heal_common.go` - Add common batch processing utilities

3. **Error Handling Files**:
   - `/pkg/errors/errors.go` - Add new error types for batch processing

### 9.2 Import Changes and Dependencies

1. **New Imports Required**:
   - No new external dependencies are needed
   - Additional internal imports may include:
     ```go
     import (
         // Existing imports
         "time"      // For implementing delays between passes
         "sync"      // For synchronization primitives if needed
     )
     ```

2. **New Types and Interfaces**:
   - Add to `/internal/core/healing/types.go`:
     ```go
     // BatchConfig holds configuration for batch processing
     type BatchConfig struct {
         MaxBatchSize  int           // Maximum transactions per batch (default: 20)
         BatchesPerPass int          // Number of batches per pass (fixed at 1)
         PassInterval  time.Duration // Minimum time between passes
         Continuous    bool          // Whether to run continuously
     }
     
     // BatchStats tracks statistics for batch processing
     type BatchStats struct {
         BatchesProcessed   int       // Total batches processed
         TxsProcessed      int       // Total transactions processed
         TxsSucceeded      int       // Transactions successfully processed
         TxsFailed         int       // Transactions that failed
         TxsSkipped        int       // Transactions skipped due to errors
         BatchesAborted    int       // Batches aborted due to critical errors
         LastBatchTime     time.Time // Time of last batch completion
     }
     ```

3. **Error Type Additions**:
   - Add to `/pkg/errors/errors.go`:
     ```go
     var (
         // SkipTransaction indicates that the current transaction should be skipped
         // but batch processing should continue
         SkipTransaction = Define("skip transaction")
         
         // BatchAbort indicates that the current batch should be aborted
         // and processing should skip to the next pass
         BatchAbort = Define("batch abort")
     )
     ```

### 9.3 Detailed Modifications

#### 1. `/internal/core/healing/synthetic.go`

```go
// Modified HealSynthetic function in internal/core/healing/synthetic.go
func (h *Healer) HealSynthetic(ctx context.Context, args HealSyntheticArgs, si SequencedInfo) error {
    if args.Querier == nil {
        args.Querier = args.Client
    }
    if args.Submitter == nil {
        args.Submitter = args.Client
    }

    // Query the synthetic transaction
    r, err := ResolveSequenced[messaging.Message](ctx, args.Client, args.NetInfo, si.Source, si.Destination, si.Number, false)
    if err != nil {
        // Log the error
        slog.ErrorContext(ctx, "Failed to resolve synthetic transaction", 
            "source", si.Source, "destination", si.Destination, 
            "number", si.Number, "error", err)
        
        // Check if this is a network-wide issue that would affect all transactions
        if errors.Is(err, errors.NetworkError) || errors.Is(err, errors.Unavailable) {
            return errors.BatchAbort.WithFormat("network error resolving transaction: %w", err)
        }
        
        // For other errors, skip just this transaction
        return errors.SkipTransaction.WithFormat("failed to resolve transaction: %w", err)
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
        slog.InfoContext(ctx, "Synthetic message has been delivered", "id", si.ID, "source", si.Source, "destination", si.Destination, "number", si.Number)
        return errors.Delivered
    }

    slog.InfoContext(ctx, "Resubmitting", "source", si.Source, "destination", si.Destination, "number", si.Number, "id", r.Message.ID())

    // Build the receipt
    var receipt *merkle.Receipt
    if args.NetInfo.Status.ExecutorVersion.V2VandenbergEnabled() {
        receipt, err = h.buildSynthReceiptV2(ctx, args, si)
    } else {
        receipt, err = h.buildSynthReceiptV1(ctx, args, si)
    }
    if err != nil {
        // Check if this is a batch abort error
        if errors.Is(err, errors.BatchAbort) {
            return err
        }
        
        // Log the error
        slog.ErrorContext(ctx, "Failed to build receipt for synthetic transaction", 
            "source", si.Source, "destination", si.Destination, 
            "number", si.Number, "error", err)
        
        // Return a special error that indicates this transaction should be skipped
        return errors.SkipTransaction.WithFormat("receipt building failed: %w", err)
    }
    
    // Rest of the function remains unchanged
    // ...
}

// Modified buildSynthReceiptV2 function in internal/core/healing/synthetic.go
func (h *Healer) buildSynthReceiptV2(_ context.Context, args HealSyntheticArgs, si SequencedInfo) (*merkle.Receipt, error) {
    batch := args.Light.OpenDB(false)
    defer batch.Discard()
    uSrc := protocol.PartitionUrl(si.Source)
    uSrcSys := uSrc.JoinPath(protocol.Ledger)
    uSrcSynth := uSrc.JoinPath(protocol.Synthetic)
    uDn := protocol.DnUrl()
    uDnSys := uDn.JoinPath(protocol.Ledger)
    uDnAnchor := uDn.JoinPath(protocol.AnchorPool)

    // Load the synthetic sequence chain entry
    b, err := batch.Account(uSrcSynth).SyntheticSequenceChain(si.Destination).Entry(int64(si.Number) - 1)
    if err != nil {
        // Check if this is a critical error that would affect all transactions
        if errors.Is(err, errors.DatabaseError) {
            // Critical error - abort the entire batch
            return nil, errors.BatchAbort.WithFormat(
                "critical database error loading synthetic sequence chain entry: %w", err)
        } else {
            // Non-critical error - skip just this transaction
            return nil, errors.SkipTransaction.WithFormat(
                "load synthetic sequence chain entry %d: %w", si.Number, err)
        }
    }
    
    seqEntry := new(protocol.IndexEntry)
    err = seqEntry.UnmarshalBinary(b)
    if err != nil {
        return nil, errors.SkipTransaction.WithFormat("unmarshal sequence entry: %w", err)
    }

    // Locate the synthetic ledger main chain index entry
    mainIndex, err := batch.Index().Account(uSrcSynth).Chain("main").SourceIndex().FindIndexEntryAfter(seqEntry.Source)
    if err != nil {
        // Check if this is a critical error that would affect all transactions
        if errors.Is(err, errors.DatabaseError) {
            // Critical error - abort the entire batch
            return nil, errors.BatchAbort.WithFormat(
                "critical error locating synthetic ledger main chain index: %w", err)
        } else {
            // Non-critical error - skip just this transaction
            return nil, errors.SkipTransaction.WithFormat(
                "locate synthetic ledger main chain index entry after %d: %w", seqEntry.Source, err)
        }
    }
    
    // Rest of the function with similar error handling patterns
    // ...
}
```

#### 2. `/internal/core/healing/anchors.go`

```go
// Modify healDnAnchorV2 to handle errors appropriately
func healDnAnchorV2(ctx context.Context, args HealAnchorArgs, si SequencedInfo) error {
    // Existing code...
    
    // Resolve the anchor sent to the BVN
    rBVN, err := ResolveSequenced[*messaging.TransactionMessage](ctx, args.Client, args.NetInfo, protocol.Directory, si.Destination, si.Number, true)
    if err != nil {
        // Log the error
        slog.ErrorContext(ctx, "Failed to resolve anchor for BVN", 
            "source", si.Source, "destination", si.Destination, 
            "number", si.Number, "error", err)
        
        // Check if this is a network-wide issue that would affect all anchors
        if errors.Is(err, errors.NetworkError) || errors.Is(err, errors.Unavailable) {
            return errors.BatchAbort.WithFormat("network error resolving anchor: %w", err)
        }
        
        // For other errors, skip just this anchor
        return errors.SkipTransaction.WithFormat("failed to resolve anchor: %w", err)
    }
    
    // Existing code...
}
```

#### 3. `/tools/cmd/debug/heal_synth.go`

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
)

func init() {
    // Existing code...
    cmdHealSynth.Flags().IntVar(&maxBatchSize, "batch-size", defaultMaxBatchSize, "Maximum transactions per batch")
    cmdHealSynth.Flags().IntVar(&batchesPerPass, "batches-per-pass", defaultBatchesPerPass, "Number of batches per pass")
    cmdHealSynth.Flags().DurationVar(&passInterval, "pass-interval", defaultPassInterval, "Interval between passes when running continuously")
}

// Add batch processing to healer struct
type healer struct {
    // Existing fields...
    skipCurrentPass bool
    batchStats      healing.BatchStats
}

// Implement batch processing
func healSynthBatch(h *healer, src, dst string, txsToHeal []TransactionInfo) {
    // Process only up to MaxBatchSize transactions
    batchSize := min(len(txsToHeal), maxBatchSize)
    
    slog.InfoContext(h.ctx, "Processing synthetic transaction batch", 
        "source", src, "destination", dst, "batch-size", batchSize)
    
    h.batchStats.BatchesProcessed++
    batchStart := time.Now()
    
    for i := 0; i < batchSize; i++ {
        // Check if we should skip the current pass
        if h.skipCurrentPass {
            slog.InfoContext(h.ctx, "Skipping remainder of current pass due to critical error")
            break
        }
        
        tx := txsToHeal[i]
        h.batchStats.TxsProcessed++
        
        success := healSingleSynth(h, src, dst, tx.Number, tx.ID)
        
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

// Update healSingleSynth to handle new error types
func healSingleSynth(h *healer, source, destination string, number uint64, id *url.TxID) bool {
    // Existing code...
    
    err := h.HealSynthetic(h.ctx, healing.HealSyntheticArgs{...})
    if err != nil {
        if errors.Is(err, errors.Delivered) {
            return true
        }
        
        if errors.Is(err, errors.SkipTransaction) {
            // Log that we're skipping this transaction but continuing the batch
            slog.WarnContext(h.ctx, "Skipping transaction in batch", 
                "source", source, "destination", destination, 
                "number", number, "error", err)
            h.batchStats.TxsSkipped++
            return false
        }
        
        if errors.Is(err, errors.BatchAbort) {
            // Log that we're aborting the entire batch
            slog.ErrorContext(h.ctx, "Aborting batch due to critical error", 
                "source", source, "destination", destination, 
                "error", err)
            
            // Signal to skip to the end of this pass
            h.skipCurrentPass = true
            h.batchStats.BatchesAborted++
            return false
        }
        
        // For other errors, log and continue
        slog.ErrorContext(h.ctx, "Failed to heal transaction", 
            "source", source, "destination", destination, 
            "number", number, "error", err)
        h.batchStats.TxsFailed++
        return false
    }
    
    // Existing code...
}
```

#### 4. `/tools/cmd/debug/heal_anchor.go`

Similar changes as for `heal_synth.go`, implementing batch processing and error handling for anchors.

### 9.4 Testing Strategy

1. **Unit Tests**:
   - Add tests for the new error handling in `/internal/core/healing/synthetic_test.go`
   - Test different error scenarios and verify proper handling
   - Ensure no data is fabricated during testing (as per critical rule in memory)

2. **Integration Tests**:
   - Test batch processing with various batch sizes
   - Verify that errors are properly logged and don't abort the entire process
   - Test continuous mode with pass intervals
   - Validate that on-demand transaction fetching follows the reference test code exactly

3. **Performance Tests**:
   - Measure the impact of batch processing on network load
   - Compare synthetic healing performance before and after the changes
   - Test with different batch sizes to find optimal configuration

## 10. Implementation Plan

1. **Phase 1: Core Error Handling**
   - Add new error types to `/pkg/errors/errors.go`
   - Update error handling in synthetic healing functions
   - Add unit tests for the new error handling

2. **Phase 2: Batch Processing**
   - Add batch processing types and configuration
   - Implement batch processing in synthetic healing command line tools
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

- [Synthetic Healing Documentation](../new_structure/03_core_components/04_healing/03_synthetic_healing.md)
- [Transaction Exchange Improvements](../new_structure/08_future_development/03_transaction_exchange.md)
