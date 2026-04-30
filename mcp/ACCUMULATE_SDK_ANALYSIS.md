# Accumulate SDK v1.4.2 - Comprehensive API Analysis

## Overview
This document provides a complete analysis of the Accumulate SDK's API, RPC methods, transaction types, query types, and available services. The SDK uses API v3 as its primary interface.

## API Versions

### Current Version: v3
- **Location**: `/pkg/api/v3/`
- **Status**: Active, primary API
- **Description**: Completely redesigned API splitting functionality along call types and transport mechanisms

### Legacy Version: v2
- Referenced in documentation but superseded by v3
- Still present in codebase but not the focus

---

## Services

The API is organized into services, each implementing a single method. This allows independent implementation and flexible middleware support.

### 1. NodeService
**Interface Methods:**
- `NodeInfo(ctx context.Context, opts NodeInfoOptions) (*NodeInfo, error)`
  - Returns information about the network node
  - JSON-RPC: `node-info`
  - REST: `GET /node/info`
  
- `FindService(ctx context.Context, opts FindServiceOptions) ([]*FindServiceResult, error)`
  - Searches for nodes providing a given service
  - JSON-RPC: `find-service`
  - REST: `GET /node/services`

### 2. ConsensusService
**Interface Methods:**
- `ConsensusStatus(ctx context.Context, opts ConsensusStatusOptions) (*ConsensusStatus, error)`
  - Returns consensus node status
  - Includes last block, version, commit hash, node/validator keys, partition info, and peer information
  - JSON-RPC: `consensus-status`
  - REST: `GET /consensus/status`

### 3. NetworkService
**Interface Methods:**
- `NetworkStatus(ctx context.Context, opts NetworkStatusOptions) (*NetworkStatus, error)`
  - Returns active network global variables
  - Includes oracle data, globals, network definition, routing table, executor version
  - JSON-RPC: `network-status`
  - REST: `GET /network/status`

### 4. SnapshotService
**Interface Methods:**
- `ListSnapshots(ctx context.Context, opts ListSnapshotsOptions) ([]*SnapshotInfo, error)`
  - Lists available snapshots
  - JSON-RPC: `list-snapshots`

### 5. MetricsService
**Interface Methods:**
- `Metrics(ctx context.Context, opts MetricsOptions) (*Metrics, error)`
  - Returns network metrics (transactions/chain entries per second)
  - Calculated from last N blocks (configurable span)
  - JSON-RPC: `metrics`
  - REST: `GET /metrics`

### 6. Querier
**Interface Methods:**
- `Query(ctx context.Context, scope *url.URL, query Query) (Record, error)`
  - Main query interface for retrieving account and transaction data
  - Accepts a scope (URL) and structured query
  - Returns varied record types based on query type

### 7. EventService
**Interface Methods:**
- `Subscribe(ctx context.Context, opts SubscribeOptions) (<-chan Event, error)`
  - Subscribes to event notifications
  - Channel closes when context is canceled
  - Implementation incomplete and subject to change

### 8. Submitter
**Interface Methods:**
- `Submit(ctx context.Context, envelope *messaging.Envelope, opts SubmitOptions) ([]*Submission, error)`
  - Submits an envelope for execution
  - Can verify envelope and optionally wait for acceptance/rejection
  - JSON-RPC: `submit`
  - REST: `POST /submit`

### 9. Validator
**Interface Methods:**
- `Validate(ctx context.Context, envelope *messaging.Envelope, opts ValidateOptions) ([]*Submission, error)`
  - Checks if envelope expected to succeed (runs CheckTx)
  - Can do partial or full validation
  - JSON-RPC: `validate`
  - REST: `POST /validate`

### 10. Faucet
**Interface Methods:**
- `Faucet(ctx context.Context, account *url.URL, opts FaucetOptions) (*Submission, error)`
  - Constructs and submits transaction depositing ACME into lite token account
  - JSON-RPC: `faucet`
  - REST: `POST /faucet`

