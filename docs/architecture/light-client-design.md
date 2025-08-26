# Accumulate Light Client Design

## Overview

The Accumulate Light Client is a modular system designed to query and retrieve various account types from the Accumulate network with cryptographic proofs. The design emphasizes separation of concerns, extensibility, and ease of use for application developers.

## Architecture

### Actual Code Structure

```
pkg/lightclient/           # Reusable light client package
├── client.go             # Core JSON-RPC 2.0 client
├── accounts.go           # Account-specific types and retrieval methods
├── operations.go         # High-level convenience operations
└── README.md            # Package documentation

tools/light-client/       # Core light client executable
├── main.go              # Operators keybook collection
├── monitor.go           # Monitoring functionality
├── root-hash-monitor.go # Root hash monitoring
└── design.md            # This design document

exp/light/               # Experimental light client code
├── client.go            # Experimental client implementation
├── client_test.go       # Client tests
├── index.go             # Indexing functionality
├── index_test.go        # Index tests
├── model.go             # Data models
├── model_gen.go         # Generated model code
├── pkg.go               # Package definitions
├── range.go             # Range operations
├── range_test.go        # Range tests
├── sync.go              # Synchronization logic
├── types.go             # Type definitions
└── types_gen.go         # Generated type code

docs/tools/debug/        # Test and debug documentation
├── lite_client_test.go  # Test with known issues
└── lite-client-test.md  # Test documentation
```

### Design Principles

1. **Separation of Concerns**: Core light client focuses solely on operators keybook collection
2. **Network-Only Architecture**: Direct network queries with no caching or persistent storage
3. **Stateless Design**: No persistent state between runs - always fresh data from network
4. **Cryptographic Verification**: Built-in proof verification before serving data to applications
5. **Ordered Data Structures**: No maps for blockchain data - everything uses ordered slices/arrays for deterministic proof construction
6. **Modularity**: Each account type in separate files for better maintainability and AI handling
7. **Extensibility**: Architecture supports future account types and proof verification
8. **Usability**: Simple API with server shortcuts and comprehensive error handling

### Critical Design Constraint: No Maps for Blockchain Data

**Important**: All structures tracking Accumulate accounts and blockchain data must use ordered data structures (slices, arrays) instead of maps. This is essential because:

- Accumulate is a blockchain where everything has inherent order
- Cryptographic proofs require deterministic, ordered data structures
- Maps are unordered and would break proof verification
- Merkle trees and other cryptographic constructs depend on consistent ordering

Maps may only be used for non-blockchain metadata like configuration, caching indexes, etc.

## API Design

### Core Client Interface

```go
type Client struct {
    serverURL  string
    httpClient *http.Client
}

// Core query method
func (c *Client) Query(ctx context.Context, accountURL string) (*QueryResponse, error)

// Account-specific retrieval methods
func (c *Client) GetADI(ctx context.Context, adiURL string) (*ADI, error)
func (c *Client) GetTokenAccount(ctx context.Context, tokenURL string) (*TokenAccount, error)
func (c *Client) GetKeyBook(ctx context.Context, keyBookURL string) (*KeyBook, error)
func (c *Client) GetKeyPage(ctx context.Context, keyPageURL string) (*KeyPage, error)
func (c *Client) GetDataAccount(ctx context.Context, dataURL string) (*DataAccount, error)
```

### High-Level Operations

```go
// Operators keybook collection (core light client functionality)
func (c *Client) GetOperatorsInfo(ctx context.Context) (*KeyBook, []*KeyPage, []string, error)

// Additional utility methods for operators keybook
func (c *Client) ValidateOperatorsKeybook(keybook *KeyBookState) error
func (c *Client) GetKeyPagesByKeybook(keybookURL string) ([]*KeyPageState, error)

// Generic account information
func (c *Client) GetAccountInfo(ctx context.Context, accountURL string) (*AccountInfo, error)
func (c *Client) BatchGetAccountInfo(ctx context.Context, accountURLs []string) ([]*AccountInfo, error)
```

### Server URL Shortcuts

The client supports convenient server shortcuts:

- `local`: `http://127.0.1.1:26660`
- `testnet`: `https://testnet.accumulatenetwork.io`
- `beta`: `https://beta.accumulatenetwork.io`
- `canary`: `https://canary.accumulatenetwork.io`
- `mainnet`: `https://mainnet.accumulatenetwork.io`
- `mainnet-ssl`: `https://mainnet-ssl.accumulatenetwork.io`

## JSON-RPC Protocol

### Request Format

The client uses JSON-RPC 2.0 over HTTP POST:

```json
{
  "jsonrpc": "2.0",
  "method": "query",
  "params": {
    "scope": "acc://dn.acme/operators"
  },
  "id": 1
}
```

### Response Format (v3 API)

