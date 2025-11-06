# Accumulate MCP API Mapping Reference

## Overview

This document provides detailed mappings between MCP tools and Accumulate API endpoints, including request/response examples and code references.

## Mapping Format

Each mapping includes:
- **MCP Tool**: The tool name exposed via MCP
- **API Endpoint**: The underlying Accumulate API endpoint
- **Request Example**: Sample API request
- **Response Example**: Sample API response
- **Code Reference**: Source file location
- **Notes**: Important considerations

---

## 1. Network & Node Information

### accumulate_node_info → node-info

**API Version:** V3

**Method:** `node-info`

**Code Reference:** `pkg/api/v3/node.go:NodeService.NodeInfo()`

**Request Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "node-info",
  "params": {
    "peerID": ""
  }
}
```

**Response Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "peerID": "QmRaefUdifL9K45hxBeSNMaTAF8n6DPpX1VMgk3QSCmkmD",
    "network": "MainNet",
    "services": [
      {
        "type": "query",
        "partition": "Directory",
        "address": "/ip4/1.2.3.4/tcp/16593"
      }
    ],
    "version": "v1.4.3",
    "commit": "abc123def"
  }
}
```

**REST Equivalent:**
```bash
GET /node/info
```

**MCP Tool Parameters:**
```typescript
{
  peer_id?: string  // Optional peer ID
}
```

**MCP Tool Response:**
```typescript
{
  peer_id: string
  network: string
  services: Array<{
    type: string
    partition?: string
    address: string
  }>
  version: string
  commit: string
}
```

---

### accumulate_find_service → find-service

**API Version:** V3

**Method:** `find-service`

**Code Reference:** `pkg/api/v3/node.go:NodeService.FindService()`

**Request Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "find-service",
  "params": {
    "network": "MainNet",
    "service": {
      "type": "query",
      "partition": "Directory"
    },
    "known": true,
    "timeout": "30s"
  }
}
```

**Response Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "services": [
      {
        "peerID": "QmRaef...",
        "status": 1,
        "addresses": [
          "/ip4/1.2.3.4/tcp/16593/p2p/QmRaef..."
        ]
      }
    ]
  }
}
```

**MCP Tool Parameters:**
```typescript
{
  network?: string
  service_type?: "Node" | "Consensus" | "Network" | "Metrics" | "Query" | "Event" | "Submit" | "Validate" | "Faucet" | "Snapshot"
  partition?: string
  known_only?: boolean
  timeout?: string  // Duration like "30s"
}
```

---

### accumulate_network_status → network-status

**API Version:** V3

**Method:** `network-status`

**Code Reference:** `pkg/api/v3/network.go:NetworkService.NetworkStatus()`

**Request Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "network-status",
  "params": {
    "partition": "BVN0"
  }
}
```

**Response Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "oracle": {
      "price": 500000,
      "timestamp": 1697123456
    },
    "routing": {
      "routes": [...]
    },
    "network": {
      "id": "MainNet",
      "partitions": ["Directory", "BVN0", "BVN1", "BVN2"]
    },
    "executorVersion": {
      "version": 2,
      "halt": false
    }
  }
}
```

**REST Equivalent:**
```bash
GET /network/status?partition=BVN0
```

---

### accumulate_consensus_status → consensus-status

**API Version:** V3

**Method:** `consensus-status`

**Code Reference:** `pkg/api/v3/consensus.go:ConsensusService.ConsensusStatus()`

**Request Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "consensus-status",
  "params": {
    "partition": "BVN0",
    "nodeID": ""
  }
}
```

**Response Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "ok": true,
    "version": "v0.37.4",
    "validator": {
      "type": "validator",
      "id": "acc://bvn-BVN0.acme/validators/1",
      "publicKey": "0123456789abcdef...",
      "publicKeyHash": "abcdef...",
      "partitions": ["BVN0"]
    },
    "lastBlock": {
      "height": 1234567,
      "time": "2024-10-20T12:00:00Z",
      "hash": "abc123..."
    },
    "validatorSet": [
      {
        "publicKey": "0123...",
        "votingPower": 1000
      }
    ],
    "catchingUp": false
  }
}
```

