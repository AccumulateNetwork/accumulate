# Phase 2: Enhanced P2P Tracking

## Timeline: 3-5 Days

## Objective
Implement connection pooling, peer scoring, and circuit breakers to dramatically improve P2P reliability.

## 1. Connection Pooling for P2P Streams

### Problem
Every P2P request creates a new connection, causing overhead and exhaustion.

### Solution: P2P Connection Pool

**File**: `/pkg/api/v3/p2p/connection_pool.go` (new file)

```go
package p2p

import (
    "context"
    "sync"
    "time"
    
    "github.com/libp2p/go-libp2p/core/network"
    "github.com/libp2p/go-libp2p/core/peer"
    "github.com/libp2p/go-libp2p/core/protocol"
)

type ConnectionPool struct {
    host       host.Host
    mu         sync.RWMutex
    conns      map[string]*pooledConn
    maxIdle    time.Duration
    maxPerPeer int
}

type pooledConn struct {
    stream    network.Stream
    peer      peer.ID
    protocol  protocol.ID
    lastUsed  time.Time
    inUse     bool
    useCount  int64
}

func NewConnectionPool(host host.Host) *ConnectionPool {
    pool := &ConnectionPool{
        host:       host,
        conns:      make(map[string]*pooledConn),
        maxIdle:    30 * time.Second,
        maxPerPeer: 5,
    }
    
    go pool.cleanupLoop()
    return pool
}

func (p *ConnectionPool) Get(ctx context.Context, peerID peer.ID, proto protocol.ID) (network.Stream, error) {
    key := fmt.Sprintf("%s:%s", peerID, proto)
    
    p.mu.Lock()
    defer p.mu.Unlock()
    
    // Check for existing connection
    if conn, exists := p.conns[key]; exists && !conn.inUse {
        // Test if connection is still alive
        if p.isAlive(conn.stream) {
            conn.inUse = true
            conn.lastUsed = time.Now()
            conn.useCount++
            return &pooledStream{conn, p}, nil
        }
        // Dead connection, remove it
        conn.stream.Close()
        delete(p.conns, key)
    }
    
    // Create new connection
    stream, err := p.host.NewStream(ctx, peerID, proto)
    if err != nil {
        return nil, err
    }
    
    conn := &pooledConn{
        stream:   stream,
        peer:     peerID,
        protocol: proto,
        lastUsed: time.Now(),
        inUse:    true,
        useCount: 1,
    }
    
    p.conns[key] = conn
    return &pooledStream{conn, p}, nil
}

func (p *ConnectionPool) isAlive(stream network.Stream) bool {
    // Send ping to test connection
    deadline := time.Now().Add(1 * time.Second)
    stream.SetDeadline(deadline)
    defer stream.SetDeadline(time.Time{})
    
    _, err := stream.Write([]byte{0}) // Ping byte
    return err == nil
}

func (p *ConnectionPool) release(conn *pooledConn) {
    p.mu.Lock()
    defer p.mu.Unlock()
    
    conn.inUse = false
    conn.lastUsed = time.Now()
}

func (p *ConnectionPool) cleanupLoop() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        p.cleanup()
    }
}

func (p *ConnectionPool) cleanup() {
    p.mu.Lock()
    defer p.mu.Unlock()
    
    now := time.Now()
    for key, conn := range p.conns {
        if !conn.inUse && now.Sub(conn.lastUsed) > p.maxIdle {
            conn.stream.Close()
            delete(p.conns, key)
        }
    }
}

// Pooled stream wrapper
type pooledStream struct {
    *pooledConn
    pool *ConnectionPool
}

func (s *pooledStream) Close() error {
    s.pool.release(s.pooledConn)
    return nil // Don't actually close the underlying stream
}
```

## 2. Peer Scoring System

**File**: `/pkg/api/v3/p2p/peer_scorer.go` (new file)

