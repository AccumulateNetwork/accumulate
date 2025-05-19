---
title: Synthetic Healing Test Design
description: Comprehensive testing strategy for synthetic transaction healing with batch processing
tags: [accumulate, healing, synthetic-transactions, testing, batch-processing]
created: 2025-05-19
version: 0.1
status: draft
---

# Synthetic Healing Test Design

## 1. Introduction

This document outlines a comprehensive testing strategy for the synthetic transaction healing process in Accumulate, with a focus on the new batch processing and error handling features. The goal is to ensure that the healing process is reliable, efficient, and resilient to various error conditions.

## 2. Test Objectives

- Verify that batch processing correctly limits transaction submission
- Validate error handling for different error scenarios
- Ensure healing effectiveness is maintained with the new limitations
- Confirm that no data is fabricated during the healing process
- Measure performance impact of batch processing

## 3. Test Environment Setup

### 3.1 Local Development Environment

- Standard Go development environment
- Local Accumulate network with multiple partitions (at least 1 DN and 2 BVNs)
- Controlled network conditions for testing error scenarios

### 3.2 Simulator Environment

- Accumulate network simulator with configurable network conditions
- Ability to inject synthetic transactions and simulate errors
- Monitoring tools for tracking batch processing statistics

## 4. Test Categories

### 4.1 Unit Tests

Unit tests will focus on individual components of the synthetic healing process, particularly the new error handling and batch processing logic.

#### 4.1.1 Error Handling Tests

| Test ID | Description | Expected Outcome |
|---------|-------------|------------------|
| ERR-001 | Test SkipTransaction error handling | Verify transaction is skipped but batch continues |
| ERR-002 | Test BatchAbort error handling | Verify batch is aborted and next pass is started |
| ERR-003 | Test network error classification | Verify network errors trigger BatchAbort |
| ERR-004 | Test database error classification | Verify critical DB errors trigger BatchAbort |
| ERR-005 | Test transaction-specific error classification | Verify transaction errors trigger SkipTransaction |

#### 4.1.2 Batch Processing Tests

| Test ID | Description | Expected Outcome |
|---------|-------------|------------------|
| BAT-001 | Test batch size enforcement | Verify no more than MaxBatchSize transactions are processed |
| BAT-002 | Test batches per pass enforcement | Verify only one batch is processed per pass |
| BAT-003 | Test pass interval enforcement | Verify minimum time between passes is respected |
| BAT-004 | Test batch statistics tracking | Verify statistics are accurately tracked |
| BAT-005 | Test batch processing with empty batch | Verify handling of empty batches |

### 4.2 Integration Tests

Integration tests will verify the interaction between different components of the healing process and ensure that the system works correctly as a whole.

#### 4.2.1 End-to-End Healing Tests

| Test ID | Description | Expected Outcome |
|---------|-------------|------------------|
| E2E-001 | Test healing with valid synthetic transactions | Verify all transactions are processed successfully |
| E2E-002 | Test healing with mix of valid and invalid transactions | Verify valid transactions are processed and invalid ones are skipped |
| E2E-003 | Test healing with network errors | Verify batch is aborted and retried after delay |
| E2E-004 | Test healing with database errors | Verify appropriate error handling based on error type |
| E2E-005 | Test continuous healing mode | Verify healing continues with appropriate delays between passes |

#### 4.2.2 Command Line Interface Tests

| Test ID | Description | Expected Outcome |
|---------|-------------|------------------|
| CLI-001 | Test batch-size flag | Verify flag correctly limits batch size |
| CLI-002 | Test batches-per-pass flag | Verify flag correctly limits batches per pass |
| CLI-003 | Test pass-interval flag | Verify flag correctly sets interval between passes |
| CLI-004 | Test continuous flag | Verify flag enables continuous healing mode |
| CLI-005 | Test with various combinations of flags | Verify flags work correctly together |

### 4.3 Performance Tests

Performance tests will measure the impact of batch processing on network load and healing effectiveness.

#### 4.3.1 Batch Size Impact Tests

