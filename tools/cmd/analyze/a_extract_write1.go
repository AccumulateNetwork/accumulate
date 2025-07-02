// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	recordpkg "gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
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
		if record.Type == "account" && record.URL != "" {
			// Parse the account URL using the Accumulate URL package
			accountURL, err := url.Parse(record.URL)
			if err != nil {
				continue
			}

			// Check if account belongs to target partition - use consistent case comparison
			if router, ok := extractState.Router.(routing.Router); ok {
				if belongsToPartition(accountURL, targetPartition, router) {
					// Add account key hash to Bloom filter
					bloomFilter.Add(record.KeyHash[:])
					accountsAdded++
				}
			}
		} else if record.Type == "chain" {
			// Extract account URL from chain URL
			if record.URL != "" {
				chainURL, err := url.Parse(record.URL)
				if err == nil && chainURL.Authority != "" {
					// Check if the account belongs to target partition - use consistent case comparison
					if router, ok := extractState.Router.(routing.Router); ok {
						if belongsToPartition(chainURL, targetPartition, router) {
							// Add chain key hash to Bloom filter
							bloomFilter.Add(record.KeyHash[:])
							chainsProcessed++
						}
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

	// Special handling for DN partition - include all messages and transactions
	// Use case-insensitive comparison for consistency
	isDNPartition := strings.EqualFold(targetPartition, "directory") ||
		strings.Contains(strings.ToLower(targetPartition), "dn")

	// Second pass: Filter records based on Bloom filter membership
	for _, record := range extractState.Records {
		secondPassProcessed++

		// Check if record should be included
		includeRecord := false

		// For DN partition, include all messages and transactions
		if isDNPartition && (record.Type == "message" || record.Type == "transaction") {
			includeRecord = true
		} else {
			// For all partitions, check Bloom filter for accounts and chains
			includeRecord = bloomFilter.Test(record.KeyHash[:])
		}

		if includeRecord {
			// Convert the byte array key to a proper *record.Key using KeyFromHash
			var keyHash [32]byte
			copy(keyHash[:], record.KeyHash[:])

			entry := &sv2.RecordEntry{
				Key:   recordpkg.KeyFromHash(keyHash), // Use the record package's KeyFromHash function
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
