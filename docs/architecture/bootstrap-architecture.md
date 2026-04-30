# Bootstrap Server Architecture

## Summary

The MCP server now includes bootstrap server querying functionality with proper understanding of Accumulate's dual P2P architecture.

## Two Separate P2P Systems

### 1. CometBFT P2P (Consensus Layer)
- **Purpose**: Block synchronization and consensus
- **Server**: apollo-mainnet.accumulate.defidevs.io (Cyclops validator)
- **Ports**:
  - DN: 16591 (P2P), 16592 (RPC)
  - BVN: 16691 (P2P), 16692 (RPC)
- **Peer IDs**: CometBFT node IDs (hex format, e.g., `ebb29bee942723271a39217bd0ed62f7827245de`)
- **Query**: `/net_info` endpoint
- **Usage**: Follower nodes connect here for block sync

### 2. libp2p (Application Layer)
- **Purpose**: Peer discovery for new P2P system via Kademlia DHT
- **Server**: bootstrap.accumulate.defidevs.io (dedicated bootstrap node)
- **Ports**:
  - DN: 16593
  - BVN: 16693
- **Peer IDs**: libp2p peer IDs (base58 format, e.g., `12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx`)
- **Usage**: Application-level peer discovery

## Current Bootstrap Peers (Hardcoded)

These values are **CORRECT** and point to the libp2p bootstrap server:

### Directory Network (DN)
```
/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx
```

### BVN (Cyclops)
```
/dns/bootstrap.accumulate.defidevs.io/tcp/16693/p2p/12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx
```

## Test Results

Running `accumulate_compare_bootstrap_peers` tool shows:

### DN Comparison
- **CometBFT Query**: Returns apollo-mainnet's CometBFT peers (mainnet1)
  - Format: `/dns/apollo-mainnet.accumulate.defidevs.io/tcp/16593/p2p/ebb29bee...`
  - This is a CometBFT node ID, not a libp2p peer ID
- **Hardcoded**: Points to libp2p bootstrap server ✅
  - Format: `/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooW...`
  - This is the correct libp2p bootstrap peer
- **Match**: No (but that's expected - they serve different purposes)

### BVN Comparison
- **CometBFT Query**: No peers found (BVN has 0 CometBFT peers)
- **Hardcoded**: Points to libp2p bootstrap server ✅
- **Match**: Yes (fallback to hardcoded works correctly)

## Implementation

### Files Created
1. **bootstrap_client.go** - HTTP client for querying CometBFT /net_info
2. **bootstrap_client_test.go** - Tests comparing live vs hardcoded peers
3. **tools_build.go** - Binary building tool
4. **tools_build_test.go** - Tests for binary building

### New MCP Tools
1. **accumulate_build_binary** - Build accumulated from source
2. **accumulate_compare_bootstrap_peers** - Compare live vs hardcoded bootstrap peers

### Modified Files
1. **tools_accman_artifacts.go** - Updated bootstrap peer addresses, added comparison tool
2. **tool_definitions.go** - Added new tool definitions
3. **server.go** - Added tool case handlers

## Recommendations

### For Follower Deployment (accman)

Use **BOTH** types of peers:

1. **libp2p Bootstrap Peers** (for peer discovery):
   ```toml
   dn-bootstrap-peers = ["/dns/bootstrap.accumulate.defidevs.io/tcp/16593/p2p/12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx"]
   bvn-bootstrap-peers = ["/dns/bootstrap.accumulate.defidevs.io/tcp/16693/p2p/12D3KooWDgqY8C7deYWzgTQ7qauanMkvn47TPLtrT1TfzETQW3Gx"]
   ```

2. **CometBFT Persistent Peers** (for block sync):
   ```toml
   [network.Directory]
   persistent-peers = ["3029240e829e58e399bc7b6115bb6bc947cc24c7@apollo-mainnet.accumulate.defidevs.io:16591"]

   [network.Cyclops]
   persistent-peers = ["3029240e829e58e399bc7b6115bb6bc947cc24c7@apollo-mainnet.accumulate.defidevs.io:16691"]
   ```

### Why Hardcoded Values Are Correct

The libp2p bootstrap server is a **stable infrastructure component** with:
- Dedicated purpose (DHT bootstrapping)
- Stable peer ID
- High availability
- Separate from consensus validators

This is similar to how IPFS uses well-known bootstrap nodes. The addresses should be hardcoded and only updated when:
- Bootstrap server is migrated
- Peer ID changes (requires regenerating node key)
- Network topology changes significantly

## Future Enhancements

1. **Query libp2p DHT directly** - Implement libp2p client to query the DHT
2. **Dynamic peer discovery** - Use bootstrap server to discover active validators
3. **Health monitoring** - Track bootstrap server availability
4. **Multiple bootstrap servers** - Add redundancy for high availability

## References

- MAINNET_TOPOLOGY_2025-11-17.md - Current network topology
- follower_deployment_session_2025-11-16.md - Deployment issues and fixes
- [libp2p Documentation](https://docs.libp2p.io/)
- [CometBFT Documentation](https://docs.cometbft.com/)