```json
{
  "id": 1,
  "jsonrpc": "2.0",
  "result": {
    "account": {
      "type": "keyBook",
      "url": "acc://dn.acme/operators",
      "authorities": [...],
      "pageCount": 1
    },
    "directory": {
      "recordType": "range",
      "records": [
        {
          "recordType": "url",
          "value": "acc://dn.acme/operators/1"
        }
      ],
      "start": 0,
      "total": 1
    },
    "lastBlockTime": "2025-07-18T21:40:03Z",
    "recordType": "account"
  }
}
```

### Backward Compatibility

The client supports both v3 API format (with `account` field) and older formats (with `data` field) for maximum compatibility.

## Accumulate Chain State

### Core Chain State Structure

Every Accumulate account has an underlying chain state with merkle state information:

```go
// AccumulateChainState represents the fundamental chain state for any Accumulate account
type AccumulateChainState struct {
    URL          string    // Account URL
    Type         string    // Account type
    BlockHeight  int64     // Current block height
    BlockIndex   int64     // Index within the block
    TxnHash      []byte    // Transaction hash that last modified this account
    MerkleState  *MerkleState // Merkle state information
    ChainHead    []byte    // Current chain head hash
    Timestamp    time.Time // Block timestamp
    Sequence     int64     // Account sequence number
}

// MerkleState contains the merkle tree information for the account
type MerkleState struct {
    Root        []byte      // Merkle root hash
    Count       int64       // Number of entries in the merkle tree
    Height      int         // Height of the merkle tree
    Entries     []MerkleEntry // Ordered merkle entries
}

// MerkleEntry represents a single entry in the account's merkle tree
type MerkleEntry struct {
    Index       int64     // Entry index (for ordering)
    Hash        []byte    // Entry hash
    BlockHeight int64     // Block height when entry was added
    TxnHash     []byte    // Transaction hash that created this entry
    Timestamp   time.Time // Calculated timestamp from block height
}

// ChainIndex provides indexing for timestamp calculations
type ChainIndex struct {
    BlockHeight int64     // Block height
    BlockTime   time.Time // Block timestamp
    BlockHash   []byte    // Block hash
    TxnCount    int64     // Number of transactions in block
}
```

### Account State Wrapper

All account types are wrapped with chain state:

```go
// AccountStateWrapper wraps any account type with its chain state
type AccountStateWrapper struct {
    ChainState *AccumulateChainState // Required chain state
    Account    interface{}           // Specific account type (KeyBook, TokenAccount, etc.)
    Proof      *MerkleProof         // Merkle proof for this account state
}

// Generic interface for all account types
type AccumulateAccount interface {
    GetURL() string
    GetType() string
    GetChainState() *AccumulateChainState
    GetMerkleState() *MerkleState
    CalculateTimestamp(blockIndex *ChainIndex) time.Time
    ValidateMerkleProof(proof *MerkleProof) bool
}
```

## Account Types

The light client uses existing Accumulate protocol types from `protocol/types_gen.go` with additional wrapper structures for chain state and merkle information:

### Core Protocol Types Used

The light client uses these existing Accumulate protocol types with their proper import paths:

```go
// Import paths for Accumulate protocol types
import (
    "gitlab.com/accumulatenetwork/accumulate/protocol"
    "gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
    "gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// From gitlab.com/accumulatenetwork/accumulate/protocol (types_gen.go)
type protocol.ADI struct {
    fieldsSet []bool
    Url       *url.URL `json:"url,omitempty"`
    protocol.AccountAuth
    extraData []byte
}

type protocol.TokenAccount struct {
    fieldsSet []bool
    Url       *url.URL `json:"url,omitempty"`
    protocol.AccountAuth
    TokenUrl  *url.URL `json:"tokenUrl,omitempty"`
    Balance   big.Int  `json:"balance,omitempty"`
    extraData []byte
}

type protocol.DataAccount struct {
    fieldsSet []bool
    Url       *url.URL `json:"url,omitempty"`
    protocol.AccountAuth
    Entry     protocol.DataEntry `json:"entry,omitempty"`
    extraData []byte
}

type protocol.KeyBook struct {
    fieldsSet []bool
    Url       *url.URL `json:"url,omitempty"`
    BookType  protocol.BookType `json:"bookType,omitempty"`
    protocol.AccountAuth
    PageCount uint64 `json:"pageCount,omitempty"`
    extraData []byte
}

type protocol.KeyPage struct {
    fieldsSet           []bool
    Url                 *url.URL `json:"url,omitempty"`
    CreditBalance       uint64   `json:"creditBalance,omitempty"`
    AcceptThreshold     uint64   `json:"acceptThreshold,omitempty"`
    RejectThreshold     uint64   `json:"rejectThreshold,omitempty"`
    ResponseThreshold   uint64   `json:"responseThreshold,omitempty"`
    BlockThreshold      uint64   `json:"blockThreshold,omitempty"`
    Version             uint64   `json:"version,omitempty"`
    Keys                []*protocol.KeySpec           `json:"keys,omitempty"`
    TransactionBlacklist *protocol.AllowedTransactions `json:"transactionBlacklist,omitempty"`
    extraData           []byte
}

// Supporting types from protocol package
type protocol.AccountAuth struct {
    Authorities []protocol.AuthorityEntry `json:"authorities,omitempty"`
}

type protocol.KeySpec struct {
    PublicKeyHash []byte `json:"publicKeyHash,omitempty"`
    LastUsedOn    uint64 `json:"lastUsedOn,omitempty"`
    Delegate      *url.URL `json:"delegate,omitempty"`
}
```

