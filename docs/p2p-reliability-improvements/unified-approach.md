# Unified Peer Management - Minimal Changes Approach

## Core Principle: Fix What's Broken, Don't Reinvent

The existing peer database (`/pkg/api/v3/p2p/peerdb/`) already handles most of what we need. We just need to:
1. **Fix the pruning bug** that causes unlimited growth
2. **Add simple health tracking** to existing structures
3. **Let other transports optionally use** the same database

## The Minimal Fix (2-3 Days Total)

### Step 1: Fix Peer Pruning Bug (1 Hour)

**File**: `/pkg/api/v3/p2p/peerdb/db.go`

```go
// Current BROKEN code - never prunes based on time or failures
func (p *PeerStatus) prune() bool {
    return len(p.Addresses.Load()) == 0 && len(p.Networks.Load()) == 0
}

// SIMPLE FIX - add time check
func (p *PeerStatus) prune() bool {
    // Prune if no addresses/networks
    if len(p.Addresses.Load()) == 0 && len(p.Networks.Load()) == 0 {
        return true
    }
    
    // NEW: Also prune if not seen recently
    for _, addr := range p.Addresses.Load() {
        if addr.Last.Success != nil {
            age := time.Since(*addr.Last.Success)
            if age < 7*24*time.Hour {
                return false // Keep if any address succeeded recently
            }
        }
    }
    
    return true // Prune if all addresses are stale
}
```

### Step 2: Add Auto-Pruning on Startup (30 Minutes)

**File**: `/internal/node/daemon/run.go`

```go
func run(cmd *cobra.Command, args []string) error {
    // ... existing node setup ...
    
    // NEW: Start automatic pruning
    if node.Services().P2P() != nil {
        go func() {
            ticker := time.NewTicker(1 * time.Hour)
            defer ticker.Stop()
            
            for range ticker.C {
                // Prune stale peers hourly
                if db := node.Services().P2P().PeerDB(); db != nil {
                    db.Prune()
                }
            }
        }()
        
        // Initial aggressive prune
        if db := node.Services().P2P().PeerDB(); db != nil {
            before := db.Len()
            db.Prune()
            after := db.Len()
            log.Info("Pruned stale peers", "removed", before-after, "remaining", after)
        }
    }
    
    // ... rest of startup ...
}
```

### Step 3: Simple Health Tracking (1 Day)

**File**: `/pkg/api/v3/p2p/peerdb/types.go`

```go
// Extend existing LastStatus with failure count
type LastStatus struct {
    Success *time.Time `json:"success,omitempty"`
    Attempt *time.Time `json:"attempt,omitempty"`
    Failed  uint       `json:"failed,omitempty"`
    
    // NEW: Track consecutive failures for circuit breaking
    ConsecutiveFailures uint `json:"consecutiveFailures,omitempty"`
}

// Add helper methods to existing PeerAddressStatus
func (s *PeerAddressStatus) IsHealthy() bool {
    if s.Last.ConsecutiveFailures > 10 {
        return false // Too many failures
    }
    
    if s.Last.Success != nil {
        age := time.Since(*s.Last.Success)
        if age > 24*time.Hour {
            return false // Too old
        }
    }
    
    return true
}
```

### Step 4: Update Dialer to Track Health (2 Hours)

**File**: `/pkg/api/v3/p2p/dial/tracker_persistent.go`

```go
func (t *PersistentTracker) Record(peer peer.ID, addr multiaddr.Multiaddr, success bool) {
    // ... existing code to find/create peer status ...
    
    if success {
        now := time.Now()
        addrStatus.Last.Success = &now
        addrStatus.Last.ConsecutiveFailures = 0  // Reset on success
    } else {
        now := time.Now()
        addrStatus.Last.Attempt = &now
        addrStatus.Last.Failed++
        addrStatus.Last.ConsecutiveFailures++  // Track consecutive
        
        // NEW: Skip this peer if too many failures
        if addrStatus.Last.ConsecutiveFailures > 10 {
            t.badPeers.Store(peer, true)  // Mark as bad temporarily
        }
    }
    
    // ... existing save code ...
}
```

### Step 5: Let HTTP/WebSocket Clients Use PeerDB (Optional, 1 Day)

**File**: `/pkg/api/v3/jsonrpc/client_with_failover.go` (NEW FILE)

