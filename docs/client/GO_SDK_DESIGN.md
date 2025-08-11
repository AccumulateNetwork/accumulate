# Accumulate Go Client Package Design

## Overview

This document describes the design for a Go package that provides a comprehensive client library for interacting with Accumulate networks. The package will offer a high-level, idiomatic Go API that maps to all Accumulate network endpoints across different API versions (v1, v2, v3, private, and internal APIs) and supports connecting to various network environments (mainnet, testnet, local devnet, etc.).

## Package Structure

```
gitlab.com/accumulatenetwork/accumulate/pkg/client/
├── client.go           # Main client implementation
├── config.go           # Configuration and network presets
├── query.go            # Query-related functions
├── submit.go           # Transaction submission functions
├── events.go           # Event subscription functions
├── networks.go         # Network-specific configurations
├── errors.go           # Custom error types
├── options.go          # Request option types
└── doc.go              # Package documentation
```

## API Endpoint Organization

### API Version History

Accumulate has evolved through multiple API versions, each adding new capabilities while maintaining backward compatibility where possible:

- **V2 API** (Legacy/Internal): Extended JSON-RPC API located in `internal/api/v2/` with 33+ query and execution methods
- **V3 API** (Current): Modern service-oriented API in `pkg/api/v3/` with multiple transport options (JSON-RPC, WebSocket, Message/P2P, REST)
- **Private API**: Internal endpoints in `internal/api/private/` for node operations and sequencing
- **Ethereum API**: Ethereum-compatible JSON-RPC in `pkg/api/ethereum/` for Web3/MetaMask compatibility
- **Light Client** (Experimental): Lightweight client in `exp/light/` with local caching and minimal network footprint

## API Endpoints by Version

### V2 API Endpoints (Legacy/Internal)

```go
// Status and Information
Status()                    // Node status
Version()                   // Software version
Describe()                  // Node configuration
Metrics(MetricsQuery)       // Network metrics

// Query Methods
Query(GeneralQuery)                     // General account/chain query
QueryDirectory(DirectoryQuery)          // Directory entries
QueryTx(TxnQuery)                      // Transaction by ID
QueryTxLocal(TxnQuery)                 // Local transaction query
QueryTxHistory(TxHistoryQuery)         // Transaction history
QueryData(DataEntryQuery)              // Data chain entry
QueryDataSet(DataEntrySetQuery)        // Data chain range
QueryKeyPageIndex(KeyPageIndexQuery)   // Key location
QueryMinorBlocks(MinorBlocksQuery)     // Minor blocks (experimental)
QueryMajorBlocks(MajorBlocksQuery)     // Major blocks (experimental)
QuerySynth(SyntheticTransactionRequest) // Synthetic transactions (experimental)

// Transaction Execution
Execute(TxRequest)                      // Submit transaction
ExecuteDirect(ExecuteRequest)          // Direct submission
ExecuteLocal(ExecuteRequest)           // Local execution (internal)

// Specialized Execute Methods (Helper Functions)
ExecuteCreateIdentity(CreateIdentity)
ExecuteCreateDataAccount(CreateDataAccount)
ExecuteCreateTokenAccount(CreateTokenAccount)
ExecuteCreateToken(CreateToken)
ExecuteSendTokens(SendTokens)
ExecuteCreateKeyPage(CreateKeyPage)
ExecuteCreateKeyBook(CreateKeyBook)
ExecuteUpdateKey(UpdateKey)
ExecuteUpdateKeyPage(UpdateKeyPage)
ExecuteAddCredits(AddCredits)
ExecuteUpdateCredits(UpdateCredits)
ExecuteUpdateAccountAuth(UpdateAccountAuth)
ExecuteWriteData(WriteData)
ExecuteWriteDataTo(WriteDataTo)
ExecuteAcmeFaucet(AcmeFaucet)
ExecuteCreateLiteTokenAccount(CreateLiteTokenAccount)

// Faucet
Faucet(AcmeFaucet)                      // Request testnet tokens
```

### V3 API Endpoints (Current - `pkg/api/v3/`)