| Test ID | Description | Expected Outcome |
|---------|-------------|------------------|
| PERF-001 | Test with various batch sizes (5, 10, 20, 50) | Measure impact on network load and healing time |
| PERF-002 | Test with very large batch size (100+) | Measure impact on network stability |
| PERF-003 | Test with very small batch size (1-2) | Measure impact on healing efficiency |
| PERF-004 | Test batch size under high network load | Verify system stability under stress |
| PERF-005 | Test batch size with various transaction types | Measure impact of transaction complexity |

#### 4.3.2 Pass Interval Impact Tests

| Test ID | Description | Expected Outcome |
|---------|-------------|------------------|
| PERF-006 | Test with various pass intervals (10s, 30s, 60s) | Measure impact on network load and healing time |
| PERF-007 | Test with very short pass interval (1s) | Measure impact on network stability |
| PERF-008 | Test with very long pass interval (5min+) | Measure impact on healing efficiency |
| PERF-009 | Test pass interval under high network load | Verify system stability under stress |
| PERF-010 | Test pass interval with various transaction volumes | Measure impact of transaction volume |

## 5. Test Implementation

### 5.1 Unit Test Implementation

Unit tests will be implemented in `/internal/core/healing/synthetic_test.go` using the standard Go testing framework. Mock objects will be used to simulate various error conditions.

```go
// Example test for SkipTransaction error handling
func TestSkipTransactionErrorHandling(t *testing.T) {
    // Setup test environment
    ctx := context.Background()
    mockClient := &mockClient{}
    mockNetInfo := &mockNetworkInfo{}
    
    // Create a test case that will trigger a SkipTransaction error
    args := healing.HealSyntheticArgs{
        Client:  mockClient,
        NetInfo: mockNetInfo,
        // Configure to trigger a non-critical error
    }
    
    si := healing.SequencedInfo{
        Source:      "bvn1",
        Destination: "bvn2",
        Number:      123,
    }
    
    // Create healer with mock dependencies
    h := &healing.Healer{}
    
    // Execute the function under test
    err := h.HealSynthetic(ctx, args, si)
    
    // Verify error is classified as SkipTransaction
    require.True(t, errors.Is(err, errors.SkipTransaction))
    
    // Verify the error is properly wrapped with context
    require.Contains(t, err.Error(), "failed to resolve transaction")
}

// Example test for batch size enforcement
func TestBatchSizeEnforcement(t *testing.T) {
    // Setup test environment with a large number of pending transactions
    ctx := context.Background()
    h := &healer{
        // Initialize with test dependencies
    }
    
    // Create a list of 50 transactions to heal
    txsToHeal := make([]TransactionInfo, 50)
    for i := 0; i < 50; i++ {
        txsToHeal[i] = TransactionInfo{
            Number: uint64(i + 1),
            ID:     nil, // Set appropriate ID
        }
    }
    
    // Set max batch size to 20
    maxBatchSize = 20
    
    // Call the batch processing function
    healSynthBatch(h, "bvn1", "bvn2", txsToHeal)
    
    // Verify only 20 transactions were processed
    require.Equal(t, 20, h.batchStats.TxsProcessed)
}
```

### 5.2 Integration Test Implementation

Integration tests will be implemented in a separate test package that sets up a complete healing environment with real or simulated network components.

```go
// Example end-to-end test for healing with mixed valid/invalid transactions
func TestHealingWithMixedTransactions(t *testing.T) {
    // Setup test environment with real components
    network := setupTestNetwork(t)
    defer network.Shutdown()
    
    // Create a mix of valid and invalid synthetic transactions
    validTxs := createValidSyntheticTransactions(10)
    invalidTxs := createInvalidSyntheticTransactions(10)
    
    // Mix them together
    allTxs := append(validTxs, invalidTxs...)
    
    // Configure batch processing
    maxBatchSize = 20
    batchesPerPass = 1
    
    // Run the healing process
    h := setupHealer(network)
    stats := runHealingProcess(h, allTxs)
    
    // Verify results
    require.Equal(t, 20, stats.TxsProcessed)
    require.Equal(t, 10, stats.TxsSucceeded)
    require.Equal(t, 0, stats.TxsFailed)
    require.Equal(t, 10, stats.TxsSkipped)
    require.Equal(t, 0, stats.BatchesAborted)
}

// Example CLI test for batch size flag
func TestBatchSizeFlag(t *testing.T) {
    // Setup test command
    cmd := exec.Command("accumulate", "debug", "heal-synth", 
        "--batch-size=15", "testnet", "bvn1", "bvn2")
    
    // Capture output
    output, err := cmd.CombinedOutput()
    require.NoError(t, err)
    
    // Verify batch size was set correctly
    require.Contains(t, string(output), "batch-size: 15")
    
    // Verify only 15 transactions were processed per batch
    // This would require parsing the output to find batch statistics
    stats := parseBatchStats(string(output))
    require.LessOrEqual(t, stats.TxsPerBatch, 15)
}
```