### Chain State and Merkle Integration

```go
// From gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle (types_gen.go)
type merkle.State struct {
    // Count is the count of hashes added to the tree
    Count int64 `json:"count,omitempty"`
    // Pending is the hashes that represent the left edge of the tree
    Pending [][]byte `json:"pending,omitempty"`
    // HashList is the hashes added to the tree
    HashList [][]byte `json:"hashList,omitempty"`
}

// From gitlab.com/accumulatenetwork/accumulate/protocol (types_gen.go)
type protocol.ChainMetadata struct {
    fieldsSet []bool
    Name      string    `json:"name,omitempty"`
    Type      protocol.ChainType `json:"type,omitempty"`
    extraData []byte
}

type protocol.BlockLedger struct {
    fieldsSet []bool
    Url       *url.URL      `json:"url,omitempty"`
    Index     uint64        `json:"index,omitempty"`
    Time      time.Time     `json:"time,omitempty"`
    Entries   []*protocol.BlockEntry `json:"entries,omitempty"`
    extraData []byte
}

type protocol.BlockEntry struct {
    fieldsSet []bool
    Account   *url.URL `json:"account,omitempty"`
    Chain     string   `json:"chain,omitempty"`
    Index     uint64   `json:"index,omitempty"`
    Entry     []byte   `json:"entry,omitempty"`
    extraData []byte
}
```

### Light Client Wrapper Structures

These are the custom wrapper structures for the light client package:

```go
// Light client package structures
package lightclient

import (
    "time"
    "gitlab.com/accumulatenetwork/accumulate/protocol"
    "gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
)

// AccumulateChainState represents blockchain state for any account
type AccumulateChainState struct {
    // BlockHeight is the current block height for this account state
    BlockHeight uint64 `json:"blockHeight"`
    
    // BlockIndex is the index within the block
    BlockIndex uint64 `json:"blockIndex"`
    
    // TransactionHash is the hash of the transaction that created this state
    TransactionHash []byte `json:"transactionHash,omitempty"`
    
    // ChainHead is the current chain head hash
    ChainHead []byte `json:"chainHead,omitempty"`
    
    // Timestamp is when this state was recorded
    Timestamp time.Time `json:"timestamp"`
    
    // SequenceNumber is the sequence number for this state
    SequenceNumber uint64 `json:"sequenceNumber"`
    
    // MerkleState contains the merkle tree state for cryptographic verification
    MerkleState *merkle.State `json:"merkleState,omitempty"`
    
    // ChainMetadata provides chain information
    ChainMetadata []protocol.ChainMetadata `json:"chainMetadata,omitempty"`
}

// AccountStateWrapper wraps any Accumulate account with chain state
type AccountStateWrapper struct {
    // Account is the underlying Accumulate account (ADI, TokenAccount, etc.)
    Account protocol.Account `json:"account"`
    
    // ChainState contains blockchain state information
    ChainState *AccumulateChainState `json:"chainState"`
    
    // LastUpdated is when this state was last refreshed
    LastUpdated time.Time `json:"lastUpdated"`
    
    // Source indicates where this data came from (network, cache, etc.)
    Source string `json:"source"`
}

// Specific account type wrappers for type safety
type ADIState struct {
    *protocol.ADI
    ChainState  *AccumulateChainState `json:"chainState"`
    LastUpdated time.Time             `json:"lastUpdated"`
}

type TokenAccountState struct {
    *protocol.TokenAccount
    ChainState  *AccumulateChainState `json:"chainState"`
    LastUpdated time.Time             `json:"lastUpdated"`
}

type DataAccountState struct {
    *protocol.DataAccount
    ChainState  *AccumulateChainState `json:"chainState"`
    LastUpdated time.Time             `json:"lastUpdated"`
}

type KeyBookState struct {
    *protocol.KeyBook
    ChainState  *AccumulateChainState `json:"chainState"`
    LastUpdated time.Time             `json:"lastUpdated"`
}

type KeyPageState struct {
    *protocol.KeyPage
    ChainState  *AccumulateChainState `json:"chainState"`
    LastUpdated time.Time             `json:"lastUpdated"`
}
```

## Implementation Approach

### Using Existing Accumulate Types

The light client implementation leverages existing Accumulate protocol types and structures:

1. **Protocol Types**: Use `protocol/types_gen.go` for all account structures (ADI, TokenAccount, DataAccount, KeyBook, KeyPage)
2. **Merkle State**: Use `pkg/database/merkle/types_gen.go` for merkle tree state management
3. **Chain Metadata**: Use `protocol.ChainMetadata` and `protocol.BlockLedger` for chain state information
4. **Marshaling/Unmarshaling**: Leverage existing generated marshal/unmarshal methods

