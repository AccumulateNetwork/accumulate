# V3 API Node Discovery and Connection Issues Analysis

## Executive Summary

After a deep dive into the V3 API implementation, node discovery mechanisms, and connection handling in the load tests, several critical issues have been identified that affect the stability of node discovery and V3 API connections. The problems stem from a combination of architectural limitations, missing connection pooling, hardcoded timeouts, and inadequate retry mechanisms.

## Key Findings

### 1. Hardcoded Timeout in JSON-RPC Client

**Location:** `pkg/api/v3/jsonrpc/client.go:43`

The V3 JSON-RPC client has a hardcoded 15-second timeout:
```go
func NewClient(server string) *Client {
    c := new(Client)
    c.Client.Timeout = 15 * time.Second  // HARDCODED!
    c.Server = server
    return c
}
```

**Impact:**
- All V3 API calls timeout after 15 seconds regardless of operation complexity
- No way to configure timeout for different use cases
- Load tests may fail under high load when responses take longer

### 2. No Connection Pooling

**Issue:** Each `jsonrpc.NewClient()` call creates a new HTTP client with default transport settings.

**Evidence:**
- No `Transport` configuration in `pkg/api/v3/jsonrpc/client.go`
- Default Go HTTP client creates new connections for each request
- No connection reuse between API calls

**Impact:**
- Connection exhaustion under load
- Increased latency due to connection establishment overhead
- TCP port exhaustion on both client and server
- "connection refused" errors when hitting system limits

### 3. Duplicate Discovery Implementations

Multiple endpoint discovery implementations exist with different approaches:

1. **`test/load/devnet_endpoint.go`** - Basic discovery with process scanning
2. **`test/load/sl-load/devnet_endpoint.go`** - Duplicate of above (exact copy)
3. **`test/load/devnet_smart_discovery.go`** - Advanced discovery with caching

**Problems:**
- Code duplication leads to inconsistent behavior
- Different tests may use different discovery methods
- Maintenance burden of multiple implementations

### 4. IP Address Confusion

**Historical Issue:** Originally used `127.0.1.1` which was changed to `127.0.0.1`

**Current State:**
- `devnet_config.sh` correctly uses `127.0.0.1`
- Some discovery code still scans `127.0.1.x` range
- Inconsistent IP handling in different parts of codebase

### 5. Port Discovery Challenges

**Process:**
1. Find accumulated process PID
2. Use `lsof` or `ss` to find listening ports
3. Test each port with `/v3` endpoint

**Issues:**
- Requires elevated permissions for `lsof` on some systems
- `ss` output parsing is fragile
- No standardized port allocation scheme
- Race conditions between port discovery and API availability

### 6. Missing Retry Logic

**Current State:**
- No built-in retry mechanism in V3 client
- Load tests don't retry failed requests
- Transient network failures cause immediate test failures

**Impact:**
- False negatives in load testing
- Reduced reliability in production
- No resilience to temporary network issues

### 7. Service Discovery Limitations

**Current Implementation:**
- Basic HTTP endpoint testing with `NetworkStatus` call
- No health checking beyond initial connection
- No load balancing across multiple nodes
- No automatic failover

## Root Cause Analysis

### Primary Causes

1. **Architectural Limitation:** The V3 JSON-RPC client was designed for simple use cases without considering high-load scenarios or connection management.

2. **Evolution Without Refactoring:** As the system grew, new discovery mechanisms were added without consolidating or removing old ones.

3. **Testing Gap:** Load testing requirements exposed limitations that weren't apparent in normal usage.

4. **Default HTTP Client:** Relying on Go's default HTTP client without customization leads to suboptimal connection handling.

### Secondary Causes

1. **Documentation Gap:** No clear documentation on proper V3 client usage patterns
2. **Missing Best Practices:** No guidance on connection pooling or client reuse
3. **Inconsistent Error Handling:** Different error handling approaches across the codebase

## Impact Assessment

### High Impact Issues
1. Connection exhaustion under load (causes test failures)
2. Hardcoded timeout (blocks long-running operations)
3. No connection pooling (performance degradation)

### Medium Impact Issues
1. Code duplication (maintenance burden)
2. Missing retry logic (reduced reliability)
3. IP address confusion (setup complexity)

### Low Impact Issues
1. Port discovery fragility (occasional failures)
2. Documentation gaps (developer confusion)

## Recommended Solutions

### Immediate Fixes (Can be implemented now)

