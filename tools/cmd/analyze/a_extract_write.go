// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// WritePartitionSnapshot writes a partition-specific snapshot by filtering accounts
func WritePartitionSnapshot(extractState *ExtractState, outputFile string, targetPartition string) error {
	fmt.Printf("Writing partition snapshot for: %s\n", targetPartition)
	fmt.Printf("Output file: %s\n", outputFile)

	// Statistics counters
	accountCount := 0
	transactionCount := 0
	messageCount := 0
	otherRecordCount := 0
	mainChainCount := 0
	anchorChainCount := 0
	otherChainCount := 0
	totalChainCount := 0
	recordCount := 0

	// Create output file
	file, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer file.Close()

	// Create snapshot writer
	writer, err := sv2.Create(file)
	if err != nil {
		return fmt.Errorf("create snapshot writer: %w", err)
	}

	// Write snapshot header
	// Convert the SystemLedger struct to protocol.SystemLedger
	// Handle empty SystemLedger URL by providing a default
	systemLedgerURLStr := extractState.SnapshotHeader.SystemLedger.URL
	if systemLedgerURLStr == "" {
		systemLedgerURLStr = "acc://system/ledger" // Default system ledger URL
	}
	systemLedgerURL, err := url.Parse(systemLedgerURLStr)
	if err != nil {
		return fmt.Errorf("parse system ledger URL: %w", err)
	}
	
	err = writer.WriteHeader(&sv2.Header{
		Version:  sv2.Version2,
		RootHash: extractState.SnapshotHeader.RootHash,
		SystemLedger: &protocol.SystemLedger{
			Url:       systemLedgerURL,
			Index:     extractState.SnapshotHeader.SystemLedger.Index,
			Timestamp: time.Unix(0, extractState.SnapshotHeader.SystemLedger.Timestamp),
		},
	})
	if err != nil {
		return fmt.Errorf("write snapshot header: %w", err)
	}

	fmt.Printf("Writing partition snapshot for: %s\n", targetPartition)
	fmt.Printf("Output file: %s\n", outputFile)

	// Re-open the original snapshot file for reading
	originalFile, err := os.Open(extractState.SnapshotFile)
	if err != nil {
		return fmt.Errorf("open original snapshot file: %w", err)
	}
	defer originalFile.Close()

	// Open the snapshot reader
	reader, err := sv2.Open(originalFile)
	if err != nil {
		return fmt.Errorf("open snapshot reader: %w", err)
	}

	// Process each section in the original snapshot
	for i, section := range reader.Sections {
		sectionType := section.Type()
		
		// Skip BPT sections as requested
		if sectionType == sv2.SectionTypeBPT || sectionType == sv2.SectionTypeRawBPT {
			fmt.Printf("Skipping BPT section %d (type %v)\n", i, sectionType)
			continue
		}
		
		// Skip header section - already written
		if sectionType == sv2.SectionTypeHeader {
			continue
		}
		
		fmt.Printf("Processing section %d (type %v)\n", i, sectionType)
		
		// Handle different section types
		switch sectionType {
		case sv2.SectionTypeRecords:
			// Open records section for writing
			collector, err := writer.OpenRecords()
			if err != nil {
				return fmt.Errorf("open records section: %w", err)
			}
			
			// Open the records section for reading
			recordReader, err := reader.OpenRecords(i)
			if err != nil {
				collector.Close()
				return fmt.Errorf("failed to open records section %d: %w", i, err)
			}

			// Iterate through all records in this section
			for {
				// Read next record entry
				entry, err := recordReader.Read()
				if err == io.EOF {
					break
				}
				if err != nil {
					return fmt.Errorf("error reading record: %w", err)
				}
				recordCount++

				// Determine if we should include this record
				shouldInclude := false
				recordType := "unknown"
				
				// For Directory partition, include ALL records and classify them properly
				if targetPartition == "Directory" {
					shouldInclude = true
					// Classify the record type using our detection logic
					recordType = detectRecordType(entry.Key)
					// Debug output removed - counters working correctly
					// Debug: print first few records of each type
					if (transactionCount + messageCount + accountCount + otherRecordCount) < 10 {
						keyBytes, _ := entry.Key.MarshalBinary()
						maxLen := 100
						if len(keyBytes) < maxLen {
							maxLen = len(keyBytes)
						}
						fmt.Printf("DEBUG: Record type %s: %q\n", recordType, string(keyBytes)[:maxLen])
					}
				} else {
					// For other partitions, we would need partition-specific logic
					shouldInclude = false
					recordType = "unknown"
				}

				if shouldInclude {
					// Write the record to the output snapshot
					err = collector.WriteRecord(entry)
					if err != nil {
						collector.Close()
						return fmt.Errorf("failed to write record: %w", err)
					}
					
					// Count the record by type
					switch recordType {
					case "transaction":
						transactionCount++
					case "message":
						messageCount++
					case "account":
						accountCount++
						// Count chain types for accounts
						chainType := detectChainType(entry.Key)
						switch chainType {
						case "main":
							mainChainCount++
						case "anchor":
							anchorChainCount++
						default:
							otherChainCount++
						}
						totalChainCount++
					default:
						otherRecordCount++
					}
				}
			}
			
			collector.Close()
			
		default:
			// For other section types, copy them as-is using raw section copying
			// This handles sections like SectionTypeRecordIndex, SectionTypeConsensus, etc.
			err = copySectionRaw(writer, reader, i, sectionType)
			if err != nil {
				return fmt.Errorf("failed to copy section %d (type %v): %w", i, sectionType, err)
			}
		}
	}

	// Print detailed statistics
	totalRecords := accountCount + transactionCount + messageCount + otherRecordCount
	fmt.Printf("\n=== Partition Snapshot Statistics for %s ===\n", targetPartition)
	fmt.Printf("Total records written: %d\n", totalRecords)
	fmt.Printf("  Accounts: %d\n", accountCount)
	fmt.Printf("  Transactions: %d\n", transactionCount)
	fmt.Printf("  Messages: %d\n", messageCount)
	fmt.Printf("  Other records: %d\n", otherRecordCount)
	fmt.Printf("\nChain Statistics:\n")
	fmt.Printf("  Total chains: %d\n", totalChainCount)
	fmt.Printf("  Main chains: %d\n", mainChainCount)
	fmt.Printf("  Anchor chains: %d\n", anchorChainCount)
	fmt.Printf("  Other chains: %d\n", otherChainCount)
	fmt.Printf("\nSuccessfully wrote partition snapshot: %s\n", outputFile)
	return nil
}

