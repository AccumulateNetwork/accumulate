# Snap Combine Algorithm Design

This document outlines the design of the new memory-efficient, file-backed bucket sorting algorithm for the `snap-combine` command. The algorithm is designed to combine multiple snapshot files into a single consolidated snapshot file while minimizing memory usage.

## File Organization

The implementation is split across multiple files to improve modularity, maintainability, and readability:

1. **snap_combine.go** - Command registration and entry point
2. **record_types.go** - Core data structures and types
3. **bucket_manager.go** - Bucket file management and record distribution
4. **snap_combiner.go** - Main algorithm implementation

## Data Structures

### In `record_types.go`:

- **SnapshotIndex** - Represents a record's location in a snapshot file
  ```go
  type SnapshotIndex struct {
      Offset int64 // Byte offset in the file
      Size   int   // Size of the record in bytes
  }
  ```

- **RecordLocation** - Represents a record's location and metadata
  ```go
  type RecordLocation struct {
      Key           []byte // The record key
      KeyHash       []byte // Hash of the key for sorting
      SnapshotIndex int    // Index of the snapshot in the input list
      Offset        int64  // Byte offset in the snapshot file
      Size          int    // Size of the record in bytes
      Type          string // Record type
  }
  ```

- **CombineStats** - Tracks statistics about the snapshot combining process
  ```go
  type CombineStats struct {
      InputSnapshots   int
      RecordsRead      int
      RecordsWritten   int
      RecordsByType    map[string]int
      DuplicateKeys    int
      BatchesProcessed int
  }
  ```

### In `bucket_manager.go`:

- **BucketFile** - Represents a temporary file used for sorting records by key hash
  ```go
  type BucketFile struct {
      File        *os.File
      Path        string
      RecordCount int
      BucketID    int
  }
  ```

- **BucketManager** - Handles the creation, writing, and reading of bucket files
  ```go
  type BucketManager struct {
      Buckets     []*BucketFile
      NumBuckets  int
      TempDir     string
      RecordCount int
  }
  ```

### In `snap_combiner.go`:

- **SnapCombineConfig** - Configuration for the snapshot combining process
  ```go
  type SnapCombineConfig struct {
      BatchSize  int  // Number of records to process in a batch
      NumBuckets int  // Number of bucket files to use for sorting
      Verbose    bool // Whether to show detailed progress information
  }
  ```

- **SnapshotReader** - Provides methods to read records from a snapshot file
  ```go
  type SnapshotReader struct {
      File   *os.File
      Path   string
      Offset int64
  }
  ```

- **SnapshotWriter** - Provides methods to write records to a snapshot file
  ```go
  type SnapshotWriter struct {
      File *os.File
      Path string
  }
  ```

- **SnapCombiner** - Main struct for the snapshot combining algorithm
  ```go
  type SnapCombiner struct {
      Config     SnapCombineConfig
      InputPaths []string
      OutputPath string
      TempDir    string
      BucketMgr  *BucketManager
      Readers    []*SnapshotReader
      Writer     *SnapshotWriter
      Stats      CombineStats
  }
  ```

## Functions

### In `snap_combine.go`:

- **init()** - Registers the command and sets up flags
- **combineSnapshots(cmd *cobra.Command, args []string) error** - Main entry point for the command
- **copySingleSnapshot(inputPath, outputPath string) error** - Helper function for single snapshot optimization

### In `bucket_manager.go`:

- **NewBucketManager(numBuckets int, tempDir string) (*BucketManager, error)** - Creates a new BucketManager
- **BucketManager.Cleanup() error** - Releases resources used by the BucketManager
- **BucketManager.GetBucketForKey(key []byte) int** - Determines which bucket a record should go into
- **BucketManager.AddRecord(record *RecordLocation) error** - Adds a record to the appropriate bucket
- **NewBucketFile(path string) (*BucketFile, error)** - Creates a new BucketFile
- **BucketFile.WriteRecord(record *RecordLocation) error** - Writes a record to the bucket file
- **BucketFile.Close() error** - Closes the bucket file
- **BucketFile.Sort() error** - Sorts the records in the bucket file
- **writeUint32(w io.Writer, val uint32) error** - Helper function for binary writing
- **writeUint64(w io.Writer, val uint64) error** - Helper function for binary writing

### In `snap_combiner.go`:

- **SnapCombiner.Execute() error** - Main method that orchestrates the combining process
- **SnapCombiner.ProcessSnapshots() error** - Processes snapshots and distributes records to buckets
- **SnapCombiner.SortBuckets() error** - Sorts records in each bucket
- **SnapCombiner.MergeBuckets() error** - Merges sorted buckets into the output snapshot
- **SnapCombiner.PrintStats()** - Prints statistics about the combining process

## Algorithm Flow

1. **Command Execution**:
   - Parse command line arguments
   - Handle special case for single snapshot (direct copy)
   - Create SnapCombiner instance
   - Execute the combining algorithm

2. **Snapshot Processing**:
   - Open input snapshots
   - Read records in batches
   - Distribute records to appropriate buckets based on key hash

3. **Bucket Sorting**:
   - Sort each bucket file independently
   - Use external sorting algorithm to handle large buckets

4. **Bucket Merging**:
   - Merge sorted buckets into the output snapshot
   - Handle duplicate records (keep most recent)
   - Update record indexes

5. **Finalization**:
   - Write the final snapshot file
   - Clean up temporary files
   - Print statistics

## Memory Optimization

- Process snapshots in batches to limit memory usage
- Use file-backed buckets for sorting large datasets
- Only load one bucket at a time into memory for sorting
- Stream records directly to output without keeping them in memory

## Error Handling

- Proper cleanup of resources on error
- Detailed error messages with context
- Graceful handling of edge cases (empty snapshots, duplicate records)