### Modular File Structure

Each account type gets its own implementation file for better maintainability:

```
pkg/lightclient/
├── structures.go        # Core wrapper structures and chain state
├── adi.go              # ADI account type implementation
├── token.go            # TokenAccount implementation  
├── data.go             # DataAccount implementation
├── keybook.go          # KeyBook implementation
├── keypage.go          # KeyPage implementation
├── tracker.go          # Account tracking and registry
├── database.go         # Database backend implementation
├── daemon.go           # Daemon mode server
└── proof.go            # Cryptographic proof verification
```

### Account Type Integration Pattern

Each account type file follows this pattern:

```go
// Example: token.go
package lightclient

import (
    "gitlab.com/accumulatenetwork/accumulate/protocol"
    "gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
)

// TokenAccountState wraps protocol.TokenAccount with chain state
type TokenAccountState struct {
    *protocol.TokenAccount
    ChainState *AccumulateChainState `json:"chainState"`
    LastUpdated time.Time `json:"lastUpdated"`
}

// GetTokenAccount retrieves and wraps a token account with chain state
func (c *Client) GetTokenAccount(accountURL string) (*TokenAccountState, error) {
    // Query the network using existing client methods
    response, err := c.Query(accountURL)
    if err != nil {
        return nil, err
    }
    
    // Use existing protocol unmarshaling
    var account protocol.TokenAccount
    if err := account.UnmarshalJSON(response.Data); err != nil {
        return nil, err
    }
    
    // Extract chain state and merkle information from response
    chainState := extractChainState(response)
    
    return &TokenAccountState{
        TokenAccount: &account,
        ChainState:   chainState,
        LastUpdated:  time.Now(),
    }, nil
}

// extractChainState extracts blockchain state from query response
func extractChainState(response *QueryResponse) *AccumulateChainState {
    // Implementation extracts merkle state, block info, etc.
    // from the query response using existing Accumulate structures
}
```

### Direct Network Queries

The light client queries the network directly without caching:

```go
// GetTokenAccount retrieves a token account directly from the network
func (c *Client) GetTokenAccount(accountURL string) (*TokenAccountState, error) {
    // Query the network using existing client methods
    response, err := c.Query(accountURL)
    if err != nil {
        return nil, err
    }
    
    // Use existing protocol unmarshaling
    var account protocol.TokenAccount
    if err := account.UnmarshalJSON(response.Data); err != nil {
        return nil, err
    }
    
    // Extract chain state and merkle information from response
    chainState := extractChainState(response)
    
    return &TokenAccountState{
        TokenAccount: &account,
        ChainState:   chainState,
        LastUpdated:  time.Now(),
    }, nil
}
```

## Network-Only Architecture

The light client uses a stateless, network-only architecture that queries the Accumulate network directly for all data. This section explains how account tracking, updating, and client responses work in this architecture.

### How Account Tracking Works

**No Persistent Tracking**: The light client does not maintain any persistent tracking of accounts. Instead, it provides on-demand querying of specific accounts when requested.

**Operators Keybook Focus**: The primary focus is on querying the operators keybook (`dn.acme/operators`) and its associated key pages. This is the core network authority information.

**Query-Based Approach**: 
```go
// Each request queries the network directly
func (c *Client) GetOperatorsKeybook() (*KeyBookState, error) {
    // 1. Query dn.acme/operators directly from network
    response, err := c.Query("dn.acme/operators")
    if err != nil {
        return nil, fmt.Errorf("failed to query operators keybook: %w", err)
    }
    
    // 2. Unmarshal using protocol.KeyBook
    var keybook protocol.KeyBook
    if err := keybook.UnmarshalJSON(response.Data); err != nil {
        return nil, fmt.Errorf("failed to unmarshal keybook: %w", err)
    }
    
    // 3. Extract chain state and merkle information
    chainState := extractChainState(response)
    
    // 4. Return wrapped keybook with current state
    return &KeyBookState{
        KeyBook:     &keybook,
        ChainState:  chainState,
        LastUpdated: time.Now(),
    }, nil
}
```

### How Account Updating Works

**No Background Updates**: The light client does not perform any background updates or polling. All data is fetched on-demand when requested.

**Fresh Data on Every Request**: Each query returns the most current state directly from the Accumulate network:

```go
// Every call gets fresh data from the network
keybook1, err := client.GetOperatorsKeybook() // Fresh network query
keybook2, err := client.GetOperatorsKeybook() // Another fresh network query
```

**No State Persistence**: The light client maintains no state between calls. Each operation is independent:

```go
// No state carried between these calls
client := lightclient.NewClient("mainnet")
keybook, err := client.GetOperatorsKeybook()  // Query 1
keyPages, err := client.GetKeyPagesByKeybook(keybook.URL.String()) // Query 2
// Client can be discarded - no state to maintain
```

### How Client Responses Are Provided

**Direct Response Pattern**: The light client provides responses directly from network queries with added chain state information:

```go
// Response structure for all account queries
type KeyBookState struct {
    *protocol.KeyBook                    // Original Accumulate protocol data
    ChainState  *AccumulateChainState   // Added blockchain state information
    LastUpdated time.Time               // When this data was fetched
}

type AccumulateChainState struct {
    BlockHeight     uint64              // Current block height
    BlockIndex      uint64              // Index within block
    TransactionHash []byte              // Transaction hash
    ChainHead       []byte              // Chain head hash
    Timestamp       time.Time           // Block timestamp
    SequenceNumber  uint64              // Sequence number
    MerkleState     *merkle.State       // Merkle tree state for proofs
    ChainMetadata   []protocol.ChainMetadata // Chain metadata
}
```

**Cryptographic Verification**: Before providing responses, the light client verifies the cryptographic integrity:

```go
func (c *Client) GetOperatorsKeybook() (*KeyBookState, error) {
    // 1. Query network
    response, err := c.Query("dn.acme/operators")
    if err != nil {
        return nil, err
    }
    
    // 2. Verify cryptographic proofs
    if err := c.verifyMerkleProof(response); err != nil {
        return nil, fmt.Errorf("proof verification failed: %w", err)
    }
    
    // 3. Verify chain state consistency
    if err := c.verifyChainState(response); err != nil {
        return nil, fmt.Errorf("chain state verification failed: %w", err)
    }
    
    // 4. Only return verified data
    return buildKeyBookState(response), nil
}
```

**Error Handling**: Comprehensive error handling for network, parsing, and verification failures:

```go
// Example error handling in client responses
keybook, err := client.GetOperatorsKeybook()
if err != nil {
    switch {
    case strings.Contains(err.Error(), "network query failed"):
        // Handle network connectivity issues
    case strings.Contains(err.Error(), "proof verification failed"):
        // Handle cryptographic verification failures
    case strings.Contains(err.Error(), "unmarshal failed"):
        // Handle data parsing issues
    default:
        // Handle other errors
    }
}
```

### Query Flow Summary

1. **Client Request**: Application requests operators keybook data
2. **Network Query**: Light client queries `dn.acme/operators` from Accumulate network
3. **Data Verification**: Cryptographic proofs and chain state are verified
4. **Response Wrapping**: Protocol data is wrapped with chain state information
5. **Response Delivery**: Verified, wrapped data is returned to the application

**Key Benefits**:
- Always fresh data from the network
- No stale cache issues
- Cryptographic verification on every request
- Simple, stateless architecture
- No background processes or storage requirements

## Command Line Interface

The light client provides a simple command-line interface for querying operators keybook information:

```bash
# Query operators keybook from mainnet
light-client --server=mainnet

# Query with specific output format
light-client --server=mainnet --format=json

# Query with verbose output
light-client --server=mainnet --verbose
```

### CLI Options

```go
type CLIConfig struct {
    ServerURL string `json:"serverURL"` // Accumulate network endpoint
    Format    string `json:"format"`    // Output format: json, yaml, table
    Verbose   bool   `json:"verbose"`   // Verbose output
    Timeout   int    `json:"timeout"`   // Request timeout in seconds
}
```

### Command Line Arguments

```bash
# Server selection
--server=mainnet          # Use mainnet endpoint
--server=testnet          # Use testnet endpoint
--server=local            # Use local development endpoint
--server=https://...      # Use custom endpoint

# Output formatting
--format=json             # JSON output (default)
--format=yaml             # YAML output
--format=table            # Human-readable table

# Behavior options
--verbose                 # Show detailed information
--timeout=30              # Request timeout (default: 30s)
--help                    # Show help information
```

### Output Examples

**JSON Format** (default):
```json
{
  "keybook": {
    "url": "dn.acme/operators",
    "pageCount": 3,
    "bookType": "validator"
  },
  "keyPages": [
    {
      "url": "dn.acme/operators/1",
      "keys": [...],
      "threshold": 1
    }
  ],
  "chainState": {
    "blockHeight": 12345,
    "timestamp": "2024-01-15T10:30:00Z"
  }
}
```

**Table Format**:
```
Operators Keybook: dn.acme/operators
├── Page Count: 3
├── Book Type: validator
├── Block Height: 12345
└── Last Updated: 2024-01-15T10:30:00Z

Key Pages:
┌─────────────────────────┬───────────┬───────────┐
│ URL                     │ Keys      │ Threshold │
├─────────────────────────┼───────────┼───────────┤
│ dn.acme/operators/1     │ 5         │ 1         │
│ dn.acme/operators/2     │ 3         │ 2         │
│ dn.acme/operators/3     │ 7         │ 3         │
└─────────────────────────┴───────────┴───────────┘
```
    rpc ForceRefresh(ForceRefreshRequest) returns (ForceRefreshResponse);
}
```

### Wallet Integration

The daemon mode is specifically designed to serve wallet applications:

```go
type WalletService struct {
    lightClient *LightClient
    proofVerifier *ProofVerifier
}

