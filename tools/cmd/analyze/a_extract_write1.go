// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"os"
	"time"

	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	recordpkg "gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// WritePartitionSnapshot writes a partition-specific snapshot by filtering records
// using a two-pass approach with a Bloom filter
func WritePartitionSnapshot(extractState *ExtractState, outputFile string, targetPartition string) error {
	fmt.Printf("Writing partition snapshot for: %s\n", targetPartition)

	// Report routing table partitions
	reportRoutingPartitions(extractState)

	// PASS 1: Create and build Bloom filter with account and chain key hashes from the target partition
	fmt.Printf("\nPASS 1: Building Bloom filter for partition %s...\n", targetPartition)
	startTime := time.Now()
	bloomFilter := NewBloom(targetPartition)

	// First pass counters
	var recordsProcessed, accountsAdded, chainsProcessed int

	// First pass: Build Bloom filter with accounts and chains from target partition
	fmt.Printf("Processing %d records to build Bloom filter...\n", len(extractState.Records))
	for _, record := range extractState.Records {
		recordsProcessed++

		// Process account records
		if record.Type == "account" {
			// Unmarshal the key
			key := new(recordpkg.Key)
			err := key.UnmarshalBinary(record.Key)
			if err != nil {
				// Skip records with invalid keys
				continue
			}

			// Extract account URL from the key
			accountURL, err := extractAccountURL(key)
			if err != nil {
				// Skip records where we can't extract the URL
				// This includes KeyHash keys and other non-Account keys
				continue
			}

			// Check if account belongs to target partition
			if belongsToPartition(accountURL, targetPartition, extractState.Router) {
				// Add key hash to Bloom filter
				bloomFilter.Add(record.KeyHash[:])
				accountsAdded++
			}
		} else if record.Type == "chain" {
			// Extract account URL from chain URL
			if record.URL != "" {
				chainURL, err := url.Parse(record.URL)
				if err == nil && chainURL.Authority != "" {
					// Check if the account belongs to target partition - use consistent case comparison
					if belongsToPartition(chainURL, targetPartition, extractState.Router) {
						// Add chain key hash to Bloom filter
						bloomFilter.Add(record.KeyHash[:])
						chainsProcessed++
					}
				}
			}
		}

		// Progress reporting for large snapshots
		if recordsProcessed%100000 == 0 {
			fmt.Printf("  Processed %d records, added %d accounts, %d chains\n",
				recordsProcessed, accountsAdded, chainsProcessed)
		}
	}

	// Set the build time for statistics
	bloomFilter.Stats.BuildTime = time.Since(startTime)

	// Report first pass statistics
	fmt.Printf("\nPASS 1 Complete: Bloom filter built in %v\n", bloomFilter.Stats.BuildTime)
	fmt.Printf("  Records processed: %d\n", recordsProcessed)
	fmt.Printf("  Accounts added to filter: %d\n", accountsAdded)
	fmt.Printf("  Chains added to filter: %d\n", chainsProcessed)
	fmt.Printf("  Bloom filter false positive rate: %.6f%%\n", bloomFilter.EstimateFalsePositiveRate()*100)

	// PASS 2: Write filtered records based on Bloom filter membership
	fmt.Printf("\nPASS 2: Writing filtered records to partition snapshot...\n")
	secondPassStart := time.Now()

	// Create output file for the filtered snapshot
	output, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer output.Close()

	// Create snapshot writer
	writer, err := sv2.Create(output)
	if err != nil {
		return fmt.Errorf("create snapshot writer: %w", err)
	}

	// Write header section
	header := &sv2.Header{
		Version: sv2.Version2,
	}
	// Copy system ledger if available
	if extractState.SnapshotReader != nil && extractState.SnapshotReader.Header != nil &&
		extractState.SnapshotReader.Header.SystemLedger != nil {
		header.SystemLedger = extractState.SnapshotReader.Header.SystemLedger
	}

	err = writer.WriteHeader(header)
	if err != nil {
		return fmt.Errorf("write snapshot header: %w", err)
	}

	// Create a records section for filtered records
	recordSection, err := writer.OpenRaw(sv2.SectionTypeRecords)
	if err != nil {
		return fmt.Errorf("create records section: %w", err)
	}

	// Second pass counters
	var secondPassProcessed, recordsIncluded, recordsFiltered int
	// Track record types for statistics
	recordTypeStats := make(map[string]int)

	// Second pass: Filter records based on Bloom filter membership
	for _, record := range extractState.Records {
		secondPassProcessed++

		if bloomFilter.Test(record.KeyHash[:]) {

			// Create a new Key object from the original serialized key bytes
			key := new(recordpkg.Key)
			err := key.UnmarshalBinary(record.Key)
			if err != nil {
				return fmt.Errorf("unmarshal key: %w", err)
			}

			entry := &sv2.RecordEntry{
				Key:   key, // Use the unmarshaled key directly
				Value: record.Value,
			}

			// Write the record to the output snapshot
			err = recordSection.WriteValue(entry)
			if err != nil {
				return fmt.Errorf("write record: %w", err)
			}

			// Update statistics
			recordsIncluded++
			recordTypeStats[record.Type]++
		} else {
			// Record is not included
			recordsFiltered++
		}

		// Progress reporting for large snapshots
		if secondPassProcessed%100000 == 0 {
			fmt.Printf("  Processed %d records, included %d, filtered out %d\n",
				secondPassProcessed, recordsIncluded, recordsFiltered)
		}
	}

	// Close the records section
	err = recordSection.Close()
	if err != nil {
		return fmt.Errorf("close records section: %w", err)
	}

	// Print summary statistics
	fmt.Printf("\nPartition Snapshot Statistics for %s:\n", targetPartition)
	fmt.Printf("  Total records processed: %d\n", secondPassProcessed)
	fmt.Printf("  Records included: %d\n", recordsIncluded)
	fmt.Printf("  Records filtered out: %d\n", recordsFiltered)
	fmt.Printf("  Processing time: %v\n", time.Since(secondPassStart))

	// Print record type statistics
	fmt.Printf("\nRecord Type Statistics:\n")
	for recordType, count := range recordTypeStats {
		fmt.Printf("  %s: %d\n", recordType, count)
	}

	// Get file size for reporting
	fileInfo, err := os.Stat(outputFile)
	var fileSize float64
	if err == nil {
		fileSize = float64(fileInfo.Size()) / (1024 * 1024) // Convert to MB
		fmt.Printf("\nSuccessfully wrote partition snapshot: %s (%.1f MB)\n", outputFile, fileSize)
	} else {
		fmt.Printf("\nSuccessfully wrote partition snapshot: %s\n", outputFile)
	}

	return nil
}
