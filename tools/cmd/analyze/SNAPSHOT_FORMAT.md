# Accumulate Snapshot Format (Version 2)

This document provides a comprehensive specification of the Accumulate version 2 snapshot format, structure, and implementation details to guide the development of snapshot processing tools.

## Snapshot File Structure

A version 2 snapshot file consists of multiple sections, each with a specific type and purpose. The file format uses a segmented approach where each section has a type identifier, size, and content.

### Section Types in Version 2 Snapshots

| Section Type | Value | Description |
|-------------|-------|-------------|
| `SectionTypeHeader` | 1 | Contains metadata about the snapshot |
| `SectionTypeSnapshot` | 6 | Contains nested snapshots |
| `SectionTypeRecords` | 7 | Contains records stored as (key, record) pairs |
| `SectionTypeRecordIndex` | 8 | Indexes record keys, including offset and section number |
| `SectionTypeRawBPT` | 9 | Contains the BPT as raw (key hash, value) pairs (deprecated in favor of SectionTypeBPT) |
| `SectionTypeConsensus` | 10 | Contains consensus parameters |
| `SectionTypeBPT` | 11 | Contains the Binary Patricia Tree as records |

### File Format Structure

A version 2 snapshot file follows this structure:

1. **Header Section** (Required):
   - Always the first section in a snapshot file
   - Contains version information (must be 2), root hash, and system ledger information
   - Encoded as a length-prefixed binary value
   - Defined by the `Header` struct in `pkg/database/snapshot/types_gen.go`

2. **Record Sections**:
   - Contains the actual data records as key-value pairs
   - Each record is stored as a `RecordEntry` with key, value, and optional receipt
   - Multiple record sections (type 7) can exist in a single snapshot file
   - These multiple sections are created by design for functional separation, not due to size limits

3. **Record Index Sections**:
   - Contains fixed-width entries that index records by key hash
   - Each entry includes the key hash, section number, and byte offset
   - Enables efficient random access to records without loading the entire snapshot
   - Entries are sorted by key hash in descending order

4. **BPT Sections**:
   - Contains Binary Patricia Tree nodes as records
   - Each record includes the key path in the BPT and the node value/hash
   - Used for database structure and verification

5. **Consensus Parameter Sections**:
   - Contains consensus parameters in an implementation-specific format

## Binary Format Specification

This section provides the detailed binary format specifications needed to implement snapshot file reading and writing.

### Section Header Binary Format

Each section in a snapshot file begins with a 64-byte header with the following binary structure:

```
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
| Type (2 bytes)  | Reserved (6)   | Size (8 bytes)                                           |
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
| Next Section Offset (8 bytes)                      | Additional Metadata (40 bytes)           |
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
```

- **Type**: 2-byte unsigned integer in **big-endian** format representing the section type
- **Size**: 8-byte unsigned integer in **big-endian** format representing the section size in bytes
- **Next Section Offset**: 8-byte unsigned integer in **big-endian** format pointing to the next section
- **Additional Metadata**: 40 bytes of additional header information (reserved or used for specific section types)

The section content immediately follows the 64-byte header and is exactly `Size` bytes long.

> **IMPORTANT**: Note that the format uses big-endian encoding for all multi-byte values, not little-endian as previously documented.

### Length-Prefixed Values

Many values in the snapshot format are length-prefixed. The format is:

```
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
| Length (varint)           | Value (Length bytes)                            ... |
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
```

- **Length**: Variable-length integer (varint) encoded using Protocol Buffers encoding
- **Value**: Binary data of exactly `Length` bytes

### Header Section Binary Format

The Header section has the following binary layout:

```
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
| Length (varint)           | Header Data (Length bytes)                     ... |
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
```

The Header Data is a serialized `Header` struct with the following fields:

```
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
| Version (varint)          | Root Hash Length (varint) | Root Hash (32 bytes)   ... |
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
| System Ledger Flag (1 byte) | System Ledger Data (if flag=1)               ... |
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
```

- **Version**: Must be 2 for version 2 snapshots
- **Root Hash**: 32-byte SHA-256 hash
- **System Ledger Flag**: 1 if system ledger data is present, 0 if not
- **System Ledger Data**: Contains height (varint) and timestamp (varint) if present

### Record Entry Binary Format

Each record in a Records section has the following binary format:

```
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
| Record Length (varint)    | Key Length (varint)      | Key Data (Key Length bytes) ... |
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
| Value Length (varint)     | Value Data (Value Length bytes)               ... |
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
| Receipt Flag (1 byte)     | Receipt Data (if flag=1)                     ... |
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
```

### Key Binary Format

A hierarchical key is encoded as follows:

```
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
| Number of Elements (varint) | Element 1 Length (varint) | Element 1 Data      ... |
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
| Element 2 Length (varint)  | Element 2 Data           ... | ... more elements ... |
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
```

### Record Index Entry Binary Format

Each entry in a Record Index section has a fixed size of 44 bytes:

```
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
| Key Hash (32 bytes)                                                           ... |
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
| Section Number (4 bytes)  | Offset (8 bytes)                                    |
+--------+--------+--------+--------+--------+--------+--------+--------+--------+--------+
```

- **Key Hash**: 32-byte SHA-256 hash of the record key
- **Section Number**: 4-byte unsigned integer in little-endian format
- **Offset**: 8-byte unsigned integer in little-endian format

### Example: Complete Binary Representation

Here's a hexadecimal representation of a minimal snapshot file with one header section and one record section containing a single record:

```
# Section 1: Header Section (64-byte header)
00 01                       # Section type = 1 (Header) in big-endian
00 00 00 00 00 00           # Reserved (6 bytes)
00 00 00 00 00 00 00 10     # Section size = 16 bytes in big-endian
00 00 00 00 00 00 00 40     # Next section offset = 64 (after this header)
00 ... 00                   # Additional metadata (40 bytes)

# Header content (16 bytes)
00 00 00 02                 # Version = 2 in big-endian
01 02 03 ... 1F 20          # Root hash (32 bytes)

# Section 2: Records Section (64-byte header)
00 07                       # Section type = 7 (Records) in big-endian
00 00 00 00 00 00           # Reserved (6 bytes)
00 00 00 00 00 00 00 30     # Section size = 48 bytes in big-endian
00 00 00 00 00 00 00 00     # Next section offset = 0 (end of file)
00 ... 00                   # Additional metadata (40 bytes)

# Records section content
# Record 1 (48 bytes)
08 41 63 63 6F 75 6E 74     # Key part 1: "Account" (8 bytes)
...
```

> Note: The actual binary representation uses 64-byte headers with big-endian encoding for multi-byte values.

## Record Structure

Records in version 2 snapshots are stored as `RecordEntry` structures:

```go
type RecordEntry struct {
	Key     *record.Key     // Hierarchical key path
	Value   []byte          // Binary encoded data
	Receipt *merkle.Receipt // Optional Merkle proof
}
```

### Key Format

Keys are hierarchical paths represented by the `record.Key` type. Each key consists of a series of components that define the record type and location. The first component typically indicates the record category.

#### Binary Encoding of Keys

A key is encoded as a sequence of length-prefixed strings. The encoding follows this format:

1. Number of elements (varint)
2. For each element:
   - Element length (varint)
   - Element data (UTF-8 encoded string)

**Example**: The key `Account/acc://example.acme/main` is encoded as:

```
03                           # 3 elements (varint)
07 4163636F756E74            # Length=7, "Account"
13 6163633A2F2F6578616D706C652E61636D65  # Length=19, "acc://example.acme"
04 6D61696E                  # Length=4, "main"
```

### Value Binary Format

The value field contains binary-encoded data specific to each record type. The encoding depends on the record type but generally follows these rules:

1. Values are serialized using Protocol Buffer encoding or custom binary formats
2. Complex structures are length-prefixed
3. All integers are encoded in little-endian format unless specified otherwise

**Example**: A simple account value might be encoded as:

```
01                           # Account type code (varint)
08 0000000000000064          # Balance=100 (length-prefixed 8-byte integer)
04 00000001                  # Nonce=1 (length-prefixed 4-byte integer)
```

### Record Types with Concrete Examples

1. **Account Records**:
   - **Format**: `Account/{url}/{chain-type}`
   - **Examples**:
     ```
     Account/acc://dn.acme/ledger/Main
     Account/acc://dn.acme/ledger/RootChain
     ```
   - **Value Content**: Serialized account data including type, balance, nonce, etc.
   - **Location**: Written in `SectionTypeRecords` sections
   - **Binary Key Example**:
     ```
     # Key: Account/acc://dn.acme/ledger/Main
     03                           # 3 elements
     07 4163636F756E74            # "Account"
     14 6163633A2F2F646E2E61636D652F6C6564676572  # "acc://dn.acme/ledger"
     04 4D61696E                  # "Main"
     ```
   - **Binary Value Example**:
     ```
     # Account type (TokenAccount)
     01                           # Account type code
     # Balance field
     08 00E87648170000000000      # Balance=100000000000 (8-byte integer)
     # Nonce field
     04 01000000                  # Nonce=1 (4-byte integer)
     # Additional fields follow...
     ```

2. **Chain Records**:
   - **Format**: `Chain/{url}/{chain-type}/{element-type}/{element-id}`
   - **Examples**:
     ```
     Chain/acc://dn.acme/ledger/Main
     Chain/acc://dn.acme/ledger/RootChain/Index/Head
     Chain/acc://dn.acme/ledger/RootChain/Index/Element/176685
     ```
   - **Value Content**: Chain state data including head, height, and entries
   - **Location**: Written in `SectionTypeRecords` sections
   - **Binary Key Example**:
     ```
     # Key: Chain/acc://dn.acme/ledger/RootChain/Index/Head
     05                           # 5 elements
     05 436861696E                # "Chain"
     14 6163633A2F2F646E2E61636D652F6C6564676572  # "acc://dn.acme/ledger"
     09 526F6F74436861696E        # "RootChain"
     05 496E646578                # "Index"
     04 48656164                  # "Head"
     ```
   - **Binary Value Example**:
     ```
     # Chain index head
     08 CD020000 00000000         # Height=717 (8-byte integer)
     20 7B5AC4...                 # Hash (32 bytes)
     # Additional fields follow...
     ```

3. **Transaction Records**:
   - **Format**: `Transaction/{hash}`
   - **Examples**:
     ```
     Transaction/0123456789abcdef0123456789abcdef
     ```
   - **Value Content**: Transaction data including type, body, and header
   - **Location**: Written in `SectionTypeRecords` sections
   - **Binary Key Example**:
     ```
     # Key: Transaction/0123456789abcdef0123456789abcdef
     02                           # 2 elements
     0B 5472616E73616374696F6E    # "Transaction"
     20 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef  # Hash (32 bytes hex)
     ```
   - **Binary Value Example**:
     ```
     # Transaction type
     01                           # Transaction type code
     # Header
     20 7B5AC4...                 # Principal hash (32 bytes)
     08 00E87648170000000000      # Fee (8-byte integer)
     # Body (length-prefixed)
     A4 ...                       # Transaction body data
     ```

