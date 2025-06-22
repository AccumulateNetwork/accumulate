// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

// Snapshot section types
const (
	SectionTypeHeader     uint32 = 1
	SectionTypeSnapshot   uint32 = 6
	SectionTypeRecords    uint32 = 7
	SectionTypeRecordIndex uint32 = 8
	SectionTypeRawBPT     uint32 = 9
	SectionTypeConsensus  uint32 = 10
	SectionTypeBPT        uint32 = 11
)

// SnapCombineConfig holds configuration for the snapshot combining process
type SnapCombineConfig struct {
	BatchSize  int  // Number of records to process in a batch
	NumBuckets int  // Number of bucket files to use for sorting
	Verbose    bool // Whether to show detailed progress information
}

// SnapshotReader provides methods to read records from a snapshot file
type SnapshotReader struct {
	File         *os.File
	Path         string
	Reader       *snapshot.Reader
	Offset       int64
	SectionCache *sectionCache
}

// SnapshotWriter provides methods to write records to a snapshot file
type SnapshotWriter struct {
	File *os.File
	Path string
	SectionStart int64 // Position where the current section starts
}

// SnapCombiner is the main struct for the new snapshot combining algorithm
type SnapCombiner struct {
	Config       SnapCombineConfig
	InputPaths   []string
	OutputPath   string
	TempDir      string
	BucketMgr    *BucketManager
	Readers      []*SnapshotReader
	Writer       *SnapshotWriter
	Stats        CombineStats
}

// CreateOutput initializes the output snapshot and bucket manager
func (sc *SnapCombiner) CreateOutput() error {
	startTime := time.Now()
	
	// Initialize the output file and write the header
	if err := sc.initializeOutputFile(); err != nil {
		return fmt.Errorf("failed to initialize output file: %w", err)
	}
	defer sc.Writer.File.Close()
	
	// Process snapshots using the in-memory approach
	if err := sc.ProcessSnapshotsInMemory(); err != nil {
		// Close all input files before returning error
		for _, reader := range sc.Readers {
			reader.File.Close()
		}
		return fmt.Errorf("failed to process snapshots: %w", err)
	}
	
	// Close all input files now that we're done with them
	fmt.Println("Closing all input files...")
	for _, reader := range sc.Readers {
		reader.File.Close()
	}
	
	// Update stats
	sc.Stats.TimeElapsed = time.Since(startTime).Milliseconds()
	
	return nil
}

// Execute runs the snapshot combining algorithm
func (sc *SnapCombiner) Execute() error {
	startTime := time.Now()
	
	// Create a temporary directory if needed
	if sc.TempDir == "" {
		var err error
		sc.TempDir, err = os.MkdirTemp("", "snap-combine-")
		if err != nil {
			return fmt.Errorf("failed to create temp directory: %w", err)
		}
		defer os.RemoveAll(sc.TempDir)
	}
	
	// Initialize the bucket manager
	var err error
	sc.BucketMgr, err = NewBucketManager(sc.Config.NumBuckets, sc.TempDir)
	if err != nil {
		return fmt.Errorf("failed to initialize bucket manager: %w", err)
	}
	defer sc.BucketMgr.Cleanup()
	
	// Initialize the output file and write the header
	if err := sc.initializeOutputFile(); err != nil {
		return fmt.Errorf("failed to initialize output file: %w", err)
	}
	defer sc.Writer.File.Close()
	
	// Process snapshots in batches
	if err := sc.ProcessSnapshots(); err != nil {
		return fmt.Errorf("failed to process snapshots: %w", err)
	}
	
	// Sort buckets and write records directly to output
	if err := sc.SortAndWriteBuckets(); err != nil {
		// Close all input files before returning error
		for _, reader := range sc.Readers {
			reader.File.Close()
		}
		return fmt.Errorf("failed to sort and write buckets: %w", err)
	}
	
	// Close all input files now that we're done with them
	fmt.Println("Closing all input files...")
	for _, reader := range sc.Readers {
		reader.File.Close()
	}
	
	// Update stats
	sc.Stats.TimeElapsed = time.Since(startTime).Milliseconds()
	
	return nil
}