```go
// Service Interfaces (api.go)
NodeService:
    NodeInfo(NodeInfoOptions) → NodeInfo
    FindService(FindServiceOptions) → [FindServiceResult]

ConsensusService:
    ConsensusStatus(ConsensusStatusOptions) → ConsensusStatus

NetworkService:
    NetworkStatus(NetworkStatusOptions) → NetworkStatus

SnapshotService:
    ListSnapshots(ListSnapshotsOptions) → [SnapshotInfo]

MetricsService:
    Metrics(MetricsOptions) → Metrics

Querier:
    Query(scope *url.URL, query Query) → Record

EventService:
    Subscribe(SubscribeOptions) → <-chan Event

Submitter:
    Submit(envelope *messaging.Envelope, SubmitOptions) → [Submission]

Validator:
    Validate(envelope *messaging.Envelope, ValidateOptions) → [Submission]

Faucet:
    Faucet(account *url.URL, FaucetOptions) → Submission

// Query Types (queries.yml - implements Query interface):
- DefaultQuery                          // Default account query with receipt options
- ChainQuery                           // Chain entries with name/index/entry/range support
- DataQuery                            // Data entries with index/entry/range support
- DirectoryQuery                       // Directory listing with range options
- PendingQuery                         // Pending transactions with range
- BlockQuery                           // Block info (minor/major blocks with ranges)
- AnchorSearchQuery                    // Search for anchors
- PublicKeySearchQuery                 // Search by public key
- PublicKeyHashSearchQuery             // Search by public key hash
- DelegateSearchQuery                  // Search for delegates
- MessageHashSearchQuery               // Find message by hash
- TransactionHashSearchQuery           // Find transaction by hash
- IndexedAccountQuery                  // Get account by index

// Transport Implementations:
- jsonrpc/   → JSON-RPC 2.0 over HTTP (15s default timeout)
- message/   → Binary message protocol over libp2p
- websocket/ → WebSocket with JSON encoding (incomplete)
- rest/      → RESTful HTTP interface
- p2p/       → Peer-to-peer networking layer
```

### Private API Endpoints (Internal - `internal/api/private/`)

```go
// Sequencer Service (api.go)
type Sequencer interface {
    Sequence(ctx context.Context, src, dst *url.URL, num uint64, opts SequenceOptions) (*api.MessageRecord[messaging.Message], error)
}

const ServiceTypeSequencer api.ServiceType = 0xF001
```

### Ethereum-Compatible API (`pkg/api/ethereum/`)

```go
// Service Interface (services.go)
type Service interface {
    // Standard Ethereum JSON-RPC Methods
    EthChainId() (*big.Int, error)
    EthBlockNumber() (uint64, error)
    EthGetBalance(address common.Address, block *big.Int) (*big.Int, error)
    EthGetCode(address common.Address, block *big.Int) ([]byte, error)
    EthGetTransactionCount(address common.Address, block *big.Int) (uint64, error)
    EthGetBlockByNumber(number *big.Int, full bool) (*types.Block, error)
    EthGetBlockByHash(hash common.Hash, full bool) (*types.Block, error)
    EthGetTransactionByHash(hash common.Hash) (*types.Transaction, error)
    EthGetTransactionReceipt(hash common.Hash) (*types.Receipt, error)
    EthSendRawTransaction(data []byte) (common.Hash, error)
    EthCall(msg ethereum.CallMsg, block *big.Int) ([]byte, error)
    EthEstimateGas(msg ethereum.CallMsg) (uint64, error)
    EthGasPrice() (*big.Int, error)
    EthGetLogs(filter ethereum.FilterQuery) ([]types.Log, error)
    
    // Web3 Methods
    Web3ClientVersion() (string, error)
    Web3Sha3(data []byte) ([]byte, error)
    
    // Net Methods
    NetVersion() (string, error)
    NetListening() (bool, error)
    NetPeerCount() (uint64, error)
}

// Implementation: Minimum viable Ethereum RPC for MetaMask compatibility
```

### Light Client API (Experimental - `exp/light/`)

