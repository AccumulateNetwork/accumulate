# Accumulate P2P Architecture Documentation

## Overview

Accumulate uses libp2p for its peer-to-peer networking layer, enabling nodes to discover each other, share data, and maintain network connectivity. This document details the implementation, current issues, and solutions discovered during debugging the mainnet infrastructure.

## Table of Contents

1. [Core Components](#core-components)
2. [Bootstrap Server](#bootstrap-server)
3. [Peer Discovery and Management](#peer-discovery-and-management)
4. [Peer Database Architecture](#peer-database-architecture)
5. [Current Network Topology](#current-network-topology)
6. [Known Issues and Solutions](#known-issues-and-solutions)
7. [Maintenance Procedures](#maintenance-procedures)

## Core Components

### P2P Package Structure

```
pkg/api/v3/p2p/
├── p2p.go              # Main P2P node implementation
├── peer_manager.go     # Peer discovery and DHT management
├── discovery.go        # DHT bootstrap and service discovery
├── dial/               # Connection management
│   ├── dialer.go       # Connection dialing logic
│   └── tracker_persistent.go  # Persistent peer tracking
└── peerdb/            # Peer database implementation
    ├── db.go          # Database operations
    └── types.go       # Data structures
```

### Key Technologies

- **libp2p**: Modular peer-to-peer networking stack
- **Kademlia DHT**: Distributed hash table for peer discovery
- **QUIC Protocol**: Transport layer (with legacy draft-29 compatibility)
- **Multiaddr**: Universal addressing format for nodes

## Bootstrap Server

### Purpose

The bootstrap server (`accumulate-p2p-bootstrap`) provides initial peer addresses to new nodes joining the network. It doesn't participate in consensus but maintains a list of active network peers.

### Configuration Locations

1. **Hardcoded in source**: `pkg/accumulate/api.go:BootstrapServers`
2. **AWS Infrastructure**: EC2 instance `i-0e053e32862689726` in `us-east-2`
3. **Docker container**: Running `accumulated-bootstrap` with peer list

### Current Bootstrap Peers

```go
// pkg/accumulate/api.go
var BootstrapServers = []string{
    "/dns/apollo-mainnet.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWPs19932secARrxoRR5J8ZtBMt2vqwyHH1Q9p8thYP7cn",
    "/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWGJTh4aeF7bFnwo9sAYRujCkuVU1Cq8wNeTNGpFgZgXdg",
}
```

### Bootstrap Process

1. New node starts and reads bootstrap addresses
2. Connects to bootstrap peers via libp2p
3. Performs DHT bootstrap to join the network
4. Discovers additional peers through DHT queries
5. Stores discovered peers in local database

## Peer Discovery and Management

### DHT (Distributed Hash Table)

The DHT enables decentralized peer discovery:

```go
// pkg/api/v3/p2p/discovery.go
func startDHT(host host.Host, ctx context.Context, mode dht.ModeOpt, bootstrapPeers []multiaddr.Multiaddr) (*dht.IpfsDHT, error) {
    // Create DHT instance
    d, err := dht.New(ctx, host, dht.Mode(mode))
    
    // Connect to bootstrap peers
    for _, addr := range bootstrapPeers {
        pi, err := peer.AddrInfoFromP2pAddr(addr)
        host.Connect(ctx, *pi)
    }
    
    // Bootstrap the DHT
    d.Bootstrap(ctx)
}
```

### Peer Manager

The `peerManager` (pkg/api/v3/p2p/peer_manager.go) handles:
- DHT queries for finding peers
- Service discovery and registration
- Event notifications for peer changes

### Service Discovery

Nodes advertise services they provide:
- `network:directory` - Directory network services
- `network:apollo` - Apollo BVN services
- `network:yutu` - Yutu BVN services
- `network:chandrayaan` - Chandrayaan BVN services

## Peer Database Architecture

### Storage Structure

The peer database uses BadgerDB to persist peer information:

```go
// pkg/api/v3/p2p/peerdb/types_gen.go
type PeerStatus struct {
    ID        peer.ID
    Addresses []PeerAddressStatus  // Known addresses
    Networks  []PeerNetworkStatus  // Networks peer participates in
}

type LastStatus struct {
    Success *time.Time  // Last successful connection
    Attempt *time.Time  // Last connection attempt
    Failed  uint        // Failed attempt counter
}
```

### Database Location

Peer databases are stored in:
- BadgerDB format: `/root/accumulate*/pkg/api/v3/p2p/peerdb/`
- JSON format: `peers.json` (if configured)

### Persistence Cycle

1. **Scanning** (every hour by default):
   - Query DHT for new peers
   - Test connectivity to known peers
   - Update peer status

2. **Pruning** (on write):
   - Remove peers with no addresses
   - Remove services with no successful connections
   - Currently doesn't remove stale peers (bug)

3. **Writing** (every hour by default):
   - Prune database
   - Persist to disk

## Current Network Topology

### MainNet Status (as of August 2025)

```
┌─────────────────────────────────────┐
│         Bootstrap Server            │
│    bootstrap.accumulate.defidevs.io │
│         54.211.10.186               │
└────────────┬────────────────────────┘
             │
    ┌────────┴────────┬────────────┐
    ▼                 ▼            ▼
┌──────────┐   ┌──────────┐   ┌──────────┐
│  Apollo  │   │   Yutu   │   │Chandrayaan│
│23.22.212.│   │54.234.31.│   │54.85.31.44│
│   106    │   │   209    │   │          │
│Validator │   │   Node   │   │   Node   │
└──────────┘   └──────────┘   └──────────┘
```

### Network Characteristics

- **Total Nodes**: 3 (apollo, yutu, chandrayaan)
- **Validators**: 1 (apollo only)
- **Partitions**: 2 (Directory + 1 BVN)
- **Protocol**: MainNet
- **Port**: 16593 (all nodes)

### Node Roles

1. **Apollo**: 
   - Full validator node
   - Runs both Directory and BVN
   - IP: 23.22.212.106

2. **Yutu**: 
   - Non-validator node
   - Runs both Directory and BVN
   - IP: 54.234.31.209

3. **Chandrayaan**: 
   - Non-validator node
   - Runs both Directory and BVN
   - IP: 54.85.31.44

## Known Issues and Solutions

### Issue 1: Stale Peer Accumulation

**Problem**: Nodes accumulate dead peer entries over time, causing connection timeouts.

**Root Cause**: 
- No automatic pruning of old peers
- DHT gossip spreads stale peer information
- Peer database grows unbounded

**Symptoms**:
```bash
# Commands timeout with errors like:
Unable to dial peer peer=12D3KooW... error="dial tcp4 ...: i/o timeout"
Error: no live peers for network:directory
```

**Solution**:
1. Clear peer databases on all nodes
2. Restart accumulated processes
3. Implement time-based peer pruning (see recommendations)

### Issue 2: QUIC Protocol Version Mismatch

**Problem**: Some peers advertise old QUIC draft-29, others use RFC 9000.

**Root Cause**: Mixed libp2p versions across network history.

**Solution**: Implemented in `pkg/api/v3/p2p/discovery.go:oldQuicCompat()`
```go
// Converts /quic/draft-29 to /quic-v1
func oldQuicCompat(addr multiaddr.Multiaddr) multiaddr.Multiaddr
```

### Issue 3: Private IP Advertisement

**Problem**: Nodes advertise private IPs (172.31.x.x) alongside public IPs.

**Impact**: External nodes fail to connect using private addresses.

**Solution**: Connection attempts try all advertised addresses.

### Issue 4: Bootstrap Server Misconfiguration

**Problem**: Bootstrap server was providing testnet peers to mainnet nodes.

**Solution**: Updated Docker container configuration:
```bash
docker run -d accumulated-bootstrap bootstrap \
  --peer="/dns/apollo-mainnet.accumulate.defidevs.io/tcp/16593/p2p/..." \
  --peer="/dns/yutu-mainnet.accumulate.defidevs.io/tcp/16593/p2p/..." \
  --peer="/dns/chandrayaan-mainnet.accumulate.defidevs.io/tcp/16593/p2p/..."
```

## Maintenance Procedures

### Clearing Peer Databases

When nodes accumulate too many stale peers:

```bash
# 1. Stop accumulated process
sudo pkill accumulated

# 2. Backup and remove peer database
sudo mv /root/accumulate*/pkg/api/v3/p2p/peerdb /root/peerdb.backup

# 3. Restart accumulated
sudo /bin/accumulated run-dual /node/dnn /node/bvnn --truncate
```

### Updating Bootstrap Server

```bash
# 1. SSH to bootstrap server (AWS EC2)
aws ec2-instance-connect send-ssh-public-key \
  --region us-east-2 \
  --instance-id i-0e053e32862689726 \
  --instance-os-user ubuntu \
  --ssh-public-key file://key.pub

# 2. Update Docker container
docker stop accumulated-bootstrap
docker run -d --name accumulated-bootstrap \
  --restart unless-stopped \
  -p 16593:16593 \
  accumulated:latest bootstrap \
  --peer="<mainnet-peer-addresses>"
```

### Testing Connectivity

```bash
# Test P2P connectivity
./debug test-p2p mainnet

# Test sequence (may timeout with stale peers)
./debug sequence mainnet

# Direct port test
nc -zv <node-ip> 16593
```

## Recommendations for Improvement

### 1. Implement Automatic Peer Pruning

Add time-based removal to `pkg/api/v3/p2p/peerdb/db.go`:

```go
func (p *PeerStatus) prune() bool {
    // Remove peers not seen in 7 days
    cutoff := time.Now().Add(-7 * 24 * time.Hour)
    hasRecentSuccess := false
    
    for _, addr := range p.Addresses.Load() {
        if addr.Last.Success != nil && addr.Last.Success.After(cutoff) {
            hasRecentSuccess = true
            break
        }
    }
    
    return !hasRecentSuccess
}
```

### 2. Add Peer Connection Limits

Limit DHT routing table size to prevent unbounded growth:

```go
// In discovery.go
dht.New(ctx, host, 
    dht.Mode(mode),
    dht.BucketSize(20),  // Limit peers per bucket
    dht.RoutingTableLatencyTolerance(time.Minute),
)
```

### 3. Implement Health Monitoring

Add metrics for:
- Active peer count
- Failed connection rate
- DHT query latency
- Peer database size

### 4. Network Expansion Process

When adding new nodes:

1. Deploy new node with accumulated
2. Configure with mainnet genesis
3. Add to bootstrap server peer list
4. Update DNS records if applicable
5. Monitor DHT propagation
6. Document in network topology

## Debugging Commands

```bash
# View peer database (if JSON format)
cat /path/to/peers.json | jq '.peers | length'

# Monitor accumulated logs
journalctl -u accumulated -f

# Check process status
ps aux | grep accumulated

# Network statistics
ss -tunap | grep 16593

# DHT debugging (if debug build)
./debug sequence mainnet --verbose
```

## Related Documentation

- [Network Expansion Guide](./network-expansion/README.md)
- [Troubleshooting Guide](./TROUBLESHOOTING.md)
- [libp2p Documentation](https://docs.libp2p.io/)
- [Accumulate Protocol Docs](https://docs.accumulate.defidevs.io/)

## Glossary

- **BVN**: Block Validator Network - Partition that processes transactions
- **DHT**: Distributed Hash Table - Decentralized peer discovery mechanism
- **DN**: Directory Network - Coordinates between BVNs
- **libp2p**: Modular peer-to-peer networking library
- **Multiaddr**: Self-describing network addresses
- **Peer ID**: Unique cryptographic identifier for a node
- **QUIC**: Quick UDP Internet Connections - Transport protocol