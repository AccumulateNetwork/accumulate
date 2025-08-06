# Accumulate Snapshot Format - Combining

This document provides comprehensive guidance for combining multiple Accumulate snapshot files, including algorithms, implementation strategies, and best practices for memory-efficient processing.

## Overview

Combining snapshots is a critical operation for network maintenance, backup consolidation, and multi-partition synchronization. The process involves merging multiple snapshot files while maintaining data integrity, handling duplicates, and preserving the Binary Patricia Tree structure.

## Combining Algorithms

### Basic Combining Strategy

The fundamental approach to combining snapshots follows these principles:

1. **Sequential Processing**: Process input snapshots one at a time to minimize memory usage
2. **Record Deduplication**: Use key-based deduplication to handle overlapping data
3. **Streaming Output**: Write combined results incrementally to avoid memory exhaustion
4. **Index Reconstruction**: Build new indexes for efficient access to combined data

### Memory-Efficient Combining Algorithm

```go
func CombineSnapshots(inputFiles []string, outputFile string) error {
    // Step 1: Initialize temporary storage for deduplication
    tempDB, err := createTemporaryDatabase()
    if err != nil {
        return fmt.Errorf("failed to create temporary database: %v", err)
    }
    defer tempDB.Close()

    // Step 2: Process each input snapshot sequentially
    var combinedHeader *snapshot.Header
    recordCount := 0
    
    for i, inputFile := range inputFiles {
        fmt.Printf("Processing snapshot %d/%d: %s\n", i+1, len(inputFiles), inputFile)
        
        count, header, err := processInputSnapshot(inputFile, tempDB)
        if err != nil {
            return fmt.Errorf("failed to process %s: %v", inputFile, err)
        }
        
        recordCount += count
        
        // Use the most recent header
        if combinedHeader == nil || header.SystemLedger.Index > combinedHeader.SystemLedger.Index {
            combinedHeader = header
        }
    }

    // Step 3: Write combined snapshot
    fmt.Printf("Writing combined snapshot with %d records\n", recordCount)
    return writeCombinedSnapshot(outputFile, tempDB, combinedHeader)
}
```

### Input Snapshot Processing

```go
func processInputSnapshot(filename string, tempDB *TemporaryDatabase) (int, *snapshot.Header, error) {
    // Open input snapshot
    file, err := os.Open(filename)
    if err != nil {
        return 0, nil, fmt.Errorf("failed to open file: %v", err)
    }
    defer file.Close()

    reader, err := snapshot.Open(file)
    if err != nil {
        return 0, nil, fmt.Errorf("failed to open snapshot: %v", err)
    }

    // Validate version compatibility
    if reader.Header.Version != 2 {
        return 0, nil, fmt.Errorf("unsupported snapshot version: %d", reader.Header.Version)
    }

    recordCount := 0
    
    // Process all record sections
    for i, section := range reader.Sections {
        if section.Type() != snapshot.SectionTypeRecords {
            continue
        }
        
        count, err := processRecordSection(reader, i, tempDB)
        if err != nil {
            return recordCount, reader.Header, fmt.Errorf("failed to process section %d: %v", i, err)
        }
        
        recordCount += count
    }
    
    return recordCount, reader.Header, nil
}

func processRecordSection(reader *snapshot.Reader, sectionIndex int, tempDB *TemporaryDatabase) (int, error) {
    recordReader, err := reader.OpenRecords(sectionIndex)
    if err != nil {
        return 0, err
    }

    count := 0
    for {
        record, err := recordReader.Read()
        if err == io.EOF {
            break
        }
        if err != nil {
            return count, fmt.Errorf("failed to read record: %v", err)
        }
        
        // Deduplicate and store record
        if err := deduplicateAndStore(record, tempDB); err != nil {
            return count, fmt.Errorf("failed to store record: %v", err)
        }
        
        count++
        if count%10000 == 0 {
            fmt.Printf("  Processed %d records from section %d\n", count, sectionIndex)
        }
    }
    
    return count, nil
}
```

### Record Deduplication Strategy

