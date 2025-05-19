# Address Caching - V4 Implementation

```yaml
# AI-METADATA
document_type: implementation_detail
project: accumulate_network
component: address_caching
version: v4
related_files:
  - ./address-directory-fundamentals.md
  - ./peer-discovery-integration.md
  - ../v3/url-standardization.md
```

## Overview

This document details the address caching system implemented in the V4 version of the Accumulate Network debug tools. The caching system is designed to reduce redundant network requests, improve performance, and enhance reliability by storing and reusing address information.

## Caching Architecture

The address caching system follows a layered architecture:

```
┌─────────────────────────────────────────┐
│            Caching Manager              │
├─────────────┬─────────────┬─────────────┤
│  URL Cache  │  Node Cache │  Query Cache│
├─────────────┼─────────────┼─────────────┤
│  Cache      │  Eviction   │  Persistence│
│  Strategy   │  Policy     │  Layer      │
└─────────────┴─────────────┴─────────────┘
```

## Core Components

### 1. Caching Manager

The Caching Manager orchestrates the caching system:

```go
// CachingManager manages the address caching system
type CachingManager struct {
    urlCache    *URLCache
    nodeCache   *NodeCache
    queryCache  *QueryCache
    config      CachingConfig
    metrics     *CacheMetrics
}

// CachingConfig configures the caching system
type CachingConfig struct {
    URLCacheSize      int
    NodeCacheSize     int
    QueryCacheSize    int
    URLCacheTTL       time.Duration
    NodeCacheTTL      time.Duration
    QueryCacheTTL     time.Duration
    PersistenceEnabled bool
    PersistencePath    string
}

// CacheMetrics tracks cache performance metrics
type CacheMetrics struct {
    URLCacheHits      int64
    URLCacheMisses    int64
    NodeCacheHits     int64
    NodeCacheMisses   int64
    QueryCacheHits    int64
    QueryCacheMisses  int64
    mu                sync.Mutex
}

// NewCachingManager creates a new caching manager
func NewCachingManager(config CachingConfig) *CachingManager {
    // Create caches
    urlCache := NewURLCache(config.URLCacheSize, config.URLCacheTTL)
    nodeCache := NewNodeCache(config.NodeCacheSize, config.NodeCacheTTL)
    queryCache := NewQueryCache(config.QueryCacheSize, config.QueryCacheTTL)
    
    // Create metrics
    metrics := &CacheMetrics{}
    
    return &CachingManager{
        urlCache:    urlCache,
        nodeCache:   nodeCache,
        queryCache:  queryCache,
        config:      config,
        metrics:     metrics,
    }
}

// GetURLInfo gets URL information from cache or computes it
func (m *CachingManager) GetURLInfo(urlStr string, compute func(string) (*URLInfo, error)) (*URLInfo, error) {
    // Check cache
    cachedInfo, found := m.urlCache.Get(urlStr)
    if found {
        // Update metrics
        atomic.AddInt64(&m.metrics.URLCacheHits, 1)
        return cachedInfo, nil
    }
    
    // Update metrics
    atomic.AddInt64(&m.metrics.URLCacheMisses, 1)
    
    // Compute URL info
    info, err := compute(urlStr)
    if err != nil {
        return nil, err
    }
    
    // Cache URL info
    m.urlCache.Set(urlStr, info)
    
    return info, nil
}

// GetNodeInfo gets node information from cache or computes it
func (m *CachingManager) GetNodeInfo(nodeID string, compute func(string) (*NodeInfo, error)) (*NodeInfo, error) {
    // Check cache
    cachedInfo, found := m.nodeCache.Get(nodeID)
    if found {
        // Update metrics
        atomic.AddInt64(&m.metrics.NodeCacheHits, 1)
        return cachedInfo, nil
    }
    
    // Update metrics
    atomic.AddInt64(&m.metrics.NodeCacheMisses, 1)
    
    // Compute node info
    info, err := compute(nodeID)
    if err != nil {
        return nil, err
    }
    
    // Cache node info
    m.nodeCache.Set(nodeID, info)
    
    return info, nil
}

// GetQueryResult gets query result from cache or computes it
func (m *CachingManager) GetQueryResult(key QueryKey, compute func(QueryKey) (*QueryResult, error)) (*QueryResult, error) {
    // Check cache
    cachedResult, found := m.queryCache.Get(key)
    if found {
        // Update metrics
        atomic.AddInt64(&m.metrics.QueryCacheHits, 1)
        return cachedResult, nil
    }
    
    // Update metrics
    atomic.AddInt64(&m.metrics.QueryCacheMisses, 1)
    
    // Compute query result
    result, err := compute(key)
    if err != nil {
        return nil, err
    }
    
    // Cache query result
    m.queryCache.Set(key, result)
    
    return result, nil
}

// GetMetrics gets cache metrics
func (m *CachingManager) GetMetrics() CacheMetricsSnapshot {
    return CacheMetricsSnapshot{
        URLCacheHits:     atomic.LoadInt64(&m.metrics.URLCacheHits),
        URLCacheMisses:   atomic.LoadInt64(&m.metrics.URLCacheMisses),
        NodeCacheHits:    atomic.LoadInt64(&m.metrics.NodeCacheHits),
        NodeCacheMisses:  atomic.LoadInt64(&m.metrics.NodeCacheMisses),
        QueryCacheHits:   atomic.LoadInt64(&m.metrics.QueryCacheHits),
        QueryCacheMisses: atomic.LoadInt64(&m.metrics.QueryCacheMisses),
    }
}

// CacheMetricsSnapshot represents a snapshot of cache metrics
type CacheMetricsSnapshot struct {
    URLCacheHits     int64
    URLCacheMisses   int64
    NodeCacheHits    int64
    NodeCacheMisses  int64
    QueryCacheHits   int64
    QueryCacheMisses int64
}
```