// Verify and serve account data to wallet
func (ws *WalletService) GetVerifiedAccount(url string) (*VerifiedAccountResponse, error) {
    // 1. Get account from network
    networkResp, err := ws.lightClient.QueryNetwork(url)
    if err != nil {
        return nil, err
    }
    
    // 2. Verify cryptographic proof
    proof, err := ws.proofVerifier.VerifyResponse(networkResp)
    if err != nil {
        return nil, fmt.Errorf("proof verification failed: %w", err)
    }
    
    // 3. Return verified data
    return &VerifiedAccountResponse{
        Account: networkResp.Account,
        Proof:   proof,
        Verified: true,
    }, nil
}
```

## Cryptographic Proof Verification

### Proof Types

```go
type ProofType int

const (
    ProofMerkle ProofType = iota  // Merkle tree proof
    ProofSignature                // Digital signature proof
    ProofChain                    // Chain of trust proof
)

type CryptographicProof struct {
    Type        ProofType
    Data        []byte
    Signature   []byte
    PublicKey   []byte
    MerkleProof *MerkleProof
    ChainProof  *ChainProof
}
```

### Merkle Proof Verification

```go
type MerkleProof struct {
    RootHash    []byte
    LeafHash    []byte
    Siblings    [][]byte
    Path        []bool // true = right, false = left
}

func (p *MerkleProof) Verify() bool {
    currentHash := p.LeafHash
    
    for i, sibling := range p.Siblings {
        if p.Path[i] {
            // Current hash is left child
            currentHash = hash(currentHash, sibling)
        } else {
            // Current hash is right child
            currentHash = hash(sibling, currentHash)
        }
    }
    
    return bytes.Equal(currentHash, p.RootHash)
}
```

### Signature Verification

```go
type SignatureVerifier struct {
    operatorsKeys []*PublicKey
    threshold     int
}

func (sv *SignatureVerifier) VerifyResponse(response *QueryResponse) error {
    // Extract signatures from response
    signatures := response.GetSignatures()
    if len(signatures) < sv.threshold {
        return fmt.Errorf("insufficient signatures: got %d, need %d", 
            len(signatures), sv.threshold)
    }
    
    // Verify each signature against operators keys
    validSigs := 0
    responseHash := response.Hash()
    
    for _, sig := range signatures {
        if sv.verifySignature(responseHash, sig) {
            validSigs++
        }
    }
    
    if validSigs < sv.threshold {
        return fmt.Errorf("insufficient valid signatures: got %d, need %d", 
            validSigs, sv.threshold)
    }
    
    return nil
}
```

### Chain of Trust Verification

```go
type ChainProof struct {
    Authorities []Authority
    Signatures  []Signature
}

type Authority struct {
    URL       string
    PublicKey []byte
    Threshold int
}

func (cp *ChainProof) Verify(rootAuthorities []*Authority) error {
    // Verify chain from root authorities to target account
    currentAuthorities := rootAuthorities
    
    for i, authority := range cp.Authorities {
        // Verify authority is signed by current authorities
        if err := cp.verifyAuthoritySignature(authority, currentAuthorities, cp.Signatures[i]); err != nil {
            return fmt.Errorf("authority verification failed at level %d: %w", i, err)
        }
        currentAuthorities = []*Authority{&authority}
    }
    
    return nil
}
```

## Tool Implementations

### Core Light Client (`tools/light-client`)

**Purpose**: Collect and display the operators keybook and its key pages

**Key Features**:
- Queries `dn.acme/operators` for operators keybook
- Retrieves all key pages associated with the operators keybook
- Validates cryptographic signatures and merkle proofs
- Provides fresh data directly from the network

**Usage**:
```bash
go run tools/light-client/main.go mainnet
```

## Error Handling

The API provides comprehensive error handling:

- **Network Errors**: HTTP connection and timeout errors
- **Protocol Errors**: JSON-RPC error responses from the server
- **Parsing Errors**: Invalid response format or missing fields
- **Type Errors**: Account type mismatches

Example error handling:
```go
account, err := client.GetTokenAccount(ctx, "acc://example.acme/tokens")
if err != nil {
    // Handle specific error types
    if strings.Contains(err.Error(), "not a token account") {
        // Handle type mismatch
    } else if strings.Contains(err.Error(), "failed to query") {
        // Handle network/API error
    }
    return err
}
```

## Extensibility

### Adding New Account Types

1. Define the account struct in `accounts.go`
2. Implement the retrieval method following the pattern:
   ```go
   func (c *Client) GetNewAccountType(ctx context.Context, url string) (*NewAccountType, error)
   ```
3. Add parsing logic for the specific account type fields

### Adding Cryptographic Proof Verification

Future enhancements can include:
- Merkle proof verification for account states
- Signature verification for key pages
- Chain-of-trust validation for authorities

### Adding Caching and Performance Optimizations

- Response caching with TTL
- Parallel query execution
- Connection pooling for high-throughput scenarios

## Security Considerations

1. **HTTPS Usage**: Always use HTTPS endpoints for production
2. **Timeout Configuration**: Prevent hanging requests with appropriate timeouts
3. **Input Validation**: Validate account URLs before querying
4. **Error Information**: Avoid exposing sensitive information in error messages

## Implementation Details

### Package Structure

The light client implementation follows a clean, modular structure:

```go
// pkg/lightclient/client.go - Core client implementation
type Client struct {
    serverURL  string
    httpClient *http.Client
    timeout    time.Duration
}