// ProcessSnapshots reads all input snapshots and distributes records to buckets
func (sc *SnapCombiner) ProcessSnapshots() error {
	fmt.Println("Processing snapshots...")
	
	// Initialize readers for all input snapshots
	sc.Readers = make([]*SnapshotReader, len(sc.InputPaths))
	for i, path := range sc.InputPaths {
		// Open the snapshot file
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open snapshot file %s: %w", path, err)
		}
		stat, err := file.Stat()
		if err != nil {
			file.Close()
			return fmt.Errorf("failed to get file stats for %s: %w", path, err)
		}
		
		// Create a SectionReader
		sectionReader, err := ioutil2.NewSectionReader(file, 0, stat.Size())
		if err != nil {
			file.Close()
			return fmt.Errorf("failed to create section reader for %s: %w", path, err)
		}
		
		// Open the snapshot using the official package
		reader, err := snapshot.Open(sectionReader)
		if err != nil {
			file.Close()
			return fmt.Errorf("failed to open snapshot %s: %w", path, err)
		}
		
		// Create a reader for this snapshot
		sc.Readers[i] = &SnapshotReader{
			File:   file,
			Path:   path,
			Reader: reader,
		}
		
		// Note: Files will be closed at the end of Execute(), not here
		// This ensures they remain open during SortAndWriteBuckets
	}
	
	// Process each snapshot
	for snapshotIndex, reader := range sc.Readers {
		// Always print snapshot processing information
		fmt.Printf("\n[SNAP COMBINE] Processing snapshot %d/%d: %s\n", 
			snapshotIndex+1, len(sc.Readers), reader.Path)
		fmt.Printf("[SNAP COMBINE] Found %d sections in the snapshot\n", len(reader.Reader.Sections))
		
		// Get file size for information
		stat, err := reader.File.Stat()
		if err == nil {
			fileSizeMB := float64(stat.Size()) / (1024 * 1024)
			fmt.Printf("[SNAP COMBINE] Snapshot file size: %.2f MB\n", fileSizeMB)
		}
		
		// Track record count for this snapshot
		recordCount := 0
		
		// Count the number of record sections
		var recordSections int
		for _, section := range reader.Reader.Sections {
			if section.Type() == snapshot.SectionTypeRecords {
				recordSections++
			}
		}
		
		// Process each section in the snapshot
		for i := 0; i < len(reader.Reader.Sections); i++ {
			section := reader.Reader.Sections[i]
			
			// Determine section type name for logging
			sectionTypeStr := "Unknown"
			sectionType := section.Type()
			switch sectionType {
			case snapshot.SectionTypeHeader:
				sectionTypeStr = "Header"
			case snapshot.SectionTypeSnapshot:
				sectionTypeStr = "Snapshot"
			case snapshot.SectionTypeRecords:
				sectionTypeStr = "Records"
			case snapshot.SectionTypeRecordIndex:
				sectionTypeStr = "RecordIndex"
			case snapshot.SectionTypeBPT:
				sectionTypeStr = "BPT"
			case snapshot.SectionTypeRawBPT:
				sectionTypeStr = "RawBPT"
			case snapshot.SectionTypeConsensus:
				sectionTypeStr = "Consensus"
			}
			
			// Always print section processing information
			fmt.Printf("[SNAP COMBINE] Processing section %d/%d: type=%s\n", 
				i, len(reader.Reader.Sections)-1, sectionTypeStr)
			
			// We're only interested in record sections
			if sectionType != snapshot.SectionTypeRecords {
				fmt.Printf("[SNAP COMBINE] Skipping non-record section %d (type=%s)\n", i, sectionTypeStr)
				continue
			}
			
			// Open the record section
			records, err := reader.Reader.OpenRecords(i)
			if err != nil {
				return fmt.Errorf("failed to open record section %d in snapshot %s: %w", i, reader.Path, err)
			}
			
			// Process records in the record section
			var sectionRecordCount int
			var lastProgressTime time.Time
			var lastProgressCount int
			var lastDotTime time.Time
			
			// Track the current position within the section
			var currentPosition int64 = 0
			
			for {
				// Read the next record
				recordEntry, err := records.Read()
				if err == io.EOF {
					break
				}
				if err != nil {
					return fmt.Errorf("failed to read record in section %d: %w", i, err)
				}
				
				// Update record counts
				recordCount++
				sectionRecordCount++
				
				// Get the key
				keyBytes, err := recordEntry.Key.MarshalBinary()
				if err != nil {
					return fmt.Errorf("failed to marshal key: %w", err)
				}
				
				// Extract record type from key string
				recordType := "unknown"
				keyStr := recordEntry.Key.String()
				parts := strings.Split(keyStr, "/")
				if len(parts) > 0 {
					recordType = parts[0]
				}
				
				// Create a record location
				record := &RecordLocation{
					Key:           keyBytes,
					SnapshotIndex: snapshotIndex,
					SectionIndex:  i,
					Position:      currentPosition,
					Size:          len(keyBytes) + len(recordEntry.Value),
					Type:          recordType,
				}
				
				// Update the position for the next record
				currentPosition += int64(record.Size)
				
				// Add the record to the appropriate bucket
				if err := sc.BucketMgr.AddRecord(record); err != nil {
					return fmt.Errorf("failed to add record to bucket: %w", err)
				}
				
				// Update statistics
				sc.Stats.RecordsRead++
				if sc.Stats.RecordsByType == nil {
					sc.Stats.RecordsByType = make(map[string]int)
				}
				sc.Stats.RecordsByType[recordType]++
				
				// Print progress periodically (not too often)
				if sc.Config.Verbose && (time.Since(lastProgressTime) > time.Second || sectionRecordCount % 10000 == 0) {
					recordsPerSecond := 0
					elapsed := time.Since(lastProgressTime).Seconds()
					if elapsed > 0 {
						recordsPerSecond = int(float64(sectionRecordCount - lastProgressCount) / elapsed)
					}
					fmt.Printf("    Read %d records from section %d (%d records/sec)\r", 
						sectionRecordCount, i, recordsPerSecond)
					lastProgressTime = time.Now()
					lastProgressCount = sectionRecordCount
				}
				
				// Print a dot every 20 seconds to show progress
				if time.Since(lastDotTime) > 20*time.Second {
					fmt.Print(".")
					// Flush stdout to ensure the dot is displayed immediately
					fmt.Fprint(os.Stdout, "")
					os.Stdout.Sync()
					lastDotTime = time.Now()
				}
			}
			
			// Log completion of section
			fmt.Printf("[SNAP COMBINE] Completed section %d: %d records read\n", i, sectionRecordCount)
		}
		
		// Log completion of snapshot
		fmt.Printf("\n[SNAP COMBINE] Completed snapshot %d/%d: %s\n", 
			snapshotIndex+1, len(sc.InputPaths), filepath.Base(reader.Path))
		fmt.Printf("[SNAP COMBINE] Processed %d record sections containing %d total records\n", 
			recordSections, recordCount)
		
		// Print record type statistics for this snapshot
		if sc.Config.Verbose {
			fmt.Println("[SNAP COMBINE] Record types processed in this snapshot:")
			for recordType, count := range sc.Stats.RecordsByType {
				fmt.Printf("  - %s: %d records\n", recordType, count)
			}
		}
	}
	
	// Log overall progress
	fmt.Printf("\n[SNAP COMBINE] SUMMARY: Processed %d records from %d snapshots\n", 
		sc.Stats.RecordsRead, len(sc.InputPaths))
	
	// Print total records by type
	fmt.Println("[SNAP COMBINE] Total records by type:")
	for recordType, count := range sc.Stats.RecordsByType {
		fmt.Printf("  - %s: %d records\n", recordType, count)
	}
	
	return nil
}