**REST Equivalent:**
```bash
GET /consensus/status?partition=BVN0
```

---

### accumulate_metrics → metrics

**API Version:** V3

**Method:** `metrics`

**Code Reference:** `pkg/api/v3/metrics.go:MetricsService.Metrics()`

**Request Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "metrics",
  "params": {
    "partition": "BVN0",
    "metric": "tps"
  }
}
```

**Response Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "tps": 42.5,
    "blockRate": 1.0,
    "apiLatency": {
      "p50": 25,
      "p95": 100,
      "p99": 250
    }
  }
}
```

**REST Equivalent:**
```bash
GET /metrics?partition=BVN0&metric=tps
```

---

## 2. Query Operations

### accumulate_query_account → query (DefaultQuery)

**API Version:** V3

**Method:** `query`

**Query Type:** `DefaultQuery`

**Code Reference:** `pkg/api/v3/query.go:Querier.Query()`

**Request Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "query",
  "params": {
    "scope": "acc://alice.acme/tokens",
    "query": {
      "type": "Default",
      "includeReceipt": true,
      "prove": false
    }
  }
}
```

**Response Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "type": "account",
    "account": {
      "type": "tokenAccount",
      "url": "acc://alice.acme/tokens",
      "tokenUrl": "acc://ACME",
      "balance": "1000000000",
      "authorities": [
        "acc://alice.acme/book"
      ]
    },
    "receipt": {
      "start": "acc://alice.acme/tokens",
      "entries": [...]
    }
  }
}
```

**MCP Tool Parameters:**
```typescript
{
  url: string              // Required: Account URL
  include_receipt?: boolean // Optional: Include Merkle receipt
  prove?: boolean          // Optional: Include cryptographic proof
}
```

**Supported Account Types:**
- `unknownAccount` - Unknown account type
- `liteIdentity` - Lite identity account
- `liteTokenAccount` - Lite token account
- `liteDataAccount` - Lite data account
- `identity` - ADI (Accumulate Digital Identifier)
- `tokenAccount` - Token account
- `tokenIssuer` - Token issuer
- `dataAccount` - Data account
- `keyBook` - Key book (authority)
- `keyPage` - Key page (contains actual keys)
- And 30+ protocol account types

---

### accumulate_query_transaction → query (DefaultQuery on txid)

**API Version:** V3

**Method:** `query`

**Query Type:** `DefaultQuery`

**Code Reference:** `pkg/api/v3/query.go:Querier.Query()`

**Request Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "query",
  "params": {
    "scope": "acc://alice.acme/tokens@abc123def456...",
    "query": {
      "type": "Default",
      "includeReceipt": true
    }
  }
}
```

**Alternative Format (txid://):**
```json
{
  "scope": "txid://abc123def456..."
}
```

**Response Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "type": "txn",
    "txid": "abc123def456...",
    "transaction": {
      "header": {
        "principal": "acc://alice.acme/tokens",
        "initiator": "abc123..."
      },
      "body": {
        "type": "sendTokens",
        "to": [
          {
            "url": "acc://bob.acme/tokens",
            "amount": "1000000000"
          }
        ]
      }
    },
    "status": {
      "code": "delivered",
      "delivered": true,
      "result": {
        "type": "unknown"
      }
    },
    "signatures": [...],
    "produced": [...]
  }
}
```

**MCP Tool Parameters:**
```typescript
{
  txid: string              // Required: Transaction ID/hash
  include_receipt?: boolean // Optional: Include Merkle receipt
  prove?: boolean          // Optional: Include proof
  wait?: string            // Optional: Wait duration (e.g., "30s")
}
```

---

### accumulate_query_chain → query (ChainQuery)

**API Version:** V3

**Method:** `query`

**Query Type:** `ChainQuery`

**Code Reference:** `pkg/api/v3/query.go:Querier.Query()`