### 2. URL Cache

The URL Cache stores URL information:

```go
// URLCache caches URL information
type URLCache struct {
    cache  *lru.Cache
    ttl    time.Duration
}

// URLInfo represents URL information
type URLInfo struct {
    URL            string
    PartitionID    string
    ChainID        string
    AccountType    string
    ParsedTime     time.Time
}

// NewURLCache creates a new URL cache
func NewURLCache(size int, ttl time.Duration) *URLCache {
    cache, _ := lru.New(size)
    return &URLCache{
        cache: cache,
        ttl:   ttl,
    }
}

// Get gets URL information from cache
func (c *URLCache) Get(urlStr string) (*URLInfo, bool) {
    // Get from cache
    value, found := c.cache.Get(urlStr)
    if !found {
        return nil, false
    }
    
    // Cast to URLCacheEntry
    entry, ok := value.(*URLCacheEntry)
    if !ok {
        return nil, false
    }
    
    // Check if expired
    if time.Since(entry.CacheTime) > c.ttl {
        c.cache.Remove(urlStr)
        return nil, false
    }
    
    return entry.Info, true
}

// Set sets URL information in cache
func (c *URLCache) Set(urlStr string, info *URLInfo) {
    // Create cache entry
    entry := &URLCacheEntry{
        Info:      info,
        CacheTime: time.Now(),
    }
    
    // Set in cache
    c.cache.Add(urlStr, entry)
}

// URLCacheEntry represents a URL cache entry
type URLCacheEntry struct {
    Info      *URLInfo
    CacheTime time.Time
}
```

### 3. Node Cache

The Node Cache stores node information:

```go
// NodeCache caches node information
type NodeCache struct {
    cache  *lru.Cache
    ttl    time.Duration
}

// NodeInfo represents node information
type NodeInfo struct {
    ID            string
    PartitionID   string
    IsValidator   bool
    Version       string
    Status        NodeStatus
    LastSeen      time.Time
}

// NewNodeCache creates a new node cache
func NewNodeCache(size int, ttl time.Duration) *NodeCache {
    cache, _ := lru.New(size)
    return &NodeCache{
        cache: cache,
        ttl:   ttl,
    }
}

// Get gets node information from cache
func (c *NodeCache) Get(nodeID string) (*NodeInfo, bool) {
    // Get from cache
    value, found := c.cache.Get(nodeID)
    if !found {
        return nil, false
    }
    
    // Cast to NodeCacheEntry
    entry, ok := value.(*NodeCacheEntry)
    if !ok {
        return nil, false
    }
    
    // Check if expired
    if time.Since(entry.CacheTime) > c.ttl {
        c.cache.Remove(nodeID)
        return nil, false
    }
    
    return entry.Info, true
}

// Set sets node information in cache
func (c *NodeCache) Set(nodeID string, info *NodeInfo) {
    // Create cache entry
    entry := &NodeCacheEntry{
        Info:      info,
        CacheTime: time.Now(),
    }
    
    // Set in cache
    c.cache.Add(nodeID, entry)
}

// NodeCacheEntry represents a node cache entry
type NodeCacheEntry struct {
    Info      *NodeInfo
    CacheTime time.Time
}
```

### 4. Query Cache

The Query Cache stores query results:

```go
// QueryCache caches query results
type QueryCache struct {
    cache  *lru.Cache
    ttl    time.Duration
}

// QueryKey represents a query key
type QueryKey struct {
    URL       string
    QueryType string
    Params    string
}

// QueryResult represents a query result
type QueryResult struct {
    Data       interface{}
    StatusCode int
    Error      string
    QueryTime  time.Time
}

// NewQueryCache creates a new query cache
func NewQueryCache(size int, ttl time.Duration) *QueryCache {
    cache, _ := lru.New(size)
    return &QueryCache{
        cache: cache,
        ttl:   ttl,
    }
}

// Get gets query result from cache
func (c *QueryCache) Get(key QueryKey) (*QueryResult, bool) {
    // Create cache key
    cacheKey := fmt.Sprintf("%s:%s:%s", key.URL, key.QueryType, key.Params)
    
    // Get from cache
    value, found := c.cache.Get(cacheKey)
    if !found {
        return nil, false
    }
    
    // Cast to QueryCacheEntry
    entry, ok := value.(*QueryCacheEntry)
    if !ok {
        return nil, false
    }
    
    // Check if expired
    if time.Since(entry.CacheTime) > c.ttl {
        c.cache.Remove(cacheKey)
        return nil, false
    }
    
    return entry.Result, true
}

// Set sets query result in cache
func (c *QueryCache) Set(key QueryKey, result *QueryResult) {
    // Create cache key
    cacheKey := fmt.Sprintf("%s:%s:%s", key.URL, key.QueryType, key.Params)
    
    // Create cache entry
    entry := &QueryCacheEntry{
        Result:    result,
        CacheTime: time.Now(),
    }
    
    // Set in cache
    c.cache.Add(cacheKey, entry)
}

// QueryCacheEntry represents a query cache entry
type QueryCacheEntry struct {
    Result    *QueryResult
    CacheTime time.Time
}
```

## Cache Strategies

The caching system supports different caching strategies:

### 1. Time-Based Caching

```go
// TimeBasedCacheStrategy implements time-based caching
type TimeBasedCacheStrategy struct {
    ttl time.Duration
}

// NewTimeBasedCacheStrategy creates a new time-based cache strategy
func NewTimeBasedCacheStrategy(ttl time.Duration) *TimeBasedCacheStrategy {
    return &TimeBasedCacheStrategy{
        ttl: ttl,
    }
}

// ShouldCache determines if a value should be cached
func (s *TimeBasedCacheStrategy) ShouldCache(key string, value interface{}) bool {
    // Always cache
    return true
}

// IsExpired determines if a cached value is expired
func (s *TimeBasedCacheStrategy) IsExpired(key string, value interface{}, cacheTime time.Time) bool {
    // Check if expired based on TTL
    return time.Since(cacheTime) > s.ttl
}
```

### 2. Response-Based Caching