```go
// Light Client Features
type LightClient struct {
    Store    storage.Store  // BadgerDB or memory storage
    Querier  api.Querier    // V3 API querier
    // Features:
    // - Local caching and indexing
    // - Minimal network footprint
    // - Asynchronous syncing
    // - Supports basic queries
}
```

## Core Components

### 1. Client Interface

```go
package client

import (
    "context"
    "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
    "gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
    "gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// Client provides high-level access to Accumulate network APIs
type Client struct {
    // Internal transport (JSON-RPC, WebSocket, or Message-based)
    transport Transport
    
    // Network configuration
    config    *Config
    
    // Embedded service interfaces
    api.NodeService
    api.ConsensusService
    api.NetworkService
    api.SnapshotService
    api.MetricsService
    api.Querier
    api.Submitter
    api.Validator
    api.Faucet
    api.EventService
}

// Transport abstraction for different connection types
type Transport interface {
    api.NodeService
    api.ConsensusService
    api.NetworkService
    api.SnapshotService
    api.MetricsService
    api.Querier
    api.Submitter
    api.Validator
    api.Faucet
    api.EventService
    Close() error
}
```

### 2. Configuration System

```go
// Config holds client configuration
type Config struct {
    // Network endpoint(s)
    Endpoints []string
    
    // Network type for validation
    Network NetworkType
    
    // Transport type
    Transport TransportType
    
    // Timeout settings
    Timeout time.Duration
    
    // Retry configuration
    RetryPolicy *RetryPolicy
    
    // TLS configuration (optional)
    TLS *tls.Config
    
    // Debug mode
    Debug bool
    
    // Connection limits (from current implementation)
    MaxConnections int     // Default: 500
    ReadTimeout    time.Duration // Default: 10s
    MaxWaitTime    time.Duration // Default: 10s
}

// NetworkType identifies the network
type NetworkType string

const (
    NetworkMainnet NetworkType = "mainnet"
    NetworkKermit  NetworkType = "kermit"  // Testnet
    NetworkLocal   NetworkType = "local"   // Local development
    NetworkDevnet  NetworkType = "devnet"  // Development network
    NetworkCustom  NetworkType = "custom"  // Custom configuration
)

// TransportType specifies the transport protocol
type TransportType string

const (
    TransportJSONRPC   TransportType = "jsonrpc"   // HTTP JSON-RPC 2.0
    TransportWebSocket TransportType = "websocket" // WebSocket (incomplete)
    TransportMessage   TransportType = "message"   // Binary/P2P libp2p
    TransportREST      TransportType = "rest"      // RESTful HTTP
    TransportAuto      TransportType = "auto"      // Auto-detect
```

### 3. Constructor Functions

```go
// New creates a new client with the given configuration
func New(config *Config) (*Client, error)

// NewMainnet creates a client connected to mainnet
func NewMainnet(opts ...Option) (*Client, error)

// NewKermit creates a client connected to Kermit testnet
func NewKermit(opts ...Option) (*Client, error)

// NewLocal creates a client connected to local network
func NewLocal(endpoint string, opts ...Option) (*Client, error)

// NewDevnet creates a client connected to a devnet
func NewDevnet(endpoint string, opts ...Option) (*Client, error)

// Option is a functional option for client configuration
type Option func(*Config)

// WithTimeout sets the request timeout
func WithTimeout(d time.Duration) Option

// WithTransport sets the transport type
func WithTransport(t TransportType) Option

// WithDebug enables debug mode
func WithDebug() Option

// WithRetry configures retry policy
func WithRetry(policy *RetryPolicy) Option
```

### 4. High-Level API Functions

The client will provide high-level wrappers for all API versions, with intelligent routing to the appropriate endpoint based on availability and network support:

