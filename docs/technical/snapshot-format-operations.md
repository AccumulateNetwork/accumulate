# Accumulate Snapshot Format - Operations

This document provides comprehensive guidance for reading, writing, and processing Accumulate snapshot files, including practical examples and best practices.

## Reader Operations

### Opening and Reading Snapshots

#### Basic Snapshot Reading

```go
package main

import (
    "fmt"
    "io"
    "os"
    
    "gitlab.com/AccumulateNetwork/accumulate/pkg/database/snapshot"
)

func readSnapshot(filename string) error {
    // Open the snapshot file
    file, err := os.Open(filename)
    if err != nil {
        return fmt.Errorf("failed to open file: %v", err)
    }
    defer file.Close()

    // Create a snapshot reader
    reader, err := snapshot.Open(file)
    if err != nil {
        return fmt.Errorf("failed to open snapshot: %v", err)
    }

    // Access header information
    fmt.Printf("Snapshot version: %d\n", reader.Header.Version)
    fmt.Printf("Root hash: %x\n", reader.Header.RootHash)
    
    if reader.Header.SystemLedger != nil {
        fmt.Printf("System ledger URL: %v\n", reader.Header.SystemLedger.Url)
        fmt.Printf("System ledger index: %d\n", reader.Header.SystemLedger.Index)
    }

    return nil
}
```

#### Reading Records from Sections

```go
func readRecordsFromSection(reader *snapshot.Reader, sectionIndex int) error {
    // Open a records section
    recordReader, err := reader.OpenRecords(sectionIndex)
    if err != nil {
        return fmt.Errorf("failed to open records section: %v", err)
    }

    // Read records sequentially
    recordCount := 0
    for {
        record, err := recordReader.Read()
        if err == io.EOF {
            break
        }
        if err != nil {
            return fmt.Errorf("failed to read record: %v", err)
        }
        
        recordCount++
        fmt.Printf("Record %d: Key=%v, ValueSize=%d\n", 
            recordCount, record.Key, len(record.Value))
        
        // Process specific record types
        if err := processRecord(record); err != nil {
            fmt.Printf("Warning: failed to process record: %v\n", err)
        }
    }
    
    fmt.Printf("Total records read: %d\n", recordCount)
    return nil
}

func processRecord(record *snapshot.RecordEntry) error {
    // Process based on key type
    keyStr := record.Key.String()
    
    switch {
    case strings.HasPrefix(keyStr, "Account/"):
        return processAccountRecord(record)
    case strings.HasPrefix(keyStr, "Transaction/"):
        return processTransactionRecord(record)
    case strings.HasPrefix(keyStr, "Chain/"):
        return processChainRecord(record)
    default:
        // Unknown record type
        return nil
    }
}
```

#### Using Record Index for Fast Lookup

```go
func lookupRecordByKeyHash(reader *snapshot.Reader, keyHash [32]byte) (*snapshot.RecordEntry, error) {
    // Find the record index section
    var indexSectionIdx int = -1
    for i, section := range reader.Sections {
        if section.Type() == snapshot.SectionTypeRecordIndex {
            indexSectionIdx = i
            break
        }
    }
    
    if indexSectionIdx == -1 {
        return nil, fmt.Errorf("no record index section found")
    }

    // Open the index section
    indexReader, err := reader.OpenIndex(indexSectionIdx)
    if err != nil {
        return nil, fmt.Errorf("failed to open index section: %v", err)
    }

    // Binary search for the key hash (index is sorted in descending order)
    left, right := 0, indexReader.Count-1
    var indexEntry *snapshot.RecordIndexEntry
    
    for left <= right {
        mid := (left + right) / 2
        entry, err := indexReader.Read(mid)
        if err != nil {
            return nil, fmt.Errorf("failed to read index entry: %v", err)
        }
        
        // Compare key hashes
        cmp := bytes.Compare(keyHash[:], entry.KeyHash[:])
        if cmp == 0 {
            indexEntry = entry
            break
        } else if cmp > 0 {
            // Target hash is larger, search right half (descending order)
            right = mid - 1
        } else {
            // Target hash is smaller, search left half
            left = mid + 1
        }
    }
    
    if indexEntry == nil {
        return nil, fmt.Errorf("record not found")
    }

    // Use index entry to read the actual record
    recordReader, err := reader.OpenRecords(int(indexEntry.Section))
    if err != nil {
        return nil, fmt.Errorf("failed to open records section: %v", err)
    }
    
    // Seek to the record offset and read
    record, err := recordReader.ReadAt(indexEntry.Offset)
    if err != nil {
        return nil, fmt.Errorf("failed to read record at offset: %v", err)
    }
    
    return record, nil
}
```

