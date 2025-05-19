# History File

The History File component in BlockchainDB provides a mechanism for tracking historical versions of keys and their associated values, which is essential for blockchain applications that require historical state access.

## Overview

The History File maintains a record of previous key states, allowing applications to access not just the current state but also historical states. This is particularly important for blockchain applications where historical state verification is often required.

## Structure

```go
type HistoryFile struct {
    Directory string      // Directory for the history file
    File      *BFile      // The underlying BFile
    Mutex     sync.Mutex  // Mutex for thread safety
    OffsetCnt uint64      // Number of offsets in the history file
    Offsets   []uint64    // Offsets into the history file
}
```

## Key Features

- Maintains historical versions of keys
- Thread-safe operations
- Efficient storage and retrieval of historical data
- Integrates with KFile for seamless history tracking

## Basic Operations

### Creating a New History File

```go
// Create a new History File
historyFile, err := blockchainDB.NewHistoryFile(
    1024,           // Offset count
    "/path/to/dir", // Directory path
)
```

### Adding Keys to History

```go
// Add keys to history
err := historyFile.AddKeys(keyData)
```

### Retrieving Historical Key Data

```go
// Get historical key data
var key [32]byte
// Fill key with data...
dbbKey, err := historyFile.Get(key)
```

## Implementation Details

- Uses a similar offset-based organization as KFile
- Stores keys in a format that enables efficient lookup
- Thread-safe operations for concurrent access
- Designed to work in conjunction with KFile's history pushing mechanism

## Performance Considerations

- The history mechanism adds storage overhead but provides valuable historical state access
- The offset-based organization balances lookup speed with storage efficiency
- Thread safety ensures correct behavior in concurrent environments
- History can be disabled if not needed, reducing storage requirements
