# Peer Database Issues and Solutions

## Executive Summary

The Accumulate network's peer database system suffers from unbounded growth of stale peer entries, causing severe connectivity issues. This document details the problem, its impact, root causes, and both immediate and long-term solutions.

## The Problem

### Symptoms

```
# Failed connection attempts
2025/08/12 08:08:38 INFO Unable to dial peer peer=12D3KooWAgrBYpWEXRViTnToNmpCoC3dvHdmR6m1FmyKjDn1NYpj error="dial tcp4 0.0.0.0:34919->23.22.249.255:16593: i/o timeout"

# Command failures
Error: query network status: dial /acc/MainNet/acc-svc/network:directory: no live peers for network:directory

# DN height stuck
Directory Network height showing static value (e.g., 2460315) instead of updating
```

### Impact

1. **Network Discovery Failures**: New nodes cannot join the network
2. **Command Timeouts**: `debug sequence mainnet` and similar commands fail
3. **Performance Degradation**: Connection attempts to hundreds of dead peers
4. **Resource Waste**: CPU and network bandwidth consumed by failed dial attempts

### Scale of the Problem

After 29 days of operation, mainnet nodes accumulated:
- **500+ dead peer entries**
- **Only 3 active peers** (apollo, yutu, chandrayaan)
- **99.4% of connection attempts fail**

## Root Cause Analysis

### 1. No Automatic Pruning

The current pruning logic (`pkg/api/v3/p2p/peerdb/db.go:52-74`) only removes:
- Peers with no addresses
- Services with no recorded attempts

It does **NOT** remove:
- Peers that haven't been seen in days/weeks
- Peers with consistent connection failures
- Peers from decommissioned nodes

```go
// Current insufficient pruning
func (p *PeerStatus) prune() bool {
    p.Addresses.RemoveFunc((*PeerAddressStatus).prune)
    p.Networks.RemoveFunc((*PeerNetworkStatus).prune)
    return len(p.Addresses.Load()) == 0 &&  // Only if NO addresses
           len(p.Networks.Load()) == 0       // Only if NO networks
}
```

### 2. DHT Gossip Propagation

The libp2p DHT continuously shares peer information:

```go
// pkg/api/v3/p2p/discovery.go
func startDHT(...) {
    d, err := dht.New(ctx, host, dht.Mode(mode))
    d.Bootstrap(ctx)  // Starts gossip protocol
}
```

This means:
- Dead peers spread between nodes
- Cleared databases quickly refill
- No mechanism to verify peer liveness before sharing

### 3. Persistent Storage Without Expiry

Peer data persists indefinitely:

```go
// pkg/api/v3/p2p/dial/tracker_persistent.go
func (t *PersistentTracker) writeDb(time.Duration) {
    err = t.db.Store(f)  // Writes ALL peers, regardless of age
}
```

### 4. Historical Network Changes

MainNet has experienced:
- Multiple node additions/removals
- IP address changes
- Node renames and reconfigurations
- TestNet/MainNet confusion

Each change leaves ghost entries in peer databases.

## Data Structure Analysis

### Peer Status Storage

```go
// pkg/api/v3/p2p/peerdb/types_gen.go
type PeerStatus struct {
    ID        peer.ID
    Addresses *AtomicSlice[*PeerAddressStatus]
    Networks  *AtomicSlice[*PeerNetworkStatus]
}

type LastStatus struct {
    Success *time.Time   // Last successful connection
    Attempt *time.Time   // Last attempt (success or failure)
    Failed  *AtomicUint  // Consecutive failure count
}
```

### Storage Locations

1. **BadgerDB**: `/root/accumulate*/pkg/api/v3/p2p/peerdb/`
2. **JSON** (if configured): `peers.json`
3. **Memory**: In-process cache

## Immediate Solutions

### 1. Manual Peer Database Clearing

**Script**: `scripts/fix-mainnet-peers.sh`

```bash
#!/bin/bash
# Clear peer databases on all nodes

for node in apollo yutu chandrayaan; do
    # Connect via SSH
    ssh ubuntu@$node_ip << 'EOF'
        # Find and backup peer databases
        sudo find / -type d -name "peerdb" -o -name "peer-db" | while read dir; do
            sudo mv "$dir" "${dir}.backup"
        done
        
        # Restart accumulated
        sudo pkill accumulated
        sudo /bin/accumulated run-dual /node/dnn /node/bvnn --truncate &
EOF
done
```