**Request Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "query",
  "params": {
    "scope": "acc://alice.acme/tokens",
    "query": {
      "type": "Chain",
      "name": "main",
      "range": {
        "start": 0,
        "count": 10
      },
      "expand": true,
      "includeReceipt": false
    }
  }
}
```

**Response Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "type": "chain",
    "name": "main",
    "total": 1000,
    "start": 0,
    "count": 10,
    "entries": [
      {
        "height": 0,
        "entry": "abc123...",
        "value": {
          "type": "transaction",
          "txid": "abc123..."
        }
      }
    ]
  }
}
```

**MCP Tool Parameters:**
```typescript
{
  url: string              // Required: Account URL
  chain_name: string       // Required: Chain name ("main", "signature", etc.)
  start?: number           // Optional: Start index (default: 0)
  count?: number           // Optional: Entry count (default: 10, max: 100)
  expand?: boolean         // Optional: Expand entries (default: false)
  include_receipt?: boolean // Optional: Include receipts
}
```

**Common Chain Names:**
- `main` - Main transaction chain
- `signature` - Signature chain
- `scratch` - Scratch chain (temporary data)
- `pending` - Pending transactions
- `anchor` - Anchor chain (for partitions)

---

### accumulate_query_data → query (DataQuery)

**API Version:** V3

**Method:** `query`

**Query Type:** `DataQuery`

**Code Reference:** `pkg/api/v3/query.go:Querier.Query()`

**Request Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "query",
  "params": {
    "scope": "acc://alice.acme/data",
    "query": {
      "type": "Data",
      "entryHash": "",
      "range": {
        "start": 0,
        "count": 10
      },
      "expand": true
    }
  }
}
```

**Response Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "type": "dataSet",
    "total": 50,
    "start": 0,
    "count": 10,
    "entries": [
      {
        "entryHash": "abc123...",
        "entry": {
          "data": ["SGVsbG8gV29ybGQ="]
        }
      }
    ]
  }
}
```

**MCP Tool Parameters:**
```typescript
{
  url: string              // Required: Data account URL
  entry_hash?: string      // Optional: Specific entry hash
  start?: number           // Optional: Start index
  count?: number           // Optional: Entry count
  expand?: boolean         // Optional: Expand entry data
}
```

---

### accumulate_query_directory → query (DirectoryQuery)

**API Version:** V3

**Method:** `query`

**Query Type:** `DirectoryQuery`

**Code Reference:** `pkg/api/v3/query.go:Querier.Query()`

**Request Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "query",
  "params": {
    "scope": "acc://alice.acme",
    "query": {
      "type": "Directory",
      "range": {
        "start": 0,
        "count": 20
      },
      "expand": true,
      "includeReceipt": false
    }
  }
}
```

**Response Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "type": "directory",
    "total": 5,
    "start": 0,
    "count": 5,
    "entries": [
      {
        "url": "acc://alice.acme/tokens",
        "type": "tokenAccount"
      },
      {
        "url": "acc://alice.acme/data",
        "type": "dataAccount"
      },
      {
        "url": "acc://alice.acme/book",
        "type": "keyBook"
      }
    ]
  }
}
```

**MCP Tool Parameters:**
```typescript
{
  url: string              // Required: Directory/ADI URL
  start?: number           // Optional: Start index
  count?: number           // Optional: Entry count (max: 100)
  expand?: boolean         // Optional: Include full account data
}
```

---

### accumulate_search_accounts → query (SearchQuery)

**API Version:** V3

**Method:** `query`

**Query Type:** `SearchQuery`

**Code Reference:** `pkg/api/v3/query.go:Querier.Query()`

**Request Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "query",
  "params": {
    "scope": "acc://alice.*",
    "query": {
      "type": "Search",
      "filter": {
        "types": ["tokenAccount"]
      },
      "range": {
        "start": 0,
        "count": 10
      }
    }
  }
}
```

**Response Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "type": "searchResult",
    "total": 3,
    "start": 0,
    "count": 3,
    "records": [
      {
        "url": "acc://alice.acme/tokens",
        "type": "tokenAccount"
      }
    ]
  }
}
```

