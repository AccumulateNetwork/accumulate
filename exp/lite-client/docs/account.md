# Accumulate Lite Client Account Handler Documentation

This document describes the account handling system in the Accumulate Lite Client implementation found in `account.go`.

## Table of Contents

- [Core Structures](#core-structures)
- [Constructor Functions](#constructor-functions)
- [Account Data Retrieval](#account-data-retrieval)
- [Account Type Detection](#account-type-detection)
- [Account Type Conversion](#account-type-conversion)
- [Data Structures](#data-structures)
- [Helper Functions](#helper-functions)
- [Architecture Overview](#architecture-overview)

---

## Core Structures

### `AccountHandler`
**Type**: Core Internal Struct  
**Purpose**: Responsible for retrieving and processing account data with type detection and caching.  
**Fields**:
- `client *LiteClient` - Reference to the parent lite client for accessing API clients and cache

**Role in Architecture**: The AccountHandler is a specialized component in the internal/core layer that handles all account-related operations. It abstracts the complexity of different account types and provides a unified interface for account data retrieval.

---

## Constructor Functions

### `NewAccountHandler(client *LiteClient) *AccountHandler`
**Type**: Internal Constructor  
**Purpose**: Creates a new account handler with the given lite client reference.  
**Parameters**:
- `client *LiteClient` - The parent lite client instance
**Returns**: `*AccountHandler` - New account handler instance  
**Why**: Initializes the account handler with access to the lite client's API clients and cache.

---

## Account Data Retrieval

### `GetAccountData(ctx context.Context, accountURL string) (*AccountData, error)`
**Type**: Public Method  
**Purpose**: Retrieves account data for the specified account URL with cache-first strategy.  
**Parameters**:
- `ctx context.Context` - Request context
- `accountURL string` - The account URL to retrieve data for
**Returns**: `*AccountData` - Account data with type information  
**Why**: Primary method for getting account data. Implements cache-first strategy with automatic fallback to network queries. Validates URLs and handles TTL-based cache invalidation.

**Workflow**:
1. Validates account URL format
2. Checks unified cache for existing data
3. Verifies data freshness using TTL
4. Falls back to network query if cache miss or stale data

### `getAccountDataFromNetwork(ctx context.Context, accountUrl string) (*AccountData, error)`
**Type**: Internal Helper  
**Purpose**: Retrieves account data directly from the Accumulate network using v2 API.  
**Parameters**:
- `ctx context.Context` - Request context
- `accountUrl string` - The account URL to query
**Returns**: `*AccountData` - Fresh account data from network  
**Why**: Handles the actual network communication with proper error handling and response parsing. Automatically caches results for future use.

**Implementation Details**:
- Uses `v2api.GeneralQuery` with URL parsing
- Extracts account type from response metadata
- Parses response data field into structured format
- Automatically stores results in unified cache

### `GetTokenBalance(ctx context.Context, accountURL string) (*TokenBalanceInfo, error)`
**Type**: Public Method  
**Purpose**: Retrieves balance information specifically for token accounts.  
**Parameters**:
- `ctx context.Context` - Request context
- `accountURL string` - Token account URL
**Returns**: `*TokenBalanceInfo` - Balance and token information  
**Why**: Specialized method for token account data with balance-specific fields and validation.

### `GetIdentityInfo(ctx context.Context, accountURL string) (*IdentityInfo, error)`
**Type**: Public Method  
**Purpose**: Retrieves identity information for ADI accounts.  
**Parameters**:
- `ctx context.Context` - Request context
- `accountURL string` - ADI account URL
**Returns**: `*IdentityInfo` - Identity and key book information  
**Why**: Specialized method for ADI accounts with identity-specific metadata.

### `DiscoverADIAccounts(ctx context.Context, adiURL string) ([]string, error)`
**Type**: Public Method  
**Purpose**: Discovers all accounts associated with an ADI by querying the ADI's directory.  
**Parameters**:
- `ctx context.Context` - Request context
- `adiURL string` - The ADI URL to discover accounts for
**Returns**: `[]string` - List of account URLs belonging to the ADI  
**Why**: Essential for ADI processing workflow. Enables the lite client to find all accounts under an ADI without prior knowledge of account names.

---

## Account Type Detection

### `IsTokenAccount() bool`
**Type**: AccountData Method  
**Purpose**: Returns true if this is any type of token account (lite or ADI token account).  
**Why**: Enables type-specific processing and validation for token-related operations.

### `IsDataAccount() bool`
**Type**: AccountData Method  
**Purpose**: Returns true if this is any type of data account (lite or ADI data account).  
**Why**: Identifies accounts that store arbitrary data rather than tokens or identity information.

### `IsIdentityAccount() bool`
**Type**: AccountData Method  
**Purpose**: Returns true if this is an ADI (Identity) account.  
**Why**: Identifies the root identity accounts that manage ADI metadata and account directories.

### `IsKeyAccount() bool`
**Type**: AccountData Method  
**Purpose**: Returns true if this is a key management account (key page or key book).  
**Why**: Identifies accounts responsible for cryptographic key management and authorization.

---

## Account Type Conversion

### `AsLiteTokenAccount() (*protocol.LiteTokenAccount, error)`
**Type**: AccountData Method  
**Purpose**: Safely converts account data to LiteTokenAccount struct if applicable.  
**Returns**: `*protocol.LiteTokenAccount` - Typed account data  
**Why**: Provides type-safe access to lite token account specific fields and methods.

### `AsTokenAccount() (*protocol.TokenAccount, error)`
**Type**: AccountData Method  
**Purpose**: Safely converts account data to TokenAccount struct if applicable.  
**Returns**: `*protocol.TokenAccount` - Typed account data  
**Why**: Provides type-safe access to ADI token account specific fields and methods.

### `AsADI() (*protocol.ADI, error)`
**Type**: AccountData Method  
**Purpose**: Safely converts account data to ADI struct if applicable.  
**Returns**: `*protocol.ADI` - Typed account data  
**Why**: Provides type-safe access to ADI identity account specific fields and methods.

---

## Data Structures

### `TokenBalanceInfo`
**Type**: Response Struct  
**Purpose**: Contains balance information for token accounts.  
**Fields**:
- `AccountURL string` - The account URL
- `AccountType string` - Human-readable account type
- `Balance string` - Token balance as string
- `TokenURL string` - URL of the token issuer
- `CreditBalance uint64` - Credit balance for transaction fees

### `IdentityInfo`
**Type**: Response Struct  
**Purpose**: Contains information about identity accounts.  
**Fields**:
- `AccountURL string` - The account URL
- `IdentityURL string` - The ADI identity URL
- `KeyBook string` - URL of the associated key book

### `DataAccountInfo`
**Type**: Response Struct  
**Purpose**: Contains information about data accounts.  
**Fields**:
- `AccountURL string` - The account URL
- `AccountType string` - Human-readable account type
- `DataURL string` - URL of the data account
- `KeyBook string` - URL of the associated key book

### `AccountSummary`
**Type**: Response Struct  
**Purpose**: Provides a unified view of any account type.  
**Fields**:
- `AccountURL string` - The account URL
- `AccountType string` - Human-readable account type
- `Category string` - Account category (token, identity, key, etc.)
- `Balance string` - Token balance (if applicable)
- `TokenURL string` - Token issuer URL (if applicable)
- `KeyBook string` - Key book URL (if applicable)

### `GenericAccount`
**Type**: Fallback Struct  
**Purpose**: Wrapper for unknown or unsupported account types.  
**Fields**:
- `AccountType protocol.AccountType` - The account type enum
- `RawData map[string]interface{}` - Raw account data

---

## Helper Functions

### `mapToStruct(data map[string]interface{}, target interface{}) error`
**Type**: Internal Helper  
**Purpose**: Converts a map to a struct using JSON marshaling/unmarshaling.  
**Parameters**:
- `data map[string]interface{}` - Source data map
- `target interface{}` - Target struct to populate
**Why**: Provides safe conversion from API response maps to typed structs with proper error handling.

---

## Architecture Overview

The AccountHandler serves as the specialized account data layer in the lite client architecture:

### **Layer Position**
- **Above**: LiteClient orchestration layer
- **Below**: Accumulate v2 API and UnifiedCache
- **Peers**: HealingProofGenerator, UnifiedCache

### **Key Responsibilities**
1. **Account Discovery**: Finding all accounts associated with an ADI
2. **Type Detection**: Identifying account types from API responses
3. **Data Retrieval**: Fetching account data with cache optimization
4. **Type Conversion**: Providing safe access to typed account structures
5. **Specialized Queries**: Token balance and identity information retrieval

### **Design Patterns**
- **Cache-First Strategy**: Always check cache before network queries
- **Type Safety**: Provide typed access to account data with validation
- **Separation of Concerns**: Handle only account-related operations
- **Error Propagation**: Detailed error messages with context

### **Integration Points**
- **LiteClient**: Receives account handler instance and delegates account operations
- **UnifiedCache**: Stores and retrieves account data with TTL management
- **v2 API Client**: Performs actual network queries to Accumulate network
- **Protocol Types**: Uses official Accumulate protocol structures for type safety

The AccountHandler abstracts the complexity of Accumulate's diverse account types while providing a clean, cache-optimized interface for the rest of the lite client system.