// ProcessSnapshotsInMemory reads all records from all snapshots, sorts them in memory, and writes them to the output
func (sc *SnapCombiner) ProcessSnapshotsInMemory() error {
	fmt.Println("[SNAP COMBINE] Processing snapshots using in-memory sorting...")
	
	// Create a slice to hold all records
	allRecords := make([]*RecordWithData, 0, 1000000) // Pre-allocate for better performance
	
	// Initialize readers for all input snapshots
	sc.Readers = make([]*SnapshotReader, len(sc.InputPaths))
	for i, path := range sc.InputPaths {
		// Open the snapshot file
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open snapshot file %s: %w", path, err)
		}
		
		// Get file stats to determine size
		stat, err := file.Stat()
		if err != nil {
			file.Close()
			return fmt.Errorf("failed to stat snapshot file %s: %w", path, err)
		}
		
		// Create a snapshot reader
		reader, err := snapshot.Open(file)
		if err != nil {
			file.Close()
			return fmt.Errorf("failed to open snapshot %s: %w", path, err)
		}
		
		// Store the reader
		sc.Readers[i] = &SnapshotReader{
			File:   file,
			Reader: reader,
			Path:   path,
		}
		
		// Always print snapshot processing information
		fmt.Printf("\n[SNAP COMBINE] Processing snapshot %d/%d: %s\n", 
			i+1, len(sc.Readers), path)
		fmt.Printf("[SNAP COMBINE] Found %d sections in the snapshot\n", len(reader.Sections))
		fmt.Printf("[SNAP COMBINE] Snapshot file size: %.2f MB\n", float64(stat.Size()) / (1024 * 1024))
		
		// Track record count for this snapshot
		var recordCount int
		var recordSections int
		
		// Process each section in the snapshot
		for j, section := range reader.Sections {
			// Get section type
			sectionType := section.Type()
			sectionTypeStr := "Unknown"
			switch sectionType {
			case snapshot.SectionTypeRecords:
				sectionTypeStr = "Records"
			case snapshot.SectionTypeHeader:
				sectionTypeStr = "Header"
			case snapshot.SectionTypeBPT:
				sectionTypeStr = "BPT"
			case snapshot.SectionTypeRecordIndex:
				sectionTypeStr = "RecordIndex"
			case snapshot.SectionTypeConsensus:
				sectionTypeStr = "Consensus"
			}
			
			// Always print section processing information
			fmt.Printf("[SNAP COMBINE] Processing section %d/%d: type=%s\n", 
				j, len(reader.Sections)-1, sectionTypeStr)
			
			// We're only interested in record sections
			if sectionType != snapshot.SectionTypeRecords {
				fmt.Printf("[SNAP COMBINE] Skipping non-record section %d (type=%s)\n", j, sectionTypeStr)
				continue
			}
			
			// Count record sections
			recordSections++
			
			// Read records from this section
			records, err := reader.OpenRecords(j)
			if err != nil {
				fmt.Printf("[SNAP COMBINE ERROR] Failed to open record section %d: %v\n", j, err)
				return fmt.Errorf("failed to open record section %d: %w", j, err)
			}
			
			// Process records in this section
			var sectionRecordCount int
			var lastProgressTime time.Time = time.Now()
			var lastProgressCount int
			var lastDotTime time.Time = time.Now()
			
			for {
				// Read the next record
				recordEntry, err := records.Read()
				if err == io.EOF {
					break
				}
				if err != nil {
					fmt.Printf("[SNAP COMBINE ERROR] Failed to read record: %v\n", err)
					return fmt.Errorf("failed to read record: %w", err)
				}
				
				// Update statistics
				sc.Stats.RecordsRead++
				recordCount++
				sectionRecordCount++
				
				// Get the key
				keyBytes, err := recordEntry.Key.MarshalBinary()
				if err != nil {
					return fmt.Errorf("failed to marshal key: %w", err)
				}
				
				// Extract record type from key string
				recordType := "unknown"
				keyStr := recordEntry.Key.String()
				parts := strings.Split(keyStr, "/")
				if len(parts) > 0 {
					recordType = parts[0]
				}
				
				// Create a record with data
				record := &RecordWithData{
					Key:       keyBytes,
					Value:     recordEntry.Value,
					Type:      recordType,
					SourceIdx: i,
				}
				
				// Add the record to our slice
				allRecords = append(allRecords, record)
				
				// Update record type statistics
				if sc.Stats.RecordsByType == nil {
					sc.Stats.RecordsByType = make(map[string]int)
				}
				sc.Stats.RecordsByType[recordType]++
				
				// Show progress
				if sc.Config.Verbose && (time.Since(lastProgressTime) > time.Second || sectionRecordCount % 10000 == 0) {
					recordsPerSecond := int(float64(sectionRecordCount-lastProgressCount) / time.Since(lastProgressTime).Seconds())
					fmt.Printf("    Read %d records from section %d (%d records/sec)\r", sectionRecordCount, j, recordsPerSecond)
					lastProgressTime = time.Now()
					lastProgressCount = sectionRecordCount
				}
				
				// Print a dot every 20 seconds to show progress
				if time.Since(lastDotTime) > 20*time.Second {
					fmt.Print(".")
					// Flush stdout to ensure the dot is displayed immediately
					fmt.Fprint(os.Stdout, "")
					os.Stdout.Sync()
					lastDotTime = time.Now()
				}
			}
			
			// Log completion of section
			fmt.Printf("[SNAP COMBINE] Completed section %d: %d records read\n", j, sectionRecordCount)
		}
		
		// Log completion of snapshot
		fmt.Printf("\n[SNAP COMBINE] Completed snapshot %d/%d: %s\n", 
			i+1, len(sc.InputPaths), filepath.Base(path))
		fmt.Printf("[SNAP COMBINE] Processed %d record sections containing %d total records\n", 
			recordSections, recordCount)
	}
	
	// Log overall progress
	fmt.Printf("\n[SNAP COMBINE] SUMMARY: Processed %d records from %d snapshots\n", 
		sc.Stats.RecordsRead, len(sc.InputPaths))
	
	// Print total records by type
	fmt.Println("[SNAP COMBINE] Total records by type:")
	for recordType, count := range sc.Stats.RecordsByType {
		fmt.Printf("  - %s: %d records\n", recordType, count)
	}
	
	// Sort all records by key
	fmt.Printf("\n[SNAP COMBINE] Sorting %d records in memory...\n", len(allRecords))
	var lastDotTime time.Time = time.Now()
	
	// Define a comparison function for sorting
	sort.Slice(allRecords, func(i, j int) bool {
		// Print a dot every 20 seconds to show progress during sorting
		if time.Since(lastDotTime) > 20*time.Second {
			fmt.Print(".")
			// Flush stdout to ensure the dot is displayed immediately
			fmt.Fprint(os.Stdout, "")
			os.Stdout.Sync()
			lastDotTime = time.Now()
		}
		return bytes.Compare(allRecords[i].Key, allRecords[j].Key) < 0
	})
	
	fmt.Printf("\n[SNAP COMBINE] Finished sorting %d records\n", len(allRecords))
	
	// Write sorted records to output snapshot
	fmt.Printf("[SNAP COMBINE] Writing sorted records to output snapshot...\n")
	lastDotTime = time.Now()
	
	// Track duplicates
	var duplicateKeys int
	var lastKey []byte
	var collisionMap = make(map[string][]int) // Maps key to snapshot indexes with collisions
	
	// Write all records to the output snapshot
	for i, record := range allRecords {
		// Print a dot every 20 seconds to show progress
		if time.Since(lastDotTime) > 20*time.Second {
			fmt.Print(".")
			// Flush stdout to ensure the dot is displayed immediately
			fmt.Fprint(os.Stdout, "")
			os.Stdout.Sync()
			lastDotTime = time.Now()
		}
		
		// Check for duplicate keys - these should be extremely rare
		if lastKey != nil && bytes.Equal(record.Key, lastKey) {
			duplicateKeys++
			
			// Track collision details
			keyStr := fmt.Sprintf("%X", record.Key)
			if _, exists := collisionMap[keyStr]; !exists {
				collisionMap[keyStr] = []int{}
			}
			collisionMap[keyStr] = append(collisionMap[keyStr], record.SourceIdx)
			
			if sc.Config.Verbose {
				fmt.Printf("[SNAP COMBINE] Found duplicate key: %X from snapshot %d\n", record.Key, record.SourceIdx)
			}
			continue // Skip duplicate keys
		}
		
		// Write the record to the output snapshot
		if err := sc.Writer.WriteRecord(record.Key, record.Value); err != nil {
			return fmt.Errorf("failed to write record to output: %w", err)
		}
		
		// Update statistics
		sc.Stats.RecordsWritten++
		
		// Remember this key for duplicate detection
		lastKey = record.Key
		
		// Show progress every 100,000 records
		if i > 0 && i % 100000 == 0 {
			fmt.Printf("\r[SNAP COMBINE] Wrote %d/%d records (%.1f%%)...", 
				i, len(allRecords), float64(i)/float64(len(allRecords))*100.0)
		}
	}
	
	fmt.Printf("\n[SNAP COMBINE] Finished writing %d records to output snapshot\n", sc.Stats.RecordsWritten)
	
	// Report collisions if any
	if duplicateKeys > 0 {
		fmt.Printf("[SNAP COMBINE] Found %d duplicate keys across snapshots\n", duplicateKeys)
		fmt.Println("[SNAP COMBINE] Collision details:")
		for key, snapshots := range collisionMap {
			fmt.Printf("  - Key %s found in snapshots: %v\n", key, snapshots)
		}
	} else {
		fmt.Println("[SNAP COMBINE] No duplicate keys found across snapshots")
	}
	
	return nil
}

