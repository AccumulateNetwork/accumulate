# Accumulate Snapshot Format - Data Structures

This document provides comprehensive reference for all Go structs and data types used in the Accumulate snapshot format.

## Core Type Reference

This section provides detailed explanations of the fundamental types used throughout the snapshot system.

### Record Key Types

#### record.Key
A structured key used to identify records in the database. Defined in `pkg/types/record/key.go`.

```go
type Key struct {
    values []any   // The path components of the key
    hash   *KeyHash // Cached hash of the key
}
```

**Usage**: Record keys use a hierarchical path structure to organize data:
- Account records: `Account/{url}`
- Chain records: `Chain/{url}/{chain-type}/{element-type}/{element-id}`
- Transaction records: `Transaction/{txid}`
- Directory records: `Directory/{url-prefix}`
- BPT records: `BPT/{key-hash}`

#### record.KeyHash
A 32-byte hash of a record key used for indexing and lookups.

```go
type KeyHash [32]byte
```

### Merkle Proof Types

#### merkle.Receipt
A Merkle proof that proves a record's inclusion in the database. Defined in `pkg/database/merkle/types_gen.go`.

```go
type Receipt struct {
    // Contains a list of hashes that form a Merkle proof
    // Implementation details in merkle package
}
```

**Purpose**: Provides cryptographic proof that a record exists in the database without requiring the entire database.

### Protocol Types

#### NetworkAccountUpdate
Represents an update to a network account. Defined in `protocol/types_gen.go`.

```go
type NetworkAccountUpdate struct {
    fieldsSet []bool
    Name      string          // Name of the network account
    Body      TransactionBody // The update transaction body
    extraData []byte
}
```

#### ExecutorVersion
Represents the version of the executor. Defined in `protocol/version.go`.

```go
type ExecutorVersion uint64
```

#### PartitionExecutorVersion
Associates an executor version with a specific partition. Defined in `protocol/types_gen.go`.

```go
type PartitionExecutorVersion struct {
    fieldsSet []bool
    Partition string          // Name of the partition
    Version   ExecutorVersion // Version of the executor for this partition
    extraData []byte
}
```

#### AnchorBody
An interface for transaction bodies that contain partition anchors. Defined in `protocol/anchor.go`.

```go
type AnchorBody interface {
    TransactionBody
    GetPartitionAnchor() *PartitionAnchor
}
```

## Snapshot Structure Types

### Section Types

The `SectionType` constants define the different types of sections in a snapshot:

```go
type SectionType uint16

const (
    SectionTypeHeader      SectionType = 1  // Contains metadata about the snapshot
    SectionTypeSnapshot    SectionType = 6  // Contains nested snapshots
    SectionTypeRecords     SectionType = 7  // Contains records stored as (key, record) pairs
    SectionTypeRecordIndex SectionType = 8  // Indexes record keys, including offset and section number
    SectionTypeRawBPT      SectionType = 9  // Contains the BPT as raw (key hash, value) pairs (deprecated)
    SectionTypeConsensus   SectionType = 10 // Contains consensus parameters
    SectionTypeBPT         SectionType = 11 // Contains the Binary Patricia Tree as records
)
```

### Header Structure

#### snapshot.Header
Contains metadata about the snapshot. Defined in `pkg/database/snapshot/types_gen.go`.

```go
type Header struct {
    fieldsSet    []bool
    Version      uint64    // Snapshot format version (must be 2)
    RootHash     [32]byte  // BPT root hash
    SystemLedger *Account  // System ledger state
    extraData    []byte
}
```

**Key Fields**:
- **Version**: Must be 2 for current format
- **RootHash**: 32-byte hash representing the root of the Binary Patricia Tree
- **SystemLedger**: Contains the system ledger account state

### Record Structures

#### snapshot.RecordEntry
Represents a single record in the snapshot. Defined in `pkg/database/snapshot/types_gen.go`.

```go
type RecordEntry struct {
    fieldsSet []bool
    Key       *record.Key     // Structured key identifying the record
    Value     []byte          // Binary data associated with the key
    Receipt   *merkle.Receipt // Optional Merkle proof for the record
    extraData []byte
}
```

**Usage**:
- **Key**: Hierarchical identifier for the record
- **Value**: Serialized account, transaction, or other data
- **Receipt**: Cryptographic proof of record inclusion (optional)