```go
// === V3 API Methods (Primary) ===

// Query Methods - V3 Style
func (c *Client) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error)
func (c *Client) GetAccount(ctx context.Context, url *url.URL) (*api.AccountRecord, error)
func (c *Client) GetTransaction(ctx context.Context, txid []byte) (*api.MessageRecord[messaging.Transaction], error)
func (c *Client) GetChainEntry(ctx context.Context, account *url.URL, chain string, index uint64) (*api.ChainEntryRecord, error)
func (c *Client) GetDataEntry(ctx context.Context, account *url.URL, index uint64) (*api.DataEntryRecord, error)
func (c *Client) GetDirectory(ctx context.Context, account *url.URL, start, count uint64) (*api.DirectoryRecord, error)
func (c *Client) GetPending(ctx context.Context, account *url.URL, start, count uint64) (*api.PendingRecord, error)
func (c *Client) GetBlock(ctx context.Context, partition string, blockNumber uint64) (*api.BlockRecord, error)
func (c *Client) SearchForAnchor(ctx context.Context, scope *url.URL, search api.AnchorSearchQuery) (*api.ChainEntryRecord, error)
func (c *Client) SearchForPublicKey(ctx context.Context, publicKey []byte, opts api.PublicKeySearchQuery) (*api.PublicKeySearchRecord, error)
func (c *Client) SearchForDelegate(ctx context.Context, scope *url.URL, query api.DelegateSearchQuery) (*api.DelegateSearchRecord, error)
func (c *Client) GetMessageByID(ctx context.Context, id []byte) (*api.MessageRecord[messaging.Message], error)
func (c *Client) GetIndexedAccount(ctx context.Context, index uint64) (*api.IndexedAccountRecord, error)

// Transaction Submission - V3 Style
func (c *Client) Submit(ctx context.Context, envelope *messaging.Envelope, opts ...SubmitOption) (*api.Submission, error)
func (c *Client) SubmitTransaction(ctx context.Context, tx messaging.Transaction, signer Signer, opts ...SubmitOption) (*api.Submission, error)
func (c *Client) Validate(ctx context.Context, envelope *messaging.Envelope) (*api.Submission, error)

// Event Subscription - V3 Only
func (c *Client) Subscribe(ctx context.Context, opts api.SubscribeOptions) (<-chan api.Event, error)
func (c *Client) SubscribeToAccount(ctx context.Context, account *url.URL) (<-chan api.Event, error)
func (c *Client) SubscribeToChain(ctx context.Context, account *url.URL, chain string) (<-chan api.Event, error)

// Network Information - V3 Style
func (c *Client) GetNodeInfo(ctx context.Context) (*api.NodeInfo, error)
func (c *Client) GetNetworkStatus(ctx context.Context) (*api.NetworkStatus, error)
func (c *Client) GetConsensusStatus(ctx context.Context) (*api.ConsensusStatus, error)
func (c *Client) GetMetrics(ctx context.Context, partition string, duration time.Duration) (*api.Metrics, error)
func (c *Client) ListSnapshots(ctx context.Context) ([]*api.SnapshotInfo, error)
func (c *Client) FindService(ctx context.Context, service api.ServiceType) ([]*api.FindServiceResult, error)

// Faucet - Available in V2 and V3
func (c *Client) Faucet(ctx context.Context, account *url.URL) (*api.Submission, error)

// === V2 API Methods (Legacy Support) ===

// V2-Specific Query Methods
func (c *Client) QueryV2(ctx context.Context, query v2.GeneralQuery) (*v2.ChainQueryResponse, error)
func (c *Client) QueryTxHistory(ctx context.Context, account *url.URL, start, count int) (*v2.MultiResponse, error)
func (c *Client) QueryKeyPageIndex(ctx context.Context, account *url.URL, key []byte) (*v2.ChainQueryResponse, error)
func (c *Client) QueryDataSet(ctx context.Context, account *url.URL, start, count int) (*v2.MultiResponse, error)
func (c *Client) QueryMinorBlocks(ctx context.Context, account *url.URL, start, count int, mode v2.TxFetchMode) (*v2.MultiResponse, error)
func (c *Client) QueryMajorBlocks(ctx context.Context, account *url.URL, start, count int) (*v2.MultiResponse, error)

// V2-Style Transaction Helpers
func (c *Client) ExecuteCreateIdentity(ctx context.Context, params *protocol.CreateIdentity, signer Signer) (*v2.TxResponse, error)
func (c *Client) ExecuteCreateDataAccount(ctx context.Context, params *protocol.CreateDataAccount, signer Signer) (*v2.TxResponse, error)
func (c *Client) ExecuteCreateTokenAccount(ctx context.Context, params *protocol.CreateTokenAccount, signer Signer) (*v2.TxResponse, error)
func (c *Client) ExecuteCreateToken(ctx context.Context, params *protocol.CreateToken, signer Signer) (*v2.TxResponse, error)
func (c *Client) ExecuteSendTokens(ctx context.Context, params *protocol.SendTokens, signer Signer) (*v2.TxResponse, error)
func (c *Client) ExecuteCreateKeyPage(ctx context.Context, params *protocol.CreateKeyPage, signer Signer) (*v2.TxResponse, error)
func (c *Client) ExecuteCreateKeyBook(ctx context.Context, params *protocol.CreateKeyBook, signer Signer) (*v2.TxResponse, error)
func (c *Client) ExecuteUpdateKey(ctx context.Context, params *protocol.UpdateKey, signer Signer) (*v2.TxResponse, error)
func (c *Client) ExecuteAddCredits(ctx context.Context, params *protocol.AddCredits, signer Signer) (*v2.TxResponse, error)
func (c *Client) ExecuteWriteData(ctx context.Context, params *protocol.WriteData, signer Signer) (*v2.TxResponse, error)

// V2-Only Methods
func (c *Client) GetStatus(ctx context.Context) (*v2.StatusResponse, error)
func (c *Client) GetVersion(ctx context.Context) (*v2.ChainQueryResponse, error)
func (c *Client) Describe(ctx context.Context) (*v2.DescriptionResponse, error)

// === Private/Internal API Methods ===

// Sequencer Service (Internal Use)
func (c *Client) Sequence(ctx context.Context, src, dst *url.URL, sequenceNumber uint64, opts private.SequenceOptions) (*api.MessageRecord[messaging.Message], error)

// === Ethereum-Compatible Methods (Web3) ===

// Ethereum JSON-RPC Methods
func (c *Client) EthChainID(ctx context.Context) (*big.Int, error)
func (c *Client) EthBlockNumber(ctx context.Context) (uint64, error)
func (c *Client) EthGetBalance(ctx context.Context, address common.Address, block *big.Int) (*big.Int, error)
func (c *Client) EthGetTransactionByHash(ctx context.Context, hash common.Hash) (*types.Transaction, error)
func (c *Client) EthGetTransactionReceipt(ctx context.Context, hash common.Hash) (*types.Receipt, error)
func (c *Client) EthSendRawTransaction(ctx context.Context, tx []byte) (common.Hash, error)
func (c *Client) EthCall(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error)
func (c *Client) EthEstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error)
func (c *Client) EthGetLogs(ctx context.Context, filter ethereum.FilterQuery) ([]types.Log, error)
```