// SortAndWriteBuckets sorts each bucket and writes records to the output snapshot
func (sc *SnapCombiner) SortAndWriteBuckets() error {
	fmt.Println("[SNAP COMBINE] Sorting buckets and writing records to output in serial mode...")
	
	// Get the total number of buckets
	numBuckets := len(sc.BucketMgr.Buckets)
	
	// Track total records written and duplicates
	var totalRecordsWritten int
	var duplicateKeys int
	var lastKey []byte
	var collisionMap = make(map[string][]int) // Maps key to snapshot indexes with collisions
	
	// Track overall progress
	var overallStartTime time.Time = time.Now()
	var lastProgressTime time.Time = time.Now()
	var totalRecordsToProcess int
	
	// Count total records to process for progress reporting
	for _, bucket := range sc.BucketMgr.Buckets {
		totalRecordsToProcess += bucket.RecordCount
	}
	
	fmt.Printf("[SNAP COMBINE] Processing a total of %d records across %d buckets\n", totalRecordsToProcess, numBuckets)
	
	// Process each bucket in strict serial order - no parallelism
	// This ensures maximum stability and prevents resource contention
	for i, bucket := range sc.BucketMgr.Buckets {
		if sc.Config.Verbose {
			fmt.Printf("Sorting bucket %d of %d (%d records)\n", i+1, numBuckets, bucket.RecordCount)
		}
		
		// Skip empty buckets
		if bucket.RecordCount == 0 {
			continue
		}
		
		// Sort the bucket
		fmt.Printf("[SNAP COMBINE] Starting to sort bucket %d of %d (bucket ID: %d) with %d records\n", i+1, numBuckets, bucket.BucketID, bucket.RecordCount)
		
		// Track time for this bucket's sorting
		bucketSortStart := time.Now()
		
		if err := bucket.Sort(); err != nil {
			fmt.Printf("[SNAP COMBINE ERROR] Failed to sort bucket %d: %v\n", i, err)
			return fmt.Errorf("failed to sort bucket %d: %w", i, err)
		}
		
		// Report sorting time
		sortDuration := time.Since(bucketSortStart)
		fmt.Printf("[SNAP COMBINE] Successfully sorted bucket %d of %d in %.2f seconds\n", 
			i+1, numBuckets, sortDuration.Seconds())
		
		// Update statistics
		sc.Stats.RecordsSorted += bucket.RecordCount
		
		// Rewind the bucket file to the beginning
		fmt.Printf("[SNAP COMBINE] Rewinding bucket %d file to beginning\n", bucket.BucketID)
		if _, err := bucket.File.Seek(0, io.SeekStart); err != nil {
			fmt.Printf("[SNAP COMBINE ERROR] Failed to seek to beginning of bucket file: %v\n", err)
			return fmt.Errorf("failed to seek to beginning of bucket file: %w", err)
		}
		
		// Create a reader for this bucket
		reader := &BucketReader{
			Bucket: bucket,
			Index:  bucket.BucketID,
		}
		
		// Read all records from this bucket and write to output
		fmt.Printf("[SNAP COMBINE] Starting to read and write records from bucket %d\n", bucket.BucketID)
		var recordsProcessed int
		var bucketDotTime time.Time = time.Now()
		var bucketStartTime time.Time = time.Now()
		
		for {
			// Read the next record
			if err := reader.ReadNext(); err != nil {
				if err == io.EOF {
					// Calculate processing rate
					bucketDuration := time.Since(bucketStartTime)
					var recordsPerSecond float64
					if bucketDuration.Seconds() > 0 {
						recordsPerSecond = float64(recordsProcessed) / bucketDuration.Seconds()
					}
					
					fmt.Printf("[SNAP COMBINE] Reached end of bucket %d after processing %d records (%.2f records/sec)\n", 
						bucket.BucketID, recordsProcessed, recordsPerSecond)
					break // End of bucket
				}
				fmt.Printf("[SNAP COMBINE ERROR] Failed to read record from bucket %d: %v\n", bucket.BucketID, err)
				return fmt.Errorf("failed to read record from bucket %d: %w", bucket.BucketID, err)
			}
			recordsProcessed++
			
			// Print a dot every 20 seconds to show progress
			if time.Since(bucketDotTime) > 20*time.Second {
				fmt.Print(".")
				// Flush stdout to ensure the dot is displayed immediately
				fmt.Fprint(os.Stdout, "")
				os.Stdout.Sync()
				bucketDotTime = time.Now()
			}
			
			// Print overall progress every 2 minutes
			if time.Since(lastProgressTime) > 2*time.Minute {
				// Calculate overall progress percentage and ETA
				overallProgress := float64(sc.Stats.RecordsSorted) / float64(totalRecordsToProcess)
				elapsedTime := time.Since(overallStartTime)
				var eta time.Duration
				if overallProgress > 0 {
					eta = time.Duration(float64(elapsedTime) / overallProgress) - elapsedTime
				}
				
				fmt.Printf("\n[SNAP COMBINE] Overall progress: %.1f%% complete, ETA: %s\n", 
					overallProgress*100, eta.Round(time.Second))
				lastProgressTime = time.Now()
			}
			
			// Get the record
			record := reader.CurrentRecord
			
			// Check for duplicate keys - these should be extremely rare
			if lastKey != nil && bytes.Equal(record.Key, lastKey) {
				duplicateKeys++
				
				// Track collision details
				keyStr := fmt.Sprintf("%X", record.Key)
				if _, exists := collisionMap[keyStr]; !exists {
					collisionMap[keyStr] = []int{}
				}
				collisionMap[keyStr] = append(collisionMap[keyStr], record.SnapshotIndex)
				
				if sc.Config.Verbose {
					fmt.Printf("[SNAP COMBINE] Found duplicate key: %X from snapshot %d\n", record.Key, record.SnapshotIndex)
				}
				continue // Skip duplicate keys
			}
			
			// This is a new key, copy the record from the source snapshot to the output
			if err := sc.copyRecord(record); err != nil {
				return fmt.Errorf("failed to copy record: %w", err)
			}
			
			// Update statistics
			totalRecordsWritten++
			
			// Remember this key
			lastKey = make([]byte, len(record.Key))
			copy(lastKey, record.Key)
		}
		
		// Log progress after each bucket
		if sc.Config.Verbose {
			fmt.Printf("  Wrote %d records from bucket %d\n", bucket.RecordCount, bucket.BucketID)
		}
	}
	
	// End the record section
	fmt.Printf("[SNAP COMBINE] Ending record section after writing %d records\n", totalRecordsWritten)
	if err := sc.endRecordSection(); err != nil {
		fmt.Printf("[SNAP COMBINE ERROR] Failed to end record section: %v\n", err)
		return fmt.Errorf("failed to end record section: %w", err)
	}
	
	// Update statistics
	sc.Stats.RecordsWritten = totalRecordsWritten
	sc.Stats.DuplicateKeys = duplicateKeys
	
	// Calculate overall processing time and rate
	overallDuration := time.Since(overallStartTime)
	var overallRecordsPerSecond float64
	if overallDuration.Seconds() > 0 {
		overallRecordsPerSecond = float64(totalRecordsWritten) / overallDuration.Seconds()
	}
	
	// Log final statistics with performance metrics
	fmt.Printf("[SNAP COMBINE] Completed writing records: %d records written, %d duplicates skipped\n", totalRecordsWritten, duplicateKeys)
	fmt.Printf("[SNAP COMBINE] Total processing time: %.2f seconds (%.2f records/sec)\n", 
		overallDuration.Seconds(), overallRecordsPerSecond)
	
	// Report any key collisions (should be extremely rare)
	if len(collisionMap) > 0 {
		fmt.Printf("\n[SNAP COMBINE] Found %d unique keys with collisions across snapshots\n", len(collisionMap))
		for key, snapshots := range collisionMap {
			fmt.Printf("  Key %s appears in snapshots: ", key)
			for i, idx := range snapshots {
				if i > 0 {
					fmt.Print(", ")
				}
				fmt.Printf("%d (%s)", idx, filepath.Base(sc.InputPaths[idx]))
			}
			fmt.Println()
		}
	} else {
		fmt.Println("[SNAP COMBINE] No key collisions detected across snapshots")
	}
	
	return nil
}

