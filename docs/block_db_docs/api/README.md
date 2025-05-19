# BlockchainDB API Reference

This section provides detailed API documentation for the main components of BlockchainDB.

## Component APIs

- [KV (Key-Value Store)](#kv-key-value-store)
- [BFile (Buffered File)](#bfile-buffered-file)
- [KFile (Key File)](#kfile-key-file)
- [HistoryFile](#historyfile)
- [BloomFilter](#bloomfilter)

## KV (Key-Value Store)

### Types

```go
type KV struct {
    Directory   string
    vFile       *BFile
    kFile       *KFile
    HistoryFile *HistoryFile
    UseHistory  bool
}
```

### Functions

#### NewKV

```go
func NewKV(history bool, directory string, offsetsCnt, KeyLimit uint64, MaxCachedBlocks int) (kv *KV, err error)
```

Creates a new Key-Value store.

**Parameters:**
- `history` - Whether to enable history tracking
- `directory` - Directory path for the database
- `offsetsCnt` - Number of offset entries
- `KeyLimit` - Key limit before history push
- `MaxCachedBlocks` - Maximum number of blocks to cache

**Returns:**
- `kv` - The new KV instance
- `err` - Error, if any

#### OpenKV

```go
func OpenKV(directory string) (kv *KV, err error)
```

Opens an existing Key-Value store.

**Parameters:**
- `directory` - Directory path for the database

**Returns:**
- `kv` - The opened KV instance
- `err` - Error, if any

### Methods

#### Put

```go
func (k *KV) Put(key [32]byte, value []byte) (err error)
```

Stores a value with the given key.

**Parameters:**
- `key` - 32-byte key
- `value` - Value data to store

**Returns:**
- `err` - Error, if any

#### Get

```go
func (k *KV) Get(key [32]byte) (value []byte, err error)
```

Retrieves a value using its key.

**Parameters:**
- `key` - 32-byte key

**Returns:**
- `value` - Retrieved value
- `err` - Error, if any

#### Close

```go
func (k *KV) Close() (err error)
```

Closes the KV store.

**Returns:**
- `err` - Error, if any

#### Open

```go
func (k *KV) Open() (err error)
```

Opens the KV store.

**Returns:**
- `err` - Error, if any

#### Compress

```go
func (k *KV) Compress() (err error)
```

Re-writes the values file to remove trash values.

**Returns:**
- `err` - Error, if any

## BFile (Buffered File)

### Types

```go
type BFile struct {
    File     *os.File
    Filename string
    Buffer   [BufferSize]byte
    EOB      uint64
    EOD      uint64
}
```

### Functions

#### NewBFile

```go
func NewBFile(filename string) (file *BFile, err error)
```

Creates a new BFile.

**Parameters:**
- `filename` - Path to the file

**Returns:**
- `file` - The new BFile instance
- `err` - Error, if any

#### OpenBFile

```go
func OpenBFile(filename string) (bFile *BFile, err error)
```

Opens an existing BFile.

**Parameters:**
- `filename` - Path to the file

**Returns:**
- `bFile` - The opened BFile instance
- `err` - Error, if any

### Methods

#### Write

```go
func (b *BFile) Write(Data []byte) (update bool, err error)
```

Writes data to the file.

**Parameters:**
- `Data` - Data to write

**Returns:**
- `update` - Whether an actual file update occurred
- `err` - Error, if any

#### ReadAt

```go
func (b *BFile) ReadAt(offset uint64, data []byte) (err error)
```

Reads data from a specific offset.

**Parameters:**
- `offset` - Offset to read from
- `data` - Buffer to read into

**Returns:**
- `err` - Error, if any

#### Flush

```go
func (b *BFile) Flush() (err error)
```

Flushes the buffer to disk.

**Returns:**
- `err` - Error, if any

#### Close

```go
func (b *BFile) Close() (err error)
```

Closes the file.

**Returns:**
- `err` - Error, if any

#### Offset

```go
func (b *BFile) Offset() (offset uint64, err error)
```

Returns the current real size of the BFile.

**Returns:**
- `offset` - Current offset
- `err` - Error, if any

## KFile (Key File)

### Types

```go
type KFile struct {
    Header
    Directory       string
    File            *BFile
    History         *HistoryFile
    Cache           map[[32]byte]*DBBKey
    BlocksCached    int
    HistoryMutex    sync.Mutex
    KeyCnt          uint64
    TotalCnt        uint64
    OffsetCnt       uint64
    KeyLimit        uint64
    MaxCachedBlocks int
    HistoryOffsets  int
}
```

### Functions

#### NewKFile

```go
func NewKFile(history bool, directory string, offsetCnt uint64, keyLimit uint64, maxCachedBlocks int) (kFile *KFile, err error)
```

Creates a new KFile.

**Parameters:**
- `history` - Whether to enable history
- `directory` - Directory path
- `offsetCnt` - Number of offsets
- `keyLimit` - Key limit
- `maxCachedBlocks` - Maximum cached blocks

**Returns:**
- `kFile` - The new KFile instance
- `err` - Error, if any

#### OpenKFile

```go
func OpenKFile(directory string) (kFile *KFile, err error)
```

Opens an existing KFile.

**Parameters:**
- `directory` - Directory path

**Returns:**
- `kFile` - The opened KFile instance
- `err` - Error, if any

### Methods

#### Put

```go
func (k *KFile) Put(Key [32]byte, Value *DBBKey) (err error)
```

Stores a key with its associated DBBKey.

**Parameters:**
- `Key` - 32-byte key
- `Value` - DBBKey to store

**Returns:**
- `err` - Error, if any

#### Get

```go
func (k *KFile) Get(Key [32]byte) (dbBKey *DBBKey, err error)
```

Retrieves a DBBKey using its key.

**Parameters:**
- `Key` - 32-byte key

**Returns:**
- `dbBKey` - Retrieved DBBKey
- `err` - Error, if any

#### PushHistory

```go
func (k *KFile) PushHistory() (err error)
```

Pushes current keys to history and resets.

**Returns:**
- `err` - Error, if any

## HistoryFile

### Types

```go
type HistoryFile struct {
    Directory string
    File      *BFile
    Mutex     sync.Mutex
    OffsetCnt uint64
    Offsets   []uint64
}
```

### Functions

#### NewHistoryFile

```go
func NewHistoryFile(offsetCnt uint64, directory string) (historyFile *HistoryFile, err error)
```

Creates a new HistoryFile.

**Parameters:**
- `offsetCnt` - Number of offsets
- `directory` - Directory path

**Returns:**
- `historyFile` - The new HistoryFile instance
- `err` - Error, if any

### Methods

#### AddKeys

```go
func (h *HistoryFile) AddKeys(keyData []byte) (err error)
```

Adds keys to history.

**Parameters:**
- `keyData` - Key data to add

**Returns:**
- `err` - Error, if any

#### Get

```go
func (h *HistoryFile) Get(Key [32]byte) (dbBKey *DBBKey, err error)
```

Gets historical key data.

**Parameters:**
- `Key` - 32-byte key

**Returns:**
- `dbBKey` - Retrieved DBBKey
- `err` - Error, if any

## BloomFilter

### Types

```go
type BloomFilter struct {
    Filter []byte
    K      int
}
```

### Functions

#### NewBloomFilter

```go
func NewBloomFilter(size uint64, k int) *BloomFilter
```

Creates a new BloomFilter.

**Parameters:**
- `size` - Size of the filter
- `k` - Number of hash functions

**Returns:**
- BloomFilter instance

### Methods

#### Add

```go
func (b *BloomFilter) Add(data []byte)
```

Adds an element to the filter.

**Parameters:**
- `data` - Data to add

#### Test

```go
func (b *BloomFilter) Test(data []byte) bool
```

Tests if an element might be in the set.

**Parameters:**
- `data` - Data to test

**Returns:**
- Whether the element might be in the set
