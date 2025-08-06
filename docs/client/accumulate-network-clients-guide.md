# Accumulate Network Clients - Complete Guide

This comprehensive guide covers all network client implementations available in the Accumulate codebase, their features, use cases, and usage examples.

## Table of Contents

- [Client Overview](#client-overview)
- [Light Client Package](#light-client-package)
- [API v2 Client](#api-v2-client)
- [API v3 JSON-RPC Client](#api-v3-json-rpc-client)
- [API v3 WebSocket Client](#api-v3-websocket-client)
- [Client Comparison](#client-comparison)
- [Usage Recommendations](#usage-recommendations)
- [Migration Guide](#migration-guide)

## Client Overview

The Accumulate network provides four main client implementations, each designed for different use cases:

| Client | Package | Transport | Features | Use Case |
|--------|---------|-----------|----------|----------|
| **Light Client** | `pkg/lightclient` | HTTP/JSON-RPC 2.0 | Simple, lightweight | Basic queries, staking |
| **API v2 Client** | `pkg/client/api/v2` | HTTP/JSON-RPC | Full v2 API, typed methods | Legacy applications |
| **API v3 JSON-RPC** | `pkg/api/v3/jsonrpc` | HTTP/JSON-RPC | Full v3 API, all services | Modern applications |
| **API v3 WebSocket** | `pkg/api/v3/websocket` | WebSocket | Real-time, streaming | Event-driven apps |

## Light Client Package

**Location**: `pkg/lightclient`  
**Best For**: Simple account queries, staking operations, lightweight applications

### Features

- ✅ Account querying (ADIs, token accounts, data accounts, key books)
- ✅ Staking registry and account operations
- ✅ Network operators keybook access
- ✅ Batch operations
- ✅ Built-in URL validation and correction
- ✅ Multiple network support (local, testnet, mainnet)
- ✅ Cryptographic proof support

### Basic Usage

```go
package main

import (
    "context"
    "log"
    "gitlab.com/AccumulateNetwork/accumulate/pkg/lightclient"
)

func main() {
    // Create client with server URL shortcuts
    client, err := lightclient.NewClient("mainnet")
    if err != nil {
        log.Fatal(err)
    }

    ctx := context.Background()
    
    // Query any account
    resp, err := client.Query(ctx, "acc://example.acme")
    if err != nil {
        log.Fatal(err)
    }
    
    accountType, _ := resp.GetType()
    data, _ := resp.GetData()
    log.Printf("Account Type: %s", accountType)
}
```

### Server URL Shortcuts

```go
// Supported shortcuts
client, _ := lightclient.NewClient("local")        // http://127.0.1.1:26660
client, _ := lightclient.NewClient("testnet")      // https://testnet.accumulatenetwork.io
client, _ := lightclient.NewClient("mainnet")      // http://apollo-mainnet.accumulate.defidevs.io:16595
client, _ := lightclient.NewClient("mainnet-ssl")  // https://mainnet.accumulatenetwork.io

// Custom URL
client, _ := lightclient.NewClient("https://custom.accumulate.network/v3")
```

### Staking Operations

```go
// Get staking registry
registry, err := client.GetStakingRegistry(ctx)
if err != nil {
    log.Fatal(err)
}

// Get staking accounts with details
accounts, err := client.GetStakingAccountsWithTotal(ctx)
if err != nil {
    log.Fatal(err)
}

for _, account := range accounts {
    log.Printf("Account: %s, Balance: %d, Authorities: %v", 
        account.TokenURL, account.Balance, account.Authorities)
}
```

### Network Operations

```go
// Get network operators keybook
operators, err := client.GetNetworkOperators(ctx)
if err != nil {
    log.Fatal(err)
}

// Batch account queries
urls := []string{"acc://example.acme", "acc://another.acme"}
accounts, err := client.GetAccounts(ctx, urls)
if err != nil {
    log.Fatal(err)
}
```

## API v2 Client

**Location**: `pkg/client/api/v2`  
**Best For**: Legacy applications, v2 API compatibility

### Features

- ✅ Full v2 API support
- ✅ Typed SDK methods
- ✅ Multiple network endpoints
- ✅ 15-second timeout
- ✅ JSON-RPC over HTTP using `jsonrpc2/v15`

### Basic Usage

```go
package main

import (
    "context"
    "gitlab.com/AccumulateNetwork/accumulate/pkg/client/api/v2"
)

func main() {
    // Create v2 client
    client := v2.New("https://mainnet.accumulatenetwork.io")
    
    ctx := context.Background()
    
    // Use v2 API methods (examples based on typical v2 patterns)
    // Note: Specific methods depend on v2 API implementation
}
```

### Network Endpoints

```go
// Supported endpoints
client := v2.New("https://testnet.accumulatenetwork.io")   // Testnet
client := v2.New("https://mainnet.accumulatenetwork.io")   // Mainnet  
client := v2.New("http://localhost:26657")                 // Local
```

## API v3 JSON-RPC Client

**Location**: `pkg/api/v3/jsonrpc`  
**Best For**: Modern applications, full API access, production systems

### Features

- ✅ Complete v3 API support
- ✅ All service interfaces (NodeService, ConsensusService, NetworkService, etc.)
- ✅ Type-safe request/response handling
- ✅ Private API access support
- ✅ Structured error handling
- ✅ 15-second timeout
- ✅ JSON-RPC using `jsonrpc2/v15`

### Basic Usage

```go
package main

import (
    "context"
    "net/http"
    "time"
    "gitlab.com/AccumulateNetwork/accumulate/pkg/api/v3/jsonrpc"
)

func main() {
    // Create HTTP client with timeout
    httpClient := &http.Client{
        Timeout: 15 * time.Second,
    }
    
    // Create v3 JSON-RPC client
    client := jsonrpc.NewClient("https://mainnet.accumulatenetwork.io/v3", httpClient)
    
    ctx := context.Background()
    
    // Use service interfaces
    nodeStatus, err := client.NodeService().NodeStatus(ctx, &api.NodeStatusRequest{
        NodeID: "node-1",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Node Status: %+v", nodeStatus)
}
```

### Available Services

```go
// All v3 API services are available
client.NodeService()        // Node operations
client.ConsensusService()   // Consensus operations  
client.NetworkService()     // Network status and info
client.QueryService()       // Account and transaction queries
client.SubmitService()      // Transaction submission
client.ValidateService()    // Transaction validation
client.FaucetService()      // Testnet faucet (if available)
client.MetricsService()     // Performance metrics
```

### Advanced Usage

```go
// Network status
networkStatus, err := client.NetworkService().NetworkStatus(ctx, &api.NetworkStatusRequest{})
if err != nil {
    log.Fatal(err)
}

// Query account
account, err := client.QueryService().QueryAccount(ctx, &api.AccountQuery{
    Url: "acc://example.acme",
})
if err != nil {
    log.Fatal(err)
}

// Submit transaction
result, err := client.SubmitService().Submit(ctx, &api.Submission{
    // Transaction data
})
if err != nil {
    log.Fatal(err)
}
```

## API v3 WebSocket Client

**Location**: `pkg/api/v3/websocket`  
**Best For**: Real-time applications, event subscriptions, streaming data

### Features

- ✅ WebSocket transport for real-time communication
- ✅ Event subscriptions and streaming
- ✅ Concurrent sub-streams support
- ✅ Event-driven architecture
- ✅ Full v3 API interface implementations
- ✅ Uses `gorilla/websocket`

### Basic Usage

```go
package main

import (
    "context"
    "log"
    "gitlab.com/AccumulateNetwork/accumulate/pkg/api/v3/websocket"
)

func main() {
    // Create WebSocket client
    client, err := websocket.NewClient("wss://mainnet.accumulatenetwork.io/v3/ws")
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()
    
    ctx := context.Background()
    
    // Use same service interfaces as JSON-RPC client
    nodeStatus, err := client.NodeService().NodeStatus(ctx, &api.NodeStatusRequest{})
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Node Status: %+v", nodeStatus)
}
```

### Event Subscriptions

```go
// Subscribe to events (example pattern)
subscription, err := client.Subscribe(ctx, &api.EventSubscription{
    Filter: &api.EventFilter{
        // Event filter criteria
    },
})
if err != nil {
    log.Fatal(err)
}

// Handle events
for event := range subscription.Events() {
    log.Printf("Received event: %+v", event)
}
```

### Streaming Operations

```go
// Stream real-time data
stream, err := client.StreamTransactions(ctx, &api.TransactionStreamRequest{
    // Stream parameters
})
if err != nil {
    log.Fatal(err)
}

for tx := range stream.Transactions() {
    log.Printf("New transaction: %+v", tx)
}
```

## Client Comparison

### Performance Characteristics

| Client | Latency | Throughput | Memory Usage | CPU Usage |
|--------|---------|------------|--------------|-----------|
| Light Client | Low | Medium | Low | Low |
| API v2 Client | Low | Medium | Medium | Medium |
| API v3 JSON-RPC | Low | High | Medium | Medium |
| API v3 WebSocket | Very Low | Very High | High | High |

### Feature Matrix

| Feature | Light Client | API v2 | API v3 JSON-RPC | API v3 WebSocket |
|---------|--------------|--------|------------------|------------------|
| Account Queries | ✅ | ✅ | ✅ | ✅ |
| Transaction Submission | ❌ | ✅ | ✅ | ✅ |
| Real-time Events | ❌ | ❌ | ❌ | ✅ |
| Batch Operations | ✅ | ✅ | ✅ | ✅ |
| Staking Operations | ✅ | ❌ | ✅ | ✅ |
| Network Operations | ✅ | ✅ | ✅ | ✅ |
| Private API Access | ❌ | ❌ | ✅ | ✅ |
| Streaming Data | ❌ | ❌ | ❌ | ✅ |
| Offline Capability | ❌ | ❌ | ❌ | ❌ |

## Usage Recommendations

### Choose Light Client When:
- ✅ Building simple applications
- ✅ Only need account queries and staking operations  
- ✅ Want minimal dependencies
- ✅ Developing proof-of-concept applications
- ✅ Need built-in URL validation

### Choose API v2 Client When:
- ✅ Maintaining legacy applications
- ✅ Need v2 API compatibility
- ✅ Migrating from older Accumulate versions
- ✅ Working with existing v2-based tools

### Choose API v3 JSON-RPC Client When:
- ✅ Building production applications
- ✅ Need full API access
- ✅ Want type-safe operations
- ✅ Require advanced error handling
- ✅ Building modern Accumulate applications
- ✅ Need private API access

### Choose API v3 WebSocket Client When:
- ✅ Building real-time applications
- ✅ Need event subscriptions
- ✅ Require streaming data
- ✅ Building monitoring or analytics tools
- ✅ Need lowest possible latency
- ✅ Building event-driven architectures

## Migration Guide

### From Light Client to API v3 JSON-RPC

```go
// Before (Light Client)
client, _ := lightclient.NewClient("mainnet")
resp, _ := client.Query(ctx, "acc://example.acme")

// After (API v3 JSON-RPC)
httpClient := &http.Client{Timeout: 15 * time.Second}
client := jsonrpc.NewClient("https://mainnet.accumulatenetwork.io/v3", httpClient)
account, _ := client.QueryService().QueryAccount(ctx, &api.AccountQuery{
    Url: "acc://example.acme",
})
```

### From API v2 to API v3

```go
// Before (API v2)
client := v2.New("https://mainnet.accumulatenetwork.io")
// v2 specific operations

// After (API v3)
httpClient := &http.Client{Timeout: 15 * time.Second}
client := jsonrpc.NewClient("https://mainnet.accumulatenetwork.io/v3", httpClient)
// Use v3 service interfaces
```

### Adding Real-time Features

```go
// Start with JSON-RPC for basic operations
jsonClient := jsonrpc.NewClient("https://mainnet.accumulatenetwork.io/v3", httpClient)

// Add WebSocket client for real-time features
wsClient, _ := websocket.NewClient("wss://mainnet.accumulatenetwork.io/v3/ws")

// Use both clients as needed
account, _ := jsonClient.QueryService().QueryAccount(ctx, query)
subscription, _ := wsClient.Subscribe(ctx, eventFilter)
```

## Error Handling Best Practices

### Light Client
```go
resp, err := client.Query(ctx, url)
if err != nil {
    // Handle network or JSON-RPC errors
    log.Printf("Query failed: %v", err)
    return
}
```

### API v3 Clients
```go
result, err := client.QueryService().QueryAccount(ctx, query)
if err != nil {
    // Check for specific error types
    if apiErr, ok := err.(*api.Error); ok {
        log.Printf("API Error: Code=%d, Message=%s", apiErr.Code, apiErr.Message)
    } else {
        log.Printf("Network Error: %v", err)
    }
    return
}
```

### WebSocket Client
```go
subscription, err := client.Subscribe(ctx, filter)
if err != nil {
    log.Printf("Subscription failed: %v", err)
    return
}

// Handle connection errors
go func() {
    for err := range subscription.Errors() {
        log.Printf("Subscription error: %v", err)
        // Implement reconnection logic
    }
}()
```

## Configuration Examples

### Production Configuration
```go
// Production HTTP client with proper timeouts
httpClient := &http.Client{
    Timeout: 30 * time.Second,
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
    },
}

client := jsonrpc.NewClient("https://mainnet.accumulatenetwork.io/v3", httpClient)
```

### Development Configuration
```go
// Development client with shorter timeouts
httpClient := &http.Client{
    Timeout: 10 * time.Second,
}

client := jsonrpc.NewClient("http://localhost:26657/v3", httpClient)
```

### High-Performance Configuration
```go
// WebSocket client for high-performance applications
client, err := websocket.NewClient("wss://mainnet.accumulatenetwork.io/v3/ws")
if err != nil {
    log.Fatal(err)
}

// Configure connection parameters
client.SetReadDeadline(time.Now().Add(60 * time.Second))
client.SetWriteDeadline(time.Now().Add(10 * time.Second))
```

---

*This guide provides comprehensive coverage of all Accumulate network clients. Choose the client that best fits your application's requirements and use case.*
