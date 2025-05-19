---
title: CometBFT APIs in Accumulate
description: A comprehensive overview of CometBFT APIs integrated with Accumulate
tags: [accumulate, api, cometbft, tendermint, consensus]
created: 2025-05-16
version: 1.0
---

# CometBFT APIs in Accumulate

## 1. Introduction

CometBFT (formerly Tendermint) is the consensus engine that powers Accumulate. It provides its own set of APIs that are distinct from Accumulate's application-level APIs. This document provides an overview of the CometBFT APIs available in Accumulate, how they're integrated, and how they can be used.

## 2. CometBFT API Architecture

### 2.1 Overview

CometBFT exposes several APIs through different interfaces:

1. **RPC API**: HTTP and WebSocket endpoints for interacting with the consensus engine
2. **ABCI (Application Blockchain Interface)**: Interface between CometBFT and Accumulate
3. **P2P Protocol**: Communication between CometBFT nodes

### 2.2 Integration with Accumulate

Accumulate integrates with CometBFT primarily through:

1. **ABCI Implementation**: Accumulate implements the ABCI interface to interact with CometBFT
2. **RPC Client**: Accumulate uses CometBFT's RPC client to query consensus state
3. **Proxy APIs**: Some Accumulate APIs proxy requests to CometBFT's RPC API

## 3. CometBFT RPC API

### 3.1 Endpoints

CometBFT exposes a comprehensive RPC API with endpoints for various purposes:

#### 3.1.1 Consensus State

- `/status`: Get node status
- `/blockchain`: Get blockchain info
- `/block`: Get block at a specific height
- `/block_by_hash`: Get block by hash
- `/block_results`: Get block results at a specific height
- `/commit`: Get commit at a specific height
- `/validators`: Get validator set at a specific height
- `/consensus_state`: Get consensus state
- `/consensus_params`: Get consensus parameters at a specific height
- `/consensus_params`: Get consensus parameters at a specific height

#### 3.1.2 Transaction Management

- `/broadcast_tx_sync`: Submit a transaction and wait for it to be checked
- `/broadcast_tx_async`: Submit a transaction and return immediately
- `/broadcast_tx_commit`: Submit a transaction and wait for it to be committed
- `/tx`: Get transaction by hash
- `/tx_search`: Search for transactions with specific criteria

#### 3.1.3 Network Information

- `/net_info`: Get network information
- `/dial_seeds`: Connect to seeds
- `/dial_peers`: Connect to peers
- `/genesis`: Get genesis file
- `/health`: Get node health
- `/unconfirmed_txs`: Get unconfirmed transactions

#### 3.1.4 ABCI Queries

- `/abci_query`: Query the application state
- `/abci_info`: Get application information

### 3.2 Access in Accumulate

In Accumulate, CometBFT's RPC API is typically exposed on port 26657 (default) and can be accessed directly:

```bash
# Get node status
curl -X GET "http://localhost:26657/status"

# Get block at height 1000
curl -X GET "http://localhost:26657/block?height=1000"

# Submit a transaction
curl -X POST "http://localhost:26657/broadcast_tx_commit" \
  --data-binary '{"jsonrpc":"2.0","id":1,"method":"broadcast_tx_commit","params":{"tx":"<tx_bytes>"}}'
```

## 4. ABCI Interface

### 4.1 Overview

The Application Blockchain Interface (ABCI) is the interface between CometBFT (consensus engine) and Accumulate (application logic). It consists of several methods that CometBFT calls at different points in the consensus process.

### 4.2 Key ABCI Methods

#### 4.2.1 Consensus Methods

- `InitChain`: Called once when the blockchain is initialized
- `BeginBlock`: Called at the beginning of each block
- `DeliverTx`: Called for each transaction in the block
- `EndBlock`: Called at the end of each block
- `Commit`: Called to commit the block state

#### 4.2.2 Mempool Methods

- `CheckTx`: Called to validate transactions before they enter the mempool

#### 4.2.3 Query Methods

- `Query`: Called to query the application state

#### 4.2.4 State Sync Methods

- `ListSnapshots`: Returns a list of available snapshots
- `LoadSnapshotChunk`: Returns a chunk of a snapshot
- `OfferSnapshot`: Offers a snapshot to the application
- `ApplySnapshotChunk`: Applies a chunk of a snapshot

### 4.3 Accumulate's ABCI Implementation

Accumulate implements the ABCI interface in the `internal/node/abci` package. The main implementation is in the `Accumulator` struct, which handles all ABCI methods:

```go
// Accumulator implements the ABCI interface for Accumulate
type Accumulator struct {
    // Fields...
}

// InitChain initializes the blockchain
func (app *Accumulator) InitChain(ctx context.Context, req *abci.RequestInitChain) (*abci.ResponseInitChain, error) {
    // Implementation...
}

// CheckTx validates a transaction before it enters the mempool
func (app *Accumulator) CheckTx(ctx context.Context, req *abci.RequestCheckTx) (*abci.ResponseCheckTx, error) {
    // Implementation...
}

// DeliverTx processes a transaction during block creation
func (app *Accumulator) DeliverTx(ctx context.Context, req *abci.RequestDeliverTx) (*abci.ResponseDeliverTx, error) {
    // Implementation...
}

// Commit commits the block
func (app *Accumulator) Commit(ctx context.Context, req *abci.RequestCommit) (*abci.ResponseCommit, error) {
    // Implementation...
}

// Query queries the application state
func (app *Accumulator) Query(ctx context.Context, req *abci.RequestQuery) (*abci.ResponseQuery, error) {
    // Implementation...
}

// Other ABCI methods...
```

## 5. State Sync APIs

### 5.1 Overview

