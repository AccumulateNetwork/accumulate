# API v2 Client Reference

**Package**: `pkg/client/api/v2`  
**Transport**: HTTP/JSON-RPC using `github.com/AccumulateNetwork/jsonrpc2/v15`  
**Timeout**: 15 seconds  
**Best For**: Legacy applications, v2 API compatibility

## Overview

The API v2 client provides access to the Accumulate v2 API through a typed SDK interface. It uses JSON-RPC over HTTP and supports testnet, mainnet, and local endpoints.

## Client Creation

```go
import "gitlab.com/AccumulateNetwork/accumulate/pkg/client/api/v2"

// Create client with endpoint URL
client := v2.New("https://mainnet.accumulatenetwork.io")

// Supported endpoints
testnetClient := v2.New("https://testnet.accumulatenetwork.io")
mainnetClient := v2.New("https://mainnet.accumulatenetwork.io") 
localClient := v2.New("http://localhost:26657")
```

## Key Features

- **Generated SDK**: Provides typed methods for all v2 API endpoints
- **Network Support**: Works with testnet, mainnet, and local development networks
- **JSON-RPC Transport**: Uses reliable HTTP-based JSON-RPC protocol
- **Error Handling**: Structured error responses from the API
- **Timeout Management**: 15-second default timeout for all requests

## Client Configuration

```go
// The v2 client uses internal configuration
// Timeout: 15 seconds (hardcoded)
// Transport: HTTP/JSON-RPC
// Content-Type: application/json
```

## Usage Patterns

### Basic Query Operations
```go
ctx := context.Background()

// Example v2 API patterns (actual methods depend on generated SDK)
// Note: Specific method signatures should be verified from the generated code

// Account queries
account, err := client.QueryAccount(ctx, "acc://example.acme")
if err != nil {
    log.Printf("Query failed: %v", err)
    return
}

// Transaction queries  
tx, err := client.QueryTransaction(ctx, txHash)
if err != nil {
    log.Printf("Transaction query failed: %v", err)
    return
}
```

### Error Handling
```go
result, err := client.SomeOperation(ctx, params)
if err != nil {
    // Handle JSON-RPC errors
    if rpcErr, ok := err.(*jsonrpc2.Error); ok {
        log.Printf("RPC Error: Code=%d, Message=%s", rpcErr.Code, rpcErr.Message)
    } else {
        log.Printf("Network Error: %v", err)
    }
    return
}
```

## Network Endpoints

| Network | URL | Description |
|---------|-----|-------------|
| Local | `http://localhost:26657` | Local development node |
| Testnet | `https://testnet.accumulatenetwork.io` | Testnet network |
| Mainnet | `https://mainnet.accumulatenetwork.io` | Production mainnet |

## Migration Considerations

### From v2 to v3
When migrating from API v2 to v3, consider:

1. **API Changes**: v3 has different service interfaces and method signatures
2. **Transport**: v3 supports both JSON-RPC and WebSocket
3. **Features**: v3 provides more advanced features and better error handling
4. **Performance**: v3 may offer better performance characteristics

### Example Migration
```go
// v2 Client
v2Client := v2.New("https://mainnet.accumulatenetwork.io")
account, err := v2Client.QueryAccount(ctx, "acc://example.acme")

// v3 Client (recommended for new applications)
httpClient := &http.Client{Timeout: 15 * time.Second}
v3Client := jsonrpc.NewClient("https://mainnet.accumulatenetwork.io/v3", httpClient)
account, err := v3Client.QueryService().QueryAccount(ctx, &api.AccountQuery{
    Url: "acc://example.acme",
})
```

## Limitations

- **v2 API Only**: Limited to v2 API functionality
- **No Real-time**: No support for real-time events or streaming
- **Fixed Timeout**: 15-second timeout cannot be customized
- **Legacy Status**: Consider migrating to v3 for new applications

## Best Practices

1. **Use for Legacy Support**: Primarily for maintaining existing v2-based applications
2. **Consider v3 Migration**: Evaluate migrating to v3 for better features and performance
3. **Error Handling**: Always handle both network and JSON-RPC errors
4. **Context Usage**: Always pass context for proper cancellation support
5. **Endpoint Selection**: Use appropriate endpoint for your target network

## Troubleshooting

### Common Issues

1. **Connection Timeouts**: 15-second timeout may be too short for some operations
2. **Network Errors**: Check endpoint URL and network connectivity
3. **API Compatibility**: Ensure server supports v2 API endpoints

### Debug Tips

```go
// Enable detailed logging for debugging
import "log"

result, err := client.SomeOperation(ctx, params)
if err != nil {
    log.Printf("Operation failed: %v", err)
    log.Printf("Parameters: %+v", params)
}
```

---

*For new applications, consider using the API v3 JSON-RPC client which provides more features, better performance, and active development support.*