### 5. Helper Types and Utilities

```go
// Signer interface for transaction signing
type Signer interface {
    Sign(hash []byte) ([]byte, error)
    PublicKey() []byte
}

// SubmitOption configures transaction submission
type SubmitOption func(*api.SubmitOptions)

// WithWait waits for transaction completion
func WithWait() SubmitOption

// WithCheckOnly validates without submitting
func WithCheckOnly() SubmitOption

// RetryPolicy defines retry behavior
type RetryPolicy struct {
    MaxAttempts int
    InitialDelay time.Duration
    MaxDelay time.Duration
    Multiplier float64
}

// Error types
type NetworkError struct {
    Network NetworkType
    Err     error
}

type ValidationError struct {
    Field string
    Err   error
}
```

### 6. Network Presets

```go
// networks.go

var Networks = map[NetworkType]*NetworkConfig{
    NetworkMainnet: {
        Endpoints: []string{
            "https://mainnet.accumulate.defidevs.io/v3",
            "https://mainnet-us-east.accumulate.defidevs.io/v3",
            "https://mainnet-us-west.accumulate.defidevs.io/v3",
            "https://mainnet-eu.accumulate.defidevs.io/v3",
        },
        ChainID: "accumulate-mainnet",
        Features: []string{"production", "anchoring"},
    },
    NetworkKermit: {
        Endpoints: []string{
            "https://kermit.accumulate.defidevs.io/v3",
            "https://testnet.accumulate.defidevs.io/v3",
        },
        ChainID: "accumulate-kermit",
        Features: []string{"faucet", "testnet"},
    },
    NetworkLocal: {
        Endpoints: []string{
            "http://localhost:8080/v3",
            "http://127.0.0.1:8080/v3",
        },
        ChainID: "accumulate-local",
        Features: []string{"development", "faucet"},
    },
}

type NetworkConfig struct {
    Endpoints []string
    ChainID   string
    Features  []string // e.g., ["faucet", "testnet", "anchoring"]
    
    // Default ports (from current config)
    HTTPPort  int      // Default: 8080
    P2PPort   int      // Default: varies by network
}
```