// initializeOutputFile creates the output snapshot file and writes the header
func (sc *SnapCombiner) initializeOutputFile() error {
	fmt.Println("Initializing output snapshot file...")
	
	// Create the output snapshot file
	outputFile, err := os.Create(sc.OutputPath)
	if err != nil {
		return fmt.Errorf("failed to create output snapshot: %w", err)
	}
	
	// Create a writer for the output snapshot
	sc.Writer = &SnapshotWriter{
		File: outputFile,
		Path: sc.OutputPath,
	}
	
	// Write the header section (copy from the first input snapshot)
	if err := sc.writeHeader(); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	
	// Start a record section
	if err := sc.startRecordSection(); err != nil {
		return fmt.Errorf("failed to start record section: %w", err)
	}
	
	return nil
}

// MergeBuckets merges sorted buckets and writes the output snapshot
// This method is kept for reference but is no longer used
func (sc *SnapCombiner) MergeBuckets() error {
	fmt.Println("Merging buckets and writing output snapshot...")
	
	// Create record readers for each bucket
	readers := make([]*BucketReader, 0, len(sc.BucketMgr.Buckets))
	for _, bucket := range sc.BucketMgr.Buckets {
		if bucket.RecordCount > 0 {
			// Rewind the bucket file to the beginning
			if _, err := bucket.File.Seek(0, io.SeekStart); err != nil {
				return fmt.Errorf("failed to seek to beginning of bucket file: %w", err)
			}
			
			// Create a reader for this bucket
			reader := &BucketReader{
				Bucket: bucket,
				Index:  bucket.BucketID,
			}
			
			// Read the first record
			if err := reader.ReadNext(); err != nil {
				if err != io.EOF {
					return fmt.Errorf("failed to read first record from bucket %d: %w", bucket.BucketID, err)
				}
				// Skip empty buckets
				continue
			}
			
			// Add to readers
			readers = append(readers, reader)
		}
	}
	
	// Process records in key order
	var lastKey []byte
	for len(readers) > 0 {
		// Find the reader with the smallest key
		minIndex := 0
		for i := 1; i < len(readers); i++ {
			if bytes.Compare(readers[i].CurrentRecord.Key, readers[minIndex].CurrentRecord.Key) < 0 {
				minIndex = i
			}
		}
		
		// Get the record with the smallest key
		record := readers[minIndex].CurrentRecord
		
		// Check if this is a duplicate key
		if lastKey != nil && bytes.Equal(record.Key, lastKey) {
			// This is a duplicate key, skip it
			sc.Stats.DuplicateKeys++
		} else {
			// This is a new key, copy the record from the source snapshot to the output
			if err := sc.copyRecord(record); err != nil {
				return fmt.Errorf("failed to copy record: %w", err)
			}
			
			// Update statistics
			sc.Stats.RecordsWritten++
			
			// Remember this key
			lastKey = make([]byte, len(record.Key))
			copy(lastKey, record.Key)
		}
		
		// Read the next record from this bucket
		if err := readers[minIndex].ReadNext(); err != nil {
			if err == io.EOF {
				// Remove this reader from the list
				readers = append(readers[:minIndex], readers[minIndex+1:]...)
			} else {
				return fmt.Errorf("failed to read next record: %w", err)
			}
		}
	}
	
	// Log completion
	fmt.Printf("Merged %d unique records (skipped %d duplicates)\n", sc.Stats.RecordsWritten, sc.Stats.DuplicateKeys)
	
	return nil
}