### 5.3 Performance Test Implementation

Performance tests will be implemented using the Accumulate simulator and will measure various metrics under different configurations.

```go
// Example performance test for batch size impact
func TestBatchSizePerformance(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping performance test in short mode")
    }
    
    // Test different batch sizes
    batchSizes := []int{5, 10, 20, 50}
    results := make(map[int]PerformanceMetrics)
    
    for _, size := range batchSizes {
        // Setup test environment with 100 pending transactions
        sim := simulator.New(t)
        defer sim.Close()
        
        // Create 100 synthetic transactions
        txs := createSyntheticTransactions(sim, 100)
        
        // Configure healing with current batch size
        maxBatchSize = size
        batchesPerPass = 1
        passInterval = 30 * time.Second
        
        // Measure performance
        start := time.Now()
        stats := runHealingProcess(sim, txs)
        duration := time.Since(start)
        
        // Collect metrics
        metrics := PerformanceMetrics{
            BatchSize:         size,
            TotalTime:         duration,
            NetworkBandwidth:  sim.MeasureNetworkBandwidth(),
            CPUUsage:          sim.MeasureCPUUsage(),
            MemoryUsage:       sim.MeasureMemoryUsage(),
            TransactionsHealed: stats.TxsSucceeded,
        }
        
        results[size] = metrics
    }
    
    // Analyze and report results
    analyzePerformanceResults(t, results)
}
```

## 6. Test Data Management

### 6.1 Test Data Generation

Test data will be generated using the following approaches:

1. **Valid Synthetic Transactions**: Create valid synthetic transactions between partitions
2. **Invalid Synthetic Transactions**: Create synthetic transactions with deliberate errors
3. **Network Error Simulation**: Simulate network errors during transaction resolution
4. **Database Error Simulation**: Simulate database errors during receipt building

### 6.2 Test Data Validation

All test data will be validated to ensure:

1. No data is fabricated during testing (as per critical rule)
2. Test data accurately represents real-world scenarios
3. Test data covers all error conditions and edge cases

## 7. Test Execution Strategy

### 7.1 Continuous Integration

- Unit tests will be run on every commit
- Integration tests will be run on pull requests
- Performance tests will be run nightly

### 7.2 Manual Testing

- Complex error scenarios will be tested manually
- Edge cases that are difficult to automate will be tested manually
- New features will undergo manual testing before automation

### 7.3 Test Reporting

- Test results will be reported in CI/CD pipeline
- Performance test results will be tracked over time
- Test coverage will be monitored and maintained above 80%

## 8. Specialized Test Scenarios

### 8.1 Error Injection Testing

To thoroughly test error handling, we will inject errors at various points in the healing process:

1. **Network Errors**: Simulate network partitions, timeouts, and connection failures
2. **Database Errors**: Simulate database corruption, missing entries, and access failures
3. **Transaction Errors**: Simulate malformed transactions, invalid signatures, and sequence errors

### 8.2 Chaos Testing

Introduce random failures and delays to test system resilience:

1. **Random Transaction Failures**: Randomly fail a percentage of transactions
2. **Random Network Delays**: Introduce random network latency
3. **Random Process Restarts**: Restart healing process during execution

### 8.3 Long-Running Tests

