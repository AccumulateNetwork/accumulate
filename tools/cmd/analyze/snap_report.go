// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package main - Snapshot Reporting Module
//
// This file contains the report structure for collecting information about snapshot file
// records and generating reports. It creates a temporary database on disk to store
// accounts, messages, and transactions for analysis.
//
// The separation of concerns is as follows:
// - scan.go: Handles the raw snapshot reading and data extraction
// - scan_processing.go: Handles the processing and analysis of the extracted data
// - scan_report.go: Handles the reporting and database storage of processed data
//
// Following the critical rule for Accumulate data analysis, this implementation
// strictly reports only what is found in the snapshot without fabricating any
// missing data. This ensures accurate reporting of the snapshot state for
// debugging and monitoring purposes.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SnapshotReport represents a report generated from a snapshot file
type SnapshotReport struct {
	// Database information
	dbPath string
	db     *SnapshotDB
	
	// Statistics
	AccountCount     int
	MessageCount     int
	TransactionCount int
	ChainCount       int
	
	// Record collections
	Accounts     map[string]string        // URL -> Type
	Messages     map[string]int           // Hash -> Count
	Transactions map[string]int           // Hash -> Count
	Chains       map[string][]string      // Account URL -> Chain IDs
	
	// Analysis collections
	AccountTypes map[string]int           // Type -> Count
	ADIs         []string                 // List of ADI URLs
}

// OpenReport creates a new snapshot report with a temporary database
// Closing the returned report will clean up the temporary database
func OpenReport() (*SnapshotReport, error) {
	// Clean up any old temporary databases
	err := cleanupOldTempDatabases()
	if err != nil {
		fmt.Printf("Warning: failed to clean up old temp databases: %v\n", err)
	}
	
	// Open a BlockchainDB database for the report
	db, err := OpenSnapshotDB()
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}
	
	// Create and initialize the report
	report := &SnapshotReport{
		dbPath:       db.dbPath,
		db:           db,
		Accounts:     make(map[string]string),
		Messages:     make(map[string]int),
		Transactions: make(map[string]int),
		Chains:       make(map[string][]string),
		AccountTypes: make(map[string]int),
		ADIs:         make([]string, 0),
	}
	
	fmt.Printf("Created temporary database at %s\n", db.dbPath)
	return report, nil
}

// Close closes the report and cleans up the temporary directory
func (r *SnapshotReport) Close() error {
	// Close the database (which will also clean up the temp directory)
	if r.db != nil {
		err := r.db.Close()
		r.db = nil
		if err != nil {
			return fmt.Errorf("failed to close database: %w", err)
		}
		r.dbPath = ""
	}
	
	return nil
}

// AddAccount adds an account to the report
func (r *SnapshotReport) AddAccount(urlStr, accountType string) error {
	if urlStr == "" {
		return fmt.Errorf("invalid account: empty URL")
	}
	
	// Check if we've already added this account
	if _, exists := r.Accounts[urlStr]; exists {
		// Account already added, nothing to do
		return nil
	}
	
	// Add the account to our maps
	r.Accounts[urlStr] = accountType
	r.AccountCount++
	
	// Track account type
	r.AccountTypes[accountType] = r.AccountTypes[accountType] + 1
	
	// Track ADIs separately
	if accountType == "Identity" || accountType == "identity" {
		r.ADIs = append(r.ADIs, urlStr)
	}
	
	// Store in database
	if r.db != nil {
		err := r.db.AddAccount(urlStr, accountType)
		if err != nil {
			return fmt.Errorf("failed to store account in database: %w", err)
		}
	}
	
	return nil
}

// AddMessage adds a message to the report
func (r *SnapshotReport) AddMessage(hash string) error {
	if hash == "" {
		return fmt.Errorf("invalid message: empty hash")
	}
	
	// Store in memory map or increment count if already exists
	r.Messages[hash]++
	r.MessageCount++
	
	return nil
}

// AddTransaction adds a transaction to the report
func (r *SnapshotReport) AddTransaction(hash string) error {
	if hash == "" {
		return fmt.Errorf("invalid transaction: empty hash")
	}
	
	// Store in memory map or increment count if already exists
	r.Transactions[hash]++
	r.TransactionCount++
	
	return nil
}

// Commit commits any pending changes to the report
func (r *SnapshotReport) Commit() error {
	if r.db != nil {
		// Commit any pending changes
		err := r.db.Commit()
		if err != nil {
			return fmt.Errorf("failed to commit changes: %w", err)
		}
		
		// Compress the database periodically
		r.db.Compress()
	}
	
	return nil
}

