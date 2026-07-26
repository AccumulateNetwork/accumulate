# Accumulate P2P Architecture

Accumulate uses **two separate P2P systems** that serve different purposes. Understanding this distinction is critical for debugging connectivity issues.

## Overview

| System | Protocol | Purpose | Port Range |
|--------|----------|---------|------------|
| **CometBFT P2P** | Tendermint P2P | Block sync, consensus between validators | 16591 (DN), 16691 (BVN) |
| **Accumulate P2P** | libp2p | API routing, cross-partition messaging | 16593 (DN), 16693 (BVN) |

## CometBFT P2P (Block Sync & Consensus)

### What It Does
- Synchronizes blocks between validators and followers
- Participates in consensus (validators only)
- Uses Tendermint's custom P2P protocol (not libp2p)

### Configuration
Located in `{node}/config/tendermint.toml`:

```toml
[p2p]
# Seed nodes - contacted once at startup to populate address book
seeds = "node_id@ip:port,node_id@ip:port"

# Persistent peers - always maintain connection to these nodes
persistent_peers = "node_id@ip:port"

# Never disconnect from these node IDs (just the hex ID, not full address)
unconditional_peer_ids = "node_id1,node_id2"

# Peer exchange - discover new peers from connected peers
pex = true
```

### Key Points
- **Node IDs are 40-character hex strings** (e.g., `3029240e829e58e399bc7b6115bb6bc947cc24c7`)
- Seeds are only used to bootstrap the address book
- Persistent peers are actively maintained connections
- `unconditional_peer_ids` takes ONLY node IDs, not `id@ip:port` format

### Common Issues
1. **Stalled validators** - A seed/persistent peer may be online but not producing new blocks
2. **Address book corruption** - Failed dial attempts cause exponential backoff
3. **Wrong format** - `unconditional_peer_ids` with `@ip:port` causes hex decode errors

## Accumulate P2P (libp2p)

### What It Does
- Routes API requests between partitions
- Enables cross-partition message passing
- Provides peer discovery via Kademlia DHT

### Bootstrap Server
The bootstrap server (`accumulated-bootstrap`) is a **libp2p DHT node**, NOT an Accumulate network node:

```go
P2P: &P2P{
    DiscoveryMode: Ptr(DhtMode(dht.ModeAutoServer)),
}
```

It serves as a "bulletin board" where:
- Nodes register their presence
- Nodes discover other active nodes
- No blockchain logic runs here

### Configuration
In `accumulate.toml`:

```toml
[[configurations]]
  type = "follower"
  # libp2p bootstrap peers (multiaddr format)
  bootstrap-peers = [
    "/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooW..."
  ]
```

### Key Points
- Uses **multiaddr format** (e.g., `/ip4/1.2.3.4/tcp/16593/p2p/PeerID`)
- Peer IDs are base58-encoded multihash (e.g., `12D3KooWDgqY8C7...`)
- Completely separate from CometBFT P2P

## Current P2P Optimizations

Applied in `cmd/accumulated/run/consensus.go`:

```go
// Increase outbound peers for better block source diversity
d.config.P2P.MaxNumOutboundPeers = 20  // default: 10

// Higher bandwidth for fast sync
d.config.P2P.SendRate = 20480000  // 20 MB/s (default: 5 MB/s)
d.config.P2P.RecvRate = 20480000  // 20 MB/s

// Reduce flush throttle for responsive connections
d.config.P2P.FlushThrottleTimeout = 50 * time.Millisecond  // default: 100ms

// Reduce reconnection churn for persistent peers
d.config.P2P.PersistentPeersMaxDialPeriod = 30 * time.Second  // default: exponential

// Never disconnect from persistent peers (extract node IDs only)
if d.config.P2P.PersistentPeers != "" {
    var ids []string
    for _, peer := range strings.Split(d.config.P2P.PersistentPeers, ",") {
        if idx := strings.Index(peer, "@"); idx > 0 {
            ids = append(ids, peer[:idx])
        }
    }
    d.config.P2P.UnconditionalPeerIDs = strings.Join(ids, ",")
}
```

## Known Issues (December 2025)

### Stalled Validator at 144.76.105.23
- This validator stopped producing blocks in October 2025
- Block height frozen at ~6.7M while mainnet is at ~14M
- **Do not use as seed/persistent peer**

### Active Validators
- `23.22.212.106` - AWS validator, currently active
- Node ID (DN): `3029240e829e58e399bc7b6115bb6bc947cc24c7`

## Debugging Checklist

### No Peers / Not Syncing

1. **Check CometBFT peer count**:
   ```bash
   curl -s http://localhost:16592/net_info | grep n_peers
   ```

2. **Verify seeds/persistent_peers are active**:
   ```bash
   curl -s http://SEED_IP:16592/status | grep latest_block_time
   ```
   If block time is old, the validator is stalled.

3. **Check address book for failed attempts**:
   ```bash
   grep attempts config/addrbook.json
   ```
   Many attempts with no success = bad addresses or network issues.

4. **Clear address book and restart**:
   ```bash
   rm config/addrbook.json
   # restart node
   ```

### libp2p Issues

1. **Check bootstrap server health**:
   ```bash
   curl http://bootstrap.accumulate.defidevs.io:8080/health
   ```

2. **List connected libp2p peers**:
   ```bash
   curl http://bootstrap.accumulate.defidevs.io:8080/peers
   ```

## Recommended Configuration for Followers

### tendermint.toml (DN)
```toml
[p2p]
persistent_peers = "3029240e829e58e399bc7b6115bb6bc947cc24c7@23.22.212.106:16591"
unconditional_peer_ids = "3029240e829e58e399bc7b6115bb6bc947cc24c7"
max_num_outbound_peers = 20
send_rate = 20480000
recv_rate = 20480000
flush_throttle_timeout = "50ms"
persistent_peers_max_dial_period = "30s"
pex = true
addr_book_strict = false
```

### tendermint.toml (BVN)
```toml
[p2p]
persistent_peers = "3029240e829e58e399bc7b6115bb6bc947cc24c7@23.22.212.106:16691"
unconditional_peer_ids = "3029240e829e58e399bc7b6115bb6bc947cc24c7"
# ... same optimizations as DN
```

### accumulate.toml
```toml
[[configurations]]
  type = "follower"
  bootstrap-peers = [
    "/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx"
  ]
```
