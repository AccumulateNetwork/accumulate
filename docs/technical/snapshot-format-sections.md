# Accumulate Snapshot Format - Sections

This document provides comprehensive information about the different section types in Accumulate snapshot files, their organization, encoding, and usage patterns.

## Section Type Overview

Accumulate snapshot files are organized into sections, each with a specific purpose and data format. Each section has a type identifier, size information, and structured content.

### Section Type Constants

```go
const (
    SectionTypeHeader         SectionType = 1  // Contains metadata about the snapshot
    SectionTypeAccountsV1     SectionType = 2  // Contains account data in v1 format (deprecated)
    SectionTypeTransactionsV1 SectionType = 3  // Contains transaction data in v1 format (deprecated)
    SectionTypeSignaturesV1   SectionType = 4  // Contains signature data in v1 format (deprecated)
    SectionTypeGzTransactionsV1 SectionType = 5  // Contains gzipped transaction data (can appear in v2 snapshots)
    SectionTypeSnapshot       SectionType = 6  // Contains nested snapshots
    SectionTypeRecords        SectionType = 7  // Contains records stored as (key, record) pairs
    SectionTypeRecordIndex    SectionType = 8  // Indexes record keys, including offset and section number
    SectionTypeRawBPT         SectionType = 9  // Contains the BPT as raw (key hash, value) pairs (deprecated)
    SectionTypeConsensus      SectionType = 10 // Contains consensus parameters
    SectionTypeBPT            SectionType = 11 // Contains the Binary Patricia Tree as records
)
```

## Section Header Format

Every section begins with a 64-byte header containing metadata about the section:

```
+--------+--------+--------+--------+--------+--------+--------+--------+
| Type (2 bytes)  | Reserved (6)   | Size (8 bytes)                   |
+--------+--------+--------+--------+--------+--------+--------+--------+
| Next Section Offset (8 bytes)     | Additional Metadata (40 bytes)  |
+--------+--------+--------+--------+--------+--------+--------+--------+
```

### Header Fields
- **Type**: 2-byte section type identifier (big-endian)
- **Reserved**: 6 bytes reserved for future use
- **Size**: 8-byte section content size in bytes (big-endian)
- **Next Section Offset**: 8-byte offset to the next section (big-endian)
- **Additional Metadata**: 40 bytes of section-specific metadata

### SectionHeader Struct

```go
type SectionHeader struct {
    Type       SectionType
    Size       int64
    Next       int64
    Compressed bool
    Encrypted  bool
    Metadata   []byte
}
```

## Active Section Types (Version 2)

### 1. Header Section (Type 1)

**Purpose**: Contains snapshot metadata and system information.

**Structure**: Uses `snapshot.Header` struct
```go
type Header struct {
    Version      uint64                // Snapshot format version (must be 2)
    RootHash     [32]byte              // BPT root hash
    SystemLedger *protocol.SystemLedger // System ledger state
}
```

**Location**: Always the first section in every snapshot file.

**Content**:
- Snapshot format version (must be 2)
- 32-byte root hash of the Binary Patricia Tree
- System ledger state including URL, index, timestamp, and pending updates

**Usage**: Read first to validate snapshot compatibility and get root hash for verification.

### 2. Records Section (Type 7)

**Purpose**: Contains the primary data records as key-value pairs.

**Structure**: Uses `snapshot.RecordEntry` struct
```go
type RecordEntry struct {
    Key     *record.Key      // Hierarchical key identifying the record
    Value   []byte           // Binary data associated with the key
    Receipt *merkle.Receipt  // Optional Merkle proof for the record
}
```

**Content Types**:
- **Account Records**: Account state data
- **Chain Records**: Chain state and entries
- **Transaction Records**: Complete transaction data
- **Message Records**: Message data and routing information
- **Directory Records**: URL directory information
- **System Records**: Network configuration and parameters

**Key Formats**:
- Account: `Account/{url}`
- Chain: `Chain/{url}/{chain-type}/{element-type}/{element-id}`
- Transaction: `Transaction/{hash}`
- Message: `Message/{hash}/Main`
- Directory: `Directory/{url-prefix}`
- System: `System/{key}`

**Multiple Sections**: A single snapshot can contain multiple Records sections for functional separation.

### 3. Record Index Section (Type 8)

**Purpose**: Provides fast random access to records without scanning entire sections.

**Structure**: Uses `snapshot.RecordIndexEntry` struct
```go
type RecordIndexEntry struct {
    KeyHash [32]byte  // SHA-256 hash of the record key
    Section uint64    // Section number containing the record
    Offset  uint64    // Byte offset within the section
}
```

**Organization**:
- Entries are sorted by key hash in **descending order**
- Fixed-width entries for efficient binary search
- Points to specific Records sections and byte offsets

**Usage**: Binary search on key hash to locate records quickly.

### 4. BPT Section (Type 11)

**Purpose**: Contains Binary Patricia Tree nodes for database structure and verification.

**Structure**: Uses `snapshot.RecordEntry` struct with BPT-specific keys
```go
// BPT records use keys like: BPT/{key-hash}
type BPTRecord struct {
    Key   *record.Key // BPT/{32-byte-hash}
    Value []byte      // BPT node value/hash
}
```