// pkg/lightclient/keybook.go - KeyBook operations
func (c *Client) GetOperatorsKeybook() (*KeyBookState, error)
func (c *Client) ValidateKeybook(keybook *KeyBookState) error

// pkg/lightclient/keypage.go - KeyPage operations  
func (c *Client) GetKeyPage(url string) (*KeyPageState, error)
func (c *Client) GetKeyPagesByKeybook(keybookURL string) ([]*KeyPageState, error)

// pkg/lightclient/operations.go - High-level operations
func (c *Client) GetOperatorsInfo() (*OperatorsInfo, error)
```

### Core Data Structures

```go
// Operators information aggregate
type OperatorsInfo struct {
    Keybook   *KeyBookState   `json:"keybook"`
    KeyPages  []*KeyPageState `json:"keyPages"`
    Summary   *OperatorsSummary `json:"summary"`
    ChainState *AccumulateChainState `json:"chainState"`
    FetchedAt time.Time       `json:"fetchedAt"`
}

type OperatorsSummary struct {
    TotalPages     int `json:"totalPages"`
    TotalKeys      int `json:"totalKeys"`
    ActiveKeys     int `json:"activeKeys"`
    MinThreshold   int `json:"minThreshold"`
    MaxThreshold   int `json:"maxThreshold"`
}
```

## Usage Examples

### Basic Operators Keybook Query

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"
    
    "gitlab.com/accumulatenetwork/accumulate/pkg/lightclient"
)

func main() {
    // Create client for mainnet
    client, err := lightclient.NewClient("mainnet")
    if err != nil {
        log.Fatal("Failed to create client:", err)
    }
    
    // Query operators keybook
    keybook, err := client.GetOperatorsKeybook()
    if err != nil {
        log.Fatal("Failed to get operators keybook:", err)
    }
    
    fmt.Printf("Operators Keybook: %s\n", keybook.URL)
    fmt.Printf("Page Count: %d\n", keybook.PageCount)
    fmt.Printf("Block Height: %d\n", keybook.ChainState.BlockHeight)
    
    // Get all key pages
    keyPages, err := client.GetKeyPagesByKeybook(keybook.URL.String())
    if err != nil {
        log.Fatal("Failed to get key pages:", err)
    }
    
    for i, page := range keyPages {
        fmt.Printf("Key Page %d: %s (Keys: %d, Threshold: %d)\n", 
            i+1, page.URL, len(page.Keys), page.AcceptThreshold)
    }
}
```

### Complete Operators Information

```go
// Get comprehensive operators information
operatorsInfo, err := client.GetOperatorsInfo()
if err != nil {
    log.Fatal("Failed to get operators info:", err)
}

fmt.Printf("Operators Summary:\n")
fmt.Printf("  Total Pages: %d\n", operatorsInfo.Summary.TotalPages)
fmt.Printf("  Total Keys: %d\n", operatorsInfo.Summary.TotalKeys)
fmt.Printf("  Active Keys: %d\n", operatorsInfo.Summary.ActiveKeys)
fmt.Printf("  Threshold Range: %d-%d\n", 
    operatorsInfo.Summary.MinThreshold, 
    operatorsInfo.Summary.MaxThreshold)
fmt.Printf("  Fetched At: %s\n", operatorsInfo.FetchedAt.Format(time.RFC3339))
```

### Error Handling

```go
keybook, err := client.GetOperatorsKeybook()
if err != nil {
    switch {
    case strings.Contains(err.Error(), "network query failed"):
        fmt.Println("Network connectivity issue - check internet connection")
    case strings.Contains(err.Error(), "proof verification failed"):
        fmt.Println("Cryptographic verification failed - data may be compromised")
    case strings.Contains(err.Error(), "unmarshal failed"):
        fmt.Println("Data parsing error - API response format may have changed")
    default:
        fmt.Printf("Unknown error: %v\n", err)
    }
    return
}
```

## Architecture Summary

### Key Design Decisions

1. **Network-Only Architecture**: No caching or persistent storage - always fresh data
2. **Stateless Design**: No state maintained between operations
3. **Operators Keybook Focus**: Core functionality centers on `dn.acme/operators`
4. **Cryptographic Verification**: All responses verified before delivery
5. **Ordered Data Structures**: No maps for blockchain data to enable proof construction
6. **Modular Implementation**: Clean separation of concerns across packages

### Data Flow

1. **Application Request** → Light client API call
2. **Network Query** → JSON-RPC 2.0 query to Accumulate network
3. **Response Parsing** → Unmarshal using protocol types
4. **Cryptographic Verification** → Verify merkle proofs and chain state
5. **Response Wrapping** → Add chain state and metadata
6. **Delivery** → Return verified data to application

### Benefits