```go
type TemporaryDatabase struct {
    records map[string]*RecordWithMetadata
    mutex   sync.RWMutex
}

type RecordWithMetadata struct {
    Record    *snapshot.RecordEntry
    Timestamp time.Time
    Source    string
    Priority  int
}

func deduplicateAndStore(record *snapshot.RecordEntry, tempDB *TemporaryDatabase) error {
    keyStr := record.Key.String()
    
    tempDB.mutex.Lock()
    defer tempDB.mutex.Unlock()
    
    existing, exists := tempDB.records[keyStr]
    if !exists {
        // New record - store it
        tempDB.records[keyStr] = &RecordWithMetadata{
            Record:    record,
            Timestamp: time.Now(),
            Priority:  calculateRecordPriority(record),
        }
        return nil
    }
    
    // Record exists - determine which to keep
    if shouldReplaceRecord(existing, record) {
        tempDB.records[keyStr] = &RecordWithMetadata{
            Record:    record,
            Timestamp: time.Now(),
            Priority:  calculateRecordPriority(record),
        }
    }
    
    return nil
}

func shouldReplaceRecord(existing *RecordWithMetadata, newRecord *snapshot.RecordEntry) bool {
    newPriority := calculateRecordPriority(newRecord)
    
    // Higher priority records replace lower priority ones
    if newPriority > existing.Priority {
        return true
    }
    
    // For same priority, prefer newer timestamps (if available in record metadata)
    if newPriority == existing.Priority {
        return isNewerRecord(newRecord, existing.Record)
    }
    
    return false
}

func calculateRecordPriority(record *snapshot.RecordEntry) int {
    keyStr := record.Key.String()
    
    // Priority based on record type
    switch {
    case strings.HasPrefix(keyStr, "Account/"):
        return 100
    case strings.HasPrefix(keyStr, "Transaction/"):
        return 90
    case strings.HasPrefix(keyStr, "Chain/"):
        return 80
    case strings.HasPrefix(keyStr, "System/"):
        return 110
    default:
        return 50
    }
}
```

### Combined Snapshot Writing

```go
func writeCombinedSnapshot(filename string, tempDB *TemporaryDatabase, header *snapshot.Header) error {
    // Create output file
    file, err := os.Create(filename)
    if err != nil {
        return fmt.Errorf("failed to create output file: %v", err)
    }
    defer file.Close()

    writer, err := snapshot.Create(file)
    if err != nil {
        return fmt.Errorf("failed to create snapshot writer: %v", err)
    }

    // Update header with combined information
    combinedHeader := *header
    combinedHeader.RootHash = calculateCombinedRootHash(tempDB)
    
    if err := writer.WriteHeader(&combinedHeader); err != nil {
        return fmt.Errorf("failed to write header: %v", err)
    }

    // Write records organized by type
    if err := writeRecordsByType(writer, tempDB); err != nil {
        return fmt.Errorf("failed to write records: %v", err)
    }

    // Write record index for efficient access
    if err := writeCombinedIndex(writer, tempDB); err != nil {
        return fmt.Errorf("failed to write index: %v", err)
    }

    return nil
}

func writeRecordsByType(writer *snapshot.Writer, tempDB *TemporaryDatabase) error {
    // Group records by type
    recordGroups := groupRecordsByType(tempDB)
    
    // Write each group to a separate section
    for recordType, records := range recordGroups {
        fmt.Printf("Writing %d %s records\n", len(records), recordType)
        
        section, err := writer.OpenRaw(snapshot.SectionTypeRecords)
        if err != nil {
            return fmt.Errorf("failed to open section for %s: %v", recordType, err)
        }
        
        // Sort records within group for consistent output
        sort.Slice(records, func(i, j int) bool {
            return records[i].Record.Key.String() < records[j].Record.Key.String()
        })
        
        for _, recordMeta := range records {
            if err := section.WriteValue(recordMeta.Record); err != nil {
                return fmt.Errorf("failed to write %s record: %v", recordType, err)
            }
        }
        
        if err := section.Close(); err != nil {
            return fmt.Errorf("failed to close %s section: %v", recordType, err)
        }
    }
    
    return nil
}

func groupRecordsByType(tempDB *TemporaryDatabase) map[string][]*RecordWithMetadata {
    groups := make(map[string][]*RecordWithMetadata)
    
    tempDB.mutex.RLock()
    defer tempDB.mutex.RUnlock()
    
    for _, recordMeta := range tempDB.records {
        keyStr := recordMeta.Record.Key.String()
        
        var recordType string
        switch {
        case strings.HasPrefix(keyStr, "Account/"):
            recordType = "Account"
        case strings.HasPrefix(keyStr, "Transaction/"):
            recordType = "Transaction"
        case strings.HasPrefix(keyStr, "Chain/"):
            recordType = "Chain"
        case strings.HasPrefix(keyStr, "Message/"):
            recordType = "Message"
        case strings.HasPrefix(keyStr, "System/"):
            recordType = "System"
        default:
            recordType = "Other"
        }
        
        groups[recordType] = append(groups[recordType], recordMeta)
    }
    
    return groups
}
```

