# Accumulate Network API Versions and Endpoints - Comprehensive Documentation

## Executive Summary

The Accumulate Network supports multiple API versions providing access to blockchain functionality through different protocols and transport mechanisms. The primary active API is **V3** (modern), with **V2** still available for backward compatibility. The APIs support JSON-RPC, REST, WebSocket, and P2P protocols.

---

## API VERSIONS

### V3 (Current/Modern API)

**Location:** `/pkg/api/v3/`

**Status:** Primary active API - fully implemented and recommended for new implementations

**Protocols Supported:**
- JSON-RPC 2.0 (HTTP POST)
- REST (HTTP GET/POST)
- WebSocket (WS/WSS)
- Binary Message Protocol (P2P)
- Ethereum RPC Compatibility

---

### V2 (Legacy/Compatibility API)

**Location:** `/internal/api/v2/`

**Status:** Maintained for backward compatibility

**Protocol:** JSON-RPC 2.0 via HTTP POST to `/v2` endpoint

**Migration Note:** V3 is recommended for new development; V2 serves primarily as a compatibility layer

---

## API V3: COMPREHENSIVE ENDPOINT REFERENCE

### 1. SERVICE TYPES (Enum)

```
ServiceType values:
- Unknown (0): Unknown service type
- Node (1): NodeService
- Consensus (2): ConsensusService
- Network (3): NetworkService
- Metrics (4): MetricsService
- Query (5): Querier
- Event (6): EventService
- Submit (7): Submitter
- Validate (8): Validator
- Faucet (9): Faucet
- Snapshot (10): SnapshotService
```

### 2. NODE SERVICE

**Interface:** `api.NodeService`

**Endpoints:**

#### 2.1 node-info
- **Description:** Returns information about the network node
- **JSON-RPC Method:** `node-info`
- **REST Endpoint:** `GET /node/info`
- **WebSocket:** Via binary message protocol
- **Request:** `NodeInfoRequest`
  - `NodeInfoOptions`:
    - `PeerID` (p2p.PeerID, optional): Peer ID to query
- **Response:** `NodeInfoResponse`
  - `PeerID` (p2p.PeerID): The peer ID
  - `Network` (string): Network name
  - `Services` (array of ServiceAddress): Available services
  - `Version` (string): Software version
  - `Commit` (string): Git commit hash

#### 2.2 find-service
- **Description:** Searches for nodes providing a specific service
- **JSON-RPC Method:** `find-service`
- **REST Endpoint:** `GET /node/services`
- **WebSocket:** Via binary message protocol
- **Request:** `FindServiceRequest`
  - `FindServiceOptions`:
    - `Network` (string): Network name
    - `Service` (ServiceAddress, pointer, optional): Service type/address to search for
    - `Known` (bool, optional): Restrict to known peers
    - `Timeout` (duration, optional): DHT query timeout
- **Response:** `FindServiceResponse`
  - Array of `FindServiceResult`:
    - `PeerID` (p2p.PeerID)
    - `Status` (KnownPeerStatus): Unknown(0), Good(1), Bad(2)
    - `Addresses` (array of p2p.Multiaddr)

### 3. CONSENSUS SERVICE

**Interface:** `api.ConsensusService`

**Endpoints:**

#### 3.1 consensus-status
- **Description:** Returns status of consensus node
- **JSON-RPC Method:** `consensus-status`
- **REST Endpoint:** `GET /consensus/status`
- **WebSocket:** Via binary message protocol
- **Request:** `ConsensusStatusRequest`
  - `ConsensusStatusOptions`:
    - `NodeID` (string): Node identifier
    - `Partition` (string): Partition name
    - `IncludePeers` (bool, pointer, optional): Include peer information
    - `IncludeAccumulate` (bool, pointer, optional): Include Accumulate-specific data
- **Response:** `ConsensusStatusResponse`
  - `Ok` (bool): Node is operational
  - `LastBlock` (LastBlock, pointer): Last block information
  - `Version` (string): Software version
  - `Commit` (string): Git commit
  - `NodeKeyHash` (hash): Hash of node key
  - `ValidatorKeyHash` (hash): Hash of validator key
  - `PartitionID` (string): Partition identifier
  - `PartitionType` (protocol.PartitionType): Partition type enum
  - `Peers` (array of ConsensusPeerInfo): Peer information