**Result**: Temporary relief, but peers accumulate again within hours.

### 2. Bootstrap Server Fix

Ensure bootstrap server only advertises active nodes:

```bash
docker run -d accumulated:latest bootstrap \
  --network="MainNet" \
  --peer="/dns/apollo-mainnet.accumulate.defidevs.io/tcp/16593/p2p/..." \
  --peer="/dns/yutu-mainnet.accumulate.defidevs.io/tcp/16593/p2p/..." \
  --peer="/dns/chandrayaan-mainnet.accumulate.defidevs.io/tcp/16593/p2p/..."
```

### 3. Skip Network Scanning

Added `--skip-scan` flag to bypass peer discovery:

```go
// tools/cmd/debug/sequence.go
var skipNetworkScan bool

func init() {
    cmdSequence.Flags().BoolVar(&skipNetworkScan, "skip-scan", false, 
        "Skip network scanning (useful for limited networks)")
}
```

## Long-Term Solutions

### Solution 1: Time-Based Peer Pruning (Recommended)

**Implementation Complexity**: Low (2-4 hours)

```go
// pkg/api/v3/p2p/peerdb/db.go
func (p *PeerStatus) prune() bool {
    const maxAge = 7 * 24 * time.Hour
    cutoff := time.Now().Add(-maxAge)
    
    // Check all addresses for recent success
    hasRecentSuccess := false
    for _, addr := range p.Addresses.Load() {
        if addr.Last.Success != nil && addr.Last.Success.After(cutoff) {
            hasRecentSuccess = true
            break
        }
    }
    
    // Remove if no recent successful connections
    return !hasRecentSuccess
}
```

**Benefits**:
- Automatic cleanup of dead peers
- Configurable retention period
- Minimal code changes

### Solution 2: Failure-Based Pruning

**Implementation Complexity**: Low-Medium (4-6 hours)

```go
func (p *PeerStatus) prune() bool {
    const maxFailures = 10
    const failureWindow = 24 * time.Hour
    
    for _, addr := range p.Addresses.Load() {
        // Keep if recently successful
        if addr.Last.Success != nil && 
           time.Since(*addr.Last.Success) < failureWindow {
            return false
        }
        
        // Remove if too many failures
        if addr.Last.Failed.Load() > maxFailures {
            continue
        }
        
        // Keep if not enough data
        return false
    }
    
    return true  // Remove peer
}
```

### Solution 3: DHT Routing Table Filter

**Implementation Complexity**: Medium (1-2 days)

```go
// pkg/api/v3/p2p/discovery.go
func startDHT(...) {
    d, err := dht.New(ctx, host, 
        dht.Mode(mode),
        dht.RoutingTableFilter(func(dht *IpfsDHT, p peer.ID) bool {
            // Only share recently active peers
            status := peerDB.GetPeer(p)
            return status != nil && status.IsRecentlyActive(24 * time.Hour)
        }),
        dht.BucketSize(20),  // Limit routing table size
    )
}
```

**Benefits**:
- Prevents dead peer propagation
- Reduces network traffic
- Improves discovery efficiency

### Solution 4: Peer Scoring System

**Implementation Complexity**: High (2-3 days)

```go
type PeerScore struct {
    SuccessRate     float64
    AvgLatency      time.Duration
    LastSeen        time.Time
    TotalAttempts   uint64
    ConsecutiveFails uint
}

func (p *PeerStatus) Score() float64 {
    // Calculate composite score
    score := p.SuccessRate * 0.4
    score += (1.0 / (1.0 + p.AvgLatency.Seconds())) * 0.3
    score += math.Min(1.0, time.Since(p.LastSeen).Hours()/24) * 0.3
    return score
}

// Use score for connection prioritization
func (d *Dialer) selectPeers(peers []PeerStatus) []PeerStatus {
    sort.Slice(peers, func(i, j int) bool {
        return peers[i].Score() > peers[j].Score()
    })
    return peers[:min(20, len(peers))]  // Top 20 peers
}
```

