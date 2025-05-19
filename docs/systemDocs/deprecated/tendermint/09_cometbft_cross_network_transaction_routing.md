---
title: CometBFT Cross-Network Transaction Routing
description: Detailed analysis of CometBFT's interface for transaction routing between independent networks and Accumulate's implementation
tags: [cometbft, p2p, cross-network, transaction-routing, dispatcher]
created: 2025-05-16
version: 1.0
---

# CometBFT Cross-Network Transaction Routing

## Introduction

CometBFT (formerly Tendermint) provides mechanisms for routing transactions between independent blockchain networks. This document examines CometBFT's cross-network transaction routing capabilities and how Accumulate leverages and extends them to support its multi-chain architecture.

## CometBFT's Cross-Network Transaction Interface

CometBFT doesn't natively implement a full cross-chain communication protocol like IBC (Inter-Blockchain Communication), but it does provide fundamental building blocks that enable cross-network transaction routing:

### 1. RPC Client Interface

CometBFT's RPC client interface allows nodes to communicate with other networks:

```go
// NetworkClient defines an interface for querying network information
type NetworkClient interface {
    // Returns the network's status
    Status(context.Context) (*coretypes.ResultStatus, error)
    
    // Returns a list of connected peers
    NetInfo(context.Context) (*coretypes.ResultNetInfo, error)
}
```

### 2. Peer Discovery and Connection

CometBFT provides mechanisms to discover and connect to peers across different networks:

```go
// PeerClient defines methods for peer discovery
type PeerClient interface {
    // Returns information about connected peers
    Peers(context.Context) (*coretypes.ResultPeers, error)
}
```

### 3. Transaction Broadcasting

CometBFT allows broadcasting transactions to other networks through its RPC interface:

```go
// BroadcastTxAsync broadcasts a transaction asynchronously
func (c *HTTP) BroadcastTxAsync(ctx context.Context, tx types.Tx) (*coretypes.ResultBroadcastTx, error)

// BroadcastTxSync broadcasts a transaction synchronously
func (c *HTTP) BroadcastTxSync(ctx context.Context, tx types.Tx) (*coretypes.ResultBroadcastTx, error)

// BroadcastTxCommit broadcasts a transaction and waits for it to be committed
func (c *HTTP) BroadcastTxCommit(ctx context.Context, tx types.Tx) (*coretypes.ResultBroadcastTxCommit, error)
```

## Accumulate's Implementation of Cross-Network Transaction Routing

Accumulate extends CometBFT's capabilities to implement a robust cross-network transaction routing system through its dispatcher implementation.

### The Dispatcher Interface

Accumulate defines a `Dispatcher` interface in `internal/core/execute/execute.go`:

```go
// Dispatcher routes synthetic transactions to their destination
type Dispatcher interface {
    // Submit queues a message to be sent to the specified URL
    Submit(context.Context, *url.URL, *messaging.Envelope) error
    
    // Send sends all queued messages and returns a channel that receives any errors
    Send(context.Context) <-chan error
    
    // Close closes the dispatcher
    Close()
}
```

### Tendermint Dispatcher Implementation

The Tendermint dispatcher implementation in `exp/tendermint/dispatcher.go` provides the core functionality for cross-network transaction routing:

#### Key Components

1. **Router Integration**

   The dispatcher uses Accumulate's routing system to determine which partition should handle a transaction:

   ```go
   func (d *dispatcher) Submit(ctx context.Context, u *url.URL, env *messaging.Envelope) error {
       // Route the account
       partition, err := d.router.RouteAccount(u)
       if err != nil {
           return err
       }

       // Queue the envelope
       partition = strings.ToLower(partition)
       d.queue[partition] = append(d.queue[partition], env.Copy())
       return nil
   }
   ```

2. **Client Management**

   The dispatcher maintains a map of clients for different partitions and dynamically discovers new clients when needed:

   ```go
   func (d *dispatcher) getClients(ctx context.Context, want map[string]bool) map[string]peerClient {
       clients := make(map[string]peerClient, len(want))

       // Prefer local clients
       for part, client := range d.self {
           clients[part] = peerClient{DispatcherClient: client}
           delete(want, part)
       }

       // Do we need any others?
       if len(want) == 0 {
           return clients
       }

       // Walk the directory for dual-mode nodes on the desired partitions
       WalkPeers(ctx, d.self["directory"], func(ctx context.Context, peer coretypes.Peer) (WalkClient, bool) {
           // ... (peer discovery logic)
       })

       return clients
   }
   ```

3. **Transaction Submission**

   The dispatcher submits transactions to the appropriate networks:

   ```go
   func (d *dispatcher) send(ctx context.Context, queue map[string][]*messaging.Envelope, errs chan<- error) {
       // ... (error handling setup)

       // Get clients for each partition
       clients := d.getClients(ctx, want)

       // For each partition
       for part, queue := range queue {
           // Get a client for the partition
           client, ok := clients[part]
           if !ok {
               errs <- errors.InternalError.WithFormat("no client for %v", part)
               continue
           }

           // Submit envelopes via Tendermint RPC
           submitter := tm.NewSubmitter(tm.SubmitterParams{
               Local: client,
           })

           // Process the queue
           for _, env := range queue {
               subs, err := submitter.Submit(ctx, env, api.SubmitOptions{})
               // ... (error handling)
           }
       }
   }
   ```

