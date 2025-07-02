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
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	snapshotpkg "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/encoding"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// PartitionAccountStats holds statistics for account processing in a specific partition
type PartitionAccountStats struct {
	PartitionID    string
	PartitionType  string
	TotalAccounts  int64
	TotalChains    int64
	AccountsByType map[string]int64 // e.g., "LiteTokenAccount", "ADI", etc.
	ChainsByType   map[string]int64 // e.g., "MainChain", "AnchorChain", etc.
	ProcessingTime int64            // milliseconds
}

// NewPartitionAccountStats creates a new PartitionAccountStats instance
func NewPartitionAccountStats(partitionID, partitionType string) *PartitionAccountStats {
	return &PartitionAccountStats{
		PartitionID:    partitionID,
		PartitionType:  partitionType,
		AccountsByType: make(map[string]int64),
		ChainsByType:   make(map[string]int64),
	}
}

// UnmarshalRecord unmarshals a binary record into a value implementing encoding.BinaryValue
// This is a lightweight version of snapshot.readValue that works directly with binary data
func UnmarshalRecord(data []byte, v encoding.BinaryValue) error {
	if len(data) == 0 {
		return fmt.Errorf("empty record data")
	}

	err := v.UnmarshalBinary(data)
	if err != nil {
		return errors.EncodingError.WithFormat("unmarshal: %w", err)
	}
	return nil
}

// extractAccountURLFromRecordValue extracts account URL from binary record value
// using the UnmarshalRecord function to unmarshal the value as a protocol.Account
func extractAccountURLFromRecordValue(valueBytes []byte) (*url.URL, error) {
	if len(valueBytes) == 0 {
		return nil, fmt.Errorf("empty record value")
	}

	// Create a new account instance
	var account protocol.Account
	
	// Use protocol.UnmarshalAccountFrom which handles the account type detection
	account, err := protocol.UnmarshalAccountFrom(io.NewSectionReader(bytes.NewReader(valueBytes), 0, int64(len(valueBytes))))
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal account: %v", err)
	}
	if account == nil {
		return nil, fmt.Errorf("account is nil after unmarshaling")
	}

	// Extract the URL from the account
	accountURL := account.GetUrl()
	if accountURL == nil {
		return nil, fmt.Errorf("account URL is nil")
	}

	return accountURL, nil
}

// ProcessPartitionAccounts processes accounts for a specific partition using in-memory records
// This function uses the pre-loaded records from ExtractState instead of reading from the snapshot file
// It uses UnmarshalRecord to unmarshal account data from binary record values
func ProcessPartitionAccounts(extractState *ExtractState, partitionID string) (*PartitionAccountStats, error) {
	fmt.Printf("Processing accounts for partition: %s\n", partitionID)

	// Find the partition info
	var targetPartition *PartitionInfo
	for i, partition := range extractState.Partitions {
		if partition.ID == partitionID {
			targetPartition = &extractState.Partitions[i]
			break
		}
	}

	if targetPartition == nil {
		return nil, fmt.Errorf("partition %s not found", partitionID)
	}

	// Initialize statistics
	stats := NewPartitionAccountStats(partitionID, targetPartition.Type)

	// Track unique accounts to avoid double-counting (since accounts are split into multiple records)
	uniqueAccounts := make(map[string]bool)

	fmt.Printf("Processing %d in-memory records for accounts...\n", len(extractState.Records))

	// Process records from in-memory collection
	recordCount := 0
	accountRecordCount := 0

	// Iterate through all records in memory
	for _, record := range extractState.Records {
		recordCount++

		// Skip non-account records based on record type
		if record.Type != "account" {
			continue
		}

		// Debug: Check first few records
		if recordCount <= 5 {
			fmt.Printf("  Record %d key: %q\n", recordCount, string(record.Key))
		}

		// Try to extract account URL from record value using UnmarshalRecord
		var accountURL *url.URL
		var err error

		// If URL is already extracted during loading, use it
		if record.URL != "" {
			accountURL, err = url.Parse(record.URL)
			if err != nil {
				// If parsing fails, try to extract from value
				accountURL, err = extractAccountURLFromRecordValue(record.Value)
				if err != nil {
					continue // Skip records that can't be parsed as accounts
				}
			}
		} else {
			// Extract from value if URL wasn't pre-extracted
			accountURL, err = extractAccountURLFromRecordValue(record.Value)
			if err != nil {
				continue // Skip records that can't be parsed as accounts
			}
		}

		accountRecordCount++

		// Check if this account belongs to our target partition
		belongs, err := accountBelongsToPartition(extractState, accountURL, partitionID)
		if err != nil {
			return nil, fmt.Errorf("error checking partition membership for %s: %v", accountURL, err)
		}

		if belongs {
			// Use account URL string as unique key to avoid double-counting
			accountKey := accountURL.String()
			if !uniqueAccounts[accountKey] {
				uniqueAccounts[accountKey] = true

				// Update statistics for this account (only count once per unique account)
				stats.TotalAccounts++
				accountType := determinePartitionAccountType(accountURL)
				stats.AccountsByType[accountType]++

				// Determine chain types for this account
				chainTypes := determineChainTypesFromURL(accountURL)
				for _, chainType := range chainTypes {
					stats.ChainsByType[chainType]++
					stats.TotalChains++
				}
			}
		}

		// Progress reporting
		if recordCount%100000 == 0 {
			fmt.Printf("  Processed %d records, found %d account records\n", recordCount, accountRecordCount)
		}
	}

	fmt.Printf("Partition %s processing complete: %d accounts, %d chains\n",
		partitionID, stats.TotalAccounts, stats.TotalChains)

	return stats, nil
}

