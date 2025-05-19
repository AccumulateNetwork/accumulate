---
title: Anchor Healing Test Design
description: Comprehensive testing strategy for anchor transaction healing with batch processing
tags: [accumulate, healing, anchor-transactions, testing, batch-processing]
created: 2025-05-19
version: 0.1
status: draft
---

# Anchor Healing Test Design

## 1. Introduction

This document outlines a comprehensive testing strategy for the anchor transaction healing process in Accumulate, with a focus on the new batch processing and error handling features. The goal is to ensure that the anchor healing process is reliable, efficient, and resilient to various error conditions.

## 2. Test Objectives

- Verify that batch processing correctly limits anchor transaction submission
- Validate error handling for different error scenarios
- Ensure healing effectiveness is maintained with the new limitations
- Confirm that no data is fabricated during the healing process
- Measure performance impact of batch processing on anchor healing

## 3. Test Environment Setup

### 3.1 Local Development Environment

- Standard Go development environment
- Local Accumulate network with multiple partitions (at least 1 DN and 2 BVNs)
- Controlled network conditions for testing error scenarios

### 3.2 Simulator Environment

- Accumulate network simulator with configurable network conditions
- Ability to inject anchor transactions and simulate errors
- Monitoring tools for tracking batch processing statistics

## 4. Test Categories

### 4.1 Unit Tests

Unit tests will focus on individual components of the anchor healing process, particularly the new error handling and batch processing logic.

#### 4.1.1 Error Handling Tests

| Test ID | Description | Expected Outcome |
|---------|-------------|------------------|
| ERR-001 | Test SkipTransaction error handling | Verify anchor is skipped but batch continues |
| ERR-002 | Test BatchAbort error handling | Verify batch is aborted and next pass is started |
| ERR-003 | Test network error classification | Verify network errors trigger BatchAbort |
| ERR-004 | Test database error classification | Verify critical DB errors trigger BatchAbort |
| ERR-005 | Test anchor-specific error classification | Verify anchor errors trigger SkipTransaction |
| ERR-006 | Test unknown anchor direction handling | Verify proper error classification |

#### 4.1.2 Batch Processing Tests

| Test ID | Description | Expected Outcome |
|---------|-------------|------------------|
| BAT-001 | Test batch size enforcement | Verify no more than MaxBatchSize anchors are processed |
| BAT-002 | Test batches per pass enforcement | Verify only one batch is processed per pass |
| BAT-003 | Test pass interval enforcement | Verify minimum time between passes is respected |
| BAT-004 | Test batch statistics tracking | Verify statistics are accurately tracked |
| BAT-005 | Test batch processing with empty batch | Verify handling of empty batches |

### 4.2 Integration Tests

Integration tests will verify the interaction between different components of the healing process and ensure that the system works correctly as a whole.

#### 4.2.1 End-to-End Healing Tests

| Test ID | Description | Expected Outcome |
|---------|-------------|------------------|
| E2E-001 | Test healing with valid anchor transactions | Verify all anchors are processed successfully |
| E2E-002 | Test healing with mix of valid and invalid anchors | Verify valid anchors are processed and invalid ones are skipped |
| E2E-003 | Test healing with network errors | Verify batch is aborted and retried after delay |
| E2E-004 | Test healing with database errors | Verify appropriate error handling based on error type |
| E2E-005 | Test continuous healing mode | Verify healing continues with appropriate delays between passes |
| E2E-006 | Test DN -> BVN anchor healing | Verify proper handling of DN to BVN anchors |
| E2E-007 | Test BVN -> DN anchor healing | Verify proper handling of BVN to DN anchors |

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
| PERF-005 | Test batch size with various anchor types | Measure impact of anchor complexity |

#### 4.3.2 Pass Interval Impact Tests

| Test ID | Description | Expected Outcome |
|---------|-------------|------------------|
| PERF-006 | Test with various pass intervals (10s, 30s, 60s) | Measure impact on network load and healing time |
| PERF-007 | Test with very short pass interval (1s) | Measure impact on network stability |
| PERF-008 | Test with very long pass interval (5min+) | Measure impact on healing efficiency |
| PERF-009 | Test pass interval under high network load | Verify system stability under stress |
| PERF-010 | Test pass interval with various anchor volumes | Measure impact of anchor volume |

