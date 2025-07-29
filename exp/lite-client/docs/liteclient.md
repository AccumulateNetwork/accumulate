# LiteClient Internal Infrastructure Documentation

## 🎯 Overview

The `liteclient.go` file provides the **internal infrastructure** for the Accumulate Lite Client. This is the low-level client used by the public API and orchestrator components. **Users should interact with the public `Client` API instead** - this documentation is for developers working on the lite client internals.

## ✅ Implementation Status

### Account Data Retrieval - **COMPLETE**

The LiteClient's account data retrieval system is **functionally complete** and production-ready:

#### Supported Account Types

| Account Type | Protocol Struct | LiteClient Method | Status |
|--------------|-----------------|-------------------|--------|
| **ADI Token Account** | `*protocol.TokenAccount` | `GetAccountData()` | ✅ Complete |
| **ADI Identity** | `*protocol.ADI` | `GetAccountData()` | ✅ Complete |
| **Key Book** | `*protocol.KeyBook` | `GetAccountData()` | ✅ Complete |
| **Key Page** | `*protocol.KeyPage` | `GetAccountData()` | ✅ Complete |
| **Lite Token Account** | `*protocol.LiteTokenAccount` | `GetAccountData()` | ✅ Complete |
| **Anchor Ledger** | `map[string]interface{}` | `GetAccountData()` | ✅ Complete |
| **Directory Service** | `*protocol.ADI` | `GetAccountData()` | ✅ Complete |

#### Core Features Implemented

- ✅ **Universal Account API**: Single `GetAccountData()` method handles all account types
- ✅ **Type-Specific Methods**: `GetTokenBalance()`, `GetIdentityInfo()` for specialized data
- ✅ **Automatic Type Detection**: `GetAccountType()` identifies account types automatically
- ✅ **Intelligent Caching**: Cache-first approach with TTL and staleness detection
- ✅ **Batch Processing**: `batchRetrieveAccountData()` for multiple accounts
- ✅ **Error Handling**: Graceful handling of non-existent or inaccessible accounts
- ✅ **Account Categorization**: Proper classification (token, identity, key, unknown)

#### Test Coverage

```go
// Comprehensive test coverage in account_handlers_test.go
func TestAccountDataRetrieval(t *testing.T) {
    // Tests 16+ account types:
    // ✅ ADI accounts: token, identity, key book/page, staking
    // ✅ Lite accounts: multiple lite token variations
    // ✅ System accounts: DN anchors, directory service
    // ✅ Error handling: non-existent accounts
}

func TestAccountTypes(t *testing.T) {
    // Tests type detection for all supported account types
    // ✅ Returns proper type names and numeric codes
    // ✅ Handles errors gracefully
}
```

## 🏗️ Architecture

The `LiteClient` follows a **clean, layered architecture** with clear separation of concerns:

```
┌─────────────────────────────────────────────────────────────┐
│                    PUBLIC CLIENT API                        │
│                     (api.go)                               │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│                 ADI ORCHESTRATOR                            │
│               (adi_orchestrator.go)                        │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│                 LITE CLIENT CORE                            │
│                 (liteclient.go)                            │
│  ┌─────────────────────────────────────────────────────┐    │
│  │           UNIVERSAL ACCOUNT API                     │    │
│  │  • GetAccountData()                                 │    │
│  │  • GetTokenBalance()                                │    │
│  │  • GetIdentityInfo()                                │    │
│  └─────────────────────────────────────────────────────┘    │
│  ┌─────────────────────────────────────────────────────┐    │
│  │           PROOF VALIDATION                          │    │
│  │  • validateAndCacheProof()                          │    │
│  └─────────────────────────────────────────────────────┘    │
│  ┌─────────────────────────────────────────────────────┐    │
│  │           BATCH PROCESSING                          │    │
│  │  • batchRetrieveAccountStates()                     │    │
│  │  • batchValidateProofs()                            │    │
│  │  • batchRetrieveAccountData()                       │    │
│  └─────────────────────────────────────────────────────┘    │
│  ┌─────────────────────────────────────────────────────┐    │
│  │           VALIDATION HELPERS                        │    │
│  │  • validateAccountURL()                             │    │
│  │  • validateTransaction()                            │    │
│  └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

## 📁 Code Organization

The file is organized into **clear, logical sections** with descriptive headers:

### 🔧 Core Types
```go
type LiteClient struct {
    v2           *v2.Client        // Accumulate v2 API client
    v3           *jsonrpc.Client   // Accumulate v3 JSON-RPC client
    unifiedCache *UnifiedCache     // Comprehensive data cache
    mu           sync.RWMutex      // Thread safety
}
```

### 🏭 Constructor
- `NewLiteClient(server string)` - Creates internal LiteClient instance
- **Note**: Users should use `NewClient()` from the public API instead

### 🌐 Universal Account API (Internal)
These methods provide **unified access** to all Accumulate account types:

#### `GetAccountData(ctx, accountURL) (*AccountData, error)`
- **Purpose**: Retrieves account data for any account type
- **Caching**: Checks cache first, queries network if needed
- **Returns**: Unified `AccountData` structure with type information

#### `GetTokenBalance(ctx, accountURL) (*TokenBalanceInfo, error)`
- **Purpose**: Retrieves balance information for token accounts
- **Caching**: Cache-first with automatic network fallback
- **Returns**: Balance, token URL, and account type information

#### `GetIdentityInfo(ctx, accountURL) (*IdentityInfo, error)`
- **Purpose**: Retrieves information about identity (ADI) accounts
- **Caching**: Intelligent caching with TTL expiration
- **Returns**: Identity URL and key book information

### 🔐 Proof Validation (Internal)
#### `validateAndCacheProof(ctx, account, knownRoot) error`
- **Purpose**: Fetches, verifies, and caches cryptographic proofs
- **Process**: 
  1. Fetch proof from network
  2. Verify against known root hash
  3. Cache verified proof for future use
- **Security**: Full cryptographic validation before caching

### ⚡ Batch Processing (Internal)
These methods enable **efficient bulk operations** for the orchestrator:

#### `batchRetrieveAccountStates(ctx, accountUrls) error`
- **Purpose**: Processes multiple accounts efficiently
- **Phases**: 
  1. Batch proof validation
  2. Batch data retrieval
- **Optimization**: Parallel processing with error handling

#### `batchValidateProofs(ctx, accountUrls) error`
- **Purpose**: Validates proofs for multiple accounts
- **Features**: BPT root hash fetching with fallback

#### `batchRetrieveAccountData(ctx, accountUrls) error`
- **Purpose**: Retrieves and caches data for multiple accounts
- **Intelligence**: Account type detection and specialized caching

#### Helper Methods:
- `cacheTokenAccountData(ctx, accountURL)` - Token-specific caching
- `cacheIdentityAccountData(ctx, accountURL)` - Identity-specific caching

### ✅ Validation Helpers (Internal)
#### `validateAccountURL(accountURL) error`
- **Purpose**: Validates account URL format using Accumulate's URL package
- **Security**: Prevents malformed URL attacks

#### `validateTransaction(tx) error`
- **Purpose**: Validates transaction data structure
- **Checks**: Required fields, account URL format

## 🎯 Design Principles

### 1. **Internal Focus**
- This is **internal infrastructure** - not user-facing
- Clean separation from public API
- Used by orchestrator and public client

### 2. **Clear Organization**
- **Logical grouping** of related functions
- **Descriptive naming** that indicates purpose and scope
- **Section headers** for easy navigation

### 3. **Caching Strategy**
- **Cache-first approach** for all data access
- **Automatic TTL expiration** and freshness validation
- **Unified caching** across all data types

### 4. **Error Handling**
- **Comprehensive validation** at all entry points
- **Graceful degradation** when services are unavailable
- **Detailed error messages** for debugging

### 5. **Thread Safety**
- **RWMutex protection** for shared state
- **Safe concurrent access** to cache and clients
- **No race conditions** in batch operations

## 🚀 Performance Features

### Intelligent Caching
- **Multi-level caching**: Account data, balances, identity info, proofs
- **TTL-based expiration**: Automatic cache invalidation
- **Cache-first queries**: Sub-second response times for cached data

### Batch Processing
- **Parallel execution**: Multiple accounts processed simultaneously
- **Connection reuse**: HTTP connection pooling
- **Efficient validation**: Batch proof validation with shared root hash

### Resource Management
- **Memory efficiency**: Proper cleanup and resource management
- **Connection limiting**: Prevents resource exhaustion
- **Graceful shutdown**: Clean resource cleanup

## 🔧 Usage Examples

### For Orchestrator Development
```go
// Create internal client
client, err := NewLiteClient("https://mainnet.accumulatenetwork.io/v2")
if err != nil {
    return err
}

// Use universal account API
accountData, err := client.GetAccountData(ctx, "acc://myadi.acme")
if err != nil {
    return err
}

// Batch process accounts
accountURLs := []string{"acc://adi1.acme", "acc://adi2.acme"}
err = client.batchRetrieveAccountStates(ctx, accountURLs)
if err != nil {
    return err
}
```

### For Public API Development
```go
// DON'T use LiteClient directly in public API
// Instead, use the public Client which wraps LiteClient:

client, err := NewMainnetClient()  // This creates LiteClient internally
if err != nil {
    return err
}

// Use the stellar public API
adiData, err := client.GetADI(ctx, "acc://myadi.acme")
```

## 🎉 Benefits of This Design

### For Developers
- **Crystal clear organization** - Easy to understand and modify
- **Logical separation** - Each section has a clear purpose
- **No confusion** - Function names clearly indicate scope and purpose
- **Easy testing** - Well-defined interfaces for mocking

### For Maintenance
- **Single responsibility** - Each function has one clear job
- **Easy debugging** - Clear error messages and logging
- **Extensible** - Easy to add new account types or features
- **Documentation** - Self-documenting code structure

### For Performance
- **Efficient caching** - Intelligent cache management
- **Batch operations** - Optimized for bulk processing
- **Resource management** - No memory leaks or connection issues

## 🔮 Future Enhancements

The clean architecture makes it easy to add:
- **New account types** - Add to universal account API
- **Additional caching strategies** - Extend caching helpers
- **Performance optimizations** - Enhance batch processing
- **Monitoring and metrics** - Add observability features

---

**Remember**: This is internal infrastructure. Users should use the public `Client` API from `api.go` which provides the stellar, simplified interface with the single `GetADI()` method! 🌟
