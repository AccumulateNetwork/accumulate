# API Documentation Index

[← Back to Main Index](../INDEX.md)

## Overview
Complete API documentation for Accumulate, including JSON-RPC APIs, command references, and client libraries.

## API Versions

### API v3 (Current)
- [API v3 README](api-v3-readme.md) - Latest API version
- Implementation: [`pkg/api/v3/`](../../pkg/api/v3/)
- [JSON-RPC Client Reference](../client/api-v3-jsonrpc-client-reference.md)
- [WebSocket Client Reference](../client/api-v3-websocket-client-reference.md)

### API v2 (Legacy)
- [API v2 README](api-v2-readme.md) - Previous API version
- Implementation: [`internal/api/v2/`](../../internal/api/v2/)
- [API v2 Client Reference](../client/api-v2-client-reference.md)

## API References

### Core Documentation
- [API Interfaces Reference](api-interfaces-reference.md) - Complete API interface documentation
- [Query Types and Scope Reference](query-types-and-scope-reference.md) - Query capabilities
- [Command Implementation Map](command-implementation-map.md) - Command to implementation mapping

### Server Documentation
- [Accumulated HTTP Server](accumulated-http-server.md) - HTTP server details
- [API Server Reference](../cmd/api-server-reference.md) - Server configuration

### Command Line
- [Accumulated Daemon Commands](accumulated-daemon-commands.md) - Daemon command reference

## Client Libraries

### Official Clients
- [Accumulate Network Clients Guide](../client/accumulate-network-clients-guide.md) - Client overview
- [Database Client](../client/database-readme.md) - Database access client
- [Light Client](../client/light-client-readme.md) - Lightweight client

### Client Implementation
- Go Client: [`pkg/client/`](../../pkg/client/)
- JavaScript SDK: [External Repository](https://github.com/AccumulateNetwork/accumulate.js)

## API Endpoints

### MainNet (Cyclops)
```
https://mainnet.accumulatenetwork.io/v2
https://mainnet.accumulatenetwork.io/v3
```

### TestNet (Kermit)
```
https://testnet.accumulatenetwork.io/v2
https://testnet.accumulatenetwork.io/v3
```

### DevNet (Local)
```
http://localhost:26660/v2
http://localhost:26660/v3
```

## API Methods

### Query Methods
- `query` - Query account or transaction
- `query-directory` - Query directory entries
- `query-chain` - Query chain data
- `query-data` - Query data entries

See [Query Types Reference](query-types-and-scope-reference.md) for details.

### Transaction Methods
- `submit` - Submit transaction
- `submit-pending` - Submit pending transaction
- `execute` - Execute transaction directly

### Monitoring Methods
- `metrics` - Get node metrics
- `status` - Get node status
- `network-status` - Get network status

## Request/Response Format

### JSON-RPC 2.0 Format
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "query",
  "params": {
    "url": "acc://account.acme"
  }
}
```

### Response Format
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "type": "identity",
    "data": { ... }
  }
}
```

## Authentication

### Signature Requirements
- All transactions require valid signatures
- See [Protocol Transactions](../../protocol/transactions.md) for transaction types

### Key Management
- ED25519 key pairs
- Hierarchical key derivation
- Multi-signature support

## Error Codes

| Code | Description |
|------|-------------|
| -32700 | Parse error |
| -32600 | Invalid request |
| -32601 | Method not found |
| -32602 | Invalid params |
| -32603 | Internal error |
| -32000 | Server error |

## Rate Limiting

### Default Limits
- 100 requests per second per IP
- 1000 requests per minute per IP
- Configurable per deployment

## Data Retrieval

### MainNet Data Retrieval
- [MainNet Data Retrieval Guide](mainnet-data-retrieval-guide.md) - Accessing MainNet data

### Pagination
- Use `start` and `count` parameters
- Maximum `count` is typically 100
- Use `expand` for nested data

## WebSocket Support

### Subscription Methods
- `subscribe` - Subscribe to events
- `unsubscribe` - Unsubscribe from events

### Event Types
- `block` - New block events
- `transaction` - Transaction events
- `account` - Account update events

See [WebSocket Client Reference](../client/api-v3-websocket-client-reference.md).

## Examples

### Query Account
```bash
curl -X POST http://localhost:26660/v2 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "query",
    "params": {
      "url": "acc://myaccount.acme"
    }
  }'
```

### Submit Transaction
```bash
curl -X POST http://localhost:26660/v2 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "submit",
    "params": {
      "envelope": { ... }
    }
  }'
```

## Testing APIs

### Test Tools
- [API Server Testing](../testing/testing-api-server.md) - Testing guide
- Postman Collection: Available on request
- [`scripts/devnet/`](../../scripts/devnet/) - DevNet test scripts

### Load Testing
- [Load Test Scripts](../../scripts/devnet/devnet_load_test.sh)
- [Performance Testing](../testing/performance-tests.md)

## Implementation Details

### API v2 Implementation
- Location: [`internal/api/v2/`](../../internal/api/v2/)
- [README](../../internal/api/v2/README.md)

### API v3 Implementation  
- Location: [`pkg/api/v3/`](../../pkg/api/v3/)
- [README](../../pkg/api/v3/README.md)

### Protocol Definitions
- [System Protocol](../../protocol/system.md)
- [Transaction Protocol](../../protocol/transactions.md)

## Related Documentation

- [Network Documentation](../network/INDEX.md) - Network configuration
- [Testing Documentation](../testing/INDEX.md) - API testing
- [Client Documentation](../client/INDEX.md) - Client libraries
- [Design Documentation](../design/INDEX.md) - API design decisions