Test the system over extended periods to identify issues that may not appear in short tests:

1. **24-Hour Test**: Run healing continuously for 24 hours with periodic transaction injection
2. **Weekend Test**: Run healing over a weekend with varying load patterns
3. **Recovery Test**: Test recovery after extended downtime

## 9. Test Validation Criteria

### 9.1 Functional Validation

- All transactions that should be healed are healed
- No transactions are processed more than once
- Errors are properly classified and handled
- Batch size and pass interval limits are enforced

### 9.2 Performance Validation

- Network load stays within acceptable limits
- CPU and memory usage stay within acceptable limits
- Healing completes within expected timeframes
- System remains stable under continuous operation

### 9.3 Error Handling Validation

- SkipTransaction errors allow processing to continue
- BatchAbort errors properly abort the batch
- Error messages contain useful diagnostic information
- Error statistics are accurately tracked

## 10. Test Tooling

### 10.1 Custom Test Utilities

Develop custom test utilities to support testing:

1. **Synthetic Transaction Generator**: Generate valid and invalid synthetic transactions
2. **Error Injector**: Inject errors at specific points in the healing process
3. **Performance Monitor**: Track system performance during testing
4. **Batch Statistics Analyzer**: Analyze batch processing statistics

### 10.2 Test Automation Framework

Implement a test automation framework that:

1. Sets up test environments automatically
2. Executes test cases in the correct order
3. Collects and reports test results
4. Cleans up test environments after testing

## 11. Implementation Notes

### 11.1 Critical Rules

1. **No Data Fabrication**: As per the critical rule in memory, no data should be fabricated during testing. All data must be retrieved from the network or explicitly created for testing purposes.
2. **Reference Implementation**: Testing should follow the reference implementation in anchor_synth_report_test.go exactly.
3. **Error Transparency**: Errors should be properly logged and not masked.

### 11.2 Test Dependencies

- Go testing framework
- Accumulate simulator
- Mock objects for network and database components
- Performance monitoring tools

## 12. AI-Optimized Implementation Plan

This implementation plan is structured as a series of self-contained modules, each with clear inputs, outputs, and verification steps. This approach allows an AI to focus on one task at a time while maintaining context of the overall goal.

### 12.1 Module: Error Types

**Purpose**: Create error types for batch processing and error handling.

**Files to Modify**:
- `/pkg/errors/errors.go`

**Implementation Steps**:
```go
// Add these error types to errors.go
var (
    // SkipTransaction indicates that the current transaction should be skipped
    // but batch processing should continue
    SkipTransaction = Define("skip transaction")
    
    // BatchAbort indicates that the current batch should be aborted
    // and processing should skip to the next pass
    BatchAbort = Define("batch abort")
)
```

**Test Cases**:
1. Verify error wrapping: `errors.SkipTransaction.WithFormat("reason: %w", err)`
2. Verify error unwrapping: `errors.Is(wrappedErr, errors.SkipTransaction)`

### 12.2 Module: Core Healing Error Classification

**Purpose**: Update core healing logic to classify errors appropriately.

**Files to Modify**:
- `/internal/core/healing/synthetic.go`

**Implementation Steps**:
```go
// Update HealSynthetic to classify errors
func (h *Healer) HealSynthetic(ctx context.Context, args HealSyntheticArgs, si SequencedInfo) error {
    // Existing code...
    
    r, err := ResolveSequenced[messaging.Message](ctx, args.Client, args.NetInfo, si.Source, si.Destination, si.Number, false)
    if err != nil {
        // Log the error
        slog.ErrorContext(ctx, "Failed to resolve synthetic transaction", 
            "source", si.Source, "destination", si.Destination, 
            "number", si.Number, "error", err)
        
        // Check if this is a network-wide issue
        if errors.Is(err, errors.NetworkError) || errors.Is(err, errors.Unavailable) {
            return errors.BatchAbort.WithFormat("network error: %w", err)
        }
        
        // For other errors, skip just this transaction
        return errors.SkipTransaction.WithFormat("failed to resolve: %w", err)
    }
    
    // Similar updates for other error handling points
}
```

