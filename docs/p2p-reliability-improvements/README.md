# P2P Reliability Improvements

## Critical Priority: Fix Protocol-Level Communication

This directory contains the analysis and implementation plan for fixing unreliable P2P communications within the Accumulate protocol. These improvements are **critical for network stability** and affect node-to-node communications.

## 🚀 Quick Start: [Unified Minimal Fix Approach](unified-approach.md)
**Read this first!** - Simple 2-3 day fix that solves 90% of the problems with minimal code changes.

## The Problem

The current P2P implementation suffers from:
- **500+ stale peers** accumulating in peer databases
- **Connection storms** attempting to reach dead peers
- **No centralized coordination** of peer health
- **Unbounded peer database growth** without pruning
- **Poor error handling** and no circuit breakers

## The Solution: Enhanced Decentralized Peer Management

Improve the existing distributed architecture with better local peer management:
- **Local peer health tracking** on each node
- **Time-based peer pruning** (7-day expiry)
- **Failure-based removal** (10 failures = removal)
- **Connection pooling** for P2P streams
- **Circuit breaker pattern** for failing peers
- **Peer scoring** for intelligent connection prioritization

## Implementation Approach

### [Minimal Fix (RECOMMENDED)](unified-approach.md) (2-3 days total)
**Start here!** This simple approach fixes 90% of the problems:
- Fix pruning bug (1 hour)
- Add automatic cleanup (30 minutes)
- Track consecutive failures (30 minutes)
- Optional HTTP failover (1 day, if needed)

### Additional Documentation (for reference)
- [Phase 1: Immediate Fixes](phase1-immediate-fixes.md) - Detailed implementation
- [Phase 2: Enhanced Tracking](phase2-enhanced-tracking.md) - Advanced features (only if minimal fix insufficient)

## API Compatibility

All improvements maintain existing APIs:

```go
// Existing API preserved
func New(opts Options) (*Node, error)
func (n *Node) RegisterService(addr *api.ServiceAddress, handler MessageStreamHandler) bool
func (n *Node) WaitForService(ctx context.Context, addr multiaddr.Multiaddr) error
```

## Expected Improvements

- **90% reduction** in stale peer entries
- **75% reduction** in failed connection attempts
- **50% improvement** in P2P message latency
- **99.9% uptime** for cross-chain messaging

## Quick Wins (Implement Today)

```go
// 1. Add time-based pruning to peerdb
func (p *PeerStatus) prune() bool {
    age := time.Since(p.LastSeen)
    return age > 7*24*time.Hour || p.ConsecutiveFailures > 10
}

// 2. Add connection pooling
var connPool = &sync.Map{}

func dial(ctx context.Context, peer peer.ID) (Stream, error) {
    if conn, ok := connPool.Load(peer); ok {
        return conn.(Stream), nil
    }
    // ... dial new connection
}
```

## Files to Modify

### Core P2P Components
- `/pkg/api/v3/p2p/peer_manager.go` - Add centralized coordination
- `/pkg/api/v3/p2p/dial/tracker_persistent.go` - Enhanced peer tracking
- `/pkg/api/v3/p2p/peerdb/db.go` - Time-based pruning

### Connection Management
- `/pkg/api/v3/p2p/dial/dialer.go` - Connection pooling & circuit breakers
- `/pkg/api/v3/message/transport.go` - Health-aware routing

## Testing Strategy

- Unit tests for peer pruning logic
- Integration tests for centralized registry
- Load tests with 1000+ peers
- Chaos testing with network partitions
- Performance benchmarks for connection pooling

## Migration Path

1. Deploy Phase 1 fixes immediately (no API changes)
2. Roll out Phase 2 enhancements to validators
3. Deploy centralized monitoring service (Phase 3)
4. Enable advanced features gradually (Phase 4)

## Success Metrics

- Peer database size < 100 active peers
- Connection success rate > 95%
- P2P message latency < 100ms (p95)
- Zero connection storms
- Cross-chain message reliability > 99.9%