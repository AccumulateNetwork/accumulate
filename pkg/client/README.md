# Accumulate Go SDK

A high-level Go SDK for interacting with Accumulate networks. This package provides a unified, idiomatic Go interface for all Accumulate operations.

## Installation

```bash
go get gitlab.com/accumulatenetwork/accumulate/pkg/client
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "gitlab.com/accumulatenetwork/accumulate/pkg/client"
)

func main() {
    // Connect to testnet
    c, err := client.NewTestnet()
    if err != nil {
        log.Fatal(err)
    }
    
    // Query an account
    account, err := c.GetAccount(context.Background(), "acc://ACME")
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Account: %+v\n", account)
}
```

## Network Options

### Predefined Networks

```go
// Mainnet
client, err := client.NewMainnet()

// Testnet (Kermit)
client, err := client.NewTestnet()

// Local development
client, err := client.NewLocal("http://localhost:8080/v3")

// Development network
client, err := client.NewDevnet("http://devnet:8080/v3")
```

### Custom Configuration

```go
config := &client.Config{
    Endpoint: "https://custom.accumulate.io/v3",
    Network:  client.NetworkCustom,
    Timeout:  30 * time.Second,
    Debug:    true,
}
client, err := client.New(config)
```

## Available Methods

### Query Methods

#### GetAccount
Query account information by URL.

```go
account, err := client.GetAccount(ctx, "acc://mytoken.acme")
```

**Curl equivalent:**
```bash
curl -X POST http://localhost:8080/v3 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "query",
    "params": {
      "scope": "acc://mytoken.acme",
      "query": {}
    },
    "id": 1
  }'
```

#### GetTransaction
Query a transaction by its ID.

```go
tx, err := client.GetTransaction(ctx, "0123456789abcdef...")
```

**Curl equivalent:**
```bash
curl -X POST http://localhost:8080/v3 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "query",
    "params": {
      "scope": "acc://dn.acme/network",
      "query": {
        "type": "transaction-hash",
        "hash": "0123456789abcdef..."
      }
    },
    "id": 1
  }'
```

#### GetChainEntry
Query a specific entry from an account's chain.

```go
entry, err := client.GetChainEntry(ctx, "acc://mytoken.acme", "main", 0)
```

#### GetDataEntry
Query a specific entry from an account's data chain.

```go
entry, err := client.GetDataEntry(ctx, "acc://mydata.acme", 0)
```

#### GetDirectory
Query the directory entries of an account.

```go
entries, err := client.GetDirectory(ctx, "acc://myadi.acme", 0, 10)
```

### Network Information

#### GetNodeInfo
Get information about the network node.

```go
info, err := client.GetNodeInfo(ctx)
fmt.Printf("Node: %s, Network: %s\n", info.PeerID, info.Network)
```

#### GetNetworkStatus
Get the status of the network.

```go
status, err := client.GetNetworkStatus(ctx)
fmt.Printf("Network: %s\n", status.Network)
```

#### GetConsensusStatus
Get the consensus status (validator nodes only).

```go
status, err := client.GetConsensusStatus(ctx)
fmt.Printf("Consensus OK: %v\n", status.Ok)
```

#### GetMetrics
Get network metrics.

```go
metrics, err := client.GetMetrics(ctx, "Directory", "1h")
fmt.Printf("TPS: %v\n", metrics.TPS)
```

#### FindService
Find nodes providing a specific service.

```go
nodes, err := client.FindService(ctx, v3.ServiceTypeQuery)
```

#### ListSnapshots
List available snapshots.

```go
snapshots, err := client.ListSnapshots(ctx)
```

## Testing

### Unit Tests

Run unit tests with:

```bash
go test ./pkg/client/...
```

### Integration Tests

To run tests against a local devnet:

```bash
RUN_DEVNET_TESTS=1 go test ./pkg/client/...
```

To run tests against a specific endpoint:

```bash
ACCUMULATE_ENDPOINT=https://testnet.accumulate.defidevs.io/v3 go test ./pkg/client/...
```

## Implementation Status

### Completed Methods
- ✅ GetAccount - Query account information
- ✅ GetTransaction - Query transaction by ID
- ✅ GetChainEntry - Query chain entries
- ✅ GetDataEntry - Query data entries
- ✅ GetDirectory - List directory entries
- ✅ GetNodeInfo - Get node information
- ✅ GetNetworkStatus - Get network status
- ✅ GetConsensusStatus - Get consensus status
- ✅ GetMetrics - Get network metrics
- ✅ FindService - Find service nodes
- ✅ ListSnapshots - List snapshots

### TODO Methods
- [ ] GetBlock - Query block information
- [ ] GetPending - Query pending transactions
- [ ] Submit - Submit transactions
- [ ] Validate - Validate transactions
- [ ] Faucet - Request testnet tokens
- [ ] Subscribe - Event subscriptions
- [ ] V2 API compatibility methods
- [ ] Ethereum-compatible methods
- [ ] Transaction building helpers

## Architecture

The SDK is built on top of the existing Accumulate API implementations:

- **V3 API**: Primary API using `pkg/api/v3` (JSON-RPC, WebSocket, Message, REST)
- **V2 API**: Legacy support via `internal/api/v2` 
- **Ethereum API**: Web3 compatibility via `pkg/api/ethereum`
- **Light Client**: Experimental support via `exp/light`

The client wraps these implementations to provide a unified, high-level interface.

## Contributing

See the [design document](../../docs/client/GO_SDK_DESIGN.md) for architecture details and contribution guidelines.

## License

MIT License - see LICENSE file for details.