#### Reading BPT Sections

```go
func readBPTSection(reader *snapshot.Reader, sectionIndex int) error {
    // Open a BPT section
    bptReader, err := reader.OpenBPT(sectionIndex)
    if err != nil {
        return fmt.Errorf("failed to open BPT section: %v", err)
    }

    // Read BPT entries sequentially
    entryCount := 0
    for {
        entry, err := bptReader.Read()
        if err == io.EOF {
            break
        }
        if err != nil {
            return fmt.Errorf("failed to read BPT entry: %v", err)
        }
        
        entryCount++
        fmt.Printf("BPT Entry %d: Key=%v, Hash=%x\n", 
            entryCount, entry.Key, entry.Value)
    }
    
    fmt.Printf("Total BPT entries read: %d\n", entryCount)
    return nil
}
```

## Writer Operations

### Creating Snapshots

#### Basic Snapshot Writing

```go
func writeSnapshot(filename string, records []*snapshot.RecordEntry) error {
    // Create the output file
    file, err := os.Create(filename)
    if err != nil {
        return fmt.Errorf("failed to create file: %v", err)
    }
    defer file.Close()

    // Create a snapshot writer
    writer, err := snapshot.Create(file)
    if err != nil {
        return fmt.Errorf("failed to create snapshot writer: %v", err)
    }

    // Create and write header
    header := &snapshot.Header{
        Version:  2,
        RootHash: calculateRootHash(records), // Implement this function
    }
    
    err = writer.WriteHeader(header)
    if err != nil {
        return fmt.Errorf("failed to write header: %v", err)
    }

    // Open a records section for writing
    sectionWriter, err := writer.OpenRaw(snapshot.SectionTypeRecords)
    if err != nil {
        return fmt.Errorf("failed to open records section: %v", err)
    }

    // Write records to the section
    for i, record := range records {
        err = sectionWriter.WriteValue(record)
        if err != nil {
            return fmt.Errorf("failed to write record %d: %v", i, err)
        }
    }

    // Close the section
    err = sectionWriter.Close()
    if err != nil {
        return fmt.Errorf("failed to close section: %v", err)
    }

    return nil
}
```

#### Writing Multiple Sections

```go
func writeMultiSectionSnapshot(filename string, accountRecords, transactionRecords []*snapshot.RecordEntry) error {
    file, err := os.Create(filename)
    if err != nil {
        return fmt.Errorf("failed to create file: %v", err)
    }
    defer file.Close()

    writer, err := snapshot.Create(file)
    if err != nil {
        return fmt.Errorf("failed to create snapshot writer: %v", err)
    }

    // Write header
    header := &snapshot.Header{Version: 2}
    if err := writer.WriteHeader(header); err != nil {
        return fmt.Errorf("failed to write header: %v", err)
    }

    // Write account records section
    accountSection, err := writer.OpenRaw(snapshot.SectionTypeRecords)
    if err != nil {
        return fmt.Errorf("failed to open account section: %v", err)
    }
    
    for _, record := range accountRecords {
        if err := accountSection.WriteValue(record); err != nil {
            return fmt.Errorf("failed to write account record: %v", err)
        }
    }
    
    if err := accountSection.Close(); err != nil {
        return fmt.Errorf("failed to close account section: %v", err)
    }

    // Write transaction records section
    txnSection, err := writer.OpenRaw(snapshot.SectionTypeRecords)
    if err != nil {
        return fmt.Errorf("failed to open transaction section: %v", err)
    }
    
    for _, record := range transactionRecords {
        if err := txnSection.WriteValue(record); err != nil {
            return fmt.Errorf("failed to write transaction record: %v", err)
        }
    }
    
    if err := txnSection.Close(); err != nil {
        return fmt.Errorf("failed to close transaction section: %v", err)
    }

    // Write record index
    if err := writeRecordIndex(writer, accountRecords, transactionRecords); err != nil {
        return fmt.Errorf("failed to write record index: %v", err)
    }

    return nil
}
```