## 5. Test Implementation

### 5.1 Unit Test Implementation

Unit tests will be implemented in `/internal/core/healing/anchors_test.go` using the standard Go testing framework. Mock objects will be used to simulate various error conditions.

```go
// Example test for SkipTransaction error handling in anchor healing
func TestAnchorSkipTransactionErrorHandling(t *testing.T) {
    // Setup test environment
    ctx := context.Background()
    mockClient := &mockClient{}
    mockNetInfo := &mockNetworkInfo{}
    
    // Create a test case that will trigger a SkipTransaction error
    args := healing.HealAnchorArgs{
        Client:  mockClient,
        NetInfo: mockNetInfo,
        // Configure to trigger a non-critical error
    }
    
    si := healing.SequencedInfo{
        Source:      "bvn1",
        Destination: protocol.Directory,
        Number:      123,
    }
    
    // Execute the function under test
    err := healing.HealAnchor(ctx, args, si)
    
    // Verify error is classified as SkipTransaction
    require.True(t, errors.Is(err, errors.SkipTransaction))
    
    // Verify the error is properly wrapped with context
    require.Contains(t, err.Error(), "anchor healing failed")
}

// Example test for batch size enforcement
func TestAnchorBatchSizeEnforcement(t *testing.T) {
    // Setup test environment with a large number of pending anchors
    ctx := context.Background()
    h := &healer{
        // Initialize with test dependencies
    }
    
    // Create a list of 50 anchors to heal
    anchorsToHeal := make([]AnchorInfo, 50)
    for i := 0; i < 50; i++ {
        anchorsToHeal[i] = AnchorInfo{
            Number: uint64(i + 1),
            ID:     nil, // Set appropriate ID
            Txns:   make(map[[32]byte]*protocol.Transaction),
        }
    }
    
    // Set max batch size to 20
    maxBatchSize = 20
    
    // Call the batch processing function
    healAnchorBatch(h, "bvn1", protocol.Directory, anchorsToHeal)
    
    // Verify only 20 anchors were processed
    require.Equal(t, 20, h.batchStats.TxsProcessed)
}
```

### 5.2 Integration Test Implementation

Integration tests will be implemented in a separate test package that sets up a complete healing environment with real or simulated network components.

```go
// Example end-to-end test for healing with mixed valid/invalid anchors
func TestHealingWithMixedAnchors(t *testing.T) {
    // Setup test environment with real components
    network := setupTestNetwork(t)
    defer network.Shutdown()
    
    // Create a mix of valid and invalid anchor transactions
    validAnchors := createValidAnchorTransactions(10)
    invalidAnchors := createInvalidAnchorTransactions(10)
    
    // Mix them together
    allAnchors := append(validAnchors, invalidAnchors...)
    
    // Configure batch processing
    maxBatchSize = 20
    batchesPerPass = 1
    
    // Run the healing process
    h := setupHealer(network)
    stats := runAnchorHealingProcess(h, allAnchors)
    
    // Verify results
    require.Equal(t, 20, stats.TxsProcessed)
    require.Equal(t, 10, stats.TxsSucceeded)
    require.Equal(t, 0, stats.TxsFailed)
    require.Equal(t, 10, stats.TxsSkipped)
    require.Equal(t, 0, stats.BatchesAborted)
}

// Example CLI test for batch size flag
func TestAnchorBatchSizeFlag(t *testing.T) {
    // Setup test command
    cmd := exec.Command("accumulate", "debug", "heal-anchor", 
        "--batch-size=15", "testnet", "bvn1", "directory")
    
    // Capture output
    output, err := cmd.CombinedOutput()
    require.NoError(t, err)
    
    // Verify batch size was set correctly
    require.Contains(t, string(output), "batch-size: 15")
    
    // Verify only 15 anchors were processed per batch
    // This would require parsing the output to find batch statistics
    stats := parseBatchStats(string(output))
    require.LessOrEqual(t, stats.TxsPerBatch, 15)
}
```