- **Always Current**: Data is always fresh from the network
- **Simple Architecture**: No complex caching or storage layers
- **Cryptographically Secure**: All data verified before delivery
- **Lightweight**: Minimal resource requirements
- **Reliable**: No stale data or cache invalidation issues

### Limitations

- **Network Dependent**: Requires network connectivity for all operations
- **No Offline Mode**: Cannot operate without network access
- **Query Latency**: Each request involves network round-trip
- **No Historical Data**: Only current state available (no tracking)

This design prioritizes simplicity, security, and data freshness over performance optimization and offline capabilities.

## Future Enhancements

1. **Enhanced Proof Verification**: More comprehensive cryptographic verification of merkle proofs
2. **Batch Operations**: Efficient bulk retrieval of multiple key pages
3. **Metrics and Monitoring**: Performance and reliability metrics for network queries
4. **Configuration Management**: External configuration files for endpoints and timeouts
5. **Advanced Output Formats**: Additional output formats (XML, CSV) for different use cases
6. **Query Optimization**: Connection pooling and request optimization for better performance
7. **Retry Logic**: Sophisticated retry mechanisms with exponential backoff
8. **Logging Framework**: Structured logging for better debugging and monitoring

## Known Issues and Limitations

### 1. Major Block Query Issues

**Problem**: The light client test demonstrates a known issue with major block queries that appears as timeout errors but is actually related to query construction or URL handling.

**Location**: `docs/tools/debug/lite_client_test.go` (lines 27-30)

**Symptoms**:
- Timeout errors when querying major blocks
- Misleading error messages that suggest network connectivity issues
- Server appears to reject requests in a way that manifests as client timeouts

**Root Cause**: 
- Issue is NOT actual network timeouts
- Problem likely related to URL construction or JSON-RPC request formatting
- May be related to how partition URLs are serialized in requests

**Current Workarounds**:
- Test tries multiple endpoints (kermit, testnet, mainnet)
- Uses different timeout values to isolate the issue
- Attempts different URL formats (Directory vs specific partitions like `acc://bvn0.acme`)

**Code References**:
- Test implementation: `docs/tools/debug/lite_client_test.go`
- Core client: `pkg/lightclient/client.go`
- Tool implementation: `tools/light-client/main.go`

### 2. Test Reliability Issues

**Problem**: Light client tests are marked as unreliable and contain explicit warnings about proper test maintenance.

**Location**: `docs/tools/debug/lite_client_test.go` (lines 32-33)

**Key Rule**: "DO NOT SKIP TESTS TO FIX THEM. Tests must be fixed properly rather than being skipped."

**Impact**: 
- Tests may fail intermittently due to network conditions
- Debugging is complicated by misleading error messages
- Proper validation of light client functionality is hindered

### 3. API Response Format Inconsistencies

**Problem**: The light client must handle different API response formats between versions.

**Details**:
- v3 API uses `account` field in responses
- Older API versions use `data` field
- Client must implement fallback logic for compatibility

**Code Reference**: `pkg/lightclient/client.go` - `GetData()` method

### 4. Experimental Code Status

**Problem**: There is experimental light client code that may not be production-ready.

**Location**: `exp/light/` directory contains experimental implementations:
- `client.go`, `client_test.go`
- `index.go`, `index_test.go` 
- `model.go`, `types.go`
- `sync.go`, `range.go`

**Status**: Unclear relationship between experimental code and production implementation

### 5. Documentation-Code Mismatch

**Problem**: The design document references code structure that doesn't fully match the actual implementation.

**Examples**:
- Design mentions `types.go`, `keybook.go`, `keypage.go`, `proof.go` in `pkg/lightclient/`
- Actual implementation has `client.go`, `accounts.go`, `operations.go`
- Some referenced files don't exist in the current codebase

**Impact**: Developers following the design document may not find the referenced files

## Recommendations for Issue Resolution

### 1. Fix Major Block Query Issues
- Investigate JSON-RPC request formatting in `pkg/lightclient/client.go`
- Test URL serialization for different partition types
- Add comprehensive logging to identify exact failure point
- Validate against working API clients for comparison

### 2. Improve Test Reliability
- Fix root cause of timeout-like errors in `lite_client_test.go`
- Add better error differentiation between network and protocol issues
- Implement retry logic with exponential backoff
- Add comprehensive test coverage for different network conditions

### 3. Consolidate Experimental Code
- Evaluate experimental code in `exp/light/` for production readiness
- Either integrate useful features or remove experimental code
- Clarify which implementation is the primary/recommended one

### 4. Update Documentation
- Align design document with actual code structure
- Add proper code references with line numbers where applicable
- Document all known limitations and workarounds
- Include troubleshooting guide for common issues

### 5. API Compatibility
- Ensure robust handling of different API response formats
- Add comprehensive tests for API version compatibility
- Document supported API versions and migration paths

This design document provides a comprehensive overview of the Accumulate Light Client architecture, focusing on operators keybook collection with a network-only, stateless approach that prioritizes simplicity, security, and data freshness.