// extractAccountURL extracts the account URL from a record key
func extractAccountURL(key *record.Key) (*url.URL, error) {
	if key == nil {
		return nil, fmt.Errorf("key is nil")
	}

	// Check if this is an account record by examining the key structure
	// Account records typically have a different structure than transaction/message records
	
	// Try to get the account URL from the key if it's an account record
	// This is a heuristic approach based on key structure analysis
	
	// For now, let's assume that if the key can be converted to a meaningful URL,
	// it's an account record. Otherwise, it's likely a transaction or message.
	
	// Marshal the key to analyze its structure
	keyBytes, err := key.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal key: %w", err)
	}

	// Convert to string and look for URL patterns
	keyStr := string(keyBytes)
	
	// Look for acc:// URLs which indicate account records
	if strings.Contains(keyStr, "acc://") {
		// Find the start of the URL
		start := strings.Index(keyStr, "acc://")
		if start >= 0 {
			// Extract the URL portion
			urlPart := keyStr[start:]
			// Find the end of the URL (look for null byte or other delimiter)
			if nullIndex := strings.Index(urlPart, "\x00"); nullIndex > 0 {
				urlPart = urlPart[:nullIndex]
			}
			// Try to parse the URL using Accumulate's URL package
			accountURL, err := url.Parse(urlPart)
			if err == nil {
				return accountURL, nil
			}
		}
	}

	// If we can't find an acc:// URL, this is likely not an account record
	return nil, fmt.Errorf("key does not contain account URL")
}

// belongsToPartition determines if an account URL belongs to the specified DN partition
// Uses the router interface to determine the correct partition for an account
func belongsToPartition(accountURL *url.URL, targetPartition string, router routing.Router) bool {
	if accountURL == nil || router == nil {
		return false
	}

	// Use the router to determine which partition this account belongs to
	partition, err := router.RouteAccount(accountURL)
	if err != nil {
		// If routing fails, fall back to heuristic approach
		return belongsToPartitionHeuristic(accountURL, targetPartition)
	}

	// Check if the routed partition matches our target partition
	return partition == targetPartition
}