### 5.3 Performance Test Implementation

Performance tests will be implemented using the Accumulate simulator and will measure various metrics under different configurations.

```go
// Example performance test for anchor batch size impact
func TestAnchorBatchSizePerformance(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping performance test in short mode")
    }
    
    // Test different batch sizes
    batchSizes := []int{5, 10, 20, 50}
    results := make(map[int]PerformanceMetrics)
    
    for _, size := range batchSizes {
        // Setup test environment with 100 pending anchors
        sim := simulator.New(t)
        defer sim.Close()
        
        // Create 100 anchor transactions
        anchors := createAnchorTransactions(sim, 100)
        
        // Configure healing with current batch size
        maxBatchSize = size
        batchesPerPass = 1
        passInterval = 30 * time.Second
        
        // Measure performance
        start := time.Now()
        stats := runAnchorHealingProcess(sim, anchors)
        duration := time.Since(start)
        
        // Collect metrics
        metrics := PerformanceMetrics{
            BatchSize:         size,
            TotalTime:         duration,
            NetworkBandwidth:  sim.MeasureNetworkBandwidth(),
            CPUUsage:          sim.MeasureCPUUsage(),
            MemoryUsage:       sim.MeasureMemoryUsage(),
            AnchorsHealed:     stats.TxsSucceeded,
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

1. **Valid Anchor Transactions**: Create valid anchor transactions between partitions
2. **Invalid Anchor Transactions**: Create anchor transactions with deliberate errors
3. **Network Error Simulation**: Simulate network errors during anchor resolution
4. **Database Error Simulation**: Simulate database errors during receipt building

### 6.2 Test Data Validation

All test data will be validated to ensure:

1. No data is fabricated during testing (as per critical rule)
2. Test data accurately represents real-world scenarios
3. Test data covers all error conditions and edge cases
4. Test data follows the reference implementation in anchor_synth_report_test.go

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

To thoroughly test error handling, we will inject errors at various points in the anchor healing process:

1. **Network Errors**: Simulate network partitions, timeouts, and connection failures
2. **Database Errors**: Simulate database corruption, missing entries, and access failures
3. **Anchor Errors**: Simulate malformed anchors, invalid signatures, and sequence errors
4. **Direction Errors**: Test with invalid source/destination combinations

### 8.2 Chaos Testing

Introduce random failures and delays to test system resilience:

1. **Random Anchor Failures**: Randomly fail a percentage of anchors
2. **Random Network Delays**: Introduce random network latency
3. **Random Process Restarts**: Restart healing process during execution
4. **Mixed Direction Testing**: Test with mixed DN->BVN and BVN->DN anchors in the same batch

### 8.3 Long-Running Tests

Test the system over extended periods to identify issues that may not appear in short tests:

1. **24-Hour Test**: Run anchor healing continuously for 24 hours with periodic anchor injection
2. **Weekend Test**: Run anchor healing over a weekend with varying load patterns
3. **Recovery Test**: Test recovery after extended downtime

## 9. Test Validation Criteria

### 9.1 Functional Validation

- All anchors that should be healed are healed
- No anchors are processed more than once
- Errors are properly classified and handled
- Batch size and pass interval limits are enforced
- Both DN->BVN and BVN->DN anchor directions are handled correctly

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

1. **Anchor Transaction Generator**: Generate valid and invalid anchor transactions
2. **Error Injector**: Inject errors at specific points in the healing process
3. **Performance Monitor**: Track system performance during testing
4. **Batch Statistics Analyzer**: Analyze batch processing statistics
5. **Direction Validator**: Validate correct handling of different anchor directions

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
4. **Direction Handling**: Special attention should be paid to testing both DN->BVN and BVN->DN anchor directions.

### 11.2 Test Dependencies

- Go testing framework
- Accumulate simulator
- Mock objects for network and database components
- Performance monitoring tools

## 12. Conclusion

This test design provides a comprehensive approach to testing the anchor transaction healing process with the new batch processing and error handling features. By implementing these tests, we can ensure that the anchor healing process is reliable, efficient, and resilient to various error conditions.
