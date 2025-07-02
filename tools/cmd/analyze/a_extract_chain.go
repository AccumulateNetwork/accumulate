// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// buildBloomFilterFromChainEntries populates the Bloom filter with chain entries from a specific partition
func buildBloomFilterFromChainEntries(extractState *ExtractState, bloomFilter *Bloom, partitionID string) {
	fmt.Printf("Building Bloom filter for partition %s...\n", partitionID)

	// Get all accounts for this partition
	accountCount := 0
	chainCount := 0
	entryCount := 0

	// First pass: Find all chain records for the partition
	chainRecords := make(map[string]*ChainRecord)
	for _, record := range extractState.Records {
		// Process chain records
		if record.Type == "chain" && record.Chain != nil {
			// Check if the chain belongs to this partition
			belongs := false
			if accountURL, err := url.Parse(record.Chain.AccountURL); err == nil {
				if router, ok := extractState.Router.(routing.Router); ok {
					belongs = belongsToPartition(accountURL, partitionID, router)
				}
			}

			if belongs {
				// Store the chain record for processing
				chainRecords[record.Chain.URL] = record.Chain
			}
		}
	}

	// Second pass: Process all chain records and extract entries
	for _, chainRecord := range chainRecords {
		// Skip empty chains
		if len(chainRecord.Entries) == 0 {
			continue
		}

		// Count this as a valid chain
		chainCount++

		// Extract account URL from chain URL
		parts := strings.Split(chainRecord.URL, "/")
		if len(parts) < 2 {
			continue
		}
		accountURL := strings.Join(parts[:len(parts)-1], "/")

		// Add each entry hash to the Bloom filter
		for _, entry := range chainRecord.Entries {
			if len(entry.Hash) > 0 {
				// Extract the hash from the entry
				entryHash := extractEntryHash(entry.Hash)
				if entryHash != nil {
					bloomFilter.Add(entryHash)
					entryCount++
				}
			}
		}

		// Count unique accounts
		if _, exists := extractState.ChainEntryCache.Get(accountURL); !exists {
			accountCount++
			// Mark this account as processed
			extractState.ChainEntryCache.Set(accountURL, [][]byte{})
		}

		// Progress reporting
		if chainCount%1000 == 0 {
			fmt.Printf("  Processed %d chains, %d entries\n", chainCount, entryCount)
		}
	}

	fmt.Printf("Bloom filter built for partition %s:\n", partitionID)
	fmt.Printf("  Accounts: %d\n", accountCount)
	fmt.Printf("  Chains: %d\n", chainCount)
	fmt.Printf("  Entries: %d\n", entryCount)
}

// getAccountChainURLs returns a list of chain URLs for an account
func getAccountChainURLs(accountURL string) []string {
	// This is a simplified implementation
	// In a real implementation, you would extract chain URLs based on account type
	
	// For now, just return the main chain URL
	return []string{accountURL + "/main"}
}

// extractEntryHash extracts a 32-byte hash from a chain entry
func extractEntryHash(entry []byte) []byte {
	// If the entry is already 32 bytes, use it directly
	if len(entry) == 32 {
		return entry
	}
	
	// Otherwise, hash the entry to get a 32-byte hash
	hash := sha256.Sum256(entry)
	return hash[:]
}

// reportBloomFilterStats prints statistics about the Bloom filter usage
func reportBloomFilterStats(bloomFilter *Bloom) {
	fmt.Printf("\nBloom Filter Statistics:\n")
	fmt.Printf("%s\n", bloomFilter.GetStats())
}