### 11. Sequencer (Private)
**Interface Methods:**
- `Sequence(ctx context.Context, source, destination *url.URL, sequenceNumber uint64, opts SequenceOptions) ([]*Submission, error)`
  - Private API for sequencing
  - JSON-RPC: `private-sequence`

---

## Query Types

All query types implement the `Query` interface with `QueryType()` method and `IsValid()` validation.

### Standard Queries (0x00-0x05)

1. **DefaultQuery** (0x00)
   - Basic query for accounts and transactions
   - Optional receipt inclusion
   - Parameters: `IncludeReceipt`

2. **ChainQuery** (0x01)
   - Query chain entries and state
   - Parameters:
     - `Name`: Chain name
     - `Index`: Entry index (optional)
     - `Entry`: Entry hash to search (optional)
     - `Range`: Range options for pagination
     - `IncludeReceipt`: Optional receipt inclusion

3. **DataQuery** (0x02)
   - Query data account entries
   - Parameters:
     - `Index`: Entry index (optional)
     - `Entry`: Entry hash to search (optional)
     - `Range`: Range options for pagination

4. **DirectoryQuery** (0x03)
   - Query identity directory entries
   - Parameters: `Range` for pagination

5. **PendingQuery** (0x04)
   - Query pending transactions on an account
   - Parameters: `Range` for pagination

6. **BlockQuery** (0x05)
   - Query minor and major blocks
   - Parameters:
     - `Minor`: Minor block number (optional)
     - `Major`: Major block number (optional)
     - `MinorRange`: Range for minor blocks
     - `MajorRange`: Range for major blocks
     - `EntryRange`: Range for entries
     - `OmitEmpty`: Omit empty blocks

### Search Queries (0x10-0x14)

7. **AnchorSearchQuery** (0x10)
   - Search for anchor in account
   - Parameters:
     - `Anchor`: Anchor hash
     - `IncludeReceipt`: Optional receipt inclusion

8. **PublicKeySearchQuery** (0x11)
   - Search for public key in account
   - Parameters:
     - `PublicKey`: Public key bytes
     - `Type`: Signature type (enum)

9. **PublicKeyHashSearchQuery** (0x12)
   - Search for public key hash
   - Parameters: `PublicKeyHash`

10. **DelegateSearchQuery** (0x13)
    - Search for delegate authority
    - Parameters: `Delegate` (URL)

11. **MessageHashSearchQuery** (0x14)
    - Search for message by hash
    - Parameters: `Hash` (32 bytes)

---

## Record Types

Record types returned by queries implementing the `Record` interface with `RecordType()` method.

### Data Records

1. **AccountRecord** (0x01)
   - Account data with optional directory and pending transactions
   - Fields: Account, Directory, Pending, Receipt, LastBlockTime

2. **ChainRecord** (0x02)
   - Chain metadata
   - Fields: Name, Type, Count, State, LastBlockTime

3. **ChainEntryRecord[T]** (0x03)
   - Chain entry with generic value type
   - Fields: Account, Name, Type, Index, Entry, Value, Receipt, State, LastBlockTime

4. **KeyRecord** (0x04)
   - Key information for searches
   - Fields: Authority, Signer, Version, Index, Entry (KeySpec)

### Message Records

5. **MessageRecord[T]** (0x10)
   - Generic message container (transactions, signatures, etc.)
   - Fields:
     - ID, Message, Status, StatusNo, Error, Result
     - Received, Produced, Cause
     - Signatures (for transactions)
     - Historical (for signature records)
     - Sequence (for sequenced messages)
     - SourceReceipt (for synthetic messages)
     - LastBlockTime

6. **SignatureSetRecord** (0x11)
   - Collection of signatures from a single authority
   - Fields: Account, Signatures

### Block Records

7. **MinorBlockRecord** (0x20)
   - Minor block with entries and anchored blocks
   - Fields: Index, Time, Source, Entries, Anchored, LastBlockTime

