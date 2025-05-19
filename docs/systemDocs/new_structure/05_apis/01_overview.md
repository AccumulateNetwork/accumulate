---
title: Accumulate API Overview
description: A comprehensive overview of all APIs available in Accumulate
tags: [accumulate, api, rest, jsonrpc, websocket, p2p]
created: 2025-05-16
version: 1.0
---

# Accumulate API Overview

## 1. Introduction

Accumulate provides a comprehensive set of APIs that allow developers to interact with the network. These APIs are designed to be flexible, consistent, and easy to use. This document provides an overview of all available APIs in Accumulate, their purposes, and how they can be accessed.

## 2. API Architecture

### 2.1 API Layers

Accumulate's API architecture consists of several layers:

1. **Core API Interfaces**: Defined in `pkg/api/v3/api.go`, these interfaces define the core functionality of each API service.
2. **Transport Layers**: Multiple transport mechanisms are supported, including:
   - REST API
   - JSON-RPC
   - WebSocket
   - P2P (Peer-to-Peer)
3. **Client Libraries**: Various client libraries that implement these APIs for different programming languages.

### 2.2 API Versioning

Accumulate uses explicit versioning for its APIs:

- **v3**: The current API version, located in `pkg/api/v3/`
- Previous versions are considered deprecated

## 3. Core API Services

Accumulate provides the following core API services:

### 3.1 Node Service

**Interface**: `api.NodeService`

**Purpose**: Provides information about network nodes and services.

**Methods**:
- `NodeInfo(ctx context.Context, opts NodeInfoOptions) (*NodeInfo, error)`: Returns information about the network node.
- `FindService(ctx context.Context, opts FindServiceOptions) ([]*FindServiceResult, error)`: Searches for nodes that provide the given service.

### 3.2 Consensus Service

**Interface**: `api.ConsensusService`

**Purpose**: Provides information about the consensus state of the network.

**Methods**:
- `ConsensusStatus(ctx context.Context, opts ConsensusStatusOptions) (*ConsensusStatus, error)`: Returns the status of the consensus node.

### 3.3 Network Service

**Interface**: `api.NetworkService`

**Purpose**: Provides information about the overall network status.

**Methods**:
- `NetworkStatus(ctx context.Context, opts NetworkStatusOptions) (*NetworkStatus, error)`: Returns the status of the network.

### 3.4 Snapshot Service

**Interface**: `api.SnapshotService`

**Purpose**: Provides access to blockchain state snapshots.

**Methods**:
- `ListSnapshots(ctx context.Context, opts ListSnapshotsOptions) ([]*SnapshotInfo, error)`: Lists available snapshots.

### 3.5 Metrics Service

**Interface**: `api.MetricsService`

**Purpose**: Provides network performance metrics.

**Methods**:
- `Metrics(ctx context.Context, opts MetricsOptions) (*Metrics, error)`: Returns network metrics such as transactions per second.

### 3.6 Querier

**Interface**: `api.Querier`

**Purpose**: Allows querying the state of accounts and transactions.

**Methods**:
- `Query(ctx context.Context, scope *url.URL, query Query) (Record, error)`: Queries the state of an account or transaction.

### 3.7 Event Service

**Interface**: `api.EventService`

**Purpose**: Provides subscription to network events.

**Methods**:
- `Subscribe(ctx context.Context, opts SubscribeOptions) (<-chan Event, error)`: Subscribes to event notifications.

### 3.8 Submitter

**Interface**: `api.Submitter`

**Purpose**: Allows submitting transactions to the network.

**Methods**:
- `Submit(ctx context.Context, envelope *messaging.Envelope, opts SubmitOptions) ([]*Submission, error)`: Submits an envelope for execution.

### 3.9 Validator

**Interface**: `api.Validator`

**Purpose**: Validates transactions before submission.

**Methods**:
- `Validate(ctx context.Context, envelope *messaging.Envelope, opts ValidateOptions) ([]*Submission, error)`: Checks if an envelope is expected to succeed.

### 3.10 Faucet

**Interface**: `api.Faucet`

**Purpose**: Provides access to the network faucet for obtaining test tokens.

**Methods**:
- `Faucet(ctx context.Context, account *url.URL, opts FaucetOptions) (*Submission, error)`: Requests tokens from the faucet.

## 4. Query Types

Accumulate supports various query types for retrieving different kinds of data:

### 4.1 Default Query

**Purpose**: Basic query with optional receipt inclusion.

**Fields**:
- `IncludeReceipt`: Optional receipt options.

### 4.2 Chain Query

**Purpose**: Query chain data.