State sync is a feature of CometBFT that allows a node to quickly catch up to the current state of the blockchain by downloading and applying a snapshot, rather than replaying all blocks from genesis.

### 5.2 State Sync ABCI Methods

As mentioned in section 4.2.4, CometBFT provides four ABCI methods for state sync:

```go
// ListSnapshots returns a list of available snapshots
func (app *Accumulator) ListSnapshots(ctx context.Context, req *abci.RequestListSnapshots) (*abci.ResponseListSnapshots, error) {
    // Implementation...
}

// LoadSnapshotChunk returns a chunk of a snapshot
func (app *Accumulator) LoadSnapshotChunk(ctx context.Context, req *abci.RequestLoadSnapshotChunk) (*abci.ResponseLoadSnapshotChunk, error) {
    // Implementation...
}

// OfferSnapshot offers a snapshot to the application
func (app *Accumulator) OfferSnapshot(ctx context.Context, req *abci.RequestOfferSnapshot) (*abci.ResponseOfferSnapshot, error) {
    // Implementation...
}

// ApplySnapshotChunk applies a chunk of a snapshot
func (app *Accumulator) ApplySnapshotChunk(ctx context.Context, req *abci.RequestApplySnapshotChunk) (*abci.ResponseApplySnapshotChunk, error) {
    // Implementation...
}
```

### 5.3 State Sync Configuration

To use state sync, you need to configure CometBFT in the `config.toml` file:

```toml
[statesync]
enable = true
rpc_servers = "node1.example.com:26657,node2.example.com:26657"
trust_height = 1000000
trust_hash = "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
```

## 6. P2P Protocol

### 6.1 Overview

CometBFT nodes communicate with each other using a P2P protocol. This protocol is used for:

1. **Block propagation**: Sharing new blocks
2. **Transaction propagation**: Sharing new transactions
3. **State sync**: Downloading snapshots
4. **Peer discovery**: Finding other nodes in the network

### 6.2 P2P Configuration

The P2P protocol can be configured in the `config.toml` file:

```toml
[p2p]
laddr = "tcp://0.0.0.0:26656"
seeds = "seed1.example.com:26656,seed2.example.com:26656"
persistent_peers = "peer1.example.com:26656,peer2.example.com:26656"
```

## 7. CometBFT Events

### 7.1 Overview

CometBFT emits events at various points in the consensus process. These events can be subscribed to using the WebSocket API.

### 7.2 Event Types

- `NewBlock`: Emitted when a new block is committed
- `Tx`: Emitted when a transaction is included in a block
- `ValidatorSetUpdates`: Emitted when the validator set changes
- Custom events emitted by the application

### 7.3 Subscribing to Events

You can subscribe to events using the WebSocket API:

```
ws://localhost:26657/websocket
```

Example subscription:

```json
{
  "jsonrpc": "2.0",
  "method": "subscribe",
  "id": 1,
  "params": {
    "query": "tm.event='NewBlock'"
  }
}
```

## 8. Accumulate's Integration with CometBFT APIs

### 8.1 Consensus Service

Accumulate's `ConsensusService` interface provides methods that interact with CometBFT's APIs:

```go
type ConsensusService interface {
    // ConsensusStatus returns the status of the consensus node.
    ConsensusStatus(ctx context.Context, opts ConsensusStatusOptions) (*ConsensusStatus, error)
}
```

The implementation of this interface typically queries CometBFT's `/status` endpoint to get consensus information.

### 8.2 Block Queries

Accumulate's `BlockQuery` type allows querying blocks, which ultimately translates to queries to CometBFT's block-related endpoints:

```go
BlockQuery:
  union: { type: query }
  custom-is-valid: true
  fields:
    - name: Minor
      type: uint
      pointer: true
      optional: true
    - name: Major
      type: uint
      pointer: true
      optional: true
    // Other fields...
```

### 8.3 Event Subscription

Accumulate's `EventService` interface provides a way to subscribe to events, which may include events from CometBFT:

```go
type EventService interface {
    // Subscribe subscribes to event notifications.
    Subscribe(ctx context.Context, opts SubscribeOptions) (<-chan Event, error)
}
```

## 9. Accessing CometBFT APIs Directly

While Accumulate provides its own APIs that abstract many CometBFT functionalities, you can also access CometBFT's APIs directly:

### 9.1 RPC API

```bash
# Get node status
curl -X GET "http://localhost:26657/status"

# Get the latest block
curl -X GET "http://localhost:26657/block"

# Get block at height 1000
curl -X GET "http://localhost:26657/block?height=1000"
```

### 9.2 WebSocket API

```bash
# Connect to the WebSocket endpoint
wscat -c ws://localhost:26657/websocket

# Subscribe to new block events
{"jsonrpc":"2.0","method":"subscribe","id":1,"params":{"query":"tm.event='NewBlock'"}}
```

## 10. Best Practices

### 10.1 When to Use CometBFT APIs Directly

- When you need low-level access to consensus information
- When you need to subscribe to specific CometBFT events
- When you're developing tools that interact directly with the consensus layer

### 10.2 When to Use Accumulate's APIs

- For most application-level interactions
- When you need to query account state, submit transactions, etc.
- When you want a more abstracted and consistent interface

### 10.3 Security Considerations

- CometBFT's RPC API should not be exposed to the public internet without proper security measures
- Consider using a reverse proxy with authentication for public-facing deployments
- Limit the exposed endpoints to only those necessary for your application

## 11. Conclusion

CometBFT provides a robust set of APIs for interacting with the consensus layer of Accumulate. While Accumulate's own APIs abstract many of these functionalities, understanding the underlying CometBFT APIs can be valuable for certain use cases.

In the following documents, we will explore specific aspects of Accumulate's APIs in more detail, including how they interact with CometBFT's APIs under the hood.
