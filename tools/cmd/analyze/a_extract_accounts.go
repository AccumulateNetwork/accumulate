// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

// DNAccountStats holds statistics for DN partition accounts
type DNAccountStats struct {
	TotalDNAccounts    int                    // Total accounts in DN partition
	AccountsByADI      map[string]int         // Count of accounts per ADI
	ChainsByType       map[string]int         // Count of chains by type
	ChainsByADI        map[string]int         // Count of chains per ADI
	TotalEntriesHeader int64                  // Total entries from chain headers
	TotalEntriesFound  int64                  // Total entries found by traversing chains
	ChainTypesByADI    map[string]map[string]int // Chain types per ADI
}

// NewDNAccountStats creates a new DNAccountStats instance
func NewDNAccountStats() *DNAccountStats {
	return &DNAccountStats{
		AccountsByADI:   make(map[string]int),
		ChainsByType:    make(map[string]int),
		ChainsByADI:     make(map[string]int),
		ChainTypesByADI: make(map[string]map[string]int),
	}
}

// ProcessDNAccounts analyzes accounts and their subchains for DN partition processing
func ProcessDNAccounts(extractState *ExtractState) error {
	if extractState.Router == nil {
		return fmt.Errorf("router not initialized")
	}

	fmt.Printf("Processing DN partition accounts...\n")

	// Initialize DN stats
	dnStats := NewDNAccountStats()

	// Process each account record
	for i, account := range extractState.Accounts {
		// Parse account URL
		accountURL, err := url.Parse(account.URL)
		if err != nil {
			fmt.Printf("Warning: failed to parse account URL %s: %v\n", account.URL, err)
			continue
		}

		// Check if account routes to DN partition
		// Note: This assumes the router has a RouteAccount method
		// In practice, you'd need to cast the router to the appropriate type
		isDNAccount, err := isAccountInDNPartition(extractState.Router, accountURL)
		if err != nil {
			fmt.Printf("Warning: failed to route account %s: %v\n", account.URL, err)
			continue
		}

		if isDNAccount {
			dnStats.TotalDNAccounts++

			// Extract ADI from account URL
			adi := extractADI(account.URL)
			if adi != "" {
				dnStats.AccountsByADI[adi]++

				// Initialize chain type map for this ADI if not exists
				if dnStats.ChainTypesByADI[adi] == nil {
					dnStats.ChainTypesByADI[adi] = make(map[string]int)
				}
			}

			// Process subchains for this account
			err = processAccountSubchains(extractState, account, adi, dnStats)
			if err != nil {
				fmt.Printf("Warning: failed to process subchains for account %s: %v\n", account.URL, err)
			}
		}

		// Progress reporting
		if (i+1)%1000 == 0 {
			fmt.Printf("Processed %d accounts, found %d DN accounts\n", i+1, dnStats.TotalDNAccounts)
		}
	}

	// Store DN stats in extract state (add to Report or create new field)
	extractState.Report.DNStats = dnStats

	fmt.Printf("DN partition processing complete:\n")
	fmt.Printf("  Total DN accounts: %d\n", dnStats.TotalDNAccounts)
	fmt.Printf("  Unique ADIs: %d\n", len(dnStats.AccountsByADI))
	fmt.Printf("  Total chains: %d\n", getTotalChains(dnStats.ChainsByType))
	fmt.Printf("  Total entries (header): %d\n", dnStats.TotalEntriesHeader)
	fmt.Printf("  Total entries (found): %d\n", dnStats.TotalEntriesFound)

	return nil
}

// isAccountInDNPartition checks if an account routes to the DN partition
func isAccountInDNPartition(router interface{}, accountURL *url.URL) (bool, error) {
	// This is a placeholder implementation
	// In practice, you'd need to cast the router to the appropriate type
	// and call its RouteAccount method
	
	// For now, use simple heuristics based on account URL patterns
	// that typically route to DN partition
	urlStr := accountURL.String()
	
	dnPatterns := []string{
		"acc://dn",
		"acc://directory",
		"acc://operators",
		"acc://network",
		"acc://routing",
		"acc://globals",
		"acc://oracle",
		"acc://ledger",
		"acc://system",
		"acc://protocol",
	}
	
	for _, pattern := range dnPatterns {
		if strings.HasPrefix(urlStr, pattern) {
			return true, nil
		}
	}
	
	// TODO: Replace with actual router.RouteAccount() call
	// Example:
	// partition, err := router.RouteAccount(accountURL)
	// if err != nil {
	//     return false, err
	// }
	// return partition == "Directory", nil
	
	return false, nil
}

// extractADI extracts the ADI (Accumulate Digital Identifier) from an account URL
func extractADI(accountURL string) string {
	// Parse URL to extract ADI
	// For acc://example.acme/path, the ADI would be "example.acme"
	
	// Remove protocol prefix
	if strings.HasPrefix(accountURL, "acc://") {
		accountURL = accountURL[6:]
	}
	
	// Split by '/' and take the first part
	parts := strings.Split(accountURL, "/")
	if len(parts) > 0 {
		return parts[0]
	}
	
	return ""
}