**Test Cases**:
1. Test network error classification returns BatchAbort
2. Test transaction-specific errors return SkipTransaction
3. Verify error messages contain useful diagnostic information

### 12.3 Module: Receipt Building Error Classification

**Purpose**: Update receipt building functions to classify errors.

**Files to Modify**:
- `/internal/core/healing/synthetic.go`

**Implementation Steps**:
```go
// Update buildSynthReceiptV2 to classify errors
func (h *Healer) buildSynthReceiptV2(ctx context.Context, args HealSyntheticArgs, si SequencedInfo) (*merkle.Receipt, error) {
    // Existing code...
    
    b, err := batch.Account(uSrcSynth).SyntheticSequenceChain(si.Destination).Entry(int64(si.Number) - 1)
    if err != nil {
        // Check if this is a critical database error
        if errors.Is(err, errors.DatabaseError) {
            return nil, errors.BatchAbort.WithFormat(
                "critical database error: %w", err)
        } else {
            return nil, errors.SkipTransaction.WithFormat(
                "load sequence chain entry %d: %w", si.Number, err)
        }
    }
    
    // Similar updates for other error handling points
}
```

**Test Cases**:
1. Test database error classification
2. Verify no data fabrication occurs during error handling

### 12.4 Module: Transaction Info Structure

**Purpose**: Add structures for batch processing of transactions.

**Files to Modify**:
- `/tools/cmd/debug/heal_common.go`

**Implementation Steps**:
```go
// Add TransactionInfo struct
type TransactionInfo struct {
    Number uint64
    ID     *url.TxID
}

// Add AnchorInfo struct
type AnchorInfo struct {
    Number uint64
    ID     *url.TxID
    Txns   map[[32]byte]*protocol.Transaction
}

// Add helper function
func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}
```

**Test Cases**:
1. Verify TransactionInfo correctly stores transaction data
2. Test min function with various inputs

### 12.5 Module: Batch Processing Configuration

**Purpose**: Add configuration options for batch processing.

**Files to Modify**:
- `/tools/cmd/debug/heal_synth.go`
- `/tools/cmd/debug/heal_anchor.go`

**Implementation Steps**:
```go
// Add to both files
const (
    defaultMaxBatchSize   = 20
    defaultBatchesPerPass = 1
    defaultPassInterval   = 30 * time.Second
)

var (
    maxBatchSize   int
    batchesPerPass int
    passInterval   time.Duration
    continuous     bool
    skipCurrentPass bool
)

// Update init function
func init() {
    // Existing code...
    cmd.Flags().IntVar(&maxBatchSize, "batch-size", defaultMaxBatchSize, "Maximum transactions per batch")
    cmd.Flags().IntVar(&batchesPerPass, "batches-per-pass", defaultBatchesPerPass, "Number of batches per pass")
    cmd.Flags().DurationVar(&passInterval, "pass-interval", defaultPassInterval, "Interval between passes when running continuously")
    cmd.Flags().BoolVar(&continuous, "continuous", false, "Run healing continuously with intervals between passes")
}
```

**Test Cases**:
1. Test flag parsing with various inputs
2. Verify default values are used when flags are not specified

### 12.6 Module: Synthetic Healing Batch Processing

**Purpose**: Implement batch processing for synthetic healing.

**Files to Modify**:
- `/tools/cmd/debug/heal_synth.go`

**Implementation Steps**:
```go
// Add batch processing function
func healSynthBatch(h *healer, src, dst string, txsToHeal []TransactionInfo) {
    batchSize := min(len(txsToHeal), maxBatchSize)
    
    slog.InfoContext(h.ctx, "Processing synthetic transaction batch", 
        "source", src, "destination", dst, "batch-size", batchSize)
    
    for i := 0; i < batchSize; i++ {
        if skipCurrentPass {
            slog.InfoContext(h.ctx, "Skipping remainder of current pass due to critical error")
            break
        }
        
        tx := txsToHeal[i]
        success := healSingleSynth(h, src, dst, tx.Number, tx.ID)
        
        if success {
            slog.InfoContext(h.ctx, "Successfully healed transaction", 
                "source", src, "destination", dst, "number", tx.Number)
        }
    }
    
    // Reset skip flag if in continuous mode
    if continuous && skipCurrentPass {
        time.Sleep(passInterval)
        skipCurrentPass = false
    }
}
```