## Testing Procedures

### Verify Peer Database Size

```bash
# Check BadgerDB size
du -sh /root/accumulate*/pkg/api/v3/p2p/peerdb/

# Count peers in JSON (if available)
cat peers.json | jq '.peers | length'

# Monitor connection attempts
./debug sequence mainnet --verbose 2>&1 | grep "Unable to dial" | wc -l
```

### Test Connectivity

```bash
# Quick test
./debug test-p2p mainnet

# Full sequence test
./debug sequence mainnet

# With skip-scan workaround
./debug sequence mainnet --skip-scan
```

### Monitor Peer Accumulation

```bash
# Track peer database growth
while true; do
    date
    find /root -name "peerdb" -type d -exec du -sh {} \;
    sleep 3600
done
```

## Prevention Strategies

### 1. Regular Maintenance

Schedule automated peer database cleanup:

```bash
# Crontab entry
0 3 * * 0 /usr/local/bin/clean_peer_db.sh
```

### 2. Monitoring and Alerts

Implement monitoring for:
- Peer database size
- Failed connection ratio
- Command success rate

### 3. Network Documentation

Maintain accurate records of:
- Active node list
- Decommissioned nodes
- IP address changes
- Network topology updates

## Recovery Procedures

### When Network is Completely Broken

1. **Stop all nodes**
   ```bash
   for node in apollo yutu chandrayaan; do
       ssh ubuntu@$node "sudo pkill accumulated"
   done
   ```

2. **Clear all peer databases**
   ```bash
   for node in apollo yutu chandrayaan; do
       ssh ubuntu@$node "sudo rm -rf /root/accumulate*/pkg/api/v3/p2p/peerdb"
   done
   ```

3. **Update bootstrap server**
   ```bash
   # Restart with only active peers
   docker restart accumulated-bootstrap
   ```

4. **Start nodes sequentially**
   ```bash
   # Start apollo first (validator)
   ssh ubuntu@apollo "sudo /bin/accumulated run-dual /node/dnn /node/bvnn &"
   sleep 30
   
   # Then other nodes
   for node in yutu chandrayaan; do
       ssh ubuntu@$node "sudo /bin/accumulated run-dual /node/dnn /node/bvnn &"
       sleep 30
   done
   ```

5. **Verify recovery**
   ```bash
   ./debug test-p2p mainnet
   ./debug sequence mainnet
   ```

## Code Locations

Key files involved in peer management:

```
pkg/api/v3/p2p/
├── peerdb/
│   ├── db.go              # Database operations and pruning
│   ├── types.go           # Data structures
│   └── types_gen.go       # Generated types
├── dial/
│   ├── tracker_persistent.go  # Persistent peer tracking
│   └── dialer.go          # Connection management
├── discovery.go           # DHT bootstrap and discovery
└── peer_manager.go        # High-level peer management
```

## Metrics to Track

1. **Peer Database Health**
   - Total peers stored
   - Active vs. dead ratio
   - Database size on disk

2. **Connection Performance**
   - Dial success rate
   - Average connection time
   - Timeout frequency

3. **Network Health**
   - Nodes reachable
   - DHT query success rate
   - Bootstrap availability

## Related Issues

1. **"Package 2" Error**: Shell redirection causing Go compilation errors
2. **QUIC Version Mismatch**: Mixed protocol versions in network
3. **Private IP Advertisement**: Nodes advertising unreachable addresses
4. **Bootstrap Misconfiguration**: Wrong network peers being served

## Recommendations Priority

1. **Immediate**: Implement time-based pruning (2-4 hours work)
2. **Short-term**: Add failure-based pruning (4-6 hours work)
3. **Medium-term**: DHT routing filter (1-2 days work)
4. **Long-term**: Full peer scoring system (2-3 days work)

## References

- [P2P Architecture Documentation](./P2P-ARCHITECTURE.md)
- [Bootstrap Server Guide](./BOOTSTRAP-SERVER.md)
- [libp2p DHT Specification](https://github.com/libp2p/specs/tree/master/kad-dht)
- [BadgerDB Documentation](https://dgraph.io/docs/badger/)