#### Writing Record Index

```go
func writeRecordIndex(writer *snapshot.Writer, recordSections ...[]*snapshot.RecordEntry) error {
    var indexEntries []*snapshot.RecordIndexEntry
    
    // Build index entries for all sections
    for sectionNum, records := range recordSections {
        offset := uint64(0)
        
        for _, record := range records {
            // Calculate key hash
            keyHash := record.Key.Hash()
            
            // Create index entry
            entry := &snapshot.RecordIndexEntry{
                KeyHash: keyHash,
                Section: uint64(sectionNum),
                Offset:  offset,
            }
            indexEntries = append(indexEntries, entry)
            
            // Update offset (approximate - actual implementation would track precise offsets)
            recordSize := estimateRecordSize(record)
            offset += uint64(recordSize)
        }
    }
    
    // Sort entries by key hash in descending order
    sort.Slice(indexEntries, func(i, j int) bool {
        return bytes.Compare(indexEntries[i].KeyHash[:], indexEntries[j].KeyHash[:]) > 0
    })

    // Write index section
    indexSection, err := writer.OpenRaw(snapshot.SectionTypeRecordIndex)
    if err != nil {
        return fmt.Errorf("failed to open index section: %v", err)
    }
    
    for _, entry := range indexEntries {
        if err := indexSection.WriteValue(entry); err != nil {
            return fmt.Errorf("failed to write index entry: %v", err)
        }
    }
    
    return indexSection.Close()
}
```

## Processing Operations

### Streaming Large Snapshots

```go
func streamProcessSnapshot(filename string, processor func(*snapshot.RecordEntry) error) error {
    file, err := os.Open(filename)
    if err != nil {
        return fmt.Errorf("failed to open file: %v", err)
    }
    defer file.Close()

    reader, err := snapshot.Open(file)
    if err != nil {
        return fmt.Errorf("failed to open snapshot: %v", err)
    }

    // Process each records section
    for i, section := range reader.Sections {
        if section.Type() != snapshot.SectionTypeRecords {
            continue
        }
        
        fmt.Printf("Processing section %d...\n", i)
        
        recordReader, err := reader.OpenRecords(i)
        if err != nil {
            return fmt.Errorf("failed to open section %d: %v", i, err)
        }

        // Stream records without loading all into memory
        recordCount := 0
        for {
            record, err := recordReader.Read()
            if err == io.EOF {
                break
            }
            if err != nil {
                return fmt.Errorf("failed to read record: %v", err)
            }
            
            // Process each record
            if err := processor(record); err != nil {
                return fmt.Errorf("failed to process record %d: %v", recordCount, err)
            }
            
            recordCount++
            if recordCount%10000 == 0 {
                fmt.Printf("Processed %d records...\n", recordCount)
            }
        }
        
        fmt.Printf("Section %d: processed %d records\n", i, recordCount)
    }
    
    return nil
}
```

### Filtering and Transforming Records

