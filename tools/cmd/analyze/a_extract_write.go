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
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// WritePartitionSnapshot writes a partition-specific snapshot by filtering accounts
func WritePartitionSnapshot(extractState *ExtractState, outputFile string, targetPartition string) error {
	fmt.Printf("Writing partition snapshot for: %s\n", targetPartition)
	fmt.Printf("Output file: %s\n", outputFile)

	// Report routing table partitions
	reportRoutingPartitions(extractState)

	// Initialize counters for statistics
	var accountCount, transactionCount, messageCount, otherRecordCount int
	var mainChainCount, anchorChainCount, otherChainCount int
	var totalProcessed, accountRecords, nonAccountRecords int
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

				// Initialize variables for this record
				shouldInclude := false
				recordType := "unknown"
				totalProcessed++
				
				// Try to extract account URL from the record key
				accountURL, err := extractAccountURL(entry.Key)
				if err != nil {
					// This is likely a message or transaction record, not an account
					// For DN partition, include all non-account records
					if targetPartition == "Directory" {
						shouldInclude = true
						nonAccountRecords++
						recordType = detectRecordTypeFromKey(entry.Key)
					}
				} else {
					// This is an account record, check partition membership
					accountRecords++
					recordType = "account"
					
					// Debug: Print first few account URLs found
					if accountRecords <= 5 {
						fmt.Printf("DEBUG: Found account URL: %s\n", accountURL.String())
					}
					
					// Type cast router with safety fallback
					if router, ok := extractState.Router.(routing.Router); ok {
						if belongsToPartition(accountURL, targetPartition, router) {
							shouldInclude = true
							// Debug: Print first few matching accounts
							if accountRecords <= 5 {
								fmt.Printf("DEBUG: Account %s belongs to partition %s\n", accountURL.String(), targetPartition)
							}
						}
					} else {
						// Fallback: include all accounts if router casting fails
						shouldInclude = true
					}
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
	fmt.Printf("\nPartition Snapshot Statistics for %s:\n", targetPartition)
	fmt.Printf("  Total records processed: %d\n", totalProcessed)
	fmt.Printf("  Account records found: %d\n", accountRecords)
	fmt.Printf("  Non-account records found: %d\n", nonAccountRecords)
	fmt.Printf("  Total records written: %d\n", recordCount)
	fmt.Printf("  Record types:\n")
	fmt.Printf("    Accounts: %d\n", accountCount)
	fmt.Printf("    Transactions: %d\n", transactionCount)
	fmt.Printf("    Messages: %d\n", messageCount)
	fmt.Printf("    Other records: %d\n", otherRecordCount)
	fmt.Printf("\nChain Statistics:\n")
	fmt.Printf("  Total chains: %d\n", totalChainCount)
	fmt.Printf("  Main chains: %d\n", mainChainCount)
	fmt.Printf("  Anchor chains: %d\n", anchorChainCount)
	fmt.Printf("  Other chains: %d\n", otherChainCount)
	// Get file size for reporting
	fileInfo, err := file.Stat()
	if err == nil {
		fmt.Printf("\nSuccessfully wrote partition snapshot: %s (%.1f MB)\n", outputFile, float64(fileInfo.Size())/(1024*1024))
	} else {
		fmt.Printf("\nSuccessfully wrote partition snapshot: %s\n", outputFile)
	}
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

	// Debug: Print routing information for first few accounts
	if accountURL.String() == "acc://system/ledger" || strings.Contains(accountURL.String(), "adi") {
		fmt.Printf("DEBUG: Account %s routed to partition '%s', target is '%s', match: %v\n", 
			accountURL.String(), partition, targetPartition, strings.EqualFold(partition, targetPartition))
	}

	// Check if the routed partition matches our target partition (case-insensitive)
	return strings.EqualFold(partition, targetPartition)
}

// belongsToPartitionHeuristic is a fallback heuristic approach
func belongsToPartitionHeuristic(accountURL *url.URL, targetPartition string) bool {
	if accountURL == nil {
		return false
	}

	// Heuristic: Check if the URL path contains elements that match the partition (case-insensitive)
	pathElements := strings.Split(strings.Trim(accountURL.Path, "/"), "/")
	for _, element := range pathElements {
		if strings.EqualFold(element, targetPartition) {
			return true
		}
	}

	// Additional heuristic: Check hostname/authority for partition matching (case-insensitive)
	if strings.Contains(strings.ToLower(accountURL.Authority), strings.ToLower(targetPartition)) {
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

// reportRoutingPartitions reports the partitions available in the routing table
func reportRoutingPartitions(extractState *ExtractState) {
	fmt.Println("\nRouting Table Analysis:")
	
	// Check if router is available
	if extractState.Router == nil {
		fmt.Println("  Router: Not initialized")
		return
	}
	
	// Try to cast router to routing.Router
	router, ok := extractState.Router.(routing.Router)
	if !ok {
		fmt.Printf("  Router: Available but not routing.Router type (actual type: %T)\n", extractState.Router)
		return
	}
	
	fmt.Println("  Router: Successfully initialized")
	
	// Test routing with known partition names from network config
	if extractState.NetworkConfig != nil && extractState.NetworkConfig.Globals.Network.Partitions != nil {
		fmt.Printf("  Network Config Partitions: %d\n", len(extractState.NetworkConfig.Globals.Network.Partitions))
		for i, partition := range extractState.NetworkConfig.Globals.Network.Partitions {
			fmt.Printf("    %d: %s (Type: %s)\n", i+1, partition.ID, partition.Type)
		}
	}
	
	// Test routing with sample account URLs to discover partition mappings
	fmt.Println("  Testing routing with sample URLs:")
	testAccountRouting(router)
	
	// Test routing overrides if available
	testRoutingOverrides(extractState)
	
	fmt.Println()
}

// testAccountRouting tests routing with various account URL patterns
func testAccountRouting(router routing.Router) {
	// Test common account URL patterns
	testURLs := []string{
		"acc://dn",
		"acc://directory",
		"acc://system",
		"acc://system/ledger",
		"acc://bvn-cyclops",
		"acc://test.acme",
		"acc://alice.acme",
		"acc://bob.acme",
		"acc://charlie.acme",
		"acc://example.acme",
	}
	
	partitionCounts := make(map[string]int)
	
	for _, urlStr := range testURLs {
		accountURL, err := url.Parse(urlStr)
		if err != nil {
			fmt.Printf("    %s: Parse error: %v\n", urlStr, err)
			continue
		}
		
		partition, err := router.RouteAccount(accountURL)
		if err != nil {
			fmt.Printf("    %s: Route error: %v\n", urlStr, err)
			continue
		}
		
		fmt.Printf("    %s -> %s\n", urlStr, partition)
		partitionCounts[partition]++
	}
	
	// Summary of discovered partitions
	if len(partitionCounts) > 0 {
		fmt.Println("  Discovered partitions from routing:")
		for partition, count := range partitionCounts {
			fmt.Printf("    %s: %d test URLs routed here\n", partition, count)
		}
	}
}

// testRoutingOverrides tests if there are any routing overrides configured
func testRoutingOverrides(extractState *ExtractState) {
	// This would require access to the routing table's override map
	// For now, we'll just report if we have network config with routing info
	if extractState.NetworkConfig != nil {
		fmt.Println("  Routing overrides: Checking network configuration...")
		// Could examine network config for any routing-specific settings
		fmt.Println("  Routing overrides: Not directly accessible from current interface")
	}
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