```go
package jsonrpc

import (
    "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p/peerdb"
    "github.com/libp2p/go-libp2p/core/peer"
)

// FailoverClient wraps standard client with peer tracking
type FailoverClient struct {
    *Client
    peerDB    *peerdb.DB  // Reuse existing peer database
    endpoints []string
}

// NewFailoverClient creates client with multiple endpoints
func NewFailoverClient(endpoints []string, peerDB *peerdb.DB) *FailoverClient {
    return &FailoverClient{
        Client:    NewClient(endpoints[0]),
        peerDB:    peerDB,
        endpoints: endpoints,
    }
}

// Override sendRequest to add failover
func (c *FailoverClient) sendRequest(ctx context.Context, method string, req, resp interface{}) error {
    // Try endpoints in order of health
    for _, endpoint := range c.getHealthyEndpoints() {
        c.Server = endpoint
        
        err := c.Client.sendRequest(ctx, method, req, resp)
        if err == nil {
            c.recordSuccess(endpoint)
            return nil
        }
        
        c.recordFailure(endpoint, err)
        
        if !isRetryable(err) {
            return err
        }
    }
    
    return errors.New("all endpoints failed")
}

func (c *FailoverClient) getHealthyEndpoints() []string {
    if c.peerDB == nil {
        return c.endpoints // No tracking, return all
    }
    
    // Sort endpoints by health using peer database
    healthy := []string{}
    unhealthy := []string{}
    
    for _, endpoint := range c.endpoints {
        // Convert endpoint to peer ID for database lookup
        peerID := peer.ID(endpoint) // Simple mapping
        
        if status := c.peerDB.Peers.Load(peerID); status != nil {
            if status.IsHealthy() {
                healthy = append(healthy, endpoint)
            } else {
                unhealthy = append(unhealthy, endpoint)
            }
        } else {
            healthy = append(healthy, endpoint) // Unknown = try it
        }
    }
    
    return append(healthy, unhealthy...) // Try healthy first
}

func (c *FailoverClient) recordSuccess(endpoint string) {
    if c.peerDB == nil {
        return
    }
    
    peerID := peer.ID(endpoint)
    status := c.peerDB.Peers.LoadOrStore(peerID, &peerdb.PeerStatus{})
    
    // Update success timestamp
    now := time.Now()
    for _, addr := range status.Addresses.Load() {
        addr.Last.Success = &now
        addr.Last.ConsecutiveFailures = 0
    }
}

func (c *FailoverClient) recordFailure(endpoint string, err error) {
    if c.peerDB == nil {
        return
    }
    
    peerID := peer.ID(endpoint)
    status := c.peerDB.Peers.LoadOrStore(peerID, &peerdb.PeerStatus{})
    
    // Update failure count
    for _, addr := range status.Addresses.Load() {
        addr.Last.Failed++
        addr.Last.ConsecutiveFailures++
    }
}
```

## That's It! 

### What We Changed (Minimal Violence)

1. **Fixed one function** (`prune()`) to actually remove stale peers
2. **Added one goroutine** to run pruning periodically  
3. **Extended existing struct** with `ConsecutiveFailures` field
4. **Created optional wrapper** for HTTP clients that want failover

### What We DIDN'T Change

- ✅ All existing APIs remain exactly the same
- ✅ P2P transport code unchanged except for health tracking
- ✅ Peer database format compatible (just adds fields)
- ✅ HTTP/WebSocket clients work exactly as before
- ✅ No new dependencies or major refactoring

## Testing the Fix

```bash
# Before fix - watch peer count grow
curl http://localhost:26660/metrics | grep peer_count
# peer_count 523

# Apply fix and restart
./accumulated run

# After fix - peer count should drop and stabilize
curl http://localhost:26660/metrics | grep peer_count  
# peer_count 47
```

## Rollout Plan

1. **Test locally** (30 minutes)
   - Apply pruning fix
   - Verify peer count drops
   - Check connections still work

2. **Deploy to one validator** (1 day)
   - Monitor peer database size
   - Verify no connection issues
   - Check performance metrics

3. **Roll out to network** (1 week)
   - Gradual deployment
   - Monitor network health
   - Revert if any issues

## Why This Works

- **Fixes root cause**: Stale peers can't accumulate if we prune them
- **Minimal changes**: ~50 lines of code total
- **Backward compatible**: Everything still works exactly as before
- **Optional improvements**: HTTP/WebSocket can opt-in to failover
- **Battle-tested**: Reuses existing peer database that already works

## Summary

Instead of redesigning everything, we:
1. Fix the bug that causes peer accumulation (1 line change)
2. Add automatic cleanup (10 lines) 
3. Track consecutive failures (5 lines)
4. Optionally let other transports use the same tracking (new file, optional)

Total effort: **2-3 days** including testing.
Risk: **Very low** - mostly adding to existing code, not changing it.