```go
func filterAndTransformSnapshot(inputFile, outputFile string, filter func(*snapshot.RecordEntry) bool) error {
    // Open input snapshot
    input, err := os.Open(inputFile)
    if err != nil {
        return fmt.Errorf("failed to open input: %v", err)
    }
    defer input.Close()

    reader, err := snapshot.Open(input)
    if err != nil {
        return fmt.Errorf("failed to open input snapshot: %v", err)
    }

    // Create output snapshot
    output, err := os.Create(outputFile)
    if err != nil {
        return fmt.Errorf("failed to create output: %v", err)
    }
    defer output.Close()

    writer, err := snapshot.Create(output)
    if err != nil {
        return fmt.Errorf("failed to create output snapshot: %v", err)
    }

    // Copy header (modify as needed)
    if err := writer.WriteHeader(reader.Header); err != nil {
        return fmt.Errorf("failed to write header: %v", err)
    }

    // Process and filter records
    sectionWriter, err := writer.OpenRaw(snapshot.SectionTypeRecords)
    if err != nil {
        return fmt.Errorf("failed to open output section: %v", err)
    }

    totalRecords := 0
    filteredRecords := 0

    for i, section := range reader.Sections {
        if section.Type() != snapshot.SectionTypeRecords {
            continue
        }
        
        recordReader, err := reader.OpenRecords(i)
        if err != nil {
            return fmt.Errorf("failed to open input section %d: %v", i, err)
        }

        for {
            record, err := recordReader.Read()
            if err == io.EOF {
                break
            }
            if err != nil {
                return fmt.Errorf("failed to read record: %v", err)
            }
            
            totalRecords++
            
            // Apply filter
            if filter(record) {
                if err := sectionWriter.WriteValue(record); err != nil {
                    return fmt.Errorf("failed to write filtered record: %v", err)
                }
                filteredRecords++
            }
        }
    }

    if err := sectionWriter.Close(); err != nil {
        return fmt.Errorf("failed to close output section: %v", err)
    }

    fmt.Printf("Filtered %d records from %d total records\n", filteredRecords, totalRecords)
    return nil
}
```

### Validating Snapshot Integrity

```go
func validateSnapshot(filename string) error {
    file, err := os.Open(filename)
    if err != nil {
        return fmt.Errorf("failed to open file: %v", err)
    }
    defer file.Close()

    reader, err := snapshot.Open(file)
    if err != nil {
        return fmt.Errorf("failed to open snapshot: %v", err)
    }

    // Validate header
    if reader.Header.Version != 2 {
        return fmt.Errorf("unsupported version: %d", reader.Header.Version)
    }

    fmt.Printf("Header validation: OK\n")

    // Validate sections
    recordCount := 0
    indexCount := 0
    bptCount := 0

    for i, section := range reader.Sections {
        fmt.Printf("Validating section %d (type=%d)...\n", i, section.Type())
        
        switch section.Type() {
        case snapshot.SectionTypeRecords:
            count, err := validateRecordsSection(reader, i)
            if err != nil {
                return fmt.Errorf("section %d validation failed: %v", i, err)
            }
            recordCount += count
            
        case snapshot.SectionTypeRecordIndex:
            count, err := validateIndexSection(reader, i)
            if err != nil {
                return fmt.Errorf("index section %d validation failed: %v", i, err)
            }
            indexCount = count
            
        case snapshot.SectionTypeBPT:
            count, err := validateBPTSection(reader, i)
            if err != nil {
                return fmt.Errorf("BPT section %d validation failed: %v", i, err)
            }
            bptCount += count
        }
    }

    fmt.Printf("Validation complete: %d records, %d index entries, %d BPT entries\n", 
        recordCount, indexCount, bptCount)
    return nil
}

func validateRecordsSection(reader *snapshot.Reader, sectionIndex int) (int, error) {
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
            return count, fmt.Errorf("failed to read record %d: %v", count, err)
        }
        
        // Validate record structure
        if record.Key == nil {
            return count, fmt.Errorf("record %d has nil key", count)
        }
        if len(record.Value) == 0 {
            return count, fmt.Errorf("record %d has empty value", count)
        }
        
        count++
    }
    
    return count, nil
}
```

## Performance Optimization

### Memory-Efficient Processing