## Usage Examples

### Basic Usage

```go
package main

import (
    "context"
    "log"
    
    "gitlab.com/accumulatenetwork/accumulate/pkg/client"
    "gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

func main() {
    // Connect to mainnet
    c, err := client.NewMainnet()
    if err != nil {
        log.Fatal(err)
    }
    defer c.Close()
    
    // Query an account
    accountURL, _ := url.Parse("acc://mytoken.acme")
    account, err := c.GetAccount(context.Background(), accountURL)
    if err != nil {
        log.Fatal(err)
    }
    
    log.Printf("Account: %+v", account)
}
```

### Transaction Submission

```go
// Submit a transaction
tx := &protocol.SendTokens{
    To: []*protocol.TokenRecipient{
        {
            URL:    recipientURL,
            Amount: 1000000, // 1 ACME
        },
    },
}

submission, err := c.SubmitTransaction(
    context.Background(),
    tx,
    signer,
    client.WithWait(), // Wait for completion
)
if err != nil {
    log.Fatal(err)
}

log.Printf("Transaction ID: %x", submission.Status.TxID)
```

### Event Subscription

```go
// Subscribe to account events
events, err := c.SubscribeToAccount(context.Background(), accountURL)
if err != nil {
    log.Fatal(err)
}

for event := range events {
    switch e := event.(type) {
    case *api.GlobalsEvent:
        log.Printf("Globals updated: %+v", e)
    case *api.BlockEvent:
        log.Printf("New block: %d", e.Index)
    }
}
```

### Custom Network

```go
// Connect to a custom network
config := &client.Config{
    Endpoints: []string{"http://localhost:8080/v3"},
    Network:   client.NetworkLocal,
    Transport: client.TransportJSONRPC,
    Timeout:   30 * time.Second,
}

c, err := client.New(config)
if err != nil {
    log.Fatal(err)
}
```

## API Version Support Matrix

| Feature | V2 | V3 | Private | Ethereum | Light Client |
|---------|----|----|---------|----------|--------------|
| Basic Queries | ✓ | ✓ | - | - | ✓ |
| Transaction Submit | ✓ | ✓ | - | ✓ | ✓ |
| Event Subscription | - | ✓ | - | ✓ | - |
| Network Status | ✓ | ✓ | - | ✓ | - |
| Consensus Status | - | ✓ | - | - | - |
| Service Discovery | - | ✓ | - | - | - |
| Snapshots | - | ✓ | - | - | ✓ |
| Transaction History | ✓ | ✓ | - | ✓ | ✓ |
| Data Chains | ✓ | ✓ | - | - | ✓ |
| Key Management | ✓ | ✓ | - | - | - |
| Faucet | ✓ | ✓ | - | - | - |
| Sequencing | - | - | ✓ | - | - |
| Web3 Compatibility | - | - | - | ✓ | - |
| Local Caching | - | - | - | - | ✓ |
| P2P Transport | - | ✓ | - | - | - |
| REST API | - | ✓ | - | - | - |

## Implementation Phases

### Phase 1: Core Infrastructure
- Basic client structure wrapping existing implementations
- Configuration system with network presets
- Transport abstraction layer (supporting JSON-RPC, Message, REST, WebSocket)
- Error handling and type definitions
- API version detection and routing