4. **Message Records**:
   - **Format**: `Message/{hash}/Main`
   - **Examples**:
     ```
     Message/99ba5065aa3b13879dd877e902d91a0d5ce5be4036a50af98169ba855b292db0/Main
     Message/bea05828b40be89ceab42cec819077d48513fe5a7c705f24a1c88cde680a83aa/Main
     ```
   - **Value Content**: Message data including sender, recipient, and content
   - **Location**: Written in `SectionTypeRecords` sections
   - **Binary Key Example**:
     ```
     # Key: Message/99ba5065aa3b13879dd877e902d91a0d5ce5be4036a50af98169ba855b292db0/Main
     03                           # 3 elements
     07 4D657373616765            # "Message"
     20 99ba5065aa3b13879dd877e902d91a0d5ce5be4036a50af98169ba855b292db0  # Hash (32 bytes hex)
     04 4D61696E                  # "Main"
     ```
   - **Binary Value Example**:
     ```
     # Message type
     02                           # Message type code
     # Sender URL (length-prefixed)
     14 6163633A2F2F73656E6465722E61636D65  # "acc://sender.acme"
     # Recipient URL (length-prefixed)
     17 6163633A2F2F7265636970656E742E61636D65  # "acc://recipient.acme"
     # Content (length-prefixed)
     64 ...                       # Message content data
     ```

5. **Directory Records**:
   - **Format**: `Directory/{url-prefix}`
   - **Examples**:
     ```
     Directory/acc://example.acme
     ```
   - **Value Content**: Directory information for URL prefixes
   - **Location**: Written in `SectionTypeRecords` sections
   - **Binary Key Example**:
     ```
     # Key: Directory/acc://example.acme
     02                           # 2 elements
     09 4469726563746F7279        # "Directory"
     11 6163633A2F2F6578616D706C652E61636D65  # "acc://example.acme"
     ```
   - **Binary Value Example**:
     ```
     # Directory entries count
     04                           # 4 entries (varint)
     # Entry 1 (length-prefixed)
     16 6163633A2F2F6578616D706C652E61636D652F6D61696E  # "acc://example.acme/main"
     # Additional entries follow...
     ```

6. **System Records**:
   - **Format**: `System/{key}`
   - **Examples**:
     ```
     System/Network
     System/Globals
     ```
   - **Value Content**: System-level configuration and state data
   - **Location**: Written in `SectionTypeRecords` sections
   - **Binary Key Example**:
     ```
     # Key: System/Network
     02                           # 2 elements
     06 53797374656D              # "System"
     07 4E6574776F726B            # "Network"
     ```
   - **Binary Value Example**:
     ```
     # Network parameters
     01                           # Version (varint)
     08 00E40B5402000000          # Network ID (8-byte integer)
     # Additional parameters follow...
     ```

## Snapshot Creation Process (Version 2)

The `debug snap collect` command creates version 2 snapshots using the following process:

1. **Initialize**:
   - Create a new snapshot file using `snapshot.Create()`
   - Write the header section using `WriteHeader()` with version set to 2
   - Set up the snapshot writer

2. **Collect Records**:
   - The `Batch.Collect` method creates separate type 7 sections for different record types:
     - `collectAccounts` opens one type 7 section for all account records
     - `collectMessages` opens another type 7 section for all message records
   - This separation is by design for logical organization, not due to size constraints
   - For each record:
     - Create a `RecordEntry` with the appropriate key and value
     - Write the record to the section using `WriteValue()`
   - Close each record section after its specific record type is fully written

3. **Build Record Index** (Optional):
   - Open an index section using `OpenIndex()`
   - For each record:
     - Create a `RecordIndexEntry` with key hash, section number, and offset
     - Write the entry to the index using `Write()`
   - Close the index section

4. **Build BPT** (Optional):
   - Open a BPT section using `OpenRaw(SectionTypeBPT)`
   - Add Binary Patricia Tree records for database structure
   - Close the BPT section
   - Can be skipped with `--skip-bpt` flag

5. **Finalize**:
   - Close the snapshot file

### Implementation Example

```go
// Create a new snapshot file
file, _ := os.Create("snapshot.dat")
writer, _ := snapshot.Create(file)

// Write the header
header := &snapshot.Header{
    RootHash: rootHash,
    SystemLedger: &snapshot.SystemLedger{
        Height:    100000,
        Timestamp: time.Now().Unix(),
    },
}
writer.WriteHeader(header)

// Open a record section
recordSection, _ := writer.OpenRaw(snapshot.SectionTypeRecords)

// Write records
for _, record := range records {
    entry := &snapshot.RecordEntry{
        Key:   record.Key(),
        Value: record.Value(),
    }
    recordSection.WriteValue(entry)
}
recordSection.Close()
```

## Snapshot Reading Process (Version 2)

When reading a version 2 snapshot file:

1. **Open and Verify**:
   - Open the snapshot file using `snapshot.Open()`
   - The function automatically verifies the version is 2
   - Access the header via the `Reader.Header` field

2. **Process Record Sections**:
   - Find record sections using `Reader.Open(SectionTypeRecords)`
   - Create a `RecordReader` using `OpenRecords()`
   - Iterate through records using `Read()`
   - Process each record based on its key path
   - Note that snapshot processing code (like `genesis.Extract` and `coredb.Restore`) is section-type agnostic
   - Records are filtered based on their content type (Account vs Message/Transaction) rather than which section they came from

3. **Use Record Index for Random Access** (Optional):
   - Open the index section using `OpenIndex()`
   - Read index entries using `Read()`
   - Use the section number and offset to directly access records

4. **Process BPT Sections** (Optional):
   - Open BPT sections using `Open(SectionTypeBPT)`
   - Read BPT entries to reconstruct the database structure

### Implementation Example

```go
// Open the snapshot file
file, _ := os.Open("snapshot.dat")
reader, _ := snapshot.Open(file)

// Access the header
fmt.Printf("Snapshot version: %d\n", reader.Header.Version)
fmt.Printf("Root hash: %x\n", reader.Header.RootHash)

// Open a record section
recordReader, _ := reader.OpenRecords(0)

// Read and process records
for {
    record, err := recordReader.Read()
    if err == io.EOF {
        break
    }
    
    // Process the record based on its key
    keyParts := record.Key.Elements()
    if len(keyParts) > 0 {
        switch keyParts[0] {
        case "Account":
            // Process account record
        case "Chain":
            // Process chain record
        case "Transaction":
            // Process transaction record
        }
    }
}
```

## Memory Optimization for Version 2 Snapshots

Version 2 snapshots implement several memory optimization techniques for processing large datasets:

### 1. Record Indexing

The `SectionTypeRecordIndex` section enables efficient random access to records:

- **Structure**: Fixed-width entries with key hash (32 bytes), section number (4 bytes), and offset (8 bytes)
- **Organization**: Entries are sorted by key hash in descending order for binary search
- **Usage**: Allows direct access to specific records without loading the entire snapshot

```go
type RecordIndexEntry struct {
    Key     [32]byte // SHA-256 hash of the record key
    Section uint32   // Section number containing the record
    Offset  uint64   // Byte offset within the section
}
```

### 2. URL Hash Handling

Version 2 snapshots use a hybrid approach for URL handling:

- **KV Database**: Stores URL hash to URL string mappings for fast lookups
- **Binary File**: Maintains URLs in a binary format for efficient iteration
- **Lookup Process**: First check the KV database, then fall back to file if needed
- **Memory Efficiency**: Avoids loading all URLs into memory at once

### 3. Streaming Record Processing

The `RecordReader` interface enables streaming record processing:

```go
type RecordReader interface {
    io.Seeker
    Read() (*RecordEntry, error)
    ReadAt(offset int64) (*RecordEntry, error)
}
```

This allows processing records one at a time:
1. Read a record using `Read()`
2. Process it
3. Discard it before reading the next record
4. Repeat until EOF

### 4. Section-Based Organization

Version 2 snapshots organize data into sections that can be processed independently:

- Each section can be read and processed separately
- Sections can be skipped if not needed
- Processing can be parallelized across sections
- Avoid loading all records into memory simultaneously

#### Multiple Type 7 (Record) Sections

Multiple type 7 (record) sections in a snapshot file are a deliberate design feature, not a limitation:

- **Functional Separation**: The snapshot collection process deliberately creates separate sections for different logical groups of records:
  - One type 7 section for account records (created by `collectAccounts`)
  - Another type 7 section for message records (created by `collectMessages`)

- **No Hard Size Limits**: There are no explicit hard-coded maximum size limits for snapshot sections:
  - The only theoretical size constraint is a 48-bit offset limit within a section (about 256 TB)
  - This limit is practically unreachable in real-world scenarios

- **Section-Agnostic Processing**: Code that processes snapshots doesn't care about section boundaries:
  ```go
  // From database.Restore
  for i, s := range rd.Sections {
      if s.Type() != snapshot.SectionTypeRecords {
          continue
      }
      rd, err := rd.OpenRecords(i)
      // Process each record section independently
  }
  ```

- **Two-Pass Processing**: Functions like `genesis.Extract` use a two-pass approach:
  - First pass processes only account records, filtering based on content
  - Second pass processes only message/transaction records, using references from the first pass
  - The snapshot reader is rewound between passes to process all sections again

## Combining Version 2 Snapshots

When combining multiple version 2 snapshots, the following approach is recommended to maintain memory efficiency and data integrity:

1. **Initialize Temporary Storage**:
   - Create a temporary key-value database for record deduplication
   - Set up URL hash mapping using both KV database and binary file format
   - Prepare output snapshot file using `snapshot.Create()`

2. **Process Input Snapshots Sequentially**:
   - Open each input snapshot using `snapshot.Open()`
   - Verify that each snapshot is version 2
   - Process record sections one at a time

3. **Deduplicate and Store Records**:
   - For each record:
     - Generate a unique key based on the record's key path
     - Check if the record already exists in the temporary database
     - If it exists, keep the newer version based on metadata
     - If it doesn't exist, add it to the temporary database

4. **Write Combined Snapshot**:
   - Write header section with updated metadata
   - Stream records from the temporary database to record sections
   - Build and write record index for efficient access
   - Optionally generate BPT sections for database structure

5. **Clean Up**:
   - Close all open files and resources
   - Remove temporary database and files

### Implementation Considerations

1. **Memory Efficiency**:
   - Process one record at a time using streaming techniques
   - Use record indexing for random access without loading entire snapshots
   - Leverage the URL hash handling approach that combines KV database and binary file format

2. **Record Organization**:
   - Group records by type (Account, Chain, Transaction, etc.)
   - Sort records within each group for better organization
   - Different record types are deliberately written to separate record sections:
     - Account records in one type 7 section
     - Message/transaction records in another type 7 section
   - This separation is maintained regardless of data size

3. **Error Handling**:
   - Implement robust validation for each record
   - Log warnings for non-critical issues but continue processing
   - Provide detailed error messages for debugging

4. **Progress Reporting**:
   - Report progress at regular intervals for large snapshots
   - Include statistics on records processed, deduplicated, and written

## Code References

### Core Snapshot Implementation

