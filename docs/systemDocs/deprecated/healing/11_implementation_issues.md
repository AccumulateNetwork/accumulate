# Implementation Issues and Observations

## Introduction

This document outlines potential issues and observations regarding the current implementation of the healing processes in Accumulate. It serves as a reference for developers working on the codebase and provides guidance for future improvements.

## Documentation Location

### Critical Rule

**All healing documentation MUST be kept under the `tools/cmd/debug/docs/healing` directory.** This ensures that healing-related documentation is properly organized and associated with the debug tools that implement the healing functionality.

### Potential Issues

1. **Incorrect Documentation Location**: Some documentation was previously placed outside the debug directory, which can lead to confusion and make it difficult to find relevant information.

2. **Documentation-Code Separation**: Keeping documentation separate from the code that implements it can lead to documentation becoming outdated as the code evolves.

### Recommendations

1. Ensure all healing documentation is kept under the `tools/cmd/debug/docs/healing` directory.
2. Update references in the codebase to point to the correct documentation location.
3. Implement a documentation review process to ensure documentation remains in the correct location.

## On-Demand Transaction Fetching

### Critical Rule

When implementing on-demand transaction fetching for the anchor healing process, we must ensure we follow the reference test code in `anchor_synth_report_test.go` exactly as specified. The implementation must:

1. Intercept "key not found" errors when attempting to retrieve transaction data
2. Use the sequence number to fetch specific transactions from the network only when needed
3. Not fabricate or fake any data that isn't available from the network
4. Implement proper fallback mechanisms as defined in the reference test code
5. Add detailed logging to track when transactions are fetched on-demand

### Current Implementation

The healing processes implement on-demand transaction fetching to avoid loading all transactions upfront:

```go
// From internal/core/healing/synthetic.go
// Try to get the transaction from the known transactions map
if txn, ok := args.TxnMap[txid.Hash()]; ok {
    return txn, nil
}

// If not in the map, fetch it from the network
// ...
```

### Potential Issues

1. **Reference Test Code Alignment**: The implementation must follow exactly what's in the reference test code (`anchor_synth_report_test.go`). Any deviation from this reference could lead to inconsistent behavior.

2. **Error Handling**: The implementation should properly intercept "key not found" errors when attempting to retrieve transaction data.

3. **Logging**: More detailed logging should be added to track when transactions are fetched on-demand.

### Recommendations

1. Audit the transaction fetching code to ensure it properly intercepts "key not found" errors.
2. Implement fallback mechanisms exactly as defined in the reference test code.
3. Add detailed logging to track when and why transactions are fetched on-demand.

## Data Integrity

### Critical Rule

Data retrieved from the Protocol CANNOT be faked. Doing so masks errors and leads to huge wastes of time for those monitoring the Network. The only acceptable implementation is to follow exactly what's in the reference test code (`anchor_synth_report_test.go`). Any data collection must match the test implementation precisely, including fallback mechanisms, but no additional data fabrication should be added.

### Potential Issues

1. **Error Masking**: Some error handling patterns might inadvertently mask issues by providing default values.

2. **Fabricated Data**: The implementation must never fabricate or fake any data that isn't available from the network.

### Recommendations

1. Review all error handling to ensure no errors are being masked.
2. Ensure that all data used in the healing process comes directly from the network.
3. Implement proper fallback mechanisms without fabricating data.

## Healing Process Configuration

### Current Implementation

The healing process for synthetic transactions is run with the following flags:
- `--since`: How far back in time to heal (default: 48 hours, 0 for forever)
- `--max-response-age`: Set to 336 hours (2 weeks) to allow for longer periods of data retrieval
- `--wait`: Flag to wait for transactions (enabled by default for heal-synth)

The command is typically run with a network name as the first argument, followed by optional partition pair and sequence number.

### Potential Issues

1. **Configuration Documentation**: The configuration options might not be fully documented or consistently applied across different healing processes.