### 4. NETWORK SERVICE

**Interface:** `api.NetworkService`

**Endpoints:**

#### 4.1 network-status
- **Description:** Returns status of the network
- **JSON-RPC Method:** `network-status`
- **REST Endpoint:** `GET /network/status`
- **WebSocket:** Via binary message protocol
- **Request:** `NetworkStatusRequest`
  - `NetworkStatusOptions`:
    - `Partition` (string): Partition name
- **Response:** `NetworkStatusResponse`
  - `Oracle` (protocol.AcmeOracle, pointer): Oracle configuration
  - `Globals` (protocol.NetworkGlobals, pointer): Global network parameters
  - `Network` (protocol.NetworkDefinition, pointer): Network definition
  - `Routing` (protocol.RoutingTable, pointer): Routing table
  - `ExecutorVersion` (protocol.ExecutorVersion, enum, optional): Active executor version
  - `DirectoryHeight` (uint): Directory network block height
  - `MajorBlockHeight` (uint): Major block height
  - `BvnExecutorVersions` (array of protocol.PartitionExecutorVersion): Per-BVN executor versions

### 5. SNAPSHOT SERVICE

**Interface:** `api.SnapshotService`

**Endpoints:**

#### 5.1 list-snapshots
- **Description:** Lists available snapshots
- **JSON-RPC Method:** `list-snapshots`
- **REST Endpoint:** `GET /list-snapshots` (implied)
- **WebSocket:** Via binary message protocol
- **Request:** `ListSnapshotsRequest`
  - `ListSnapshotsOptions`:
    - `NodeID` (string): Node identifier
    - `Partition` (string): Partition name
- **Response:** Array of `SnapshotInfo`
  - `Header` (snapshot.Header, pointer): Snapshot header
  - `ConsensusInfo` (cometbft.GenesisDoc, pointer): Consensus genesis info

### 6. METRICS SERVICE

**Interface:** `api.MetricsService`

**Endpoints:**

#### 6.1 metrics
- **Description:** Returns network metrics (TPS, etc.)
- **JSON-RPC Method:** `metrics`
- **REST Endpoint:** `GET /metrics`
- **WebSocket:** Via binary message protocol
- **Request:** `MetricsRequest`
  - `MetricsOptions`:
    - `Partition` (string): Partition name
    - `Span` (uint, optional): Window width in blocks
- **Response:** `MetricsResponse`
  - `TPS` (float): Transactions per second

### 7. QUERY SERVICE (Querier)

**Interface:** `api.Querier`

**Endpoints:**

#### 7.1 query
- **Description:** Generic query of account/transaction state
- **JSON-RPC Method:** `query`
- **REST Endpoint:** `POST /query` (implied)
- **WebSocket:** Via binary message protocol
- **Request:** `QueryRequest`
  - `Scope` (url.URL, pointer): Target URL for query
  - `Query` (api.Query union, optional): Query object (see Query Types below)
- **Response:** `RecordResponse`
  - Returns union type of Record (see Record Types below)

**Query Types (QueryType enum):**

| Type | Value | Description | Fields |
|------|-------|-------------|--------|
| Default | 0x00 | Default account query | `IncludeReceipt` (ReceiptOptions, optional) |
| Chain | 0x01 | Query chain entries | `Name` (string), `Index` (uint, pointer), `Entry` (bytes), `Range` (RangeOptions), `IncludeReceipt` |
| Data | 0x02 | Query data chain | `Index` (uint, pointer), `Entry` (bytes), `Range` (RangeOptions) |
| Directory | 0x03 | Query directory | `Range` (RangeOptions) |
| Pending | 0x04 | Query pending transactions | `Range` (RangeOptions) |
| Block | 0x05 | Query blocks | `Minor` (uint), `Major` (uint), `MinorRange`, `MajorRange`, `EntryRange`, `OmitEmpty` |
| AnchorSearch | 0x10 | Search by anchor | `Anchor` (bytes), `IncludeReceipt` |
| PublicKeySearch | 0x11 | Search by public key | `PublicKey` (bytes), `Type` (protocol.SignatureType enum) |
| PublicKeyHashSearch | 0x12 | Search by key hash | `PublicKeyHash` (bytes) |
| DelegateSearch | 0x13 | Search by delegate | `Delegate` (url.URL, pointer) |
| MessageHashSearch | 0x14 | Search by message hash | `Hash` (hash) |