4. **Port Offset System**

   Accumulate uses a port offset system to connect to different CometBFT instances:

   ```go
   // Create a client for the BVNN
   bvn, err := NewHTTPClientForPeer(peer, config.PortOffsetBlockValidator-config.PortOffsetDirectory)
   ```

   This allows a single node to participate in multiple networks by running multiple CometBFT instances on different ports.

### Cross-Network Transaction Flow

The flow of a cross-network transaction in Accumulate follows these steps:

1. **Transaction Routing**
   - When a transaction is received, the system determines which partition should process it
   - If the transaction belongs to a different partition, it's queued for cross-network dispatch

2. **Client Discovery**
   - The dispatcher identifies or discovers a client for the target partition
   - It first checks for local clients (same node, different port)
   - If no local client exists, it queries the directory network for peers

3. **Transaction Submission**
   - The transaction is submitted to the target network via CometBFT's RPC interface
   - The system handles errors and retries if necessary

4. **Acknowledgment**
   - The dispatcher collects and reports errors from the submission process
   - Successful submissions are acknowledged

## Synthetic Transaction Handling

Accumulate uses synthetic transactions for cross-chain consistency. These are system-generated transactions that maintain state synchronization across different chains.

### Requesting Missing Synthetic Transactions

When a node detects missing synthetic transactions, it uses the dispatcher to request them from other partitions:

```go
func (x *Executor) requestMissingSyntheticTransactions(ctx context.Context) {
    dispatcher := x.NewDispatcher()
    defer dispatcher.Close()

    // Request missing transactions from each partition
    for _, partition := range partitions {
        x.requestMissingTransactionsFromPartition(ctx, dispatcher, partition, false)
        x.requestMissingTransactionsFromPartition(ctx, dispatcher, partition, true)
    }

    // Send all requests
    for err := range dispatcher.Send(ctx) {
        // Handle errors
    }
}
```

### Cross-Partition Message Routing

For cross-partition messages, Accumulate uses the dispatcher to route messages to the appropriate partition:

```go
func (x *Executor) requestMissingTransactionsFromPartition(ctx context.Context, dispatcher Dispatcher, partition *protocol.PartitionSyntheticLedger, anchor bool) {
    // Determine destination URL
    dest := protocol.PartitionUrl(partition.Partition)
    
    // Create message
    msg := &protocol.SyntheticTransactionRequest{
        Partition: x.Describe.PartitionId,
        Anchor:    anchor,
        TxIDs:     txids,
    }
    
    // Submit message to dispatcher
    err = dispatcher.Submit(ctx, dest, &messaging.Envelope{Messages: []messaging.Message{msg}})
}
```

## Comparison with libp2p Implementation

While CometBFT provides basic cross-network transaction routing capabilities, Accumulate also implements a custom libp2p-based P2P system for additional flexibility. Here's how they compare:

### CometBFT Advantages

1. **Simplicity**: CometBFT's approach is more straightforward and tightly integrated with the consensus engine
2. **Reliability**: The system is designed specifically for blockchain consensus communication
3. **Performance**: Optimized for the specific use case of transaction propagation
4. **Maturity**: The implementation is more mature and battle-tested

### libp2p Advantages

1. **Flexibility**: Supports multiple transport protocols and dynamic service discovery
2. **Extensibility**: Easier to extend with custom protocols and services
3. **NAT Traversal**: Better support for NAT traversal and relay capabilities
4. **DHT-based Routing**: More sophisticated peer discovery through DHT

## Practical Implementation Considerations

When implementing cross-network transaction routing with CometBFT, consider the following:

1. **Network Topology**
   - CometBFT works best with a known set of peers
   - For dynamic networks, additional discovery mechanisms may be needed

2. **Transaction Format**
   - Ensure transactions are properly formatted for cross-network transmission
   - Include sufficient routing information in the transaction

3. **Error Handling**
   - Implement robust error handling for network failures
   - Consider retry mechanisms for important transactions

4. **Security**
   - Verify the authenticity of transactions from other networks
   - Implement proper access controls for cross-network communication

5. **Monitoring**
   - Monitor cross-network transaction latency and success rates
   - Implement alerting for persistent routing failures

## Conclusion

CometBFT provides a solid foundation for cross-network transaction routing through its RPC client interface, peer discovery mechanisms, and transaction broadcasting capabilities. Accumulate extends these capabilities with its dispatcher implementation to support its multi-chain architecture.

The hybrid approach of using CometBFT for consensus-related communication and libp2p for more flexible service discovery provides a balance of reliability and flexibility. By understanding the strengths and limitations of each system, developers can make informed decisions about which components to use for different aspects of cross-network communication.

## References

1. [CometBFT RPC Documentation](https://docs.cometbft.com/v0.37/rpc/)
2. [Accumulate Multi-Chain Structure](01_multi_chain_structure.md)
3. [Accumulate Transaction Processing](03_transaction_processing.md)
4. [CometBFT P2P Networking](06_cometbft_p2p_networking.md)
5. [Hybrid P2P Approach for Accumulate](08_hybrid_p2p_approach.md)