**Content**:
- BPT node data including hashes and tree structure
- Enables Merkle proof verification
- Supports database integrity checking

**Key Format**: `BPT/{32-byte-key-hash}`

### 5. Nested Snapshot Section (Type 6)

**Purpose**: Contains complete nested snapshots for multi-network scenarios.

**Structure**: Complete snapshot with its own header and sections

**Usage**:
- Multi-partition network snapshots
- Hierarchical network organization
- Snapshot composition and merging

### 6. Consensus Section (Type 10)

**Purpose**: Contains consensus parameters and network configuration.

**Structure**: Implementation-specific format

**Content**:
- Consensus algorithm parameters
- Network configuration settings
- Validator information
- Protocol version information

## Deprecated Section Types (Version 1)

### AccountsV1 Section (Type 2) - Deprecated
- **Purpose**: Account data in v1 format
- **Replacement**: Use Records sections with Account keys
- **Structure**: `snapshot.Account` structs

### TransactionsV1 Section (Type 3) - Deprecated
- **Purpose**: Transaction data in v1 format
- **Replacement**: Use Records sections with Transaction keys
- **Structure**: `snapshot.Transaction` structs in `txnSection` wrapper

### SignaturesV1 Section (Type 4) - Deprecated
- **Purpose**: Signature data in v1 format
- **Replacement**: Include signatures in transaction records
- **Structure**: `snapshot.Signature` structs in `sigSection` wrapper

### GzTransactionsV1 Section (Type 5) - Legacy Format
- **Purpose**: Gzipped transaction data in v1 format
- **Usage**: Can appear in v2 snapshots for performance optimization when transaction data is large
- **Replacement**: Use Records sections with Transaction keys for new implementations
- **Structure**: Compressed `snapshot.Transaction` structs
- **Note**: Despite the "V1" name, this section type is still used in some v2 snapshots when compression is beneficial

### RawBPT Section (Type 9) - Deprecated
- **Purpose**: BPT data as raw key-value pairs
- **Replacement**: Use BPT sections with RecordEntry format
- **Structure**: Raw 64-byte entries (32-byte key + 32-byte value)

## Record Key Examples

### Account Records
```
Key: Account/acc://example.acme
Binary:
02                                    # 2 elements
07 4163636F756E74                     # "Account"
11 6163633A2F2F6578616D706C652E61636D65 # "acc://example.acme"
```

### Chain Records
```
Key: Chain/acc://dn.acme/ledger/RootChain/Index/Head
Binary:
05                                    # 5 elements
05 436861696E                         # "Chain"
14 6163633A2F2F646E2E61636D652F6C6564676572 # "acc://dn.acme/ledger"
09 526F6F74436861696E                 # "RootChain"
05 496E646578                         # "Index"
04 48656164                           # "Head"
```

### Transaction Records
```
Key: Transaction/0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
Binary:
02                                    # 2 elements
0B 5472616E73616374696F6E             # "Transaction"
20 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef # Hash (32 bytes)
```

### BPT Records
```
Key: BPT/99ba5065aa3b13879dd877e902d91a0d5ce5be4036a50af98169ba855b292db0
Binary:
02                                    # 2 elements
03 425054                             # "BPT"
20 99ba5065aa3b13879dd877e902d91a0d5ce5be4036a50af98169ba855b292db0 # Hash (32 bytes)
```

## Section Organization Patterns

### Standard Snapshot Structure
1. **Header Section** (Type 1) - Always first
2. **Records Sections** (Type 7) - One or more sections containing data
3. **Record Index Section** (Type 8) - Index for efficient access
4. **BPT Section** (Type 11) - Binary Patricia Tree data
5. **Consensus Section** (Type 10) - Network parameters

### Multi-Network Snapshots
1. **Header Section** (Type 1) - Main snapshot header
2. **Nested Snapshot Sections** (Type 6) - Each containing complete sub-snapshots
3. **Record Index Section** (Type 8) - Combined index for all networks

### Streaming-Optimized Snapshots
1. **Header Section** (Type 1) - Metadata
2. **Multiple Records Sections** (Type 7) - Data split by type or size
3. **Record Index Section** (Type 8) - Index at end for random access

## Processing Guidelines

### Reading Sections
1. **Always read Header first** - Validate version and get root hash
2. **Use section headers** - Check type and size before processing
3. **Handle unknown types** - Skip sections with unknown types gracefully
4. **Validate offsets** - Ensure section boundaries are respected

### Writing Sections
1. **Start with Header** - Always write header section first
2. **Group related data** - Use multiple Records sections for organization
3. **Build indexes last** - Write Record Index after all Records sections
4. **Include BPT data** - Write BPT section for verification support

### Memory Management
1. **Stream large sections** - Don't load entire Records sections into memory
2. **Use indexes efficiently** - Leverage Record Index for random access
3. **Process incrementally** - Handle records one at a time when possible
4. **Cache strategically** - Cache frequently accessed index data

### Error Handling
1. **Validate section headers** - Check type, size, and offset values
2. **Handle corruption gracefully** - Skip corrupted sections when possible
3. **Verify checksums** - Use Merkle receipts for data integrity
4. **Log processing errors** - Provide detailed error information

---

*This document provides complete information about snapshot sections. For operational guidance, see [Snapshot Operations](snapshot-format-operations.md).*
