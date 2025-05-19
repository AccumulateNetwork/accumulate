# Accumulate API Overview

```yaml
# AI-METADATA
document_type: reference
project: accumulate_network
component: api
topic: overview
version: 1.0
date: 2025-04-29
```

## Introduction

This document provides a high-level overview of the Accumulate API versions (V1, V2, and V3). It identifies the main endpoints, parameters, and functionality of each API version, serving as a quick reference for developers.

## API Versions Overview

| Version | Status | Base URL Format | Key Features |
|---------|--------|-----------------|--------------|
| V1 | Legacy | `https://{network}.accumulatenetwork.io/v1` | Basic account and transaction operations |
| V2 | Stable | `https://{network}.accumulatenetwork.io/v2` | Enhanced query capabilities, transaction submission |
| V3 | Current | `https://{network}.accumulatenetwork.io/v3` | Comprehensive API with advanced features, P2P support |

## API V3 (Current)

The V3 API is the most comprehensive and current API for interacting with the Accumulate network.

### Service Interfaces

V3 API is organized into several service interfaces:

1. **NodeService** - Node information and service discovery
2. **ConsensusService** - Consensus status and operations
3. **NetworkService** - Network status and operations
4. **SnapshotService** - Snapshot management
5. **MetricsService** - System metrics
6. **Querier** - Data queries
7. **Submitter** - Transaction submission
8. **Validator** - Transaction validation
9. **Faucet** - Token faucet operations

### Main Endpoints

#### Node Operations

| Endpoint | Description | Parameters |
|----------|-------------|------------|
| `node-info` | Get information about a node | `NodeInfoOptions` |
| `find-service` | Find services by type | `FindServiceOptions` |

#### Consensus Operations

| Endpoint | Description | Parameters |
|----------|-------------|------------|
| `consensus-status` | Get consensus status | `ConsensusStatusOptions` |

#### Network Operations

| Endpoint | Description | Parameters |
|----------|-------------|------------|
| `network-status` | Get network status | `NetworkStatusOptions` |

#### Snapshot Operations

| Endpoint | Description | Parameters |
|----------|-------------|------------|
| `list-snapshots` | List available snapshots | `ListSnapshotsOptions` |

#### Metrics Operations

| Endpoint | Description | Parameters |
|----------|-------------|------------|
| `metrics` | Get system metrics | `MetricsOptions` |

#### Query Operations

| Endpoint | Description | Parameters |
|----------|-------------|------------|
| `query` | Execute a query | `scope`, `query` |

#### Transaction Operations

| Endpoint | Description | Parameters |
|----------|-------------|------------|
| `submit` | Submit a transaction | `envelope`, `SubmitOptions` |
| `validate` | Validate a transaction | `envelope`, `ValidateOptions` |

#### Faucet Operations

| Endpoint | Description | Parameters |
|----------|-------------|------------|
| `faucet` | Request tokens from faucet | `account`, `FaucetOptions` |

### Query Types

V3 API supports various query types:

1. **ChainQuery** - Query chain data
2. **DataQuery** - Query account data
3. **DirectoryQuery** - Query directory entries
4. **KeyPageIndexQuery** - Query key page indices
5. **MajorBlocksQuery** - Query major blocks
6. **MinorBlocksQuery** - Query minor blocks
7. **TransactionQuery** - Query transactions

### Example Usage (Go)

```go
// Create a client
client, err := api.NewClient("https://mainnet.accumulatenetwork.io/v3", nil)
if err != nil {
    // Handle error
}

// Get network status
status, err := client.NetworkStatus(context.Background(), nil)
if err != nil {
    // Handle error
}

// Query an account
record, err := client.Query(context.Background(), accountUrl, &api.AccountQuery{})
if err != nil {
    // Handle error
}
```

## API V2

The V2 API provides a stable interface for interacting with the Accumulate network.

### Main Endpoints

#### Account Operations

| Endpoint | Description | Parameters |
|----------|-------------|------------|
| `get-account` | Get account information | `url` |
| `get-data-entry` | Get data entry | `url`, `entryHash` |
| `query-data` | Query data entries | `url`, `start`, `count` |
| `get-directory` | Get directory entries | `url`, `start`, `count` |

#### Transaction Operations

