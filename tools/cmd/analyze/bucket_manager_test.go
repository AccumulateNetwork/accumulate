// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestBucketSorting tests the bucket sorting functionality
func TestBucketSorting(t *testing.T) {
	// Create a temporary directory for test buckets
	tempDir, err := os.MkdirTemp("", "bucket-test-")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a bucket manager with 16 buckets
	bucketMgr, err := NewBucketManager(16, tempDir)
	if err != nil {
		t.Fatalf("Failed to create bucket manager: %v", err)
	}
	defer bucketMgr.Cleanup()

	// Create test records with known keys
	// We'll create keys that will distribute across different buckets
	testRecords := make([]*RecordLocation, 0, 100)
	for i := 0; i < 100; i++ {
		// Create keys with different first bytes to distribute across buckets
		// Use a pattern that's not in sorted order
		keyByte := byte((i * 7) % 16) // This will distribute keys across buckets in non-sequential order
		
		key := make([]byte, 4)
		key[0] = keyByte
		key[1] = byte(i / 16)
		key[2] = byte(i % 16)
		key[3] = 0xFF

		record := &RecordLocation{
			Key:           key,
			SnapshotIndex: 0,
			SectionIndex:  0,
			Size:          len(key) + 10, // Some arbitrary size
			Type:          "test",
		}
		
		testRecords = append(testRecords, record)
	}

	// Add records to buckets in reverse order to ensure they need sorting
	for i := len(testRecords) - 1; i >= 0; i-- {
		err := bucketMgr.AddRecord(testRecords[i])
		if err != nil {
			t.Fatalf("Failed to add record to bucket: %v", err)
		}
	}

	// Verify records were distributed across buckets
	bucketCounts := make(map[int]int)
	for _, bucket := range bucketMgr.Buckets {
		bucketCounts[bucket.BucketID] = bucket.RecordCount
	}

	// There should be records in multiple buckets
	if len(bucketCounts) < 2 {
		t.Errorf("Expected records to be distributed across multiple buckets, but only %d buckets have records", len(bucketCounts))
	}

	// Sort all buckets
	for _, bucket := range bucketMgr.Buckets {
		if bucket.RecordCount > 0 {
			err := bucket.Sort()
			if err != nil {
				t.Fatalf("Failed to sort bucket %d: %v", bucket.BucketID, err)
			}
		}
	}

	// Verify that records in each bucket are sorted
	for _, bucket := range bucketMgr.Buckets {
		if bucket.RecordCount == 0 {
			continue
		}

		// Rewind the bucket file
		if _, err := bucket.File.Seek(0, io.SeekStart); err != nil {
			t.Fatalf("Failed to seek to beginning of bucket %d: %v", bucket.BucketID, err)
		}

		// Read all records and check they're in sorted order
		var lastKey []byte
		reader := &BucketReader{
			Bucket: bucket,
			Index:  bucket.BucketID,
		}

		for i := 0; i < bucket.RecordCount; i++ {
			if err := reader.ReadNext(); err != nil {
				if err == io.EOF {
					break
				}
				t.Fatalf("Failed to read record %d from bucket %d: %v", i, bucket.BucketID, err)
			}

			// Check if keys are in ascending order
			if lastKey != nil {
				if bytes.Compare(reader.CurrentRecord.Key, lastKey) > 0 {
					t.Errorf("Records in bucket %d are not sorted correctly. Record %d has key %v which is greater than previous key %v",
						bucket.BucketID, i, reader.CurrentRecord.Key, lastKey)
				}
			}

			// Remember this key
			lastKey = make([]byte, len(reader.CurrentRecord.Key))
			copy(lastKey, reader.CurrentRecord.Key)
		}
	}

	// Test the bucket merging logic by reading all records in order
	allRecords := make([]*RecordLocation, 0, 100)
	
	// Create readers for all buckets
	readers := make([]*BucketReader, 0)
	for _, bucket := range bucketMgr.Buckets {
		if bucket.RecordCount > 0 {
			// Rewind the bucket file
			if _, err := bucket.File.Seek(0, io.SeekStart); err != nil {
				t.Fatalf("Failed to seek to beginning of bucket %d: %v", bucket.BucketID, err)
			}
			
			// Create a reader
			reader := &BucketReader{
				Bucket: bucket,
				Index:  bucket.BucketID,
			}
			
			// Read the first record
			if err := reader.ReadNext(); err != nil {
				if err != io.EOF {
					t.Fatalf("Failed to read first record from bucket %d: %v", bucket.BucketID, err)
				}
				// Skip empty buckets
				continue
			}
			
			readers = append(readers, reader)
		}
	}
	
	// Merge records from all buckets
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
		allRecords = append(allRecords, record)
		
		// Read the next record from this bucket
		if err := readers[minIndex].ReadNext(); err != nil {
			if err == io.EOF {
				// Remove this reader from the list
				readers = append(readers[:minIndex], readers[minIndex+1:]...)
			} else {
				t.Fatalf("Failed to read next record: %v", err)
			}
		}
	}
	
	// Verify all records were read
	if len(allRecords) != 100 {
		t.Errorf("Expected to read 100 records, but got %d", len(allRecords))
	}
	
	// Verify records are in sorted order
	for i := 1; i < len(allRecords); i++ {
		if bytes.Compare(allRecords[i].Key, allRecords[i-1].Key) > 0 {
			t.Errorf("Records are not sorted correctly. Record %d has key %v which is greater than previous key %v",
				i, allRecords[i].Key, allRecords[i-1].Key)
		}
	}
}

// TestBucketManagerAddRecord tests the AddRecord method of BucketManager
func TestBucketManagerAddRecord(t *testing.T) {
	// Create a temporary directory for test buckets
	tempDir, err := os.MkdirTemp("", "bucket-test-")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a bucket manager with 256 buckets (one for each possible first byte)
	bucketMgr, err := NewBucketManager(256, tempDir)
	if err != nil {
		t.Fatalf("Failed to create bucket manager: %v", err)
	}
	defer bucketMgr.Cleanup()

	// Add records with different first bytes
	for i := 0; i < 256; i++ {
		key := []byte{byte(i), 0x01, 0x02, 0x03}
		record := &RecordLocation{
			Key:           key,
			SnapshotIndex: 0,
			SectionIndex:  0,
			Size:          len(key) + 10,
			Type:          "test",
		}
		
		err := bucketMgr.AddRecord(record)
		if err != nil {
			t.Fatalf("Failed to add record to bucket: %v", err)
		}
	}

	// Verify each record went to the correct bucket
	for i := 0; i < 256; i++ {
		bucket := bucketMgr.Buckets[i]
		if bucket.RecordCount != 1 {
			t.Errorf("Expected bucket %d to have 1 record, but it has %d", i, bucket.RecordCount)
		}
		
		// Verify the bucket file exists
		bucketPath := filepath.Join(tempDir, fmt.Sprintf("bucket_%d.dat", i))
		if _, err := os.Stat(bucketPath); os.IsNotExist(err) {
			t.Errorf("Bucket file %s does not exist", bucketPath)
		}
	}
}