```go
package p2p

type PeerScorer struct {
    mu     sync.RWMutex
    scores map[peer.ID]*PeerScore
}

type PeerScore struct {
    PeerID              peer.ID
    SuccessfulRequests  int64
    FailedRequests      int64
    TotalLatency        time.Duration
    LastUpdateTime      time.Time
    ConsecutiveFailures int
    
    // Computed fields
    SuccessRate float64
    AvgLatency  time.Duration
    Score       float64
}

func NewPeerScorer() *PeerScorer {
    return &PeerScorer{
        scores: make(map[peer.ID]*PeerScore),
    }
}

func (s *PeerScorer) RecordSuccess(peerID peer.ID, latency time.Duration) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    score, exists := s.scores[peerID]
    if !exists {
        score = &PeerScore{PeerID: peerID}
        s.scores[peerID] = score
    }
    
    score.SuccessfulRequests++
    score.TotalLatency += latency
    score.ConsecutiveFailures = 0
    score.LastUpdateTime = time.Now()
    
    s.updateScore(score)
}

func (s *PeerScorer) RecordFailure(peerID peer.ID, err error) {
    s.mu.Lock()
    defer s.mu.Unlock()
    
    score, exists := s.scores[peerID]
    if !exists {
        score = &PeerScore{PeerID: peerID}
        s.scores[peerID] = score
    }
    
    score.FailedRequests++
    score.ConsecutiveFailures++
    score.LastUpdateTime = time.Now()
    
    s.updateScore(score)
}

func (s *PeerScorer) updateScore(score *PeerScore) {
    total := score.SuccessfulRequests + score.FailedRequests
    if total == 0 {
        score.Score = 0
        return
    }
    
    // Calculate success rate
    score.SuccessRate = float64(score.SuccessfulRequests) / float64(total)
    
    // Calculate average latency
    if score.SuccessfulRequests > 0 {
        score.AvgLatency = score.TotalLatency / time.Duration(score.SuccessfulRequests)
    }
    
    // Calculate composite score (0-100)
    // 70% weight on success rate, 20% on latency, 10% on recency
    successScore := score.SuccessRate * 70
    
    // Latency score (lower is better, cap at 1 second)
    latencyMs := float64(score.AvgLatency.Milliseconds())
    latencyScore := math.Max(0, 20*(1-latencyMs/1000))
    
    // Recency score
    hoursSinceUpdate := time.Since(score.LastUpdateTime).Hours()
    recencyScore := math.Max(0, 10*(1-hoursSinceUpdate/24))
    
    // Penalty for consecutive failures
    penalty := float64(score.ConsecutiveFailures) * 10
    
    score.Score = math.Max(0, successScore+latencyScore+recencyScore-penalty)
}

func (s *PeerScorer) GetTopPeers(n int) []peer.ID {
    s.mu.RLock()
    defer s.mu.RUnlock()
    
    // Convert to slice for sorting
    peers := make([]*PeerScore, 0, len(s.scores))
    for _, score := range s.scores {
        peers = append(peers, score)
    }
    
    // Sort by score
    sort.Slice(peers, func(i, j int) bool {
        return peers[i].Score > peers[j].Score
    })
    
    // Return top N peer IDs
    result := make([]peer.ID, 0, n)
    for i := 0; i < n && i < len(peers); i++ {
        if peers[i].Score > 10 { // Minimum score threshold
            result = append(result, peers[i].PeerID)
        }
    }
    
    return result
}
```

## 3. Circuit Breaker Implementation

**File**: `/pkg/api/v3/p2p/circuit_breaker.go` (new file)

```go
package p2p

type CircuitBreaker struct {
    mu         sync.RWMutex
    states     map[peer.ID]*BreakerState
    threshold  float64
    timeout    time.Duration
    halfOpenMax int
}

type BreakerState struct {
    State           CircuitState
    Failures        int64
    Successes       int64
    LastFailureTime time.Time
    HalfOpenTests   int
}

type CircuitState int

const (
    CircuitClosed CircuitState = iota
    CircuitOpen
    CircuitHalfOpen
)

func NewCircuitBreaker() *CircuitBreaker {
    return &CircuitBreaker{
        states:      make(map[peer.ID]*BreakerState),
        threshold:   0.5,  // Open circuit if >50% failures
        timeout:     30 * time.Second,
        halfOpenMax: 3,    // Max attempts in half-open state
    }
}

func (cb *CircuitBreaker) Allow(peerID peer.ID) bool {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    
    state, exists := cb.states[peerID]
    if !exists {
        state = &BreakerState{State: CircuitClosed}
        cb.states[peerID] = state
    }
    
    switch state.State {
    case CircuitClosed:
        return true
        
    case CircuitOpen:
        // Check if timeout has passed
        if time.Since(state.LastFailureTime) > cb.timeout {
            state.State = CircuitHalfOpen
            state.HalfOpenTests = 0
            return true
        }
        return false
        
    case CircuitHalfOpen:
        // Allow limited tests in half-open state
        if state.HalfOpenTests < cb.halfOpenMax {
            state.HalfOpenTests++
            return true
        }
        return false
    }
    
    return false
}

func (cb *CircuitBreaker) RecordSuccess(peerID peer.ID) {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    
    state, exists := cb.states[peerID]
    if !exists {
        return
    }
    
    state.Successes++
    
    if state.State == CircuitHalfOpen {
        // Successful test in half-open, close the circuit
        state.State = CircuitClosed
        state.Failures = 0
        state.Successes = 0
    }
}

func (cb *CircuitBreaker) RecordFailure(peerID peer.ID) {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    
    state, exists := cb.states[peerID]
    if !exists {
        state = &BreakerState{State: CircuitClosed}
        cb.states[peerID] = state
    }
    
    state.Failures++
    state.LastFailureTime = time.Now()
    
    total := state.Failures + state.Successes
    if total > 10 { // Minimum sample size
        failureRate := float64(state.Failures) / float64(total)
        
        if failureRate > cb.threshold {
            state.State = CircuitOpen
        }
    }
    
    if state.State == CircuitHalfOpen {
        // Failed test in half-open, reopen the circuit
        state.State = CircuitOpen
    }
}
```

