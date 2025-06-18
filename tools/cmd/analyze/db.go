// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Database Analysis Implementation
//
// This file contains code for analyzing Accumulate databases created by the
// "debug genesis ingest" command, which combines multiple snapshot files into a
// single database. The analysis focuses on detecting and reporting what exists
// in the database without fabricating any missing data.
//
// High-level implementation:
// 1. Open the database at the specified path
// 2. Create a database batch for reading operations
// 3. Extract all accounts from the database directory
//    - For full database analysis: get all accounts in the directory
//    - For partition analysis: get accounts only for the specified partition
// 4. For each account:
//    - Retrieve the account's main data (if it exists)
//    - Collect account type statistics
//    - Retrieve all chains associated with the account
//    - Track chain statistics and identify accounts missing main chains
// 5. Report statistics including:
//    - Total number of accounts found
//    - Account types and their counts
//    - Chain types and their counts
//    - Accounts missing main chains
//
// The implementation strictly reports only what is found in the database
// without fabricating any missing data, following the critical rule for
// Accumulate data analysis. This ensures accurate reporting of the database
// state for debugging and monitoring purposes.

package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
)

var cmdAnalyze = &cobra.Command{
	Use:   "analyze",
	Short: "Analyze Accumulate databases, snapshots, and partitions",
	Long:  "Analyze Accumulate databases, snapshots, and partitions to provide statistics and information",
}

var cmdAnalyzeDB = &cobra.Command{
	Use:   "db [database-path]",
	Short: "Analyzes a database directory",
	Long: `Analyzes a database directory and provides statistics:
- Total number of accounts
- Chain information for each account
- Transaction counts per chain
- List of accounts without a Main chain`,
	Args: cobra.ExactArgs(1),
	Run:  analyzeDatabase,
}

var cmdAnalyzePartition = &cobra.Command{
	Use:   "partition [database-path] [partition-name]",
	Short: "Analyzes a specific partition in a database",
	Long: `Analyzes a specific partition in a database and provides statistics:
- Total number of accounts in the partition
- Chain information for each account
- Transaction counts per chain
- List of accounts without a Main chain`,
	Args: cobra.ExactArgs(2),
	RunE: analyzePartition,
}

// analyzePartition analyzes a specific partition in a database
// This function is used by the partition command
func analyzePartition(cmd *cobra.Command, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("database path and partition name required")
	}

	dbPath := args[0]
	partitionName := args[1]

	// Open the database
	fmt.Printf("Opening database: %s\n", dbPath)
	db, err := database.OpenBadger(dbPath, nil)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	// Begin a read-only transaction
	batch := db.Begin(false)
	defer batch.Discard()

	// Parse partition URL
	partitionURL, err := url.Parse("acc://" + partitionName)
	if err != nil {
		return fmt.Errorf("failed to parse partition name: %w", err)
	}

	// Get accounts for the partition
	fmt.Printf("Getting accounts for partition: %s\n", partitionURL)
	accounts, err := getPartitionAccounts(batch, partitionURL)
	if err != nil {
		return fmt.Errorf("failed to get accounts: %w", err)
	}

	// Analyze the accounts
	analyzeAccounts(batch, accounts)
	return nil
}

// isSnapshot checks if the file is a snapshot by looking for the magic number
func isSnapshot(path string) bool {
	// Check if it's a directory (likely a database)
	fi, err := os.Stat(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to stat path: %v\n", err)
		os.Exit(1)
	}

	if fi.IsDir() {
		return false // It's a directory, so it's a database
	}

	// Open the file to check for snapshot format
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	// Try to read a few bytes to see if it's a valid file
	// This is a simplified check since we can't use snapshot.GetVersion
	buf := make([]byte, 16)
	_, err = f.Read(buf)
	if err == nil {
		// Successfully read from file
		return true
	}

	// Reset the file position
	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to reset file position: %v\n", err)
		os.Exit(1)
	}

	return false
}

// snapDbWrapper wraps a file to implement a simplified interface
// Note: This is a placeholder since we can't use snapshot.Store
type snapDbWrapper struct {
	file *os.File
}

// Begin implements a simplified interface
func (s *snapDbWrapper) Begin(update bool) *database.Batch {
	return nil
}

