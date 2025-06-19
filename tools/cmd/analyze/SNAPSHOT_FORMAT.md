# Accumulate Snapshot Format Documentation

This document provides a comprehensive overview of the Accumulate snapshot format, structure, and implementation details to guide the development of snapshot processing tools like `snap-combine`.

## Snapshot File Structure

A snapshot file consists of multiple sections, each with a specific type and purpose. The file format uses a segmented approach where each section has a type identifier, size, and content.

### Section Types

| Section Type | Value | Description |
|-------------|-------|-------------|
| `SectionTypeHeader` | 1 | Contains metadata about the snapshot |
| `SectionTypeAccountsV1` | 2 | Contains accounts (v1 format - legacy) |
| `SectionTypeTransactionsV1` | 3 | Contains transactions (v1 format - legacy) |
| `SectionTypeSignaturesV1` | 4 | Contains signatures (v1 format - legacy) |
| `SectionTypeGzTransactionsV1` | 5 | Contains gzipped transactions (v1 format - legacy) |
| `SectionTypeSnapshot` | 6 | Contains nested snapshots |
| `SectionTypeRecords` | 7 | Contains records stored as (key, record) pairs |
| `SectionTypeRecordIndex` | 8 | Indexes record keys, including offset and section number |
| `SectionTypeRawBPT` | 9 | Contains the BPT as raw (key hash, value) pairs (deprecated) |
| `SectionTypeBPT` | 10 | Contains the Binary Patricia Tree as records |
| `SectionTypeConsensus` | 10 | Contains consensus parameters |

### File Format

1. **Header Section** (Required):
   - Always the first section in a snapshot file
   - Contains version information and metadata

2. **Record Sections**:
   - Contains the actual data records
   - Modern snapshots primarily use `SectionTypeRecords`

3. **Optional Sections**:
   - BPT sections for database tree structure
   - Record index for faster lookups
   - Consensus parameters

## Record Structure

Records in the `SectionTypeRecords` section follow a specific structure:

### Record Entry

Each record entry consists of:
- **Key**: A hierarchical path represented as a series of components
- **Value**: Binary encoded data representing the record content

### Key Path Structure

The key path follows a hierarchical structure where the first component typically indicates the record type:

1. **Account Records**:
   - Format: `Account/<URL>/<ChainID>`
   - Example: `Account/acc://example.acme/main`

2. **Transaction Records**:
   - Format: `Transaction/<TxID>`
   - Example: `Transaction/0123456789abcdef0123456789abcdef`

3. **Chain Records**:
   - Format: `Chain/<URL>/<ChainID>/<Index>`
   - Example: `Chain/acc://example.acme/main/1`

## Snapshot Creation Process

The `debug snap collect` command creates snapshots using the following process:

1. **Initialize**:
   - Create a new snapshot file with a header section
   - Set up the snapshot writer

2. **Collect Records**:
   - Query the database for all accounts
   - For each account:
     - Add the account record
     - Add records for all chains associated with the account
     - Add transaction records referenced by the account's chains

3. **Build BPT** (Optional):
   - Add Binary Patricia Tree records for database structure
   - Can be skipped with `--skip-bpt` flag

4. **Finalize**:
   - Write all sections to the snapshot file
   - Close the file

## Snapshot Reading Process

When reading a snapshot file:

1. **Verify Version**:
   - Check that the snapshot version is supported (currently version 2)
   - Validate the header section

2. **Process Sections**:
   - Iterate through each section in the file
   - For `SectionTypeRecords` sections:
     - Read each record entry
     - Process the record based on its key path structure

3. **Extract Metadata**:
   - Parse record keys to determine record types, account URLs, and chain IDs
   - Build indexes for efficient lookups

## Memory Optimization for Snapshot Processing

When processing large snapshots, memory usage is a critical concern. The following strategies can be employed:

### 1. Hash-Based Key Storage

Since the BlockchainDB KV2 database only handles hashes as keys:
- Use SHA-256 hashes of key paths as database keys
- Store the original key paths in a separate lookup structure

### 2. URL Hash Mapping

For account URLs:
- Hash account URLs to use as keys in the database
- Maintain a mapping between URL hashes and original URLs
- This allows efficient storage and retrieval of account data

### 3. Temporary File Storage

Instead of keeping all record keys in memory:
- Write record keys and metadata to temporary files
- Use a CSV-like format for easy parsing
- Read the files sequentially when processing records

### 4. Streaming Processing

Process records in a streaming fashion:
- Read records from input snapshots one at a time
- Write records to the output snapshot as they're processed
- Avoid loading all records into memory simultaneously

## Combining Snapshots

When combining multiple snapshots, the following approach is recommended:

1. **Read All Snapshots**:
   - Process each input snapshot sequentially
   - Store all records in a temporary database
   - Track record keys and metadata in temporary files

2. **Deduplicate Records**:
   - Use record key hashes to identify duplicate records
   - Keep only the latest version of each record

3. **Write Combined Snapshot**:
   - Create a new snapshot file
   - Write a header section
   - Stream records from the temporary database to the output file
   - Organize records by type and account for better structure

4. **Clean Up**:
   - Close and remove all temporary files
   - Close the temporary database

## Implementation Notes

### Key Considerations

1. **Version Compatibility**:
   - Only version 2 snapshots are fully supported
   - Legacy sections from version 1 may be present but are not used in modern processing

2. **Record Ordering**:
   - When writing a combined snapshot, consider ordering records logically
   - Group records by account and chain for better organization

3. **Error Handling**:
   - Implement robust error handling for malformed records
   - Log warnings for non-critical issues but continue processing

4. **Progress Reporting**:
   - For large snapshots, provide regular progress updates
   - Report statistics on records processed and written

### Database Interface

The BlockchainDB KV2 database interface requires:
- Keys must be 32-byte hashes (`[32]byte`)
- Values are arbitrary byte slices (`[]byte`)

To work with this constraint:
- Hash string keys (like URLs and key paths) using SHA-256
- Store the mapping between hashes and original strings if needed

## Conclusion

This document provides a comprehensive reference for understanding and working with Accumulate snapshots. By following these guidelines, tools like `snap-combine` can efficiently process and combine snapshots while minimizing memory usage.