## 4. Integration with Dialer

**File**: `/pkg/api/v3/p2p/dial/dialer.go` (modifications)

```go
type Dialer struct {
    // ... existing fields ...
    
    // New fields
    connPool       *ConnectionPool
    scorer         *PeerScorer
    circuitBreaker *CircuitBreaker
}

func NewDialer(node *p2p.Node, opts ...Option) *Dialer {
    d := &Dialer{
        // ... existing initialization ...
        
        // Initialize new components
        connPool:       NewConnectionPool(node.Host()),
        scorer:         NewPeerScorer(),
        circuitBreaker: NewCircuitBreaker(),
    }
    
    return d
}

func (d *Dialer) Dial(ctx context.Context, addr multiaddr.Multiaddr) (message.Stream, error) {
    // Get prioritized peer list
    peers := d.getPrioritizedPeers(addr)
    
    for _, peerID := range peers {
        // Check circuit breaker
        if !d.circuitBreaker.Allow(peerID) {
            continue
        }
        
        start := time.Now()
        
        // Try to get pooled connection first
        stream, err := d.connPool.Get(ctx, peerID, protocolID)
        
        if err == nil {
            // Record success
            latency := time.Since(start)
            d.scorer.RecordSuccess(peerID, latency)
            d.circuitBreaker.RecordSuccess(peerID)
            return stream, nil
        }
        
        // Record failure
        d.scorer.RecordFailure(peerID, err)
        d.circuitBreaker.RecordFailure(peerID)
    }
    
    return nil, errors.NoPeer.WithFormat("no available peers for %v", addr)
}

func (d *Dialer) getPrioritizedPeers(addr multiaddr.Multiaddr) []peer.ID {
    // Get top peers from scorer
    topPeers := d.scorer.GetTopPeers(10)
    
    // If not enough scored peers, add from discovery
    if len(topPeers) < 5 {
        discovered := d.discover(addr)
        for _, p := range discovered {
            if !contains(topPeers, p) {
                topPeers = append(topPeers, p)
            }
        }
    }
    
    return topPeers
}
```

## 5. Error Classification Enhancement

**File**: `/pkg/api/v3/p2p/errors.go` (new file)

```go
package p2p

type ErrorClass int

const (
    ErrorTransient ErrorClass = iota
    ErrorPermanent
    ErrorTimeout
    ErrorProtocol
)

func ClassifyError(err error) ErrorClass {
    if err == nil {
        return ErrorTransient
    }
    
    errStr := err.Error()
    
    // Timeout errors
    if errors.Is(err, context.DeadlineExceeded) ||
       strings.Contains(errStr, "timeout") {
        return ErrorTimeout
    }
    
    // Permanent errors
    if strings.Contains(errStr, "no route to host") ||
       strings.Contains(errStr, "protocol not supported") ||
       strings.Contains(errStr, "incompatible protocol version") {
        return ErrorPermanent
    }
    
    // Protocol errors
    if strings.Contains(errStr, "protocol error") ||
       strings.Contains(errStr, "bad request") {
        return ErrorProtocol
    }
    
    // Default to transient
    return ErrorTransient
}

func ShouldRetry(err error) bool {
    class := ClassifyError(err)
    return class == ErrorTransient || class == ErrorTimeout
}
```

## Testing

### Load Test
```go
func TestConnectionPoolUnderLoad(t *testing.T) {
    pool := NewConnectionPool(mockHost)
    
    var wg sync.WaitGroup
    for i := 0; i < 100; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            
            for j := 0; j < 10; j++ {
                stream, err := pool.Get(ctx, testPeer, testProto)
                require.NoError(t, err)
                
                // Simulate work
                time.Sleep(10 * time.Millisecond)
                
                stream.Close() // Returns to pool
            }
        }()
    }
    
    wg.Wait()
    
    // Verify connection reuse
    assert.Less(t, pool.TotalConnections(), 20)
}
```

## Deployment

1. **Update dependencies**
2. **Build and test locally**
3. **Deploy to testnet first**
4. **Monitor metrics**
5. **Roll out to mainnet**

## Success Metrics

- [ ] Connection reuse rate > 80%
- [ ] P2P latency reduced by 50%
- [ ] Circuit breaker prevents cascade failures
- [ ] Peer scoring improves connection success to > 90%

## Next Phase

[Phase 3: Centralized Coordination](phase3-centralized-coordination.md)