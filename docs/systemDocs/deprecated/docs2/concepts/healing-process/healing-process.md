> **DEPRECATED**: This document has been moved to the new documentation structure. Please refer to the [new location](../../../new_structure/03_core_components/04_healing/01_overview.md) for the most up-to-date version.

# Healing Process Fundamentals

```yaml
# AI-METADATA
document_type: concept
project: accumulate_network
component: healing_process
version: current
key_concepts:
  - synthetic_ledger_healing
  - anchor_healing
  - transaction_healing
  - healing_algorithms
related_files:
  - ../implementations/v1/overview.md
  - ../implementations/v2/transaction-healing-process.md
  - ../concepts/url-construction.md
```

## Overview

This document provides a comprehensive explanation of the healing process in the Accumulate network. It covers both anchor healing and synthetic transaction healing, explaining the fundamental concepts, algorithms, and implementation approaches across different versions.

## Healing Types

The Accumulate network uses two primary types of healing:

1. **Anchor Healing**: Ensures that anchors (cryptographic commitments) are properly propagated between partitions.
2. **Synthetic Transaction Healing**: Ensures that synthetic transactions (transactions that span multiple partitions) are properly executed across all involved partitions.

## Anchor Healing

Anchor healing ensures that anchors created in one partition are properly propagated to all other partitions that need them. This is critical for maintaining the security and consistency of the Accumulate network.

### Anchor Healing Process

1. **Identify Missing Anchors**: 
   - For each partition, check if it has received all the anchors it should have from other partitions
   - Compare the local anchor sequence with the remote anchor sequence to identify gaps

2. **Retrieve Missing Anchors**:
   - For each missing anchor, query the source partition to retrieve the anchor data
   - Validate the anchor data to ensure it is authentic

3. **Apply Missing Anchors**:
   - Submit the missing anchors to the local partition
   - Verify that the anchors are properly applied

### Anchor URL Construction

Anchor URLs are constructed using the following format:
- BVN to BVN: `acc://bvn-{source-partition}.acme/anchors/{dest-partition}`
- DN to BVN: `acc://dn.acme/anchors/{dest-partition}`

See the [URL Construction document](./url-construction.md) for detailed information on URL construction.

## Synthetic Transaction Healing

Synthetic transaction healing ensures that transactions that span multiple partitions (synthetic transactions) are properly executed across all involved partitions. This is critical for maintaining the consistency of the Accumulate network.

### Synthetic Transaction Healing Process

1. **Identify Pending Synthetic Transactions**:
   - For each partition, check its synthetic ledger for pending synthetic transactions
   - A pending synthetic transaction is one that has been initiated but not yet completed

2. **Retrieve Transaction Details**:
   - For each pending transaction, query the source partition to retrieve the transaction details
   - Validate the transaction details to ensure they are authentic

3. **Execute Missing Steps**:
   - Determine which steps of the transaction have not yet been executed
   - Execute the missing steps in the correct order

4. **Verify Transaction Completion**:
   - Check that all steps of the transaction have been executed
   - Update the synthetic ledger to mark the transaction as completed

### Synthetic Ledger URL Construction

Synthetic ledger URLs are constructed using the following format:
- BVN: `acc://bvn-{partition-name}.acme/synthetic`
- DN: `acc://dn.acme/synthetic`

Sequence chain URLs are constructed using the following format:
- BVN to BVN: `acc://bvn-{source-partition}.acme/synthetic/synthetic-sequence-{dest-partition}`
- DN to BVN: `acc://dn.acme/synthetic/synthetic-sequence-{dest-partition}`

See the [URL Construction document](./url-construction.md) for detailed information on URL construction.

## Healing Algorithms

### V1 Algorithm (Original Implementation)

The original healing algorithm follows a straightforward approach:

1. For each partition pair (source, destination):
   - Query the source partition for its anchor sequence
   - Query the destination partition for its anchor sequence
   - Compare the sequences to identify missing anchors
   - Retrieve and apply missing anchors

This approach is simple but can be inefficient as it requires querying all partition pairs, even when there are no missing anchors.

### V2 Algorithm (Modified Implementation)

The V2 algorithm introduces several optimizations:

1. **Caching**: Cache query results to avoid redundant network requests
2. **Prioritization**: Prioritize healing based on the age of missing anchors
3. **Batching**: Batch anchor retrievals to reduce network overhead
4. **Error Handling**: Improved error handling with fallback mechanisms

### V3 Algorithm (Stateless Design)

The V3 algorithm introduces a more stateless approach:

1. **Minimal Caching**: Only cache essential data to reduce memory overhead
2. **On-Demand Healing**: Heal only when explicitly requested, rather than proactively
3. **Parallel Processing**: Process multiple healing requests in parallel
4. **Improved Diagnostics**: Better error reporting and diagnostic capabilities

### V4 Algorithm (Peer Discovery Focus)

The V4 algorithm focuses on improving peer discovery:

1. **Multiple Extraction Methods**: Support multiple methods for extracting peer information
2. **Robust Error Handling**: Improved error handling with fallback mechanisms
3. **Performance Optimizations**: Method prioritization based on reliability

## Common Pitfalls

1. **Incorrect URL Construction**: Using the wrong URL format for anchors or synthetic ledgers
   - See the [URL Construction document](./url-construction.md) for correct URL formats

2. **Missing Partition Validation**: Not validating partition names before constructing URLs
   - Always validate partition names using `IsValidPartition()`

3. **Redundant Network Requests**: Not caching query results, leading to excessive network traffic
   - See the [Caching Strategies document](./caching-strategies.md) for caching best practices

4. **Incomplete Error Handling**: Not properly handling network errors or invalid data
   - See the [Error Handling document](./error-handling.md) for error handling best practices

5. **Inconsistent Anchor Formats**: Using different anchor formats in different parts of the code
   - Standardize on a single anchor format throughout the codebase

## Implementation Notes

1. Always use the standardized URL construction functions for consistent URL formats
2. Validate all partition names before constructing URLs
3. Implement proper caching to reduce network overhead
4. Handle errors gracefully with appropriate fallback mechanisms
5. Use consistent terminology throughout the codebase

## See Also

- [URL Construction](./url-construction.md)
- [Caching Strategies](./caching-strategies.md)
- [Error Handling](./error-handling.md)
- [Original Healing Overview (v1)](../implementations/v1/overview.md)
- [Transaction Healing Process (v2)](../implementations/v2/transaction-healing-process.md)