| Endpoint | Description | Parameters |
|----------|-------------|------------|
| `get-tx` | Get transaction by hash | `txid` |
| `get-pending-txs` | Get pending transactions | `url`, `start`, `count` |
| `create-tx` | Create a transaction | `type`, `from`, `to`, `amount` |
| `execute-tx` | Execute a transaction | `tx`, `sig` |
| `query-tx` | Query transactions | `url`, `start`, `count` |

#### Network Operations

| Endpoint | Description | Parameters |
|----------|-------------|------------|
| `get-version` | Get node version | - |
| `get-status` | Get node status | - |

### Example Usage (Go)

```go
// Create a client
client, err := client.New("https://mainnet.accumulatenetwork.io")
if err != nil {
    // Handle error
}

// Get account information
account, err := client.GetAccount(context.Background(), &v2.GetAccountRequest{
    Url: "acc://example.acme",
})
if err != nil {
    // Handle error
}
```

## API V1 (Legacy)

The V1 API is the original API for interacting with the Accumulate network. It provides basic functionality for account and transaction operations.

### Main Endpoints

#### Account Operations

| Endpoint | Description | Parameters |
|----------|-------------|------------|
| `get-account` | Get account information | `url` |
| `get-data` | Get account data | `url` |

#### Transaction Operations

| Endpoint | Description | Parameters |
|----------|-------------|------------|
| `get-tx` | Get transaction by hash | `txid` |
| `create-tx` | Create a transaction | `type`, `from`, `to`, `amount` |
| `sign-tx` | Sign a transaction | `tx`, `key` |
| `submit-tx` | Submit a transaction | `tx`, `sig` |

## Network Initialization

One of the key operations when working with the Accumulate API is initializing a connection to the network. This involves discovering and connecting to peers, validating multiaddresses, and establishing a P2P node.

### Challenges with Network Initialization

Our implementation experience revealed several important challenges with network initialization:

1. **Missing P2P Component in Multiaddresses**: The nodes discovered via JSON-RPC don't have the required P2P component in their multiaddresses. Valid P2P multiaddresses must have:
   - Network component (e.g., `/ip4`, `/ip6`, `/dns`)  
   - Transport component (e.g., `/tcp`, `/udp`)
   - P2P component (e.g., `/p2p/QmPeerID`)

2. **Empty Service Arguments**: The NodeInfo API sometimes returns services with empty arguments, which are not valid multiaddresses and must be filtered out.

3. **Peer ID Mapping**: To enhance multiaddresses with the P2P component, you need to map hosts to peer IDs, which requires additional processing.

4. **Fallback Mechanism Needed**: Due to the issues above, a fallback mechanism with known good bootstrap peers is essential for reliable network initialization.

### Enhanced Network Initialization Process

Based on our implementation, here's a more robust network initialization process:

```go
// Create API client
client, err := api.NewClient("https://mainnet.accumulatenetwork.io/v3", nil)
if err != nil {
    return fmt.Errorf("failed to create API client: %w", err)
}

// Get network status to discover nodes
ns, err := client.NetworkStatus(ctx, nil)
if err != nil {
    return fmt.Errorf("failed to get network status: %w", err)
}

// Prepare collections for peer addresses and peer IDs
var peerAddrs []multiaddr.Multiaddr
peerIDs := make(map[string]string)

// For each partition, get node info and extract peer addresses
for _, p := range ns.Partitions {
    ni, err := client.NodeInfo(ctx, p.ID, nil)
    if err != nil {
        continue
    }
    
    // Process services to extract addresses
    for _, svc := range ni.Services {
        // Skip services with empty arguments
        if svc.Argument == "" {
            continue
        }
        
        // Try to parse the argument as a multiaddress
        addr := svc.Argument
        maddr, err := multiaddr.NewMultiaddr(addr)
        if err != nil {
            continue
        }
        
        // Extract host from multiaddress for peer ID mapping
        host := extractHostFromMultiaddr(maddr)
        if host != "" {
            peerIDs[host] = ni.PeerID.String()
        }
        
        peerAddrs = append(peerAddrs, maddr)
    }
}

// Validate and enhance bootstrap peers
validPeers := getValidBootstrapPeers(peerAddrs, peerIDs)

// If no valid peers were found, use fallback bootstrap peers
if len(validPeers) == 0 {
    // Add known good bootstrap peers for the network
    fallbackPeers := []string{
        "/ip4/144.76.105.23/tcp/16593/p2p/12D3KooWEYv4x3K8PJjDZfNvgRJqK3ZRYJwz9iLrHnQnUWL4opZF",
        "/ip4/95.216.2.219/tcp/16593/p2p/12D3KooWMJPUTkqpyxkBJrpQvww4K3pCoBRvPgpJjUmJJGbtJJzJ",
        "/ip4/65.108.0.59/tcp/16593/p2p/12D3KooWFtf3yW5U9Q8LLgLKUxZ6aDJF3Db4FaKcfN2KrHbCnz7N",
    }
    
    for _, peer := range fallbackPeers {
        maddr, err := multiaddr.NewMultiaddr(peer)
        if err == nil {
            validPeers = append(validPeers, maddr)
        }
    }
}

// Initialize P2P node with validated peers
node, err := p2p.New(p2p.Options{
    Network:              network,
    BootstrapPeers:       validPeers,
    EnablePeerTracker:    true,
})
```

