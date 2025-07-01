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
	"gitlab.com/accumulatenetwork/accumulate/internal/database/snapshot"
	snapshotpkg "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
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

// ProcessPartitionAccounts processes accounts for a specific partition using streaming approach
// This function uses the pre-opened snapshot file and section information from ExtractState
// For v2 snapshots, accounts are stored as individual records in Records sections, not in dedicated Accounts sections
func ProcessPartitionAccounts(extractState *ExtractState, partitionID string) (*PartitionAccountStats, error) {
	fmt.Printf("Processing accounts for partition: %s\n", partitionID)

	// Ensure snapshot is initialized
	if extractState.SnapshotFileHandle == nil || extractState.SnapshotReader == nil {
		return nil, fmt.Errorf("snapshot not initialized - call InitializeSnapshot() first")
	}

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

	// Use the pre-opened snapshot reader
	reader := extractState.SnapshotReader

	// In v2 snapshots, accounts are stored as records in Records sections
	// We need to process all Records sections and filter for account records
	var recordsSections []int
	for i, section := range reader.Sections {
		if section.Type() != snapshot.SectionTypeAccounts {
			continue
		}
		recordsSections = append(recordsSections, i)
	}

	if len(recordsSections) == 0 {
		return nil, fmt.Errorf("no Records sections found in snapshot")
	}

	fmt.Printf("Found %d Records sections to process for accounts...\n", len(recordsSections))

	// Track unique accounts to avoid double-counting (since accounts are split into multiple records)
	uniqueAccounts := make(map[string]bool)

	// Process each Records section
	for _, sectionIndex := range recordsSections {
		fmt.Printf("Processing Records section %d for account data...\n", sectionIndex)

		// Open record reader for this Records section
		recordReader, err := reader.OpenRecords(sectionIndex)
		if err != nil {
			return nil, fmt.Errorf("failed to open records section %d: %v", sectionIndex, err)
		}

		// Process each record in this Records section
		recordCount := 0
		accountRecordCount := 0
		for {
			record, err := recordReader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, fmt.Errorf("error reading record from section %d: %v", sectionIndex, err)
			}

			recordCount++

			// Debug: Check first few records
			if recordCount <= 5 {
				keyBytes, _ := record.Key.MarshalBinary()
				keyStr := string(keyBytes)
				fmt.Printf("  Record %d key: %q\n", recordCount, keyStr)
			}

			// Try to extract account URL from record value
			accountURL, err := extractAccountURLFromValue(record)
			if err != nil {
				continue // Skip records that can't be parsed as accounts
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
					chainTypes := determineChainTypes(accountURL, record)
					for _, chainType := range chainTypes {
						stats.ChainsByType[chainType]++
						stats.TotalChains++
					}
				}
			}
			
			fmt.Printf("  Section %d: processed %d records, found %d account records\n", 
				sectionIndex, recordCount, accountRecordCount)
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

	return partition == partitionID, nil
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