8. **MajorBlockRecord** (0x21)
   - Major block with minor blocks
   - Fields: Index, Time, MinorBlocks, LastBlockTime

### Utility Records

9. **RecordRange[T]** (0x80)
   - Paginated results container
   - Fields: Records, Start, Total, LastBlockTime

10. **UrlRecord** (0x81)
    - URL wrapper
    - Fields: Value (URL)

11. **TxIDRecord** (0x82)
    - Transaction ID wrapper
    - Fields: Value (TxID)

12. **IndexEntryRecord** (0x83)
    - Index entry information
    - Fields: Value (IndexEntry with source, anchor, blockIndex, blockTime, rootIndexIndex)

13. **ErrorRecord** (0x8F)
    - Error response
    - Fields: Value (Error object)

---

## Event Types

Events returned by the EventService, implementing the `Event` interface.

1. **ErrorEvent** (1)
   - Error event with error details
   - Fields: Err

2. **BlockEvent** (2)
   - Block commitment event
   - Fields:
     - Partition: Partition name
     - Index: Block index
     - Time: Block timestamp
     - Major: Major block number
     - Entries: Chain entries

3. **GlobalsEvent** (3)
   - Network globals change event
   - Fields: Old, New (GlobalValues)

---

## Transaction Types

### User Transactions (23 types)

1. **CreateIdentity** - Create new ADI with optional key book
2. **CreateTokenAccount** - Create token account with token specification
3. **SendTokens** - Transfer tokens to recipients
4. **CreateDataAccount** - Create data account
5. **WriteData** - Write data entry (scratch or state)
6. **WriteDataTo** - Write data to recipient account
7. **AcmeFaucet** - Request ACME from faucet
8. **CreateToken** - Create new token with symbol/precision
9. **IssueTokens** - Issue tokens to recipients
10. **BurnTokens** - Burn tokens
11. **CreateLiteTokenAccount** - Create lite token account (no args)
12. **CreateKeyPage** - Create key page with keys
13. **CreateKeyBook** - Create key book with optional authorities
14. **AddCredits** - Add credits to account (from ACME)
15. **BurnCredits** - Burn credits
16. **TransferCredits** - Transfer credits to recipients
17. **UpdateKeyPage** - Update key page with operations
18. **LockAccount** - Lock account until major block height
19. **UpdateAccountAuth** - Update account authorities with operations
20. **UpdateKey** - Update key hash
21. **NetworkMaintenance** - Network maintenance operations
22. **ActivateProtocolVersion** - Activate executor version
23. **RemoteTransaction** - Remote transaction with hash

### Synthetic Transactions (5 types)

1. **SyntheticCreateIdentity**
   - Created internally when transaction creates identity
   - Embeds SyntheticOrigin (cause, source, initiator, fee refund, index)
   - Fields: Accounts

2. **SyntheticWriteData**
   - Synthetic write operation
   - Embeds SyntheticOrigin
   - Fields: Entry

3. **SyntheticDepositTokens**
   - Synthetic token deposit
   - Embeds SyntheticOrigin
   - Fields: Token, Amount, IsIssuer, IsRefund

4. **SyntheticDepositCredits**
   - Synthetic credit deposit
   - Embeds SyntheticOrigin
   - Fields: Amount, AcmeRefundAmount, IsRefund

5. **SyntheticBurnTokens**
   - Synthetic token burn
   - Embeds SyntheticOrigin
   - Fields: Amount, IsRefund

6. **SyntheticForwardTransaction**
   - Forward transaction across networks
   - Fields: Signatures, Transaction

---

## Account Types

### Standard Accounts

1. **ADI (Accumulate Digital Identity)** - Main identity
2. **TokenAccount** - Token holding account
3. **DataAccount** - Data storage account
4. **TokenIssuer** - Token issuer/creator
5. **KeyBook** - Authority key storage
6. **KeyPage** - Individual key page (subordinate to KeyBook)

