# Accumulate Network API - Quick Reference Guide

## API Endpoints Summary

### V3 API - Primary Access Methods

#### JSON-RPC 2.0 (HTTP POST)
**Base:** Most nodes expose via `/jsonrpc` endpoint

**Service Methods:**
```
node-info
find-service
consensus-status
network-status
list-snapshots
metrics
query
submit
validate
faucet
```

#### REST API (HTTP)
**Base:** Most nodes expose via API root

**Endpoints:**
```
GET  /node/info
GET  /node/services
GET  /consensus/status
GET  /network/status
GET  /metrics
POST /submit
POST /validate
POST /faucet
```

#### WebSocket (WS/WSS)
**Binary message protocol over WebSocket**

**Supports:** All V3 services via StreamID-multiplexed messages

#### P2P Network
**Protocol:** libp2p-based peer-to-peer
**Service Discovery:** DHT + peer database

---

## Quick Query Examples

### Query an Account
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "query",
  "params": {
    "scope": "acc://account-url",
    "query": {
      "type": "DefaultQuery"
    }
  }
}
```

### Query Transaction by ID
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "query",
  "params": {
    "scope": "txid://account-url/hash",
    "query": {
      "type": "DefaultQuery"
    }
  }
}
```

### Query Chain Entries
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "query",
  "params": {
    "scope": "acc://account-url",
    "query": {
      "type": "ChainQuery",
      "name": "main",
      "range": {
        "start": 0,
        "count": 10
      }
    }
  }
}
```

### Submit Transaction
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "submit",
  "params": {
    "envelope": {
      "transaction": [...],
      "signatures": [...]
    },
    "submitOptions": {
      "verify": true,
      "wait": true
    }
  }
}
```

### Get Network Status
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "network-status",
  "params": {
    "partition": "Directory"
  }
}
```

### Find Service Nodes
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "find-service",
  "params": {
    "network": "Mainnet",
    "service": {
      "type": "Query",
      "partition": "Directory"
    }
  }
}
```

### Get Node Info
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "node-info",
  "params": {}
}
```

---

## Query Types Reference

| Type | Code | Use Case |
|------|------|----------|
| DefaultQuery | 0x00 | Get account details |
| ChainQuery | 0x01 | Get chain entries by name/index/hash |
| DataQuery | 0x02 | Get data chain entries |
| DirectoryQuery | 0x03 | Get directory entries |
| PendingQuery | 0x04 | Get pending transactions |
| BlockQuery | 0x05 | Get blocks |
| AnchorSearchQuery | 0x10 | Find by anchor hash |
| PublicKeySearchQuery | 0x11 | Find by public key |
| PublicKeyHashSearchQuery | 0x12 | Find by key hash |
| DelegateSearchQuery | 0x13 | Find by delegate |
| MessageHashSearchQuery | 0x14 | Find by message hash |

---

## Record Types Reference

| Type | Code | Contains |
|------|------|----------|
| AccountRecord | 0x01 | Account + directory + pending |
| ChainRecord | 0x02 | Chain metadata |
| ChainEntryRecord | 0x03 | Single entry + merkle proof |
| KeyRecord | 0x04 | Key specification |
| MessageRecord | 0x10 | Transaction + status + result |
| SignatureSetRecord | 0x11 | Account + signature range |
| MinorBlockRecord | 0x20 | Minor block data |
| MajorBlockRecord | 0x21 | Major block data |
| RecordRange | 0x80 | Paginated results |
| UrlRecord | 0x81 | URL value |
| TxIDRecord | 0x82 | Transaction ID value |
| IndexEntryRecord | 0x83 | Index entry spec |
| ErrorRecord | 0x8F | Error details |

---

## Common Options

### RangeOptions (Pagination)
```json
{
  "start": 0,
  "count": 10,
  "expand": true,
  "fromEnd": false
}
```

### ReceiptOptions (Merkle Proofs)
```json
{
  "forAny": true,
  "forHeight": 0
}
```

### SubmitOptions
```json
{
  "verify": true,
  "wait": true
}
```

### ValidateOptions
```json
{
  "full": true
}
```

---

## Response Status Codes

### Success
- 200 OK - Successful request

### Client Errors
- 400 Bad Request - Invalid parameters
- 404 Not Found - Resource not found
- 409 Conflict - Conflicting state

### Server Errors
- 500 Internal Error - Server error
- JSON-RPC Error Codes (base -33000):
  - -33001: Protocol error
  - -33002: Invalid parameters
  - etc.

---

## Event Types

| Type | Code | Contains |
|------|------|----------|
| ErrorEvent | 1 | Error information |
| BlockEvent | 2 | Block + entries committed |
| GlobalsEvent | 3 | Global values change |

---

## V2 API (Legacy)

**Endpoint:** `/v2` (POST)

**Key Methods:**
- `status`, `version`, `describe`
- `query`, `query-directory`, `query-tx`
- `execute`, `execute-direct`, `faucet`
- `create-adi`, `create-token`, `send-tokens`
- `query-data`, `query-data-set`

---

## Ethereum RPC API

**Methods:**
```
eth_chainId()
eth_blockNumber()
eth_gasPrice()
eth_getBalance(address, block)
eth_getBlockByNumber(block, expand)
net_version()
acc_typedData(transaction, signature)
```

---

## Service Types

| ID | Name | Purpose |
|----|------|---------|
| 1 | Node | Node information + discovery |
| 2 | Consensus | Validator consensus status |
| 3 | Network | Network configuration + status |
| 4 | Metrics | TPS and performance metrics |
| 5 | Query | Account/transaction state queries |
| 6 | Event | Event stream subscription |
| 7 | Submit | Transaction submission |
| 8 | Validate | Transaction validation |
| 9 | Faucet | Token requests |
| 10 | Snapshot | Snapshot management |

---

## Common Patterns

### Error Handling
```
All APIs return standardized error responses:
{
  "code": -33XXX,
  "message": "Description",
  "data": {
    "code": XXX,
    "message": "Description"
  }
}
```

### URL Formats
```
Account:     acc://account-url
Transaction: txid://account-url/hash
Authority:   acc://authority-url
Partition:   partition name (e.g., "Directory", "BVN-0")
```

### Transaction Flow
1. Create Envelope (transaction + signatures)
2. Submit via `submit` or validate via `validate`
3. Poll with `query` for status
4. Monitor via `subscribe` for events

---

## Discovery and Bootstrap

**Find Query Service:**
```json
{
  "network": "Mainnet",
  "service": {
    "type": "Query",
    "partition": "Directory"
  }
}
```

**Find Any Service:**
```json
{
  "network": "Mainnet",
  "service": null
}
```

---

## Configuration

**Network Parameters in NetworkStatus:**
- Oracle configuration
- Global parameters
- Network definition
- Routing table
- Executor versions

---

## Rate Limiting & Timeouts

- Default DHT discovery timeout: 2 seconds
- Submit wait timeout: configurable
- Query timeout: depends on client
- Service discovery timeout: optional parameter

---

## Best Practices

1. **Use Find-Service** for dynamic peer discovery
2. **Pool queries** with Range options for large datasets
3. **Handle errors** gracefully with retry logic
4. **Monitor events** for real-time updates
5. **Validate** transactions before submitting
6. **Use receipts** for transaction proof
7. **Cache** service addresses
8. **Implement** connection pooling for efficiency