// BucketReader is a helper for reading records from a bucket file
type BucketReader struct {
	Bucket        *BucketFile
	Index         int
	CurrentRecord *RecordLocation
}

// ReadNext reads the next record from the bucket
func (br *BucketReader) ReadNext() error {
	// Read key length (4 bytes)
	keyLenBuf := make([]byte, 4)
	if _, err := io.ReadFull(br.Bucket.File, keyLenBuf); err != nil {
		return err
	}
	keyLen := binary.BigEndian.Uint32(keyLenBuf)
	
	// Read key
	key := make([]byte, keyLen)
	if _, err := io.ReadFull(br.Bucket.File, key); err != nil {
		return err
	}
	
	// Read snapshot index (4 bytes)
	indexBuf := make([]byte, 4)
	if _, err := io.ReadFull(br.Bucket.File, indexBuf); err != nil {
		return err
	}
	snapshotIndex := binary.BigEndian.Uint32(indexBuf)
	
	// Read section index (4 bytes)
	sectionBuf := make([]byte, 4)
	if _, err := io.ReadFull(br.Bucket.File, sectionBuf); err != nil {
		return err
	}
	sectionIndex := binary.BigEndian.Uint32(sectionBuf)
	
	// Read size (4 bytes)
	sizeBuf := make([]byte, 4)
	if _, err := io.ReadFull(br.Bucket.File, sizeBuf); err != nil {
		return err
	}
	size := binary.BigEndian.Uint32(sizeBuf)
	
	// Read position (8 bytes) - new field for direct access
	posBuf := make([]byte, 8)
	if _, err := io.ReadFull(br.Bucket.File, posBuf); err != nil {
		return err
	}
	position := int64(binary.BigEndian.Uint64(posBuf))
	
	// Create record location
	br.CurrentRecord = &RecordLocation{
		Key:           key,
		SnapshotIndex: int(snapshotIndex),
		SectionIndex:  int(sectionIndex),
		Size:          int(size),
		Position:      position,
	}
	
	return nil
}