| File Path | Description |
|-----------|-------------|
| `/pkg/database/snapshot/format.go` | Core snapshot file format handling including reading and writing snapshots |
| `/pkg/database/snapshot/types.go` | Defines the SectionType type and its methods |
| `/pkg/database/snapshot/enums_gen.go` | Defines constants for snapshot section types |
| `/pkg/database/snapshot/types_gen.go` | Defines main data structures for snapshots (Header, RecordEntry, etc.) |
| `/pkg/database/snapshot/index.go` | Implements the Indexer type for writing record index sections |
| `/pkg/database/snapshot/store.go` | Implements the Store type providing keyvalue.Store interface over snapshot files |
| `/pkg/database/snapshot/collect.go` | Implements the Collector type for writing records into snapshot sections |
| `/pkg/database/snapshot/encoding.go` | Utilities for encoding and decoding snapshot data |

### Snapshot Reading and Writing

| File Path | Description |
|-----------|-------------|
| `/internal/database/snapshot.go` | Database integration for snapshots, including the Restore function |
| `/internal/database/snapshot/merkle_test.go` | Tests for snapshot functionality with Merkle trees |
| `/internal/bsn/executor.go` | Contains LoadSnapshot function for BSN (Block Summary Network) |
| `/internal/node/genesis/provider.go` | Contains ConvertSnapshotToJson function |
| `/tools/cmd/analyze/snap_read.go` | Implements snapshot reading logic, including opening files, verifying version, and processing records |
| `/tools/cmd/analyze/snap_write.go` | Implements snapshot writing logic, including writing records and streaming from database or file |

### Snapshot Tools and Utilities

| File Path | Description |
|-----------|-------------|
| `/tools/cmd/analyze/snap.go` | Core snapshot analysis functions including scanSnapshot and countRecordsByType |
| `/tools/cmd/analyze/snap_report.go` | Defines structures for snapshot reporting |
| `/tools/cmd/analyze/snap_report_cmd.go` | Command-line interface for generating reports from snapshots |
| `/tools/cmd/analyze/snap_processing.go` | Implements processing functions for snapshot data |
| `/tools/cmd/analyze/snap_url_hash.go` | Implements URL hash handling for snapshots with both KV database and binary file format |
| `/tools/cmd/debug/snap.go` | Main snapshot command group with utilities like rich-list extraction |
| `/tools/cmd/debug/snap_collect.go` | Implements snapshot collection from database to file with filtering options |
| `/tools/cmd/debug/snap_dump.go` | Dumps snapshot contents with options for short output and verification |
| `/tools/cmd/debug/snap_restore.go` | Restores a database from a snapshot file |
| `/tools/cmd/debug/snap_scan.go` | Core scanning utilities for snapshots including version detection and section processing |
| `/tools/cmd/debug/snap_index.go` | Contains indexSnapshot function for creating snapshot indexes |
| `/tools/cmd/snapshot/list.go` | Tool for listing snapshots |
| `/tools/cmd/genesis/snapshot.go` | Genesis-related snapshot functionality |

### Tests and Examples

| File Path | Description |
|-----------|-------------|
| `/pkg/database/snapshot/collect_test.go` | Tests for the snapshot collection functionality |
| `/pkg/accumulate/checkpoint_test.go` | Tests for checkpoint functionality using snapshots |
| `/test/encoding/db_test.go` | Tests for database encoding using snapshots |
| `/tools/cmd/analyze/snap_read_test.go` | Tests for snapshot reading functionality |
| `/tools/cmd/analyze/snap_report_test.go` | Tests for snapshot reporting functionality |
| `/tools/cmd/analyze/snap_url_hash_test.go` | Tests for URL hash handling in snapshots |

## Binary Implementation Guide

This section provides a step-by-step guide for implementing snapshot file reading and writing at the binary level.

### Writing a Snapshot File

#### 1. Create the File and Write the Header Section

```go
// Open a file for writing
file, err := os.Create("snapshot.dat")
if err != nil {
    return err
}
defer file.Close()

// Write the header section header
headerType := uint32(1) // SectionTypeHeader
headerSize := uint64(15) // Size will be 15 bytes for this example

// Write section type (4 bytes, little-endian)
binary.Write(file, binary.LittleEndian, headerType)

// Write section size (8 bytes, little-endian)
binary.Write(file, binary.LittleEndian, headerSize)

// Write header content
// - Length prefix (varint): 13 bytes
file.Write([]byte{0x0D})

// - Version (varint): 2
file.Write([]byte{0x02})

// - Root hash length (varint): 32 bytes
file.Write([]byte{0x20})

// - Root hash (32 bytes)
rootHash := make([]byte, 32) // All zeros for this example
file.Write(rootHash)

// - System ledger flag: 0 (not present)
file.Write([]byte{0x00})
```

#### 2. Write a Records Section with One Record

```go
// Write the records section header
recordsType := uint32(7) // SectionTypeRecords
recordsSize := uint64(35) // Size will be 35 bytes for this example

// Write section type (4 bytes, little-endian)
binary.Write(file, binary.LittleEndian, recordsType)

// Write section size (8 bytes, little-endian)
binary.Write(file, binary.LittleEndian, recordsSize)

// Write a single record
// - Record length (varint): 33 bytes
file.Write([]byte{0x21})

// - Key with 2 elements
file.Write([]byte{0x02}) // 2 elements (varint)

// - First element: "Account" (7 bytes)
file.Write([]byte{0x07}) // Length (varint)
file.Write([]byte("Account"))

// - Second element: "acc://example" (12 bytes)
file.Write([]byte{0x0C}) // Length (varint)
file.Write([]byte("acc://example"))

// - Value: "data" (4 bytes)
file.Write([]byte{0x04}) // Length (varint)
file.Write([]byte("data"))

// - Receipt flag: 0 (not present)
file.Write([]byte{0x00})
```

#### 3. Write a Record Index Section

```go
// Write the record index section header
indexType := uint32(8) // SectionTypeRecordIndex
indexSize := uint64(44) // Size will be 44 bytes for this example (one entry)

// Write section type (4 bytes, little-endian)
binary.Write(file, binary.LittleEndian, indexType)

// Write section size (8 bytes, little-endian)
binary.Write(file, binary.LittleEndian, indexSize)

// Write a single index entry (44 bytes total)
// - Key hash (32 bytes) - SHA-256 hash of the record key
keyHash := sha256.Sum256([]byte("Account/acc://example"))
file.Write(keyHash[:])

// - Section number (4 bytes, little-endian) - refers to the records section
sectionNumber := uint32(0) // First section
binary.Write(file, binary.LittleEndian, sectionNumber)

// - Offset (8 bytes, little-endian) - byte offset within the section
offset := uint64(0) // Start of the section
binary.Write(file, binary.LittleEndian, offset)
```

### Reading a Snapshot File

#### 1. Open the File and Read the Header Section

```go
// Open the file for reading
file, err := os.Open("snapshot.dat")
if err != nil {
    return err
}
defer file.Close()

// Read the section header
var sectionType uint32
var sectionSize uint64

binary.Read(file, binary.LittleEndian, &sectionType)
binary.Read(file, binary.LittleEndian, &sectionSize)

// Verify it's a header section
if sectionType != 1 { // SectionTypeHeader
    return errors.New("first section is not a header")
}

// Read the header content
headerData := make([]byte, sectionSize)
file.Read(headerData)

// Parse the header
// - Read length prefix (varint)
headerLength, bytesRead := binary.Uvarint(headerData)
pos := bytesRead

// - Read version (varint)
version, bytesRead := binary.Uvarint(headerData[pos:])
pos += bytesRead
if version != 2 {
    return errors.New("unsupported snapshot version")
}

// - Read root hash length (varint)
hashLength, bytesRead := binary.Uvarint(headerData[pos:])
pos += bytesRead
if hashLength != 32 {
    return errors.New("invalid root hash length")
}

// - Read root hash (32 bytes)
rootHash := headerData[pos:pos+32]
pos += 32

// - Read system ledger flag
hasSystemLedger := headerData[pos] != 0
pos++

// - Read system ledger data if present
if hasSystemLedger {
    // Read height (varint)
    height, bytesRead := binary.Uvarint(headerData[pos:])
    pos += bytesRead
    
    // Read timestamp (varint)
    timestamp, _ := binary.Uvarint(headerData[pos:])
    
    fmt.Printf("System ledger: height=%d, timestamp=%d\n", height, timestamp)
}
```

#### 2. Read Records Sections

```go
// Read the next section header
binary.Read(file, binary.LittleEndian, &sectionType)
binary.Read(file, binary.LittleEndian, &sectionSize)

// Check if it's a records section
if sectionType != 7 { // SectionTypeRecords
    return errors.New("expected records section")
}

// Read the records section content
recordsData := make([]byte, sectionSize)
file.Read(recordsData)

// Parse records
pos := 0
for pos < len(recordsData) {
    // Read record length (varint)
    recordLength, bytesRead := binary.Uvarint(recordsData[pos:])
    pos += bytesRead
    recordEnd := pos + int(recordLength)
    
    // Read key
    // - Number of elements (varint)
    elementCount, bytesRead := binary.Uvarint(recordsData[pos:])
    pos += bytesRead
    
    // - Read each element
    elements := make([]string, elementCount)
    for i := 0; i < int(elementCount); i++ {
        // Element length (varint)
        elemLength, bytesRead := binary.Uvarint(recordsData[pos:])
        pos += bytesRead
        
        // Element data
        elements[i] = string(recordsData[pos:pos+int(elemLength)])
        pos += int(elemLength)
    }
    
    // Read value length (varint)
    valueLength, bytesRead := binary.Uvarint(recordsData[pos:])
    pos += bytesRead
    
    // Read value data
    value := recordsData[pos:pos+int(valueLength)]
    pos += int(valueLength)
    
    // Read receipt flag
    hasReceipt := recordsData[pos] != 0
    pos++
    
    // Read receipt if present
    if hasReceipt {
        // Receipt parsing logic here
        // ...
    }
    
    // Process the record
    keyPath := strings.Join(elements, "/")
    fmt.Printf("Record: key=%s, value=%x\n", keyPath, value)
    
    // Ensure we're at the expected position
    if pos != recordEnd {
        return errors.New("record parsing error")
    }
}
```

#### 3. Read Record Index Section

```go
// Read the next section header
binary.Read(file, binary.LittleEndian, &sectionType)
binary.Read(file, binary.LittleEndian, &sectionSize)

// Check if it's a record index section
if sectionType != 8 { // SectionTypeRecordIndex
    return errors.New("expected record index section")
}

// Calculate number of entries (each entry is 44 bytes)
entryCount := sectionSize / 44

// Read and process each entry
for i := 0; i < int(entryCount); i++ {
    // Read key hash (32 bytes)
    keyHash := make([]byte, 32)
    file.Read(keyHash)
    
    // Read section number (4 bytes)
    var sectionNumber uint32
    binary.Read(file, binary.LittleEndian, &sectionNumber)
    
    // Read offset (8 bytes)
    var offset uint64
    binary.Read(file, binary.LittleEndian, &offset)
    
    fmt.Printf("Index entry: keyHash=%x, section=%d, offset=%d\n", 
               keyHash, sectionNumber, offset)
}
```

### Complete Minimal Snapshot Example

Here's a hexadecimal dump of a complete minimal snapshot file with one header section and one record section containing a single record:

```
# Section Header (Type=1, Size=15)
01 00 00 00  0F 00 00 00  00 00 00 00

# Header Section (Length=13, Version=2, Root Hash=32 zeros, No System Ledger)
0D  02  20  00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00  00

# Section Header (Type=7, Size=35)
07 00 00 00  23 00 00 00  00 00 00 00

# Record Section with one record
# Record Length=33, Key with 2 elements: "Account", "acc://example", Value="data", No Receipt
21  02  07 41 63 63 6F 75 6E 74  0C 61 63 63 3A 2F 2F 65 78 61 6D 70 6C 65  04 64 61 74 61  00
```

This example demonstrates the binary structure of a minimal valid snapshot file that can be read and processed by snapshot tools.

## Record Sorting in Snapshots

Understanding how records are sorted in snapshots is crucial for correctly processing and combining them. Different record types follow different sorting rules:

### Account Records

- **No Explicit Sorting**: Account records are **not** explicitly sorted during snapshot collection
- **Order Preservation**: Accounts appear in the order they are returned by `batch.IterateAccounts()`
- **Database Insertion Order**: The database preserves the insertion order of accounts during ingest
- **Snapshot Combination**: When combining multiple snapshots, accounts from the first snapshot will appear before accounts from the second snapshot, and so on

### Message Records

- **Explicit Sorting**: Message records are explicitly sorted by their hash values
- **Sort Order**: Messages are sorted in ascending order by hash (smaller hash values first)
- **Sort Implementation**:
  ```go
  sort.Slice(hashes, func(i, j int) bool {
      return bytes.Compare(hashes[i].Hash[:], hashes[j].Hash[:]) < 0
  })
  ```
- **Deduplication**: Sorting enables efficient detection and elimination of duplicate messages
- **Snapshot Combination**: When combining snapshots, all message hashes are collected and sorted together, ensuring consistent ordering regardless of input order

### Record Index

- **Sort Order**: Record index entries are sorted by key hash in descending order
- **Purpose**: Enables efficient binary search for random access to records

### Implications for Processing

- **Deterministic Processing**: Message sorting ensures deterministic processing across different nodes
- **Functional Separation**: The different sorting approaches reflect the functional separation between accounts and messages
- **Reconstruction**: When a snapshot is loaded, the database will rebuild its internal structures regardless of the original ordering

## Snapshot Combining Algorithm

Version 2 snapshots can be combined to create a unified view of the Accumulate state. This is particularly useful when working with snapshots from different partitions or time periods. The following algorithm describes the step-by-step process for combining snapshots, based on the implementation used by the `debug genesis ingest` command.

### Memory-Efficient Batch Processing Algorithm

To optimize memory usage when combining large snapshots, we use a bucket-based processing approach that:

1. Processes accounts in manageable chunks to limit memory usage
2. Uses file-backed buckets for intermediate storage
3. Performs sorting only once at the end of processing
4. Ensures all records are properly sorted in descending order as required by the snapshot format

The algorithm consists of these main phases:

1. **Preparation**: Set up bucket files and validate input snapshots
2. **Index Creation**: Create indices for all input snapshots for efficient record lookup
3. **Record Processing**:
   - Account records are processed in the order they appear in each input snapshot
   - Message records are collected from all snapshots and then sorted by hash
3. **Batch Processing**: Process accounts in batches, distributing records to bucket files
4. **Bucket Sorting**: Sort records within each bucket in descending key order
5. **Final Assembly**: Combine sorted buckets into the final snapshot with proper section structure

#### 1. Preparation and Index Creation

```go
// Constants for bucket organization and batch processing
const (
    // Number of bucket files to use for sorting
    // More buckets = less memory per bucket but more files
    numBuckets = 256
    
    // Bucket directory for temporary files
    bucketDir = "./snapshot_buckets"
    
    // Batch size for processing accounts
    batchSize = 10000
)

// Global registry of bucket files
var (
    accountBuckets [numBuckets]*os.File
    txBuckets [numBuckets]*os.File
    bucketInitialized bool
)

// Record location in a snapshot file
type RecordLocation struct {
    SectionNumber uint32
    Offset        uint64
}

// Index of a snapshot file for fast record lookups
type SnapshotIndex struct {
    Path        string
    RecordIndex map[[32]byte]RecordLocation
    UrlIndex    map[[32]byte]RecordLocation  // For URL lookups
}

// Initialize bucket files for storing intermediate records
func initBuckets() error {
    if bucketInitialized {
        return nil
    }
    
    // Clean up any existing bucket directory
    os.RemoveAll(bucketDir)
    
    // Create bucket directory
    if err := os.MkdirAll(bucketDir, 0755); err != nil {
        return err
    }
    
    // Create bucket files for accounts
    for i := 0; i < numBuckets; i++ {
        file, err := os.Create(fmt.Sprintf("%s/accounts_%03d.tmp", bucketDir, i))
        if err != nil {
            return err
        }
        accountBuckets[i] = file
    }
    
    // Create bucket files for transactions
    for i := 0; i < numBuckets; i++ {
        file, err := os.Create(fmt.Sprintf("%s/transactions_%03d.tmp", bucketDir, i))
        if err != nil {
            return err
        }
        txBuckets[i] = file
    }
    
    bucketInitialized = true
    return nil
}

// Create indices for all input snapshots for efficient record lookup
func indexAllSnapshots(paths []string) ([]*SnapshotIndex, error) {
    // Initialize bucket files first
    if err := initBuckets(); err != nil {
        return nil, fmt.Errorf("failed to initialize buckets: %w", err)
    }
    
    var indices []*SnapshotIndex
    
    for _, path := range paths {
        // Open snapshot file
        file, err := os.Open(path)
        if err != nil {
            return nil, fmt.Errorf("failed to open snapshot %s: %w", path, err)
        }
        defer file.Close()
        
        // Create index for this snapshot
        index := &SnapshotIndex{
            Path:        path,
            RecordIndex: make(map[[32]byte]RecordLocation),
            UrlIndex:    make(map[[32]byte]RecordLocation),
        }
        
        // Scan sections
        scanner := snapshot.NewScanner(file)
        for scanner.Scan() {
            section := scanner.Section()
            
            // Process record index sections
            if section.Type == snapshot.SectionTypeRecordIndex {
                // Read record index entries
                reader := snapshot.NewRecordIndexReader(section)
                for reader.Next() {
                    entry := reader.Entry()
                    
                    // Store key hash -> location mapping
                    index.RecordIndex[entry.KeyHash] = RecordLocation{
                        SectionNumber: entry.SectionNumber,
                        Offset:        entry.Offset,
                    }
                }
            }
            
            // Also index URL sections for efficient URL lookups
            if section.Type == snapshot.SectionTypeUrls {
                // Read URL entries
                reader := snapshot.NewUrlReader(section)
                for reader.Next() {
                    url := reader.Url()
                    
                    // Calculate URL hash
                    urlHash := sha256.Sum256([]byte(url.String()))
                    
                    // Store URL hash -> location mapping
                    index.UrlIndex[urlHash] = RecordLocation{
                        SectionNumber: uint32(scanner.SectionNumber()),
                        Offset:        uint64(reader.Offset()),
                    }
                }
            }
        }
        
        if len(index.RecordIndex) == 0 {
            return nil, fmt.Errorf("no record index found in snapshot %s", path)
        }
        
        indices = append(indices, index)
    }
    
    return indices, nil
}
```

#### 2. Group Accounts by Batch

```go
// Account data with metadata
type AccountData struct {
    Record        *snapshot.RecordEntry
    TxReferences  [][32]byte         // Transaction hashes referenced by this account
    UrlReferences [][32]byte         // URL hashes referenced by this account
}

// Process all snapshots in batches
func processBatchedAccounts(indices []*SnapshotIndex) error {
    // Get all unique account IDs across all snapshots
    allAccounts := make(map[[32]byte]bool)
    
    // First pass: collect all account keys
    for _, idx := range indices {
        for keyHash := range idx.RecordIndex {
            // Only include Account records
            // This check uses the key prefix to determine if it's an account
            if isAccountRecord(keyHash, idx) {
                allAccounts[keyHash] = true
            }
        }
    }
    
    // Convert to slice for batch processing
    accountList := make([][32]byte, 0, len(allAccounts))
    for acc := range allAccounts {
        accountList = append(accountList, acc)
    }
    
    // Process in batches to limit memory usage
    for i := 0; i < len(accountList); i += batchSize {
        end := i + batchSize
        if end > len(accountList) {
            end = len(accountList)
        }
        
        // Process this batch
        batch := accountList[i:end]
        if err := processBatch(batch, indices); err != nil {
            return fmt.Errorf("error processing batch %d-%d: %w", i, end, err)
        }
        
        // Force garbage collection after each batch to minimize memory usage
        runtime.GC()
    }
    
    return nil
}

// Helper function to determine if a key hash belongs to an account record
// Uses the record key prefix to check if it's an account
func isAccountRecord(keyHash [32]byte, idx *SnapshotIndex) bool {
    // Look up the record location
    loc, exists := idx.RecordIndex[keyHash]
    if !exists {
        return false
    }
    
    // Open the snapshot file
    file, err := os.Open(idx.Path)
    if err != nil {
        return false
    }
    defer file.Close()
    
    // Read the record
    record := readRecordFromLocation(file, loc)
    if record == nil {
        return false
    }
    
    // Check if the key starts with "Account/"
    return strings.HasPrefix(record.Key.String(), "Account/")
}
```

#### 3. Process Each Batch

```go
func processBatch(accountBatch [][32]byte, indices []*SnapshotIndex) error {
    // Create temporary storage for this batch
    batchData := make(map[[32]byte]*AccountData)
    batchTransactions := make(map[[32]byte]bool)
    batchUrls := make(map[[32]byte]bool)
    
    // For each account in the batch
    for _, accID := range accountBatch {
        // Find the account in all snapshots (last one wins)
        var latestAccount *AccountData
        
        // Process snapshots in reverse order to ensure "last one wins"
        for i := len(indices) - 1; i >= 0; i-- {
            idx := indices[i]
            if loc, exists := idx.RecordIndex[accID]; exists {
                // Open the snapshot file
                file, err := os.Open(idx.Path)
                if err != nil {
                    continue
                }
                defer file.Close()
                
                // Read the record
                record := readRecordFromLocation(file, loc)
                if record == nil {
                    continue
                }
                
                // Parse account data and extract references
                acc := &AccountData{
                    Record: record,
                }
                
                // Extract transaction references
                txRefs, err := extractTransactionReferences(record)
                if err == nil && len(txRefs) > 0 {
                    acc.TxReferences = txRefs
                }
                
                // Extract URL references
                urlRefs, err := extractUrlReferences(record)
                if err == nil && len(urlRefs) > 0 {
                    acc.UrlReferences = urlRefs
                }
                
                latestAccount = acc
                break // Found the latest version of this account
            }
        }
        
        if latestAccount != nil {
            // Store the account data
            batchData[accID] = latestAccount
            
            // Collect transaction hashes referenced by this account
            for _, txHash := range latestAccount.TxReferences {
                batchTransactions[txHash] = true
            }
            
            // Collect URL hashes referenced by this account
            for _, urlHash := range latestAccount.UrlReferences {
                batchUrls[urlHash] = true
            }
        }
    }
    
    // Write batch data to temporary bucket files
    if err := writeBatchToBucketFiles(batchData, batchTransactions, batchUrls, indices); err != nil {
        return fmt.Errorf("failed to write batch to bucket files: %w", err)
    }
    
    // Clear memory used by this batch
    batchData = nil
    batchTransactions = nil
    batchUrls = nil
    
    return nil
}

// Extract transaction references from a record
func extractTransactionReferences(record *snapshot.RecordEntry) ([][32]byte, error) {
    var refs [][32]byte
    
    // Implementation depends on the specific record structure
    // This is a simplified example - actual implementation would parse the record value
    // to find transaction references based on the record type and schema
    
    // Example: Extract transaction hashes from account state
    if strings.HasPrefix(record.Key.String(), "Account/") {
        // Parse account state and extract transaction references
        // ...
    }
    
    return refs, nil
}

// Extract URL references from a record
func extractUrlReferences(record *snapshot.RecordEntry) ([][32]byte, error) {
    var refs [][32]byte
    
    // Implementation depends on the specific record structure
    // This is a simplified example - actual implementation would parse the record value
    // to find URL references based on the record type and schema
    
    // Example: Extract URLs from account state
    if strings.HasPrefix(record.Key.String(), "Account/") {
        // Parse account state and extract URL references
        // ...
    }
    
    return refs, nil
}
```