## Advanced Combining Strategies

### Parallel Processing Approach

```go
func CombineSnapshotsParallel(inputFiles []string, outputFile string, workerCount int) error {
    // Create worker pool for processing input files
    inputChan := make(chan string, len(inputFiles))
    resultChan := make(chan ProcessingResult, len(inputFiles))
    
    // Start workers
    var wg sync.WaitGroup
    for i := 0; i < workerCount; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            processInputWorker(workerID, inputChan, resultChan)
        }(i)
    }
    
    // Send input files to workers
    for _, file := range inputFiles {
        inputChan <- file
    }
    close(inputChan)
    
    // Wait for all workers to complete
    wg.Wait()
    close(resultChan)
    
    // Collect results and merge
    var results []ProcessingResult
    for result := range resultChan {
        if result.Error != nil {
            return fmt.Errorf("worker error: %v", result.Error)
        }
        results = append(results, result)
    }
    
    // Merge worker results
    return mergeWorkerResults(results, outputFile)
}

type ProcessingResult struct {
    Records []RecordWithMetadata
    Header  *snapshot.Header
    Error   error
}

func processInputWorker(workerID int, inputChan <-chan string, resultChan chan<- ProcessingResult) {
    for filename := range inputChan {
        fmt.Printf("Worker %d processing: %s\n", workerID, filename)
        
        records, header, err := processSnapshotToMemory(filename)
        resultChan <- ProcessingResult{
            Records: records,
            Header:  header,
            Error:   err,
        }
    }
}
```

### Incremental Combining

```go
func CombineSnapshotsIncremental(baseSnapshot, deltaSnapshot, outputFile string) error {
    // Open base snapshot
    baseFile, err := os.Open(baseSnapshot)
    if err != nil {
        return fmt.Errorf("failed to open base snapshot: %v", err)
    }
    defer baseFile.Close()
    
    baseReader, err := snapshot.Open(baseFile)
    if err != nil {
        return fmt.Errorf("failed to open base snapshot reader: %v", err)
    }
    
    // Open delta snapshot
    deltaFile, err := os.Open(deltaSnapshot)
    if err != nil {
        return fmt.Errorf("failed to open delta snapshot: %v", err)
    }
    defer deltaFile.Close()
    
    deltaReader, err := snapshot.Open(deltaFile)
    if err != nil {
        return fmt.Errorf("failed to open delta snapshot reader: %v", err)
    }
    
    // Create output snapshot
    outputFileHandle, err := os.Create(outputFile)
    if err != nil {
        return fmt.Errorf("failed to create output file: %v", err)
    }
    defer outputFileHandle.Close()
    
    writer, err := snapshot.Create(outputFileHandle)
    if err != nil {
        return fmt.Errorf("failed to create output writer: %v", err)
    }
    
    // Use the newer header
    header := deltaReader.Header
    if baseReader.Header.SystemLedger.Index > deltaReader.Header.SystemLedger.Index {
        header = baseReader.Header
    }
    
    if err := writer.WriteHeader(header); err != nil {
        return fmt.Errorf("failed to write header: %v", err)
    }
    
    // Load delta records into memory for fast lookup
    deltaRecords, err := loadRecordsToMap(deltaReader)
    if err != nil {
        return fmt.Errorf("failed to load delta records: %v", err)
    }
    
    // Stream base records, applying delta updates
    return streamWithDeltaUpdates(baseReader, writer, deltaRecords)
}
```

### BPT Reconstruction

