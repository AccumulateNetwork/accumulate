# MemDB Implementation (Version 2)

This document details the MemDB implementation that replaces Bolt DB in Version 2 of the healing processes in the Accumulate network.

## Overview

The MemDB implementation provides the same interfaces as the Bolt DB implementation but stores all data in memory rather than on disk. This improves performance and simplifies deployment while maintaining compatibility with existing code.

The MemDB implementation is designed as a direct drop-in replacement for Bolt DB in the healing process. The only code change required is to replace the Bolt DB initialization with the MemDB initialization in the healing code. No other modifications to the healing code are necessary.

## Interface Implementation

The MemDB implements the following interfaces required by the healing code:

### 1. keyvalue.Store Interface

```go
// Store is a key-value store.
type Store interface {
    // Get loads a value.
    Get(*record.Key) ([]byte, error)

    // Put stores a value.
    Put(*record.Key, []byte) error

    // Delete deletes a key-value pair.
    Delete(*record.Key) error

    // ForEach iterates over each value.
    ForEach(func(*record.Key, []byte) error) error
}
```

The MemDB implementation provides thread-safe access to the key-value store with appropriate error handling for key not found scenarios. Note that the `MemDB` type itself does not directly implement this interface, but its `ChangeSet` implementation does, which is what the healing code interacts with.

### 2. keyvalue.Beginner Interface

```go
// A Beginner can begin key-value change sets.
type Beginner interface {
    // Begin begins a transaction or sub-transaction with a prefix applied to keys.
    Begin(prefix *record.Key, writable bool) ChangeSet
}
```

The `MemDB` type directly implements this interface, providing a thread-safe transaction mechanism that supports both read-only and read-write transactions.

### 3. keyvalue.ChangeSet Interface

```go
// ChangeSet is a key-value change set.
type ChangeSet interface {
    Store
    Beginner

    // Commit commits pending changes.
    Commit() error

    // Discard discards pending changes.
    Discard()
}
```

The `MemDBBatch` type implements this interface, providing ACID-compliant transaction operations with appropriate isolation levels. This is the primary interface that the healing code interacts with when performing database operations.

### 4. io.Closer Interface

```go
// The Closer interface is used to close the database
type Closer interface {
    Close() error
}
```

The `MemDB` type directly implements this interface, providing a Close method that cleans up resources and releases memory.

## Implementation Details

### Data Structure

The MemDB uses a map-based data structure with appropriate synchronization:

```go
type MemDB struct {
    data     map[[32]byte]valueEntry // Main data store using key hash for lookups
    rwMutex  sync.RWMutex            // Read-write mutex for thread safety
    txnMutex sync.Mutex              // Transaction mutex for serializing write transactions
    closed   bool                    // Flag indicating if the database is closed
}

// valueEntry stores both the value and the original key
type valueEntry struct {
    key   *record.Key
    value []byte
}
```

### Transaction Implementation

The MemDB implementation provides snapshot isolation for transactions:

```go
type MemDBBatch struct {
    db       *MemDB                  // Reference to the database
    writable bool                    // Flag indicating if the transaction is writable
    prefix   *record.Key             // Key prefix for this batch
    snapshot map[[32]byte]valueEntry // Snapshot of data at transaction start (for read-only)
    changes  map[[32]byte]entry      // Pending changes (for writable transactions)
    closed   bool                    // Flag indicating if the transaction is closed
    mu       sync.RWMutex            // Mutex for thread safety within the batch
}

type entry struct {
    key    *record.Key // The original key
    value  []byte      // The value (nil if this is a delete operation)
    delete bool        // True if this is a delete operation
}
```

### Thread Safety

The MemDB implementation uses a combination of read-write locks and transaction serialization to ensure thread safety:

- Read transactions can run concurrently with other read transactions
- Write transactions are serialized to maintain consistency
- The database uses a read-write mutex to protect the main data store
- Each transaction maintains its own snapshot and pending changes

### Key Handling

The MemDB implementation handles keys in a way that's compatible with the existing code:

1. Keys are stored as hashes (using `record.Key.Hash()`) for efficient lookups
2. The original key is preserved for ForEach operations
3. Key prefixes are applied when transactions are created
4. Nested transactions inherit and extend the parent's prefix
5. Nil keys are handled the same way as in Bolt to ensure compatibility

### Error Handling

The MemDB implementation provides error handling that is compatible with the Bolt implementation:

```go
var (
    ErrDatabaseClosed = errors.BadRequest.With("database is closed")
    ErrBatchClosed    = errors.BadRequest.With("batch is closed")
    ErrReadOnlyBatch  = errors.BadRequest.With("batch is read-only")
)
```

The implementation uses the following error types to ensure compatibility with the Bolt implementation:

1. **Key Not Found Errors**: Both implementations use `(*database.NotFoundError)(key)` when a key is not found. This ensures that code that checks for key not found errors using `errors.Is(err, (*database.NotFoundError)(nil))` or similar will work correctly with both implementations.

2. **Database Closed Errors**: The MemDB implementation returns `ErrDatabaseClosed` when an operation is attempted on a closed database, while Bolt returns its own internal error. Both are wrapped with appropriate error types that can be checked using `errors.Is`.

3. **Transaction Errors**: MemDB returns `ErrBatchClosed` when an operation is attempted on a closed transaction and `ErrReadOnlyBatch` when a write operation is attempted on a read-only transaction. These are similar to the errors returned by Bolt.

4. **Nil Key Handling**: MemDB allows nil keys to be used in operations, matching Bolt's behavior. This ensures that code that works with Bolt will work correctly with MemDB without modification.

5. **Internal Errors**: Both implementations wrap internal errors with `errors.UnknownError` or similar to provide consistent error handling.