#### 4. Write Batch to Bucket Files

```go
// Global registry of bucket files
var (
    accountBuckets [numBuckets]*os.File
    txBuckets [numBuckets]*os.File
    urlBuckets [numBuckets]*os.File
    bucketInitialized bool
)

// Initialize bucket files if needed
func initBuckets() error {
    if bucketInitialized {
        return nil
    }
    
    // Clean up any existing bucket directory
    os.RemoveAll(bucketDir)
    
    // Create bucket directory
    if err := os.MkdirAll(bucketDir, 0755); err != nil {
        return err
    }
    
    // Create bucket files for accounts
    for i := 0; i < numBuckets; i++ {
        file, err := os.Create(fmt.Sprintf("%s/accounts_%03d.tmp", bucketDir, i))
        if err != nil {
            return err
        }
        accountBuckets[i] = file
    }
    
    // Create bucket files for transactions
    for i := 0; i < numBuckets; i++ {
        file, err := os.Create(fmt.Sprintf("%s/transactions_%03d.tmp", bucketDir, i))
        if err != nil {
            return err
        }
        txBuckets[i] = file
    }
    
    // Create bucket files for URLs
    for i := 0; i < numBuckets; i++ {
        file, err := os.Create(fmt.Sprintf("%s/urls_%03d.tmp", bucketDir, i))
        if err != nil {
            return err
        }
        urlBuckets[i] = file
    }
    
    bucketInitialized = true
    return nil
}

// Get bucket index from key hash
func getBucketIndex(keyHash [32]byte) int {
    // Use the first byte of the hash for bucketing
    // This gives us 256 buckets (0-255)
    return int(keyHash[0])
}

// Write a batch of records to their respective bucket files
func writeBatchToBucketFiles(accounts map[[32]byte]*AccountData, transactions map[[32]byte]bool, urls map[[32]byte]bool, indices []*SnapshotIndex) error {
    // Write accounts to appropriate bucket files
    for accID, acc := range accounts {
        bucketIndex := getBucketIndex(accID)
        if err := writeRecordToFile(accountBuckets[bucketIndex], acc.Record); err != nil {
            return fmt.Errorf("failed to write account to bucket: %w", err)
        }
    }
    
    // Write transactions to appropriate bucket files
    for txHash := range transactions {
        bucketIndex := getBucketIndex(txHash)
        
        // Find and write transaction from source snapshots
        if err := writeRecordFromSnapshots(txHash, txBuckets[bucketIndex], indices); err != nil {
            // Log error but continue - we don't want to fail the entire batch for one transaction
            fmt.Printf("Warning: failed to write transaction %x: %v\n", txHash, err)
        }
    }
    
    // Write URLs to appropriate bucket files
    for urlHash := range urls {
        bucketIndex := getBucketIndex(urlHash)
        
        // Find and write URL from source snapshots
        if err := writeUrlFromSnapshots(urlHash, urlBuckets[bucketIndex], indices); err != nil {
            // Log error but continue - we don't want to fail the entire batch for one URL
            fmt.Printf("Warning: failed to write URL %x: %v\n", urlHash, err)
        }
    }
    
    return nil
}

// Write a record from snapshots to a bucket file
func writeRecordFromSnapshots(keyHash [32]byte, bucketFile *os.File, indices []*SnapshotIndex) error {
    // Process snapshots in reverse order to ensure "last one wins"
    for i := len(indices) - 1; i >= 0; i-- {
        idx := indices[i]
        if loc, exists := idx.RecordIndex[keyHash]; exists {
            // Open the snapshot file
            srcFile, err := os.Open(idx.Path)
            if err != nil {
                continue
            }
            defer srcFile.Close()
            
            // Navigate to the record
            record := readRecordFromLocation(srcFile, loc)
            if record != nil {
                // Write the record to the bucket file
                if err := writeRecordToFile(bucketFile, record); err != nil {
                    return err
                }
                return nil // Found and wrote the record, no need to check other snapshots
            }
        }
    }
    
    return fmt.Errorf("record not found in any snapshot")
}

// Write a URL from snapshots to a bucket file
func writeUrlFromSnapshots(urlHash [32]byte, bucketFile *os.File, indices []*SnapshotIndex) error {
    // Process snapshots in reverse order to ensure "last one wins"
    for i := len(indices) - 1; i >= 0; i-- {
        idx := indices[i]
        if loc, exists := idx.UrlIndex[urlHash]; exists {
            // Open the snapshot file
            srcFile, err := os.Open(idx.Path)
            if err != nil {
                continue
            }
            defer srcFile.Close()
            
            // Navigate to the URL
            url := readUrlFromLocation(srcFile, loc)
            if url != nil {
                // Write the URL to the bucket file
                if err := writeUrlToFile(bucketFile, url); err != nil {
                    return err
                }
                return nil // Found and wrote the URL, no need to check other snapshots
            }
        }
    }
    
    return fmt.Errorf("URL not found in any snapshot")
}

// Write a record to a file
func writeRecordToFile(file *os.File, record *snapshot.RecordEntry) error {
    // Serialize the record
    data, err := record.MarshalBinary()
    if err != nil {
        return err
    }
    
    // Write size prefix (8 bytes, little-endian)
    sizeBuf := make([]byte, 8)
    binary.LittleEndian.PutUint64(sizeBuf, uint64(len(data)))
    if _, err := file.Write(sizeBuf); err != nil {
        return err
    }
    
    // Write record data
    if _, err := file.Write(data); err != nil {
        return err
    }
    
    return nil
}

// Write a URL to a file
func writeUrlToFile(file *os.File, url *snapshot.Url) error {
    // Serialize the URL
    data, err := url.MarshalBinary()
    if err != nil {
        return err
    }
    
    // Write size prefix (8 bytes, little-endian)
    sizeBuf := make([]byte, 8)
    binary.LittleEndian.PutUint64(sizeBuf, uint64(len(data)))
    if _, err := file.Write(sizeBuf); err != nil {
        return err
    }
    
    // Write URL data
    if _, err := file.Write(data); err != nil {
        return err
    }
    
    return nil
}
```

#### 5. Sort and Merge Bucket Files

