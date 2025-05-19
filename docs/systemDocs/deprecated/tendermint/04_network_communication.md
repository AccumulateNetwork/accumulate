---
title: Network Communication with Tendermint in Accumulate
description: Detailed explanation of Tendermint's P2P network and its role in Accumulate's inter-node communication
tags: [tendermint, p2p, network, communication, consensus]
created: 2025-05-16
version: 1.0
---

# Network Communication with Tendermint in Accumulate

## Introduction

Tendermint's peer-to-peer (P2P) network forms the backbone of communication between nodes in the Accumulate protocol. This document provides a detailed explanation of how Tendermint's networking capabilities are leveraged and extended to support Accumulate's multi-chain architecture.

## Tendermint P2P Network Overview

Tendermint provides a robust P2P network implementation with several key features:

### Core Components

1. **Node Discovery**: Finding and connecting to peers
2. **Connection Management**: Establishing and maintaining connections
3. **Message Broadcasting**: Efficiently distributing messages
4. **Peer Exchange (PEX)**: Sharing peer information
5. **Transport Layer**: Secure communication channels

### Network Topology

Tendermint forms a partially connected mesh network:

- Each node maintains connections to multiple peers
- Connections are bidirectional
- Network is resilient to node failures
- Information propagates through gossip protocols

## Accumulate's Multi-Network Architecture

Accumulate extends Tendermint's networking capabilities to support its multi-chain structure:

### Partition-Specific Networks

Each partition (DN, BVNs) has its own independent P2P network:

```
Directory Network P2P Network
  |
  +-- BVN0 P2P Network
  |
  +-- BVN1 P2P Network
  |
  +-- BVN2 P2P Network
```

### Dual-Mode Nodes

Nodes that participate in multiple partitions run multiple Tendermint instances:

```go
// Configuration for a dual-mode node (DN + BVN)
type NodeConfig struct {
    DirectoryConfig    *config.Config
    BlockValidatorConfig *config.Config
    // Other configuration
}
```

## Peer Discovery and Management

### Bootstrap Process

When a node starts, it needs to discover peers:

1. **Seed Nodes**: Connect to predefined seed nodes
2. **Persistent Peers**: Connect to configured persistent peers
3. **Peer Exchange**: Discover additional peers through PEX

```go
// Tendermint P2P configuration
p2p := &config.P2P{
    Seeds:           "seed1.example.com:26656,seed2.example.com:26656",
    PersistentPeers: "peer1.example.com:26656,peer2.example.com:26656",
    AddrBookStrict:  false,
    MaxNumInboundPeers: 40,
    MaxNumOutboundPeers: 10,
}
```

### Peer Walking Implementation

Accumulate implements a custom peer discovery mechanism:

```go
// WalkPeers walks a Tendermint network, using the nodes' peer lists
func WalkPeers(ctx context.Context, client rpc.NetworkClient, fn func(context.Context, coretypes.Peer) (WalkClient, bool)) {
    // Get the initial set of peers
    netInfo, err := client.NetInfo(ctx)
    
    // Process each peer
    for _, peer := range netInfo.Peers {
        // Call the provided function for each peer
        nextClient, cont := fn(ctx, peer)
        if !cont {
            return
        }
        
        // Continue walking if a client was returned
        if nextClient != nil {
            WalkPeers(ctx, nextClient, fn)
        }
    }
}
```

### Client Creation for Peers

Creating RPC clients for discovered peers:

```go
// NewHTTPClientForPeer creates a new Tendermint RPC HTTP client for the given peer
func NewHTTPClientForPeer(peer coretypes.Peer, offset uint64) (rpc.Client, error) {
    // Extract the P2P address from the peer
    listenAddr := peer.NodeInfo.ListenAddr
    
    // Convert P2P port to RPC port
    port = port - uint64(config.PortOffsetTendermintP2P) + uint64(config.PortOffsetTendermintRpc) + uint64(offset)
    
    // Create and return the HTTP client
    return rpc.NewHTTP(fmt.Sprintf("http://%s:%d", ip.String(), port), "/websocket")
}
```

## Message Types and Protocols

Tendermint's P2P network handles several types of messages:

### Consensus Messages

- **Proposal**: Block proposals from proposers
- **Vote**: Prevotes and precommits from validators
- **BlockPart**: Parts of a block being gossiped
- **NewRoundStep**: Consensus round updates

### Mempool Messages

- **Tx**: New transactions
- **TxResponse**: Transaction processing results

### Evidence Messages

- **Evidence**: Proof of validator misbehavior

### State Sync Messages

- **ChunkRequest**: Request for a state chunk
- **ChunkResponse**: Response with state data

## Network Communication Flow

### Transaction Broadcast

When a transaction is submitted:

1. Node receives transaction via API
2. Transaction is validated locally
3. If valid, transaction is broadcast to peers via Tendermint's mempool
4. Peers receive and validate the transaction
5. Transaction propagates through the network

