# API v3 JSON-RPC Client Reference

**Package**: `pkg/api/v3/jsonrpc`  
**Transport**: HTTP/JSON-RPC using `github.com/AccumulateNetwork/jsonrpc2/v15`  
**Timeout**: 15 seconds (configurable)  
**Best For**: Modern applications, production systems, full API access

## Overview

The API v3 JSON-RPC client provides complete access to all Accumulate v3 API services through type-safe interfaces. It's the recommended client for modern applications requiring full API functionality.

## Client Creation

```go
import (
    "net/http"
    "time"
    "gitlab.com/AccumulateNetwork/accumulate/pkg/api/v3/jsonrpc"
)

// Create HTTP client with custom timeout
httpClient := &http.Client{
    Timeout: 15 * time.Second,
}

// Create v3 JSON-RPC client
client := jsonrpc.NewClient("https://mainnet.accumulatenetwork.io/v3", httpClient)
```

## Available Services

The client provides access to all v3 API service interfaces:

```go
// Core services
client.NodeService()        // Node status and operations
client.NetworkService()     // Network status and information
client.ConsensusService()   // Consensus operations
client.QueryService()       // Account and transaction queries
client.SubmitService()      // Transaction submission
client.ValidateService()    // Transaction validation
client.MetricsService()     // Performance metrics
client.SnapshotService()    // Snapshot management

// Additional services (if available)
client.FaucetService()      // Testnet faucet operations
// ... other services as defined in the API
```

## Service Interface Examples

### NodeService
```go
import "gitlab.com/AccumulateNetwork/accumulate/pkg/api/v3"

ctx := context.Background()

// Get node status
nodeStatus, err := client.NodeService().NodeStatus(ctx, &api.NodeStatusRequest{
    NodeID: "node-1", // Optional: specific node ID
})
if err != nil {
    log.Printf("Node status query failed: %v", err)
    return
}

log.Printf("Node Version: %s", nodeStatus.Version)
log.Printf("Node Type: %s", nodeStatus.Type)
log.Printf("Network: %s", nodeStatus.Network)
```

### NetworkService
```go
// Get network status
networkStatus, err := client.NetworkService().NetworkStatus(ctx, &api.NetworkStatusRequest{})
if err != nil {
    log.Printf("Network status query failed: %v", err)
    return
}

log.Printf("Network Name: %s", networkStatus.Network)
log.Printf("Partition Count: %d", len(networkStatus.Partitions))

// Get network information
networkInfo, err := client.NetworkService().NetworkInfo(ctx, &api.NetworkInfoRequest{})
if err != nil {
    log.Printf("Network info query failed: %v", err)
    return
}
```

### QueryService
```go
// Query account by URL
account, err := client.QueryService().QueryAccount(ctx, &api.AccountQuery{
    Url: "acc://example.acme",
    IncludeReceipt: true, // Include cryptographic proof
})
if err != nil {
    log.Printf("Account query failed: %v", err)
    return
}

log.Printf("Account Type: %s", account.Type)
log.Printf("Account Data: %+v", account.Data)

// Query transaction
tx, err := client.QueryService().QueryTransaction(ctx, &api.TransactionQuery{
    TxHash: txHash,
    IncludeReceipt: true,
})
if err != nil {
    log.Printf("Transaction query failed: %v", err)
    return
}
```

### SubmitService
```go
// Submit transaction
submission := &api.Submission{
    Transaction: envelope, // *protocol.Envelope
    Signature:   signature, // *protocol.Signature
}

result, err := client.SubmitService().Submit(ctx, submission)
if err != nil {
    log.Printf("Transaction submission failed: %v", err)
    return
}

log.Printf("Transaction Hash: %x", result.TransactionHash)
log.Printf("Status: %s", result.Status)
```

### ValidateService
```go
// Validate transaction before submission
validation, err := client.ValidateService().Validate(ctx, &api.ValidateRequest{
    Transaction: envelope,
    Signature:   signature,
})
if err != nil {
    log.Printf("Transaction validation failed: %v", err)
    return
}

if validation.Valid {
    log.Println("Transaction is valid")
} else {
    log.Printf("Transaction invalid: %s", validation.Error)
}
```

### MetricsService
```go
// Get performance metrics
metrics, err := client.MetricsService().Metrics(ctx, &api.MetricsRequest{
    Partition: "BVN0", // Optional: specific partition
    Duration:  "1h",   // Time window
})
if err != nil {
    log.Printf("Metrics query failed: %v", err)
    return
}

log.Printf("TPS: %.2f", metrics.TransactionsPerSecond)
log.Printf("Block Time: %v", metrics.AverageBlockTime)
```

### SnapshotService
```go
// List available snapshots
snapshots, err := client.SnapshotService().ListSnapshots(ctx, &api.ListSnapshotsOptions{
    Partition: "BVN0", // Optional: specific partition
    Limit:     10,     // Optional: limit results
})
if err != nil {
    log.Printf("Snapshot listing failed: %v", err)
    return
}

for _, snapshot := range snapshots {
    log.Printf("Snapshot: %s (Height: %d, Size: %d bytes)", 
        snapshot.ID, snapshot.Height, snapshot.Size)
}
```

## Advanced Configuration

### Custom HTTP Client
```go
// Production HTTP client with connection pooling
httpClient := &http.Client{
    Timeout: 30 * time.Second,
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
        DisableCompression:  false,
    },
}

client := jsonrpc.NewClient("https://mainnet.accumulatenetwork.io/v3", httpClient)
```