```go
// After all batches are processed, sort each bucket and merge into final snapshot
func sortAndMergeBuckets(outputPath string) error {
    // Create the output snapshot file
    outFile, err := os.Create(outputPath)
    if err != nil {
        return fmt.Errorf("failed to create output file: %w", err)
    }
    defer outFile.Close()
    
    // Create a snapshot writer
    writer := snapshot.NewWriter(outFile)
    
    // Write header section
    if err := writer.WriteHeader(&snapshot.HeaderSection{
        Version:     2,
        Created:     time.Now(),
        Description: "Combined snapshot",
    }); err != nil {
        return fmt.Errorf("failed to write header: %w", err)
    }
    
    // Start record section
    recordSectionWriter, err := writer.NewSection(snapshot.SectionTypeRecords)
    if err != nil {
        return fmt.Errorf("failed to create record section: %w", err)
    }
    
    // Keep track of record locations for index creation
    recordLocations := make(map[[32]byte]snapshot.RecordIndexEntry)
    sectionNumber := uint32(writer.SectionCount() - 1) // Current section number (0-indexed)
    
    // Process account records first (accounts have priority)
    fmt.Println("Processing account records...")
    for i := 0; i < numBuckets; i++ {
        if err := processBucket(accountBuckets[i], recordSectionWriter, recordLocations, sectionNumber); err != nil {
            return fmt.Errorf("failed to process account bucket %d: %w", i, err)
        }
        
        // Close the bucket file and remove it
        accountBuckets[i].Close()
        os.Remove(fmt.Sprintf("%s/accounts_%03d.tmp", bucketDir, i))
    }
    
    // Process transaction records next
    fmt.Println("Processing transaction records...")
    for i := 0; i < numBuckets; i++ {
        if err := processBucket(txBuckets[i], recordSectionWriter, recordLocations, sectionNumber); err != nil {
            return fmt.Errorf("failed to process transaction bucket %d: %w", i, err)
        }
        
        // Close the bucket file and remove it
        txBuckets[i].Close()
        os.Remove(fmt.Sprintf("%s/transactions_%03d.tmp", bucketDir, i))
    }
    
    // Close the record section
    if err := recordSectionWriter.Close(); err != nil {
        return fmt.Errorf("failed to close record section: %w", err)
    }
    
    // Start URL section
    urlSectionWriter, err := writer.NewSection(snapshot.SectionTypeUrls)
    if err != nil {
        return fmt.Errorf("failed to create URL section: %w", err)
    }
    
    // Process URL records
    fmt.Println("Processing URL records...")
    for i := 0; i < numBuckets; i++ {
        if err := processUrlBucket(urlBuckets[i], urlSectionWriter); err != nil {
            return fmt.Errorf("failed to process URL bucket %d: %w", i, err)
        }
        
        // Close the bucket file and remove it
        urlBuckets[i].Close()
        os.Remove(fmt.Sprintf("%s/urls_%03d.tmp", bucketDir, i))
    }
    
    // Close the URL section
    if err := urlSectionWriter.Close(); err != nil {
        return fmt.Errorf("failed to close URL section: %w", err)
    }
    
    // Write record index section
    if err := writeRecordIndex(writer, recordLocations); err != nil {
        return fmt.Errorf("failed to write record index: %w", err)
    }
    
    // Clean up bucket directory
    os.RemoveAll(bucketDir)
    
    return nil
}

// Process a single bucket file for records
func processBucket(bucketFile *os.File, writer snapshot.ValueWriter, recordLocations map[[32]byte]snapshot.RecordIndexEntry, sectionNumber uint32) error {
    // Seek to beginning of bucket file
    if _, err := bucketFile.Seek(0, 0); err != nil {
        return err
    }
    
    // Read all records from bucket file
    var records []*snapshot.RecordEntry
    
    for {
        // Read size prefix
        sizeBuf := make([]byte, 8)
        n, err := bucketFile.Read(sizeBuf)
        if err != nil {
            if err == io.EOF {
                break
            }
            return err
        }
        
        if n < 8 {
            break // Incomplete read, end of file
        }
        
        size := binary.LittleEndian.Uint64(sizeBuf)
        
        // Read record data
        data := make([]byte, size)
        n, err = bucketFile.Read(data)
        if err != nil {
            return err
        }
        
        if uint64(n) < size {
            return fmt.Errorf("incomplete record read: got %d, expected %d", n, size)
        }
        
        // Unmarshal record
        var record snapshot.RecordEntry
        if err := record.UnmarshalBinary(data); err != nil {
            return err
        }
        
        records = append(records, &record)
    }
    
    // Sort records by key hash in descending order
    sort.Slice(records, func(i, j int) bool {
        // Compare key hashes in descending order
        return bytes.Compare(records[i].KeyHash[:], records[j].KeyHash[:]) > 0
    })
    
    // Write sorted records to output snapshot
    for _, record := range records {
        // Record the offset before writing
        offset := writer.Position()
        
        // Write the record
        if err := writer.WriteValue(record); err != nil {
            return err
        }
        
        // Store record location for index
        recordLocations[record.KeyHash] = snapshot.RecordIndexEntry{
            KeyHash:       record.KeyHash,
            SectionNumber: sectionNumber,
            Offset:        offset,
        }
    }
    
    return nil
}

// Process a single bucket file for URLs
func processUrlBucket(bucketFile *os.File, writer snapshot.UrlWriter) error {
    // Seek to beginning of bucket file
    if _, err := bucketFile.Seek(0, 0); err != nil {
        return err
    }
    
    // Read all URLs from bucket file
    var urls []*snapshot.Url
    
    for {
        // Read size prefix
        sizeBuf := make([]byte, 8)
        n, err := bucketFile.Read(sizeBuf)
        if err != nil {
            if err == io.EOF {
                break
            }
            return err
        }
        
        if n < 8 {
            break // Incomplete read, end of file
        }
        
        size := binary.LittleEndian.Uint64(sizeBuf)
        
        // Read URL data
        data := make([]byte, size)
        n, err = bucketFile.Read(data)
        if err != nil {
            return err
        }
        
        if uint64(n) < size {
            return fmt.Errorf("incomplete URL read: got %d, expected %d", n, size)
        }
        
        // Unmarshal URL
        var url snapshot.Url
        if err := url.UnmarshalBinary(data); err != nil {
            return err
        }
        
        urls = append(urls, &url)
    }
    
    // Sort URLs alphabetically
    sort.Slice(urls, func(i, j int) bool {
        return urls[i].String() < urls[j].String()
    })
    
    // Write sorted URLs to output snapshot
    for _, url := range urls {
        if err := writer.WriteUrl(url); err != nil {
            return err
        }
    }
    
    return nil
}

// Write the record index section to the snapshot
func writeRecordIndex(writer *snapshot.Writer, recordLocations map[[32]byte]snapshot.RecordIndexEntry) error {
    // Create a new index section
    indexWriter, err := writer.NewSection(snapshot.SectionTypeRecordIndex)
    if err != nil {
        return fmt.Errorf("failed to create record index section: %w", err)
    }
    
    // Convert map to slice for sorting
    entries := make([]snapshot.RecordIndexEntry, 0, len(recordLocations))
    for _, entry := range recordLocations {
        entries = append(entries, entry)
    }
    
    // Sort entries by key hash in descending order
    // This is critical - the Indexer.Write method enforces this order
    sort.Slice(entries, func(i, j int) bool {
        return bytes.Compare(entries[i].KeyHash[:], entries[j].KeyHash[:]) > 0
    })
    
    // Create an indexer to write entries
    indexer := snapshot.NewIndexer(indexWriter)
    
    // Write all entries
    for _, entry := range entries {
        if err := indexer.Write(entry); err != nil {
            return fmt.Errorf("failed to write index entry: %w", err)
        }
    }
    
    // Close the index section
    if err := indexWriter.Close(); err != nil {
        return fmt.Errorf("failed to close record index section: %w", err)
    }
    
    return nil
}

// Clean up all bucket files and temporary directory
func cleanupBuckets() error {
    // Close all bucket files
    for i := 0; i < numBuckets; i++ {
        if accountBuckets[i] != nil {
            accountBuckets[i].Close()
        }
        if txBuckets[i] != nil {
            txBuckets[i].Close()
        }
        if urlBuckets[i] != nil {
            urlBuckets[i].Close()
        }
    }
    
    // Reset bucket initialized flag
    bucketInitialized = false
    
    // Remove bucket directory
    return os.RemoveAll(bucketDir)
}

```

### Summary of the Snapshot Combining Algorithm

The snapshot combining algorithm uses a memory-efficient, file-backed bucket sorting approach to merge multiple snapshots while maintaining proper record ordering. Here's a summary of the key aspects:

1. **Memory Efficiency**:
   - Processes accounts in batches of 10,000 to limit memory usage
   - Uses file-backed buckets (256 buckets based on the first byte of key hash) to store intermediate records
   - Performs garbage collection after each batch to minimize memory footprint

2. **Record Ordering**:
   - Ensures all records are sorted in descending key hash order as required by the snapshot format
   - Performs global sorting across all snapshots, not just per-batch sorting
   - Maintains proper ordering by sorting each bucket after all records are processed

3. **Idempotency**:
   - Processes snapshots in reverse order to ensure "last one wins" semantics
   - Maintains logical idempotency (same input snapshots always produce equivalent output)
   - Handles duplicate records correctly by using the latest version

4. **URL Handling**:
   - Efficiently processes URLs using both direct file access and indexing
   - Maintains URL references from accounts
   - Sorts URLs alphabetically in the final snapshot

5. **BPT Handling**:
   - Intentionally skips BPT section during combining to simplify processing
   - BPT can be regenerated from the record section if needed

6. **Robust Indexing**:
   - Creates comprehensive record and URL indices for efficient lookups
   - Ensures index entries are properly sorted in descending order
   - Handles both record key hashes and URL hashes for complete coverage

7. **Error Handling**:
   - Provides detailed error messages with context
   - Implements proper cleanup of temporary files
   - Handles edge cases like single-snapshot input

8. **Scalability**:
   - Can process very large snapshots by keeping most data on disk rather than in memory
   - Uses 256 buckets to distribute records evenly
   - Supports efficient processing of millions of records

#### 6. Main Combining Function

```go
func combineSnapshots(inputPaths []string, outputPath string) error {
    // Validate input
    if len(inputPaths) == 0 {
        return fmt.Errorf("no input snapshots provided")
    }
    
    // Special case: if only one snapshot, just copy it
    if len(inputPaths) == 1 {
        fmt.Printf("Only one snapshot provided, copying %s to %s\n", inputPaths[0], outputPath)
        return copySingleSnapshot(inputPaths[0], outputPath)
    }
    
    fmt.Printf("Combining %d snapshots into %s\n", len(inputPaths), outputPath)
    
    // 1. Create indices for all input snapshots
    fmt.Println("Indexing input snapshots...")
    indices, err := indexAllSnapshots(inputPaths)
    if err != nil {
        return fmt.Errorf("failed to index snapshots: %w", err)
    }
    
    // 2. Process accounts in batches
    fmt.Println("Processing accounts in batches...")
    err = processBatchedAccounts(indices)
    if err != nil {
        return fmt.Errorf("failed to process accounts: %w", err)
    }
    
    // 3. Sort and merge all bucket files into the final snapshot
    fmt.Println("Sorting and merging buckets...")
    err = sortAndMergeBuckets(outputPath)
    if err != nil {
        return fmt.Errorf("failed to sort and merge buckets: %w", err)
    }
    
    fmt.Println("Snapshot combining completed successfully")
    return nil
}

// Copy a single snapshot file (for the case where only one input is provided)
func copySingleSnapshot(inputPath, outputPath string) error {
    // Open source file
    src, err := os.Open(inputPath)
    if err != nil {
        return fmt.Errorf("failed to open source snapshot: %w", err)
    }
    defer src.Close()
    
    // Create destination file
    dst, err := os.Create(outputPath)
    if err != nil {
        return fmt.Errorf("failed to create destination file: %w", err)
    }
    defer dst.Close()
    
    // Copy content
    _, err = io.Copy(dst, src)
    if err != nil {
        return fmt.Errorf("failed to copy snapshot: %w", err)
    }
    
    return nil
}
```

## Implementation Plan for Replacing `snap_combine`

This section outlines the step-by-step approach to replace the current `snap_combine` implementation with our new memory-efficient, file-backed bucket sorting algorithm. Each step is designed to be independently testable to ensure robustness and correctness throughout the development process.

### Phase 1: Setup and Command Structure

#### Step 1: Remove Current Implementation
- Remove the entire current implementation of `snap_combine`
- Keep the command registration in the command hierarchy
- Create a stub implementation that returns "not implemented yet"

#### Step 2: Update Command Parameters
- Update the command help text to describe the new memory-efficient approach
- Define command-line parameters:
  - Input snapshots (multiple paths)
  - Output snapshot path
  - Optional batch size parameter (default: 10,000)
  - Optional bucket count parameter (default: 256)
  - Verbose flag for detailed progress reporting

#### Step 3: Create Basic Structure
- Implement the main command function that parses parameters
- Add validation for input parameters
- Implement the special case for single snapshot input (direct copy)

### Phase 2: Core Algorithm Components

#### Step 4: Implement Snapshot Indexing
- Create the `SnapshotIndex` structure
- Implement `indexAllSnapshots` function to build indices for all input snapshots
- Add tests to verify correct index creation

#### Step 5: Implement Bucket Management
- Create bucket file initialization and cleanup functions
- Implement bucket file management (creation, writing, reading)
- Add tests to verify bucket file operations

#### Step 6: Implement Batch Processing
- Create the batch processing logic for accounts
- Implement record identification and extraction
- Add tests for batch processing with small datasets

### Phase 3: Record Processing and References

#### Step 7: Implement Reference Extraction
- Implement transaction reference extraction from account records
- Implement URL reference extraction from account records
- Add tests to verify reference extraction

#### Step 8: Implement Record Writing
- Implement functions to write records to bucket files
- Implement functions to read records from source snapshots
- Add tests to verify record writing and reading

### Phase 4: Sorting and Final Assembly

#### Step 9: Implement Bucket Sorting
- Implement the sorting logic for each bucket
- Ensure proper descending order for key hashes
- Add tests to verify sorting correctness

#### Step 10: Implement Final Snapshot Assembly
- Implement the logic to merge sorted buckets into the final snapshot
- Create record index section with proper ordering
- Add tests to verify final snapshot structure

### Phase 5: Integration and Optimization

#### Step 11: Add Progress Reporting
- Implement detailed progress reporting
- Add memory usage monitoring
- Add estimated time remaining calculations

#### Step 12: Optimize Performance
- Add buffered I/O for better performance
- Implement parallel processing where possible
- Benchmark and optimize critical sections

#### Step 13: Full Integration Testing
- Test with various snapshot sizes (small, medium, large)
- Verify memory usage remains bounded
- Ensure idempotency and correctness of output