### Key Utility Functions

These utility functions are essential for proper network initialization:

```go
// validateP2PMultiaddress checks if a multiaddress is a valid P2P address
func validateP2PMultiaddress(addr multiaddr.Multiaddr) bool {
    // Check if the address has a P2P component
    if !hasP2PComponent(addr) {
        return false
    }
    
    // Check if the address has a network component (ip4, ip6, dns)
    hasNetwork := false
    for _, proto := range []int{multiaddr.P_IP4, multiaddr.P_IP6, multiaddr.P_DNS} {
        if addr.Protocols()[0].Code == proto {
            hasNetwork = true
            break
        }
    }
    
    // Check if the address has a transport component (tcp, udp)
    hasTransport := false
    for _, proto := range []int{multiaddr.P_TCP, multiaddr.P_UDP} {
        if addr.Protocols()[1].Code == proto {
            hasTransport = true
            break
        }
    }
    
    return hasNetwork && hasTransport
}

// hasP2PComponent checks if a multiaddress has a P2P component
func hasP2PComponent(addr multiaddr.Multiaddr) bool {
    return addr.Protocols()[len(addr.Protocols())-1].Code == multiaddr.P_P2P
}

// addP2PComponentToAddress adds a P2P component to a multiaddress
func addP2PComponentToAddress(addr multiaddr.Multiaddr, peerID string) (multiaddr.Multiaddr, error) {
    p2pComponent, err := multiaddr.NewComponent("p2p", peerID)
    if err != nil {
        return nil, err
    }
    return addr.Encapsulate(p2pComponent), nil
}

// extractHostFromMultiaddr extracts the host from a multiaddress
func extractHostFromMultiaddr(addr multiaddr.Multiaddr) string {
    host := ""
    
    // Check for IP4 address
    if ip4, err := addr.ValueForProtocol(multiaddr.P_IP4); err == nil {
        host = ip4
    }
    
    // Check for IP6 address
    if host == "" {
        if ip6, err := addr.ValueForProtocol(multiaddr.P_IP6); err == nil {
            host = ip6
        }
    }
    
    // Check for DNS address
    if host == "" {
        if dns, err := addr.ValueForProtocol(multiaddr.P_DNS); err == nil {
            host = dns
        }
    }
    
    return host
}
```

For a complete implementation of network initialization, see the [Network Initialization Guide](../debugging/network_initialization_guide.md) and our [network-init-clone](../debugging/network-init-clone/) implementation.

## Tendermint/CometBFT APIs

Accumulate uses Tendermint (now known as CometBFT) as its consensus engine. Understanding the Tendermint/CometBFT APIs is essential for certain network operations, especially when working with peer discovery, consensus status, and transaction submission.

### Key Tendermint/CometBFT Endpoints

| Endpoint | Description | Use Case |
|----------|-------------|----------|
| `/status` | Get node status information | Network health monitoring, block height synchronization |
| `/net_info` | Get network information including peers | Peer discovery, network topology mapping |
| `/block` | Get block data at a specific height | Block exploration, transaction verification |
| `/broadcast_tx_sync` | Submit a transaction synchronously | Transaction submission with immediate response |
| `/broadcast_tx_async` | Submit a transaction asynchronously | High-throughput transaction submission |
| `/broadcast_tx_commit` | Submit a transaction and wait for commit | Transaction submission with confirmation |
| `/abci_query` | Query the application state | Custom queries to the Accumulate state machine |
| `/validators` | Get validator set at a specific height | Consensus participant discovery |