// writeHeader writes the header section to the output snapshot
func (sc *SnapCombiner) writeHeader() error {
	// Open the first input snapshot to copy the header
	firstSnapshot, err := os.Open(sc.InputPaths[0])
	if err != nil {
		return fmt.Errorf("failed to open first snapshot: %w", err)
	}
	defer firstSnapshot.Close()
	
	// Read the header section type and size (12 bytes)
	header := make([]byte, 12)
	if _, err := firstSnapshot.Read(header); err != nil {
		return fmt.Errorf("failed to read header from first snapshot: %w", err)
	}
	
	// Extract section size
	sectionSize := uint64(header[4]) | uint64(header[5])<<8 | uint64(header[6])<<16 | uint64(header[7])<<24 |
					uint64(header[8])<<32 | uint64(header[9])<<40 | uint64(header[10])<<48 | uint64(header[11])<<56
	
	// Write the header section type and size to the output
	if _, err := sc.Writer.File.Write(header); err != nil {
		return fmt.Errorf("failed to write header section type and size: %w", err)
	}
	
	// Copy the header content
	headerContent := make([]byte, sectionSize)
	if _, err := firstSnapshot.Read(headerContent); err != nil {
		return fmt.Errorf("failed to read header content: %w", err)
	}
	
	if _, err := sc.Writer.File.Write(headerContent); err != nil {
		return fmt.Errorf("failed to write header content: %w", err)
	}
	
	return nil
}

// startRecordSection starts a new record section in the output snapshot
func (sc *SnapCombiner) startRecordSection() error {
	// Record section has type 7 (SectionTypeRecords)
	sectionType := uint32(7)
	
	// We don't know the size yet, so we'll write a placeholder
	sectionSize := uint64(0)
	
	// Write the section type (4 bytes)
	typeBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(typeBuf, sectionType)
	if _, err := sc.Writer.File.Write(typeBuf); err != nil {
		return fmt.Errorf("failed to write section type: %w", err)
	}
	
	// Write the section size placeholder (8 bytes)
	sizeBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(sizeBuf, sectionSize)
	if _, err := sc.Writer.File.Write(sizeBuf); err != nil {
		return fmt.Errorf("failed to write section size placeholder: %w", err)
	}
	
	// Remember the position where the record section starts
	pos, err := sc.Writer.File.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("failed to get current position: %w", err)
	}
	sc.Writer.SectionStart = pos
	
	return nil
}

// endRecordSection updates the record section size in the output snapshot
func (sc *SnapCombiner) endRecordSection() error {
	// Get the current position
	currentPos, err := sc.Writer.File.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("failed to get current position: %w", err)
	}
	
	// Calculate the section size
	sectionSize := uint64(currentPos - sc.Writer.SectionStart)
	
	// Go back to the section size field
	if _, err := sc.Writer.File.Seek(sc.Writer.SectionStart-8, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek to section size field: %w", err)
	}
	
	// Write the actual section size
	sizeBuf := make([]byte, 8)
	binary.LittleEndian.PutUint64(sizeBuf, sectionSize)
	if _, err := sc.Writer.File.Write(sizeBuf); err != nil {
		return fmt.Errorf("failed to write section size: %w", err)
	}
	
	// Go back to the end of the file
	if _, err := sc.Writer.File.Seek(0, io.SeekEnd); err != nil {
		return fmt.Errorf("failed to seek to end of file: %w", err)
	}
	
	return nil
}

// WriteRecord writes a record to the snapshot file
func (sw *SnapshotWriter) WriteRecord(key, value []byte) error {
	// Write key length as varint
	keyLen := uint64(len(key))
	keyLenBuf := make([]byte, binary.MaxVarintLen64)
	keyLenSize := binary.PutUvarint(keyLenBuf, keyLen)
	if _, err := sw.File.Write(keyLenBuf[:keyLenSize]); err != nil {
		return fmt.Errorf("failed to write key length: %w", err)
	}
	
	// Write key
	if _, err := sw.File.Write(key); err != nil {
		return fmt.Errorf("failed to write key: %w", err)
	}
	
	// Write value length as varint
	valLen := uint64(len(value))
	valLenBuf := make([]byte, binary.MaxVarintLen64)
	valLenSize := binary.PutUvarint(valLenBuf, valLen)
	if _, err := sw.File.Write(valLenBuf[:valLenSize]); err != nil {
		return fmt.Errorf("failed to write value length: %w", err)
	}
	
	// Write value
	if _, err := sw.File.Write(value); err != nil {
		return fmt.Errorf("failed to write value: %w", err)
	}
	
	return nil
}

// sectionCache is used to cache open record sections to avoid repeatedly opening the same sections
type sectionCache struct {
	reader   *snapshot.Reader
	sections map[int]snapshot.RecordReader
}

// newSectionCache creates a new section cache for a snapshot reader
func newSectionCache(reader *snapshot.Reader) *sectionCache {
	return &sectionCache{
		reader:   reader,
		sections: make(map[int]snapshot.RecordReader),
	}
}

// getSection gets or opens a record section
func (sc *sectionCache) getSection(sectionIndex int) (snapshot.RecordReader, error) {
	// Check if we already have this section open
	if section, ok := sc.sections[sectionIndex]; ok {
		return section, nil
	}
	
	// Open the section
	section, err := sc.reader.OpenRecords(sectionIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to open record section %d: %w", sectionIndex, err)
	}
	
	// Cache the section
	sc.sections[sectionIndex] = section
	return section, nil
}

// close closes all open sections
func (sc *sectionCache) close() {
	// Nothing to do - the snapshot.RecordReader doesn't have a Close method
	// and the underlying file is managed by the snapshot.Reader
}