// processAccountSubchains processes subchains for a given account
func processAccountSubchains(extractState *ExtractState, account AccountRecord, adi string, dnStats *DNAccountStats) error {
	// Find all chains that belong to this account (same ADI)
	accountADI := extractADI(account.URL)
	if accountADI == "" {
		return nil
	}

	// Look through all account records to find subchains
	// In practice, you might have a separate chains collection or
	// need to parse the account data to find its chains
	
	// For now, we'll simulate finding subchains by looking for related accounts
	// This is a simplified approach - in reality you'd parse the account data
	// to find its actual chain references
	
	chainCount := 0
	for _, otherAccount := range extractState.Accounts {
		otherADI := extractADI(otherAccount.URL)
		
		// Check if this is a subchain of our account (same ADI, different path)
		if otherADI == accountADI && otherAccount.URL != account.URL {
			// This looks like a subchain
			chainType := determineDNChainType(otherAccount.URL)
			
			// Update statistics
			dnStats.ChainsByType[chainType]++
			dnStats.ChainsByADI[adi]++
			dnStats.ChainTypesByADI[adi][chainType]++
			
			// Count entries (simplified - in practice you'd parse the chain data)
			headerEntries, foundEntries := countChainEntries(otherAccount)
			dnStats.TotalEntriesHeader += headerEntries
			dnStats.TotalEntriesFound += foundEntries
			
			chainCount++
		}
	}
	
	return nil
}

// determineDNChainType determines the type of chain based on the URL for DN processing
func determineDNChainType(chainURL string) string {
	// Simple heuristics to determine chain type
	// In practice, you'd parse the actual chain data
	
	if strings.Contains(chainURL, "/main") {
		return "MainChain"
	} else if strings.Contains(chainURL, "/anchor") {
		return "AnchorChain"
	} else if strings.Contains(chainURL, "/scratch") {
		return "ScratchChain"
	} else if strings.Contains(chainURL, "/data") {
		return "DataChain"
	} else if strings.Contains(chainURL, "/index") {
		return "IndexChain"
	}
	
	return "UnknownChain"
}

// countChainEntries counts entries in a chain from header and by traversing
func countChainEntries(chainAccount AccountRecord) (headerEntries int64, foundEntries int64) {
	// This is a placeholder implementation
	// In practice, you'd parse the chain data to:
	// 1. Extract header information about entry count
	// 2. Traverse the chain to count actual entries found
	
	// For now, return simulated counts
	// TODO: Implement actual chain parsing and traversal
	headerEntries = 10  // Placeholder - would come from chain header
	foundEntries = 8    // Placeholder - would come from actual traversal
	
	return headerEntries, foundEntries
}

// getTotalChains calculates total chains from chain type counts
func getTotalChains(chainsByType map[string]int) int {
	total := 0
	for _, count := range chainsByType {
		total += count
	}
	return total
}

// PrintDNStats prints detailed DN partition statistics
func (stats *DNAccountStats) PrintDNStats() {
	fmt.Printf("\nDN Partition Statistics:\n")
	fmt.Printf("  Total DN Accounts: %d\n", stats.TotalDNAccounts)
	fmt.Printf("  Unique ADIs: %d\n", len(stats.AccountsByADI))
	
	fmt.Printf("\n  Accounts by ADI:\n")
	for adi, count := range stats.AccountsByADI {
		fmt.Printf("    %s: %d accounts\n", adi, count)
	}
	
	fmt.Printf("\n  Chains by Type:\n")
	for chainType, count := range stats.ChainsByType {
		fmt.Printf("    %s: %d chains\n", chainType, count)
	}
	
	fmt.Printf("\n  Chains by ADI:\n")
	for adi, count := range stats.ChainsByADI {
		fmt.Printf("    %s: %d chains\n", adi, count)
	}
	
	fmt.Printf("\n  Chain Types by ADI:\n")
	for adi, chainTypes := range stats.ChainTypesByADI {
		fmt.Printf("    %s:\n", adi)
		for chainType, count := range chainTypes {
			fmt.Printf("      %s: %d\n", chainType, count)
		}
	}
	
	fmt.Printf("\n  Entry Counts:\n")
	fmt.Printf("    Total entries (from headers): %d\n", stats.TotalEntriesHeader)
	fmt.Printf("    Total entries (found): %d\n", stats.TotalEntriesFound)
	
	if stats.TotalEntriesHeader > 0 {
		completeness := float64(stats.TotalEntriesFound) / float64(stats.TotalEntriesHeader) * 100
		fmt.Printf("    Completeness: %.2f%%\n", completeness)
	}
}