**Test Cases**:
1. Test batch processing with various batch sizes
2. Verify batch processing stops on BatchAbort errors
3. Test continuous mode with pass intervals

### 12.7 Module: Synthetic Healing Error Handling

**Purpose**: Update synthetic healing to handle new error types.

**Files to Modify**:
- `/tools/cmd/debug/heal_synth.go`

**Implementation Steps**:
```go
// Update healSingleSynth function
func healSingleSynth(h *healer, source, destination string, number uint64, id *url.TxID) bool {
    var count int
retry:
    err := h.HealSynthetic(h.ctx, healing.HealSyntheticArgs{
        // Existing arguments...
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
        slog.WarnContext(h.ctx, "Skipping transaction in batch", 
            "source", source, "destination", destination, 
            "number", number, "error", err)
        return false
    }
    
    if errors.Is(err, errors.BatchAbort) {
        slog.ErrorContext(h.ctx, "Aborting batch due to critical error", 
            "source", source, "destination", destination, 
            "error", err)
        skipCurrentPass = true
        return false
    }
    
    // Existing retry logic...
}
```

**Test Cases**:
1. Test handling of SkipTransaction errors
2. Test handling of BatchAbort errors
3. Verify retry logic works with new error types

### 12.8 Module: Anchor Healing Error Classification

**Purpose**: Update core anchor healing logic to classify errors appropriately.

**Files to Modify**:
- `/internal/core/healing/anchors.go`

**Implementation Steps**:
```go
// Update HealAnchor to classify errors
func HealAnchor(ctx context.Context, args HealAnchorArgs, si SequencedInfo) error {
    // Existing code...
    
    // Update error handling
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

// Update healDnAnchorV2 to classify errors
func healDnAnchorV2(ctx context.Context, args HealAnchorArgs, si SequencedInfo) error {
    // Existing code...
    
    // Example error handling for transaction fetching
    txn, err := args.Querier.QueryTransaction(ctx, &api.TransactionQueryRequest{
        TxID: si.ID,
    })
    if err != nil {
        // Check if this is a network-wide issue
        if errors.Is(err, errors.NetworkError) || errors.Is(err, errors.Unavailable) {
            return errors.BatchAbort.WithFormat("network error fetching transaction: %w", err)
        }
        
        // For key not found errors, follow the reference implementation approach
        // without fabricating data
        if errors.Is(err, errors.NotFound) {
            slog.WarnContext(ctx, "Transaction not found, attempting to fetch by sequence number", 
                "source", si.Source, "destination", si.Destination, 
                "number", si.Number)
            
            // Use sequence number to fetch transaction as per reference implementation
            // Do not fabricate data if not available
        }
        
        // For other errors, skip just this transaction
        return errors.SkipTransaction.WithFormat("failed to fetch transaction: %w", err)
    }
    
    // Similar updates for other error handling points
    return nil
}

// Update healAnchorV1 to classify errors (similar approach)
func healAnchorV1(ctx context.Context, args HealAnchorArgs, si SequencedInfo) error {
    // Similar error classification as healDnAnchorV2
    return nil
}
```

**Test Cases**:
1. Test network error classification returns BatchAbort
2. Test transaction-specific errors return SkipTransaction
3. Test handling of "key not found" errors follows reference implementation
4. Verify no data fabrication occurs when transactions aren't found
5. Verify error messages contain useful diagnostic information

### 12.9 Module: Anchor Healing Batch Processing

**Purpose**: Implement batch processing for anchor healing.

**Files to Modify**:
- `/tools/cmd/debug/heal_anchor.go`

