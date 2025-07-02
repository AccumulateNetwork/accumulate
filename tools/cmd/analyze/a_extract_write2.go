package main

import (
	"crypto/sha256"
	"fmt"
	"io"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// extractMessageHash extracts the hash from a message record key
func extractMessageHash(key *record.Key) ([]byte, error) {
	if key == nil {
		return nil, fmt.Errorf("key is nil")
	}

	// Marshal the key to analyze its structure
	keyBytes, err := key.MarshalBinary()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal key: %w", err)
	}

	// Create a 32-byte hash from the key bytes
	hash := sha256.Sum256(keyBytes)

	// Return the fixed-size 32-byte hash
	return hash[:], nil
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