---

### accumulate_query_block → query (BlockQuery)

**API Version:** V3

**Method:** `query`

**Query Type:** `BlockQuery`

**Code Reference:** `pkg/api/v3/query.go:Querier.Query()`

**Request Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "query",
  "params": {
    "scope": "acc://bvn-BVN0.acme",
    "query": {
      "type": "Block",
      "height": 1000000,
      "blockHash": "",
      "includeEntries": true
    }
  }
}
```

**Response Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "type": "block",
    "height": 1000000,
    "blockHash": "abc123...",
    "timestamp": "2024-10-20T12:00:00Z",
    "entries": [
      {
        "type": "transaction",
        "txid": "def456..."
      }
    ]
  }
}
```

---

### accumulate_query_pending → query (PendingQuery)

**API Version:** V3

**Method:** `query`

**Query Type:** `PendingQuery`

**Request Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "query",
  "params": {
    "scope": "acc://alice.acme/tokens",
    "query": {
      "type": "Pending",
      "range": {
        "start": 0,
        "count": 10
      }
    }
  }
}
```

**Response Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "type": "recordSet",
    "total": 2,
    "start": 0,
    "count": 2,
    "records": [
      {
        "type": "pending",
        "txid": "abc123..."
      }
    ]
  }
}
```

---

## 3. Transaction Operations

### accumulate_submit_transaction → submit

**API Version:** V3

**Method:** `submit`

**Code Reference:** `pkg/api/v3/submit.go:Submitter.Submit()`

**Request Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "submit",
  "params": {
    "envelope": {
      "transaction": [
        {
          "header": {
            "principal": "acc://alice.acme/tokens",
            "initiator": "abc123..."
          },
          "body": {
            "type": "sendTokens",
            "to": [
              {
                "url": "acc://bob.acme/tokens",
                "amount": "1000000000"
              }
            ]
          }
        }
      ],
      "signatures": [
        {
          "type": "ed25519",
          "publicKey": "0123456789abcdef...",
          "signature": "fedcba9876543210...",
          "signer": "acc://alice.acme/book/1",
          "signerVersion": 1,
          "timestamp": 1697123456
        }
      ]
    },
    "checkOnly": false
  }
}
```

**Response Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "txid": "abc123def456...",
    "txHash": "fedcba987654...",
    "status": {
      "code": "pending",
      "delivered": false
    },
    "message": "Transaction submitted successfully"
  }
}
```

**REST Equivalent:**
```bash
POST /submit
Content-Type: application/json

{envelope...}
```

**MCP Tool Parameters:**
```typescript
{
  envelope: object | string  // Required: Signed envelope (JSON or hex/base64)
  check_only?: boolean       // Optional: Validate without submitting
}
```

**Important Notes:**
- Envelope must be fully signed
- MCP server does NOT sign transactions
- Signing must be done externally with private keys

---

### accumulate_validate_transaction → validate

**API Version:** V3

**Method:** `validate`

**Code Reference:** `pkg/api/v3/validate.go:Validator.Validate()`

**Request Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "validate",
  "params": {
    "envelope": {...}
  }
}
```

**Response Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "ok": true,
    "results": [
      {
        "ok": true,
        "message": "Transaction is valid"
      }
    ]
  }
}
```

**REST Equivalent:**
```bash
POST /validate
```

---

### accumulate_faucet → faucet

**API Version:** V3

**Method:** `faucet`

**Code Reference:** `pkg/api/v3/faucet.go:Faucet.Faucet()`

**Request Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "faucet",
  "params": {
    "account": "acc://alice.acme/tokens"
  }
}
```

**Response Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "txid": "abc123...",
    "amount": "10000000000",
    "message": "Faucet dispense successful"
  }
}
```

**REST Equivalent:**
```bash
POST /faucet
Content-Type: application/json

{"account": "acc://alice.acme/tokens"}
```