**Helper Methods (Querier2):**
- `QueryAccount()` - Query account by URL
- `QueryMessage()` - Query message by TxID
- `QueryTransaction()` - Query transaction message
- `QuerySignature()` - Query signature message
- `QueryChain()` - Query single chain
- `QueryAccountChains()` - Query all chains of account
- `QueryChainEntry()` - Query single chain entry
- `QueryChainEntries()` - Query range of chain entries
- `QueryMainChainEntry()` - Query transaction in main chain
- `QueryMainChainEntries()` - Query transaction range
- `QuerySignatureChainEntry()` - Query signature entry
- `QuerySignatureChainEntries()` - Query signature entry range
- `QueryIndexChainEntry()` - Query index entry
- `QueryIndexChainEntries()` - Query index entry range
- `QueryDataEntry()` - Query data entry
- `QueryDataEntries()` - Query data entry range
- `QueryDirectoryUrls()` - Query directory URLs
- `QueryDirectory()` - Query directory with expansion
- `QueryPendingIds()` - Query pending transaction IDs
- `QueryPending()` - Query pending transactions
- `QueryMinorBlock()` - Query single minor block
- `QueryMinorBlocks()` - Query minor block range
- `QueryMajorBlock()` - Query single major block
- `QueryMajorBlocks()` - Query major block range
- `SearchForAnchor()` - Search chain entries by anchor
- `SearchForPublicKey()` - Search by public key
- `SearchForPublicKeyHash()` - Search by key hash
- `SearchForDelegate()` - Search by delegate
- `SearchForMessage()` - Search transactions by message hash

**Record Types (RecordType enum):**

| Type | Value | Description |
|------|-------|-------------|
| Account | 0x01 | AccountRecord |
| Chain | 0x02 | ChainRecord |
| ChainEntry | 0x03 | ChainEntryRecord[T] |
| Key | 0x04 | KeyRecord |
| Message | 0x10 | MessageRecord[T] |
| SignatureSet | 0x11 | SignatureSetRecord |
| MinorBlock | 0x20 | MinorBlockRecord |
| MajorBlock | 0x21 | MajorBlockRecord |
| Range | 0x80 | RecordRange[T] |
| Url | 0x81 | UrlRecord |
| TxID | 0x82 | TxIDRecord |
| IndexEntry | 0x83 | IndexEntryRecord |
| Error | 0x8F | ErrorRecord |

### 8. SUBMIT SERVICE (Submitter)

**Interface:** `api.Submitter`

**Endpoints:**

#### 8.1 submit
- **Description:** Submits an envelope for execution
- **JSON-RPC Method:** `submit`
- **REST Endpoint:** `POST /submit`
- **WebSocket:** Via binary message protocol
- **Request:** `SubmitRequest`
  - `Envelope` (messaging.Envelope, pointer): Transaction envelope
  - `SubmitOptions`:
    - `Verify` (bool, pointer, optional): Verify before submit (default: yes)
    - `Wait` (bool, pointer, optional): Wait for acceptance (default: yes)
- **Response:** `SubmitResponse`
  - Array of `Submission`:
    - `Status` (protocol.TransactionStatus, pointer): Transaction status
    - `Success` (bool): Submission successful
    - `Message` (string): Status message

### 9. VALIDATE SERVICE (Validator)

**Interface:** `api.Validator`

**Endpoints:**

#### 9.1 validate
- **Description:** Validates an envelope without submission
- **JSON-RPC Method:** `validate`
- **REST Endpoint:** `POST /validate`
- **WebSocket:** Via binary message protocol
- **Request:** `ValidateRequest`
  - `Envelope` (messaging.Envelope, pointer): Transaction envelope
  - `ValidateOptions`:
    - `Full` (bool, pointer, optional): Full validation including signatures (default: yes)
