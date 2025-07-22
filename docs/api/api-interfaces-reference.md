# Accumulate API Interfaces Reference

> **Complete reference for all Accumulate API interfaces, methods, and practical examples**

## Table of Contents

1. [Quick Start with DevNet](#quick-start-with-devnet)
2. [API v3 Services - Complete Reference](#api-v3-services---complete-reference)
3. [Query Types - All Available Queries](#query-types---all-available-queries)
4. [Record Types - Complete Coverage](#record-types---complete-coverage)
5. [Transport Options](#transport-options)
6. [Error Handling](#error-handling)
7. [API v2 Legacy Reference](#api-v2-legacy-reference)
8. [Migration Guide](#migration-guide)

## Quick Start with DevNet

### Launch Local DevNet

**Method 1: Using accumulated daemon (Recommended)**

```bash
# From accumulate repository root
# Step 1: Initialize DevNet
go run ./cmd/accumulated run devnet --init-only --reset -w .nodes

# Step 2: Run DevNet
go run ./cmd/accumulated run devnet -w .nodes
```

**Method 2: Using test automation script**

```bash
# From accumulate repository root
cd test/cmd/devnet
go run main.go
```

**Note**: Method 2 may have dependency issues. Use Method 1 for reliable DevNet setup.

This starts a local Accumulate network with:
- **Directory Network**: Multiple nodes on `127.0.1.x:26657`
- **Block Validator Networks**: Multiple BVNs on different ports
- **HTTP API**: `http://127.0.0.1:26660/v2` (JSON-RPC)
- **Individual Nodes**: `http://127.0.1.x:26657` (various IPs)

### Test All API Endpoints

```bash
# Test basic connectivity - Query the Directory Network identity
curl -s -X POST http://127.0.0.1:26660/v2 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"query","params":{"url":"acc://dn.acme"},"id":1}'

# Query network status (using individual node)
curl -s -X POST http://127.0.1.2:26657/v2 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"query","params":{"url":"acc://dn.acme"},"id":1}'

# Create a test identity (example)
curl -s -X POST http://127.0.0.1:26660/v2 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"submit","params":{"envelope":{"transaction":[{"header":{"principal":"acc://dn.acme","initiator":"acc://dn.acme"},"body":{"type":"createIdentity","url":"acc://alice"}}]}},"id":1}'
```

## API v3 Services - Complete Reference

### 1. NodeService
**Purpose**: Node information and service discovery

#### Methods:

##### NodeInfo
```go
NodeInfo(ctx context.Context, opts NodeInfoOptions) (*NodeInfo, error)
```

**JSON-RPC**: `node-info`  
**REST**: `GET /node/info`  
**WebSocket**: `NodeInfoRequest`

**Parameters**:
- `PeerID` (optional): Specific peer to query

**Example**:
```bash
# REST
curl http://localhost:26657/v3/node/info

# JSON-RPC
curl -X POST http://localhost:26657/v3 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "method": "node-info", "params": {}, "id": 1}'
```

**Response**:
```json
{
  "nodeID": "16Uiu2HAm...",
  "network": "DevNet",
  "version": "v1.4.0",
  "commit": "abc123...",
  "services": [
    {"type": "consensus", "address": "/acc/DevNet/consensus"},
    {"type": "query", "address": "/acc/DevNet/query"}
  ]
}
```

##### FindService
```go
FindService(ctx context.Context, opts FindServiceOptions) ([]*FindServiceResult, error)
```

**JSON-RPC**: `find-service`  
**REST**: `GET /node/services`  
**WebSocket**: `FindServiceRequest`

**Parameters**:
- `Network` (string): Target network name
- `Service` (ServiceAddress): Service type to find
- `Known` (bool): Search known peers only
- `Timeout` (duration): Discovery timeout

**Example**:
```bash
# Find query services
curl "http://localhost:26657/v3/node/services?type=query&network=DevNet"

# JSON-RPC
curl -X POST http://localhost:26657/v3 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "find-service",
    "params": {
      "network": "DevNet",
      "service": {"type": "query"}
    },
    "id": 1
  }'
```

### 2. NetworkService
**Purpose**: Network status and information

#### Methods:

##### NetworkStatus
```go
NetworkStatus(ctx context.Context, opts NetworkStatusOptions) (*NetworkStatus, error)
```

**JSON-RPC**: `network-status`  
**REST**: `GET /network/status`  
**WebSocket**: `NetworkStatusRequest`

**Parameters**:
- `Partition` (optional): Specific partition to query

**Example**:
```bash
# REST
curl http://localhost:26657/v3/network/status

# JSON-RPC
curl -X POST http://localhost:26657/v3 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc": "2.0", "method": "network-status", "params": {}, "id": 1}'
```

**Response**:
```json
{
  "network": "DevNet",
  "partitions": [
    {
      "id": "Directory",
      "type": "directory",
      "nodes": 1,
      "height": 12345,
      "timestamp": "2025-01-21T19:11:52Z"
    },
    {
      "id": "BVN0",
      "type": "blockValidator",
      "nodes": 3,
      "height": 12340,
      "timestamp": "2025-01-21T19:11:50Z"
    }
  ]
}
```

### 3. ConsensusService
**Purpose**: Consensus node status and information

#### Methods:

##### ConsensusStatus
```go
ConsensusStatus(ctx context.Context, opts ConsensusStatusOptions) (*ConsensusStatus, error)
```

**JSON-RPC**: `consensus-status`  
**REST**: `GET /consensus/status`  
**WebSocket**: `ConsensusStatusRequest`

**Parameters**:
- `NodeID` (optional): Specific node to query
- `Partition` (optional): Partition to query
- `IncludePeers` (bool): Include peer information
- `IncludeAccumulate` (bool): Include Accumulate-specific data

**Example**:
```bash
# REST
curl http://localhost:26657/v3/consensus/status

# JSON-RPC with parameters
curl -X POST http://localhost:26657/v3 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "consensus-status",
    "params": {
      "partition": "Directory",
      "includePeers": true
    },
    "id": 1
  }'
```

### 4. Querier (Query Service)
**Purpose**: Query account state, transactions, and blockchain data

#### Methods:

##### Query
```go
Query(ctx context.Context, scope *url.URL, query Query) (Record, error)
```

**JSON-RPC**: `query`  
**REST**: `POST /query/{scope}`  
**WebSocket**: `QueryRequest`

**Parameters**:
- `scope` (URL): Account or resource to query
- `query` (Query): Query type and parameters

**Example**:
```bash
# Query account state
curl -X POST "http://localhost:26657/v3/query/acc://alice" \
  -H "Content-Type: application/json" \
  -d '{"queryType": "default"}'

# Query account directory
curl -X POST "http://localhost:26657/v3/query/acc://alice" \
  -H "Content-Type: application/json" \
  -d '{
    "queryType": "directory",
    "range": {"start": 0, "count": 10}
  }'

# Query transaction
curl -X POST "http://localhost:26657/v3/query/acc://alice" \
  -H "Content-Type: application/json" \
  -d '{
    "queryType": "chain",
    "name": "main",
    "range": {"start": 0, "count": 5}
  }'
```

### 5. Submitter (Submit Service)
**Purpose**: Submit transactions to the network

#### Methods:

##### Submit
```go
Submit(ctx context.Context, envelope *messaging.Envelope, opts SubmitOptions) ([]*Submission, error)
```

**JSON-RPC**: `submit`  
**REST**: `POST /submit`  
**WebSocket**: `SubmitRequest`

**Parameters**:
- `envelope` (Envelope): Transaction envelope with signatures
- `opts` (SubmitOptions): Submission options

**Example**:
```bash
# Submit a transaction (envelope must be properly signed)
curl -X POST http://localhost:26657/v3/submit \
  -H "Content-Type: application/json" \
  -d '{
    "envelope": {
      "transaction": {
        "header": {
          "principal": "acc://alice",
          "initiator": "acc://alice/book/1"
        },
        "body": {
          "type": "sendTokens",
          "to": [{"url": "acc://bob", "amount": "1000000"}]
        }
      },
      "signatures": [{
        "type": "ed25519",
        "publicKey": "...",
        "signature": "..."
      }]
    }
  }'
```

### 6. Validator (Validate Service)
**Purpose**: Validate transactions without submitting

#### Methods:

##### Validate
```go
Validate(ctx context.Context, envelope *messaging.Envelope, opts ValidateOptions) ([]*Submission, error)
```

**JSON-RPC**: `validate`  
**REST**: `POST /validate`  
**WebSocket**: `ValidateRequest`

**Parameters**:
- `envelope` (Envelope): Transaction envelope to validate
- `opts` (ValidateOptions): Validation options

**Example**:
```bash
# Validate transaction without submitting
curl -X POST http://localhost:26657/v3/validate \
  -H "Content-Type: application/json" \
  -d '{
    "envelope": {
      "transaction": {
        "header": {
          "principal": "acc://alice",
          "initiator": "acc://alice/book/1"
        },
        "body": {
          "type": "sendTokens",
          "to": [{"url": "acc://bob", "amount": "1000000"}]
        }
      }
    }
  }'
```

### 7. Faucet
**Purpose**: Request test tokens (DevNet/TestNet only)

#### Methods:

##### Faucet
```go
Faucet(ctx context.Context, account *url.URL, opts FaucetOptions) (*Submission, error)
```

**JSON-RPC**: `faucet`  
**REST**: `POST /faucet`  
**WebSocket**: `FaucetRequest`

**Parameters**:
- `account` (URL): Account to fund
- `opts` (FaucetOptions): Faucet options

**Example**:
```bash
# Request test tokens
curl -X POST http://localhost:26657/v3/faucet \
  -H "Content-Type: application/json" \
  -d '{"account": "acc://alice"}'

# JSON-RPC
curl -X POST http://localhost:26657/v3 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "faucet",
    "params": {"account": "acc://alice"},
    "id": 1
  }'
```

### 8. MetricsService
**Purpose**: Network performance metrics and statistics

#### Methods:

##### Metrics
```go
Metrics(ctx context.Context, opts MetricsOptions) (*Metrics, error)
```

**JSON-RPC**: `metrics`  
**REST**: `GET /metrics`  
**WebSocket**: `MetricsRequest`

**Parameters**:
- `Partition` (optional): Specific partition to query
- `Span` (optional): Time span for metrics

**Example**:
```bash
# Get network metrics
curl http://localhost:26657/v3/metrics

# Get partition-specific metrics
curl "http://localhost:26657/v3/metrics?partition=BVN0&span=3600"

# JSON-RPC
curl -X POST http://localhost:26657/v3 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "metrics",
    "params": {"partition": "Directory"},
    "id": 1
  }'
```

**Response**:
```json
{
  "tps": 15.7,
  "blockTime": 1.2,
  "totalTransactions": 1234567,
  "totalAccounts": 98765,
  "partitionMetrics": {
    "Directory": {"height": 12345, "tps": 5.2},
    "BVN0": {"height": 12340, "tps": 10.5}
  }
}
```

### 9. EventService
**Purpose**: Real-time event subscriptions (WebSocket only)

#### Methods:

##### Subscribe
```go
Subscribe(ctx context.Context, opts SubscribeOptions) (<-chan Event, error)
```

**WebSocket Only**: `SubscribeRequest`

**Parameters**:
- `Account` (optional): Filter events for specific account
- `Types` (optional): Filter by event types

**Example**:
```javascript
// WebSocket subscription
const ws = new WebSocket('ws://localhost:26657/v3/ws');

ws.onopen = () => {
  ws.send(JSON.stringify({
    type: 'subscribeRequest',
    account: 'acc://alice',
    types: ['transaction', 'block']
  }));
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('Event:', data);
};
```

### 10. SnapshotService
**Purpose**: Snapshot management and information

#### Methods:

##### ListSnapshots
```go
ListSnapshots(ctx context.Context, opts ListSnapshotsOptions) ([]*SnapshotInfo, error)
```

**JSON-RPC**: `list-snapshots`  
**REST**: `GET /snapshots`  
**WebSocket**: `ListSnapshotsRequest`

**Parameters**:
- `Partition` (optional): Specific partition
- `Network` (optional): Network filter

**Example**:
```bash
# List available snapshots
curl http://localhost:26657/v3/snapshots

# JSON-RPC
curl -X POST http://localhost:26657/v3 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "list-snapshots",
    "params": {},
    "id": 1
  }'
```

## Query Types - All Available Queries

The `Querier` service supports multiple query types for different data retrieval needs:

### 1. Default Query (`QueryTypeDefault`)
**Purpose**: Get basic account information

**Parameters**: None

**Example**:
```bash
curl -X POST "http://localhost:26657/v3/query/acc://alice" \
  -H "Content-Type: application/json" \
  -d '{"queryType": "default"}'
```

**Response**: Account record with current state

### 2. Chain Query (`QueryTypeChain`)
**Purpose**: Query transaction chains

**Parameters**:
- `name` (string): Chain name (e.g., "main", "scratch")
- `range` (Range): Start index and count
- `expand` (bool): Include full transaction data

**Example**:
```bash
# Get main chain transactions
curl -X POST "http://localhost:26657/v3/query/acc://alice" \
  -H "Content-Type: application/json" \
  -d '{
    "queryType": "chain",
    "name": "main",
    "range": {"start": 0, "count": 10},
    "expand": true
  }'

# Get scratch chain
curl -X POST "http://localhost:26657/v3/query/acc://alice" \
  -H "Content-Type: application/json" \
  -d '{
    "queryType": "chain",
    "name": "scratch",
    "range": {"start": 0, "count": 5}
  }'
```

### 3. Data Query (`QueryTypeData`)
**Purpose**: Query data account entries

**Parameters**:
- `range` (Range): Entry range to retrieve
- `expand` (bool): Include full data content

**Example**:
```bash
curl -X POST "http://localhost:26657/v3/query/acc://alice/data" \
  -H "Content-Type: application/json" \
  -d '{
    "queryType": "data",
    "range": {"start": 0, "count": 20},
    "expand": true
  }'
```

### 4. Directory Query (`QueryTypeDirectory`)
**Purpose**: List account directory contents

**Parameters**:
- `range` (Range): Range of entries to retrieve
- `expand` (bool): Include sub-account details

**Example**:
```bash
# List sub-accounts
curl -X POST "http://localhost:26657/v3/query/acc://alice" \
  -H "Content-Type: application/json" \
  -d '{
    "queryType": "directory",
    "range": {"start": 0, "count": 50}
  }'
```

### 5. Pending Query (`QueryTypePending`)
**Purpose**: Get pending transactions

**Parameters**:
- `range` (Range): Range of pending transactions

**Example**:
```bash
curl -X POST "http://localhost:26657/v3/query/acc://alice" \
  -H "Content-Type: application/json" \
  -d '{
    "queryType": "pending",
    "range": {"start": 0, "count": 10}
  }'
```

### 6. Block Query (`QueryTypeBlock`)
**Purpose**: Query block information

**Parameters**:
- `minor` (uint64): Minor block number
- `major` (uint64): Major block number
- `expand` (bool): Include transaction details

**Example**:
```bash
# Query specific block
curl -X POST "http://localhost:26657/v3/query/acc://alice" \
  -H "Content-Type: application/json" \
  -d '{
    "queryType": "block",
    "minor": 12345,
    "expand": true
  }'
```

### 7. Anchor Search (`QueryTypeAnchorSearch`)
**Purpose**: Search for anchor transactions

**Parameters**:
- `anchor` ([]byte): Anchor hash to search for
- `includeReceipt` (bool): Include anchor receipt

**Example**:
```bash
curl -X POST "http://localhost:26657/v3/query/acc://alice" \
  -H "Content-Type: application/json" \
  -d '{
    "queryType": "anchorSearch",
    "anchor": "0x1234567890abcdef...",
    "includeReceipt": true
  }'
```

### 8. Public Key Search (`QueryTypePublicKeySearch`)
**Purpose**: Find accounts by public key

**Parameters**:
- `publicKey` ([]byte): Public key to search for
- `type` (string): Key type ("ed25519", "secp256k1", etc.)

**Example**:
```bash
curl -X POST "http://localhost:26657/v3/query/acc://alice" \
  -H "Content-Type: application/json" \
  -d '{
    "queryType": "publicKeySearch",
    "publicKey": "0x1234567890abcdef...",
    "type": "ed25519"
  }'
```

### 9. Public Key Hash Search (`QueryTypePublicKeyHashSearch`)
**Purpose**: Find accounts by public key hash

**Parameters**:
- `publicKeyHash` ([]byte): Hash of public key

**Example**:
```bash
curl -X POST "http://localhost:26657/v3/query/acc://alice" \
  -H "Content-Type: application/json" \
  -d '{
    "queryType": "publicKeyHashSearch",
    "publicKeyHash": "0x9876543210fedcba..."
  }'
```

### 10. Delegate Search (`QueryTypeDelegateSearch`)
**Purpose**: Find accounts by delegate

**Parameters**:
- `delegate` (URL): Delegate account URL

**Example**:
```bash
curl -X POST "http://localhost:26657/v3/query/acc://alice" \
  -H "Content-Type: application/json" \
  -d '{
    "queryType": "delegateSearch",
    "delegate": "acc://validator1"
  }'
```

### 11. Message Hash Search (`QueryTypeMessageHashSearch`)
**Purpose**: Find transactions by message hash

**Parameters**:
- `hash` ([]byte): Message hash to search for

**Example**:
```bash
curl -X POST "http://localhost:26657/v3/query/acc://alice" \
  -H "Content-Type: application/json" \
  -d '{
    "queryType": "messageHashSearch",
    "hash": "0xabcdef1234567890..."
  }'
```

## Record Types - Complete Coverage

API v3 returns different record types based on the query:

### Account Records
- **LiteIdentity**: Basic identity account
- **LiteTokenAccount**: Token balance account
- **LiteDataAccount**: Data storage account
- **ADI**: Accumulate Digital Identifier
- **TokenIssuer**: Token issuing account
- **KeyBook**: Key management account
- **KeyPage**: Individual key page

### Transaction Records
- **Transaction**: Complete transaction data
- **TransactionStatus**: Transaction execution status
- **Signature**: Transaction signature data

### Message Records
- **BlockAnchor**: Block anchoring message
- **DirectoryAnchor**: Directory anchoring message
- **SequencedMessage**: Sequenced network message

### Chain Records
- **ChainEntry**: Individual chain entry
- **IndexEntry**: Chain index entry
- **AnchorChain**: Anchor chain data

## Transport Options

Accumulate API v3 supports multiple transport mechanisms:

### 1. HTTP/REST
**Endpoint**: `http://localhost:26657/v3/`

**Advantages**:
- Simple request/response model
- Standard HTTP status codes
- Easy debugging and testing
- Cacheable responses

### 2. JSON-RPC
**Endpoint**: `http://localhost:26657/v3`

**Advantages**:
- Batch requests support
- Standardized error format
- Method-based routing
- Compatible with existing tools

### 3. WebSocket
**Endpoint**: `ws://localhost:26657/v3/ws`

**Advantages**:
- Real-time event subscriptions
- Bi-directional communication
- Lower latency for frequent requests
- Connection persistence

### 4. P2P (Peer-to-Peer)
**Direct node communication**

**Advantages**:
- Direct node-to-node queries
- Bypasses HTTP layer
- Lower overhead
- Network topology aware

## Error Handling

Accumulate API v3 uses structured error responses:

### HTTP Status Codes
- `200` - Success
- `400` - Bad Request (invalid parameters)
- `404` - Not Found (account/transaction not found)
- `500` - Internal Server Error
- `503` - Service Unavailable (node offline)

### Error Response Format
```json
{
  "error": {
    "code": -32600,
    "message": "Invalid Request",
    "data": {
      "type": "ValidationError",
      "details": "Account URL is malformed"
    }
  }
}
```

### Common Error Types
- **ValidationError**: Invalid input parameters
- **NotFoundError**: Resource not found
- **NetworkError**: Network connectivity issues
- **ConsensusError**: Consensus-related failures
- **RateLimitError**: Too many requests

### Error Handling Example
```bash
# Handle errors in curl
response=$(curl -s -w "%{http_code}" "http://localhost:26657/v3/query/acc://invalid")
http_code=$(echo "$response" | tail -c 4)
if [ "$http_code" != "200" ]; then
  echo "Error: HTTP $http_code"
  echo "$response" | head -c -4 | jq '.error'
fi
```

## API v2 Legacy Reference

> **⚠️ Deprecated**: API v2 is deprecated. Use API v3 for new applications.

### Key Differences from v3
- Single JSON-RPC endpoint only
- Different method names and parameters
- Less structured error handling
- No WebSocket support
- Limited query capabilities

### v2 Endpoint
**URL**: `http://localhost:26657/v2`

### Common v2 Methods
```bash
# Get account (v2)
curl -X POST http://localhost:26657/v2 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "query",
    "params": {
      "url": "acc://alice"
    },
    "id": 1
  }'

# Submit transaction (v2)
curl -X POST http://localhost:26657/v2 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "execute",
    "params": {
      "envelope": {...}
    },
    "id": 1
  }'
```

## Migration Guide

### Migrating from API v2 to v3

#### 1. Update Endpoints
```bash
# v2
POST http://localhost:26657/v2

# v3 (multiple options)
POST http://localhost:26657/v3                    # JSON-RPC
GET  http://localhost:26657/v3/node/info          # REST
WS   ws://localhost:26657/v3/ws                   # WebSocket
```

#### 2. Update Method Names
| v2 Method | v3 Equivalent | Notes |
|-----------|---------------|---------|
| `query` | `query` | Same name, different parameters |
| `execute` | `submit` | New parameter structure |
| `query-tx` | `query` with transaction scope | More flexible |
| `version` | `node-info` | Extended information |
| `metrics` | `metrics` | Enhanced metrics |

#### 3. Update Query Structure
```bash
# v2 Query
{
  "method": "query",
  "params": {
    "url": "acc://alice"
  }
}

# v3 Query
{
  "method": "query",
  "params": {
    "scope": "acc://alice",
    "query": {
      "queryType": "default"
    }
  }
}
```

#### 4. Update Error Handling
```javascript
// v2 Error Handling
if (response.error) {
  console.error('Error:', response.error.message);
}

// v3 Error Handling
if (response.error) {
  console.error('Error:', response.error.message);
  console.error('Type:', response.error.data?.type);
  console.error('Details:', response.error.data?.details);
}
```

### Best Practices for Migration

1. **Gradual Migration**: Migrate endpoints one at a time
2. **Test Thoroughly**: Use DevNet for testing before production
3. **Update Error Handling**: Take advantage of structured errors
4. **Use New Features**: Leverage WebSocket for real-time updates
5. **Monitor Performance**: v3 offers better performance characteristics

### Complete DevNet Testing Workflow

```bash
#!/bin/bash
# Complete API testing script for DevNet

set -e

echo "Starting DevNet..."
# Initialize DevNet
go run ./cmd/accumulated run devnet --init-only --reset -w .nodes

# Start DevNet in background
go run ./cmd/accumulated run devnet -w .nodes &
DEVNET_PID=$!

# Wait for DevNet to start
echo "Waiting for DevNet to start..."
sleep 15

# Test all API endpoints
echo "Testing API endpoints..."

# 1. Query Directory Network
echo "1. Testing Directory Network query..."
curl -s -X POST http://127.0.0.1:26660/v2 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"query","params":{"url":"acc://dn.acme"},"id":1}' | jq '.result.type'

# 2. Test individual node
echo "2. Testing individual BVN node..."
curl -s -X POST http://127.0.1.2:26657/v2 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"query","params":{"url":"acc://dn.acme"},"id":1}' | jq '.result.type'

# 3. Create test identity (if faucet available)
echo "3. Creating test identity..."
curl -s -X POST http://127.0.0.1:26660/v2 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"submit","params":{"envelope":{"transaction":[{"header":{"principal":"acc://dn.acme","initiator":"acc://dn.acme"},"body":{"type":"createIdentity","url":"acc://test-identity"}}]}},"id":1}' | jq '.result'

# 4. Wait and query the created identity
echo "4. Querying created identity..."
sleep 3  # Wait for transaction processing
curl -s -X POST http://127.0.0.1:26660/v2 \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"query","params":{"url":"acc://test-identity"},"id":1}' | jq '.result.type'

echo "All tests completed successfully!"

# Cleanup
kill $DEVNET_PID
```

---

## Summary

This comprehensive API reference covers:

✅ **Complete API v3 Service Coverage**: All 10 services with detailed method documentation  
✅ **Exhaustive Query Types**: All 11 query types with parameters and examples  
✅ **Practical DevNet Examples**: Ready-to-run commands for testing  
✅ **Multiple Transport Options**: HTTP, JSON-RPC, WebSocket, and P2P  
✅ **Error Handling**: Structured error responses and handling patterns  
✅ **Migration Guide**: Complete v2 to v3 migration instructions  
✅ **Testing Workflow**: End-to-end DevNet testing script

For additional help:
- **DevNet Setup**: Use `go run ./cmd/accumulated run devnet --init-only --reset -w .nodes` then `go run ./cmd/accumulated run devnet -w .nodes`
- **TestNet Access**: Connect to public TestNet endpoints (Kermit network)
- **MainNet Access**: Connect to public MainNet endpoints (Cyclops network)
- **Tool Documentation**: See `/docs/tools/` for CLI tool references
- **Implementation Details**: Check `/docs/api/` for technical specifications