```go
func reconstructBPT(tempDB *TemporaryDatabase) ([]*snapshot.RecordEntry, error) {
    // Collect all record keys for BPT construction
    var keys []record.Key
    
    tempDB.mutex.RLock()
    for _, recordMeta := range tempDB.records {
        keys = append(keys, *recordMeta.Record.Key)
    }
    tempDB.mutex.RUnlock()
    
    // Build Binary Patricia Tree
    bptBuilder := NewBPTBuilder()
    for _, key := range keys {
        keyHash := key.Hash()
        if err := bptBuilder.Insert(keyHash, key); err != nil {
            return nil, fmt.Errorf("failed to insert key into BPT: %v", err)
        }
    }
    
    // Generate BPT records
    bptRecords := make([]*snapshot.RecordEntry, 0)
    
    err := bptBuilder.Walk(func(nodeKey []byte, nodeValue []byte) error {
        // Create BPT record key
        bptKey := record.NewKey("BPT", hex.EncodeToString(nodeKey))
        
        // Create BPT record entry
        bptRecord := &snapshot.RecordEntry{
            Key:   bptKey,
            Value: nodeValue,
        }
        
        bptRecords = append(bptRecords, bptRecord)
        return nil
    })
    
    if err != nil {
        return nil, fmt.Errorf("failed to walk BPT: %v", err)
    }
    
    return bptRecords, nil
}
```

## Performance Optimization

### Memory Management

```go
type MemoryEfficientCombiner struct {
    maxMemoryUsage int64
    currentUsage   int64
    tempFiles      []string
    mutex          sync.Mutex
}

func (c *MemoryEfficientCombiner) ProcessRecord(record *snapshot.RecordEntry) error {
    recordSize := estimateRecordSize(record)
    
    c.mutex.Lock()
    defer c.mutex.Unlock()
    
    if c.currentUsage+recordSize > c.maxMemoryUsage {
        // Flush to temporary file
        if err := c.flushToTempFile(); err != nil {
            return fmt.Errorf("failed to flush to temp file: %v", err)
        }
        c.currentUsage = 0
    }
    
    // Add record to memory buffer
    c.currentUsage += recordSize
    return c.addToMemoryBuffer(record)
}

func (c *MemoryEfficientCombiner) flushToTempFile() error {
    tempFile, err := os.CreateTemp("", "snapshot-combine-*.tmp")
    if err != nil {
        return err
    }
    
    c.tempFiles = append(c.tempFiles, tempFile.Name())
    
    // Write memory buffer to temp file
    writer, err := snapshot.Create(tempFile)
    if err != nil {
        return err
    }
    
    // Write records from memory buffer
    return c.writeMemoryBufferToFile(writer)
}
```

### Streaming Merge

```go
func streamingMerge(tempFiles []string, outputFile string) error {
    // Open all temp files
    readers := make([]*snapshot.Reader, len(tempFiles))
    recordReaders := make([]RecordIterator, len(tempFiles))
    
    for i, tempFile := range tempFiles {
        file, err := os.Open(tempFile)
        if err != nil {
            return fmt.Errorf("failed to open temp file %s: %v", tempFile, err)
        }
        defer file.Close()
        
        reader, err := snapshot.Open(file)
        if err != nil {
            return fmt.Errorf("failed to open temp snapshot %s: %v", tempFile, err)
        }
        
        readers[i] = reader
        recordReaders[i] = NewRecordIterator(reader)
    }
    
    // Create output writer
    outputFileHandle, err := os.Create(outputFile)
    if err != nil {
        return fmt.Errorf("failed to create output file: %v", err)
    }
    defer outputFileHandle.Close()
    
    writer, err := snapshot.Create(outputFileHandle)
    if err != nil {
        return fmt.Errorf("failed to create output writer: %v", err)
    }
    
    // Perform k-way merge
    return performKWayMerge(recordReaders, writer)
}

func performKWayMerge(readers []RecordIterator, writer *snapshot.Writer) error {
    // Use priority queue for efficient k-way merge
    pq := NewRecordPriorityQueue()
    
    // Initialize priority queue with first record from each reader
    for i, reader := range readers {
        if record, err := reader.Next(); err == nil {
            pq.Push(&RecordWithSource{
                Record:     record,
                SourceID:   i,
                SourceIter: reader,
            })
        }
    }
    
    section, err := writer.OpenRaw(snapshot.SectionTypeRecords)
    if err != nil {
        return fmt.Errorf("failed to open output section: %v", err)
    }
    defer section.Close()
    
    var lastKey string
    
    for !pq.Empty() {
        item := pq.Pop()
        
        // Skip duplicates (keep first occurrence)
        currentKey := item.Record.Key.String()
        if currentKey == lastKey {
            // Get next record from same source
            if nextRecord, err := item.SourceIter.Next(); err == nil {
                item.Record = nextRecord
                pq.Push(item)
            }
            continue
        }
        
        // Write record to output
        if err := section.WriteValue(item.Record); err != nil {
            return fmt.Errorf("failed to write record: %v", err)
        }
        
        lastKey = currentKey
        
        // Get next record from same source
        if nextRecord, err := item.SourceIter.Next(); err == nil {
            item.Record = nextRecord
            pq.Push(item)
        }
    }
    
    return nil
}
```

