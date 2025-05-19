# Key-Value Store (KV)

The Key-Value Store (KV) is the main interface for interacting with BlockchainDB. It provides a simple key-value database implementation optimized for blockchain applications.

## Overview

The KV component combines several underlying components to provide efficient storage and retrieval:
- Uses `BFile` for value storage
- Uses `KFile` for key management
- Optionally uses `HistoryFile` for maintaining historical data

## Structure

```go
type KV struct {
    Directory   string
    vFile       *BFile
    kFile       *KFile
    HistoryFile *HistoryFile
    UseHistory  bool
}
```

## Key Features

- Fixed-size 32-byte keys
- Variable-length values
- Optional history tracking
- Efficient storage with separate files for keys and values

## Basic Operations

### Creating a New KV Store

```go
// Create a new KV store with history enabled
kv, err := blockchainDB.NewKV(
    true,           // Enable history
    "/path/to/db",  // Directory path
    1024,           // Number of offset entries
    10000,          // Key limit before history push
    100,            // Max cached blocks
)
```

### Opening an Existing KV Store

```go
// Open an existing KV store
kv, err := blockchainDB.OpenKV("/path/to/db")
```

### Storing Data

```go
// Create a 32-byte key
var key [32]byte
// Fill key with data...

// Store a value
err := kv.Put(key, []byte("value data"))
```

### Retrieving Data

```go
// Retrieve a value using its key
value, err := kv.Get(key)
```

### Closing the Store

```go
// Close the KV store
err := kv.Close()
```

## Implementation Details

The KV store separates keys and values into different files:
- Keys are stored in a structured `KFile` that enables efficient lookups
- Values are stored in a `BFile` with their offsets referenced by the keys
- When history tracking is enabled, older versions of keys can be preserved

## Performance Considerations

- The separation of keys and values optimizes for blockchain use cases where keys are frequently accessed
- The buffered file implementation reduces disk I/O operations
- The `Compress()` method can be used to reclaim space from deleted or updated values
