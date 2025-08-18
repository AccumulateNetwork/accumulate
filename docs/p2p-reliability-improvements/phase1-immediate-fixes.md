# Phase 1: Immediate P2P Fixes

## Timeline: 1-2 Days

## Objective
Implement critical fixes to stop peer database growth and connection storms without changing APIs.

## 1. Time-Based Peer Pruning

### Current Problem
```go
// pkg/api/v3/p2p/peerdb/db.go - Current inadequate pruning
func (p *PeerStatus) prune() bool {
    return len(p.Addresses.Load()) == 0 && len(p.Networks.Load()) == 0
}
```
Peers are NEVER removed based on age or failures.

### Fix: Add Time and Failure Based Pruning

**File**: `/pkg/api/v3/p2p/peerdb/db.go`

```go
type PeerStatus struct {
    // Add new fields
    LastSuccessfulConnection time.Time
    ConsecutiveFailures      int
    LastAttempt             time.Time
}

func (p *PeerStatus) prune() bool {
    now := time.Now()
    
    // Remove if not seen in 7 days
    if p.LastSuccessfulConnection.Before(now.Add(-7 * 24 * time.Hour)) {
        return true
    }
    
    // Remove if failed 10+ times
    if p.ConsecutiveFailures >= 10 {
        return true
    }
    
    // Remove if no addresses and no networks
    if len(p.Addresses.Load()) == 0 && len(p.Networks.Load()) == 0 {
        return true
    }
    
    return false
}

// Add periodic pruning goroutine
func (db *DB) StartPruning(interval time.Duration) {
    go func() {
        ticker := time.NewTicker(interval)
        defer ticker.Stop()
        
        for range ticker.C {
            db.PruneStalePeers()
        }
    }()
}

func (db *DB) PruneStalePeers() {
    var toRemove []peer.ID
    
    db.Peers.Range(func(id peer.ID, status *PeerStatus) bool {
        if status.prune() {
            toRemove = append(toRemove, id)
        }
        return true
    })
    
    for _, id := range toRemove {
        db.Peers.Delete(id)
        log.Debug("Pruned stale peer", "peer", id, "total_pruned", len(toRemove))
    }
}
```

## 2. Fix Bootstrap Server Configuration

### Current Problem
Bootstrap servers advertise wrong network peers (TestNet mixed with MainNet).

### Fix: Network-Specific Bootstrap

**File**: `/pkg/api/v3/p2p/peer_manager.go`

```go
func (m *PeerManager) Bootstrap(ctx context.Context) error {
    // Filter bootstrap peers by network
    bootstrapPeers := m.filterByNetwork(m.opts.BootstrapPeers)
    
    for _, peer := range bootstrapPeers {
        // Validate peer is on correct network before adding
        if err := m.validatePeerNetwork(ctx, peer); err != nil {
            log.Warn("Bootstrap peer on wrong network", "peer", peer, "error", err)
            continue
        }
        
        if err := m.host.Connect(ctx, peer); err != nil {
            log.Warn("Failed to connect to bootstrap peer", "peer", peer, "error", err)
            continue
        }
    }
    
    return nil
}

func (m *PeerManager) validatePeerNetwork(ctx context.Context, peer peer.AddrInfo) error {
    // Query peer for network info
    stream, err := m.host.NewStream(ctx, peer.ID, "/acc/network/1.0.0")
    if err != nil {
        return err
    }
    defer stream.Close()
    
    // Read network response
    var network string
    if err := json.NewDecoder(stream).Decode(&network); err != nil {
        return err
    }
    
    if network != m.network {
        return fmt.Errorf("peer on wrong network: expected %s, got %s", m.network, network)
    }
    
    return nil
}
```

## 3. Connection Timeout and Retry Improvements

### Current Problem
Connections hang indefinitely or fail without retry.

### Fix: Add Timeouts and Smart Retry

**File**: `/pkg/api/v3/p2p/dial/dialer.go`

```go
const (
    defaultDialTimeout = 10 * time.Second
    maxRetries        = 3
    retryBackoff      = 2
)

func (d *Dialer) dial(ctx context.Context, peer peer.ID, service *api.ServiceAddress) (message.Stream, error) {
    // Add timeout to context
    ctx, cancel := context.WithTimeout(ctx, defaultDialTimeout)
    defer cancel()
    
    var lastErr error
    backoff := 100 * time.Millisecond
    
    for attempt := 0; attempt < maxRetries; attempt++ {
        if attempt > 0 {
            select {
            case <-time.After(backoff):
                backoff *= retryBackoff
            case <-ctx.Done():
                return nil, ctx.Err()
            }
        }
        
        stream, err := d.attemptDial(ctx, peer, service)
        if err == nil {
            // Update success metrics
            if tracker, ok := d.tracker.(*PersistentTracker); ok {
                tracker.RecordSuccess(peer)
            }
            return stream, nil
        }
        
        lastErr = err
        
        // Don't retry on permanent errors
        if !isRetryableError(err) {
            break
        }
    }
    
    // Record failure
    if tracker, ok := d.tracker.(*PersistentTracker); ok {
        tracker.RecordFailure(peer, lastErr)
    }
    
    return nil, fmt.Errorf("dial failed after %d attempts: %w", maxRetries, lastErr)
}

func isRetryableError(err error) bool {
    // Classify errors
    switch {
    case errors.Is(err, context.DeadlineExceeded):
        return true
    case errors.Is(err, io.EOF):
        return true
    case strings.Contains(err.Error(), "connection refused"):
        return true
    case strings.Contains(err.Error(), "no route to host"):
        return false // Don't retry unreachable hosts
    default:
        return true
    }
}
```