1. **Client Pooling Helper**
```go
// Add to test/load/client_pool.go
var clientPool = sync.Map{}

func GetPooledClient(endpoint string) *jsonrpc.Client {
    if client, ok := clientPool.Load(endpoint); ok {
        return client.(*jsonrpc.Client)
    }
    
    client := jsonrpc.NewClient(endpoint)
    // Configure transport
    transport := &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
    }
    client.Client.Transport = transport
    client.Client.Timeout = 30 * time.Second
    
    clientPool.Store(endpoint, client)
    return client
}
```

2. **Retry Wrapper**
```go
func QueryWithRetry(ctx context.Context, client *jsonrpc.Client, 
                    account *url.URL, maxRetries int) (*api.AccountRecord, error) {
    for i := 0; i < maxRetries; i++ {
        resp, err := client.Query(ctx, account, nil)
        if err == nil {
            return resp, nil
        }
        if !isRetryable(err) {
            return nil, err
        }
        time.Sleep(time.Duration(i) * time.Second)
    }
    return nil, fmt.Errorf("max retries exceeded")
}
```

3. **Consolidate Discovery Code**
   - Remove duplicate `sl-load/devnet_endpoint.go`
   - Use `devnet_smart_discovery.go` as primary implementation
   - Update all tests to use consolidated discovery

### Long-term Fixes (Require API changes)

1. **Configurable Client Options**
```go
type ClientOptions struct {
    Timeout         time.Duration
    MaxIdleConns    int
    MaxConnsPerHost int
    RetryPolicy     RetryPolicy
}

func NewClientWithOptions(server string, opts ClientOptions) *Client
```

2. **Built-in Connection Pooling**
   - Implement connection pooling in the V3 client itself
   - Add metrics for connection usage
   - Implement circuit breaker pattern

3. **Service Registry**
   - Implement proper service discovery with health checking
   - Support multiple endpoints per service
   - Automatic failover and load balancing

## Testing Recommendations

1. **Connection Pool Testing**
   - Test with pooled clients vs new clients
   - Measure connection count and performance
   - Verify no resource leaks

2. **Load Testing Improvements**
   - Use pooled clients in all load tests
   - Implement retry logic for transient failures
   - Add connection metrics to test output

3. **Discovery Testing**
   - Test discovery with various network configurations
   - Verify fallback mechanisms work
   - Test with process restarts

## Migration Path

### Phase 1: Immediate Mitigation (Week 1)
1. Implement client pooling helper
2. Add retry wrappers to critical paths
3. Update load tests to use pooled clients

### Phase 2: Consolidation (Week 2)
1. Remove duplicate discovery code
2. Standardize on smart discovery
3. Document best practices

### Phase 3: API Enhancement (Week 3-4)
1. Design configurable client options
2. Implement in V3 client
3. Update all usage sites

### Phase 4: Production Rollout (Week 5-6)
1. Test in staging environment
2. Monitor connection metrics
3. Gradual production rollout

## Monitoring and Metrics

### Key Metrics to Track
1. Active connection count
2. Connection establishment time
3. API call success rate
4. Retry count and success rate
5. Discovery success rate

### Alerting Thresholds
1. Connection count > 1000
2. API success rate < 95%
3. Discovery failures > 5%
4. Average latency > 1s

## Conclusion

The V3 API node discovery and connection issues stem from architectural limitations and evolution without refactoring. While the immediate issues can be mitigated with client pooling and retry logic, long-term stability requires API enhancements and proper connection management.

The most critical issue is the lack of connection pooling, which causes connection exhaustion under load. This can be immediately addressed with the client pooling helper, providing significant stability improvements for load testing.

## Appendix: File Locations

### Core V3 Implementation
- `pkg/api/v3/jsonrpc/client.go` - JSON-RPC client
- `cmd/accumulated/run/api.go` - API service setup
- `cmd/accumulated/run/http.go` - HTTP server configuration

### Discovery Implementations
- `test/load/devnet_endpoint.go` - Basic discovery
- `test/load/sl-load/devnet_endpoint.go` - Duplicate (should be removed)
- `test/load/devnet_smart_discovery.go` - Advanced discovery

### Load Test Files
- `test/load/sl-load/sl_load.go` - Main load test logic
- `test/load/sl-load/sl_helpers.go` - Helper functions
- `test/load/simple_100k_test.go` - 100k transaction test

### Configuration
- `test/load/devnet_config.sh` - DevNet setup script
- `test/load/v3_connection_fixes.md` - Previous fix documentation

### Related Documentation
- `test/load/LOAD_TEST_GUIDE.md` - Load testing guide
- `CLAUDE.md` - Project memory and instructions