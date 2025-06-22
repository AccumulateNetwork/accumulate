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
	"time"
)

// BucketFile represents a temporary file used for sorting records by key hash
type BucketFile struct {
	File        *os.File
	Path        string
	RecordCount int
	BucketID    int
}

// BucketManager handles the creation, writing, and reading of bucket files
type BucketManager struct {
	Buckets     []*BucketFile
	NumBuckets  int
	TempDir     string
	RecordCount int
}

// NewBucketManager creates a new BucketManager for handling file-backed buckets
func NewBucketManager(numBuckets int, tempDir string) (*BucketManager, error) {
	// If tempDir is empty, create a temporary directory
	if tempDir == "" {
		var err error
		tempDir, err = os.MkdirTemp("", "snap-combine-")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp directory: %w", err)
		}
	}
	
	// Initialize the bucket manager
	bm := &BucketManager{
		Buckets:    make([]*BucketFile, numBuckets),
		NumBuckets: numBuckets,
		TempDir:    tempDir,
	}
	
	// Create bucket files
	for i := 0; i < numBuckets; i++ {
		bucketPath := filepath.Join(tempDir, fmt.Sprintf("bucket-%04d.tmp", i))
		bucketFile, err := NewBucketFile(bucketPath)
		if err != nil {
			// Clean up already created buckets on error
			bm.Cleanup()
			return nil, fmt.Errorf("failed to create bucket file %d: %w", i, err)
		}
		bucketFile.BucketID = i
		bm.Buckets[i] = bucketFile
	}
	
	return bm, nil
}

// Cleanup releases resources used by the BucketManager
func (bm *BucketManager) Cleanup() error {
	// Close all bucket files
	for _, bucket := range bm.Buckets {
		if bucket != nil && bucket.File != nil {
			if err := bucket.Close(); err != nil {
				fmt.Printf("Warning: failed to close bucket file: %v\n", err)
			}
		}
	}
	
	// Remove the temporary directory and all its contents
	if bm.TempDir != "" {
		if err := os.RemoveAll(bm.TempDir); err != nil {
			return fmt.Errorf("failed to remove temp directory: %w", err)
		}
	}
	
	return nil
}

// GetBucketForKey determines which bucket a record should go into based on its key
func (bm *BucketManager) GetBucketForKey(key []byte) int {
	// Use a simple hash of the key to determine the bucket
	// This distributes records evenly across buckets
	var hash uint32
	for _, b := range key {
		hash = hash*31 + uint32(b)
	}
	return int(hash % uint32(bm.NumBuckets))
}

// AddRecord adds a record to the appropriate bucket
func (bm *BucketManager) AddRecord(record *RecordLocation) error {
	// Determine which bucket this record belongs in
	bucketIndex := bm.GetBucketForKey(record.Key)
	
	// Add the record to the bucket
	err := bm.Buckets[bucketIndex].WriteRecord(record)
	if err != nil {
		return fmt.Errorf("failed to write record to bucket %d: %w", bucketIndex, err)
	}
	
	// Update statistics
	bm.RecordCount++
	return nil
}

// SortBuckets sorts all bucket files
func (bm *BucketManager) SortBuckets() error {
	for i, bucket := range bm.Buckets {
		if err := bucket.Sort(); err != nil {
			return fmt.Errorf("failed to sort bucket %d: %w", i, err)
		}
	}
	return nil
}

// NewBucketFile creates a new BucketFile for sorting records
func NewBucketFile(path string) (*BucketFile, error) {
	// Create the bucket file
	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("failed to create bucket file: %w", err)
	}
	
	return &BucketFile{
		File:        file,
		Path:        path,
		RecordCount: 0,
	}, nil
}

// WriteRecord writes a record to the bucket file
func (bf *BucketFile) WriteRecord(record *RecordLocation) error {
	// For now, we'll just write the key and location information
	// In a real implementation, we'd use a more efficient binary format
	
	// Format: keyLength(4) + key + snapshotIndex(4) + offset(8) + size(4)
	keyLen := len(record.Key)
	
	// Write key length (4 bytes)
	if err := writeUint32(bf.File, uint32(keyLen)); err != nil {
		return err
	}
	
	// Write key
	if _, err := bf.File.Write(record.Key); err != nil {
		return fmt.Errorf("failed to write key: %w", err)
	}
	
	// Write snapshot index (4 bytes)
	if err := writeUint32(bf.File, uint32(record.SnapshotIndex)); err != nil {
		return err
	}
	
	// Write section index (4 bytes)
	if err := writeUint32(bf.File, uint32(record.SectionIndex)); err != nil {
		return err
	}
	
	// Write size (4 bytes)
	if err := writeUint32(bf.File, uint32(record.Size)); err != nil {
		return err
	}
	
	// Update record count
	bf.RecordCount++
	return nil
}

