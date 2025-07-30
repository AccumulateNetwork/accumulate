# Accumulate Lite Client Core Documentation

This document describes the core internal structures and functions of the Accumulate Lite Client implementation found in `liteclient.go`.

## Table of Contents

- [Core Structures](#core-structures)
- [Constructor Functions](#constructor-functions)
- [Main Orchestration Methods](#main-orchestration-methods)
- [Account Data Retrieval](#account-data-retrieval)
- [Proof Validation](#proof-validation)
- [Batch Processing](#batch-processing)
- [Validation Helpers](#validation-helpers)
- [Architecture Overview](#architecture-overview)

---

## Core Structures

### `LiteClient`
**Type**: Core Internal Struct  
**Purpose**: Orchestrates account data retrieval, proof generation, and caching.  
**Fields**:
- `v2 *v2.Client` - Accumulate v2 API client
- `v3 *jsonrpc.Client` - Accumulate v3 API client
- `unifiedCache *UnifiedCache` - Cache for account data, proofs, and transactions
- `adisOfInterest map[string]bool` - ADIs this client is tracking
- `accountHandler *AccountHandler` - Handler for account data retrieval
- `proofGenerator *HealingProofGenerator` - Generator for cryptographic proofs

**Role in Architecture**: The LiteClient is the central orchestrator in the internal/core layer. It coordinates between the account handler, proof generator, and cache components to provide verified account data to the public API layer.

### `VerifiedAccountInfo`
**Type**: Response Struct  
**Purpose**: Represents account data with cryptographic proof validation.  
**Fields**:
- `URL string` - Account URL
- `Type protocol.AccountType` - Account type
- `Balance string` - Account balance
- `Receipt *merkle.Receipt` - Cryptographic proof
- `Height int64` - Block height
- `LastUpdated time.Time` - When the data was last updated
- `Transactions []*TransactionInfo` - Transaction history

### `TransactionInfo`
**Type**: Response Struct  
**Purpose**: Represents transaction data.  
**Fields**:
- `TxID string` - Transaction ID
- `Type string` - Transaction type
- `Status string` - Transaction status
- `Timestamp time.Time` - Transaction timestamp
- `Amount string` - Transaction amount
- `From string` - Source account
- `To string` - Destination account

---

## Constructor Functions

### `NewLiteClient(serverURL string) (*LiteClient, error)`
**Type**: Constructor  
**Purpose**: Creates a new internal lite client with the unified architecture.  
**Parameters**:
- `serverURL string` - URL of the Accumulate API server

**Implementation Details**:
1. Creates v2 API client
2. Converts server URL to v3 format and creates v3 API client
3. Creates unified cache with default TTL
4. Creates healing proof generator
5. Creates account handler
6. Returns initialized LiteClient

**Error Handling**:
- Returns error if server URL is empty
- Returns error if v2 client creation fails
- Returns error if URL conversion fails
- Returns error if proof generator creation fails

### `convertToV3URL(v2URL string) (string, error)`
**Type**: Helper Function  
**Purpose**: Converts a v2 API URL to v3 format.  
**Parameters**:
- `v2URL string` - v2 API URL

**Implementation Details**:
1. Parses the URL
2. Replaces /v2 with /v3 in the path
3. If no /v2 found, appends /v3
4. Returns the modified URL

---

## Main Orchestration Methods

### `ProcessADI(ctx context.Context, adiURL string) ([]*AccountData, error)`
**Type**: Core Orchestration Method  
**Purpose**: Implements the GetADI workflow.  
**Parameters**:
- `ctx context.Context` - Context for the operation
- `adiURL string` - URL of the ADI to process

**Implementation Details**:
1. Checks cache for fresh data
2. If cache miss, discovers accounts using AccountHandler
3. Processes each account to retrieve data and generate proofs
4. Stores results in UnifiedCache
5. Returns verified account information

**Error Handling**:
- Returns error if ADI URL is invalid
- Returns error if account discovery fails
- Returns error if account processing fails

### `processAccount(ctx context.Context, accountURL string) (*AccountData, error)`
**Type**: Helper Method  
**Purpose**: Handles the complete workflow for a single account.  
**Parameters**:
- `ctx context.Context` - Context for the operation
- `accountURL string` - URL of the account to process

**Implementation Details**:
1. Retrieves account data using AccountHandler
2. Generates proof using HealingProofGenerator
3. Stores in cache using UnifiedCache
4. Returns account data with proof

**Error Handling**:
- Returns error if account URL is invalid
- Returns error if account data retrieval fails
- Returns error if proof generation fails

---

## Account Data Retrieval

### `getAccountData(ctx context.Context, accountURL string) (*AccountData, error)`
**Type**: Internal Method  
**Purpose**: Retrieves account data using the universal account API.  
**Parameters**:
- `ctx context.Context` - Context for the operation
- `accountURL string` - URL of the account to retrieve

**Implementation Details**:
1. Delegates to AccountHandler.GetAccountData
2. Returns account data or error

### `getTokenBalance(ctx context.Context, accountURL string) (*TokenBalanceInfo, error)`
**Type**: Internal Method  
**Purpose**: Retrieves token balance information.  
**Parameters**:
- `ctx context.Context` - Context for the operation
- `accountURL string` - URL of the token account

**Implementation Details**:
1. Delegates to AccountHandler.GetTokenBalance
2. Returns token balance information or error

### `getIdentityInfo(ctx context.Context, accountURL string) (*IdentityInfo, error)`
**Type**: Internal Method  
**Purpose**: Retrieves identity information.  
**Parameters**:
- `ctx context.Context` - Context for the operation
- `accountURL string` - URL of the identity account

**Implementation Details**:
1. Delegates to AccountHandler.GetIdentityInfo
2. Returns identity information or error

---

## Proof Validation

### `validateAndCacheProof(ctx context.Context, account string, knownRoot []byte) error`
**Type**: Internal Method  
**Purpose**: Fetches, verifies, and caches a proof for an account.  
**Parameters**:
- `ctx context.Context` - Context for the operation
- `account string` - Account URL to validate
- `knownRoot []byte` - Known BPT root hash for validation

**Implementation Details**:
1. Checks cache for existing proof
2. If not found or stale, generates new proof using HealingProofGenerator
3. Validates proof against known root
4. Caches validated proof
5. Returns error if validation fails

**Error Handling**:
- Returns error if proof generation fails
- Returns error if proof validation fails

---

## Batch Processing

### `batchRetrieveAccountStates(ctx context.Context, accountUrls []string) error`
**Type**: Internal Method  
**Purpose**: Retrieves and caches account states in batches.  
**Parameters**:
- `ctx context.Context` - Context for the operation
- `accountUrls []string` - List of account URLs to process

**Implementation Details**:
1. Validates proofs for all accounts
2. Retrieves account data for all accounts
3. Returns error if any operation fails

### `batchValidateProofs(ctx context.Context, accountUrls []string) error`
**Type**: Internal Method  
**Purpose**: Validates proofs for multiple accounts.  
**Parameters**:
- `ctx context.Context` - Context for the operation
- `accountUrls []string` - List of account URLs to validate

**Implementation Details**:
1. Fetches BPT root hash
2. Validates each account URL
3. Validates and caches proof for each account
4. Returns error if any operation fails

### `batchRetrieveAccountData(ctx context.Context, accountUrls []string) error`
**Type**: Internal Method  
**Purpose**: Fetches and caches data for multiple accounts.  
**Parameters**:
- `ctx context.Context` - Context for the operation
- `accountUrls []string` - List of account URLs to retrieve

**Implementation Details**:
1. Gets account data to determine type
2. Processes based on account type (token or identity)
3. Caches appropriate data
4. Continues processing even if some accounts fail

### `cacheTokenAccountData(ctx context.Context, accountURL string) error`
**Type**: Helper Method  
**Purpose**: Caches token account specific data.  
**Parameters**:
- `ctx context.Context` - Context for the operation
- `accountURL string` - URL of the token account

**Implementation Details**:
1. Gets token balance information
2. Stores in cache
3. Returns error if operation fails

### `cacheIdentityAccountData(ctx context.Context, accountURL string) error`
**Type**: Helper Method  
**Purpose**: Caches identity account specific data.  
**Parameters**:
- `ctx context.Context` - Context for the operation
- `accountURL string` - URL of the identity account

**Implementation Details**:
1. Gets identity information
2. Stores in cache
3. Returns error if operation fails

---

## Validation Helpers

### `validateAccountURL(accountURL string) error`
**Type**: Helper Method  
**Purpose**: Validates account URL format using Accumulate's URL package.  
**Parameters**:
- `accountURL string` - Account URL to validate

**Implementation Details**:
1. Checks if URL is empty
2. Validates URL format
3. Returns error if validation fails

### `validateTransaction(tx Transaction) error`
**Type**: Helper Method  
**Purpose**: Validates transaction data structure.  
**Parameters**:
- `tx Transaction` - Transaction to validate

**Implementation Details**:
1. Checks if transaction ID is empty
2. Checks if transaction account is empty
3. Validates account URL format
4. Returns error if validation fails

---

## Architecture Overview

The LiteClient implements a three-layer architecture:

1. **Public Layer** (api.go):
   - Exposes a simple API for users
   - Handles user configuration and preferences
   - Provides high-level methods like GetADI()

2. **Internal/Core Layer** (liteclient.go):
   - Orchestrates the workflow between components
   - Manages caching and proof validation
   - Coordinates account data retrieval and processing

3. **Data Layer** (Accumulate API clients):
   - Interacts with Accumulate protocol's v2 and v3 APIs
   - Retrieves raw account and chain data
   - Provides low-level access to the blockchain

The LiteClient serves as the central orchestrator in this architecture, coordinating between:

- **AccountHandler**: Responsible for retrieving and processing account data
- **HealingProofGenerator**: Generates cryptographic proofs for account validation
- **UnifiedCache**: Stores and manages cached data with TTL and invalidation

This architecture provides clear separation of concerns, maintainability, and testability while ensuring that the public API remains simple and user-friendly.