```go
func processLargeSnapshotEfficiently(filename string) error {
    file, err := os.Open(filename)
    if err != nil {
        return err
    }
    defer file.Close()

    reader, err := snapshot.Open(file)
    if err != nil {
        return err
    }

    // Use buffered processing to limit memory usage
    const batchSize = 1000
    batch := make([]*snapshot.RecordEntry, 0, batchSize)
    
    for i, section := range reader.Sections {
        if section.Type() != snapshot.SectionTypeRecords {
            continue
        }
        
        recordReader, err := reader.OpenRecords(i)
        if err != nil {
            return err
        }

        for {
            record, err := recordReader.Read()
            if err == io.EOF {
                // Process final batch
                if len(batch) > 0 {
                    if err := processBatch(batch); err != nil {
                        return err
                    }
                }
                break
            }
            if err != nil {
                return err
            }
            
            batch = append(batch, record)
            
            // Process batch when full
            if len(batch) >= batchSize {
                if err := processBatch(batch); err != nil {
                    return err
                }
                // Clear batch for reuse
                batch = batch[:0]
            }
        }
    }
    
    return nil
}

func processBatch(records []*snapshot.RecordEntry) error {
    // Process records in batch to optimize I/O and memory usage
    for _, record := range records {
        // Process individual record
        _ = record
    }
    return nil
}
```

### Parallel Processing

```go
func processSnapshotInParallel(filename string, workerCount int) error {
    file, err := os.Open(filename)
    if err != nil {
        return err
    }
    defer file.Close()

    reader, err := snapshot.Open(file)
    if err != nil {
        return err
    }

    // Create worker pool
    recordChan := make(chan *snapshot.RecordEntry, 100)
    errorChan := make(chan error, workerCount)
    
    // Start workers
    var wg sync.WaitGroup
    for i := 0; i < workerCount; i++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            
            for record := range recordChan {
                if err := processRecordConcurrently(record, workerID); err != nil {
                    errorChan <- fmt.Errorf("worker %d error: %v", workerID, err)
                    return
                }
            }
        }(i)
    }

    // Feed records to workers
    go func() {
        defer close(recordChan)
        
        for i, section := range reader.Sections {
            if section.Type() != snapshot.SectionTypeRecords {
                continue
            }
            
            recordReader, err := reader.OpenRecords(i)
            if err != nil {
                errorChan <- fmt.Errorf("failed to open section %d: %v", i, err)
                return
            }

            for {
                record, err := recordReader.Read()
                if err == io.EOF {
                    break
                }
                if err != nil {
                    errorChan <- fmt.Errorf("failed to read record: %v", err)
                    return
                }
                
                recordChan <- record
            }
        }
    }()

    // Wait for completion
    wg.Wait()
    
    // Check for errors
    select {
    case err := <-errorChan:
        return err
    default:
        return nil
    }
}
```

## Error Handling Best Practices

### Robust Error Handling

```go
func robustSnapshotProcessing(filename string) error {
    file, err := os.Open(filename)
    if err != nil {
        return fmt.Errorf("failed to open snapshot file: %w", err)
    }
    defer func() {
        if closeErr := file.Close(); closeErr != nil {
            fmt.Printf("Warning: failed to close file: %v\n", closeErr)
        }
    }()

    reader, err := snapshot.Open(file)
    if err != nil {
        return fmt.Errorf("failed to open snapshot reader: %w", err)
    }

    // Validate snapshot before processing
    if err := validateSnapshotHeader(reader.Header); err != nil {
        return fmt.Errorf("invalid snapshot header: %w", err)
    }

    // Process with error recovery
    var lastError error
    successfulSections := 0
    
    for i, section := range reader.Sections {
        if section.Type() != snapshot.SectionTypeRecords {
            continue
        }
        
        if err := processSection(reader, i); err != nil {
            lastError = err
            fmt.Printf("Warning: failed to process section %d: %v\n", i, err)
            continue
        }
        
        successfulSections++
    }
    
    if successfulSections == 0 && lastError != nil {
        return fmt.Errorf("failed to process any sections: %w", lastError)
    }
    
    fmt.Printf("Successfully processed %d sections\n", successfulSections)
    return nil
}

func validateSnapshotHeader(header *snapshot.Header) error {
    if header.Version != 2 {
        return fmt.Errorf("unsupported version: %d", header.Version)
    }
    
    if header.SystemLedger == nil {
        return fmt.Errorf("missing system ledger")
    }
    
    return nil
}
```

---

*This document provides comprehensive operational guidance for working with Accumulate snapshot files. For data structure details, see [Snapshot Data Structures](snapshot-format-structures.md).*
