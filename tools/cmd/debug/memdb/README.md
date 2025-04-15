# MemDB: In-Memory Database Implementation

MemDB is an in-memory key-value store designed as a drop-in replacement for Bolt. It provides a lightweight, thread-safe implementation for the healing process in Accumulate.

## Core Components

### 1. Basic Key-Value Operations

- **Data Structure**: Uses `map[[32]byte]valueEntry` where `valueEntry` stores both the original key and value
- **Operations**: Implements standard Get, Put, Delete operations
- **Key Handling**: Uses `record.Key` for key representation
- **Bolt Compatibility**: Matches Bolt's behavior for all operations, including nil key handling

### 2. Batch Operations

- **Data Structures**: Implements Batch and batchEntry for transaction support
- **Operations**: Begin, Commit, Discard for transaction management
- **Nested Batches**: Supports nested transactions with proper parent-child relationships
- **Isolation**: Provides simplified transaction semantics adequate for the healing process

### 3. Error Handling

- **Core Errors**:
  - `ErrDatabaseClosed`: Returned when operations are attempted on a closed database
  - `ErrBatchClosed`: Returned when operations are attempted on a closed batch
  - `ErrReadOnlyBatch`: Returned when write operations are attempted on a read-only batch
- **Not Found Errors**: Uses `database.NotFoundError` for key not found situations
- **Bolt Compatibility**: Error behavior matches Bolt for compatibility

### 4. Concurrency Considerations

- **Thread Safety**: Implements basic thread safety with `sync.RWMutex`
- **Transaction Serialization**: Uses a separate `txnMutex` for serializing write transactions
- **Defensive Copying**: Minimal defensive copying, assumes mostly immutable data
- **Performance**: Optimized for the specific needs of the healing process

### 5. Initialization

- **Creation**: Simple `New()` function to create a new MemDB instance
- **Cleanup**: Implements `Close()` to properly clean up resources

## Implementation Notes

1. **Bolt Compatibility**:
   - MemDB is designed as a drop-in replacement for Bolt
   - All behavior, including error handling and edge cases, matches Bolt
   - No additional type safety or checks that would make MemDB throw errors when Bolt would not

2. **Simplifications**:
   - Simplified for the specific needs of the healing process
   - In-memory only, no persistence
   - Assumes mostly immutable data

3. **Usage**:
   - Primarily intended for use in the healing process
   - Not recommended for general-purpose database needs requiring persistence

## Example Usage

```go
// Create a new database
db := memdb.New()
defer db.Close()

// Start a transaction
batch := db.Begin(nil, true)

// Perform operations
batch.Put(record.NewKey("key1"), []byte("value1"))
value, err := batch.Get(record.NewKey("key1"))

// Commit the transaction
batch.Commit()
```

## Error Handling Examples

```go
// Handle not found errors
value, err := batch.Get(record.NewKey("nonexistent"))
if _, ok := err.(*database.NotFoundError); ok {
    // Key not found
}

// Handle closed database
db.Close()
_, err = batch.Get(record.NewKey("key"))
if err == memdb.ErrBatchClosed {
    // Database is closed
}

// Handle read-only batch
batch := db.Begin(nil, false)
err := batch.Put(record.NewKey("key"), []byte("value"))
if err == memdb.ErrReadOnlyBatch {
    // Cannot write to read-only batch
}
```
