# Phase 3: Smart Routing

## Objective
Implement health monitoring, latency-aware routing, and automatic failover to improve reliability and performance.

## Timeline
Week 3 (5 days)

## Dependencies
- Phase 1 & 2 complete (connection pooling, retry logic, options pattern)

## Components

### 1. Smart Router (`/pkg/api/v3/message/smart_router.go`)

Health-aware routing implementation:

```go
package message

import (
    "context"
    "sync"
    "time"
    "github.com/multiformats/go-multiaddr"
)

type SmartRouter struct {
    base   Router
    mu     sync.RWMutex
    health map[string]*EndpointHealth
}

type EndpointHealth struct {
    Address       multiaddr.Multiaddr
    Latency       time.Duration
    SuccessCount  int64
    FailureCount  int64
    LastCheck     time.Time
    CircuitOpen   bool
    CircuitOpenAt time.Time
}

func NewSmartRouter(base Router) *SmartRouter {
    r := &SmartRouter{
        base:   base,
        health: make(map[string]*EndpointHealth),
    }
    go r.monitorHealth()
    return r
}

func (r *SmartRouter) Route(msg Message) (multiaddr.Multiaddr, error) {
    // Get base routing
    addr, err := r.base.Route(msg)
    if err != nil {
        return nil, err
    }
    
    // Select best endpoint based on health
    return r.selectBestEndpoint(addr), nil
}

func (r *SmartRouter) selectBestEndpoint(primary multiaddr.Multiaddr) multiaddr.Multiaddr {
    r.mu.RLock()
    defer r.mu.RUnlock()
    
    health := r.health[primary.String()]
    if health == nil || !health.CircuitOpen {
        return primary
    }
    
    // Find alternative with best latency
    var best multiaddr.Multiaddr
    var bestLatency time.Duration = time.Hour
    
    for _, h := range r.health {
        if !h.CircuitOpen && h.Latency < bestLatency {
            best = h.Address
            bestLatency = h.Latency
        }
    }
    
    if best != nil {
        return best
    }
    return primary
}
```

### 2. Health Monitor (`/pkg/api/v3/message/health_monitor.go`)

Active health checking:

```go
package message

type HealthMonitor struct {
    router   *SmartRouter
    interval time.Duration
    timeout  time.Duration
}

func NewHealthMonitor(router *SmartRouter) *HealthMonitor {
    return &HealthMonitor{
        router:   router,
        interval: 30 * time.Second,
        timeout:  5 * time.Second,
    }
}

func (m *HealthMonitor) Start(ctx context.Context) {
    ticker := time.NewTicker(m.interval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            m.checkAllEndpoints()
        }
    }
}

func (m *HealthMonitor) checkAllEndpoints() {
    m.router.mu.RLock()
    endpoints := make([]*EndpointHealth, 0, len(m.router.health))
    for _, h := range m.router.health {
        endpoints = append(endpoints, h)
    }
    m.router.mu.RUnlock()
    
    for _, endpoint := range endpoints {
        m.checkEndpoint(endpoint)
    }
}

func (m *HealthMonitor) checkEndpoint(endpoint *EndpointHealth) {
    ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
    defer cancel()
    
    start := time.Now()
    err := m.ping(ctx, endpoint.Address)
    latency := time.Since(start)
    
    m.router.mu.Lock()
    defer m.router.mu.Unlock()
    
    endpoint.LastCheck = time.Now()
    endpoint.Latency = latency
    
    if err != nil {
        endpoint.FailureCount++
        if endpoint.FailureCount > 3 {
            endpoint.CircuitOpen = true
            endpoint.CircuitOpenAt = time.Now()
        }
    } else {
        endpoint.SuccessCount++
        if endpoint.CircuitOpen && time.Since(endpoint.CircuitOpenAt) > 30*time.Second {
            endpoint.CircuitOpen = false
            endpoint.FailureCount = 0
        }
    }
}
```

### 3. Load Balancer (`/pkg/api/v3/message/load_balancer.go`)

Distribute load across healthy endpoints:

```go
package message

type LoadBalancer struct {
    router    *SmartRouter
    strategy  LoadBalancingStrategy
    mu        sync.RWMutex
    roundRobin int
}

type LoadBalancingStrategy int

const (
    RoundRobin LoadBalancingStrategy = iota
    LeastLatency
    WeightedRandom
)

func NewLoadBalancer(router *SmartRouter, strategy LoadBalancingStrategy) *LoadBalancer {
    return &LoadBalancer{
        router:   router,
        strategy: strategy,
    }
}

func (lb *LoadBalancer) SelectEndpoint() multiaddr.Multiaddr {
    switch lb.strategy {
    case RoundRobin:
        return lb.roundRobinSelect()
    case LeastLatency:
        return lb.leastLatencySelect()
    case WeightedRandom:
        return lb.weightedRandomSelect()
    default:
        return lb.roundRobinSelect()
    }
}

func (lb *LoadBalancer) roundRobinSelect() multiaddr.Multiaddr {
    lb.mu.Lock()
    defer lb.mu.Unlock()
    
    healthy := lb.getHealthyEndpoints()
    if len(healthy) == 0 {
        return nil
    }
    
    selected := healthy[lb.roundRobin%len(healthy)]
    lb.roundRobin++
    return selected.Address
}

func (lb *LoadBalancer) leastLatencySelect() multiaddr.Multiaddr {
    healthy := lb.getHealthyEndpoints()
    if len(healthy) == 0 {
        return nil
    }
    
    best := healthy[0]
    for _, h := range healthy[1:] {
        if h.Latency < best.Latency {
            best = h
        }
    }
    return best.Address
}
```

## Implementation Steps

### Day 1: Smart Router
1. Create `/pkg/api/v3/message/smart_router.go`
2. Implement health-aware routing
3. Add endpoint health tracking
4. Unit test routing decisions

### Day 2: Health Monitoring
1. Create `/pkg/api/v3/message/health_monitor.go`
2. Implement active health checks
3. Add ping functionality
4. Test health state transitions

### Day 3: Load Balancing
1. Create `/pkg/api/v3/message/load_balancer.go`
2. Implement multiple strategies
3. Add weighted selection
4. Test load distribution

### Day 4: Integration
1. Wire smart router into message transport
2. Start health monitor in background
3. Configure load balancing strategy
4. Test failover scenarios

### Day 5: Testing & Metrics
1. Test automatic failover
2. Verify latency-based routing
3. Load test with multiple endpoints
4. Add metrics collection

## Usage Examples

### Enable Smart Routing
```go
// Create smart router
baseRouter := message.NewStaticRouter(endpoints)
smartRouter := message.NewSmartRouter(baseRouter)

// Use in transport
transport := &message.RoutedTransport{
    Router: smartRouter,
    Dialer: dialer,
}
```

### With Load Balancing
```go
// Create load balancer
lb := message.NewLoadBalancer(smartRouter, message.LeastLatency)

// Use for endpoint selection
endpoint := lb.SelectEndpoint()
```

### Health Monitoring
```go
// Start health monitor
monitor := message.NewHealthMonitor(smartRouter)
go monitor.Start(ctx)
```

## Success Metrics

- [ ] Automatic failover within 5 seconds
- [ ] Latency-based routing reduces average response time by 15%
- [ ] Load distributed evenly across healthy endpoints
- [ ] Circuit breaker prevents cascading failures
- [ ] No requests lost during endpoint failures

## Files Created

1. `/pkg/api/v3/message/smart_router.go` - Health-aware routing
2. `/pkg/api/v3/message/health_monitor.go` - Active health checking
3. `/pkg/api/v3/message/load_balancer.go` - Load distribution

## Files Modified

1. `/pkg/api/v3/message/transport.go` - Integration point for smart router

## Testing Checklist

- [ ] Unit tests for smart routing logic
- [ ] Health monitor state management
- [ ] Load balancer distribution patterns
- [ ] Failover scenario testing
- [ ] Latency tracking accuracy
- [ ] Integration with existing transport

## Next Phase

[Phase 4: Message Transport](phase4-message-transport.md) - Optimize stream pooling and connection lifecycle