```go
// ResponseBasedCacheStrategy implements response-based caching
type ResponseBasedCacheStrategy struct {
    ttl time.Duration
}

// NewResponseBasedCacheStrategy creates a new response-based cache strategy
func NewResponseBasedCacheStrategy(ttl time.Duration) *ResponseBasedCacheStrategy {
    return &ResponseBasedCacheStrategy{
        ttl: ttl,
    }
}

// ShouldCache determines if a value should be cached
func (s *ResponseBasedCacheStrategy) ShouldCache(key string, value interface{}) bool {
    // Check if value is a QueryResult
    result, ok := value.(*QueryResult)
    if !ok {
        return true
    }
    
    // Don't cache errors
    if result.Error != "" {
        return false
    }
    
    // Don't cache non-200 responses
    if result.StatusCode != 200 {
        return false
    }
    
    return true
}

// IsExpired determines if a cached value is expired
func (s *ResponseBasedCacheStrategy) IsExpired(key string, value interface{}, cacheTime time.Time) bool {
    // Check if expired based on TTL
    return time.Since(cacheTime) > s.ttl
}
```

### 3. Hybrid Caching

```go
// HybridCacheStrategy implements hybrid caching
type HybridCacheStrategy struct {
    ttl           time.Duration
    errorTTL      time.Duration
    successTTL    time.Duration
}

// NewHybridCacheStrategy creates a new hybrid cache strategy
func NewHybridCacheStrategy(ttl, errorTTL, successTTL time.Duration) *HybridCacheStrategy {
    return &HybridCacheStrategy{
        ttl:        ttl,
        errorTTL:   errorTTL,
        successTTL: successTTL,
    }
}

// ShouldCache determines if a value should be cached
func (s *HybridCacheStrategy) ShouldCache(key string, value interface{}) bool {
    // Always cache
    return true
}

// IsExpired determines if a cached value is expired
func (s *HybridCacheStrategy) IsExpired(key string, value interface{}, cacheTime time.Time) bool {
    // Check if value is a QueryResult
    result, ok := value.(*QueryResult)
    if !ok {
        // Use default TTL for non-QueryResult values
        return time.Since(cacheTime) > s.ttl
    }
    
    // Use error TTL for errors
    if result.Error != "" || result.StatusCode != 200 {
        return time.Since(cacheTime) > s.errorTTL
    }
    
    // Use success TTL for successful responses
    return time.Since(cacheTime) > s.successTTL
}
```

## Eviction Policies

The caching system supports different eviction policies:

### 1. LRU Eviction

```go
// LRUEvictionPolicy implements LRU eviction
type LRUEvictionPolicy struct {
    cache *lru.Cache
}

// NewLRUEvictionPolicy creates a new LRU eviction policy
func NewLRUEvictionPolicy(size int) *LRUEvictionPolicy {
    cache, _ := lru.New(size)
    return &LRUEvictionPolicy{
        cache: cache,
    }
}

// Add adds a value to the cache
func (p *LRUEvictionPolicy) Add(key string, value interface{}) {
    p.cache.Add(key, value)
}

// Get gets a value from the cache
func (p *LRUEvictionPolicy) Get(key string) (interface{}, bool) {
    return p.cache.Get(key)
}

// Remove removes a value from the cache
func (p *LRUEvictionPolicy) Remove(key string) {
    p.cache.Remove(key)
}

// Purge purges all values from the cache
func (p *LRUEvictionPolicy) Purge() {
    p.cache.Purge()
}
```

### 2. TTL Eviction