// Sort sorts the records in the bucket file using in-memory sorting
// This is a strictly serial operation with no parallelism
func (bf *BucketFile) Sort() error {
	if bf.RecordCount == 0 {
		// Nothing to sort
		return nil
	}
	
	fmt.Printf("[BUCKET SORT] Starting sort for bucket %d with %d records\n", bf.BucketID, bf.RecordCount)
	
	// Create a temporary file for the sorted records
	sortedPath := bf.Path + ".sorted"
	fmt.Printf("[BUCKET SORT] Creating temporary sorted file: %s\n", sortedPath)
	sortedFile, err := os.Create(sortedPath)
	if err != nil {
		fmt.Printf("[BUCKET SORT ERROR] Failed to create sorted file: %v\n", err)
		return fmt.Errorf("failed to create sorted file: %w", err)
	}
	defer sortedFile.Close()
	
	// Rewind the file to the beginning
	_, err = bf.File.Seek(0, io.SeekStart)
	if err != nil {
		fmt.Printf("[BUCKET SORT ERROR] Failed to seek to beginning of file: %v\n", err)
		return fmt.Errorf("failed to seek to beginning of file: %w", err)
	}
	
	// Read all records into memory
	records := make([]*RecordLocation, 0, bf.RecordCount)
	
	// Add progress tracking
	var lastDotTime time.Time = time.Now()
	var recordsRead int
	
	// Track current position in the file for direct access later
	var currentPosition int64 = 0
	
	for i := 0; i < bf.RecordCount; i++ {
		// Print a dot every 20 seconds to show progress during reading
		if time.Since(lastDotTime) > 20*time.Second {
			fmt.Print(".")
			// Flush stdout to ensure the dot is displayed immediately
			fmt.Fprint(os.Stdout, "")
			os.Stdout.Sync()
			lastDotTime = time.Now()
		}
		
		// Store the current position for this record
		recordStartPos := currentPosition
		
		// Read key length (4 bytes)
		keyLenBuf := make([]byte, 4)
		if _, err := io.ReadFull(bf.File, keyLenBuf); err != nil {
			return fmt.Errorf("failed to read key length: %w", err)
		}
		keyLen := binary.BigEndian.Uint32(keyLenBuf)
		currentPosition += 4
		
		// Read key
		key := make([]byte, keyLen)
		if _, err := io.ReadFull(bf.File, key); err != nil {
			return fmt.Errorf("failed to read key: %w", err)
		}
		currentPosition += int64(keyLen)
		
		// Read snapshot index (4 bytes)
		indexBuf := make([]byte, 4)
		if _, err := io.ReadFull(bf.File, indexBuf); err != nil {
			return fmt.Errorf("failed to read snapshot index: %w", err)
		}
		snapshotIndex := binary.BigEndian.Uint32(indexBuf)
		currentPosition += 4
		
		// Read section index (4 bytes)
		sectionBuf := make([]byte, 4)
		if _, err := io.ReadFull(bf.File, sectionBuf); err != nil {
			return fmt.Errorf("failed to read section index: %w", err)
		}
		sectionIndex := binary.BigEndian.Uint32(sectionBuf)
		currentPosition += 4
		
		// Read size (4 bytes)
		sizeBuf := make([]byte, 4)
		if _, err := io.ReadFull(bf.File, sizeBuf); err != nil {
			return fmt.Errorf("failed to read size: %w", err)
		}
		size := binary.BigEndian.Uint32(sizeBuf)
		currentPosition += 4
		
		// Create record location with position for direct access
		record := &RecordLocation{
			Key:           key,
			SnapshotIndex: int(snapshotIndex),
			SectionIndex:  int(sectionIndex),
			Size:          int(size),
			Position:      recordStartPos, // Store position for direct access
		}
		
		// Add to records slice
		records = append(records, record)
		recordsRead++
	}
	
	// Print a newline after the progress dots
	if recordsRead > 0 && time.Since(lastDotTime) < 20*time.Second {
		fmt.Println()
	}
	
	// Sort records by key - this is done serially in a single thread
	// No parallel sorting is used to ensure maximum stability
	fmt.Printf("[BUCKET SORT] Sorting %d records in memory for bucket %d\n", len(records), bf.BucketID)
	
	// Reset dot timer for sorting progress
	lastDotTime = time.Now()
	
	// Sort using binary key comparison for maximum performance
	sort.Slice(records, func(i, j int) bool {
		// Print a dot every 20 seconds to show progress during sorting
		if time.Since(lastDotTime) > 20*time.Second {
			fmt.Print(".")
			// Flush stdout to ensure the dot is displayed immediately
			fmt.Fprint(os.Stdout, "")
			os.Stdout.Sync()
			lastDotTime = time.Now()
		}
		return bytes.Compare(records[i].Key, records[j].Key) < 0
	})
	
	// Print a newline after the progress dots
	if time.Since(lastDotTime) < 20*time.Second {
		fmt.Println()
	}
	
	fmt.Printf("[BUCKET SORT] Finished in-memory sort for bucket %d\n", bf.BucketID)
	
	// Write sorted records to the temporary file
	fmt.Printf("[BUCKET SORT] Writing %d sorted records to file for bucket %d\n", len(records), bf.BucketID)
	
	// Reset dot timer for writing progress
	lastDotTime = time.Now()
	var recordsWritten int
	
	for _, record := range records {
		// Print a dot every 20 seconds to show progress during writing
		if time.Since(lastDotTime) > 20*time.Second {
			fmt.Print(".")
			// Flush stdout to ensure the dot is displayed immediately
			fmt.Fprint(os.Stdout, "")
			os.Stdout.Sync()
			lastDotTime = time.Now()
		}
		
		// Write key length (4 bytes)
		keyLen := uint32(len(record.Key))
		keyLenBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(keyLenBuf, keyLen)
		if _, err := sortedFile.Write(keyLenBuf); err != nil {
			return fmt.Errorf("failed to write key length: %w", err)
		}
		
		// Write key
		if _, err := sortedFile.Write(record.Key); err != nil {
			return fmt.Errorf("failed to write key: %w", err)
		}
		
		// Write snapshot index (4 bytes)
		indexBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(indexBuf, uint32(record.SnapshotIndex))
		if _, err := sortedFile.Write(indexBuf); err != nil {
			return fmt.Errorf("failed to write snapshot index: %w", err)
		}
		
		// Write section index (4 bytes)
		sectionBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(sectionBuf, uint32(record.SectionIndex))
		if _, err := sortedFile.Write(sectionBuf); err != nil {
			return fmt.Errorf("failed to write section index: %w", err)
		}
		
		// Write size (4 bytes)
		sizeBuf := make([]byte, 4)
		binary.BigEndian.PutUint32(sizeBuf, uint32(record.Size))
		if _, err := sortedFile.Write(sizeBuf); err != nil {
			return fmt.Errorf("failed to write size: %w", err)
		}
		
		// Write position (8 bytes) - new field for direct access
		posBuf := make([]byte, 8)
		binary.BigEndian.PutUint64(posBuf, uint64(record.Position))
		if _, err := sortedFile.Write(posBuf); err != nil {
			return fmt.Errorf("failed to write position: %w", err)
		}
		
		recordsWritten++
	}
	
	// Print a newline after the progress dots
	if recordsWritten > 0 && time.Since(lastDotTime) < 20*time.Second {
		fmt.Println()
	}
	
	// Close files
	sortedFile.Close()
	bf.File.Close()
	
	// Replace original file with sorted file
	fmt.Printf("[BUCKET SORT] Replacing original file with sorted file for bucket %d\n", bf.BucketID)
	if err := os.Rename(sortedPath, bf.Path); err != nil {
		fmt.Printf("[BUCKET SORT ERROR] Failed to replace original file with sorted file: %v\n", err)
		return fmt.Errorf("failed to replace original file with sorted file: %w", err)
	}
	
	// Reopen the sorted file
	fmt.Printf("[BUCKET SORT] Reopening sorted file for bucket %d\n", bf.BucketID)
	bf.File, err = os.Open(bf.Path)
	if err != nil {
		fmt.Printf("[BUCKET SORT ERROR] Failed to reopen sorted bucket file: %v\n", err)
		return fmt.Errorf("failed to reopen sorted bucket file: %w", err)
	}
	
	fmt.Printf("[BUCKET SORT] Successfully sorted bucket %d with %d records\n", bf.BucketID, bf.RecordCount)
	return nil
}

// Close closes the bucket file
func (bf *BucketFile) Close() error {
	// Close the file
	if bf.File != nil {
		return bf.File.Close()
	}
	return nil
}

// Helper functions for binary writing
func writeUint32(w io.Writer, val uint32) error {
	buf := make([]byte, 4)
	buf[0] = byte(val >> 24)
	buf[1] = byte(val >> 16)
	buf[2] = byte(val >> 8)
	buf[3] = byte(val)
	_, err := w.Write(buf)
	return err
}

func writeUint64(w io.Writer, val uint64) error {
	buf := make([]byte, 8)
	buf[0] = byte(val >> 56)
	buf[1] = byte(val >> 48)
	buf[2] = byte(val >> 40)
	buf[3] = byte(val >> 32)
	buf[4] = byte(val >> 24)
	buf[5] = byte(val >> 16)
	buf[6] = byte(val >> 8)
	buf[7] = byte(val)
	_, err := w.Write(buf)
	return err
}
