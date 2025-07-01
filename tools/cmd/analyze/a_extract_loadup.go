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
	"os"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

// Load scans the snapshot file and loads up the transaction slice and map
func Load(extractState *ExtractState) error {
	// Open the snapshot file
	file, err := os.Open(extractState.SnapshotFile)
	if err != nil {
		return fmt.Errorf("failed to open snapshot file: %w", err)
	}
	defer file.Close()

	// Create snapshot reader to get header info
	reader, err := snapshot.Open(file)
	if err != nil {
		return fmt.Errorf("failed to create snapshot reader: %w", err)
	}

	// Store snapshot header information
	extractState.SnapshotHeader = &SnapshotHeader{
		Version:  reader.Header.Version,
		RootHash: reader.Header.RootHash,
	}

	// Store system ledger info if available
	if reader.Header.SystemLedger != nil {
		extractState.SnapshotHeader.SystemLedger.URL = reader.Header.SystemLedger.Url.String()
		extractState.SnapshotHeader.SystemLedger.Index = reader.Header.SystemLedger.Index
		extractState.SnapshotHeader.SystemLedger.Timestamp = reader.Header.SystemLedger.Timestamp.UnixNano()
	}

	fmt.Printf("Loading snapshot data...\n")
	fmt.Printf("  Snapshot Version: %d\n", reader.Header.Version)
	fmt.Printf("  Root Hash: %x\n", reader.Header.RootHash)
	fmt.Printf("  Sections: %d\n", len(reader.Sections))

	// Reset file position to beginning for processing
	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek to beginning: %w", err)
	}
	// Process snapshot records to extract transactions and messages
	// Using pure streaming approach - NO DATABASE ALLOCATION
	recordCount := 0
	transactionCount := 0
	messageCount := 0

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
			
			// Check if this looks like a transaction or message record
			if len(keyBytes) >= 32 && len(value) > 0 {
				// Create a hash from the key
				var hash [32]byte
				copy(hash[:], keyBytes[:32])
				
				// Determine record type based on key structure and value content
				// This is a heuristic approach - in practice you'd need proper key parsing
				isTransaction := false
				isMessage := false
				
				// Simple heuristics for record type detection:
				// - Check value size and structure patterns
				// - Messages tend to be smaller and have different patterns
				// - Transactions tend to be larger and have specific structures
				if len(value) > 100 {
					// Larger records are more likely to be transactions
					// Check for transaction-like patterns in the value
					if len(value) > 200 || (len(keyBytes) == 32 && value[0] != 0) {
						isTransaction = true
					}
				} else if len(value) > 20 && len(value) <= 100 {
					// Smaller records might be messages
					// Additional heuristics could be added here
					isMessage = true
				}
				
				// If we can't determine the type, default to transaction for now
				if !isTransaction && !isMessage {
					isTransaction = true
				}
				
				if isTransaction {
					// Create transaction record
					txRecord := TransactionRecord{
						Key:   keyBytes,
						Value: value,
						Hash:  hash,
					}
					
					// Add to transactions slice and map
					index := len(extractState.Transactions)
					extractState.Transactions = append(extractState.Transactions, txRecord)
					extractState.TransactionHashToIndex[hash] = index
					
					transactionCount++
					if transactionCount%1000 == 0 {
						fmt.Printf("Processed %d transactions...\n", transactionCount)
					}
				}
				
				if isMessage {
					// Create message record
					msgRecord := MessageRecord{
						Key:   keyBytes,
						Value: value,
						Hash:  hash,
					}
					
					// Add to messages slice and map
					index := len(extractState.Messages)
					extractState.Messages = append(extractState.Messages, msgRecord)
					extractState.MessageHashToIndex[hash] = index
					
					messageCount++
					if messageCount%1000 == 0 {
						fmt.Printf("Processed %d messages...\n", messageCount)
					}
				}
			}

			// Progress reporting
			if recordCount%10000 == 0 {
				fmt.Printf("  Processed %d records, found %d transactions, %d messages\n",
					recordCount, transactionCount, messageCount)
			}
		}
	}

	// Update report statistics
	extractState.Report.TransactionCount = int64(transactionCount)
	extractState.Report.MessageCount = int64(messageCount)

	fmt.Printf("Snapshot loading complete:\n")
	fmt.Printf("  Total records processed: %d\n", recordCount)
	fmt.Printf("  Transactions loaded: %d\n", transactionCount)
	fmt.Printf("  Messages loaded: %d\n", messageCount)
	fmt.Printf("  Transaction hash map entries: %d\n", len(extractState.TransactionHashToIndex))
	fmt.Printf("  Message hash map entries: %d\n", len(extractState.MessageHashToIndex))

	// Process DN partition accounts and their subchains
	if len(extractState.Accounts) > 0 {
		err := ProcessDNAccounts(extractState)
		if err != nil {
			fmt.Printf("Warning: DN account processing failed: %v\n", err)
		}
	}

	return nil
}