**Notes:**
- Only works on testnet
- Rate limited per account
- Typical dispense: 10 ACME

---

## 4. Event Subscriptions

### accumulate_subscribe_events → subscribe

**API Version:** V3

**Service:** Event Service

**Code Reference:** `pkg/api/v3/events.go:EventService.Subscribe()`

**WebSocket Connection Required**

**Request Example:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "subscribe",
  "params": {
    "account": "acc://alice.acme/tokens",
    "types": ["Block", "Transaction"]
  }
}
```

**Response Example (subscription confirmation):**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "subscriptionID": "sub-abc123",
    "message": "Subscribed to events"
  }
}
```

**Event Stream Example:**
```json
{
  "jsonrpc": "2.0",
  "method": "event",
  "params": {
    "subscriptionID": "sub-abc123",
    "event": {
      "type": "Transaction",
      "txid": "def456...",
      "status": "delivered"
    }
  }
}
```

**Event Types:**
- `Block` - New blocks
- `Transaction` - Transaction updates
- `Error` - Error events

---

## 5. Query Type Summary

| Query Type | MCP Tool | Primary Use Case |
|------------|----------|------------------|
| DefaultQuery | accumulate_query_account | Account/transaction lookup |
| ChainQuery | accumulate_query_chain | Chain entry traversal |
| DataQuery | accumulate_query_data | Data account entries |
| DirectoryQuery | accumulate_query_directory | List ADI contents |
| SearchQuery | accumulate_search_accounts | Pattern-based search |
| BlockQuery | accumulate_query_block | Block information |
| AnchorSearchQuery | accumulate_query_anchors | Cross-chain anchors |
| PublicKeySearchQuery | accumulate_query_key_index | Find key by public key |
| PublicKeyHashSearchQuery | accumulate_query_public_key_hash | Find by key hash |
| DelegateSearchQuery | accumulate_query_delegate | Authority delegation |
| PendingQuery | accumulate_query_pending | Pending transactions |

---

## 6. Transaction Type Summary

All transaction building tools use protocol types from `protocol/`:

| Transaction Type | MCP Builder Tool | Protocol Type | Source File |
|------------------|------------------|---------------|-------------|
| Send Tokens | accumulate_build_send_tokens | SendTokens | protocol/send_tokens.go |
| Create Identity | accumulate_build_create_account | CreateIdentity | protocol/create_identity.go |
| Create Token Account | accumulate_build_create_account | CreateTokenAccount | protocol/create_token_account.go |
| Create Data Account | accumulate_build_create_account | CreateDataAccount | protocol/create_data_account.go |
| Update Account Auth | accumulate_build_update_account | UpdateAccountAuth | protocol/update_account_auth.go |
| Update Key Page | accumulate_build_update_account | UpdateKeyPage | protocol/update_key_page.go |
| Write Data | accumulate_build_write_data | WriteData | protocol/write_data.go |
| Write Data To | accumulate_build_write_data | WriteDataTo | protocol/write_data_to.go |
| Issue Tokens | accumulate_build_token_issuance | IssueTokens | protocol/issue_tokens.go |
| Burn Tokens | accumulate_build_burn_tokens | BurnTokens | protocol/burn_tokens.go |
| Create Token | accumulate_build_create_account | CreateToken | protocol/create_token.go |
| Add Credits | (future) | AddCredits | protocol/add_credits.go |
| Update Key | (future) | UpdateKey | protocol/update_key.go |

Full list of transaction types: See `protocol/types.yml` (60+ types)

---

## 7. V2 API Compatibility Mapping

For backward compatibility with V2 API:

| V2 Method | V3 Equivalent | MCP Tool |
|-----------|---------------|----------|
| query | query (DefaultQuery) | accumulate_query_account |
| query-tx | query (txid scope) | accumulate_query_transaction |
| query-tx-history | query (ChainQuery) | accumulate_query_chain |
| query-data | query (DataQuery) | accumulate_query_data |
| query-directory | query (DirectoryQuery) | accumulate_query_directory |
| execute | submit | accumulate_submit_transaction |
| validate | validate | accumulate_validate_transaction |
| faucet | faucet | accumulate_faucet |
| version | node-info | accumulate_node_info |
| describe | network-status | accumulate_network_status |
| metrics | metrics | accumulate_metrics |

