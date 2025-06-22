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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBucketManagerInitialization tests the initialization of the BucketManager
func TestBucketManagerInitialization(t *testing.T) {
	// Create a temporary directory for test buckets
	tempDir, err := os.MkdirTemp("", "bucket-test-")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tempDir)

	// Test with different bucket counts
	bucketCounts := []int{1, 16, 256}
	for _, count := range bucketCounts {
		t.Run(fmt.Sprintf("BucketCount_%d", count), func(t *testing.T) {
			// Create a bucket manager
			bucketMgr, err := NewBucketManager(count, tempDir)
			require.NoError(t, err, "Failed to create bucket manager with %d buckets", count)
			defer bucketMgr.Cleanup()

			// Verify bucket count
			assert.Equal(t, count, len(bucketMgr.Buckets), "BucketManager should have %d buckets", count)

			// Verify each bucket is initialized correctly
			for i, bucket := range bucketMgr.Buckets {
				assert.Equal(t, i, bucket.BucketID, "Bucket ID should match index")
				assert.Equal(t, 0, bucket.RecordCount, "New bucket should have 0 records")
				
				// Verify bucket file path
				expectedPath := filepath.Join(tempDir, fmt.Sprintf("bucket-%04d.tmp", i))
				assert.Equal(t, expectedPath, bucket.Path, "Bucket file path should be correct")
				
				// Verify bucket file exists
				_, err := os.Stat(bucket.Path)
				assert.NoError(t, err, "Bucket file should exist")
			}
		})
	}
}

// TestBucketAddRecord tests adding records to buckets
func TestBucketAddRecord(t *testing.T) {
	// Create a temporary directory for test buckets
	tempDir, err := os.MkdirTemp("", "bucket-test-")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tempDir)

	// Create a bucket manager with 16 buckets
	bucketMgr, err := NewBucketManager(16, tempDir)
	require.NoError(t, err, "Failed to create bucket manager")
	defer bucketMgr.Cleanup()

	// Create test records
	testRecords := make([]*RecordLocation, 0, 100)
	for i := 0; i < 100; i++ {
		// Create keys with different first bytes to distribute across buckets
		keyByte := byte(i % 16) // This will distribute keys evenly across buckets
		
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

	// Add records to buckets
	for _, record := range testRecords {
		err := bucketMgr.AddRecord(record)
		require.NoError(t, err, "Failed to add record to bucket")
	}

	// Verify records were distributed across buckets
	totalRecords := 0
	for i, bucket := range bucketMgr.Buckets {
		// Each bucket should have approximately the same number of records
		expectedCount := 100 / 16 // 6 or 7 records per bucket
		assert.GreaterOrEqual(t, bucket.RecordCount, expectedCount-1, "Bucket %d should have approximately %d records", i, expectedCount)
		assert.LessOrEqual(t, bucket.RecordCount, expectedCount+1, "Bucket %d should have approximately %d records", i, expectedCount)
		
		totalRecords += bucket.RecordCount
	}

	// Verify total record count
	assert.Equal(t, 100, totalRecords, "Total record count should match")
}

// TestBucketSort tests sorting records within buckets
func TestBucketSort(t *testing.T) {
	// Create a temporary directory for test buckets
	tempDir, err := os.MkdirTemp("", "bucket-test-")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tempDir)

	// Create a bucket manager with 4 buckets
	bucketMgr, err := NewBucketManager(4, tempDir)
	require.NoError(t, err, "Failed to create bucket manager")
	defer bucketMgr.Cleanup()

	// Create test records with non-sequential keys
	testRecords := make([]*RecordLocation, 0, 20)
	for i := 0; i < 20; i++ {
		// Use a non-sequential pattern to ensure records need sorting
		// All records will go to the same bucket for this test
		key := make([]byte, 4)
		key[0] = 0 // All records go to bucket 0
		key[1] = 0
		key[2] = byte((i * 7) % 20) // Non-sequential pattern
		key[3] = 0xFF

		record := &RecordLocation{
			Key:           key,
			SnapshotIndex: 0,
			SectionIndex:  0,
			Size:          len(key) + 10,
			Type:          "test",
			// Position will be set when the record is written to the bucket file
		}
		
		testRecords = append(testRecords, record)
	}

	// Add records to bucket 0
	for _, record := range testRecords {
		err := bucketMgr.AddRecord(record)
		require.NoError(t, err, "Failed to add record to bucket")
	}

	// Verify all records went to bucket 0
	assert.Equal(t, 20, bucketMgr.Buckets[0].RecordCount, "All records should be in bucket 0")
	for i := 1; i < 4; i++ {
		assert.Equal(t, 0, bucketMgr.Buckets[i].RecordCount, "Bucket %d should be empty", i)
	}

	// Sort bucket 0
	err = bucketMgr.Buckets[0].Sort()
	require.NoError(t, err, "Failed to sort bucket 0")

	// Rewind the bucket file
	_, err = bucketMgr.Buckets[0].File.Seek(0, io.SeekStart)
	require.NoError(t, err, "Failed to seek to beginning of bucket file")

	// Read all records from bucket 0
	reader := &BucketReader{
		Bucket: bucketMgr.Buckets[0],
		Index:  0,
	}

	// Read and verify records are sorted
	records := make([]*RecordLocation, 0)
	var lastKey []byte

	for i := 0; i < 20; i++ {
		err := reader.ReadNext()
		require.NoError(t, err, "Failed to read record %d", i)

		record := reader.CurrentRecord
		records = append(records, record)

		// Verify records are in ascending order
		if lastKey != nil {
			assert.Less(t, bytes.Compare(lastKey, record.Key), 0, 
				"Records should be sorted in ascending order. Record %d key %v should be greater than previous key %v", 
				i, record.Key, lastKey)
		}
		
		// Verify Position field is set
		assert.GreaterOrEqual(t, record.Position, int64(0), "Position field should be set")
		
		lastKey = make([]byte, len(record.Key))
		copy(lastKey, record.Key)
	}

	// Verify we got all records
	assert.Equal(t, 20, len(records), "Should have read all records")
	
	// Verify the first and last records have the expected keys
	assert.Equal(t, byte(0), records[0].Key[2], "First record should have key[2] = 0")
	assert.Equal(t, byte(19), records[19].Key[2], "Last record should have key[2] = 19")
	
	// Verify records have unique positions
	positionMap := make(map[int64]bool)
	for _, record := range records {
		// Ensure position is not already in the map
		assert.False(t, positionMap[record.Position], "Position %d should be unique", record.Position)
		positionMap[record.Position] = true
	}
}

