# Database Requirements for Healing

This document details the database requirements for the healing processes in the Accumulate network. It focuses on the interface definitions, implementation guidelines, and the specific needs of each healing approach.

## Database Usage in Healing Approaches {#database-usage-in-healing-approaches}

The healing approaches have different database requirements:

1. **heal_anchor**: Does not require a database
2. **heal_synth**: Requires a database to build Merkle proofs

**Related Documentation**:
- [heal_anchor Approach](./overview.md#heal-anchor)
- [heal_synth Approach](./overview.md#heal-synth)
- [Light Client as Infrastructure](./overview.md#light-client)

## Database Interface Requirements {#database-interface-requirements}

The healing code expects the database to implement several key interfaces:

### 1. keyvalue.Beginner Interface {#keyvalue-beginner}

```go
// The Beginner interface is used to begin transactions
type Beginner interface {
    Begin(writable bool) (Batch, error)
}
```

This interface is used to start database transactions, which can be read-only or read-write.

### 2. keyvalue.Batch Interface {#keyvalue-batch}

```go
// The Batch interface represents a database transaction
type Batch interface {
    Get(key []byte) ([]byte, error)
    Put(key, value []byte) error
    Delete(key []byte) error
    Commit() error
    Discard()
}
```

This interface provides the core operations needed within a transaction:
- `Get`: Retrieve a value by key
- `Put`: Store a key-value pair
- `Delete`: Remove a key-value pair
- `Commit`: Commit the transaction
- `Discard`: Discard the transaction

### 3. io.Closer Interface {#io-closer}

```go
// The Closer interface is used to close the database
type Closer interface {
    Close() error
}
```

This interface is used to properly close the database when it's no longer needed.

**Related Documentation**:
- [Light Client Implementation](./light_client.md#light-client-interface)
- [Implementation Guidelines](./implementation.md#bright-line-guidelines)

## Current Database Implementation {#current-database-implementation}

The current implementation uses Bolt DB, a key-value store that provides ACID transactions:

```go
// In heal_common.go
func (h *healer) initDB() error {
    // Create a temporary directory for the database
    dir, err := os.MkdirTemp("", "heal-*")
    if err != nil {
        return errors.UnknownError.WithFormat("create temp dir: %w", err)
    }
    h.dbDir = dir

    // Open the database
    db, err := bolt.Open(filepath.Join(dir, "accumulate.db"), 0600, nil)
    if err != nil {
        return errors.UnknownError.WithFormat("open database: %w", err)
    }
    h.db = db

    // Initialize the Light client
    h.light = light.New(db, nil, nil)
    return nil
}
```

**Key Files**:
- `heal_common.go`: Contains database initialization code
- `pkg/database/keyvalue/bolt`: Bolt DB implementation

## Database Usage in heal_synth {#database-usage-in-heal-synth}

The `heal_synth` approach uses the database primarily for building Merkle proofs:

```go
// In healing/synthetic.go
func (h *Healer) buildSynthReceiptV1(ctx context.Context, args HealSyntheticArgs, si SequencedInfo) (*merkle.Receipt, error) {
    // Find the root anchor using the Light client (which uses the database)
    rootAnchor, err := h.findRootAnchor(ctx, args, si)
    if err != nil {
        return nil, err
    }

    // Build the receipt from the root anchor
    receipt, err := h.buildReceiptFromRootAnchor(ctx, args, si, rootAnchor)
    if err != nil {
        return nil, err
    }

    return receipt, nil
}
```

The Light client uses the database to:
1. Store and retrieve chain entries
2. Track anchor relationships
3. Cache transaction data
4. Build Merkle proofs for synthetic transactions

**Related Documentation**:
- [heal_synth Transaction Creation](./transactions.md#heal_synth-transaction-creation)
- [Light Client Usage in Synthetic Transaction Healing](./light_client.md#light-client-usage-in-synthetic-healing)

## In-Memory Database Implementation {#in-memory-database-implementation}

When implementing an in-memory database replacement for Bolt DB, ensure it:

1. **Implements the Required Interfaces**:
   - [`keyvalue.Beginner`](#keyvalue-beginner)
   - [`keyvalue.Batch`](#keyvalue-batch)
   - [`io.Closer`](#io-closer)

2. **Preserves Transaction Semantics**:
   - Maintains ACID properties
   - Supports concurrent transactions
   - Properly handles transaction isolation

3. **Supports the Key Structure**:
   - Uses the same key format as the existing implementation
   - Handles binary keys and values correctly

4. **Provides Efficient Access**:
   - Optimizes for chain access patterns
   - Efficiently handles large values (like chain entries)

5. **Handles URL Formats Correctly**:
   - Stores keys exactly as provided
   - Does not modify or interpret URL-derived keys

**Related Documentation**:
- [Version 2 Documentation](../heal_v2/heal_mem_db.md)
- [URL Construction Considerations](#url-construction-considerations)

## URL Construction Considerations {#url-construction-considerations}

A critical aspect of the database implementation is handling the different URL construction methods used by different parts of the codebase:

- `sequence.go` uses raw partition URLs (e.g., `acc://bvn-Apollo.acme`)
- `heal_anchor.go` appends the partition ID to the anchor pool URL (e.g., `acc://dn.acme/anchors/Apollo`)

The in-memory database must:
1. Store keys exactly as provided without modification
2. Not attempt to parse or normalize URLs
3. Support lookups using the exact key format provided by each component

This ensures compatibility with all healing approaches regardless of their URL construction methods.

**Related Documentation**:
- [URL Construction Differences](../heal.md#url-construction-differences)
- [Implementation Guidelines: URL Construction](./implementation.md#url-construction)

## Testing the Database Implementation {#testing-the-database}

When implementing a replacement database, test it with:

1. **heal_synth**: Verify it can build Merkle proofs correctly
2. **Multiple Concurrent Transactions**: Verify proper isolation
3. **Large Chain Entries**: Verify efficient handling of large values
4. **Different URL Formats**: Verify compatibility with different URL construction methods

## Implementation Checklist {#implementation-checklist}

When implementing a new database for healing, ensure you:

- [ ] Implement all required interfaces: `keyvalue.Beginner`, `keyvalue.Batch`, and `io.Closer`
- [ ] Support both read-only and read-write transactions
- [ ] Handle binary keys and values correctly
- [ ] Maintain ACID transaction properties
- [ ] Support concurrent transactions with proper isolation
- [ ] Handle large values efficiently
- [ ] Store keys exactly as provided without modification
- [ ] Test with `heal_synth` approach
- [ ] Test with different URL construction methods

## See Also {#see-also}

- [Transaction Creation](./transactions.md)
- [Light Client Implementation](./light_client.md)
- [Implementation Guidelines](./implementation.md)
- [URL Construction Differences](../heal.md#url-construction-differences)
- [In-Memory Database Implementation](../heal_v2/heal_mem_db.md)