### Phase 2: V3 API Implementation (Primary)
- Wrap existing V3 clients (jsonrpc.Client, message.Client)
- Implement all V3 service interfaces
- Support all Query types from queries.yml
- Event subscription via WebSocket/streaming
- Multi-transport support with automatic selection

### Phase 3: V2 API Support (Legacy)
- Wrap existing V2 implementation from internal/api/v2
- Support all 33+ V2 methods
- Transaction helper methods for all transaction types
- Backward compatibility for existing V2 users

### Phase 4: Specialized APIs
- Private API integration (Sequencer service)
- Ethereum API wrapper (pkg/api/ethereum)
- Light client integration (exp/light)
- Web3 provider interface implementation
- Cross-version fallback logic

### Phase 5: Advanced Features
- Automatic API version negotiation
- Batch operations using V3 batch support
- Connection pooling (leverage existing 500 conn limit)
- Retry mechanisms with exponential backoff
- Circuit breaker pattern for resilience
- Load balancing across multiple endpoints

### Phase 6: Testing and Documentation
- Unit tests for each API version
- Integration tests against local devnet
- Version compatibility tests (V2 vs V3)
- Migration guides from raw API usage
- Comprehensive API documentation
- Example applications

## Benefits

1. **Unified Interface**: Single client for all Accumulate API operations across all versions
2. **Version Compatibility**: Support for V1, V2, V3, Private, and Ethereum APIs
3. **Network Abstraction**: Easy switching between mainnet, testnet, and local networks
4. **Type Safety**: Leverages Go's type system for compile-time safety
5. **Idiomatic Go**: Follows Go best practices and conventions
6. **Automatic Fallback**: Intelligent routing between API versions based on availability
7. **Web3 Compatible**: Ethereum-compatible interface for Web3 tools
8. **Extensible**: Easy to add new networks and features
9. **Well-Tested**: Comprehensive test coverage across all API versions
10. **Production Ready**: Built-in retry, timeout, and error handling

## Compatibility

- Requires Go 1.21 or later (based on current go.mod)
- Compatible with Accumulate API v2, v3
- Supports private/internal APIs
- Ethereum JSON-RPC compatible via pkg/api/ethereum
- Supports all standard Accumulate networks (mainnet, kermit/testnet, local, devnet)
- Backward compatible with existing api packages
- Works with existing transport implementations (JSON-RPC, Message, P2P, REST)

## Migration Path

### From V2 API
For users currently using the V2 API:
1. Replace v2 client initialization with new unified client
2. V2 methods are available with `Client.QueryV2()`, `Client.ExecuteCreateIdentity()`, etc.
3. Gradually migrate to V3 methods for better performance
4. Both V2 and V3 methods can be used simultaneously

### From V3 API
For users currently using the low-level api/v3 packages:
1. The new client package wraps the existing V3 API packages
2. Direct V3 methods are the default (`Client.Query()`, `Client.Submit()`)
3. Existing code can use the high-level wrappers immediately
4. Direct access to underlying transport is available for advanced use cases

### From Ethereum/Web3
For users coming from Ethereum:
1. Use the Ethereum-compatible methods (`Client.EthGetBalance()`, etc.)
2. Compatible with existing Web3 tools and libraries
3. Can mix Accumulate-native and Ethereum-style calls

## API Deprecation Strategy

- **V2**: Legacy support via internal/api/v2, all methods available but not recommended for new code
- **V3**: Primary API in pkg/api/v3, recommended for all new development
- **Private**: Internal use only via internal/api/private, subject to change without notice
- **Ethereum**: Stable in pkg/api/ethereum, maintained for Web3 ecosystem compatibility
- **Light Client**: Experimental in exp/light, API may change

## Key Implementation Notes

Based on the current codebase analysis:

1. **Existing Clients**: The package should wrap, not replace, existing client implementations
2. **Transport Layer**: V3 already has jsonrpc, message, websocket, and rest transports
3. **Service Architecture**: V3 uses a service-oriented design with clear interfaces
4. **Code Generation**: Many types are generated from YAML files (queries.yml, types.yml, etc.)
5. **Network Routing**: Complex routing logic exists for cross-partition communication
6. **Recent Features**: CrossChain Conductor, improved snapshot syncing, light client design