// extractAccountURLFromValue extracts account URL from record value
// by unmarshaling the value as a snapshot.Account struct
func extractAccountURLFromValue(record *snapshotpkg.RecordEntry) (*url.URL, error) {
	// Unmarshal the record value as a protocol.Account
	account, err := protocol.UnmarshalAccountFrom(io.NewSectionReader(bytes.NewReader(record.Value), 0, int64(len(record.Value))))
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal account: %v", err)
	}
	if account == nil {
		return nil, fmt.Errorf("account is nil after unmarshaling")
	}

	// Extract the URL from the account
	accountURL := account.GetUrl()
	if accountURL == nil {
		return nil, fmt.Errorf("account URL is nil")
	}

	return accountURL, nil
}

// accountBelongsToPartition checks if an account belongs to the specified partition
func accountBelongsToPartition(extractState *ExtractState, accountURL *url.URL, partitionID string) (bool, error) {
	if extractState.Router == nil {
		return false, fmt.Errorf("router not available")
	}

	// Convert to Accumulate URL type
	accURL, err := url.Parse(accountURL.String())
	if err != nil {
		return false, fmt.Errorf("failed to parse URL: %v", err)
	}

	// Cast router to proper type and use it to determine partition
	router, ok := extractState.Router.(routing.Router)
	if !ok {
		return false, fmt.Errorf("router is not of expected type")
	}

	partition, err := router.RouteAccount(accURL)
	if err != nil {
		return false, fmt.Errorf("failed to route account: %v", err)
	}

	return strings.EqualFold(partition, partitionID), nil
}

// determinePartitionAccountType determines the type of account from URL for partition processing
func determinePartitionAccountType(accountURL *url.URL) string {
	urlStr := accountURL.String()

	// Use URL patterns to determine account type
	if strings.Contains(urlStr, "/ACME") {
		return "LiteTokenAccount"
	} else if strings.Contains(urlStr, "/data") {
		return "DataAccount"
	} else if strings.Contains(urlStr, "/key") {
		return "KeyPage"
	} else if strings.Contains(urlStr, "/book") {
		return "KeyBook"
	} else if !strings.Contains(urlStr, "/") {
		return "ADI"
	}

	// Default fallback
	return "UnknownAccount"
}

// determinePartitionChainType determines the primary chain type for an account (simplified version)
func determinePartitionChainType(accountURL *url.URL) string {
	urlStr := accountURL.String()

	// Check for specific chain type patterns
	if strings.Contains(urlStr, "/anchor") {
		return "AnchorChain"
	} else if strings.Contains(urlStr, "/scratch") {
		return "ScratchChain"
	} else {
		return "MainChain" // Default for most accounts
	}
}

// determineChainTypes determines the types of chains for an account
func determineChainTypes(accountURL *url.URL, record *snapshotpkg.RecordEntry) []string {
	urlStr := accountURL.String()
	chainTypes := []string{}

	// Most accounts have a main chain
	chainTypes = append(chainTypes, "MainChain")

	// Check for other chain types based on URL patterns
	if strings.Contains(urlStr, "/anchor") {
		chainTypes = append(chainTypes, "AnchorChain")
	}
	if strings.Contains(urlStr, "/scratch") {
		chainTypes = append(chainTypes, "ScratchChain")
	}

	return chainTypes
}

// determineChainTypesFromURL determines the types of chains for an account based only on URL
// This version doesn't require the original record, just the account URL
func determineChainTypesFromURL(accountURL *url.URL) []string {
	urlStr := accountURL.String()
	chainTypes := []string{}

	// Most accounts have a main chain
	chainTypes = append(chainTypes, "MainChain")

	// Check for other chain types based on URL patterns
	if strings.Contains(urlStr, "/anchor") {
		chainTypes = append(chainTypes, "AnchorChain")
	}
	if strings.Contains(urlStr, "/scratch") {
		chainTypes = append(chainTypes, "ScratchChain")
	}

	return chainTypes
}

// PrintPartitionStats prints detailed statistics for a partition
func (stats *PartitionAccountStats) PrintPartitionStats() {
	fmt.Printf("\n=== Partition %s (%s) Statistics ===\n", stats.PartitionID, stats.PartitionType)
	fmt.Printf("Total Accounts: %d\n", stats.TotalAccounts)
	fmt.Printf("Total Chains: %d\n", stats.TotalChains)

	if len(stats.AccountsByType) > 0 {
		fmt.Printf("\nAccounts by Type:\n")
		for accountType, count := range stats.AccountsByType {
			fmt.Printf("  %s: %d\n", accountType, count)
		}
	}

	if len(stats.ChainsByType) > 0 {
		fmt.Printf("\nChains by Type:\n")
		for chainType, count := range stats.ChainsByType {
			fmt.Printf("  %s: %d\n", chainType, count)
		}
	}

	if stats.ProcessingTime > 0 {
		fmt.Printf("\nProcessing Time: %d ms\n", stats.ProcessingTime)
	}
}