### Lite Accounts

7. **LiteIdentity** - Lite ADI (no full directory structure)
8. **LiteTokenAccount** - Lite token account (no authorities)
9. **LiteDataAccount** - Lite data account (minimal structure)

### System Accounts

10. **UnknownAccount** - Unknown account type
11. **UnknownSigner** - Unknown signer with version

---

## Signature Types

1. **LegacyED25519** - Legacy ED25519 signature
2. **ED25519** - Standard ED25519 signature
3. **RCD1** - Factom RCD1 signature
4. **BTC** - Bitcoin signature
5. **BTCLegacy** - Legacy Bitcoin signature
6. **ETH** - Ethereum signature
7. **Delegated** - Delegated signature (uses delegate key)
8. **Authority** - Authority signature

---

## REST API Endpoints

### Query Endpoints

#### Default Query
- `GET /query/{id}` - Query account or transaction

#### Chain Queries
- `GET /query/{id}/chain` - List all chains
- `GET /query/{id}/chain/{name}` - Query specific chain
- `GET /query/{id}/chain/{name}/index/{index}` - Query entry by index
- `GET /query/{id}/chain/{name}/entry/{value}` - Query entry by hash

#### Data Queries
- `GET /query/{id}/data` - Query data account (latest or range)
- `GET /query/{id}/data/index/{index}` - Query data entry by index
- `GET /query/{id}/data/entry/{hash}` - Query data entry by hash

#### Directory/Pending
- `GET /query/{id}/directory` - List directory entries
- `GET /query/{id}/pending` - List pending transactions

#### Block Queries
- `GET /block/minor` - List minor blocks
- `GET /block/major` - List major blocks
- `GET /block/minor/{index}` - Query specific minor block
- `GET /block/major/{index}` - Query specific major block

#### Search Endpoints
- `GET /search/{id}/anchor/{value}` - Search for anchor
- `GET /search/{id}/publicKey/{value}` - Search for public key
- `GET /search/{id}/delegate/{value}` - Search for delegate

### Service Endpoints

#### Node Service
- `GET /node/info` - Get node information
- `GET /node/services` - Find services

#### Consensus
- `GET /consensus/status` - Get consensus status

#### Network
- `GET /network/status` - Get network status

#### Metrics
- `GET /metrics` - Get network metrics

#### Submission
- `POST /submit` - Submit transaction envelope
- `POST /validate` - Validate transaction envelope
- `POST /faucet` - Request ACME from faucet

### JSON-RPC Endpoint
- `POST /v3` - JSON-RPC 2.0 endpoint
  - Methods: `query`, `node-info`, `find-service`, `consensus-status`, `network-status`, `metrics`, `submit`, `validate`, `faucet`, `private-sequence`

---

## Query Options

### RangeOptions
Used for paginating results
- `Start`: Starting index (default: 0)
- `Count`: Number of results (default: all)
- `Expand`: Resolve nested values (default: varies)
- `FromEnd`: Query from end (default: false)

### ReceiptOptions
- `ForAny`: Include receipt for any block
- `ForHeight`: Include receipt for specific height

### SubmitOptions
- `Verify`: Verify envelope format (default: true)
- `Wait`: Wait for acceptance/rejection (default: true)

### ValidateOptions
- `Full`: Full validation vs CheckTx (default: true)

### FaucetOptions
- `Token`: Specific token URL (optional)

### SubscribeOptions
- `Partition`: Specific partition (optional)
- `Account`: Specific account (optional)

---

## Service Types (ServiceType Enum)

1. **Unknown** (0) - Unknown service
2. **Node** (1) - Node service
3. **Consensus** (2) - Consensus service
4. **Network** (3) - Network service
5. **Metrics** (4) - Metrics service
6. **Query** (5) - Querier service
7. **Event** (6) - Event service
8. **Submit** (7) - Submitter service
9. **Validate** (8) - Validator service
10. **Faucet** (9) - Faucet service
11. **Snapshot** (10) - Snapshot service