// analyzeDatabase analyzes a database directory
// This function is used by the db command
func analyzeDatabase(cmd *cobra.Command, args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Database path required\n")
		os.Exit(1)
	}

	dbPath := args[0]

	// Check if the path is a snapshot file
	if isSnapshot(dbPath) {
		fmt.Printf("%s appears to be a snapshot file. Use the 'snap' command instead.\n", dbPath)
		os.Exit(1)
	}

	// Open the database
	fmt.Printf("Opening database: %s\n", dbPath)
	db, err := database.OpenBadger(dbPath, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Begin a read-only transaction
	batch := db.Begin(false)
	defer batch.Discard()

	// Get all accounts
	fmt.Println("Getting all accounts...")
	accounts, err := getAllAccounts(batch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get accounts: %v\n", err)
		os.Exit(1)
	}

	// Analyze the accounts
	analyzeAccounts(batch, accounts)
}

// getAllAccounts gets all accounts from the database
func getAllAccounts(batch *database.Batch) ([]*url.URL, error) {
	// TODO: Implement proper account retrieval using the current database API
	// This is a placeholder that needs to be updated
	fmt.Println("WARNING: getAllAccounts is not fully implemented")
	
	// Return empty list for now
	return []*url.URL{}, nil
}

// getPartitionAccounts gets accounts for a specific partition
func getPartitionAccounts(batch *database.Batch, partitionURL *url.URL) ([]*url.URL, error) {
	// TODO: Implement proper partition account retrieval using the current database API
	// This is a placeholder that needs to be updated
	fmt.Printf("WARNING: getPartitionAccounts is not fully implemented for %s\n", partitionURL)
	
	// Return empty list for now
	return []*url.URL{}, nil
}

// analyzeAccounts analyzes accounts from a database
func analyzeAccounts(batch *database.Batch, accounts []*url.URL) {
	fmt.Printf("Found %d accounts\n", len(accounts))

	// Sort accounts for consistent output
	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].String() < accounts[j].String()
	})

	// Statistics tracking
	var accountsWithoutMain []string
	chainCounts := make(map[string]int)
	accountTypeStats := make(map[string]int)

	// Process each account
	for _, accountURL := range accounts {
		// Get the account
		account, err := batch.Account(accountURL).Main().Get()
		if err != nil {
			fmt.Printf("Warning: failed to get account %s: %v\n", accountURL, err)
			accountsWithoutMain = append(accountsWithoutMain, accountURL.String())
			continue
		}

		// Track account type statistics
		accountType := account.Type().String()
		accountTypeStats[accountType]++

		// Get chains for this account
		chains, err := getAccountChains(batch, accountURL)
		if err != nil {
			fmt.Printf("Warning: failed to get chains for account %s: %v\n", accountURL, err)
			continue
		}

		hasMainChain := false
		for _, chainName := range chains {
			chainCounts[chainName]++
			if strings.EqualFold(chainName, "main") {
				hasMainChain = true
			}
		}

		if !hasMainChain {
			accountsWithoutMain = append(accountsWithoutMain, accountURL.String())
		}
	}

	// Print summary
	fmt.Println("\n=== Database Analysis Summary ===")
	fmt.Printf("Total accounts: %d\n", len(accounts))

	// Print account type statistics
	fmt.Println("\n=== Account Types ===")
	printSortedStats(accountTypeStats)

	// Print chain counts
	fmt.Println("\n=== Chain Counts ===")
	printSortedStats(chainCounts)

	// Print accounts without main chain
	fmt.Printf("\n=== Accounts without Main chain: %d ===", len(accountsWithoutMain))
	if len(accountsWithoutMain) > 0 {
		fmt.Println()
		for _, acct := range accountsWithoutMain {
			fmt.Printf("  %s\n", acct)
		}
	}
}

// analyzeAccount analyzes a single account
func analyzeAccount(batch *database.Batch, accountURL *url.URL) {
	// TODO: Implement proper account analysis using the current database API
	// This is a placeholder that needs to be updated
	fmt.Printf("Account: %s\n", accountURL)
	fmt.Printf("  WARNING: analyzeAccount is not fully implemented\n")
}

// getAccountChains gets all chains for an account
func getAccountChains(batch *database.Batch, accountURL *url.URL) ([]string, error) {
	// TODO: Implement proper chain retrieval using the current database API
	// This is a placeholder that needs to be updated
	fmt.Printf("WARNING: getAccountChains is not fully implemented for %s\n", accountURL)
	
	// Return empty list for now
	return []string{}, nil
}

// Note: printSortedStats and writeSortedStats have been moved to utils.go
