# Phase 1: Core Infrastructure

## Objective
Implement connection pooling, client reuse, and retry logic to provide immediate reliability and performance improvements.

## Timeline
Week 1 (5 days)

## Components

### 1. Pooled HTTP Transport (`/pkg/api/v3/jsonrpc/transport.go`)

Create optimized HTTP transport with connection pooling:

```go
package jsonrpc

import (
    "net"
    "net/http"
    "sync"
    "time"
)

type PooledTransport struct {
    *http.Transport
    mu    sync.RWMutex
    stats ConnectionStats
}

type ConnectionStats struct {
    ActiveConnections int64
    TotalRequests    int64
    FailedRequests   int64
    AverageLatency   time.Duration
}

func NewPooledTransport() *PooledTransport {
    return &PooledTransport{
        Transport: &http.Transport{
            MaxIdleConns:        100,
            MaxIdleConnsPerHost: 20,
            IdleConnTimeout:     90 * time.Second,
            DisableKeepAlives:   false,
            DialContext: (&net.Dialer{
                Timeout:   30 * time.Second,
                KeepAlive: 30 * time.Second,
            }).DialContext,
            TLSHandshakeTimeout:   10 * time.Second,
            ExpectContinueTimeout: 1 * time.Second,
        },
    }
}
```

### 2. Client Pool Manager (`/pkg/client/pool.go`)

Manage reusable client instances:

```go
package client

type ClientPool struct {
    mu        sync.RWMutex
    clients   map[string]*jsonrpc.Client
    transport *jsonrpc.PooledTransport
}

var (
    globalPool     *ClientPool
    globalPoolOnce sync.Once
)

func GetPooledClient(endpoint string) *jsonrpc.Client {
    globalPoolOnce.Do(func() {
        globalPool = &ClientPool{
            clients:   make(map[string]*jsonrpc.Client),
            transport: jsonrpc.NewPooledTransport(),
        }
    })
    
    return globalPool.getOrCreate(endpoint)
}
```

### 3. Retry Logic (`/pkg/client/retry.go`)

Implement exponential backoff retry:

```go
package client

type RetryConfig struct {
    MaxAttempts   int
    InitialDelay  time.Duration
    MaxDelay      time.Duration
    BackoffFactor float64
}

var DefaultRetryConfig = RetryConfig{
    MaxAttempts:   3,
    InitialDelay:  100 * time.Millisecond,
    MaxDelay:      5 * time.Second,
    BackoffFactor: 2.0,
}

func WithRetry(ctx context.Context, cfg RetryConfig, fn func(context.Context) error) error {
    var lastErr error
    delay := cfg.InitialDelay
    
    for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
        if attempt > 0 {
            select {
            case <-time.After(delay):
            case <-ctx.Done():
                return ctx.Err()
            }
            delay = time.Duration(float64(delay) * cfg.BackoffFactor)
            if delay > cfg.MaxDelay {
                delay = cfg.MaxDelay
            }
        }
        
        err := fn(ctx)
        if err == nil {
            return nil
        }
        
        if !isRetryable(err) {
            return err
        }
        
        lastErr = err
    }
    
    return fmt.Errorf("max retries exceeded: %w", lastErr)
}
```

## Implementation Steps

### Day 1: HTTP Transport
1. Create `/pkg/api/v3/jsonrpc/transport.go`
2. Implement `PooledTransport` with connection pooling
3. Add connection statistics tracking
4. Unit test transport behavior

### Day 2: Client Pool
1. Create `/pkg/client/pool.go`
2. Implement global client pool
3. Add thread-safe client management
4. Test concurrent access patterns

### Day 3: Retry Logic
1. Create `/pkg/client/retry.go`
2. Implement exponential backoff
3. Add error classification for retryable errors
4. Test retry scenarios

### Day 4: Integration
1. Modify existing tests to use pooled clients
2. Add helper functions for easy adoption
3. Create compatibility wrappers
4. Document usage patterns

### Day 5: Testing & Validation
1. Load testing with connection pooling
2. Verify 26.7% performance improvement
3. Test retry logic under failures
4. Memory leak testing

## Usage Examples

### Basic Usage
```go
// Old way - creates new connection each time
client := jsonrpc.NewClient(endpoint)

// New way - uses connection pool
client := client.GetPooledClient(endpoint)
```

### With Retry
```go
err := client.WithRetry(ctx, client.DefaultRetryConfig, func(ctx context.Context) error {
    return client.Query(ctx, account, nil)
})
```

### In Tests
```go
func TestStreamlinedLoad(t *testing.T) {
    // Use pooled client for better performance
    client := client.GetPooledClient("http://localhost:26660/v3")
    
    // Run load test with connection reuse
    // ...
}
```

## Success Metrics

- [ ] Connection pooling reduces latency by 26.7%
- [ ] Retry logic achieves 99%+ success rate
- [ ] No memory leaks under sustained load
- [ ] Zero breaking changes to existing code
- [ ] Load tests pass with improved performance

## Files Created

1. `/pkg/api/v3/jsonrpc/transport.go` - Pooled HTTP transport
2. `/pkg/client/pool.go` - Client pool manager
3. `/pkg/client/retry.go` - Retry logic with backoff

## Files Modified

None in Phase 1 - all additions are new files to ensure compatibility

## Testing Checklist

- [ ] Unit tests for pooled transport
- [ ] Unit tests for client pool
- [ ] Unit tests for retry logic
- [ ] Integration test with existing load tests
- [ ] Performance benchmarks show improvement
- [ ] Memory profiling shows no leaks
- [ ] Concurrent access tests pass

## Next Phase

[Phase 2: Enhanced Client](phase2-enhanced-client.md) - Add options pattern and circuit breakers