### Tendermint/CometBFT Client Usage

Here's how to create and use a Tendermint/CometBFT client in Accumulate:

```go
import (
    "context"
    "fmt"
    
    "github.com/cometbft/cometbft/rpc/client/http"
    coretypes "github.com/cometbft/cometbft/rpc/core/types"
)

func workWithTendermint(ctx context.Context, nodeAddress string) error {
    // Create a Tendermint HTTP client
    // The RPC port is typically base port + PortOffsetTendermintRpc (16592 for primary nodes)
    client, err := http.New(nodeAddress, "/websocket")
    if err != nil {
        return fmt.Errorf("failed to create Tendermint client: %w", err)
    }
    
    // Get node status
    status, err := client.Status(ctx)
    if err != nil {
        return fmt.Errorf("failed to get node status: %w", err)
    }
    fmt.Printf("Node ID: %s\n", status.NodeInfo.ID)
    fmt.Printf("Latest block height: %d\n", status.SyncInfo.LatestBlockHeight)
    
    // Get network info (peers)
    netInfo, err := client.NetInfo(ctx)
    if err != nil {
        return fmt.Errorf("failed to get network info: %w", err)
    }
    fmt.Printf("Number of peers: %d\n", netInfo.NPeers)
    
    // Get validators
    validators, err := client.Validators(ctx, nil, nil, nil)
    if err != nil {
        return fmt.Errorf("failed to get validators: %w", err)
    }
    fmt.Printf("Number of validators: %d\n", validators.Total)
    
    return nil
}
```

### Port Configuration

Tendermint/CometBFT services in Accumulate use specific port offsets from the base port:

- **Tendermint P2P**: Base port + `PortOffsetTendermintP2P` (typically +0)
- **Tendermint RPC**: Base port + `PortOffsetTendermintRpc` (typically +1)

For example, if a node's base port is 16592:
- Tendermint P2P would be on port 16592
- Tendermint RPC would be on port 16593

### Integration with Accumulate APIs

In many cases, you'll need to use both Accumulate APIs and Tendermint/CometBFT APIs together:

```go
func queryWithBothAPIs(ctx context.Context, accEndpoint, tendermintEndpoint string) error {
    // Create Accumulate client
    accClient, err := api.NewClient(accEndpoint, nil)
    if err != nil {
        return fmt.Errorf("failed to create Accumulate client: %w", err)
    }
    
    // Create Tendermint client
    tmClient, err := http.New(tendermintEndpoint, "/websocket")
    if err != nil {
        return fmt.Errorf("failed to create Tendermint client: %w", err)
    }
    
    // Use Accumulate API for account information
    query := &api.GeneralQuery{QueryType: api.QueryTypeChain, ChainID: "acc://myaccount"}
    result, err := accClient.Query(ctx, query)
    if err != nil {
        return fmt.Errorf("failed to query account: %w", err)
    }
    
    // Use Tendermint API for consensus information
    status, err := tmClient.Status(ctx)
    if err != nil {
        return fmt.Errorf("failed to get node status: %w", err)
    }
    
    fmt.Printf("Account data: %+v\n", result)
    fmt.Printf("Consensus height: %d\n", status.SyncInfo.LatestBlockHeight)
    
    return nil
}
```

### When to Use Tendermint/CometBFT APIs

1. **Peer Discovery**: When you need detailed information about the network topology and connected peers
2. **Consensus Monitoring**: When you need to track block production, validator performance, or consensus health
3. **Low-Level Transaction Control**: When you need more control over transaction submission and confirmation
4. **Custom ABCI Queries**: For specialized queries that aren't exposed through the Accumulate API
5. **Network Diagnostics**: When troubleshooting network connectivity or consensus issues

## URL Formats

Accumulate uses two distinct URL formats that are important to understand:

### API Endpoint URLs

API endpoint URLs are used to connect to the Accumulate API services and follow this format:

```
https://[network-domain]/v[version-number]
```

Examples:
- `https://mainnet.accumulatenetwork.io/v3`
- `https://testnet.accumulatenetwork.io/v2`
- `http://localhost:8080/v3`