- **Response:** `ValidateResponse`
  - Array of `Submission` (same as submit)

### 10. FAUCET SERVICE

**Interface:** `api.Faucet`

**Endpoints:**

#### 10.1 faucet
- **Description:** Requests tokens from the ACME faucet
- **JSON-RPC Method:** `faucet`
- **REST Endpoint:** `POST /faucet`
- **WebSocket:** Via binary message protocol
- **Request:** `FaucetRequest`
  - `Account` (url.URL, pointer): Target account URL
  - `FaucetOptions`:
    - `Token` (url.URL, pointer, optional): Token to mint (default: ACME)
- **Response:** `FaucetResponse`
  - Single `Submission`

### 11. EVENT SERVICE (Subscribe)

**Interface:** `api.EventService`

**Endpoints:**

#### 11.1 subscribe
- **Description:** Subscribes to event stream notifications
- **JSON-RPC Method:** N/A (streaming only)
- **REST Endpoint:** N/A (streaming only)
- **WebSocket:** `SubscribeRequest` -> stream of events
- **Binary Protocol:** Primary method for event subscription
- **Request:** `SubscribeRequest`
  - `SubscribeOptions`:
    - `Partition` (string, optional): Partition to subscribe to
    - `Account` (url.URL, pointer, optional): Specific account to monitor
- **Response:** Stream of `Event` messages (see Event Types below)

**Event Types (EventType enum):**

| Type | Value | Description | Fields |
|------|-------|-------------|--------|
| Error | 1 | Error event | `Err` (errors.Error, pointer) |
| Block | 2 | Block committed | `Partition` (string), `Index` (uint), `Time` (time), `Major` (uint), `Entries` (array of ChainEntryRecord[Record]) |
| Globals | 3 | Global values changed | `Old` (core.GlobalValues, pointer), `New` (core.GlobalValues, pointer) |

### 12. PRIVATE SERVICE (Sequencer)

**Interface:** `private.Sequencer`

**Service Type:** 0xF001 (internal only)

**Endpoints:**

#### 12.1 private-sequence
- **Description:** Internal sequencing operation
- **JSON-RPC Method:** `private-sequence`
- **Request:** `PrivateSequenceRequest`
  - `Source` (url.URL, pointer)
  - `Destination` (url.URL, pointer)
  - `SequenceNumber` (uint64)
  - `SequenceOptions`
- **Response:** `MessageRecord[messaging.Message]`

---

## ETHERNET RPC API (V3)

**Location:** `/pkg/api/ethereum/`

**Protocol:** JSON-RPC 2.0 (Ethereum-compatible)

**Supported Methods:**

### Ethereum Standard Methods

#### eth_chainId
- **Description:** Returns the chain ID
- **Parameters:** None
- **Returns:** `Number` (hex-encoded)

#### eth_blockNumber
- **Description:** Returns current block number
- **Parameters:** None
- **Returns:** `Number` (hex-encoded)

#### eth_gasPrice
- **Description:** Returns current gas price
- **Parameters:** None
- **Returns:** `Number` (hex-encoded)

#### eth_getBalance
- **Description:** Returns account balance
- **Parameters:** 
  - `Address` (Address): Account address
  - `block` (string): Block identifier
- **Returns:** `Number` (hex-encoded)

#### eth_getBlockByNumber
- **Description:** Returns block by number
- **Parameters:**
  - `block` (string): Block number
  - `expand` (bool): Include full transaction data
- **Returns:** `BlockData`

#### net_version
- **Description:** Returns network ID
- **Parameters:** None
- **Returns:** `uint64`

### Accumulate-Specific Methods

#### acc_typedData
- **Description:** Returns EIP-712 typed data for transaction
- **Parameters:**
  - `transaction` (protocol.Transaction)
  - `signature` (protocol.Signature)
- **Returns:** `encoding.EIP712Call`

---

## API V2 (LEGACY) - ENDPOINTS