### Context with Timeout
```go
// Per-request timeout
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

result, err := client.QueryService().QueryAccount(ctx, query)
```

### Retry Logic
```go
func queryWithRetry(client *jsonrpc.Client, query *api.AccountQuery) (*api.AccountResponse, error) {
    maxRetries := 3
    backoff := time.Second
    
    for i := 0; i < maxRetries; i++ {
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        result, err := client.QueryService().QueryAccount(ctx, query)
        cancel()
        
        if err == nil {
            return result, nil
        }
        
        if i < maxRetries-1 {
            time.Sleep(backoff)
            backoff *= 2 // Exponential backoff
        }
    }
    
    return nil, fmt.Errorf("query failed after %d retries", maxRetries)
}
```

## Error Handling

### API Error Types
```go
result, err := client.QueryService().QueryAccount(ctx, query)
if err != nil {
    // Check for specific API error types
    if apiErr, ok := err.(*api.Error); ok {
        switch apiErr.Code {
        case api.ErrCodeNotFound:
            log.Println("Account not found")
        case api.ErrCodeInvalidRequest:
            log.Printf("Invalid request: %s", apiErr.Message)
        case api.ErrCodeInternalError:
            log.Printf("Server error: %s", apiErr.Message)
        default:
            log.Printf("API Error: Code=%d, Message=%s", apiErr.Code, apiErr.Message)
        }
    } else {
        // Network or transport error
        log.Printf("Network Error: %v", err)
    }
    return
}
```

### Structured Error Handling
```go
func handleAPIError(err error) {
    if err == nil {
        return
    }
    
    switch e := err.(type) {
    case *api.Error:
        log.Printf("API Error [%d]: %s", e.Code, e.Message)
        if e.Data != nil {
            log.Printf("Additional data: %+v", e.Data)
        }
    case *url.Error:
        log.Printf("URL Error: %v", e)
    case net.Error:
        if e.Timeout() {
            log.Println("Request timed out")
        } else {
            log.Printf("Network Error: %v", e)
        }
    default:
        log.Printf("Unknown Error: %v", e)
    }
}
```

## Batch Operations

### Multiple Queries
```go
// Query multiple accounts efficiently
urls := []string{
    "acc://example.acme",
    "acc://another.acme", 
    "acc://third.acme",
}

var wg sync.WaitGroup
results := make([]*api.AccountResponse, len(urls))
errors := make([]error, len(urls))

for i, url := range urls {
    wg.Add(1)
    go func(index int, accountUrl string) {
        defer wg.Done()
        
        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()
        
        result, err := client.QueryService().QueryAccount(ctx, &api.AccountQuery{
            Url: accountUrl,
        })
        
        results[index] = result
        errors[index] = err
    }(i, url)
}

wg.Wait()

// Process results
for i, result := range results {
    if errors[i] != nil {
        log.Printf("Query %s failed: %v", urls[i], errors[i])
        continue
    }
    log.Printf("Account %s: %+v", urls[i], result.Data)
}
```

## Network Endpoints

| Network | URL | Description |
|---------|-----|-------------|
| Local | `http://localhost:26657/v3` | Local development node |
| Testnet | `https://testnet.accumulatenetwork.io/v3` | Testnet network |
| Mainnet | `https://mainnet.accumulatenetwork.io/v3` | Production mainnet |

## Performance Optimization

### Connection Reuse
```go
// Reuse HTTP client and connections
var globalClient *jsonrpc.Client

func init() {
    httpClient := &http.Client{
        Timeout: 30 * time.Second,
        Transport: &http.Transport{
            MaxIdleConns:        100,
            MaxIdleConnsPerHost: 20,
            IdleConnTimeout:     90 * time.Second,
        },
    }
    
    globalClient = jsonrpc.NewClient("https://mainnet.accumulatenetwork.io/v3", httpClient)
}
```

### Request Optimization
```go
// Minimize data transfer
query := &api.AccountQuery{
    Url:            "acc://example.acme",
    IncludeReceipt: false, // Skip proof if not needed
    // Other optimization flags as available
}
```

## Best Practices

1. **Reuse Clients**: Create one client instance and reuse it across your application
2. **Context Management**: Always use context for cancellation and timeouts
3. **Error Handling**: Handle both API errors and network errors appropriately
4. **Connection Pooling**: Configure HTTP transport for optimal connection reuse
5. **Timeouts**: Set appropriate timeouts based on your application needs
6. **Retry Logic**: Implement retry logic for transient network errors
7. **Resource Cleanup**: Properly close resources and cancel contexts

## Troubleshooting

### Common Issues

1. **Timeout Errors**: Increase timeout or check network connectivity
2. **API Version Mismatch**: Ensure server supports v3 API
3. **Authentication**: Check if private API access requires authentication
4. **Rate Limiting**: Implement backoff for high-frequency requests

### Debug Configuration
```go
// Enable HTTP request/response logging
httpClient := &http.Client{
    Timeout: 15 * time.Second,
    Transport: &debugTransport{http.DefaultTransport},
}

type debugTransport struct {
    http.RoundTripper
}

func (t *debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
    log.Printf("Request: %s %s", req.Method, req.URL)
    resp, err := t.RoundTripper.RoundTrip(req)
    if err != nil {
        log.Printf("Error: %v", err)
    } else {
        log.Printf("Response: %s", resp.Status)
    }
    return resp, err
}
```

---

*The API v3 JSON-RPC client is the recommended choice for modern Accumulate applications requiring full API access and type safety.*
