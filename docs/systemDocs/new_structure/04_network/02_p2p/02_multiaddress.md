# P2P Multiaddress Construction and Peer Discovery

```yaml
# AI-METADATA
document_type: technical_guide
project: accumulate_network
component: peer_discovery
version: current
related_files:
  - ./network_initiation_fix.md
  - ./network_initiation_analysis.md
  - ../implementations/v4/peer-discovery-analysis.md
  - ../implementations/v4/peer-discovery-integration.md
```

## 1. Introduction

This document provides a comprehensive guide to constructing valid P2P multiaddresses and implementing effective peer discovery in the Accumulate Network. It is based on analysis of the working implementation in `mainnet-status/main.go` and aims to address the issues identified in the `heal_common.go` implementation.

## 2. P2P Multiaddress Requirements

A valid P2P multiaddress must include all three of the following components:

1. **Network Component**: Specifies the network protocol and address
   - Examples: `/dns/example.com`, `/ip4/192.168.1.1`, `/ip6/2001:db8::1`

2. **Transport Component**: Specifies the transport protocol and port
   - Example: `/tcp/16593`

3. **P2P Component**: Specifies the peer ID
   - Example: `/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N`

### 2.1 Complete Example

A complete valid multiaddress looks like:
```
/dns/node1.example.com/tcp/16593/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N
```

### 2.2 Common Errors

1. **Missing P2P Component**: Addresses like `/ip4/144.76.105.23/tcp/16593` are invalid for P2P connections because they lack the peer ID.
   - Error: "cannot specify address without peer ID"

2. **Invalid P2P Component**: The peer ID must be a valid CID (Content Identifier).
   - Error: "parse address: invalid p2p multiaddr"

3. **Missing Transport Component**: Addresses must specify the transport protocol.
   - Error: "no transport protocol specified"

## 3. Proper Multiaddress Construction

The following code snippet demonstrates the correct way to construct a multiaddress:

```go
// Correct way to construct a multiaddress
addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/dns/%s/tcp/16593/p2p/%v", node.Host, node.PeerID))
if err != nil {
    // Handle error
    return err
}
```

When encapsulating a service type:

```go
// Encapsulate a service type
serviceAddr := addr.Encapsulate(api.ServiceTypeNode.Address().Multiaddr())
```

## 4. Peer Discovery Process

The peer discovery process should follow these steps:

### 4.1 Initial Network Scan

1. **Query Network Status**: Get the network status to identify validators and partitions
2. **Query Node Info**: Get node information to identify the network and peer ID

```go
// Get network status
ns, err := public.NetworkStatus(ctx, api.NetworkStatusOptions{})
if err != nil {
    return nil, err
}

// Get node info
ni, err := public.NodeInfo(ctx, api.NodeInfoOptions{})
if err != nil {
    return nil, err
}
```

### 4.2 Scan Network for Peers

Use the `healing.ScanNetwork` function to discover peers:

```go
// Scan the network
networkInfo, err = healing.ScanNetwork(ctx, public)
if err != nil {
    return nil, err
}
```

The `ScanNetwork` function performs the following:

1. Queries the network status to get validators and partitions
2. For each partition, finds services using `FindService`
3. For each peer, gets consensus status to identify validators
4. Maps validators to their peer IDs and addresses

### 4.3 Recursive Peer Discovery

Implement recursive peer discovery to find all peers in the network:

1. Start with a set of known peers
2. Query each peer's network info to discover their peers
3. Add newly discovered peers to the set
4. Repeat until no new peers are found

```go
// Use consensus peers to find more peers
for len(cometPeers) > 0 {
    scan := cometPeers
    cometPeers = nil
    for _, peer := range scan {
        if seenComet[peer.NodeInfo.ID()] {
            continue
        }
        
        // Find a usable address
        addr, ok := addressForPeer(peer)
        if !ok {
            continue
        }
        
        seenComet[peer.NodeInfo.ID()] = true
        
        // Get network info to discover more peers
        netInfo, err := c.NetInfo(ctx)
        if err != nil {
            continue
        }
        
        // Add newly discovered peers
        for _, peer := range netInfo.Peers {
            if !seenComet[peer.NodeInfo.ID()] {
                cometPeers = append(cometPeers, peer)
            }
        }
    }
}
```

## 5. P2P Node Initialization

When initializing a P2P node, ensure that all bootstrap peers have valid multiaddresses:

```go
// Initialize P2P node
node, err := p2p.New(p2p.Options{
    Network:           ni.Network,
    BootstrapPeers:    validBootstrapPeers, // Must have valid multiaddresses
    PeerDatabase:      peerDb,
    EnablePeerTracker: true,
})
if err != nil {
    return nil, err
}
```

### 5.1 Validating Bootstrap Peers

Before using addresses as bootstrap peers, validate them:

```go
var validBootstrapPeers []multiaddr.Multiaddr
for _, addr := range discoveredAddresses {
    // Ensure the address has a P2P component
    if !hasP2PComponent(addr) {
        continue
    }
    
    validBootstrapPeers = append(validBootstrapPeers, addr)
}
```

## 6. Integration with JSON-RPC

To integrate nodes discovered via JSON-RPC with the P2P network:

1. Construct valid multiaddresses for each discovered node
2. Include the peer ID in the multiaddress
3. Use these addresses as bootstrap peers for the P2P node

```go
// Construct valid multiaddresses for JSON-RPC discovered nodes
for _, node := range discoveredNodes {
    if node.Host == "" || node.PeerID == "" {
        continue
    }
    
    addr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/dns/%s/tcp/16593/p2p/%v", node.Host, node.PeerID))
    if err != nil {
        continue
    }
    
    bootstrapPeers = append(bootstrapPeers, addr)
}
```

## 7. Error Handling and Recovery

Implement robust error handling and recovery mechanisms:

1. **Timeouts**: Use context with timeouts for all network operations
2. **Retries**: Implement retry logic for transient failures
3. **Fallbacks**: Have fallback mechanisms when primary peers fail
4. **Logging**: Log errors but continue with the discovery process

```go
ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
defer cancel()

// Implement retry logic
for retries := 0; retries < maxRetries; retries++ {
    result, err := operation(ctx)
    if err == nil {
        return result, nil
    }
    
    // Log error and retry
    slog.DebugContext(ctx, "Operation failed, retrying", "error", err, "retry", retries+1)
    time.Sleep(backoff(retries))
}
```

## 8. Best Practices

1. **Always Include Peer ID**: Always include the peer ID in multiaddresses used for P2P connections
2. **Validate Addresses**: Validate multiaddresses before using them as bootstrap peers
3. **Implement Recursive Discovery**: Use recursive peer discovery to find all peers in the network
4. **Handle Errors Gracefully**: Log errors but continue with the discovery process
5. **Use Timeouts**: Use context with timeouts for all network operations
6. **Cache Results**: Cache discovery results to avoid repeated network scanning
7. **Check for nil**: Always check for nil before accessing methods/fields

## 9. References

- `mainnet-status/main.go`: Reference implementation for successful network initialization
- `healing.ScanNetwork`: Function for scanning the network and discovering peers
- `multiaddr` package: Library for working with multiaddresses