```go
// Submit a transaction to the network
func (d *dispatcher) Submit(ctx context.Context, u *url.URL, env *messaging.Envelope) error {
    // Route the transaction to the appropriate partition
    partition, err := d.router.RouteAccount(u)
    
    // Queue the envelope for broadcast
    d.queue[partition] = append(d.queue[partition], env.Copy())
    return nil
}

// Send queued transactions
func (d *dispatcher) send(ctx context.Context, queue map[string][]*messaging.Envelope, errs chan<- error) {
    // Get clients for each partition
    clients := d.getClients(ctx, want)
    
    // For each partition, submit transactions
    for part, queue := range queue {
        client, ok := clients[part]
        
        // Submit envelopes directly via Tendermint RPC
        submitter := tm.NewSubmitter(tm.SubmitterParams{
            Local: client,
        })
        
        // Process the queue
        for _, env := range queue {
            subs, err := submitter.Submit(ctx, env, api.SubmitOptions{})
            // Handle errors
        }
    }
}
```

### Block Propagation

When a new block is created:

1. Proposer creates a block and sends it to peers
2. Peers validate the block
3. If valid, peers vote on the block
4. Once committed, block is gossiped to non-validator nodes
5. Nodes apply the block to their state

### State Synchronization

For nodes joining the network:

1. New node connects to peers
2. Node determines if it needs state sync or block sync
3. For state sync, node requests state chunks from peers
4. For block sync, node requests blocks sequentially
5. Node applies state or blocks to catch up

## Network Security

Tendermint's P2P network includes several security features:

### Encryption and Authentication

- **Transport Security**: TLS for encrypted communication
- **Node Identity**: Ed25519 keys for node identification
- **Peer Authentication**: Validating peer identities

### DoS Protection

- **Rate Limiting**: Limiting message rates from peers
- **Resource Allocation**: Fair allocation of resources
- **Peer Scoring**: Tracking peer behavior and reliability

### Firewall Configuration

Recommended firewall settings for Accumulate nodes:

```
# Directory Network
- P2P: TCP port 26656
- RPC: TCP port 26657

# Block Validator Network
- P2P: TCP port 26756
- RPC: TCP port 26757

# Block Summary Network
- P2P: TCP port 26856
- RPC: TCP port 26857
```

## Cross-Partition Communication

While each partition has its own Tendermint network, Accumulate implements cross-partition communication:

### Dispatcher Implementation

The dispatcher routes messages between partitions:

```go
// NewDispatcher creates a new dispatcher
func NewDispatcher(router routing.Router, self map[string]DispatcherClient) execute.Dispatcher {
    d := new(dispatcher)
    d.router = router
    d.queue = map[string][]*messaging.Envelope{}
    
    // Make sure the partition IDs are all lower-case
    d.self = make(map[string]DispatcherClient, len(self))
    for part, client := range self {
        d.self[strings.ToLower(part)] = client
    }
    
    return d
}
```

### Client Management

Managing connections to different partitions:

```go
// getClients returns a map of Tendermint RPC clients for the given partitions
func (d *dispatcher) getClients(ctx context.Context, want map[string]bool) map[string]peerClient {
    clients := make(map[string]peerClient, len(want))
    
    // Prefer local clients
    for part, client := range d.self {
        clients[part] = peerClient{DispatcherClient: client}
        delete(want, part)
    }
    
    // Walk the directory for dual-mode nodes on the desired partitions
    WalkPeers(ctx, d.self["directory"], func(ctx context.Context, peer coretypes.Peer) (WalkClient, bool) {
        // Create a client for the BVN
        bvn, err := NewHTTPClientForPeer(peer, config.PortOffsetBlockValidator-config.PortOffsetDirectory)
        
        // Check which BVN it's on
        st, err := bvn.Status(ctx)
        part := st.NodeInfo.Network
        
        // Do we want this partition?
        if want[part] {
            delete(want, part)
            clients[part] = peerClient{bvn, &peer}
        }
        
        return dir, true
    })
    
    return clients
}
```

## Network Monitoring and Metrics

Tendermint provides extensive metrics for network monitoring:

### Connection Metrics

- **Peers**: Number of connected peers
- **Bandwidth**: Inbound and outbound bandwidth usage
- **Latency**: Peer response times

### Message Metrics

- **Messages**: Count of messages by type
- **Bytes**: Volume of data transferred
- **Errors**: Communication errors

### Prometheus Integration

Metrics are exposed via Prometheus:

```go
// Tendermint metrics configuration
instrumentation := &config.Instrumentation{
    Prometheus:           true,
    PrometheusListenAddr: ":26660",
    Namespace:            "tendermint",
}
```

## Troubleshooting Network Issues

Common network issues and their solutions:

### Peer Connectivity

If a node cannot connect to peers:
1. Check firewall settings
2. Verify peer addresses are correct
3. Ensure node identity is properly configured
4. Check network connectivity

### Message Propagation

If messages are not propagating:
1. Check peer connections
2. Verify message validation
3. Look for network partitions
4. Check for rate limiting issues

### Network Partitions

If the network becomes partitioned:
1. Tendermint's consensus will halt until reconnection
2. Once reconnected, consensus will resume
3. State sync can be used to quickly recover

## References

- [Accumulate Overview](../01_accumulate_overview.md)
- [Tendermint Integration Overview](../05_tendermint_integration.md)
- [Multi-Chain Structure](01_multi_chain_structure.md)
- [ABCI Application](02_abci_application.md)
- [Transaction Processing](03_transaction_processing.md)
- [Tendermint P2P Documentation](https://docs.tendermint.com/master/spec/p2p/)