// GenerateReport generates a report of the snapshot contents
func (r *SnapshotReport) GenerateReport() string {
	var report strings.Builder
	
	report.WriteString("=== Snapshot Report ===\n\n")
	
	// Summary
	report.WriteString("Summary:\n")
	report.WriteString(fmt.Sprintf("  Accounts: %d\n", r.AccountCount))
	report.WriteString(fmt.Sprintf("  Messages: %d\n", r.MessageCount))
	report.WriteString(fmt.Sprintf("  Transactions: %d\n", r.TransactionCount))
	report.WriteString(fmt.Sprintf("  Chains: %d\n", r.ChainCount))
	report.WriteString(fmt.Sprintf("  ADIs: %d\n", len(r.ADIs)))
	report.WriteString("\n")
	
	// ADI List (sorted alphabetically)
	report.WriteString("ADIs Found:\n")
	sort.Strings(r.ADIs)
	maxADIs := 20 // Limit to 20 ADIs in the report
	displayCount := len(r.ADIs)
	if displayCount > maxADIs {
		displayCount = maxADIs
	}
	
	for i := 0; i < displayCount; i++ {
		report.WriteString(fmt.Sprintf("  - %s\n", r.ADIs[i]))
	}
	
	if len(r.ADIs) > maxADIs {
		report.WriteString(fmt.Sprintf("  ... and %d more ADIs\n", len(r.ADIs) - maxADIs))
	}
	report.WriteString("\n")
	
	// Account types (using the pre-calculated map)
	report.WriteString("Account Types:\n")
	
	// Sort account types by count (descending)
	typeNames := make([]string, 0, len(r.AccountTypes))
	for typeName := range r.AccountTypes {
		typeNames = append(typeNames, typeName)
	}
	
	sort.Slice(typeNames, func(i, j int) bool {
		return r.AccountTypes[typeNames[i]] > r.AccountTypes[typeNames[j]]
	})
	
	for _, accountType := range typeNames {
		count := r.AccountTypes[accountType]
		report.WriteString(fmt.Sprintf("  %s: %d\n", accountType, count))
	}
	report.WriteString("\n")
	
	// Top accounts by type
	report.WriteString("Top Accounts by Type:\n")
	accountsByType := make(map[string][]string)
	for url, accountType := range r.Accounts {
		accountsByType[accountType] = append(accountsByType[accountType], url)
	}
	
	// Show up to 5 examples of each account type, prioritizing types with more accounts
	for _, accountType := range typeNames {
		urls := accountsByType[accountType]
		report.WriteString(fmt.Sprintf("  %s (%d total):\n", accountType, len(urls)))
		
		// Limit to 5 examples
		maxExamples := 5
		if len(urls) < maxExamples {
			maxExamples = len(urls)
		}
		
		for i := 0; i < maxExamples; i++ {
			report.WriteString(fmt.Sprintf("    - %s\n", urls[i]))
		}
		
		if len(urls) > maxExamples {
			report.WriteString(fmt.Sprintf("    ... and %d more\n", len(urls) - maxExamples))
		}
	}
	report.WriteString("\n")
	
	// Chain information
	report.WriteString("Chain Information:\n")
	report.WriteString(fmt.Sprintf("  Total chains found: %d\n", r.ChainCount))
	
	// Find accounts with the most chains
	type accountChainCount struct {
		url   string
		count int
	}
	
	accountChains := make([]accountChainCount, 0, len(r.Chains))
	for url, chains := range r.Chains {
		accountChains = append(accountChains, accountChainCount{url, len(chains)})
	}
	
	// Sort by chain count (descending)
	sort.Slice(accountChains, func(i, j int) bool {
		return accountChains[i].count > accountChains[j].count
	})
	
	// Show top 10 accounts by chain count
	maxAccounts := 10
	if len(accountChains) < maxAccounts {
		maxAccounts = len(accountChains)
	}
	
	if maxAccounts > 0 {
		report.WriteString("\n  Top accounts by chain count:\n")
		for i := 0; i < maxAccounts; i++ {
			ac := accountChains[i]
			report.WriteString(fmt.Sprintf("    - %s: %d chains\n", ac.url, ac.count))
		}
	}
	report.WriteString("\n")
	
	return report.String()
}

// cleanupOldTempDatabases removes any temporary databases that might have been
// left behind by previous failed runs
func cleanupOldTempDatabases() error {
	tempDir := os.TempDir()
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return fmt.Errorf("failed to read temp directory: %w", err)
	}
	
	prefix := "acc-snapshot-report-"
	threshold := time.Now().Add(-24 * time.Hour) // Remove temp dirs older than 24 hours
	
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		
		// Get file info to check modification time
		info, err := entry.Info()
		if err != nil {
			fmt.Printf("Warning: failed to get info for %s: %v\n", entry.Name(), err)
			continue
		}
		
		// Check if the directory is old enough to be removed
		if info.ModTime().Before(threshold) {
			path := filepath.Join(tempDir, entry.Name())
			err := os.RemoveAll(path)
			if err != nil {
				fmt.Printf("Warning: failed to remove old temp directory %s: %v\n", path, err)
			} else {
				fmt.Printf("Cleaned up old temp directory: %s\n", path)
			}
		}
	}
	
	return nil
}

// AddChain adds a chain to an account in the report
func (r *SnapshotReport) AddChain(accountUrl, chainID string) error {
	if accountUrl == "" {
		return fmt.Errorf("invalid account: empty URL")
	}
	
	if chainID == "" {
		return fmt.Errorf("invalid chain: empty ID")
	}
	
	// Check if we've already added this chain
	chains, exists := r.Chains[accountUrl]
	if !exists {
		chains = make([]string, 0)
	}
	
	// Check if this chain is already in the list
	for _, existingChain := range chains {
		if existingChain == chainID {
			// Chain already added, nothing to do
			return nil
		}
	}
	
	// Add the chain to the list
	r.Chains[accountUrl] = append(chains, chainID)
	r.ChainCount++
	
	// Store in database
	if r.db != nil {
		err := r.db.AddChain(accountUrl, chainID)
		if err != nil {
			return fmt.Errorf("failed to store chain in database: %w", err)
		}
	}
	
	return nil
}
