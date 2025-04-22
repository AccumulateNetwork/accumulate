# Development Overview (Version 2)

This document provides a high-level overview of the Version 2 changes to the development processes in the Accumulate network, focusing on key improvements including the in-memory database implementation, URL construction standardization, caching system, and enhanced reporting capabilities.

This overview builds upon the foundation established in the [Version 1 Overview](../heal_v1/overview.md) and assumes familiarity with the core concepts described there.

## Goals of Version 2

Version 2 has several key goals that aim to improve the development and operational aspects of the Accumulate network:

1. **Redesign Transaction Healing Process**:
   - Implement a lightweight transaction wrapper for efficient tracking
   - Standardize URL construction across all components
   - Remove retry mechanisms within a single healing cycle to prevent network backoff issues
   - Limit to maximum of 10 submission attempts across different healing cycles
   - Create synthetic alternatives after 10 failed cycles

2. **Improve Performance**: 
   - Replace the Bolt database with an in-memory implementation for faster operations
   - Implement caching to reduce redundant network requests
   - Optimize data structures for common access patterns

3. **Enhance Reliability**:
   - Standardize URL construction across different code components
   - Track problematic nodes to avoid querying them for certain types of requests
   - Improve error handling and recovery mechanisms

4. **Simplify Development and Deployment**:
   - Maintain consistent interfaces for backward compatibility
   - Provide clearer documentation for both human developers and AI assistants
   - Reduce dependencies on external systems

5. **Improve Observability and Monitoring**:
   - Add comprehensive reporting for healing operations
   - Track and visualize the state of partition pairs
   - Provide actionable insights for network operators

## What Remains the Same

Version 2 maintains the same core approaches as Version 1:

1. **Anchor Healing**: Still focuses on healing anchor transactions and does not require a database. The implementation details are documented in [Version 1 Implementation](../heal_v1/implementation.md#anchor-healing).

2. **Synthetic Healing**: Still requires a database to build Merkle proofs, but now uses the in-memory implementation. The core process follows the approach documented in [Version 1 Implementation](../heal_v1/implementation.md#synthetic-healing).

3. **API Interfaces**: The public interfaces remain unchanged to ensure compatibility with existing code. These interfaces are detailed in the [Version 1 Light Client documentation](../heal_v1/light_client.md).

4. **Transaction Creation**: The transaction creation process remains consistent with the approach described in [Version 1 Transactions](../heal_v1/transactions.md).

The shared code components, transaction creation processes, and implementation guidelines remain largely the same as in Version 1.

## What Has Changed

### 1. Database Implementation

The most significant change in Version 2 is the replacement of the Bolt database with an in-memory implementation:

```go
// In common.go (Version 2)
func (h *healer) initDB() error {
    // Initialize the MemDB
    db := memdb.New()
    h.db = db

    // Initialize the Light client with the MemDB
    h.light = light.New(db, nil, nil)
    return nil
}
```

The in-memory implementation:
- Implements the `keyvalue.Beginner`, `keyvalue.Batch`, and `io.Closer` interfaces
- Maintains ACID transaction semantics
- Optimizes for in-memory access patterns
- Does not persist data between restarts

### 2. URL Construction Standardization

Version 2 addresses a fundamental difference in how URLs are constructed between different components:

```go
// Method 1: Used in sequence.go (now standardized)
srcUrl := protocol.PartitionUrl(src.ID)  // e.g., acc://bvn-Apollo.acme

// Method 2: Previously used in heal_anchor.go
srcUrl := protocol.DnUrl().JoinPath(protocol.AnchorPool).JoinPath(src.ID)  // e.g., acc://dn.acme/anchors/Apollo
```

This standardization helps prevent "element does not exist" errors that occurred when code was looking for anchors at different URL paths. The original URL construction approaches and their implications are documented in [Version 1 Chains and URLs](../heal_v1/chains_urls.md).

### 3. Caching System

Version 2 implements a caching system to reduce redundant network requests:

```go
// Cache key structure
type cacheKey struct {
    url  string
    typ  string
}

// Thread-safe cache access
func (c *cache) Get(url, typ string) (interface{}, bool) {
    c.mutex.RLock()
    defer c.mutex.RUnlock()
    value, ok := c.cache[cacheKey{url, typ}]
    return value, ok
}
```

The caching system:
- Stores query results indexed by URL and query type
- Reduces redundant network requests for frequently accessed data
- Tracks problematic nodes to avoid querying them

### 4. Performance Optimizations

Version 2 includes several optimizations:
- Reduced lock contention for read operations
- Optimized data structures for common access patterns
- More efficient error handling and recovery mechanisms
- Efficient handling of large values (like chain entries)

### 5. Comprehensive Reporting System

Version 2 introduces a comprehensive reporting system for the `heal-all` command:

```go
// Data structure for tracking partition pair status
type partitionPairStatus struct {
    Source            string
    Destination       string
    AnchorGaps        int
    AnchorHealed      int
    SynthGaps         int
    SynthHealed       int
    // Additional tracking fields
}
```

The reporting system:
- Tracks the healing status of all partition pairs in the network
- Presents data in a well-formatted table for easy interpretation
- Highlights areas that require attention
- Integrates with the controlled pacing mechanism
- Provides actionable insights for network operators

For detailed information about the reporting system, see [Heal-All Reporting](./heal_all_reporting.md).

## Impact on Healing Approaches

### heal_anchor

The `heal_anchor` approach is largely unaffected by the database change since it does not require a database. It continues to follow the approach documented in [Version 1 Implementation](../heal_v1/implementation.md#anchor-healing), with the addition of URL standardization and caching improvements.

### heal_synth

The `heal_synth` approach now uses the MemDB for building Merkle proofs:
- The interface remains the same as documented in [Version 1 Implementation](../heal_v1/implementation.md#synthetic-healing)
- Performance is improved for proof construction
- Memory usage is more predictable
- The underlying database implementation has changed, but the interaction patterns remain consistent with those described in [Version 1 Database](../heal_v1/database.md)

## For More Information

### Version 2 Documentation
- For comprehensive design of the transaction healing process, see [Transaction Healing Process](./transaction_healing_process.md)
- For detailed implementation of the heal-all command, see [Implementation Guide](./heal_all_implementation.md)
- For reporting system design, see [Reporting Design](./heal_all_reporting.md)
- For detailed information about the MemDB implementation, see [MemDB Implementation](./mem_db.md)
- For fast sync optimizations, see [Fast Sync Implementation](./fast_sync.md)
- For peer management improvements, see [Peer Management](./peers.md)
- For URL diagnostics, see [URL Diagnostics](./url_diagnostics.md)
- For routing and URL handling, see [Routing and URL Handling](./routing_and_url_handling.md)

### Version 1 Documentation
- For core healing concepts, see [Version 1 Overview](../heal_v1/overview.md)
- For implementation details, see [Version 1 Implementation](../heal_v1/implementation.md)
- For database architecture, see [Version 1 Database](../heal_v1/database.md)
- For transaction creation, see [Version 1 Transactions](../heal_v1/transactions.md)
- For light client usage, see [Version 1 Light Client](../heal_v1/light_client.md)
- For chain and URL relationships, see [Version 1 Chains and URLs](../heal_v1/chains_urls.md)
- For routing system details, see [Routing System](../heal_v1/route.md) and [Routing Errors](../heal_v1/routing_errors.md)