// belongsToPartitionHeuristic is a fallback heuristic approach
func belongsToPartitionHeuristic(accountURL *url.URL, targetPartition string) bool {
	if accountURL == nil {
		return false
	}

	// Heuristic: Check if the URL path contains elements that match the partition
	pathElements := strings.Split(strings.Trim(accountURL.Path, "/"), "/")
	for _, element := range pathElements {
		if element == targetPartition {
			return true
		}
	}

	// Additional heuristic: Check hostname/authority for partition matching
	if strings.Contains(accountURL.Authority, targetPartition) {
		return true
	}

	return false
}

// copySectionRaw copies a section from the source snapshot to the destination snapshot as-is
// This is used for sections that don't need filtering (like indexes, consensus data, etc.)
func copySectionRaw(writer *sv2.Writer, reader *sv2.Reader, sectionIndex int, sectionType sv2.SectionType) error {
	// Open the source section for reading
	sourceReader, err := reader.Open(sectionType)
	if err != nil {
		return fmt.Errorf("failed to open source section: %w", err)
	}
	// Note: ioutil.SectionReader doesn't have a Close method
	
	// Open the destination section for writing
	destWriter, err := writer.OpenRaw(sectionType)
	if err != nil {
		return fmt.Errorf("failed to open destination section: %w", err)
	}
	defer destWriter.Close()
	
	// Copy the section data
	_, err = io.Copy(destWriter, sourceReader)
	if err != nil {
		return fmt.Errorf("failed to copy section data: %w", err)
	}
	
	return nil
}

// writeAccountChains finds and writes all chain records belonging to an account
// This is a placeholder for future implementation
func writeAccountChains(collector *sv2.Collector, accountURL *url.URL, reader *sv2.Reader) error {
	// TODO: Implement chain writing
	// This should:
	// 1. Find all chains belonging to the account
	// 2. Read chain records from the original snapshot
	// 3. Write chain records to the output snapshot
	// 4. Follow the same filtering logic as accounts
	
	fmt.Printf("TODO: Write chains for account %s\n", accountURL.String())
	return nil
}

// detectRecordTypeFromKey determines the type of record based on the key structure
// This is used when we know the record is NOT an account (extractAccountURL failed)
func detectRecordTypeFromKey(key *record.Key) string {
	if key == nil {
		return "unknown"
	}

	// Marshal the key to analyze its structure
	keyBytes, err := key.MarshalBinary()
	if err != nil {
		return "unknown"
	}

	// Convert to string for pattern matching
	keyStr := string(keyBytes)

	// More specific detection for non-account records
	if strings.Contains(keyStr, "transaction") || strings.Contains(keyStr, "txn") || 
	   strings.Contains(keyStr, "Transaction") {
		return "transaction"
	}
	if strings.Contains(keyStr, "message") || strings.Contains(keyStr, "msg") ||
	   strings.Contains(keyStr, "Message") {
		return "message"
	}

	// If we can't determine the type, assume it's a transaction since
	// the majority of non-account records are transactions
	return "transaction"
}

// detectRecordType determines the type of record based on the key
func detectRecordType(key *record.Key) string {
	if key == nil {
		return "unknown"
	}

	// Marshal the key to analyze its structure
	keyBytes, err := key.MarshalBinary()
	if err != nil {
		return "unknown"
	}

	// Convert to string for pattern matching
	keyStr := string(keyBytes)

	// Heuristic detection based on key patterns
	if strings.Contains(keyStr, "transaction") || strings.Contains(keyStr, "txn") {
		return "transaction"
	}
	if strings.Contains(keyStr, "message") || strings.Contains(keyStr, "msg") {
		return "message"
	}
	if strings.Contains(keyStr, "account") || strings.Contains(keyStr, "acc://") {
		return "account"
	}

	// Try to extract URL to determine if it's an account
	_, err = extractAccountURL(key)
	if err == nil {
		return "account"
	}

	return "other"
}

// detectChainType determines the chain type for account records
func detectChainType(key *record.Key) string {
	if key == nil {
		return "unknown"
	}

	// Marshal the key to analyze its structure
	keyBytes, err := key.MarshalBinary()
	if err != nil {
		return "unknown"
	}

	// Convert to string for pattern matching
	keyStr := string(keyBytes)

	// Heuristic detection based on key patterns
	if strings.Contains(keyStr, "main") || strings.Contains(keyStr, "MainChain") {
		return "main"
	}
	if strings.Contains(keyStr, "anchor") || strings.Contains(keyStr, "AnchorChain") {
		return "anchor"
	}
	if strings.Contains(keyStr, "chain") {
		return "other"
	}

	return "main" // Default to main chain for accounts
}
