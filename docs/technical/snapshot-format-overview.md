# Accumulate Snapshot Format - Overview

This document provides a high-level overview of the Accumulate version 2 snapshot format and serves as an entry point to the detailed technical documentation.

## What are Accumulate Snapshots?

Accumulate snapshots are binary files that capture the complete state of the Accumulate network at a specific point in time. They contain:

- **Account States**: All account data and balances
- **Transaction History**: Complete transaction records
- **Network Configuration**: Consensus parameters and network settings
- **Binary Patricia Tree (BPT)**: Merkle tree structure for cryptographic proofs
- **Indexes**: Efficient lookup structures for fast data access

## Snapshot Format Version 2

The current snapshot format (version 2) is implemented using Go structs with automatic binary marshaling. Key characteristics:

- **Binary Format**: Efficient storage and fast loading
- **Sectioned Structure**: Organized into typed sections for different data types
- **Indexed Access**: Random access to records without loading entire snapshots
- **Merkle Proofs**: Cryptographic verification of data integrity
- **Streaming Support**: Process large snapshots without loading everything into memory

## Documentation Structure

The snapshot format documentation is organized into focused documents:

### 📋 **Core Documentation**
- **[Snapshot Format Overview](snapshot-format-overview.md)** *(this document)* - High-level introduction
- **[Snapshot Data Structures](snapshot-format-structures.md)** - Go struct definitions and type reference
- **[Snapshot Sections](snapshot-format-sections.md)** - Section types, encoding, and organization
- **[Snapshot Operations](snapshot-format-operations.md)** - Reading, writing, and processing operations
- **[Snapshot Combining](snapshot-format-combining.md)** - Algorithms for merging multiple snapshots

### 🔗 **Related Documentation**
- **[Genesis Format](genesis-format.md)** - Genesis block creation from snapshots
- **[Record Format](record-format.md)** - Individual record structure and encoding
- **[BPT Restoration Design](bpt-restoration-design.md)** - Binary Patricia Tree restoration strategies

## Key Concepts

### Section-Based Architecture
Snapshots are organized into sections, each with a specific purpose:
- **Header Section**: Metadata and root hash
- **Records Section**: Account and transaction data
- **BPT Section**: Merkle tree entries
- **Index Section**: Fast lookup structures
- **Consensus Section**: Network consensus parameters

### Record Types
Different types of records are stored with specific key formats:
- **Account Records**: `Account/{url}`
- **Chain Records**: `Chain/{url}/{chain-type}/{element-type}/{element-id}`
- **Transaction Records**: `Transaction/{txid}`
- **Directory Records**: `Directory/{url-prefix}`
- **BPT Records**: `BPT/{key-hash}`

### Processing Approaches
- **Streaming**: Process records one at a time for memory efficiency
- **Indexed Access**: Use record indexes for random access
- **Batch Processing**: Handle large datasets in manageable chunks
- **Parallel Processing**: Utilize multiple cores for performance

## Developer Guidelines

### Working with Snapshots
1. **Always use provided Go structs** - Don't handle binary data directly
2. **Use streaming for large snapshots** - Avoid loading entire files into memory
3. **Leverage indexing** - Use record indexes for efficient random access
4. **Follow key formats** - Use standard key naming conventions
5. **Validate data integrity** - Use Merkle proofs for verification

### Performance Considerations
- **Memory Usage**: Use streaming techniques for large files
- **I/O Efficiency**: Implement buffered reading/writing
- **Parallel Processing**: Process independent sections concurrently
- **Index Utilization**: Use indexes to avoid full scans

### Common Use Cases
- **Network Bootstrap**: Loading initial network state
- **State Synchronization**: Syncing node state with network
- **Backup and Recovery**: Creating and restoring network backups
- **Genesis Creation**: Building genesis blocks from existing state
- **Development Testing**: Creating test networks with known state

## Implementation Files

Key implementation files in the Accumulate codebase:
- `pkg/database/snapshot/types_gen.go` - Core data structures
- `pkg/database/snapshot/reader.go` - Snapshot reading operations
- `pkg/database/snapshot/writer.go` - Snapshot writing operations
- `pkg/types/record/key.go` - Record key definitions
- `pkg/database/merkle/types_gen.go` - Merkle proof structures

## Next Steps

For detailed technical information, proceed to:
1. **[Snapshot Data Structures](snapshot-format-structures.md)** - Understand the Go structs and types
2. **[Snapshot Sections](snapshot-format-sections.md)** - Learn about section organization
3. **[Snapshot Operations](snapshot-format-operations.md)** - Implement reading and writing
4. **[Snapshot Combining](snapshot-format-combining.md)** - Merge multiple snapshots

---

*This overview provides the foundation for understanding Accumulate's snapshot format. Each linked document provides detailed technical information for specific aspects of the format.*
