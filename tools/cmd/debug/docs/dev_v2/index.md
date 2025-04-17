# Accumulate Network Development Documentation (Version 2)

## Overview

This documentation covers Version 2 of the development tools and processes for the Accumulate network, with a focus on healing processes, performance optimizations, and in-memory database implementations. This documentation is structured to be easily navigable by both human developers and AI assistants.

## What is Healing?

Healing is the process of ensuring that transactions are properly delivered across partitions in the Accumulate network. When a transaction needs to be processed by multiple partitions (e.g., a transaction originating in one BVN that affects an account in another BVN), the network uses anchors and synthetic transactions to ensure consistency across partitions.

Sometimes these cross-partition transactions can fail to be delivered due to network issues or other problems. The healing process identifies these failed deliveries and resubmits the necessary transactions to ensure they are properly processed.

For a comprehensive explanation of the healing process fundamentals, refer to the [Version 1 Overview](../heal_v1/overview.md) and [Implementation Details](../heal_v1/implementation.md).

## Documentation Structure

The Version 2 development documentation is organized into the following documents:

1. [**Developer's Guide**](./developer_guide.md): A practical, code-focused guide for developers and AI assistants who need to implement, modify, or debug code.

2. [**Transaction Healing Process**](./transaction_healing_process.md): Comprehensive design documentation for the transaction healing process, including URL standardization, transaction lifecycle, and submission policy.

3. [**Implementation Guide**](./heal_all_implementation.md): Detailed code implementation with examples for the heal-all command.

4. [**Reporting Design**](./heal_all_reporting.md): Specific design for the reporting system used in the healing process.

5. [**Overview**](./overview.md): A high-level overview of the changes in Version 2 and the goals of the implementation.

6. [**MemDB Implementation**](./mem_db.md): Detailed documentation of the MemDB implementation that replaces Bolt DB.

7. [**AI Assistant Guide**](./ai_guide.md): Special documentation to help AI assistants effectively navigate and utilize Version 2 documentation.
   - [Context Management for AI Assistants](./ai_guide.md#context-management-for-ai-assistants)
   - [Diagnostic Decision Trees](./ai_guide.md#diagnostic-decision-trees-for-ai-assistants)
   - [Code Pattern Recognition](./ai_guide.md#code-pattern-recognition-guide)
   - [Version Comparison](./ai_guide.md#comparing-version-1-and-version-2)

## Changes from Version 1

Version 2 focuses on the following improvements:

1. **Transaction Healing Process**: A comprehensive redesign of the transaction healing process with:
   - Lightweight transaction wrapper for efficient tracking
   - Standardized URL construction across all components
   - No retry mechanism within a single healing cycle to prevent network backoff issues
   - Maximum of 10 submission attempts across different healing cycles
   - Synthetic alternatives after 10 failed cycles

2. **MemDB**: Replacing the Bolt database with a MemDB implementation to improve performance and simplify deployment.

3. **Interface Compatibility**: Maintaining the same interfaces to ensure compatibility with existing code.

4. **Performance Optimizations**: Optimizing database operations for the in-memory context.

5. **URL Construction Standardization**: Standardizing on the Raw Partition URL Format (`protocol.PartitionUrl(id)`) to prevent routing conflicts and caching inefficiencies.

6. **Caching System**: Implementing a caching system to reduce redundant network requests.

For all other aspects of healing not specifically updated in Version 2, please refer to the [Version 1 Documentation](../heal_v1/index.md). Specifically:

- For transaction creation and processing details, see [Transactions](../heal_v1/transactions.md)
- For database implementation details that Version 2 builds upon, see [Database](../heal_v1/database.md)
- For light client usage that remains consistent, see [Light Client](../heal_v1/light_client.md)
- For known issues and their resolutions, see [Issues](../heal_v1/issues.md)
- For original URL construction and chain relationships, see [Chains and URLs](../heal_v1/chains_urls.md)
- For routing system details, see [Routing System](../heal_v1/route.md) and [Routing Errors](../heal_v1/routing_errors.md)

## Key Considerations

When working with the Version 2 code, be aware of these key considerations:

### 1. Database Implementation

The MemDB implementation:
- Implements the same interfaces as the Bolt DB implementation
- Maintains ACID transaction semantics
- Optimizes for in-memory access patterns
- Does not persist data between restarts

### 2. URL Construction

There are two different URL construction methods used in the codebase:
- `sequence.go` uses raw partition URLs (e.g., `acc://bvn-Apollo.acme`)
- `heal_anchor.go` appends the partition ID to the anchor pool URL (e.g., `acc://dn.acme/anchors/Apollo`)

Version 2 standardizes on the `sequence.go` approach. For details on how URLs and chains interact in the original implementation, see [Chains and URLs](../heal_v1/chains_urls.md) in the Version 1 documentation.

### 3. Caching System

The caching system:
- Stores query results in a map indexed by URL and query type
- Reduces redundant network requests for frequently accessed data
- Tracks problematic nodes to avoid querying them for certain types of requests

### 4. Additional Resources

- [Fast Sync Implementation](./fast_sync.md)
- [Synthetic Chains](./synthetic_chains.md)
- [URL Diagnostics](./url_diagnostics.md)
- [Peer Management](./peers.md)

For detailed information about the MemDB implementation, see [MemDB Implementation](./mem_db.md).
