# Minimal Caching in V3 Implementation

```yaml
# AI-METADATA
document_type: implementation_detail
project: accumulate_network
component: minimal_caching
version: v3
related_files:
  - ./development-plan.md
  - ./stateless-architecture.md
  - ../../concepts/caching-strategies.md
```

## Overview

The V3 implementation of the Accumulate Network healing process introduces a minimal caching approach that significantly differs from the more comprehensive caching systems in previous implementations. This document outlines the design, benefits, and tradeoffs of this approach.

## Minimal Caching Design

The minimal caching approach in V3 follows these core principles:

1. **Cache Only Essential Data**: Only cache data that is absolutely necessary to avoid redundant work
2. **Limited Cache Lifetime**: Cache data only for the duration of a single healing operation
3. **Memory Efficiency**: Minimize memory usage by limiting cache size and scope
4. **Simplicity**: Keep the caching implementation simple and focused

## Caching Components

### Rejected Transaction Cache

The primary caching component in V3 is the rejected transaction cache, which stores hashes of transactions that have been rejected by the network:

```go
// Healer implements the stateless healing process
type Healer struct {
    // rejectedTxs is a minimal cache of rejected transaction hashes
    // to avoid regenerating the same transaction repeatedly
    rejectedTxs map[string]bool
    
    // mutex protects the rejectedTxs map
    mutex sync.RWMutex
    
    // Other fields...
}

// IsRejectedTransaction checks if a transaction has been rejected before
func (h *Healer) IsRejectedTransaction(hash string) bool {
    h.mutex.RLock()
    defer h.mutex.RUnlock()
    return h.rejectedTxs[hash]
}

// MarkTransactionRejected marks a transaction as rejected
func (h *Healer) MarkTransactionRejected(hash string) {
    h.mutex.Lock()
    defer h.mutex.Unlock()
    h.rejectedTxs[hash] = true
}
```

### Request-Scoped Caching

In addition to the rejected transaction cache, V3 implements request-scoped caching for frequently accessed data during a single healing operation:

```go
// HealContext represents the context for a single healing operation
type HealContext struct {
    // requestCache is a request-scoped cache for frequently accessed data
    requestCache map[string]interface{}
    
    // Other fields...
}

// GetFromCache gets a value from the request-scoped cache
func (ctx *HealContext) GetFromCache(key string) (interface{}, bool) {
    value, ok := ctx.requestCache[key]
    return value, ok
}

// SetInCache sets a value in the request-scoped cache
func (ctx *HealContext) SetInCache(key string, value interface{}) {
    ctx.requestCache[key] = value
}
```

### Cache Key Construction

The V3 implementation uses a simple approach to cache key construction:

```go
// constructCacheKey constructs a cache key from components
func constructCacheKey(components ...string) string {
    return strings.Join(components, ":")
}

// Example usage
func (h *Healer) queryAccountWithCache(ctx context.Context, healCtx *HealContext, url *url.URL) (*protocol.Account, error) {
    // Construct the cache key
    key := constructCacheKey("account", url.String())
    
    // Check if the result is in the request-scoped cache
    if value, ok := healCtx.GetFromCache(key); ok {
        if account, ok := value.(*protocol.Account); ok {
            return account, nil
        }
    }
    
    // Execute the query
    account, err := h.queryAccount(ctx, url)
    if err != nil {
        return nil, err
    }
    
    // Cache the result in the request-scoped cache
    healCtx.SetInCache(key, account)
    
    return account, nil
}
```

## Benefits of Minimal Caching

### 1. Reduced Memory Footprint

The minimal caching approach significantly reduces memory usage compared to previous implementations:

- No need for a comprehensive query cache
- No need to store historical performance metrics
- No need to maintain a large transaction history
- No need for complex cache invalidation logic

### 2. Simplified Implementation

The minimal caching approach simplifies the implementation:

- No complex cache management logic
- No need for cache synchronization
- No need for cache invalidation
- No need for cache persistence

### 3. Improved Reliability

The minimal caching approach improves reliability:

- No cache corruption issues
- No cache synchronization issues
- No cache invalidation issues
- No cache persistence issues

### 4. Better Scalability

The minimal caching approach improves scalability:

- Multiple healing processes can run in parallel without cache conflicts
- No need for distributed cache management
- No need for cache replication
- No need for cache consistency protocols

## Tradeoffs and Limitations

### 1. Increased Network Requests

The minimal caching approach may result in increased network requests:

- Each healing operation may need to query the same data multiple times
- No long-term caching of frequently accessed data
- No sharing of cached data between healing operations

### 2. Limited Optimization Opportunities

The minimal caching approach limits optimization opportunities:

- No ability to optimize based on historical query patterns
- No ability to prefetch frequently accessed data
- No ability to batch similar queries

### 3. Potential for Redundant Work

The minimal caching approach may result in redundant work:

- Multiple healing operations may perform the same queries
- No built-in mechanism for coordinating query efforts
- Limited caching only prevents regenerating rejected transactions

## Implementation Details

### 1. Cache Initialization

The caches are initialized with minimal configuration:

```go
// NewHealer creates a new stateless healer
func NewHealer(client api.Client, logger *zap.Logger) *Healer {
    return &Healer{
        rejectedTxs: make(map[string]bool),
        client:      client,
        logger:      logger,
    }
}

// NewHealContext creates a new healing context
func NewHealContext() *HealContext {
    return &HealContext{
        requestCache: make(map[string]interface{}),
    }
}
```

### 2. Cache Usage in Healing Operations

The caches are used in healing operations to avoid redundant work:

```go
// HealAnchorGap heals an anchor gap between two partitions
func (h *Healer) HealAnchorGap(ctx context.Context, src, dst string, seqNum uint64) error {
    // Create a new healing context for this operation
    healCtx := NewHealContext()
    
    // 1. Query the source partition for the anchor
    anchor, err := h.queryAnchorWithCache(ctx, healCtx, src, dst, seqNum)
    if err != nil {
        return fmt.Errorf("failed to query anchor: %w", err)
    }
    
    // 2. Create a healing transaction
    tx, err := h.createAnchorHealingTransaction(ctx, healCtx, src, dst, seqNum, anchor)
    if err != nil {
        return fmt.Errorf("failed to create healing transaction: %w", err)
    }
    
    // 3. Check if the transaction has been rejected before
    if h.IsRejectedTransaction(tx.GetHash().String()) {
        return fmt.Errorf("transaction has been rejected before")
    }
    
    // 4. Submit the transaction
    err = h.submitTransaction(ctx, tx)
    if err != nil {
        // Mark the transaction as rejected if it was rejected by the network
        if isRejectedError(err) {
            h.MarkTransactionRejected(tx.GetHash().String())
        }
        return fmt.Errorf("failed to submit transaction: %w", err)
    }
    
    return nil
}
```

### 3. Cache-Aware Query Methods

The V3 implementation includes cache-aware query methods:

```go
// queryAnchorWithCache queries an anchor with request-scoped caching
func (h *Healer) queryAnchorWithCache(ctx context.Context, healCtx *HealContext, src, dst string, seqNum uint64) (*protocol.Anchor, error) {
    // Construct the cache key
    key := constructCacheKey("anchor", src, dst, fmt.Sprintf("%d", seqNum))
    
    // Check if the result is in the request-scoped cache
    if value, ok := healCtx.GetFromCache(key); ok {
        if anchor, ok := value.(*protocol.Anchor); ok {
            return anchor, nil
        }
    }
    
    // Query the anchor
    anchor, err := h.queryAnchor(ctx, src, dst, seqNum)
    if err != nil {
        return nil, err
    }
    
    // Cache the result in the request-scoped cache
    healCtx.SetInCache(key, anchor)
    
    return anchor, nil
}
```

### 4. Thread Safety

The V3 implementation ensures thread safety for the rejected transaction cache:

```go
// IsRejectedTransaction checks if a transaction has been rejected before
func (h *Healer) IsRejectedTransaction(hash string) bool {
    h.mutex.RLock()
    defer h.mutex.RUnlock()
    return h.rejectedTxs[hash]
}

// MarkTransactionRejected marks a transaction as rejected
func (h *Healer) MarkTransactionRejected(hash string) {
    h.mutex.Lock()
    defer h.mutex.Unlock()
    h.rejectedTxs[hash] = true
}
```

## Comparison with Previous Implementations

| Feature | V1 | V2 | V3 |
|---------|----|----|----| 
| Cache Scope | Global | Global | Request-scoped |
| Cache Size | Unbounded | Bounded | Minimal |
| Cache Lifetime | Persistent | Persistent with TTL | Request duration |
| Cache Contents | Query results, transaction history | Query results, transaction history, node performance | Rejected transactions only |
| Thread Safety | Basic | Enhanced | Basic |
| Memory Usage | High | Medium | Low |
| Implementation Complexity | Medium | High | Low |
| Network Requests | Low | Low | Higher |

## See Also

- [Development Plan](./development-plan.md): Comprehensive development plan for V3
- [Stateless Architecture](./stateless-architecture.md): Explanation of the stateless architecture in V3
- [Caching Strategies](../../concepts/caching-strategies.md): Comparison of different caching approaches