Network-specific port configurations:
- **Mainnet**: Base port 16592
- **Testnet**: Base port 16692
- **Devnet**: Base port 26492

### Accumulate Account URLs

Accumulate account URLs identify accounts and resources within the Accumulate network and follow this format:

```
acc://[authority]/[account-type]/[sub-account]
```

Components:
- **Scheme**: Always `acc://`
- **Authority**: The identity (ADI) or token issuer
- **Account Type**: The type of account (e.g., `tokens`, `data`, `staking`)
- **Sub-account**: Optional sub-account path

Examples:
- `acc://acme` - An ADI (Accumulate Digital Identifier)
- `acc://acme/tokens` - A token account within the ACME ADI
- `acc://acme/tokens/staking` - A sub-account of the tokens account
- `acc://acme/data` - A data account within the ACME ADI

#### URL Parsing and Manipulation

The Accumulate SDK provides utilities for working with URLs:

```go
// Parse a URL string
url, err := url.Parse("acc://acme/tokens")
if err != nil {
    // Handle error
}

// Create a URL with a different path
newUrl := url.WithPath("data")

// Get a string representation
urlString := url.String() // "acc://acme/data"
```

## Event Subscription

The EventService interface allows clients to subscribe to events from the Accumulate network. This is useful for monitoring account changes, block production, and other network events in real-time.

### Event Types

Accumulate supports the following event types:

| Event Type | Description | Use Case |
|------------|-------------|----------|
| `Error` | Error notifications | Monitoring for system errors |
| `Block` | Block creation events | Tracking new blocks and transactions |
| `Globals` | Global parameter updates | Monitoring network parameter changes |

### SubscribeOptions

When subscribing to events, you can specify the following options:

| Option | Description | Required |
|--------|-------------|----------|
| `Partition` | Specific partition to monitor | Optional |
| `Account` | Specific account to monitor | Optional |

If no options are specified, you'll receive events from all partitions and accounts.

### Example Usage

```go
// Create a client
client, err := api.NewClient("https://mainnet.accumulatenetwork.io/v3", nil)
if err != nil {
    // Handle error
}

// Subscribe to events
ctx, cancel := context.WithCancel(context.Background())
defer cancel() // Important: cancel the context to prevent goroutine leaks

// Subscribe to all events for a specific account
events, err := client.Subscribe(ctx, api.SubscribeOptions{
    Account: url.MustParse("acc://acme/tokens"),
})
if err != nil {
    // Handle error
}

// Process events as they arrive
for event := range events {
    switch event.EventType() {
    case api.EventTypeBlock:
        block := event.(*api.BlockEvent)
        fmt.Printf("New block: %d\n", block.Height)
    case api.EventTypeError:
        errEvent := event.(*api.ErrorEvent)
        fmt.Printf("Error: %s\n", errEvent.Error)
    case api.EventTypeGlobals:
        globals := event.(*api.GlobalsEvent)
        fmt.Printf("Globals updated: %+v\n", globals.Globals)
    }
}
```

### Best Practices for Event Handling

1. **Always cancel the context**: The subscription will continue until the context is canceled. Failing to cancel the context will leak goroutines.

2. **Handle backpressure**: If your event processing is slow, the event channel may fill up. Consider using a buffered channel or a separate goroutine for processing.

3. **Reconnection logic**: Implement reconnection logic to handle network interruptions.

4. **Filtering**: Use the subscription options to filter events and reduce network traffic.

## Common Parameters

### Transaction Envelope

All transaction-related API calls use a common envelope structure that includes:

- **Transaction data**: The specific transaction payload
- **Signatures**: One or more signatures authorizing the transaction
- **Metadata**: Additional information like timestamps and fees

## Best Practices

1. **Use the Latest API**: Prefer V3 API for new development
2. **Handle Errors Properly**: Check error codes and messages
3. **Implement Retries**: Network operations may fail temporarily
4. **Validate Transactions**: Use the validate endpoint before submitting
5. **Monitor Network Status**: Check network status before critical operations

## Conclusion

This document provides a high-level overview of the Accumulate API versions. For detailed information on specific endpoints and parameters, refer to the API documentation or explore the codebase.

As we continue development, this document will be refined and expanded with more detailed information on specific API calls and use cases.
