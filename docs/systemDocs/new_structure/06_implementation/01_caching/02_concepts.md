# Caching Strategies

```yaml
# AI-METADATA
document_type: concept
project: accumulate_network
component: caching
version: current
key_concepts:
  - query_caching
  - memory_management
  - cache_invalidation
  - problematic_node_tracking
related_files:
  - ../implementations/v1/implementation.md
  - ../implementations/v2/index.md
  - ../implementations/v3/development-plan.md
```

## Overview

This document provides a comprehensive explanation of the caching strategies used in the Accumulate network healing process. It compares different caching approaches across implementation versions, discusses their trade-offs, and provides best practices for effective caching.

## Why Caching is Needed

The healing process involves numerous network queries to retrieve account data, chain data, and transaction information. Without caching:

1. **Redundant Queries**: The same data might be queried multiple times, increasing network load
2. **Slow Performance**: Each query adds latency, slowing down the healing process
3. **Increased Error Rates**: More queries mean more opportunities for network errors
4. **Higher Resource Usage**: Both client and server resources are consumed by redundant queries

## Caching Approaches

### V1 Approach: Basic Query Caching

The original implementation uses a simple caching mechanism:

```go
type QueryCache struct {
    cache map[string]interface{}
    mutex sync.RWMutex
}

func (c *QueryCache) Get(key string) (interface{}, bool) {
    c.mutex.RLock()
    defer c.mutex.RUnlock()
    value, ok := c.cache[key]
    return value, ok
}

func (c *QueryCache) Set(key string, value interface{}) {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    c.cache[key] = value
}
```

**Characteristics**:
- Simple key-value storage
- No expiration mechanism
- No size limits
- Thread-safe with mutex

**Use Cases**:
- Caching account query results
- Caching chain query results
- Caching transaction data

**Limitations**:
- Memory usage grows unbounded
- No way to invalidate stale data
- No prioritization of cache entries

### V2 Approach: Enhanced Query Caching with Problematic Node Tracking

The V2 implementation enhances the caching system with problematic node tracking:

```go
type EnhancedQueryCache struct {
    cache            map[string]interface{}
    problematicNodes map[string]map[string]bool // node -> query type -> is problematic
    mutex            sync.RWMutex
}

func (c *EnhancedQueryCache) Get(key string) (interface{}, bool) {
    c.mutex.RLock()
    defer c.mutex.RUnlock()
    value, ok := c.cache[key]
    return value, ok
}

func (c *EnhancedQueryCache) Set(key string, value interface{}) {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    c.cache[key] = value
}

func (c *EnhancedQueryCache) MarkNodeAsProblematic(node, queryType string) {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    if _, ok := c.problematicNodes[node]; !ok {
        c.problematicNodes[node] = make(map[string]bool)
    }
    c.problematicNodes[node][queryType] = true
}

func (c *EnhancedQueryCache) IsNodeProblematic(node, queryType string) bool {
    c.mutex.RLock()
    defer c.mutex.RUnlock()
    if nodeMap, ok := c.problematicNodes[node]; ok {
        return nodeMap[queryType]
    }
    return false
}
```

**Characteristics**:
- Key-value storage for query results
- Tracking of problematic nodes by query type
- Thread-safe with mutex
- No expiration mechanism
- No size limits

**Use Cases**:
- Caching query results
- Avoiding repeated queries to nodes that have failed
- Routing queries to more reliable nodes

**Limitations**:
- Memory usage grows unbounded
- No way to invalidate stale data
- No mechanism to "rehabilitate" problematic nodes

### V3 Approach: Stateless Design with Minimal Caching

The V3 implementation takes a more stateless approach with minimal caching:

```go
type MinimalCache struct {
    cache       map[string]interface{}
    expiry      map[string]time.Time
    maxSize     int
    expiration  time.Duration
    mutex       sync.RWMutex
}

func (c *MinimalCache) Get(key string) (interface{}, bool) {
    c.mutex.RLock()
    defer c.mutex.RUnlock()
    
    // Check if the entry exists and hasn't expired
    value, ok := c.cache[key]
    if !ok {
        return nil, false
    }
    
    expiry, ok := c.expiry[key]
    if !ok || time.Now().After(expiry) {
        // Entry has expired
        return nil, false
    }
    
    return value, true
}

func (c *MinimalCache) Set(key string, value interface{}) {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    
    // If we're at capacity, remove the oldest entry
    if len(c.cache) >= c.maxSize {
        var oldestKey string
        var oldestTime time.Time
        first := true
        
        for k, t := range c.expiry {
            if first || t.Before(oldestTime) {
                oldestKey = k
                oldestTime = t
                first = false
            }
        }
        
        delete(c.cache, oldestKey)
        delete(c.expiry, oldestKey)
    }
    
    // Add the new entry
    c.cache[key] = value
    c.expiry[key] = time.Now().Add(c.expiration)
}
```

**Characteristics**:
- Limited cache size with LRU eviction
- Entry expiration based on time
- Thread-safe with mutex
- More memory-efficient than previous approaches

**Use Cases**:
- Caching frequently accessed data for short periods
- Reducing redundant queries during a single healing operation
- Temporary storage of intermediate results

**Limitations**:
- Less effective for long-running operations
- May still evict useful data if cache size is too small
- No prioritization based on query cost or frequency

## Cache Key Construction

Effective caching requires well-designed cache keys that uniquely identify the data being cached. For the Accumulate network, cache keys should include:

1. **Query Type**: The type of query (account, chain, transaction, etc.)
2. **URL**: The URL being queried
3. **Parameters**: Any parameters that affect the query result

Example cache key construction:

```go
func constructCacheKey(queryType string, url *url.URL, params map[string]interface{}) string {
    // Start with the query type
    key := queryType + ":"
    
    // Add the URL
    key += url.String() + ":"
    
    // Add the parameters in a deterministic order
    var paramKeys []string
    for k := range params {
        paramKeys = append(paramKeys, k)
    }
    sort.Strings(paramKeys)
    
    for _, k := range paramKeys {
        key += k + "=" + fmt.Sprintf("%v", params[k]) + ","
    }
    
    return key
}
```

## Cache Invalidation Strategies

### Time-Based Invalidation

Time-based invalidation removes cache entries after a certain period:

```go
func (c *Cache) SetWithExpiry(key string, value interface{}, expiry time.Duration) {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    c.cache[key] = value
    c.expiry[key] = time.Now().Add(expiry)
}

// Background goroutine to clean up expired entries
func (c *Cache) cleanupExpiredEntries() {
    ticker := time.NewTicker(c.cleanupInterval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            c.mutex.Lock()
            now := time.Now()
            for k, exp := range c.expiry {
                if now.After(exp) {
                    delete(c.cache, k)
                    delete(c.expiry, k)
                }
            }
            c.mutex.Unlock()
        }
    }
}
```

### Size-Based Invalidation

Size-based invalidation removes the least recently used entries when the cache reaches a certain size:

```go
func (c *Cache) Set(key string, value interface{}) {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    
    // If we're at capacity, remove the least recently used entry
    if len(c.cache) >= c.maxSize {
        var lruKey string
        var lruTime time.Time
        first := true
        
        for k, t := range c.lastAccessed {
            if first || t.Before(lruTime) {
                lruKey = k
                lruTime = t
                first = false
            }
        }
        
        delete(c.cache, lruKey)
        delete(c.lastAccessed, lruKey)
    }
    
    // Add the new entry
    c.cache[key] = value
    c.lastAccessed[key] = time.Now()
}
```

### Event-Based Invalidation

Event-based invalidation removes cache entries when certain events occur:

```go
func (c *Cache) InvalidateByPrefix(prefix string) {
    c.mutex.Lock()
    defer c.mutex.Unlock()
    
    for k := range c.cache {
        if strings.HasPrefix(k, prefix) {
            delete(c.cache, k)
            delete(c.lastAccessed, k)
        }
    }
}
```

## Problematic Node Tracking

Problematic node tracking helps avoid repeated queries to nodes that have failed:

```go
type NodeTracker struct {
    problematicNodes map[string]map[string]time.Time // node -> query type -> last failure time
    cooldownPeriod   time.Duration
    mutex            sync.RWMutex
}

func (t *NodeTracker) MarkNodeAsProblematic(node, queryType string) {
    t.mutex.Lock()
    defer t.mutex.Unlock()
    
    if _, ok := t.problematicNodes[node]; !ok {
        t.problematicNodes[node] = make(map[string]time.Time)
    }
    
    t.problematicNodes[node][queryType] = time.Now()
}

func (t *NodeTracker) IsNodeProblematic(node, queryType string) bool {
    t.mutex.RLock()
    defer t.mutex.RUnlock()
    
    if nodeMap, ok := t.problematicNodes[node]; ok {
        if lastFailure, ok := nodeMap[queryType]; ok {
            // Check if the cooldown period has elapsed
            return time.Since(lastFailure) < t.cooldownPeriod
        }
    }
    
    return false
}

func (t *NodeTracker) GetHealthyNodes(nodes []string, queryType string) []string {
    var healthyNodes []string
    
    for _, node := range nodes {
        if !t.IsNodeProblematic(node, queryType) {
            healthyNodes = append(healthyNodes, node)
        }
    }
    
    return healthyNodes
}
```

## Best Practices

1. **Use Time-Limited Caching**: Always set an expiration time for cache entries to avoid stale data
2. **Limit Cache Size**: Set a maximum size for the cache to avoid memory issues
3. **Use Appropriate Cache Keys**: Ensure cache keys uniquely identify the data being cached
4. **Track Problematic Nodes**: Avoid repeated queries to nodes that have failed
5. **Implement Cache Metrics**: Track cache hit rates and other metrics to optimize cache performance
6. **Consider Query Cost**: Cache expensive queries for longer periods
7. **Use Thread-Safe Implementation**: Ensure the cache is thread-safe with appropriate locking

## Implementation Comparison

| Feature | V1 | V2 | V3 |
|---------|----|----|----| 
| Basic Key-Value Caching | ✅ | ✅ | ✅ |
| Thread Safety | ✅ | ✅ | ✅ |
| Problematic Node Tracking | ❌ | ✅ | ✅ |
| Cache Expiration | ❌ | ❌ | ✅ |
| Size Limits | ❌ | ❌ | ✅ |
| LRU Eviction | ❌ | ❌ | ✅ |
| Memory Efficiency | Low | Low | High |
| Implementation Complexity | Low | Medium | High |

## Conclusion

Effective caching is critical for the performance and reliability of the Accumulate network healing process. The caching strategy should be chosen based on the specific requirements of the implementation, with consideration for memory usage, query patterns, and network characteristics.

The V3 approach with minimal caching and a stateless design offers the best balance of performance and resource usage for most healing operations. However, for long-running operations or environments with limited network bandwidth, the V2 approach with enhanced query caching and problematic node tracking may be more appropriate.

## See Also

- [Healing Process Overview](./healing-process.md)
- [URL Construction](./url-construction.md)
- [Error Handling](./error-handling.md)
- [Original Implementation (v1)](../implementations/v1/implementation.md)
- [Enhanced Caching (v2)](../implementations/v2/index.md)
- [Stateless Design (v3)](../implementations/v3/development-plan.md)