2. **Default Values**: Some default values might not be optimal for all network sizes and conditions.

### Recommendations

1. Ensure all configuration options are well-documented and consistently applied.
2. Consider making more configuration options adjustable based on network size and conditions.

## Version-Specific Logic

### Current Implementation

The healing processes include version-specific logic to handle different network protocol versions:

```go
// From internal/core/healing/anchors.go
func HealAnchor(ctx context.Context, args HealAnchorArgs, si SequencedInfo) error {
    // If the network is running Vandenberg and the anchor is from the DN to a
    // BVN, use version 2
    if args.NetInfo.Status.ExecutorVersion.V2VandenbergEnabled() &&
        strings.EqualFold(si.Source, protocol.Directory) &&
        !strings.EqualFold(si.Destination, protocol.Directory) {
        return healDnAnchorV2(ctx, args, si)
    }
    return healAnchorV1(ctx, args, si)
}
```

### Potential Issues

1. **Maintenance Complexity**: As the protocol evolves, maintaining multiple versions of the healing logic could become increasingly complex.

2. **Fallback Mechanisms**: The fallback from V2 to V1 in certain error conditions might not be well-documented or tested.

### Recommendations

1. Consider consolidating version-specific logic where possible.
2. Improve documentation of version-specific behavior and fallback mechanisms.
3. Add comprehensive tests for version-specific logic and fallbacks.

## Caching Infrastructure

### Current Implementation

The healing processes use an LRU cache with a fixed size:

```go
// From tools/cmd/debug/heal_common.go
func newTxFetcher(client message.AddressedClient) *txFetcher {
    cache, err := lru.New[string, *protocol.Transaction](1000)
    if err != nil {
        panic(err)
    }
    return &txFetcher{
        Client: client,
        Cache:  cache,
    }
}
```

### Potential Issues

1. **Cache Size Limitations**: The fixed cache size of 1000 entries might not be sufficient for large networks or during intensive healing operations.

2. **Cache Performance Monitoring**: While there is some monitoring of cache performance, it might not be comprehensive enough to identify all potential bottlenecks.

### Recommendations

1. Make the cache size configurable based on network size and healing workload.
2. Enhance cache performance monitoring to provide more detailed metrics.
3. Consider implementing a tiered caching strategy for very large networks.

## Documentation-Implementation Mismatch

### Potential Issues

1. **File Naming**: The README mentions files like "07_receipt_creation.md" and "08_signature_gathering.md", but the actual files are named differently (e.g., "07_receipt_signature.md").

2. **Code References**: Some code references in the documentation might not match the actual implementation.

### Recommendations

1. Update documentation to match the actual file structure.
2. Verify that all code references in the documentation match the actual implementation.
3. Implement a documentation review process to keep documentation in sync with code changes.

## Retry Logic

### Current Implementation

The anchor healing process includes retry logic with a fixed limit:

```go
// From tools/cmd/debug/heal_anchor.go
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
```

### Potential Issues

1. **Fixed Retry Limit**: The fixed retry limit of 10 attempts might not be sufficient in some network conditions.

2. **Backoff Strategy**: The retry logic might benefit from an exponential backoff strategy to avoid overwhelming the network.

### Recommendations

1. Make the retry limit configurable.
2. Implement an exponential backoff strategy for retries.
3. Add more detailed logging for retry attempts.

## Conclusion

The healing processes in Accumulate are sophisticated and well-designed, but there are several areas where improvements could be made. By addressing these issues, the healing processes can become more robust, maintainable, and efficient.

The most critical issues to address are:

1. Ensuring that all healing documentation is kept under the `tools/cmd/debug/docs/healing` directory.
2. Ensuring that the on-demand transaction fetching implementation follows the reference test code exactly, as emphasized in the project requirements.
3. Maintaining strict data integrity by never fabricating data that isn't available from the network.
4. Implementing proper fallback mechanisms as defined in the reference test code.
5. Adding detailed logging to track when transactions are fetched on-demand.