// TestBucketEndToEnd tests the complete bucket sorting process
func TestBucketEndToEnd(t *testing.T) {
	// Create a temporary directory for test buckets
	tempDir, err := os.MkdirTemp("", "bucket-test-")
	require.NoError(t, err, "Failed to create temp directory")
	defer os.RemoveAll(tempDir)

	// Create a bucket manager with 16 buckets
	bucketMgr, err := NewBucketManager(16, tempDir)
	require.NoError(t, err, "Failed to create bucket manager")
	defer bucketMgr.Cleanup()

	// Create test records with random keys
	testRecords := make([]*RecordLocation, 0, 100)
	for i := 0; i < 100; i++ {
		// Use a non-sequential pattern to ensure records need sorting
		keyByte := byte((i * 7) % 16) // This will distribute keys across buckets in non-sequential order
		
		key := make([]byte, 4)
		key[0] = keyByte
		key[1] = byte((i * 3) % 16) // More randomness in the second byte
		key[2] = byte((i * 5) % 16) // More randomness in the third byte
		key[3] = 0xFF

		record := &RecordLocation{
			Key:           key,
			SnapshotIndex: 0,
			SectionIndex:  0,
			Size:          len(key) + 10,
			Type:          "test",
			// Position will be set when the record is written to the bucket file
		}
		
		testRecords = append(testRecords, record)
	}

	// Add records to buckets
	for _, record := range testRecords {
		err := bucketMgr.AddRecord(record)
		require.NoError(t, err, "Failed to add record to bucket")
	}

	// Count non-empty buckets and total records before sorting
	nonEmptyBuckets := 0
	totalRecords := 0
	for _, bucket := range bucketMgr.Buckets {
		if bucket.RecordCount > 0 {
			nonEmptyBuckets++
			totalRecords += bucket.RecordCount
		}
	}
	
	// Verify total records match what we added
	assert.Equal(t, 100, totalRecords, "Total record count should match")
	
	// Sort all buckets
	for i, bucket := range bucketMgr.Buckets {
		if bucket.RecordCount > 0 {
			// Track time for performance testing
			startTime := time.Now()
			
			err := bucket.Sort()
			require.NoError(t, err, "Failed to sort bucket %d", i)
			
			// Log sorting time for performance analysis
			sortDuration := time.Since(startTime)
			t.Logf("Sorted bucket %d with %d records in %.3f seconds", 
				i, bucket.RecordCount, sortDuration.Seconds())
		}
	}

	// Create readers for all buckets
	readers := make([]*BucketReader, 0)
	for _, bucket := range bucketMgr.Buckets {
		if bucket.RecordCount > 0 {
			// Rewind the bucket file
			_, err := bucket.File.Seek(0, io.SeekStart)
			require.NoError(t, err, "Failed to seek to beginning of bucket")
			
			// Create a reader
			reader := &BucketReader{
				Bucket: bucket,
				Index:  bucket.BucketID,
			}
			
			// Read the first record
			err = reader.ReadNext()
			if err == io.EOF {
				// Skip empty buckets
				continue
			}
			require.NoError(t, err, "Failed to read first record from bucket %d", bucket.BucketID)
			
			readers = append(readers, reader)
		}
	}

	// Merge records from all buckets
	var allRecords []*RecordLocation
	var lastKey []byte
	positionMap := make(map[int64]bool) // Track positions to ensure uniqueness
	
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
		
		// Verify keys are in ascending order
		if lastKey != nil {
			assert.Less(t, bytes.Compare(lastKey, record.Key), 0, 
				"Records should be in ascending order. Record key %v should be greater than previous key %v", 
				record.Key, lastKey)
		}
		
		// Verify Position field is set
		assert.GreaterOrEqual(t, record.Position, int64(0), "Position field should be set")
		
		// Verify position is unique
		assert.False(t, positionMap[record.Position], "Position %d should be unique", record.Position)
		positionMap[record.Position] = true
		
		// Remember this key
		lastKey = make([]byte, len(record.Key))
		copy(lastKey, record.Key)
		
		// Read the next record from this bucket
		err := readers[minIndex].ReadNext()
		if err == io.EOF {
			// Remove this reader from the list
			readers = append(readers[:minIndex], readers[minIndex+1:]...)
		} else {
			require.NoError(t, err, "Failed to read next record from bucket %d", readers[minIndex].Index)
		}
	}

	// Verify we got all records
	assert.Equal(t, 100, len(allRecords), "Should have read all records")
	
	// Verify all positions are unique
	assert.Equal(t, 100, len(positionMap), "All positions should be unique")
}
