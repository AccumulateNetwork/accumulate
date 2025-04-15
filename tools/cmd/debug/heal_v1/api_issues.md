# API Issues and Solutions

## Common API Errors and Their Solutions

### Chain Query Type Mismatch

#### Error
```
rpc returned unexpected type: want *api.ChainRecord, got *api.RecordRange[gitlab.com/accumulatenetwork/accumulate/pkg/api/v3.Record]
```

#### Cause
This error occurs because of a mismatch between the expected return type and the actual return type from the API. The V3 API has evolved, and the QueryChain method now returns a different type than what older code expects.

Specifically:
1. The code expects a `*api.ChainRecord` type
2. The API returns a `*api.RecordRange[gitlab.com/accumulatenetwork/accumulate/pkg/api/v3.Record]` type

This happens when the `Name` parameter is not properly specified in the `ChainQuery` struct.

#### Solution
When using the QueryChain method, ensure you're using it correctly:

1. **Correct Usage**:
   ```go
   // This is the correct way to query a chain
   chain, err := client.QueryChain(ctx, url, &api.ChainQuery{Name: chainName})
   ```

2. **Important Parameters**:
   - The `Name` parameter in the ChainQuery struct is **critical** for specifying which chain to query
   - For anchor chains, use `"anchor"` or `"anchor-sequence"` as appropriate
   - For synthetic transactions, use `"synthetic-sequence-0"`, `"synthetic-sequence-1"`, or `"synthetic-sequence-index"`
   - **Never** leave the Name parameter empty or omit it - this will cause the type mismatch error

3. **Handling the Response**:
   - Check for errors first
   - Access the chain height using `chain.Count` (not `chain.Chain.Height`)
   - Remember that the first entry is at index 1, not 0

#### Example Implementation
```go
func findAnchorGaps(ctx context.Context, src, dst *protocol.PartitionInfo) ([]sequenceInfo, error) {
    // Use consistent URL construction (raw partition URLs)
    srcUrl := protocol.PartitionUrl(src.ID)
    dstUrl := protocol.PartitionUrl(dst.ID)
    
    // Query source chain with the correct chain name
    srcChain, err := client.QueryChain(ctx, srcUrl.JoinPath("anchor"), &api.ChainQuery{Name: "anchor"})
    if err != nil {
        if errors.Code(err) == errors.NotFound {
            return nil, nil
        }
        return nil, fmt.Errorf("query source chain: %w", err)
    }
    
    // Query destination chain with the correct chain name
    dstChain, err := client.QueryChain(ctx, dstUrl.JoinPath("anchor", src.ID), &api.ChainQuery{Name: "anchor"})
    if err != nil {
        if errors.Code(err) == errors.NotFound {
            // If the destination chain doesn't exist, all entries need to be healed
            var gaps []sequenceInfo
            for i := uint64(1); i <= srcChain.Count; i++ {
                gaps = append(gaps, sequenceInfo{
                    Source:      src.ID,
                    Destination: dst.ID,
                    Number:      i,
                })
            }
            return gaps, nil
        }
        return nil, fmt.Errorf("query destination chain: %w", err)
    }
    
    // Find gaps
    var gaps []sequenceInfo
    for i := dstChain.Count + 1; i <= srcChain.Count; i++ {
        gaps = append(gaps, sequenceInfo{
            Source:      src.ID,
            Destination: dst.ID,
            Number:      i,
        })
    }
    
    return gaps, nil
}
```

## URL Construction Issues

### Inconsistent URL Patterns

There is a fundamental difference in how URLs are constructed between different parts of the codebase:

1. **sequence.go** uses raw partition URLs for tracking:
   - Example: `acc://bvn-Apollo.acme`
   - Then joins paths: `acc://bvn-Apollo.acme/anchor`

2. **heal_anchor.go** appends the partition ID to the anchor pool URL:
   - Example: `acc://dn.acme/anchors/Apollo`

This discrepancy can cause anchor healing to fail because:
- The code might be looking for anchors at different URL paths
- Queries might return "element does not exist" errors when checking the wrong URL format
- Anchor relationships might not be properly maintained between partitions