**V2 Endpoint:** `/v2` (JSON-RPC only)

**Migration Recommendation:** Use V3 API for new implementations

---

## 8. Code Reference Map

### API Client Packages

```
pkg/api/v3/
├── api.go              # Main API interface definitions
├── node.go             # NodeService implementation
├── consensus.go        # ConsensusService implementation
├── network.go          # NetworkService implementation
├── metrics.go          # MetricsService implementation
├── query.go            # Querier implementation
├── submit.go           # Submitter implementation
├── validate.go         # Validator implementation
├── faucet.go           # Faucet implementation
├── events.go           # EventService implementation
├── snapshot.go         # SnapshotService implementation
├── client/             # HTTP/WebSocket clients
├── message/            # Message types
└── tm/                 # Tendermint integration
```

### Protocol Types

```
protocol/
├── types.yml           # All transaction/account type definitions
├── send_tokens.go      # SendTokens transaction
├── create_identity.go  # CreateIdentity transaction
├── write_data.go       # WriteData transaction
├── account.go          # Base account types
├── token_account.go    # TokenAccount type
├── data_account.go     # DataAccount type
├── key_page.go         # KeyPage type
└── [60+ transaction type files]
```

### URL Handling

```
pkg/url/
├── url.go              # URL parsing and validation
├── txid.go             # Transaction ID URLs
└── query.go            # Query URL construction
```

### Message Types

```
pkg/types/messaging/
├── envelope.go         # Transaction envelopes
├── signature.go        # Signature types
└── batch.go            # Batch processing
```

---

## 9. Error Code Reference

Common API errors that MCP tools should handle:

| Error Code | HTTP Status | Description | MCP Handling |
|------------|-------------|-------------|--------------|
| NotFound | 404 | Account/transaction not found | Return null or specific error |
| BadRequest | 400 | Invalid parameters | Validate inputs |
| Unauthorized | 401 | Missing/invalid auth | Return auth error |
| Forbidden | 403 | Action not permitted | Return permission error |
| InternalError | 500 | Server error | Retry with backoff |
| ServiceUnavailable | 503 | Service down | Retry with backoff |
| Timeout | 408 | Request timeout | Retry or return timeout error |

**Error Response Format:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32000,
    "message": "Account not found",
    "data": {
      "url": "acc://invalid.acme",
      "suggestion": "Check URL spelling"
    }
  }
}
```

---

## 10. Testing Endpoints

### Testnet
```
Primary: https://testnet.accumulatenetwork.io/v3
Backup:  https://testnet-api.accumulate.defidevs.io/v3
```

### Devnet (Local)
```
Default: http://localhost:26660/v3
DN Node: http://localhost:16592/v3
BVN Node: http://localhost:16692/v3
```

### Health Check
```bash
curl https://mainnet.accumulatenetwork.io/v3/node/info
```

---

## 11. Implementation Checklist

For each MCP tool:

- [ ] Define tool schema (name, parameters, description)
- [ ] Map to correct Accumulate API endpoint
- [ ] Implement request building
- [ ] Implement response parsing
- [ ] Add error handling
- [ ] Add parameter validation
- [ ] Write unit tests
- [ ] Write integration tests
- [ ] Add usage examples
- [ ] Document in MCP catalog

---

## References

- V3 API Implementation: `pkg/api/v3/`
- Protocol Types: `protocol/`
- URL Handling: `pkg/url/`
- Message Types: `pkg/types/messaging/`
- API Tests: `test/e2e/`

## Version History

- **v1.0** (2025-10-20): Initial API mapping reference
  - Complete V3 API mapping
  - All query types documented
  - Transaction operations mapped
  - Code references added