---

## Transaction Status Codes

- `OK` - No error
- `Delivered` - Transaction delivered/accepted
- `Pending` - Transaction pending
- `Remote` - Remote partition
- `WrongPartition` - Wrong partition error
- `BadRequest` - Invalid request
- `Unauthenticated` - Missing/invalid signatures
- `InsufficientCredits` - Low credit balance
- `Unauthorized` - Not authorized
- `NotFound` - Not found
- `NotAllowed` - Not allowed
- `Rejected` - Transaction rejected
- `Expired` - Transaction expired
- `Conflict` - Conflicting state
- `BadSignerVersion` - Signer version mismatch
- `BadTimestamp` - Invalid timestamp
- `BadUrlLength` - Invalid URL length
- `IncompleteChain` - Incomplete chain
- `InsufficientBalance` - Low token balance
- `InternalError` - Internal error
- `UnknownError` - Unknown error
- `EncodingError` - Encoding error
- `FatalError` - Fatal error
- `NotReady` - Service not ready
- `WrongType` - Wrong type
- `NoPeer` - No peer available
- `PeerMisbehaved` - Peer misbehavior
- `InvalidRecord` - Invalid record

---

## Transport Mechanisms

### 1. JSON-RPC 2.0
- HTTP(S) POST endpoint `/v3`
- Request format: `{"jsonrpc":"2.0","method":"...","params":{},"id":1}`
- Used for simple request-response operations

### 2. REST
- HTTP(S) GET/POST endpoints
- Query parameters for options
- Direct URL mapping to account hierarchy

### 3. P2P (libp2p)
- Direct node-to-node communication
- Validators expose P2P interface
- API nodes proxy between clients and P2P network
- Service discovery via DHT

### 4. WebSocket
- Real-time event subscriptions
- Implementation incomplete, subject to change

---

## Data Entry Types

From `protocol/general.yml`, data entries support multiple formats:
- Text entries
- JSON entries
- Accumulize entries
- RawJson entries

---

## Implementation Notes

### API Design Principles
1. Each service has exactly one method (flexibility, independent implementation)
2. Services are independently implementable
3. Middleware support is straightforward
4. Transport is transparent to client

### Query Design
- Query is a union type (extensible)
- Returns varied record types based on input
- Supports pagination via RangeOptions
- Optional receipt proofs

### Transaction Execution
- Signed in envelopes
- Can verify or validate before submitting
- Returns submission status with transaction ID
- Supports pending transactions awaiting signatures

---

## Key Features

1. **Comprehensive Query Interface** - Multiple query types for flexible data retrieval
2. **Streaming/Range Queries** - Pagination support for large result sets
3. **Transaction Signing** - Full envelope-based transaction signing
4. **Multi-Authority** - Support for multi-sig and delegated signing
5. **Cross-Network** - Support for synthetic and remote transactions
6. **Event Subscriptions** - Real-time event streaming
7. **Flexible Transport** - JSON-RPC, REST, P2P, and WebSocket options
8. **Rate Limiting Ready** - P2P infrastructure for load balancing

---

## File Locations in SDK

- API Interfaces: `/pkg/api/v3/api.go`
- Query Helpers: `/pkg/api/v3/querier.go`
- Query Types: `/pkg/api/v3/queries.yml`
- Record Types: `/pkg/api/v3/records.yml`
- Response Types: `/pkg/api/v3/responses.yml`
- Events: `/pkg/api/v3/events.yml`
- Enums: `/pkg/api/v3/enums.yml`
- JSON-RPC: `/pkg/api/v3/jsonrpc/services.go`
- REST: `/pkg/api/v3/rest/`
- Transactions: `/protocol/user_transactions.yml`, `/protocol/synthetic_transactions.yml`
- Accounts: `/protocol/accounts.yml`
- OpenAPI: `/pkg/api/v3/openapi.yml`