## 4. Enhanced Peer Tracking

**File**: `/pkg/api/v3/p2p/dial/tracker_persistent.go`

```go
type PersistentTracker struct {
    db *peerdb.DB
    mu sync.RWMutex
}

func (t *PersistentTracker) RecordSuccess(peer peer.ID) {
    t.mu.Lock()
    defer t.mu.Unlock()
    
    status := t.db.Peers.LoadOrStore(peer, &peerdb.PeerStatus{})
    status.LastSuccessfulConnection = time.Now()
    status.ConsecutiveFailures = 0
}

func (t *PersistentTracker) RecordFailure(peer peer.ID, err error) {
    t.mu.Lock()
    defer t.mu.Unlock()
    
    status := t.db.Peers.LoadOrStore(peer, &peerdb.PeerStatus{})
    status.ConsecutiveFailures++
    status.LastAttempt = time.Now()
    
    // Classify error
    if !isRetryableError(err) {
        status.ConsecutiveFailures += 5 // Penalize permanent errors more
    }
}

func (t *PersistentTracker) GetHealthyPeers(limit int) []peer.ID {
    t.mu.RLock()
    defer t.mu.RUnlock()
    
    type peerScore struct {
        id    peer.ID
        score float64
    }
    
    var peers []peerScore
    now := time.Now()
    
    t.db.Peers.Range(func(id peer.ID, status *peerdb.PeerStatus) bool {
        // Skip unhealthy peers
        if status.ConsecutiveFailures >= 5 {
            return true
        }
        
        // Calculate health score
        age := now.Sub(status.LastSuccessfulConnection).Hours()
        score := 100.0 / (1.0 + age + float64(status.ConsecutiveFailures)*10)
        
        peers = append(peers, peerScore{id: id, score: score})
        return true
    })
    
    // Sort by score
    sort.Slice(peers, func(i, j int) bool {
        return peers[i].score > peers[j].score
    })
    
    // Return top peers
    result := make([]peer.ID, 0, limit)
    for i := 0; i < limit && i < len(peers); i++ {
        result = append(result, peers[i].id)
    }
    
    return result
}
```

## 5. Startup Configuration

**File**: `/internal/node/daemon/run.go`

```go
func run(cmd *cobra.Command, args []string) error {
    // ... existing code ...
    
    // Start peer pruning on node startup
    if node.P2P != nil {
        if db, ok := node.P2P.PeerDB(); ok {
            db.StartPruning(1 * time.Hour) // Prune every hour
            
            // Initial aggressive prune on startup
            db.PruneStalePeers()
            log.Info("Initial peer pruning completed", 
                "remaining_peers", db.Peers.Len())
        }
    }
    
    // ... rest of startup
}
```

## Testing Plan

### Unit Tests
```go
func TestPeerPruning(t *testing.T) {
    db := peerdb.New()
    
    // Add old peer
    oldPeer := peer.ID("old")
    status := &peerdb.PeerStatus{
        LastSuccessfulConnection: time.Now().Add(-8 * 24 * time.Hour),
    }
    db.Peers.Store(oldPeer, status)
    
    // Add failed peer
    failedPeer := peer.ID("failed")
    status2 := &peerdb.PeerStatus{
        ConsecutiveFailures: 11,
    }
    db.Peers.Store(failedPeer, status2)
    
    // Prune
    db.PruneStalePeers()
    
    // Verify removed
    assert.Nil(t, db.Peers.Load(oldPeer))
    assert.Nil(t, db.Peers.Load(failedPeer))
}
```

### Integration Test
```bash
# Test with network partition
./test/scripts/partition_test.sh

# Verify peer count stays bounded
curl http://localhost:26660/metrics | grep peer_count
```

## Deployment

1. **Build with fixes**:
```bash
git checkout -b p2p-phase1-fixes
# Apply changes
go build ./cmd/accumulated
```

2. **Test on single node**:
```bash
./accumulated run --test-mode
# Monitor peer database size
```

3. **Deploy to validators**:
```bash
# Rolling update to avoid network disruption
./scripts/deploy/rolling_update.sh
```

## Success Metrics

After Phase 1 deployment:
- [ ] Peer database size reduced by 80%
- [ ] No more connection storms (< 10 failed attempts/minute)
- [ ] Bootstrap connects to correct network peers only
- [ ] Connection success rate > 70%
- [ ] No unbounded peer database growth

## Next Phase

[Phase 2: Enhanced Tracking](phase2-enhanced-tracking.md) - Add connection pooling and circuit breakers