## Error Handling and Recovery

### Robust Combining with Recovery

```go
func CombineSnapshotsWithRecovery(inputFiles []string, outputFile string, checkpointFile string) error {
    // Load checkpoint if exists
    checkpoint, err := loadCheckpoint(checkpointFile)
    if err != nil && !os.IsNotExist(err) {
        return fmt.Errorf("failed to load checkpoint: %v", err)
    }
    
    startIndex := 0
    var tempDB *TemporaryDatabase
    
    if checkpoint != nil {
        startIndex = checkpoint.LastProcessedFile + 1
        tempDB = checkpoint.TempDB
        fmt.Printf("Resuming from file %d\n", startIndex)
    } else {
        tempDB, err = createTemporaryDatabase()
        if err != nil {
            return fmt.Errorf("failed to create temporary database: %v", err)
        }
    }
    
    defer tempDB.Close()
    
    // Process remaining files
    for i := startIndex; i < len(inputFiles); i++ {
        fmt.Printf("Processing file %d/%d: %s\n", i+1, len(inputFiles), inputFiles[i])
        
        if err := processInputSnapshotWithRecovery(inputFiles[i], tempDB); err != nil {
            // Save checkpoint before returning error
            checkpoint := &Checkpoint{
                LastProcessedFile: i - 1,
                TempDB:           tempDB,
                Timestamp:        time.Now(),
            }
            saveCheckpoint(checkpointFile, checkpoint)
            return fmt.Errorf("failed to process %s: %v", inputFiles[i], err)
        }
        
        // Save checkpoint periodically
        if i%10 == 0 {
            checkpoint := &Checkpoint{
                LastProcessedFile: i,
                TempDB:           tempDB,
                Timestamp:        time.Now(),
            }
            if err := saveCheckpoint(checkpointFile, checkpoint); err != nil {
                fmt.Printf("Warning: failed to save checkpoint: %v\n", err)
            }
        }
    }
    
    // Write final combined snapshot
    if err := writeCombinedSnapshot(outputFile, tempDB, nil); err != nil {
        return fmt.Errorf("failed to write combined snapshot: %v", err)
    }
    
    // Clean up checkpoint
    os.Remove(checkpointFile)
    return nil
}

type Checkpoint struct {
    LastProcessedFile int
    TempDB           *TemporaryDatabase
    Timestamp        time.Time
}
```

## Best Practices

### 1. Memory Management
- **Stream Processing**: Process records one at a time to minimize memory usage
- **Temporary Storage**: Use disk-based temporary storage for large datasets
- **Batch Processing**: Process records in batches to balance memory and I/O efficiency
- **Memory Monitoring**: Monitor memory usage and implement backpressure mechanisms

### 2. Performance Optimization
- **Parallel Processing**: Use worker pools for CPU-intensive operations
- **I/O Optimization**: Use buffered I/O and minimize disk seeks
- **Index Utilization**: Leverage record indexes for efficient random access
- **Compression**: Consider compressing temporary files to save disk space

### 3. Error Handling
- **Graceful Degradation**: Continue processing when encountering non-critical errors
- **Checkpointing**: Implement checkpointing for long-running operations
- **Validation**: Validate input snapshots before processing
- **Logging**: Provide detailed logging for debugging and monitoring

### 4. Data Integrity
- **Deduplication**: Implement robust deduplication logic
- **Verification**: Verify combined snapshots after creation
- **Backup**: Keep backups of original snapshots during combining
- **Atomic Operations**: Ensure combining operations are atomic where possible

---

*This document provides comprehensive guidance for combining Accumulate snapshot files. For operational details, see [Snapshot Operations](snapshot-format-operations.md).*
