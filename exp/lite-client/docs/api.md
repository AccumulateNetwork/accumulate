# Accumulate Lite Client API Documentation

This document describes all public functions and data structures available in the Accumulate Lite Client API, as well as internal helper functions.

## Table of Contents

- [Public API Functions](#public-api-functions)
- [Internal Helper Functions](#internal-helper-functions)
- [Data Structures](#data-structures)

---

## Public API Functions

These are the main functions that users of the lite client should interact with.

### Client Management

#### `NewClient(config *Config) (*Client, error)`
**Type**: Public Constructor  
**Purpose**: Creates a new lite client with the provided configuration. If config is nil, uses DefaultConfig().  
**Why**: Primary entry point for creating a lite client instance with custom configuration.

#### `NewMainnetClient() (*Client, error)`
**Type**: Public Constructor  
**Purpose**: Creates a client configured for Accumulate mainnet.  
**Why**: Convenience function for mainnet users who don't need custom configuration.

#### `NewTestnetClient() (*Client, error)`
**Type**: Public Constructor  
**Purpose**: Creates a client configured for Accumulate testnet.  
**Why**: Convenience function for testnet users who don't need custom configuration.

#### `NewDevnetClient() (*Client, error)`
**Type**: Public Constructor  
**Purpose**: Creates a client configured for local development.  
**Why**: Convenience function for local development environments.

#### `Close() error`
**Type**: Public Method  
**Purpose**: Releases all resources used by the client.  
**Why**: Proper resource cleanup to prevent memory leaks and close network connections.

### Core Data Retrieval

#### `GetADI(ctx context.Context, adiURL string) (*ADIData, error)`
**Type**: Public Method  
**Purpose**: Retrieves complete information about an ADI and all its accounts. This is the main entry point for the simplified API.  
**Why**: Primary function that automatically handles cache freshness verification and receipt construction/verification. Implements requirement 2.1-2.3 from requirements.md.

### Cache Management

#### `GetCachedADIs() []string`
**Type**: Public Method  
**Purpose**: Returns a list of ADI URLs that have cached data.  
**Why**: Allows users to see which ADIs are currently cached for monitoring and management purposes.

#### `GetCacheMetadata(adiURL string) (*CacheMetadata, error)`
**Type**: Public Method  
**Purpose**: Returns cache metadata for freshness verification including account count, update times, and TTL information.  
**Why**: Provides transparency into cache state for debugging and monitoring. Implements requirement 5.3 for cache metadata storage.

#### `ClearCache() error`
**Type**: Public Method  
**Purpose**: Clears all cached data.  
**Why**: Allows users to force a complete cache refresh when needed.

### ADI Interest Management

#### `AddADIOfInterest(adiURL string) error`
**Type**: Public Method  
**Purpose**: Adds an ADI to the list of ADIs this client cares about, enabling automatic caching and background updates.  
**Why**: Implements requirement 1.2 for dynamic ADI list updates (add new ADIs).

#### `RemoveADIOfInterest(adiURL string) error`
**Type**: Public Method  
**Purpose**: Removes an ADI from the list of ADIs this client cares about and prunes all cached data for that ADI.  
**Why**: Implements requirement 1.2 for dynamic ADI list updates (remove specific ADIs).

#### `GetADIsOfInterest() []string`
**Type**: Public Method  
**Purpose**: Returns the list of ADIs this client is currently tracking.  
**Why**: Allows users to see which ADIs are being monitored for transparency and management.

### Pruning Operations

#### `PruneADI(adiURL string) error`
**Type**: Public Method  
**Purpose**: Removes all cached data for a specific ADI.  
**Why**: Implements requirement 6.1 for pruning entire ADIs. Allows selective cache cleanup without affecting other ADIs.

#### `PruneAccount(accountURL string) error`
**Type**: Public Method  
**Purpose**: Removes cached data for a specific account under an ADI.  
**Why**: Implements requirement 1.2 and 6.1 for removing specific accounts under a given ADI. Provides fine-grained cache control.

#### `PruneStaleData(olderThan time.Duration) error`
**Type**: Public Method  
**Purpose**: Removes cached data older than the specified duration.  
**Why**: Implements requirement 6.2 for cache invalidation. Allows time-based cache cleanup to manage memory usage.

### Receipt Verification

#### `VerifyReceipt(ctx context.Context, accountURL string) (*ReceiptVerificationResult, error)`
**Type**: Public Method  
**Purpose**: Manually verifies a receipt for transparency, exposing the receipt verification process to users.  
**Why**: Implements requirement 3.2 for receipt verification. Provides transparency into the cryptographic proof validation process.

---

## Internal Helper Functions

These functions are used internally by the public API and should not be called directly by users.

#### `getCachedADI(adiURL string) *ADIData`
**Type**: Internal Helper  
**Purpose**: Checks if we have cached data for an ADI (without freshness check).  
**Why**: Used by GetADI() to retrieve cached data before checking freshness. Separates data retrieval from freshness validation logic.

#### `isCacheDataFresh(data *ADIData) bool`
**Type**: Internal Helper  
**Purpose**: Verifies if cached data meets freshness requirements based on TTL.  
**Why**: Used by GetADI() to implement requirement 2.2 for cache freshness verification. Centralizes TTL-based freshness logic.

---

## Data Structures

### Core Types

#### `Client`
**Type**: Main Client Struct  
**Purpose**: The simplified public interface for the Accumulate Lite Client. Users specify an ADI and get all data - proofs, caching, and validation are handled automatically.  
**Fields**:
- `config *Config` - Client configuration
- `impl *LiteClient` - Internal lite client implementation
- `orch *ADIOrchestrator` - ADI orchestration layer

### Response Types

#### `ADIData`
**Type**: Public Response Struct  
**Purpose**: Represents complete information about an ADI and all its accounts.  
**Fields**:
- `URL string` - ADI URL
- `Accounts []*SimpleAccountData` - List of accounts under this ADI
- `LastUpdated time.Time` - When this data was last updated
- `FromCache bool` - Whether this data came from cache

#### `CacheMetadata`
**Type**: Public Response Struct  
**Purpose**: Provides information about cached data freshness for monitoring and debugging.  
**Fields**:
- `ADIURL string` - ADI URL this metadata refers to
- `AccountCount int` - Number of cached accounts for this ADI
- `OldestUpdate time.Time` - Timestamp of oldest cached data
- `NewestUpdate time.Time` - Timestamp of newest cached data
- `IsFresh bool` - Whether the data is within TTL
- `TTL time.Duration` - Time-to-live setting

#### `ReceiptVerificationResult`
**Type**: Public Response Struct  
**Purpose**: Contains the result of receipt verification for transparency.  
**Fields**:
- `AccountURL string` - Account URL that was verified
- `ReceiptValid bool` - Whether the receipt passed validation
- `MerkleRoot string` - Merkle root hash from the receipt
- `BlockHeight uint64` - Block height from the verification
- `VerifiedAt time.Time` - When the verification was performed
- `ReceiptExists bool` - Whether a receipt was found

#### `SimpleAccountData`
**Type**: Public Response Struct  
**Purpose**: Represents simplified account information for public API consumption.  
**Fields**:
- `URL string` - Account URL
- `Type string` - Account type (token, identity, etc.)
- `Balance string` - Account balance (for token accounts)
- `Transactions []*SimpleTransaction` - Transaction history

#### `SimpleTransaction`
**Type**: Public Response Struct  
**Purpose**: Represents simplified transaction information for public API consumption.  
**Fields**:
- `TxID string` - Transaction ID
- `Type string` - Transaction type
- `Status string` - Transaction status
- `Timestamp time.Time` - Transaction timestamp
- `Amount string` - Transaction amount
- `From string` - Source account
- `To string` - Destination account

---

## API Design Principles

### Alignment with Requirements

The API is designed to fully implement the functional requirements from `requirements.md`:

1. **User-Specified ADI Management** (Req 1.1-1.2): `AddADIOfInterest()`, `RemoveADIOfInterest()`, `GetADIsOfInterest()`
2. **Cache Lookup and Freshness** (Req 2.1-2.3): `GetADI()` with automatic freshness checking
3. **Receipt Construction/Verification** (Req 3.1-3.2): `VerifyReceipt()` for transparency
4. **Account Data Retrieval** (Req 4.1-4.4): `GetADI()` handles all account types
5. **Caching** (Req 5.1-5.3): `GetCacheMetadata()`, automatic caching in `GetADI()`
6. **Pruning and Cache Invalidation** (Req 6.1-6.2): `PruneADI()`, `PruneAccount()`, `PruneStaleData()`

### Simplicity and Transparency

- **Single Entry Point**: `GetADI()` is the primary method users need
- **Automatic Optimization**: Caching, freshness checks, and proof validation happen automatically
- **Transparency Options**: `VerifyReceipt()` and `GetCacheMetadata()` provide visibility into internal processes
- **Granular Control**: Multiple pruning methods for different use cases

### Error Handling

All public methods return appropriate errors for:
- Invalid input parameters (empty URLs)
- Network failures
- Proof validation failures
- Cache misses

The API follows Go conventions with clear error messages and proper error wrapping.