#### Common URL Patterns in the Codebase

| Purpose | Correct URL Pattern | Example |
|---------|---------------------|----------|
| Partition URL | `protocol.PartitionUrl(partitionID)` | `acc://bvn-Apollo.acme` |
| Anchor Chain | `partitionUrl.JoinPath("anchor")` | `acc://bvn-Apollo.acme/anchor` |
| Anchor Sequence | `partitionUrl.JoinPath(protocol.AnchorPool)` | `acc://bvn-Apollo.acme/anchors` |
| Destination Anchor | `dstUrl.JoinPath("anchor", srcID)` | `acc://dn.acme/anchor/Apollo` |

#### Testing for URL Existence

When testing URLs in a real network, you may encounter "not found" errors for some chains. This is normal and expected behavior, especially for:

- Newly created partitions
- Chains that haven't been initialized yet
- Testing environments

Your code should handle these "not found" errors gracefully:

### Recommended Approach

Standardize on the sequence.go approach (raw partition URLs) as it appears to be the original implementation:

```go
// Correct approach - use raw partition URLs
srcUrl := protocol.PartitionUrl(src.ID)  // e.g., acc://bvn-Apollo.acme
dstUrl := protocol.PartitionUrl(dst.ID)  // e.g., acc://dn.acme

// Then join paths as needed
srcAnchorChain := srcUrl.JoinPath("anchor")
dstAnchorChain := dstUrl.JoinPath("anchor", src.ID)
```

Ensure all related code (including the caching system) is aware of the correct URL format to maintain consistency across the codebase.

## Caching System for API Requests

To reduce redundant network requests and improve performance, implement a caching system that:

1. Stores query results in a map indexed by a key composed of the URL and query type
2. Tracks problematic nodes to avoid querying them for certain types of requests
3. Sets appropriate TTL (Time-To-Live) values for different types of queries

This is especially helpful for account queries and chain queries which frequently cause "element does not exist" errors.

### Caching Strategy for Different Query Types

| Query Type | TTL | Rationale |
|------------|-----|------------|
| Chain Height | 10 seconds | Chain heights change frequently but caching briefly reduces load |
| Account State | 30 seconds | Account state changes less frequently than chain heights |
| Transaction Status | 5 seconds | Transaction status can change quickly during processing |
| Network Status | 60 seconds | Network configuration changes infrequently |
| Error Responses | 5 seconds | Cache errors briefly to avoid hammering failing nodes |

### Handling "Not Found" Errors

Special consideration should be given to "element does not exist" errors:

1. These errors are common and expected in many scenarios
2. They should be cached, but with a shorter TTL than successful responses
3. The caching system should distinguish between different types of errors

```go
// Example of error-aware caching
if errors.Code(err) == errors.NotFound {
    // Cache the "not found" result for a shorter period
    cache.Set(key, "NOT_FOUND", 5*time.Second)
    return nil, nil
} else if err != nil {
    // Don't cache other errors
    return nil, fmt.Errorf("query failed: %w", err)
}
```

### Example Caching Implementation

```go
type QueryCache struct {
    mutex sync.RWMutex
    data  map[string]CacheEntry
}

type CacheEntry struct {
    response interface{}
    expires  time.Time
}

func NewQueryCache() *QueryCache {
    return &QueryCache{
        data: make(map[string]CacheEntry),
    }
}

func (c *QueryCache) Get(url string, queryType string) (interface{}, bool) {
    key := url + ":" + queryType
    c.mutex.RLock()
    defer c.mutex.RUnlock()
    
    entry, ok := c.data[key]
    if !ok {
        return nil, false
    }
    
    if time.Now().After(entry.expires) {
        return nil, false
    }
    
    return entry.response, true
}

func (c *QueryCache) Set(url string, queryType string, response interface{}, ttl time.Duration) {
    key := url + ":" + queryType
    c.mutex.Lock()
    defer c.mutex.Unlock()
    
    c.data[key] = CacheEntry{
        response: response,
        expires:  time.Now().Add(ttl),
    }
}
```

By implementing these solutions, you can significantly reduce API errors and improve the reliability of the healing process.
