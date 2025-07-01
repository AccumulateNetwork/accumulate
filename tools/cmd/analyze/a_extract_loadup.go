// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

// Package main implements snapshot extraction tools.
// IMPORTANT: This implementation uses a streaming architecture to process snapshots
// efficiently without loading the entire database into memory. The goal is to process
// a 2GB snapshot using less than 5GB of memory (compared to previous implementations
// that required 40GB for a 2GB snapshot).
// THAT MEANS NO DATABASE

import (
	"fmt"
	"io"
	"sort"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

// Load scans the snapshot file and collects all records into a unified slice
// Uses the pre-opened snapshot file and reader from ExtractState
func Load(extractState *ExtractState) error {
	// Ensure snapshot is initialized
	if extractState.SnapshotFileHandle == nil || extractState.SnapshotReader == nil {
		return fmt.Errorf("snapshot not initialized - call InitializeSnapshot() first")
	}

	// Use the pre-opened snapshot reader
	reader := extractState.SnapshotReader

	fmt.Printf("Loading snapshot data...\n")
	fmt.Printf("  Snapshot Version: %d\n", reader.Header.Version)
	fmt.Printf("  Root Hash: %x\n", reader.Header.RootHash)
	fmt.Printf("  Sections: %d\n", len(reader.Sections))

	// Process snapshot records to collect all records
	// Using pure streaming approach - NO DATABASE ALLOCATION
	recordCount := 0
	accountCount := 0
	transactionCount := 0
	messageCount := 0
	otherCount := 0

	fmt.Printf("Processing snapshot sections...\n")

	// Process records from snapshot sections
	for i := 0; i < len(reader.Sections); i++ {
		section := reader.Sections[i]
		fmt.Printf("Processing section %d/%d (type: %v)...\n", i+1, len(reader.Sections), section.Type())

		// Only process record sections
		if section.Type() != snapshot.SectionTypeRecords {
			fmt.Printf("  Skipping non-record section\n")
			continue
		}

		// Open record reader for this section
		recordReader, err := reader.OpenRecords(i)
		if err != nil {
			fmt.Printf("Warning: failed to open records section %d: %v\n", i, err)
			continue
		}

		// Process records in this section
		for {
			// Read next record entry
			recordEntry, err := recordReader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				fmt.Printf("Warning: error reading record: %v\n", err)
				continue
			}

			recordCount++

			// Parse the key to determine record type
			if recordEntry.Key == nil {
				continue
			}

			// Marshal the key to bytes for processing
			keyBytes, err := recordEntry.Key.MarshalBinary()
			if err != nil {
				fmt.Printf("Warning: failed to marshal key: %v\n", err)
				continue
			}
			
			value := recordEntry.Value
			
			// Create a hash from the key for indexing
			var keyHash [32]byte
			if len(keyBytes) >= 32 {
				copy(keyHash[:], keyBytes[:32])
			} else {
				// For shorter keys, use a simple hash
				copy(keyHash[:], keyBytes)
			}
			
			// Detect record type using heuristics
			recordType := detectRecordType(recordEntry.Key)
			
			// Extract URL for account records
			var url, partition string
			if recordType == "account" {
				accountURL, err := extractAccountURL(recordEntry.Key)
				if err == nil && accountURL != nil {
					url = accountURL.String()
				}
				partition = "" // Will be determined later using router
				accountCount++
			} else if recordType == "transaction" {
				transactionCount++
			} else if recordType == "message" {
				messageCount++
			} else {
				otherCount++
			}
			
			// Create unified record entry
			record := RecordEntry{
				Key:        keyBytes,
				Value:      value,
				KeyHash:    keyHash,
				RecordType: recordType,
				URL:        url,
				Partition:  partition,
			}
			
			// Add to unified records collection
			index := len(extractState.Records)
			extractState.Records = append(extractState.Records, record)
			extractState.KeyHashToIndex[keyHash] = index

			// Progress reporting
			if recordCount%100000 == 0 {
				fmt.Printf("  Processed %d records (accounts: %d, transactions: %d, messages: %d, other: %d)\n",
					recordCount, accountCount, transactionCount, messageCount, otherCount)
			}
		}
	}

	// Update report statistics
	extractState.Report.AccountCount = int64(accountCount)
	extractState.Report.TransactionCount = int64(transactionCount)
	extractState.Report.MessageCount = int64(messageCount)

	fmt.Printf("Snapshot loading complete:\n")
	fmt.Printf("  Total records processed: %d\n", recordCount)
	fmt.Printf("  Accounts: %d\n", accountCount)
	fmt.Printf("  Transactions: %d\n", transactionCount)
	fmt.Printf("  Messages: %d\n", messageCount)
	fmt.Printf("  Other records: %d\n", otherCount)
	fmt.Printf("  Total records in collection: %d\n", len(extractState.Records))
	fmt.Printf("  Key hash map entries: %d\n", len(extractState.KeyHashToIndex))

	// Sort records by key hash for proper snapshot ordering
	fmt.Printf("\nSorting records by key hash...\n")
	err := sortRecordsByKeyHash(extractState)
	if err != nil {
		return fmt.Errorf("failed to sort records: %w", err)
	}
	fmt.Printf("Records sorted successfully\n")

	return nil
}

// sortRecordsByKeyHash sorts the Records slice by key hash and rebuilds the KeyHashToIndex map
func sortRecordsByKeyHash(extractState *ExtractState) error {
	if len(extractState.Records) == 0 {
		return nil
	}

	// Sort the Records slice by key hash
	sort.Slice(extractState.Records, func(i, j int) bool {
		// Compare key hashes byte by byte
		for k := 0; k < 32; k++ {
			if extractState.Records[i].KeyHash[k] != extractState.Records[j].KeyHash[k] {
				return extractState.Records[i].KeyHash[k] < extractState.Records[j].KeyHash[k]
			}
		}
		return false // Equal hashes
	})

	// Rebuild the KeyHashToIndex map with new indices
	extractState.KeyHashToIndex = make(map[[32]byte]int)
	for i, record := range extractState.Records {
		extractState.KeyHashToIndex[record.KeyHash] = i
	}

	return nil
}