```go
// TTLEvictionPolicy implements TTL eviction
type TTLEvictionPolicy struct {
    cache     map[string]*TTLCacheEntry
    ttl       time.Duration
    mu        sync.RWMutex
    janitor   *time.Ticker
    done      chan struct{}
}

// TTLCacheEntry represents a TTL cache entry
type TTLCacheEntry struct {
    Value     interface{}
    CacheTime time.Time
}

// NewTTLEvictionPolicy creates a new TTL eviction policy
func NewTTLEvictionPolicy(ttl time.Duration, cleanupInterval time.Duration) *TTLEvictionPolicy {
    policy := &TTLEvictionPolicy{
        cache:   make(map[string]*TTLCacheEntry),
        ttl:     ttl,
        janitor: time.NewTicker(cleanupInterval),
        done:    make(chan struct{}),
    }
    
    // Start janitor
    go policy.janitorTask()
    
    return policy
}

// Add adds a value to the cache
func (p *TTLEvictionPolicy) Add(key string, value interface{}) {
    p.mu.Lock()
    defer p.mu.Unlock()
    
    // Create cache entry
    entry := &TTLCacheEntry{
        Value:     value,
        CacheTime: time.Now(),
    }
    
    // Add to cache
    p.cache[key] = entry
}

// Get gets a value from the cache
func (p *TTLEvictionPolicy) Get(key string) (interface{}, bool) {
    p.mu.RLock()
    defer p.mu.RUnlock()
    
    // Get from cache
    entry, found := p.cache[key]
    if !found {
        return nil, false
    }
    
    // Check if expired
    if time.Since(entry.CacheTime) > p.ttl {
        return nil, false
    }
    
    return entry.Value, true
}

// Remove removes a value from the cache
func (p *TTLEvictionPolicy) Remove(key string) {
    p.mu.Lock()
    defer p.mu.Unlock()
    
    delete(p.cache, key)
}

// Purge purges all values from the cache
func (p *TTLEvictionPolicy) Purge() {
    p.mu.Lock()
    defer p.mu.Unlock()
    
    p.cache = make(map[string]*TTLCacheEntry)
}

// Stop stops the janitor
func (p *TTLEvictionPolicy) Stop() {
    p.janitor.Stop()
    close(p.done)
}

// janitorTask runs the janitor task
func (p *TTLEvictionPolicy) janitorTask() {
    for {
        select {
        case <-p.janitor.C:
            p.cleanup()
        case <-p.done:
            return
        }
    }
}

// cleanup cleans up expired entries
func (p *TTLEvictionPolicy) cleanup() {
    p.mu.Lock()
    defer p.mu.Unlock()
    
    now := time.Now()
    for key, entry := range p.cache {
        if now.Sub(entry.CacheTime) > p.ttl {
            delete(p.cache, key)
        }
    }
}
```

## Persistence Layer

The caching system supports persistence of cache data:

```go
// CachePersistence provides persistence for cache data
type CachePersistence struct {
    path  string
    codec Codec
}

// Codec encodes and decodes cache data
type Codec interface {
    Encode(v interface{}) ([]byte, error)
    Decode(data []byte, v interface{}) error
}

// NewCachePersistence creates a new cache persistence
func NewCachePersistence(path string, codec Codec) *CachePersistence {
    return &CachePersistence{
        path:  path,
        codec: codec,
    }
}

// Save saves cache data to disk
func (p *CachePersistence) Save(data interface{}) error {
    // Create directory if it doesn't exist
    dir := filepath.Dir(p.path)
    if err := os.MkdirAll(dir, 0755); err != nil {
        return err
    }
    
    // Encode data
    encoded, err := p.codec.Encode(data)
    if err != nil {
        return err
    }
    
    // Write to file
    return ioutil.WriteFile(p.path, encoded, 0644)
}

// Load loads cache data from disk
func (p *CachePersistence) Load(data interface{}) error {
    // Check if file exists
    if _, err := os.Stat(p.path); os.IsNotExist(err) {
        return nil
    }
    
    // Read file
    encoded, err := ioutil.ReadFile(p.path)
    if err != nil {
        return err
    }
    
    // Decode data
    return p.codec.Decode(encoded, data)
}

// JSONCodec implements Codec using JSON
type JSONCodec struct{}

// Encode encodes data to JSON
func (c *JSONCodec) Encode(v interface{}) ([]byte, error) {
    return json.MarshalIndent(v, "", "  ")
}

// Decode decodes JSON data
func (c *JSONCodec) Decode(data []byte, v interface{}) error {
    return json.Unmarshal(data, v)
}
```

## Integration with Healing Process

The caching system is integrated with the healing process:

```go
// HealingCache provides caching for the healing process
type HealingCache struct {
    cachingManager *CachingManager
}

// NewHealingCache creates a new healing cache
func NewHealingCache(cachingManager *CachingManager) *HealingCache {
    return &HealingCache{
        cachingManager: cachingManager,
    }
}

// GetURLInfo gets URL information
func (c *HealingCache) GetURLInfo(urlStr string) (*URLInfo, error) {
    return c.cachingManager.GetURLInfo(urlStr, func(urlStr string) (*URLInfo, error) {
        // Parse URL
        parsedURL, err := url.Parse(urlStr)
        if err != nil {
            return nil, err
        }
        
        // Extract partition ID
        partitionID, err := extractPartitionFromURL(parsedURL)
        if err != nil {
            return nil, err
        }
        
        // Extract chain ID
        chainID, err := extractChainIDFromURL(parsedURL)
        if err != nil {
            return nil, err
        }
        
        // Extract account type
        accountType, err := extractAccountTypeFromURL(parsedURL)
        if err != nil {
            return nil, err
        }
        
        // Create URL info
        info := &URLInfo{
            URL:         urlStr,
            PartitionID: partitionID,
            ChainID:     chainID,
            AccountType: accountType,
            ParsedTime:  time.Now(),
        }
        
        return info, nil
    })
}

// GetQueryResult gets query result
func (c *HealingCache) GetQueryResult(url, queryType, params string) (*QueryResult, error) {
    // Create query key
    key := QueryKey{
        URL:       url,
        QueryType: queryType,
        Params:    params,
    }
    
    return c.cachingManager.GetQueryResult(key, func(key QueryKey) (*QueryResult, error) {
        // Execute query
        data, statusCode, err := executeQuery(key.URL, key.QueryType, key.Params)
        
        // Create query result
        result := &QueryResult{
            Data:       data,
            StatusCode: statusCode,
            QueryTime:  time.Now(),
        }
        
        // Set error if any
        if err != nil {
            result.Error = err.Error()
        }
        
        return result, nil
    })
}

// executeQuery executes a query
func executeQuery(url, queryType, params string) (interface{}, int, error) {
    // Implementation details omitted for brevity
    return nil, 200, nil
}
```

## Benefits of Caching

The address caching system provides several benefits:

1. **Reduced Network Requests**: Caching reduces the number of network requests
2. **Improved Performance**: Cached results are returned faster than network requests
3. **Enhanced Reliability**: Caching provides fallback when network requests fail
4. **Reduced Load**: Caching reduces the load on network nodes
5. **Consistent Results**: Caching provides consistent results for repeated queries

## Comparison with Previous Versions

| Feature | V1 | V2 | V3 | V4 |
|---------|----|----|----|----|
| URL Caching | None | Basic | Improved | Comprehensive |
| Node Caching | None | None | Basic | Comprehensive |
| Query Caching | None | None | Basic | Comprehensive |
| Cache Strategies | None | None | Basic | Multiple |
| Eviction Policies | None | None | Basic | Multiple |
| Persistence | None | None | None | Supported |

## Best Practices

When working with the address caching system, follow these best practices:

1. **Configure Cache Sizes**: Set appropriate cache sizes based on memory constraints
2. **Configure TTLs**: Set appropriate TTLs based on data volatility
3. **Choose Appropriate Strategies**: Choose appropriate caching strategies based on use cases
4. **Monitor Cache Metrics**: Monitor cache metrics to optimize cache performance
5. **Enable Persistence**: Enable persistence to maintain cache data across restarts

## Conclusion

The address caching system in the V4 implementation provides a comprehensive, flexible approach to caching address information. By reducing redundant network requests, improving performance, and enhancing reliability, it significantly improves the efficiency of the healing process.

The comparison with previous versions shows the significant improvements in caching capabilities in the V4 implementation, making it a robust foundation for the healing process.

## See Also

- [Address Directory Fundamentals](./address-directory-fundamentals.md): Core concepts of the Address Directory
- [Peer Discovery Integration](./peer-discovery-integration.md): Integration of Peer Discovery with the Address Directory
- [URL Standardization](../v3/url-standardization.md): URL standardization in V3