#### snapshot.RecordIndexEntry
Provides fast lookup for records. Defined in `pkg/database/snapshot/types_gen.go`.

```go
type RecordIndexEntry struct {
    fieldsSet []bool
    KeyHash   [32]byte // SHA-256 hash of the record key
    Section   uint32   // Section number containing the record
    Offset    uint64   // Byte offset within the section
    extraData []byte
}
```

**Purpose**: Enables random access to records without scanning entire sections.

### BPT Structures

#### BPT Record Entry
Binary Patricia Tree entries are stored as regular records with special key formatting:

```go
// BPT records use keys like: BPT/{key-hash}
// Value contains the BPT node data
type BPTRecord struct {
    Key   *record.Key // BPT/{32-byte-hash}
    Value []byte      // BPT node value/hash
}
```

### Reader and Writer Structures

#### snapshot.Reader
Provides read access to snapshot files. Defined in `pkg/database/snapshot/reader.go`.

```go
type Reader struct {
    Header   *Header           // Parsed header information
    Sections []SectionInfo     // Information about all sections
    // Internal fields for file handling
}
```

**Key Methods**:
- `OpenRecords(sectionIndex int)` - Open a records section for reading
- `ReadRecord()` - Read the next record from a section
- `GetRecordIndex()` - Get the record index for fast lookups

#### snapshot.Writer
Provides write access for creating snapshot files. Defined in `pkg/database/snapshot/writer.go`.

```go
type Writer struct {
    // Internal fields for file handling and section management
}
```

**Key Methods**:
- `WriteHeader(header *Header)` - Write the snapshot header
- `NewSection(sectionType SectionType)` - Create a new section
- `WriteRecord(record *RecordEntry)` - Write a record to the current section
- `WriteRecordIndex(entries []RecordIndexEntry)` - Write the record index

## Binary Format Specifications

### Section Header Format
Each section begins with a 64-byte header:

```
+--------+--------+--------+--------+--------+--------+--------+--------+
| Type (2 bytes)  | Reserved (6)   | Size (8 bytes)                   |
+--------+--------+--------+--------+--------+--------+--------+--------+
| Next Section Offset (8 bytes)     | Additional Metadata (40 bytes)  |
+--------+--------+--------+--------+--------+--------+--------+--------+
```

- **Type**: 2-byte section type (big-endian)
- **Size**: 8-byte section size (big-endian)
- **Next Section Offset**: 8-byte offset to next section (big-endian)
- **Additional Metadata**: 40 bytes reserved/section-specific data

### Length-Prefixed Values
Many values use Protocol Buffers varint encoding for length:

```
+--------+--------+--------+--------+--------+--------+
| Length (varint) | Value (Length bytes)        ... |
+--------+--------+--------+--------+--------+--------+
```

## Account and Transaction Types

### Account Records
Account records contain serialized account data:

```go
// Example account record structure
type AccountRecord struct {
    URL     string  // Account URL
    Type    string  // Account type (e.g., "liteTokenAccount")
    Balance uint64  // Account balance
    // Additional fields based on account type
}
```

### Transaction Records
Transaction records contain complete transaction data:

```go
// Example transaction record structure  
type TransactionRecord struct {
    ID        [32]byte        // Transaction ID
    Body      TransactionBody // Transaction body
    Signature []Signature     // Transaction signatures
    Status    TransactionStatus // Transaction status
}
```

## Usage Guidelines

### Working with Structures
1. **Always use provided structs** - Don't handle binary data directly
2. **Check field validation** - Use `fieldsSet` arrays for optional fields
3. **Handle errors properly** - All marshal/unmarshal operations can fail
4. **Use appropriate methods** - Leverage reader/writer methods for file operations

### Memory Management
1. **Stream large files** - Don't load entire snapshots into memory
2. **Use indexes efficiently** - Leverage record indexes for random access
3. **Close resources** - Always close readers/writers when done
4. **Batch operations** - Process records in batches for better performance

### Type Safety
1. **Validate types** - Check section types before processing
2. **Handle versions** - Ensure snapshot version compatibility
3. **Verify hashes** - Use Merkle receipts for data integrity
4. **Check bounds** - Validate offsets and sizes before access

---

*This document provides the complete reference for all data structures used in the Accumulate snapshot format. For operational guidance, see [Snapshot Operations](snapshot-format-operations.md).*
