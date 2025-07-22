# Accumulate API v3 Query Types and Scope Reference

This document provides comprehensive documentation for all available query types and proper usage of the `scope` parameter in Accumulate Network API v3.

## Table of Contents

1. [Overview](#overview)
2. [Scope Parameter Usage](#scope-parameter-usage)
3. [Complete Query Types Reference](#complete-query-types-reference)
4. [Query Construction Examples](#query-construction-examples)
5. [Routing and Partitioning](#routing-and-partitioning)
6. [Best Practices](#best-practices)

## Overview

The Accumulate API v3 uses a unified query interface where all queries are routed based on a `scope` URL parameter and executed using typed query objects. This design enables efficient distribution of queries across network partitions while maintaining a consistent developer experience.

### Core Interface

```go
type Querier interface {
    Query(ctx context.Context, scope *url.URL, query Query) (Record, error)
}
```

## Scope Parameter Usage

### Purpose

The `scope` parameter is a `*url.URL` that serves as:
- **Resource Identifier**: Specifies the target account, chain, or transaction
- **Routing Key**: Determines which partition should handle the query
- **Context Provider**: Gives the query context about what resource to operate on

### Scope URL Formats

#### Account URLs
```
acc://alice.acme           # User account
acc://dn.acme             # Directory Node system account
acc://staking.acme        # Staking system account
acc://bvn-example.acme    # BVN partition account
```

#### Transaction URLs
```
acc://alice.acme/TXID123456789  # Specific transaction
```

#### Chain URLs
```
acc://alice.acme/main     # Main chain
acc://alice.acme/data     # Data chain
acc://alice.acme/scratch  # Scratch chain
```

### Routing Logic

The routing system uses `Router.RouteAccount(scope)` to determine the target partition:

#### System Account Routing (Directory Partition)
- `acc://dn.acme` → Directory
- `acc://staking.acme` → Directory
- `acc://ACME` → Directory (main token account)

#### User Account Routing (BVN Partitions)
- `acc://alice.acme` → BVN (determined by routing table)
- `acc://company.acme` → BVN (determined by routing table)

## Complete Query Types Reference

### Core Query Types

#### 1. DefaultQuery (QueryType: 0)
**Purpose**: Basic account and transaction state queries

**Fields**:
- `IncludeReceipt` (bool): Include transaction receipt data

**Usage**: General account information, transaction status

**Example Scope URLs**:
- `acc://alice.acme` - Query account state
- `acc://alice.acme/TXID123` - Query transaction status

#### 2. ChainQuery (QueryType: 1)
**Purpose**: Query chain data with range and indexing support

**Fields**:
- `Name` (string): Chain name (e.g., "main", "data")
- `Range` (*RangeOptions): Specify entry range
- `Index` (uint64): Specific entry index
- `IncludeReceipt` (bool): Include receipt data

**Usage**: Retrieve chain entries, transaction history

**Example Scope URLs**:
- `acc://alice.acme` - Query main chain
- `acc://alice.acme/data` - Query data chain

#### 3. DataQuery (QueryType: 2)
**Purpose**: Query data account entries

**Fields**:
- `Entry` (*DataEntryQuery): Specific entry query parameters
- `IncludeReceipt` (bool): Include receipt data

**Usage**: Retrieve stored data from data accounts

**Example Scope URLs**:
- `acc://datastore.acme` - Query data account entries

#### 4. DirectoryQuery (QueryType: 3)
**Purpose**: List directory contents and subdirectories

**Fields**:
- `Start` (uint64): Starting index for pagination
- `Count` (uint64): Number of entries to return
- `ExpandChains` (bool): Include chain information

**Usage**: Browse account hierarchies, list sub-accounts

**Example Scope URLs**:
- `acc://alice.acme` - List sub-accounts under alice.acme
- `acc://dn.acme` - List system accounts

#### 5. PendingQuery (QueryType: 4)
**Purpose**: Query pending transactions

**Fields**:
- `Range` (*RangeOptions): Range of pending transactions

**Usage**: Monitor transaction processing status

**Example Scope URLs**:
- `acc://alice.acme` - Query pending transactions for account

#### 6. BlockQuery (QueryType: 5)
**Purpose**: Query block information

**Fields**:
- `Minor` (uint64): Minor block number
- `Major` (uint64): Major block number
- `IncludeEntries` (bool): Include block entries

**Usage**: Retrieve block data and contained transactions

**Example Scope URLs**:
- `acc://dn.acme` - Query Directory partition blocks
- `acc://bvn-example.acme` - Query BVN partition blocks

### Search Query Types

#### 7. AnchorSearchQuery (QueryType: 16)
**Purpose**: Search for anchor records

**Fields**:
- `Anchor` ([]byte): Anchor hash to search for
- `IncludeReceipt` (bool): Include receipt data

**Usage**: Find transactions anchored to specific hashes

#### 8. PublicKeySearchQuery (QueryType: 17)
**Purpose**: Search by public key

**Fields**:
- `PublicKey` ([]byte): Public key to search for
- `Type` (SignatureType): Key type (Ed25519, RCD1, etc.)

**Usage**: Find accounts associated with a public key

#### 9. PublicKeyHashSearchQuery (QueryType: 18)
**Purpose**: Search by public key hash

**Fields**:
- `PublicKeyHash` ([]byte): Hash of public key
- `Type` (SignatureType): Key type

**Usage**: Find accounts by key hash (more efficient than full key)

#### 10. DelegateSearchQuery (QueryType: 19)
**Purpose**: Search for delegate information

**Fields**:
- `Delegate` (*url.URL): Delegate account URL

**Usage**: Find accounts that delegate to a specific validator

#### 11. MessageHashSearchQuery (QueryType: 20)
**Purpose**: Search by message hash

**Fields**:
- `Hash` ([]byte): Message hash to search for

**Usage**: Find transactions by their message hash

## Query Construction Examples

### Basic Account Query

```go
// Create scope URL
scope, _ := url.Parse("acc://alice.acme")

// Create default query
query := &api.DefaultQuery{
    IncludeReceipt: true,
}

// Execute query
record, err := querier.Query(ctx, scope, query)
```

### Chain Data Query

```go
// Query specific chain entries
scope, _ := url.Parse("acc://alice.acme")

query := &api.ChainQuery{
    Name: "main",
    Range: &api.RangeOptions{
        Start: 0,
        Count: 10,
    },
    IncludeReceipt: false,
}

record, err := querier.Query(ctx, scope, query)
```

### Directory Listing

```go
// List sub-accounts
scope, _ := url.Parse("acc://alice.acme")

query := &api.DirectoryQuery{
    Start: 0,
    Count: 50,
    ExpandChains: true,
}

record, err := querier.Query(ctx, scope, query)
```

### Public Key Search

```go
// Search for accounts by public key
scope, _ := url.Parse("acc://dn.acme") // Search in Directory

query := &api.PublicKeySearchQuery{
    PublicKey: publicKeyBytes,
    Type: protocol.SignatureTypeEd25519,
}

record, err := querier.Query(ctx, scope, query)
```

## Routing and Partitioning

### Partition Types

1. **Directory Partition**: Handles system accounts and global state
2. **BVN Partitions**: Handle user accounts and transactions

### Routing Override Examples

```go
// These accounts always route to Directory partition
systemAccounts := []string{
    "acc://dn.acme",        // Directory Node
    "acc://staking.acme",   // Staking registry
    "acc://ACME",           // Main token account
}

// User accounts route to BVN partitions based on routing table
userAccounts := []string{
    "acc://alice.acme",     // Routes to assigned BVN
    "acc://company.acme",   // Routes to assigned BVN
}
```

### Query Routing Process

1. **Parse Scope**: Extract account URL from scope parameter
2. **Route Account**: Use `Router.RouteAccount(scope)` to determine partition
3. **Execute Query**: Forward query to appropriate partition's Querier
4. **Return Result**: Marshal response back to client

## Best Practices

### Scope URL Construction

```go
// ✅ Good: Use proper URL parsing
scope, err := url.Parse("acc://alice.acme")
if err != nil {
    return err
}

// ❌ Bad: Manual string construction
scope := &url.URL{Scheme: "acc", Host: "alice.acme"}
```

### Query Parameter Validation

```go
// ✅ Good: Validate query parameters
query := &api.ChainQuery{
    Name: "main",
    Range: &api.RangeOptions{
        Start: 0,
        Count: 100, // Reasonable limit
    },
}

if !query.IsValid() {
    return errors.New("invalid query parameters")
}
```

### Error Handling

```go
// ✅ Good: Handle routing and query errors
record, err := querier.Query(ctx, scope, query)
if err != nil {
    switch {
    case errors.Is(err, ErrNotFound):
        // Handle not found
    case errors.Is(err, ErrInvalidScope):
        // Handle invalid scope
    default:
        // Handle other errors
    }
}
```

### Performance Considerations

1. **Batch Queries**: Use range queries instead of multiple individual queries
2. **Pagination**: Use appropriate count limits for large result sets
3. **Receipt Inclusion**: Only request receipts when necessary
4. **Scope Specificity**: Use the most specific scope possible for better routing

### Common Patterns

#### Account State Monitoring

```go
func monitorAccount(querier api.Querier, accountURL string) {
    scope, _ := url.Parse(accountURL)
    
    query := &api.DefaultQuery{
        IncludeReceipt: false, // Faster without receipts
    }
    
    record, err := querier.Query(ctx, scope, query)
    // Process account state...
}
```

#### Transaction History Retrieval

```go
func getTransactionHistory(querier api.Querier, accountURL string, limit int) {
    scope, _ := url.Parse(accountURL)
    
    query := &api.ChainQuery{
        Name: "main",
        Range: &api.RangeOptions{
            Start: 0,
            Count: uint64(limit),
        },
        IncludeReceipt: true, // Include receipts for history
    }
    
    record, err := querier.Query(ctx, scope, query)
    // Process transaction history...
}
```

## Migration from API v2

### Key Differences

1. **Unified Interface**: Single `Query` method instead of multiple specific methods
2. **Typed Queries**: Query parameters are strongly typed structs
3. **URL Scoping**: All queries require a scope URL for routing
4. **Record Responses**: All responses implement the `Record` interface

### Migration Example

```go
// API v2 (old)
account, err := client.QueryAccount(accountURL)

// API v3 (new)
scope, _ := url.Parse(accountURL)
query := &api.DefaultQuery{}
record, err := querier.Query(ctx, scope, query)
account := record.(*api.AccountRecord)
```

This comprehensive reference provides all the information needed to effectively use the Accumulate API v3 query system with proper scope parameter usage and query type selection.