The error handling in MemDB ensures that code that works with Bolt will work correctly with MemDB without modification. For example, code that checks for key not found errors will work the same way with both implementations:

```go
value, err := db.Get(key)
if errors.Is(err, (*database.NotFoundError)(nil)) {
    // Handle key not found
    return nil, errors.NotFound.WithFormat("account %v not found", key)
}
```

## Transaction Handling

The MemDB implementation closely mirrors Bolt's transaction handling to ensure compatibility:

### Transaction Isolation

Both Bolt and MemDB provide snapshot isolation for transactions:

1. **Read-Only Transactions**: Both implementations create a snapshot of the database at the time the transaction begins, ensuring consistent reads even if the database is modified by other transactions.

2. **Read-Write Transactions**: Both implementations track changes in a separate map and only apply them to the database when the transaction is committed.

3. **Nested Transactions**: Both implementations support nested transactions with prefix inheritance, allowing for hierarchical data organization.

### Key Prefixing

Both implementations handle key prefixing in the same way:

```go
// Apply the prefix to the key
if b.prefix != nil && key != nil {
    key = b.prefix.AppendKey(key)
}
```

This ensures that keys are properly namespaced within transactions, which is essential for the healing process that uses prefixed keys to organize data. The check for nil keys ensures compatibility with Bolt's behavior.

### Value Copying

Both implementations make defensive copies of values to ensure transaction isolation:

```go
// Return a copy of the value to ensure isolation
result := make([]byte, len(entry.value))
copy(result, entry.value)
return result, nil
```

This prevents modifications to returned values from affecting the database state, maintaining data integrity.

### ForEach Implementation

The ForEach implementation in MemDB is designed to match Bolt's behavior:

1. **Transaction Visibility**: Both implementations ensure that ForEach sees the transaction's pending changes.

2. **Consistent View**: Both implementations provide a consistent view of the database, including pending changes and the database state at the time the transaction began.

3. **Error Handling**: Both implementations propagate errors from the callback function and stop iteration when an error occurs.

The key difference is in the implementation details:

- Bolt uses a cursor to iterate over buckets and keys
- MemDB uses map iteration with a processed map to track which keys have been processed

Despite these implementation differences, the behavior is the same from the caller's perspective.

## Integration with Healing Process

The MemDB is designed to be a direct replacement for Bolt DB in the healing process. The current healing process initializes Bolt DB as follows:

```go
// Current Bolt DB initialization in heal_common.go
db, err := bolt.Open(lightDb, bolt.WithPlainKeys)
if err != nil {
    return errors.UnknownError.WithFormat("open database: %w", err)
}
h.db = db

// Initialize the Light client
h.light = light.New(db, nil, nil)
```

To use MemDB instead, the initialization would be changed to:

```go
// Replacement with MemDB
db := memdb.New()
h.db = db

// Initialize the Light client with the MemDB
h.light = light.New(db, nil, nil)
```

This simple change is all that's required to switch from Bolt DB to MemDB in the healing process. The Light client and other components that interact with the database will continue to work as expected because MemDB implements the same interfaces as Bolt DB.

### Key Differences from Bolt DB

While MemDB implements the same interfaces as Bolt DB, there are some key differences in the implementation:

1. **In-Memory Storage**: MemDB stores all data in memory, while Bolt DB stores data on disk. This means that MemDB is faster but does not persist data across restarts.

2. **No Plain Keys Option**: Bolt DB supports a `WithPlainKeys` option that affects how keys are stored. MemDB does not need this option as its key handling is transparent to the healing process.

3. **Simplified Implementation**: MemDB is specifically designed for the healing process and includes only the functionality needed for this use case, making it more efficient and easier to maintain.

4. **No File I/O**: MemDB does not require file I/O operations, which simplifies the implementation and improves performance.

## URL Construction Handling

The MemDB implementation maintains compatibility with both URL construction methods used in the codebase:

- `sequence.go` uses raw partition URLs (e.g., `acc://bvn-Apollo.acme`)
- `heal_anchor.go` appends the partition ID to the anchor pool URL (e.g., `acc://dn.acme/anchors/Apollo`)

The database stores keys exactly as provided without modification or interpretation, ensuring compatibility with both approaches. This is achieved by:

1. Using the raw key hash for storage
2. Preserving the original key object for retrieval
3. Not performing any URL parsing or manipulation within the database layer

## Performance Considerations

The MemDB implementation offers several performance advantages:

1. **Reduced I/O Overhead**: All operations are performed in memory, eliminating disk I/O overhead
2. **Optimized Data Structures**: The implementation uses data structures optimized for in-memory access patterns
3. **Reduced Lock Contention**: The implementation uses fine-grained locking to reduce contention
4. **No Serialization Overhead**: The implementation stores keys and values directly, eliminating serialization overhead

## Memory Considerations

Since the MemDB stores all data in memory, it's essential to consider the memory usage:

1. **Memory Footprint**: The memory footprint is proportional to the amount of data stored
2. **Garbage Collection**: The implementation is designed to minimize garbage collection pressure
3. **Memory Leaks**: The implementation ensures proper cleanup when the database is closed

For the healing process, the memory usage is expected to be minimal as the database is only used to store temporary data during the healing process.

## Testing

The MemDB implementation includes comprehensive tests to ensure correctness:

1. **Basic Operations**: Tests for Get, Put, Delete operations
2. **Transaction Semantics**: Tests for Begin, Commit, Discard operations
3. **Concurrency**: Tests for thread safety and transaction isolation
4. **Error Handling**: Tests for error conditions and recovery
5. **Integration**: Tests for integration with the Light client