**Endpoint:** `/v2` (JSON-RPC POST)

**Status-related Methods:**
- `status` - Node status
- `version` - Software version
- `describe` - Node configuration
- `metrics` - Network metrics

**Query Methods:**
- `query` - General query
- `query-directory` - Directory entries
- `query-tx` - Query transaction
- `query-tx-local` - Local transaction query
- `query-tx-history` - Transaction history
- `query-data` - Data chain entry
- `query-data-set` - Data chain range
- `query-key-index` - Key location (deprecated name for query-key-page-index)
- `query-minor-blocks` - Minor blocks (experimental)
- `query-major-blocks` - Major blocks (experimental)
- `query-synth` - Synthetic transaction (experimental)

**Transaction Execution Methods:**
- `execute` - Generic transaction submit
- `execute-direct` - Direct envelope submit
- `execute-local` - Local-only submit (internal)
- `faucet` - Request tokens

**Specialized Execute Methods:**
- `create-adi` / `create-identity` - Create identity
- `create-data-account` - Create data account
- `create-key-book` - Create key book
- `create-key-page` - Create key page
- `create-token` - Create token
- `create-token-account` - Create token account
- `send-tokens` - Transfer tokens
- `add-credits` - Add credits to account
- `update-key-page` - Modify key page
- `update-key` - Update key spec
- `write-data` - Write data entry
- `issue-tokens` - Mint tokens
- `write-data-to` - Write data to another account
- `burn-tokens` - Destroy tokens
- `update-account-auth` - Update account authorization

---

## TRANSPORT/PROTOCOL IMPLEMENTATIONS

### 1. JSON-RPC Handler
**Location:** `/pkg/api/v3/jsonrpc/`

- HTTP POST to defined endpoint
- Request/Response format per JSON-RPC 2.0 specification
- Error code offset: -33000

**Supported Services:**
- NodeService
- ConsensusService
- NetworkService
- SnapshotService
- MetricsService
- Querier
- Submitter
- Validator
- Faucet
- Sequencer (private)

### 2. REST Handler
**Location:** `/pkg/api/v3/rest/`

- HTTP GET/POST with query/body parameters
- Type-safe parameter parsing
- Standard HTTP status codes and JSON responses

**Registered Endpoints:**
- `GET /node/info` - Node information
- `GET /node/services` - Find services
- `GET /consensus/status` - Consensus status
- `GET /network/status` - Network status
- `GET /metrics` - Metrics
- `POST /submit` - Submit transaction
- `POST /validate` - Validate transaction
- `POST /faucet` - Faucet request

### 3. WebSocket Handler
**Location:** `/pkg/api/v3/websocket/`

- Bidirectional message streaming
- Multiplexed streams via message IDs
- Binary message protocol
- Connection fallback support

**Features:**
- Stream-based request/response
- Concurrent request handling
- Graceful connection management
- Panic recovery

### 4. Binary Message Protocol
**Location:** `/pkg/api/v3/message/`

- Efficient binary serialization
- Union type encoding
- Supports all V3 services
- Used by P2P and WebSocket transports

**Message Types:**
- Request messages (e.g., QueryRequest)
- Response messages (e.g., RecordResponse)
- Error responses (ErrorResponse)
- Event messages (for subscriptions)

### 5. P2P/Network Protocol
**Location:** `/pkg/api/v3/p2p/`

- libp2p-based peer-to-peer communication
- Service registration and discovery
- DHT integration for peer discovery
- Protocol ID format: `/acc/rpc/{service-address}/1.0.0`

**Key Components:**
- Node service registration
- Peer manager for lifecycle
- Service discovery with DHT
- Peer database and tracking

---

## SHARED DATA STRUCTURES

### Common Options

**RangeOptions:** (Pagination/Range Query)
- `Start` (uint, optional): Starting index
- `Count` (uint, pointer, optional): Number of results
- `Expand` (bool, pointer, optional): Request expanded results
- `FromEnd` (bool, optional): Count from end

**ReceiptOptions:** (Merkle Receipt Options)
- `ForAny` (bool): Include any receipt
- `ForHeight` (uint): Include receipt for specific height

