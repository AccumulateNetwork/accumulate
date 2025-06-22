// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

// SnapshotIndex represents a record's location in a snapshot file
type SnapshotIndex struct {
	Offset int64 // Byte offset in the file
	Size   int   // Size of the record in bytes
}

// RecordLocation represents a record's location and metadata
type RecordLocation struct {
	Key           []byte // Binary key
	KeyHash       []byte // Hash of the key for sorting
	SnapshotIndex int    // Index of the snapshot containing the record
	SectionIndex  int    // Index of the section containing the record
	Position      int64  // Position of the record within the section
	Size          int    // Size of the record in bytes
	Type          string // Type of the record
}

// RecordWithData holds both the record location and its data for in-memory sorting
type RecordWithData struct {
	Key       []byte // Binary key
	Value     []byte // Binary value
	Type      string // Record type
	SourceIdx int    // Index of the source snapshot
}

// CombineStats tracks statistics about the snapshot combining process
type CombineStats struct {
	InputSnapshots   int
	RecordsRead      int
	RecordsSorted    int
	RecordsWritten   int
	RecordsByType    map[string]int
	DuplicateKeys    int
	BatchesProcessed int
	BucketsSorted    int
	TimeElapsed      int64 // In milliseconds
}
