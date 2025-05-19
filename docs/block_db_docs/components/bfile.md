# Buffered File (BFile)

The Buffered File (BFile) component provides efficient file I/O operations for BlockchainDB by implementing buffered reads and writes to reduce disk access.

## Overview

BFile is a core component that handles the low-level file operations for BlockchainDB. It implements buffered writing to improve performance by reducing the number of disk write operations.

## Structure

```go
type BFile struct {
    File     *os.File         // The file being buffered
    Filename string           // Fully qualified file name for the BFile
    Buffer   [BufferSize]byte // The current buffer under construction
    EOB      uint64           // End within the buffer
    EOD      uint64           // Current End of Data
}
```

## Key Features

- Buffered write operations to reduce disk I/O
- Efficient read operations with offset support
- Automatic buffer management
- Transparent file handling (opening/closing as needed)

## Basic Operations

### Creating a New BFile

```go
// Create a new BFile
bfile, err := blockchainDB.NewBFile("/path/to/file.dat")
```

### Opening an Existing BFile

```go
// Open an existing BFile
bfile, err := blockchainDB.OpenBFile("/path/to/file.dat")
```

### Writing Data

```go
// Write data to the BFile
updated, err := bfile.Write([]byte("data to write"))
```

### Reading Data

```go
// Read data from a specific offset
data := make([]byte, dataLength)
err := bfile.ReadAt(offset, data)
```

### Flushing the Buffer

```go
// Flush the buffer to disk
err := bfile.Flush()
```

### Closing the File

```go
// Close the BFile
err := bfile.Close()
```

## Implementation Details

- Uses a buffer size of 32KB (defined by `BufferSize = 1024 * 32`)
- Automatically flushes the buffer when it's full
- Manages the file handle, opening and closing as needed
- Tracks both the end of buffer (EOB) and end of data (EOD)

## Performance Considerations

- The buffering mechanism significantly reduces disk I/O operations for write-heavy workloads
- The `ReadAt` operation is smart about reading from the buffer vs. disk
- For large sequential writes, the buffering provides substantial performance benefits