### Phase 6: Documentation and Finalization

#### Step 14: Update Documentation
- Update command help text with final parameters
- Document performance characteristics and memory usage
- Add examples for common use cases

#### Step 15: Final Review and Release
- Conduct code review
- Perform final testing with production-sized snapshots
- Remove any debug code and finalize implementation

#### Helper Functions

```go
// Helper function to read a record from a specific location in a snapshot file
func readRecordFromLocation(file *os.File, loc RecordLocation) *snapshot.RecordEntry {
    // Navigate to the appropriate section
    scanner := snapshot.NewScanner(file)
    currentSection := uint32(0)
    
    for scanner.Scan() {
        section := scanner.Section()
        
        // Skip non-Records sections
        if section.Type != snapshot.SectionTypeRecords {
            continue
        }
        
        // Check if this is the section we want
        if currentSection == loc.SectionNumber {
            // Navigate to the record within the section
            reader := snapshot.NewRecordReader(section)
            currentOffset := uint64(0)
            
            for reader.Next() {
                if currentOffset == loc.Offset {
                    // Found the record
                    return reader.Record()
                }
                
                // Move to next record
                currentOffset += uint64(reader.Record().EncodedSize())
                
                // If we've gone past the offset, the record doesn't exist
                if currentOffset > loc.Offset {
                    return nil
                }
            }
            
            return nil // Record not found in this section
        }
        
        currentSection++
    }
    
    return nil // Section not found
}

// Helper function to read a URL from a specific location in a snapshot file
func readUrlFromLocation(file *os.File, loc RecordLocation) *snapshot.Url {
    // Navigate to the appropriate section
    scanner := snapshot.NewScanner(file)
    currentSection := uint32(0)
    
    for scanner.Scan() {
        section := scanner.Section()
        
        // Skip non-URL sections
        if section.Type != snapshot.SectionTypeUrls {
            continue
        }
        
        // Check if this is the section we want
        if currentSection == loc.SectionNumber {
            // Navigate to the URL within the section
            reader := snapshot.NewUrlReader(section)
            currentOffset := uint64(0)
            
            for reader.Next() {
                if currentOffset == loc.Offset {
                    // Found the URL
                    return reader.Url()
                }
                
                // Move to next URL
                // Note: URL size calculation depends on implementation
                // This is a simplified approach
                currentOffset = reader.Offset()
            }
            
            return nil // URL not found in this section
        }
        
        currentSection++
    }
    
    return nil // Section not found
}

// Helper function to merge files into a section
func mergeFilesIntoSection(output *os.File, files []string, sectionType snapshot.SectionType) error {
    // Calculate total size of the section
    var totalSize uint64
    for _, file := range files {
        info, err := os.Stat(file)
        if err != nil {
            return err
        }
        totalSize += uint64(info.Size())
    }
    
    // Write section header
    binary.Write(output, binary.LittleEndian, uint32(sectionType))
    binary.Write(output, binary.LittleEndian, totalSize)
    
    // Copy content from each file
    for _, file := range files {
        src, err := os.Open(file)
        if err != nil {
            return err
        }
        
        _, err = io.Copy(output, src)
        src.Close()
        
        if err != nil {
            return err
        }
    }
    
    return nil
}
```

### Section Handling During Combining

When combining snapshots, different section types are handled as follows:

1. **Header Sections (Type 1)**:
   - Only one header section is retained in the final output.
   - The header from the first snapshot is typically used, but its root hash may be recalculated.

2. **Records Sections (Type 7)**:
   - Records from all input snapshots are processed and selectively included based on filtering criteria.
   - Records are not directly copied to the output; instead, they are processed and written to a database.

3. **Record Index Sections (Type 8)**:
   - Record index sections are not copied from input snapshots.
   - A new record index is generated for the combined output if needed.

4. **BPT Sections (Type 11)**:
   - BPT sections are typically not copied from input snapshots.
   - A new BPT may be calculated for the combined output if the `calculateBPT` flag is set.
   - For Genesis block creation, the BPT section is often omitted entirely as it can be reconstructed from the records.

5. **Consensus Sections (Type 9)**:
   - Consensus parameters may be selectively included or modified during the combining process.
   - System-specific consensus data may be stripped when creating a genesis snapshot.

### Step-by-Step Combining Process

#### 1. Prepare the Output Database

```go
// Open or create the output database
db := database.New(store, nil)
```

#### 2. Process Each Input Snapshot

For each snapshot file:

```go
// Open the snapshot file
file, err := os.Open(snapshotPath)
if err != nil {
    return err
}
defer file.Close()

// Read the snapshot header
header, err := snapshot.ReadHeader(file)
if err != nil {
    return err
}

// Verify snapshot version
if header.Version != 2 {
    return errors.New("unsupported snapshot version")
}
```

#### 3. First Pass: Extract Accounts

```go
// Create a map to track processed accounts
accounts := map[[32]byte]*AccountData{}

// Restore accounts (first pass)
err := Restore(db, file, &RestoreOptions{
    BatchRecordLimit: 50_000,
    SkipHashCheck:    true,
    Predicate: func(e *snapshot.RecordEntry) (bool, error) {
        // Process only Account records in the first pass
        if e.Key.Get(0) != "Account" {
            return false, nil
        }
        
        // Get the account URL
        u := e.Key.Get(1).(*url.URL)
        
        // Skip system accounts if needed
        if isSystemAccount(u) {
            return false, nil
        }
        
        // Process account data based on record type
        switch e.Key.Get(2) {
        case "Main":
            // Extract and store the main account state
            acct, _ := UnmarshalAccount(e.Value)
            accounts[u.AccountID32()].Main = acct
            
        case "MainChain", "ScratchChain":
            // Process chain states
            if e.Key.Get(3) == "States" || e.Key.Get(3) == "Head" {
                // Extract and store chain states
                state := new(merkle.State)
                err := state.UnmarshalBinary(e.Value)
                if err != nil {
                    return false, err
                }
                
                // Store the state
                chainName := e.Key.Get(2).(string)
                accounts[u.AccountID32()].States[chainName] = 
                    append(accounts[u.AccountID32()].States[chainName], state)
            }
        }
        
        return true, nil
    },
})
```

#### 4. Collect Transaction Hashes

```go
// Extract transaction hashes from account states
hashes := map[[32]byte]bool{}
for _, accountData := range accounts {
    for _, states := range accountData.States {
        for _, state := range states {
            // Collect all transaction hashes from the state
            for _, h := range state.HashList {
                hashes[*(*[32]byte)(h)] = true
            }
        }
    }
}
```

#### 5. Second Pass: Extract Transactions

```go
// Reset file pointer to beginning
file.Seek(0, io.SeekStart)

// Restore transactions (second pass)
err = Restore(db, file, &RestoreOptions{
    BatchRecordLimit: 50_000,
    SkipHashCheck:    true,
    Predicate: func(e *snapshot.RecordEntry) (bool, error) {
        // Process only Transaction and Message records
        if e.Key.Get(0) != "Transaction" && e.Key.Get(0) != "Message" {
            return false, nil
        }
        
        // Only keep transactions referenced by accounts
        h := e.Key.Get(1).([32]byte)
        return hashes[h], nil
    },
})
```

#### 6. Repeat for All Input Snapshots

Repeat steps 2-5 for each input snapshot file.

#### 7. Finalize the Combined Database

```go
// Note: By default, we skip BPT calculation for Genesis block creation
// const calculateBPT = false

// BPT calculation is skipped to optimize memory usage and processing time
// The BPT can be reconstructed on-demand when the snapshot is loaded

// If BPT calculation is needed (non-Genesis use cases):
// if calculateBPT {
//     db.SetObserver(&bpt.Observer{Store: store})
// }

// Commit the final database
db.Close()
```

### BPT Handling in Genesis Block Creation

When creating a Genesis block by combining snapshots, special consideration is given to the Binary Patricia Tree (BPT) section:

#### Omitting the BPT Section

In the Genesis block creation process (`debug genesis ingest`), the BPT section is typically omitted entirely. This is controlled by the `calculateBPT` flag, which is set to `false` by default in the Genesis creation code:

```go
const calculateBPT = false

// Don't calculate BPT hashes since genesis doesn't want them
db.SetObserver(testing.NullObserver{})
```

#### Database Structure Without BPT

Even without calculating BPT hashes, the Genesis ingest process:

1. Creates an in-memory database that maintains internal key-value structures
2. Preserves the order of accounts as they are added from each input snapshot
3. Allows iteration over accounts using the database's internal organization
4. Collects and sorts message records by hash across all input snapshots

```go
const calculateBPT = false
```

Omitting the BPT is possible because:

1. **On-Demand Reconstruction**: The BPT can be fully reconstructed from the record data when needed.

2. **Initial State Optimization**: For a Genesis block, there's no need to preserve the historical BPT state from the source snapshots.

3. **Memory Efficiency**: Omitting the BPT calculation significantly reduces the memory requirements during Genesis creation.

4. **Clean Slate Approach**: Starting with a fresh BPT ensures there are no inconsistencies carried over from the source snapshots.

#### BPT Reconstruction Process

When a node loads a Genesis snapshot without a BPT section, it will:

1. Read all records from the Records sections
2. Insert each record into a new BPT structure
3. Calculate the BPT root hash
4. Verify the root hash against the header's root hash (if provided)

This reconstruction process happens automatically when the snapshot is loaded, ensuring that the BPT is available for subsequent operations without needing to store it in the snapshot file.

#### When to Include a BPT Section

Including a BPT section may be beneficial in non-Genesis snapshots when:

1. **Performance Optimization**: Pre-calculating the BPT can speed up snapshot loading.
2. **Verification**: Including the BPT allows for additional integrity checks.
3. **Specialized Use Cases**: Some analysis tools may require the BPT structure directly.

To include a BPT section when combining snapshots, set the `calculateBPT` flag to `true` and provide a proper observer implementation:

```go
if calculateBPT {
    // Set up a proper observer for BPT calculation
    db.SetObserver(&bpt.Observer{Store: store})
}
```

### Conflict Resolution

When combining snapshots, conflicts may occur when the same record exists in multiple snapshots. The algorithm resolves conflicts as follows:

1. **Last Write Wins**: When the same record exists in multiple snapshots, the last processed snapshot's version is kept.

2. **Selective Inclusion**: The algorithm can selectively include or exclude records based on predicates, allowing for fine-grained control over which records are included in the combined output.

3. **Transaction Consistency**: By extracting transactions in a second pass and only including those referenced by account states, the algorithm ensures transaction consistency in the combined output.

This combining algorithm is memory-efficient as it processes snapshots sequentially and only keeps necessary data structures in memory. The two-pass approach (accounts first, then transactions) ensures that only relevant transactions are included in the final database.

### Idempotency in Snapshot Combining

An important property of the snapshot combining algorithm is how it handles the special case of combining a single snapshot. Let's examine whether combining a single snapshot would produce an identical output.

#### Record Ordering Considerations

After examining the code, there are several factors that affect record ordering in snapshots:

1. **Record Index Ordering**: The `Indexer.Write` method enforces that keys in the record index must be in **descending** order. The code specifically checks for this with:
   ```go
   // Keys must be in descending order
   c := bytes.Compare(i.last[:], e.Key[:])
   // ...
   case c > 0:
       return errors.BadRequest.WithFormat("keys must be sorted in descending order")
   ```

2. **Collection Process**: Records are collected via the `Collector.Collect` method, which walks through database records based on a predicate and writes them to the snapshot. The order of records in the output depends on:
   - The order in which records are provided to the collector
   - Any filtering applied by the predicate function
   - The database's internal traversal order

3. **Batch Processing**: Our batch processing algorithm processes accounts in batches of 10,000, which could potentially reorder records if the original snapshot wasn't created with the same batching strategy.

#### When Combining a Single Snapshot

Given these considerations, when combining a single snapshot:

1. **Record Content Preservation**: All record contents will be preserved exactly as they were in the original snapshot.

2. **Order May Change**: The order of records might change if:
   - The original snapshot wasn't created with records in descending key order
   - The batch processing algorithm traverses records in a different order than the original snapshot creation process
   - Different predicates or filters are applied during combining

3. **Section Structure Changes**: The section structure will definitely change:
   - The Record Index section will be regenerated
   - Multiple Records sections might be consolidated or split differently
   - The BPT section is typically omitted during combining, as noted earlier

4. **Binary Non-Equivalence**: Due to these ordering and structural changes, the binary representation of the combined snapshot will likely differ from the original, even though the logical content remains the same.

#### Verifying Logical Equivalence

To verify that a combined snapshot is logically equivalent to the original, you can:

1. Compare the root hash in the header section
2. Count the total number of records in each snapshot
3. Sample key records and compare their values

The combining process should be considered idempotent in terms of the logical content represented by the snapshot, but not in terms of the exact binary representation or record ordering.

## Receipt Format Details

The `Receipt` field in the `RecordEntry` struct represents a Merkle proof that verifies the record's inclusion in the BPT (Binary Patricia Tree). When present, it is encoded as follows:

### Binary Format of Receipt

1. **Length Prefix**: A varint encoding the total length of the receipt data in bytes.

2. **Receipt Structure**:
   - **Start**: The starting hash (32 bytes)
   - **End**: The ending hash (32 bytes)
   - **Anchor**: The anchor hash (32 bytes)
   - **Entries**: A length-prefixed array of receipt entries, each containing:
     - **Hash**: A 32-byte hash value
     - **Right**: A boolean flag (1 byte, 0 = left, 1 = right)

### Example Receipt Encoding

```
# Receipt with 2 entries (length prefix: 103 bytes)
67

# Start hash (32 bytes)
01 02 03 04 05 06 07 08 09 0A 0B 0C 0D 0E 0F 10 11 12 13 14 15 16 17 18 19 1A 1B 1C 1D 1E 1F 20

# End hash (32 bytes)
A1 A2 A3 A4 A5 A6 A7 A8 A9 AA AB AC AD AE AF B0 B1 B2 B3 B4 B5 B6 B7 B8 B9 BA BB BC BD BE BF C0

# Anchor hash (32 bytes)
F1 F2 F3 F4 F5 F6 F7 F8 F9 FA FB FC FD FE FF 00 01 02 03 04 05 06 07 08 09 0A 0B 0C 0D 0E 0F 10

# Entries count (varint: 2)
02

# Entry 1: Hash (32 bytes) + Right flag (1 byte)
D1 D2 D3 D4 D5 D6 D7 D8 D9 DA DB DC DD DE DF E0 E1 E2 E3 E4 E5 E6 E7 E8 E9 EA EB EC ED EE EF F0 00

# Entry 2: Hash (32 bytes) + Right flag (1 byte)
B1 B2 B3 B4 B5 B6 B7 B8 B9 BA BB BC BD BE BF C0 C1 C2 C3 C4 C5 C6 C7 C8 C9 CA CB CC CD CE CF D0 01
```

## BPT Section Format

The BPT (Binary Patricia Tree) section (type 11) stores the state of the Binary Patricia Tree used for efficient record lookups. The binary format is as follows:

### BPT Section Header

```
# Section type (2 bytes, big-endian): 11 (SectionTypeBPT)
00 0B

# Reserved (6 bytes)
00 00 00 00 00 00

# Section size (8 bytes, big-endian)
00 00 00 00 00 00 XX XX

# Next section offset (8 bytes, big-endian)
00 00 00 00 00 00 YY YY

# Additional metadata (40 bytes)
00 ... 00
```

### BPT Node Format

Each BPT node is encoded as:

1. **Node Length**: A varint encoding the total length of the node in bytes.

2. **Node Type**: A single byte indicating the node type:
   - 0x01: Branch node
   - 0x02: Leaf node

3. **Node Data**: Based on the node type:
   - For Branch nodes:
     - Key fragment (length-prefixed)
     - Left child reference (32 bytes or empty)
     - Right child reference (32 bytes or empty)
   - For Leaf nodes:
     - Key (length-prefixed)
     - Value hash (32 bytes)

## Consensus Section Format

The Consensus section (type 9) stores consensus parameters. The binary format is:

### Consensus Section Header

```
# Section type (2 bytes, big-endian): 10 (SectionTypeConsensus)
00 0A

# Reserved (6 bytes)
00 00 00 00 00 00

# Section size (8 bytes, big-endian)
00 00 00 00 00 00 XX XX

# Next section offset (8 bytes, big-endian)
00 00 00 00 00 00 YY YY

# Additional metadata (40 bytes)
00 ... 00
```

### Consensus Data Format

The consensus data is encoded as a length-prefixed Protocol Buffers message containing:

1. **Network Type**: A varint encoding the network type (mainnet, testnet, etc.)
2. **Network ID**: A length-prefixed string
3. **Partition ID**: A length-prefixed string
4. **Validators**: A length-prefixed array of validator entries
5. **Parameters**: Various consensus parameters encoded as Protocol Buffer fields

## Custom Snapshot Parsing Implementation

This section provides a practical example of how to implement custom parsing of snapshot files without relying on the official snapshot package. This approach is useful when you need more control over memory usage or when working with potentially incompatible snapshot formats.

### Memory-Efficient Parsing Approach

The key to memory-efficient snapshot parsing is to:

1. Process the file in a streaming fashion, reading only what's needed at any given time
2. Avoid loading the entire snapshot into memory
3. Use temporary storage for large sections when necessary
4. Process records incrementally rather than all at once

### Example: Custom Snapshot Parser in Go

```go
// SectionHeader represents a snapshot section header
type SectionHeader struct {
	Type            uint16 // Section type (2 bytes)
	Size            uint64 // Section size (8 bytes)
	NextOffset      uint64 // Offset to next section (8 bytes)
	HeaderOffset    int64  // Offset of this header in the file
	ContentOffset   int64  // Offset of the section content (header + 64)
}

// readSectionHeader reads a section header from the current position in the file
func readSectionHeader(file *os.File) (*SectionHeader, error) {
	// Get current position
	currentPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, fmt.Errorf("failed to get current position: %w", err)
	}
	
	// Read the 64-byte header
	headerBytes := make([]byte, 64)
	n, err := io.ReadFull(file, headerBytes)
	if err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			// End of file reached
			return nil, io.EOF
		}
		return nil, fmt.Errorf("failed to read section header: %w", err)
	}
	if n != 64 {
		return nil, fmt.Errorf("incomplete section header: read %d bytes, expected 64", n)
	}
	
	// Parse the header fields
	header := &SectionHeader{
		Type:           binary.BigEndian.Uint16(headerBytes[0:2]),
		Size:           binary.BigEndian.Uint64(headerBytes[8:16]),
		NextOffset:     binary.BigEndian.Uint64(headerBytes[16:24]),
		HeaderOffset:   currentPos,
		ContentOffset:  currentPos + 64,
	}
	
	return header, nil
}

// parseSnapshot reads all sections from the snapshot file
func parseSnapshot(filePath string) error {
	// Open the snapshot file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open snapshot file: %w", err)
	}
	defer file.Close()

	// Read sections until EOF or until nextOffset is 0
	var currentOffset int64 = 0
	for {
		// Seek to the current section header
		_, err := file.Seek(currentOffset, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to seek to section at offset %d: %w", currentOffset, err)
		}

		// Read section header (64 bytes)
		header, err := readSectionHeader(file)
		if err != nil {
			if err == io.EOF {
				// End of file reached
				break
			}
			return fmt.Errorf("failed to read section header at offset %d: %w", currentOffset, err)
		}

		// Process the section based on its type
		switch header.Type {
		case 1: // SectionTypeHeader
			// Read the header content (which follows the 64-byte section header)
			headerContent := make([]byte, header.Size)
			_, err = io.ReadFull(file, headerContent)
			if err != nil {
				return fmt.Errorf("failed to read header content: %w", err)
			}
			
			// The header content should contain the format version as a uint32 in big-endian format
			if len(headerContent) < 4 {
				return fmt.Errorf("header content too small: %d bytes", len(headerContent))
			}
			
			// Parse the format version (first 4 bytes of header content, big-endian)
			formatVersion := binary.BigEndian.Uint32(headerContent[0:4])
			fmt.Printf("Snapshot format version: %d\n", formatVersion)
			
		case 7: // SectionTypeRecords
			// Process records section
			fmt.Printf("Records section: %d bytes at offset %d\n", header.Size, header.ContentOffset)
			
			// For large sections, you might want to process the data in chunks
			// rather than loading it all into memory at once
			processRecordsSection(file, header)
			
		case 8: // SectionTypeRecordIndex
			// Process record index section
			fmt.Printf("Record index section: %d bytes at offset %d\n", header.Size, header.ContentOffset)
			
			// Process the record index entries
			processRecordIndexSection(file, header)
			
		case 11: // SectionTypeBPT
			// Process BPT section
			fmt.Printf("BPT section: %d bytes at offset %d\n", header.Size, header.ContentOffset)
			
			// Process the BPT nodes
			processBPTSection(file, header)
			
		default:
			// Skip unknown section types
			fmt.Printf("Skipping unknown section type %d: %d bytes at offset %d\n", 
				header.Type, header.Size, header.ContentOffset)
		}

		// Move to the next section
		if header.NextOffset == 0 {
			// No more sections
			break
		}
		currentOffset = int64(header.NextOffset)
	}

	return nil
}
```

### URL Hash Handling for Memory Efficiency

For efficient URL hash handling without loading all URLs into memory:

1. Store URLs in both a key-value database for fast lookups and the binary file format for iteration
2. Perform lookups primarily from the KV database (fast)
3. Fall back to file-based lookup if needed
4. Avoid memory-loading approaches that could cause memory issues with large datasets

This hybrid approach maintains both performance and memory efficiency when working with large snapshots.

## Conclusion

This document provides a comprehensive specification of the Accumulate version 2 snapshot format. It details the structure, record types, and implementation considerations for working with version 2 snapshots. By following these guidelines and referencing the provided code examples, developers can efficiently create, read, and combine snapshots while minimizing memory usage.

The version 2 snapshot format offers significant improvements in memory efficiency through features like record indexing, streaming processing, and optimized URL hash handling. These features make it possible to work with large snapshots without excessive memory consumption, enabling scalable snapshot processing tools.