**Fields**:
- `Name`: Chain name.
- `Index`: Optional index.
- `Entry`: Optional entry.
- `Range`: Optional range options.
- `IncludeReceipt`: Optional receipt options.

### 4.3 Data Query

**Purpose**: Query data entries.

**Fields**:
- `Index`: Optional index.
- `Entry`: Optional entry.
- `Range`: Optional range options.

### 4.4 Directory Query

**Purpose**: Query directory entries.

**Fields**:
- `Range`: Range options.

### 4.5 Pending Query

**Purpose**: Query pending transactions.

**Fields**:
- `Range`: Range options.

### 4.6 Block Query

**Purpose**: Query blocks.

**Fields**:
- `Minor`: Optional minor block number.
- `Major`: Optional major block number.
- `MinorRange`: Optional minor block range.
- `MajorRange`: Optional major block range.
- `EntryRange`: Optional entry range.
- `OmitEmpty`: Optional flag to omit empty blocks.

### 4.7 Search Queries

Several specialized search queries are available:

- **Anchor Search Query**: Searches for anchors.
- **Public Key Search Query**: Searches by public key.
- **Public Key Hash Search Query**: Searches by public key hash.
- **Delegate Search Query**: Searches by delegate.
- **Message Hash Search Query**: Searches by message hash.

## 5. Transport Layers

### 5.1 REST API

**Implementation**: `pkg/api/v3/rest/`

**Endpoints**:
- `/node/info`: Get node information.
- `/node/services`: Find services.
- `/consensus/status`: Get consensus status.
- `/network/status`: Get network status.
- `/metrics`: Get network metrics.
- `/submit`: Submit a transaction.
- `/validate`: Validate a transaction.
- `/faucet`: Request tokens from the faucet.
- `/query/{account}`: Query an account (implemented in `query.go`).

### 5.2 JSON-RPC API

**Implementation**: `pkg/api/v3/jsonrpc/`

**Methods**:
- `node-info`: Get node information.
- `find-service`: Find services.
- `consensus-status`: Get consensus status.
- `network-status`: Get network status.
- `list-snapshots`: List snapshots.
- `metrics`: Get network metrics.
- `query`: Query an account or transaction.
- `submit`: Submit a transaction.
- `validate`: Validate a transaction.
- `faucet`: Request tokens from the faucet.
- `private-sequence`: (Private API) Sequence transactions.

### 5.3 WebSocket API

**Implementation**: `pkg/api/v3/websocket/`

Provides real-time communication with the network, particularly useful for subscribing to events.

### 5.4 P2P API

**Implementation**: `pkg/api/v3/p2p/`

Enables peer-to-peer communication between nodes in the network.

## 6. Ethereum API

**Implementation**: `pkg/api/ethereum/`

Provides Ethereum-compatible JSON-RPC endpoints for interacting with Accumulate using Ethereum tooling.

## 7. Message API

**Implementation**: `pkg/api/v3/message/`

Defines message types and services for communication between nodes.

## 8. API Usage Examples

### 8.1 REST API Example

```bash
# Get node information
curl -X GET "http://localhost:26660/node/info"

# Submit a transaction
curl -X POST "http://localhost:26660/submit" \
  -H "Content-Type: application/json" \
  -d '{"envelope": {...}}'

# Query an account
curl -X GET "http://localhost:26660/query/acc://example.acme"
```

### 8.2 JSON-RPC Example

```json
// Get node information
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "node-info",
  "params": {}
}

// Submit a transaction
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "submit",
  "params": {
    "envelope": {...},
    "wait": true
  }
}

// Query an account
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "query",
  "params": {
    "scope": "acc://example.acme",
    "query": {
      "type": "default"
    }
  }
}
```

## 9. API Security

Accumulate APIs implement several security measures:

1. **Request Validation**: All requests are validated before processing.
2. **Error Handling**: Consistent error handling across all APIs.
3. **Rate Limiting**: Protection against DoS attacks.
4. **Authentication**: Some APIs may require authentication.

## 10. API Extensibility

The Accumulate API architecture is designed to be extensible:

1. **New Services**: New services can be added by implementing the appropriate interfaces.
2. **New Transport Layers**: New transport mechanisms can be added.
3. **API Versioning**: New API versions can be introduced without breaking existing clients.

## 11. Conclusion

Accumulate provides a comprehensive set of APIs that enable developers to interact with the network in various ways. The APIs are designed to be flexible, consistent, and easy to use, with multiple transport layers to accommodate different use cases.

In the following documents, we will explore each API service in more detail, including request and response formats, examples, and best practices.
