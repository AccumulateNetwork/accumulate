# Key File (KFile)

The Key File (KFile) component manages the storage and retrieval of keys in BlockchainDB, providing an efficient mechanism for key indexing and lookup.

## Overview

KFile is responsible for organizing and managing keys within BlockchainDB. It implements a specialized file structure that enables efficient key lookups and supports historical key tracking when needed.

## Immutability Behavior

KFile can operate in two distinct modes based on whether history is enabled or disabled:

### With History Enabled (Immutable Values)

When a KFile is created with `history=true` (as used in PermKV):

- **Values are immutable**: Once a key is associated with a value, that value cannot be changed
- **Overwrite attempts**: Attempting to overwrite a key with a different value will result in an error
- **Same-value writes**: Overwriting a key with the same value is allowed (effectively a no-op)
- **Use case**: Ideal for content-addressed storage where keys are derived from values (e.g., hash of value)
- **Example**: Blockchain transaction data, where transaction IDs (keys) are derived from the transaction content (values)

### With History Disabled (Mutable Values)

When a KFile is created with `history=false` (as used in DynaKV):

- **Values are mutable**: Keys can be freely associated with different values over time
- **Overwrite behavior**: Overwriting a key with a different value is allowed and expected
- **Use case**: Suitable for state storage where keys have an arbitrary relationship to values
- **Example**: Account balances or other state information that changes over time

## Structure

```go
type KFile struct {
    Header                               // kFile header (what is pushed to disk)
    Directory       string               // Directory of the BFile
    File            *BFile               // Key File
    History         *HistoryFile         // The History Database
    Cache           map[[32]byte]*DBBKey // Cache of DBBKey Offsets
    BlocksCached    int                  // Track blocks cached before rewritten
    HistoryMutex    sync.Mutex           // Allow the History to be merged in background
    KeyCnt          uint64               // Number of keys in the current KFile
    TotalCnt        uint64               // Total number of keys processed
    OffsetCnt       uint64               // Number of key sets in the kFile
    KeyLimit        uint64               // How many keys triggers to send keys to History
    MaxCachedBlocks int                  // Maximum number of keys cached before flushing to kfile
    HistoryOffsets  int                  // History offset cnt
}
```

## Key Features

- Efficient key organization and indexing
- In-memory caching for frequently accessed keys
- Support for historical key tracking
- Optimized for 32-byte keys common in blockchain applications
- Configurable immutability behavior based on history setting

## Basic Operations

### Creating a New KFile

```go
// Create a new KFile with immutable values (history enabled)
kfile, err := blockchainDB.NewKFile(
    true,           // Enable history - values will be immutable
    "/path/to/dir", // Directory path
    1024,           // Offset count
    10000,          // Key limit
    100,            // Max cached blocks
)

// Create a new KFile with mutable values (history disabled)
kfile, err := blockchainDB.NewKFile(
    false,          // Disable history - values will be mutable
    "/path/to/dir", // Directory path
    1024,           // Offset count
    10000,          // Key limit
    100,            // Max cached blocks
)
```

### Opening an Existing KFile

```go
// Open an existing KFile
kfile, err := blockchainDB.OpenKFile("/path/to/dir")
```

### Storing a Key

```go
// Store a key with its associated DBBKey
var key [32]byte
// Fill key with data...
dbbKey := new(blockchainDB.DBBKey)
// Set dbbKey properties...

// With history enabled, this will fail if the key already exists with a different value
// With history disabled, this will overwrite any existing value
err := kfile.Put(key, dbbKey)

// Handling immutability errors (when history is enabled)
if err != nil && strings.Contains(err.Error(), "cannot overwrite immutable value") {
    // Handle the immutability constraint violation
}
```

### Retrieving a Key

```go
// Retrieve a DBBKey using its key
dbbKey, err := kfile.Get(key)
```

### Pushing to History

```go
// Push current keys to history and reset
err := kfile.PushHistory()
```

## Implementation Details

- Uses a header structure to track key offsets
- Implements an in-memory cache to reduce disk reads
- Organizes keys into sections for efficient lookup
- Supports pushing keys to a history file when the key limit is reached
- Enforces immutability constraints when history is enabled
- Allows key overwriting when history is disabled

## Performance Considerations

- The caching mechanism significantly improves performance for frequently accessed keys
- The key organization strategy balances lookup speed with storage efficiency
- The history mechanism allows for efficient storage of historical key states
- The `MaxCachedBlocks` parameter can be tuned based on memory availability and access patterns

## Usage in KV2 System

The KFile component is used in two different ways in the KV2 system:

1. **PermKV**: Uses KFile with history enabled (immutable values)
   - Once a key is associated with a value, it cannot be changed
   - If a key in PermKV needs to be updated, it's moved to DynaKV
   - Ideal for blockchain transaction data and other immutable content

2. **DynaKV**: Uses KFile with history disabled (mutable values)
   - Keys can be freely associated with different values over time
   - Used for account balances, state information, and other mutable data

This two-layer design efficiently separates immutable data (content-addressed storage) from mutable data (state storage) in a blockchain-style database.
