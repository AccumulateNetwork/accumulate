// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestReadSnapshots tests reading multiple snapshots into the database
func TestReadSnapshots(t *testing.T) {
	// Create a new SnapCombine instance
	sc := &SnapCombine{
		InputPaths:     []string{"testdata/bvn0.snap", "testdata/bvn1.snap"},
		OutputPath:     "testdata/combined.snap",
		RecordsByType:  make(map[string][]int),
		AccountRecords: make(map[string][]int),
		RecordTypes:    make(map[string]int),
		useMemory:      false, // Use file-based storage
	}

	// Initialize the SnapCombine instance
	err := sc.Initialize()
	require.NoError(t, err, "Failed to initialize SnapCombine")
	defer sc.Cleanup()

	// Read each snapshot
	for _, path := range sc.InputPaths {
		// Check if the file exists
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Logf("Skipping non-existent snapshot file: %s", path)
			continue
		}

		// Read the snapshot
		err := sc.ReadSnapshot(path)
		require.NoError(t, err, "Failed to read snapshot: %s", path)
	}

	// Print statistics
	t.Logf("Snapshots read: %d", sc.SnapshotsRead)
	t.Logf("Records read: %d", sc.RecordsRead)
	
	// Print record type statistics
	t.Logf("Record types:")
	for recordType, count := range sc.RecordTypes {
		t.Logf("  %s: %d", recordType, count)
	}

	// Verify the database contains records
	// We'll check the database size
	dbPath := sc.dbPath
	dbSize, err := getDirSize(dbPath)
	require.NoError(t, err, "Failed to get database size")
	t.Logf("Database size: %d bytes", dbSize)
	require.Greater(t, dbSize, int64(0), "Database should not be empty")
}

// Helper function to get directory size
func getDirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}