### Response Structures

**AccountRecord:**
- `Account` (protocol.Account union)
- `Directory` (RecordRange[UrlRecord], pointer)
- `Pending` (RecordRange[TxIDRecord], pointer)
- `Receipt` (Receipt, pointer)
- `LastBlockTime` (time.Time, pointer)

**ChainRecord:**
- `Name` (string)
- `Type` (merkle.ChainType enum)
- `Count` (uint)
- `State` (bytes array)
- `IndexOf` (ChainRecord, pointer)
- `LastBlockTime` (time.Time, pointer)

**MessageRecord[T]:**
- `ID` (txid.TxID, pointer)
- `Message` (T union)
- `Status` (errors.Status enum)
- `Error` (errors.Error, pointer)
- `Result` (protocol.TransactionResult union)
- `Received` (uint)
- `Produced` (RecordRange[TxIDRecord], pointer)
- `Cause` (RecordRange[TxIDRecord], pointer)
- `Signatures` (RecordRange[SignatureSetRecord], pointer)
- `Historical` (bool)
- `Sequence` (messaging.SequencedMessage, pointer, optional)
- `SourceReceipt` (merkle.Receipt, pointer)
- `LastBlockTime` (time.Time, pointer)

**RecordRange[T]:**
- `Records` (array of T union)
- `Start` (uint)
- `Total` (uint)
- `LastBlockTime` (time.Time, pointer)

---

## ENUMERATIONS

### KnownPeerStatus
```
Unknown (0): PeerStatusIsUnknown
Good (1): PeerStatusIsKnownGood
Bad (2): PeerStatusIsKnownBad
```

### Error Handling

**Error Response Structure:**
- `Code` (int): Error code (base -33000 for protocol errors)
- `Message` (string): Human-readable error message
- `Data` (Error object):
  - `Code` (int): Error code
  - `Message` (string): Error message
  - Additional error context

---

## USAGE PATTERNS

### Query Flow
1. Client constructs Query object (DefaultQuery, ChainQuery, BlockQuery, etc.)
2. Client sends QueryRequest via chosen transport
3. API validates query and returns Record response
4. Client processes Record type (Account, Chain, Message, Range, etc.)

### Submit Flow
1. Client creates messaging.Envelope with transaction
2. Client sends SubmitRequest via chosen transport
3. API validates and routes transaction
4. Returns Submission status
5. Optional: Wait for acceptance into block

### Event Subscription Flow
1. Client sends SubscribeRequest with options
2. API establishes subscription stream
3. Events (Block, Globals, Errors) streamed to client
4. Client processes events as received

---

## CONFIGURATION & DISCOVERY

### Service Discovery
- **Mechanism:** DHT + Known Peer Database
- **API:** `FindService()` with FindServiceOptions
- **Returns:** Array of FindServiceResult with peer addresses

### Network Bootstrap
- **Entry Points:** Initial peer addresses configured
- **DHT Lookup:** /acc/{network}/partition/{partition-name}/{service-type}
- **Multiaddr Format:** /acc/network/{network}/partition/{name}/service/{type}

---

## API VERSIONING STRATEGY

- **V3:** Primary, fully-featured, modern design
- **V2:** Compatibility layer, deprecated for new work
- **Backward Compatibility:** V2 API maintained alongside V3
- **Migration Path:** Clients should migrate to V3

---

## IMPLEMENTATION REQUIREMENTS

### For Clients
1. Support chosen transport protocol (JSON-RPC, REST, WebSocket, P2P)
2. Implement error handling for common error codes
3. Handle optional fields and pointer types
4. Implement retry logic for network failures
5. Support request timeouts

### For Services
1. Implement required service interface(s)
2. Register with transport handler(s)
3. Validate input parameters
4. Handle context cancellation
5. Implement proper error responses
6. Support concurrent requests

---

## CONCLUSION

Accumulate Network provides a comprehensive, multi-protocol API suite suitable for diverse client implementations from lightweight mobile apps to high-performance backend systems. The V3 API represents the modern direction with support for multiple transport protocols, while V2 remains available for backward compatibility during migration periods.