**Implementation Steps**:
```go
// Add batch processing function
func healAnchorBatch(h *healer, src, dst string, anchorsToHeal []AnchorInfo) {
    batchSize := min(len(anchorsToHeal), maxBatchSize)
    
    slog.InfoContext(h.ctx, "Processing anchor transaction batch", 
        "source", src, "destination", dst, "batch-size", batchSize)
    
    for i := 0; i < batchSize; i++ {
        if skipCurrentPass {
            slog.InfoContext(h.ctx, "Skipping remainder of current pass due to critical error")
            break
        }
        
        anchor := anchorsToHeal[i]
        success := h.healSingleAnchor(src, dst, anchor.Number, anchor.ID, anchor.Txns)
        
        if success {
            slog.InfoContext(h.ctx, "Successfully healed anchor", 
                "source", src, "destination", dst, "number", anchor.Number)
        }
    }
    
    // Reset skip flag if in continuous mode
    if continuous && skipCurrentPass {
        time.Sleep(passInterval)
        skipCurrentPass = false
    }
}
```

**Test Cases**:
1. Test batch processing with various batch sizes
2. Verify batch processing stops on BatchAbort errors
3. Test continuous mode with pass intervals

### 12.10 Module: Anchor Healing Error Handling

**Purpose**: Update anchor healing to handle new error types.

**Files to Modify**:
- `/tools/cmd/debug/heal_anchor.go`

**Implementation Steps**:
```go
// Update healSingleAnchor function
func (h *healer) healSingleAnchor(srcId, dstId string, seqNum uint64, txid *url.TxID, txns map[[32]byte]*protocol.Transaction) bool {
    var count int
retry:
    err := healing.HealAnchor(h.ctx, healing.HealAnchorArgs{
        Client:  h.C2.ForAddress(nil),
        Querier: h.tryEach(),
        NetInfo: h.net,
        Known:   txns,
        Pretend: pretend,
        Wait:    waitForTxn,
        Submit: func(m ...messaging.Message) error {
            select {
            case h.submit <- m:
                return nil
            case <-h.ctx.Done():
                return errors.NotReady.With("canceled")
            }
        },
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
        slog.WarnContext(h.ctx, "Skipping anchor in batch", 
            "source", srcId, "destination", dstId, 
            "number", seqNum, "error", err)
        return false
    }
    
    if errors.Is(err, errors.BatchAbort) {
        slog.ErrorContext(h.ctx, "Aborting batch due to critical error", 
            "source", srcId, "destination", dstId, 
            "error", err)
        skipCurrentPass = true
        return false
    }
    
    // Existing retry logic for anchor healing
    if errors.Is(err, healing.ErrRetry) {
        count++
        if count >= 10 {
            slog.ErrorContext(h.ctx, "Anchor still pending after multiple attempts, skipping", 
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

**Test Cases**:
1. Test handling of SkipTransaction errors
2. Test handling of BatchAbort errors
3. Verify retry logic works with new error types
4. Test handling of errors.Delivered
5. Test handling of healing.ErrRetry with retry limits

### 12.9 Module: Integration Testing

**Purpose**: Test the complete healing process with batch processing.

**Test Implementation**:
```go
// Example integration test
func TestBatchHealingIntegration(t *testing.T) {
    // Setup test environment
    // ...
    
    // Create a mix of valid and invalid transactions
    // ...
    
    // Run batch healing
    // ...
    
    // Verify results
    // - Check that valid transactions were healed
    // - Check that invalid transactions were skipped
    // - Verify no data fabrication occurred
}
```

**Test Scenarios**:
1. All transactions succeed
2. Mix of successful and skipped transactions
3. Batch abort due to critical error
4. Recovery after batch abort

### 12.10 Verification Checklist

**For Each Module**:
- [ ] Implementation follows reference code in anchor_synth_report_test.go
- [ ] No data fabrication occurs
- [ ] Errors are properly logged and not masked
- [ ] Tests verify both success and failure paths
- [ ] Implementation handles edge cases (empty batches, all errors, etc.)
- [ ] Code is well-documented with comments explaining the error handling logic

## 13. Conclusion

This test design provides a comprehensive approach to testing the synthetic transaction healing process with the new batch processing and error handling features. By implementing these tests, we can ensure that the healing process is reliable, efficient, and resilient to various error conditions.
