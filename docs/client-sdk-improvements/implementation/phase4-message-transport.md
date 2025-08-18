# Phase 4: Message Transport Optimization

## Objective
Optimize stream pooling, connection lifecycle management, and integrate all improvements for maximum performance.

## Timeline
Week 4 (5 days)

## Dependencies
- Phases 1-3 complete (pooling, client enhancements, smart routing)

## Components

### 1. Pooled Dialer (`/pkg/api/v3/message/pooled_dialer.go`)

Stream connection pooling:

```go
package message

import (
    "context"
    "sync"
    "time"
    "github.com/multiformats/go-multiaddr"
)

type PooledDialer struct {
    base     Dialer
    mu       sync.RWMutex
    pool     map[string]*pooledStream
    maxIdle  time.Duration
    maxConns int
}

type pooledStream struct {
    Stream
    addr     multiaddr.Multiaddr
    lastUsed time.Time
    useCount int64
    inUse    bool
}

func NewPooledDialer(base Dialer) *PooledDialer {
    d := &PooledDialer{
        base:     base,
        pool:     make(map[string]*pooledStream),
        maxIdle:  30 * time.Second,
        maxConns: 100,
    }
    go d.cleanupIdleConnections()
    return d
}

func (d *PooledDialer) Dial(ctx context.Context, addr multiaddr.Multiaddr) (Stream, error) {
    key := addr.String()
    
    // Try to get pooled connection
    d.mu.RLock()
    if ps, exists := d.pool[key]; exists && !ps.inUse {
        ps.inUse = true
        ps.lastUsed = time.Now()
        ps.useCount++
        d.mu.RUnlock()
        return &pooledStreamWrapper{ps, d, key}, nil
    }
    d.mu.RUnlock()
    
    // Create new connection
    stream, err := d.base.Dial(ctx, addr)
    if err != nil {
        return nil, err
    }
    
    // Add to pool
    d.mu.Lock()
    ps := &pooledStream{
        Stream:   stream,
        addr:     addr,
        lastUsed: time.Now(),
        useCount: 1,
        inUse:    true,
    }
    d.pool[key] = ps
    d.mu.Unlock()
    
    return &pooledStreamWrapper{ps, d, key}, nil
}

func (d *PooledDialer) cleanupIdleConnections() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        d.mu.Lock()
        now := time.Now()
        for key, ps := range d.pool {
            if !ps.inUse && now.Sub(ps.lastUsed) > d.maxIdle {
                ps.Stream.Close()
                delete(d.pool, key)
            }
        }
        d.mu.Unlock()
    }
}
```

### 2. Stream Manager (`/pkg/api/v3/message/stream_manager.go`)

Lifecycle management for streams:

```go
package message

type StreamManager struct {
    dialer    *PooledDialer
    mu        sync.RWMutex
    streams   map[string]*ManagedStream
    metrics   *StreamMetrics
}

type ManagedStream struct {
    Stream
    id           string
    created      time.Time
    lastActivity time.Time
    messageCount int64
    errorCount   int64
}

type StreamMetrics struct {
    ActiveStreams   int64
    TotalMessages   int64
    TotalErrors     int64
    AverageLifetime time.Duration
}

func NewStreamManager(dialer *PooledDialer) *StreamManager {
    return &StreamManager{
        dialer:  dialer,
        streams: make(map[string]*ManagedStream),
        metrics: &StreamMetrics{},
    }
}

func (m *StreamManager) GetStream(ctx context.Context, addr multiaddr.Multiaddr) (*ManagedStream, error) {
    key := addr.String()
    
    m.mu.RLock()
    if ms, exists := m.streams[key]; exists {
        ms.lastActivity = time.Now()
        m.mu.RUnlock()
        return ms, nil
    }
    m.mu.RUnlock()
    
    // Create new managed stream
    stream, err := m.dialer.Dial(ctx, addr)
    if err != nil {
        return nil, err
    }
    
    ms := &ManagedStream{
        Stream:       stream,
        id:          key,
        created:     time.Now(),
        lastActivity: time.Now(),
    }
    
    m.mu.Lock()
    m.streams[key] = ms
    m.metrics.ActiveStreams++
    m.mu.Unlock()
    
    return ms, nil
}

func (m *StreamManager) ReleaseStream(ms *ManagedStream) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    ms.lastActivity = time.Now()
    lifetime := ms.lastActivity.Sub(ms.created)
    
    // Update metrics
    m.metrics.TotalMessages += ms.messageCount
    m.metrics.TotalErrors += ms.errorCount
    m.metrics.AverageLifetime = (m.metrics.AverageLifetime + lifetime) / 2
}
```

### 3. Optimized Transport (`/pkg/api/v3/message/optimized_transport.go`)

Integration of all improvements:

```go
package message

type OptimizedTransport struct {
    *RoutedTransport
    streamManager *StreamManager
    loadBalancer  *LoadBalancer
    metrics       *TransportMetrics
}

type TransportMetrics struct {
    RequestCount     int64
    SuccessCount     int64
    FailureCount     int64
    RetryCount       int64
    AverageLatency   time.Duration
    P95Latency       time.Duration
    P99Latency       time.Duration
}

func NewOptimizedTransport(network string) *OptimizedTransport {
    // Create base components
    baseDialer := NewBaseDialer()
    pooledDialer := NewPooledDialer(baseDialer)
    streamManager := NewStreamManager(pooledDialer)
    
    // Create router with health monitoring
    baseRouter := NewStaticRouter()
    smartRouter := NewSmartRouter(baseRouter)
    loadBalancer := NewLoadBalancer(smartRouter, LeastLatency)
    
    // Start health monitoring
    monitor := NewHealthMonitor(smartRouter)
    go monitor.Start(context.Background())
    
    return &OptimizedTransport{
        RoutedTransport: &RoutedTransport{
            Network:  network,
            Dialer:   pooledDialer,
            Router:   smartRouter,
            Attempts: 3,
        },
        streamManager: streamManager,
        loadBalancer:  loadBalancer,
        metrics:       &TransportMetrics{},
    }
}

func (t *OptimizedTransport) RoundTrip(ctx context.Context, requests []Message, callback ResponseCallback) error {
    start := time.Now()
    t.metrics.RequestCount++
    
    // Use optimized routing
    err := t.RoutedTransport.RoundTrip(ctx, requests, func(res, req Message) error {
        // Track metrics
        latency := time.Since(start)
        t.updateLatencyMetrics(latency)
        
        err := callback(res, req)
        if err == nil {
            t.metrics.SuccessCount++
        } else {
            t.metrics.FailureCount++
        }
        return err
    })
    
    return err
}
```

## Implementation Steps

### Day 1: Stream Pooling
1. Create `/pkg/api/v3/message/pooled_dialer.go`
2. Implement connection pooling for streams
3. Add idle connection cleanup
4. Test stream reuse

### Day 2: Stream Management
1. Create `/pkg/api/v3/message/stream_manager.go`
2. Implement lifecycle management
3. Add metrics collection
4. Test stream tracking

### Day 3: Transport Integration
1. Create `/pkg/api/v3/message/optimized_transport.go`
2. Integrate all components
3. Add comprehensive metrics
4. Test end-to-end flow

### Day 4: Performance Tuning
1. Profile connection usage
2. Optimize pool sizes
3. Tune timeouts and intervals
4. Benchmark improvements

### Day 5: Final Testing
1. Load test with all optimizations
2. Verify 40-50% throughput improvement
3. Memory leak testing
4. Documentation and examples

## Usage Examples

### Complete Setup
```go
// Create optimized transport with all improvements
transport := message.NewOptimizedTransport("mainnet")

// Create client with optimized transport
client := &message.Client{
    Transport: transport,
}

// Use client normally - all optimizations are transparent
response, err := client.Query(ctx, account, nil)
```

### Custom Configuration
```go
// Create custom optimized transport
transport := &message.OptimizedTransport{
    RoutedTransport: &message.RoutedTransport{
        Network:  "testnet",
        Dialer:   NewPooledDialer(baseDialer),
        Router:   smartRouter,
        Attempts: 5,
        Debug:    true,
    },
    streamManager: streamManager,
    loadBalancer:  loadBalancer,
}
```

### Metrics Access
```go
// Get transport metrics
metrics := transport.GetMetrics()
fmt.Printf("Success Rate: %.2f%%\n", 
    float64(metrics.SuccessCount)/float64(metrics.RequestCount)*100)
fmt.Printf("P95 Latency: %v\n", metrics.P95Latency)
```

## Success Metrics

- [ ] 40-50% throughput improvement under load
- [ ] Stream reuse reduces connection overhead by 30%
- [ ] P95 latency improved by 25%
- [ ] Zero memory leaks after 24-hour test
- [ ] All integration tests pass

## Files Created

1. `/pkg/api/v3/message/pooled_dialer.go` - Stream pooling
2. `/pkg/api/v3/message/stream_manager.go` - Lifecycle management
3. `/pkg/api/v3/message/optimized_transport.go` - Complete integration

## Files Modified

None - all improvements are additive

## Testing Checklist

- [ ] Stream pooling and reuse
- [ ] Connection lifecycle management
- [ ] Metrics accuracy
- [ ] Load test showing 40%+ improvement
- [ ] Memory profiling
- [ ] Integration with all phases
- [ ] Examples and documentation

## Completion

This completes the 4-phase implementation plan. All improvements:
- Are backward compatible
- Require no protocol changes
- Can be adopted incrementally
- Provide significant performance benefits

See [Migration Guide](migration-guide.md) for adoption instructions.