// copyRecord copies a record from a source snapshot to the output snapshot
func (sc *SnapCombiner) copyRecord(record *RecordLocation) error {
	// Get the reader for this snapshot
	reader := sc.Readers[record.SnapshotIndex]
	
	// Create a section reader to find the specific record
	sectionReader, err := reader.Reader.OpenRecords(record.SectionIndex)
	if err != nil {
		fmt.Printf("[COPY RECORD ERROR] Failed to open record section %d: %v\n", record.SectionIndex, err)
		return fmt.Errorf("failed to open record section %d: %w", record.SectionIndex, err)
	}
	
	// Get the record directly using its position
	entry, err := sectionReader.ReadAt(record.Position)
	if err != nil {
		// If direct seek failed, fall back to sequential search
		fmt.Printf("[COPY RECORD WARNING] Failed to read record at position %d, falling back to sequential search: %v\n", 
			record.Position, err)
		
		// Read records until we find the one with the matching key
		found := false
		var keyBytes []byte
		var valueBytes []byte
		
		for !found {
			// Read the next record
			entry, err := sectionReader.Read()
			if err == io.EOF {
				fmt.Printf("[COPY RECORD ERROR] Record with key not found in section %d\n", record.SectionIndex)
				return fmt.Errorf("record not found in section %d", record.SectionIndex)
			}
			if err != nil {
				fmt.Printf("[COPY RECORD ERROR] Failed to read record: %v\n", err)
				return fmt.Errorf("failed to read record: %w", err)
			}
			
			// Check if this is the record we're looking for by comparing binary keys
			entryKeyBytes, err := entry.Key.MarshalBinary()
			if err != nil {
				fmt.Printf("[COPY RECORD ERROR] Failed to marshal entry key: %v\n", err)
				return fmt.Errorf("failed to marshal entry key: %w", err)
			}
			
			// Compare the binary keys directly
			if bytes.Equal(entryKeyBytes, record.Key) {
				// Use the already marshaled key bytes
				keyBytes = entryKeyBytes
				valueBytes = entry.Value
				found = true
			}
		}
		
		// Write the record to the output snapshot
		if err := sc.Writer.WriteRecord(keyBytes, valueBytes); err != nil {
			fmt.Printf("[COPY RECORD ERROR] Failed to write record to output: %v\n", err)
			return fmt.Errorf("failed to write record to output: %w", err)
		}
		
		return nil
	}
	
	// Marshal the key
	keyBytes, err := entry.Key.MarshalBinary()
	if err != nil {
		fmt.Printf("[COPY RECORD ERROR] Failed to marshal entry key: %v\n", err)
		return fmt.Errorf("failed to marshal entry key: %w", err)
	}
	
	// Verify that this is the correct record by comparing keys
	if !bytes.Equal(keyBytes, record.Key) {
		fmt.Printf("[COPY RECORD ERROR] Key mismatch at position %d\n", record.Position)
		return fmt.Errorf("key mismatch at position %d", record.Position)
	}
	
	// Write the record to the output snapshot
	if err := sc.Writer.WriteRecord(keyBytes, entry.Value); err != nil {
		fmt.Printf("[COPY RECORD ERROR] Failed to write record to output: %v\n", err)
		return fmt.Errorf("failed to write record to output: %w", err)
	}
	
	return nil
}

// PrintStats prints statistics about the snapshot combining process
func (sc *SnapCombiner) PrintStats() {
	fmt.Printf("\nSnapshot Combine Statistics:\n")
	fmt.Printf("------------------------\n")
	fmt.Printf("Input snapshots:    %d\n", sc.Stats.InputSnapshots)
	fmt.Printf("Records read:       %d\n", sc.Stats.RecordsRead)
	fmt.Printf("Records sorted:     %d\n", sc.Stats.RecordsSorted)
	fmt.Printf("Records written:    %d\n", sc.Stats.RecordsWritten)
	fmt.Printf("Duplicate keys:     %d\n", sc.Stats.DuplicateKeys)
	fmt.Printf("Batches processed:  %d\n", sc.Stats.BatchesProcessed)
	fmt.Printf("Buckets sorted:     %d\n", sc.Stats.BucketsSorted)
	fmt.Printf("Time elapsed:       %.2f seconds\n", float64(sc.Stats.TimeElapsed)/1000.0)
	
	// Print record types if available
	if len(sc.Stats.RecordsByType) > 0 {
		fmt.Printf("\nRecord Types:\n")
		for recordType, count := range sc.Stats.RecordsByType {
			fmt.Printf("  %-20s %d\n", recordType+":", count)
		}
	}
	fmt.Println("\nSnapshot Combining Statistics:")
	fmt.Printf("  Input snapshots: %d\n", sc.Stats.InputSnapshots)
	fmt.Printf("  Records read: %d\n", sc.Stats.RecordsRead)
	fmt.Printf("  Records written: %d\n", sc.Stats.RecordsWritten)
	fmt.Printf("  Duplicate keys: %d\n", sc.Stats.DuplicateKeys)
	fmt.Printf("  Batches processed: %d\n", sc.Stats.BatchesProcessed)
	fmt.Printf("  Time elapsed: %.2f seconds\n", float64(sc.Stats.TimeElapsed)/1000.0)
	
	fmt.Println("\nRecord types:")
	for typeName, count := range sc.Stats.RecordsByType {
		fmt.Printf("  %s: %d\n", typeName, count)
	}
}



// skipBytes reads and discards the specified number of bytes from the reader
// This is a safer alternative to Seek for large offsets
func skipBytes(file *os.File, size uint64) error {
	// Use a buffer to read data in chunks
	bufSize := uint64(64 * 1024) // 64KB buffer
	buf := make([]byte, bufSize)

	// Read in chunks until we've skipped all bytes
	for remaining := size; remaining > 0; {
		// Determine how much to read in this iteration
		readSize := bufSize
		if remaining < bufSize {
			readSize = remaining
		}

		// Read and discard the data
		n, err := io.ReadFull(file, buf[:readSize])
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return err
		}

		// Update remaining bytes
		remaining -= uint64(n)

		// If we couldn't read as much as we wanted, we've reached EOF
		if uint64(n) < readSize {
			break
		}
	}

	return nil
}
