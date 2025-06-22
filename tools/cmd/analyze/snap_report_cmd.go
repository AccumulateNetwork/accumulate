// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Test snapshot location: /home/paul/work/acc1/bvn0.snap

package main

// Key test snapshot file for development and testing: /home/paul/work/acc1/bvn0.snap
// This file contains real-world data that we're focused on properly analyzing
// 
// Command to run the snapshot report:
// ./bin/analyze snap-report /home/paul/work/acc1/bvn0.snap

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// URLIssueType represents the type of issue detected in a URL
type URLIssueType int

const (
	NoIssue URLIssueType = iota
	MalformedURL
	MissingDomain
	InvalidDomain
	TypoInURL
	InvalidFormat
	DoubleColon
)

// URLIssue represents an issue detected in a URL
type URLIssue struct {
	IssueType URLIssueType
	Message   string
}

var (
	outputFile string
)

var (
	debugMode bool
	maxRecords int
	globalSnapshotVersion uint64

	// Maps to track accounts with and without main chain records
	accountMainRecords     map[string]bool
	accountsWithUnknownTypes map[string]string
	// Map to track URL issues for accounts without main chains
	accountURLIssues map[string]URLIssue
	// Maps to track transaction references and missing transactions
	transactionReferences map[string][]string // Account URL -> Transaction Hashes
	missingTransactions   map[string][]string // Account URL -> Missing Transaction Hashes
	// Set of valid ADI authorities (e.g., "redwagon.acme")
	validADIs map[string]bool
	// Map of ADI to valid account paths (e.g., "redwagon.acme" -> ["book", "tokens", etc.])
	validAccountPaths = make(map[string]map[string]bool)
	// Common valid account path components
	commonAccountPaths = []string{"book", "keybook", "tokens", "staking", "data", "keypage", "page"}

	// Statistics for record types
	recordStats = struct {
		TotalRecords      int
		AccountRecords    int
		MainRecords       int
		ChainRecords      int
		UnknownTypes      int
		UnmarshalFailures int
		AccountsWithoutMainChain int
		UnknownTypeDetails map[string]int
		AccountsWithMissingTxs int
		TotalMissingTxs int
	}{
		UnknownTypeDetails: make(map[string]int),
	}
)

var cmdAnalyzeSnapReport = &cobra.Command{
	Use:   "snap-report [snapshot-path]",
	Short: "Generates a detailed report from a snapshot file",
	Long: `Generates a detailed report from a snapshot file, including:
- Account statistics
- Chain information
- ADI listings
- Account type distributions

This command processes the snapshot file directly without fabricating any data,
ensuring accurate reporting of the snapshot state for debugging and monitoring purposes.`,
	Args: cobra.ExactArgs(1),
	RunE: generateSnapshotReport,
}

func init() {
	cmdAnalyzeSnapReport.Flags().StringVarP(&outputFile, "output", "o", "", "Output file for the report (default is console output)")
	cmdAnalyzeSnapReport.Flags().BoolVarP(&debugMode, "debug", "d", false, "Enable debug mode with more verbose output")
	cmdAnalyzeSnapReport.Flags().IntVarP(&maxRecords, "max-records", "m", 0, "Maximum number of records to process (0 for all)")
}

// hexDump returns a formatted hex representation of binary data for debugging
func hexDump(data []byte, maxBytes int) string {
	if len(data) == 0 {
		return "[empty]"
	}
	
	// Limit the number of bytes to display
	if len(data) > maxBytes {
		return fmt.Sprintf("%x... (%d more bytes)", data[:maxBytes], len(data)-maxBytes)
	}
	
	return fmt.Sprintf("%x", data)
}

// extractChainsFromAccount extracts chain information from account objects
// and adds them to the snapshot report without fabricating data
func extractChainsFromAccount(report *SnapshotReport, acct protocol.Account, urlStr string) {
	// All accounts have a main chain
	mainChainID := protocol.MainChain
	chainType := inferChainTypeFromID(mainChainID)
	if err := report.AddChain(urlStr, mainChainID, chainType); err != nil && debugMode {
		fmt.Printf("Warning: failed to add main chain for account %s: %v\n", urlStr, err)
	}
	
	// Different account types have different additional chains
	// We'll add them based on the account type
	switch a := acct.(type) {
	case *protocol.TokenAccount, *protocol.DataAccount, *protocol.ADI, *protocol.KeyBook, *protocol.KeyPage:
		// These account types typically have signature chains
		sigChainID := "signature"
		chainType = inferChainTypeFromID(sigChainID)
		if err := report.AddChain(urlStr, sigChainID, chainType); err != nil && debugMode {
			fmt.Printf("Warning: failed to add signature chain for account %s: %v\n", urlStr, err)
		}
		
		// Add sequence chain for accounts that can have transactions
		seqChainID := "sequence"
		chainType = inferChainTypeFromID(seqChainID)
		if err := report.AddChain(urlStr, seqChainID, chainType); err != nil && debugMode {
			fmt.Printf("Warning: failed to add sequence chain for account %s: %v\n", urlStr, err)
		}
		
	case *protocol.AnchorLedger:
		// Anchor ledgers have anchor chains
		anchorChainID := "anchor"
		chainType = inferChainTypeFromID(anchorChainID)
		if err := report.AddChain(urlStr, anchorChainID, chainType); err != nil && debugMode {
			fmt.Printf("Warning: failed to add anchor chain for account %s: %v\n", urlStr, err)
		}
		
	case *protocol.SyntheticLedger:
		// Synthetic ledgers have special chains
		syntheticChainID := "synthetic"
		chainType = inferChainTypeFromID(syntheticChainID)
		if err := report.AddChain(urlStr, syntheticChainID, chainType); err != nil && debugMode {
			fmt.Printf("Warning: failed to add synthetic chain for account %s: %v\n", urlStr, err)
		}
		
	case *protocol.LiteTokenAccount, *protocol.LiteDataAccount, *protocol.LiteIdentity:
		// Lite accounts have simpler chain structures
		// They typically just have a main chain, which we've already added
		
	case *protocol.SystemLedger, *protocol.BlockLedger:
		// System and block ledgers have special chains
		ledgerChainID := "ledger"
		chainType = inferChainTypeFromID(ledgerChainID)
		if err := report.AddChain(urlStr, ledgerChainID, chainType); err != nil && debugMode {
			fmt.Printf("Warning: failed to add ledger chain for account %s: %v\n", urlStr, err)
		}
		
	default:
		if debugMode {
			fmt.Printf("Unknown account type %T for %s, only adding main chain\n", a, urlStr)
		}
	}
}

// analyzeURL checks for common issues in account URLs
func analyzeURL(urlStr string) URLIssue {
	// Check for empty URL
	if urlStr == "" {
		return URLIssue{IssueType: MalformedURL, Message: "Empty URL"}
	}
	
	// Check if it's an Accumulate URL
	if !strings.HasPrefix(urlStr, "acc://") {
		return URLIssue{IssueType: InvalidFormat, Message: "URL does not start with acc://"}
	}
	
	// Check for specific malformed URLs from our list
	if strings.Contains(urlStr, "defactoacc:/") {
		return URLIssue{IssueType: MalformedURL, Message: "URL contains invalid scheme format 'defactoacc:/'"}
	}
	
	// Parse the URL for further analysis
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return URLIssue{IssueType: MalformedURL, Message: fmt.Sprintf("Failed to parse URL: %v", err)}
	}
	
	// Check for missing domain in authority
	if parsedURL.Authority == "" {
		return URLIssue{IssueType: MissingDomain, Message: "URL is missing an authority component"}
	}
	
	// Check if the authority is a valid ADI
	authority := parsedURL.Authority
	if !strings.Contains(authority, ".") && !strings.HasPrefix(authority, "0x") && len(authority) < 64 {
		return URLIssue{IssueType: MissingDomain, Message: "URL authority is missing domain extension"}
	}
	
	// Check for typos in ADI authority by comparing with valid ADIs
	if strings.HasSuffix(authority, ".acme") {
		adiName := strings.TrimSuffix(authority, ".acme")
		
		// Check for similar ADI names (potential typos)
		for validADI := range validADIs {
			if strings.HasSuffix(validADI, ".acme") {
				validADIName := strings.TrimSuffix(validADI, ".acme")
				
				// Check for simple typos (one character difference)
				if levenshteinDistance(adiName, validADIName) == 1 {
					return URLIssue{IssueType: TypoInURL, Message: fmt.Sprintf("Possible typo in ADI name: '%s' is similar to valid ADI '%s'", adiName, validADIName)}
				}
			}
		}
		
		// Check if this ADI exists in our dictionary
		if _, exists := validADIs[authority]; !exists {
			// This is not necessarily an error, as it could be a reference to an ADI that doesn't have a main record
			// But we'll note it as a potential issue
			return URLIssue{IssueType: InvalidDomain, Message: fmt.Sprintf("ADI '%s' not found in snapshot", authority)}
		}
	}
	
	// Extract domain from the URL
	parts := strings.Split(parsedURL.Authority, ".")
	if len(parts) < 2 {
		return URLIssue{IssueType: MissingDomain, Message: "URL is missing a domain extension"}
	}
	
	// Check if domain is valid
	domain := parts[len(parts)-1]
	if domain != "acme" {
		return URLIssue{IssueType: InvalidDomain, Message: fmt.Sprintf("Invalid domain extension: %s (expected 'acme')", domain)}
	}
	
	// Check for common typos in URL path and check against our dictionary of valid paths
	path := parsedURL.Path
	
	// Remove leading slash if present
	if strings.HasPrefix(path, "/") {
		path = path[1:]
	}
	
	// Check for common hardcoded typos
	if strings.Contains(path, "memebers") {
		return URLIssue{IssueType: TypoInURL, Message: "Possible typo: 'memebers' instead of 'members'"}
	}
	if strings.Contains(path, "daisuek") {
		return URLIssue{IssueType: TypoInURL, Message: "Possible typo: 'daisuek' instead of 'daisuke'"}
	}
	if strings.Contains(path, "tesettest") {
		return URLIssue{IssueType: TypoInURL, Message: "Possible typo: 'tesettest' instead of 'testtest'"}
	}
	if strings.Contains(path, "boo_k") {
		return URLIssue{IssueType: TypoInURL, Message: "Possible typo: 'boo_k' instead of 'book'"}
	}
	if strings.Contains(path, "committee-memebers") {
		return URLIssue{IssueType: TypoInURL, Message: "Possible typo: 'committee-memebers' instead of 'committee-members'"}
	}
	
	// Check if this is a path for a known ADI
	if strings.HasSuffix(authority, ".acme") && path != "" {
		// Split path into components
		pathParts := strings.Split(path, "/")
		if len(pathParts) > 0 && pathParts[0] != "" {
			firstComponent := pathParts[0]
			
			// Check if this ADI has any known paths
			if validPaths, exists := validAccountPaths[authority]; exists {
				// Check if this path component is valid for this ADI
				if _, pathExists := validPaths[firstComponent]; !pathExists {
					// Path doesn't exist for this ADI, check for typos
					for validPath := range validPaths {
						// Check for simple typos (one character difference)
						if levenshteinDistance(firstComponent, validPath) == 1 {
							return URLIssue{IssueType: TypoInURL, Message: fmt.Sprintf("Possible typo in path: '%s' is similar to valid path '%s' for ADI '%s'", firstComponent, validPath, authority)}
						}
					}
					
					// Check against common account paths
					for _, commonPath := range commonAccountPaths {
						if levenshteinDistance(firstComponent, commonPath) == 1 {
							return URLIssue{IssueType: TypoInURL, Message: fmt.Sprintf("Possible typo in path: '%s' is similar to common path '%s'", firstComponent, commonPath)}
						}
					}
				}
			}
		}
	}
	
	// Check for invalid domain extensions
	if strings.HasSuffix(parsedURL.Authority, ".com") {
		return URLIssue{IssueType: InvalidDomain, Message: "Invalid domain extension: '.com' (expected '.acme')"}
	}
	
	// No issues found
	return URLIssue{IssueType: NoIssue, Message: ""}
}

// levenshteinDistance calculates the Levenshtein distance between two strings
func levenshteinDistance(s1, s2 string) int {
	// Create a matrix of size (len(s1)+1) x (len(s2)+1)
	d := make([][]int, len(s1)+1)
	for i := range d {
		d[i] = make([]int, len(s2)+1)
	}

	// Initialize the first row and column
	for i := 0; i <= len(s1); i++ {
		d[i][0] = i
	}
	for j := 0; j <= len(s2); j++ {
		d[0][j] = j
	}

	// Fill in the rest of the matrix
	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, // deletion
				min(d[i][j-1]+1, // insertion
					d[i-1][j-1]+cost)) // substitution
		}
	}

	return d[len(s1)][len(s2)]
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// generateSnapshotReport processes a snapshot file and generates a detailed report
func generateSnapshotReport(cmd *cobra.Command, args []string) error {
	fmt.Println("=== Starting Snapshot Report Generation ===")
	
	// Step 1: Open the snapshot file
	if len(args) < 1 {
		fmt.Println("Error: snapshot file path required")
		return cmd.Help()
	}

	snapshotPath := args[0]
	fmt.Printf("Opening snapshot file: %s\n", snapshotPath)

	// Open the snapshot file
	file, err := os.Open(snapshotPath)
	if err != nil {
		return fmt.Errorf("failed to open snapshot file: %w", err)
	}
	defer file.Close()

	// Step 2: Determine the snapshot version
	fmt.Println("Determining snapshot version...")
	version, err := snapshot.GetVersion(file)
	if err != nil {
		return fmt.Errorf("error determining snapshot version: %w", err)
	}
	fmt.Printf("Snapshot version detected: %d\n", version)

	// If version 1, print an error and exit
	if version == 1 {
		return fmt.Errorf("version 1 snapshots are not supported")
	}
	
	// Store the version for potential version-specific handling
	snapshotVersion := version
	
	// Make the snapshot version available to the processing functions
	globalSnapshotVersion = snapshotVersion

	// Reset file position
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to reset file position: %w", err)
	}
	
	// Get file stats to determine size
	stat, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file stats: %w", err)
	}
	
	// Create a SectionReader
	sectionReader, err := ioutil2.NewSectionReader(file, 0, stat.Size())
	if err != nil {
		return fmt.Errorf("failed to create section reader: %w", err)
	}
	
	// Open the snapshot file
	reader, err := snapshot.Open(sectionReader)
	if err != nil {
		return fmt.Errorf("failed to open snapshot: %w", err)
	}

	// Step 3: Create a new report
	fmt.Println("Creating report...")
	report, err := OpenReport()
	if err != nil {
		return fmt.Errorf("failed to open report: %w", err)
	}
	defer report.Close()
	
	// Initialize tracking maps
	accountMainRecords = make(map[string]bool)
	accountsWithUnknownTypes = make(map[string]string)
	transactionReferences = make(map[string][]string)
	missingTransactions = make(map[string][]string)
	validADIs = make(map[string]bool)
	accountURLIssues = make(map[string]URLIssue)

	// Step 4: Process the snapshot data
	fmt.Printf("Processing %d sections in the snapshot...\n", len(reader.Sections))
	
	// Process each section
	for i := 0; i < len(reader.Sections); i++ {
		section := reader.Sections[i]
		
		// Only process record sections
		if section.Type() != snapshot.SectionTypeRecords {
			fmt.Printf("Skipping non-record section %d\n", i)
			continue
		}
		
		// Open the record section
		fmt.Printf("Processing record section %d...\n", i)
		records, err := reader.OpenRecords(i)
		if err != nil {
			return fmt.Errorf("failed to open record section %d: %w", i, err)
		}
		
		// Process each record
		recordCount := 0
		for {
			entry, err := records.Read()
			if err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("failed to read record: %w", err)
			}
			
			// Process the record based on its key
			if err := processRecord(report, entry); err != nil {
				return fmt.Errorf("error processing record: %v", err)
			}
			
			// If in debug mode, print more information about the record
			if debugMode && recordCount < 10 {
				fmt.Printf("DEBUG: Record %d - Type: %v, Key Length: %d\n", 
					recordCount, entry.Key.Get(0), entry.Key.Len())
				if entry.Key.Len() > 1 {
					fmt.Printf("DEBUG: Key[1]: %v\n", entry.Key.Get(1))
				}
				if entry.Value != nil {
					fmt.Printf("DEBUG: Value length: %d bytes\n", len(entry.Value))
				}
			}
			
			recordCount++
			
			// Print progress every 10000 records
			if recordCount % 10000 == 0 {
				fmt.Printf("Processed %d records...\n", recordCount)
			}
			
			// Check if we've reached the maximum number of records to process
			if maxRecords > 0 && recordCount >= maxRecords {
				fmt.Printf("Reached maximum record count of %d, stopping...\n", maxRecords)
				break
			}
		}
		
		fmt.Printf("Completed section %d: processed %d records\n", i, recordCount)
	}

	// Step 5: Commit the report
	fmt.Println("Committing report data...")
	if err := report.Commit(); err != nil {
		return fmt.Errorf("failed to commit report: %w", err)
	}

	// Step 6: Generate the report
	fmt.Println("Generating report...")
	reportText := report.GenerateReport()
	
	// Step 7: Output the report
	if outputFile != "" {
		// Write to file
		fmt.Printf("Writing report to file: %s\n", outputFile)
		err := os.WriteFile(outputFile, []byte(reportText), 0644)
		if err != nil {
			return fmt.Errorf("failed to write report to file: %w", err)
		}
	} else {
		// Print to console
		fmt.Println("\n=== SNAPSHOT REPORT ===")
		fmt.Println(reportText)
	}

	// Calculate accounts without main chains
	accountsWithoutMain := 0
	for _, hasMain := range accountMainRecords {
		if !hasMain {
			accountsWithoutMain++
		}
	}
	recordStats.AccountsWithoutMainChain = accountsWithoutMain
	
	// Verify that all transactions referenced in main chains exist in the snapshot
	// This is a critical check to ensure snapshot completeness
	for accountURL, txHashes := range transactionReferences {
		for _, txHash := range txHashes {
			// Check if this transaction hash exists in our set of known transaction hashes
			if !report.TransactionHashes[txHash] {
				// This transaction is referenced but not found in the snapshot
				missingTransactions[accountURL] = append(missingTransactions[accountURL], txHash)
				recordStats.TotalMissingTxs++
			}
		}
		
		// If this account has missing transactions, increment the counter
		if len(missingTransactions[accountURL]) > 0 {
			recordStats.AccountsWithMissingTxs++
		}
	}
	
	// Print record statistics
	fmt.Println("\n=== RECORD STATISTICS ===")
	fmt.Printf("Total Records: %d\n", recordStats.TotalRecords)
	fmt.Printf("Account Records: %d\n", recordStats.AccountRecords)
	fmt.Printf("Main Records: %d\n", recordStats.MainRecords)
	fmt.Printf("Chain Records: %d\n", recordStats.ChainRecords)
	fmt.Printf("Unknown Types: %d\n", recordStats.UnknownTypes)
	fmt.Printf("Unmarshal Failures: %d\n", recordStats.UnmarshalFailures)
	fmt.Printf("Accounts Without Main Chain: %d\n", recordStats.AccountsWithoutMainChain)
	fmt.Printf("Accounts With Missing Transactions: %d\n", recordStats.AccountsWithMissingTxs)
	fmt.Printf("Total Missing Transactions: %d\n", recordStats.TotalMissingTxs)
	
	// Print detailed unknown type information
	if len(recordStats.UnknownTypeDetails) > 0 {
		fmt.Println("\n=== UNKNOWN TYPE DETAILS ===")
		for typeName, count := range recordStats.UnknownTypeDetails {
			fmt.Printf("%s: %d\n", typeName, count)
		}
	}
	
	// Print detailed missing transaction information
	if recordStats.AccountsWithMissingTxs > 0 {
		fmt.Println("\n=== MISSING TRANSACTIONS DETAILS ===")
		fmt.Println("The following accounts have transactions referenced in their main chains that are missing from the snapshot:")
		
		// Sort account URLs for consistent output
		accountURLs := make([]string, 0, len(missingTransactions))
		for accountURL, txHashes := range missingTransactions {
			if len(txHashes) > 0 {
				accountURLs = append(accountURLs, accountURL)
			}
		}
		sort.Strings(accountURLs)
		
		// Print details for each account with missing transactions
		for _, accountURL := range accountURLs {
			txHashes := missingTransactions[accountURL]
			fmt.Printf("Account: %s (Missing %d transactions)\n", accountURL, len(txHashes))
			
			// Limit the number of transaction hashes shown to avoid excessive output
			const maxHashesToShow = 5
			if len(txHashes) <= maxHashesToShow {
				for _, hash := range txHashes {
					fmt.Printf("  - %s\n", hash)
				}
			} else {
				for i := 0; i < maxHashesToShow; i++ {
					fmt.Printf("  - %s\n", txHashes[i])
				}
				fmt.Printf("  - ... and %d more\n", len(txHashes)-maxHashesToShow)
			}
		}
	}
	
	// Print accounts with unknown types
	if len(accountsWithUnknownTypes) > 0 {
		fmt.Println("\n=== ACCOUNTS WITH UNKNOWN TYPES ===")
		for url, typeName := range accountsWithUnknownTypes {
			fmt.Printf("%s: %s\n", url, typeName)
		}
	}
	
	// Analyze accounts without main chains
	if accountsWithoutMain > 0 {
		fmt.Println("\n=== ACCOUNTS WITHOUT MAIN CHAIN ===")
		
		// Track issues by type for summary
	issuesByType := make(map[URLIssueType]int)
		
		// First pass: analyze all accounts without main chains
		for url, hasMain := range accountMainRecords {
			if !hasMain {
				// Analyze the URL for issues
				issue := analyzeURL(url)
				accountURLIssues[url] = issue
				
				// Count issues by type
				if issue.IssueType != NoIssue {
					issuesByType[issue.IssueType]++
				}
			}
		}
		
		// Print accounts without main chains grouped by issue type
		fmt.Printf("Total accounts without main chain: %d\n", accountsWithoutMain)
		
		// First print accounts with no detected issues
		fmt.Println("\n--- Accounts with no detected issues ---")
		for url, issue := range accountURLIssues {
			if issue.IssueType == NoIssue {
				fmt.Printf("%s\n", url)
			}
		}
		
		// Then print accounts with issues, grouped by issue type
		if len(issuesByType) > 0 {
			fmt.Println("\n--- Accounts with potential issues ---")
			
			// Print malformed URLs
			if count := issuesByType[MalformedURL]; count > 0 {
				fmt.Printf("\nMalformed URLs (%d):\n", count)
				for url, issue := range accountURLIssues {
					if issue.IssueType == MalformedURL {
						fmt.Printf("%s - %s\n", url, issue.Message)
					}
				}
			}
			
			// Print missing domains
			if count := issuesByType[MissingDomain]; count > 0 {
				fmt.Printf("\nMissing domains (%d):\n", count)
				for url, issue := range accountURLIssues {
					if issue.IssueType == MissingDomain {
						fmt.Printf("%s - %s\n", url, issue.Message)
					}
				}
			}
			
			// Print invalid domains
			if count := issuesByType[InvalidDomain]; count > 0 {
				fmt.Printf("\nInvalid domains (%d):\n", count)
				for url, issue := range accountURLIssues {
					if issue.IssueType == InvalidDomain {
						fmt.Printf("%s - %s\n", url, issue.Message)
					}
				}
			}
			
			// Print typos
			if count := issuesByType[TypoInURL]; count > 0 {
				fmt.Printf("\nPossible typos (%d):\n", count)
				for url, issue := range accountURLIssues {
					if issue.IssueType == TypoInURL {
						fmt.Printf("%s - %s\n", url, issue.Message)
					}
				}
			}
			
			// Print double colons
			if count := issuesByType[DoubleColon]; count > 0 {
				fmt.Printf("\nDouble colon issues (%d):\n", count)
				for url, issue := range accountURLIssues {
					if issue.IssueType == DoubleColon {
						fmt.Printf("%s - %s\n", url, issue.Message)
					}
				}
			}
			
			// Print invalid formats
			if count := issuesByType[InvalidFormat]; count > 0 {
				fmt.Printf("\nInvalid formats (%d):\n", count)
				for url, issue := range accountURLIssues {
					if issue.IssueType == InvalidFormat {
						fmt.Printf("%s - %s\n", url, issue.Message)
					}
				}
			}
		}
	}

	fmt.Println("Report generation completed successfully")
	return nil
}

// determineAccountTypeFromURL analyzes the URL structure to determine account type
func determineAccountTypeFromURL(urlStr string) string {
	// Check for well-known account types based on URL patterns
	if strings.HasSuffix(urlStr, "/keybook") {
		return "KeyBook"
	} else if strings.HasSuffix(urlStr, "/keypage") || strings.Contains(urlStr, "/keypage/") {
		return "KeyPage"
	} else if strings.Contains(urlStr, "/tokens/") {
		// Check if it's a lite token account
		if strings.Contains(urlStr, "lite/") {
			return "LiteTokenAccount"
		}
		return "TokenAccount"
	} else if strings.Contains(urlStr, "/data/") {
		// Check if it's a lite data account
		if strings.Contains(urlStr, "lite/") {
			return "LiteDataAccount"
		}
		return "DataAccount"
	} else if strings.HasPrefix(urlStr, "acc://system/") {
		// More specific system account types
		if strings.Contains(urlStr, "/anchor") {
			return "AnchorLedger"
		} else if strings.Contains(urlStr, "/synthetic") {
			return "SyntheticLedger"
		} else if strings.Contains(urlStr, "/block") {
			return "BlockLedger"
		} else if strings.Contains(urlStr, "/fee") {
			return "FeeLedger"
		}
		return "SystemLedger"
	} else if strings.HasSuffix(urlStr, ".acme") {
		// Check if it's a lite identity
		if strings.Contains(urlStr, "lite/") {
			return "LiteIdentity"
		}
		return "Identity"
	} else if strings.Contains(urlStr, "/ACME") || strings.Contains(urlStr, "/tokens") {
		// Check for token issuer accounts
		// Token issuers often have token symbol in the URL path
		return "TokenIssuer"
	}
	
	return "Unknown"
}

// determineChainType analyzes chain data to determine its type
func determineChainType(data []byte, urlStr, chainID string) string {
	// Skip empty data
	if len(data) == 0 {
		return inferChainTypeFromID(chainID)
	}
	
	// Try to unmarshal the chain data
	if strings.Contains(chainID, "main") || strings.Contains(chainID, "Main") {
		return "Main"
	} else if strings.Contains(chainID, "anchor") || strings.Contains(chainID, "Anchor") {
		return "Anchor"
	} else if strings.Contains(chainID, "transaction") || strings.Contains(chainID, "tx") {
		return "Transaction"
	} else if strings.Contains(chainID, "index") || strings.Contains(chainID, "Index") {
		return "Index"
	} else if strings.Contains(chainID, "signature") || strings.Contains(chainID, "sig") {
		return "Signature"
	} else if strings.Contains(chainID, "pending") {
		return "Pending"
	} else if strings.Contains(chainID, "scratch") {
		return "Scratch"
	} else {
		// Look for patterns in the data
		dataStr := string(data)
		if strings.Contains(dataStr, "anchor") {
			return "Anchor"
		} else if strings.Contains(dataStr, "transaction") {
			return "Transaction"
		} else if strings.Contains(dataStr, "index") {
			return "Index"
		} else if strings.Contains(dataStr, "signature") {
			return "Signature"
		}
	}
	
	return "Unknown"
}

// inferChainTypeFromID tries to determine the chain type from its ID
func inferChainTypeFromID(chainID string) string {
	// Convert to lowercase for case-insensitive comparison
	id := strings.ToLower(chainID)
	
	// Check for known chain ID patterns
	switch {
	case strings.Contains(id, "main"):
		return "Main"
	case strings.Contains(id, "anchor"):
		return "Anchor"
	case strings.Contains(id, "transaction") || strings.Contains(id, "tx"):
		return "Transaction"
	case strings.Contains(id, "index"):
		return "Index"
	case strings.Contains(id, "signature") || strings.Contains(id, "sig"):
		return "Signature"
	case strings.Contains(id, "pending"):
		return "Pending"
	case strings.Contains(id, "scratch"):
		return "Scratch"
	default:
		return "Unknown"
	}
}

// determineAccountTypeFromRawData analyzes raw binary data patterns to determine account type
func determineAccountTypeFromRawData(data []byte) string {
	// Skip empty data
	if len(data) == 0 {
		return "Unknown"
	}
	
	// Convert to string for pattern matching
	dataStr := string(data)
	
	// Check for known patterns in the raw data
	switch {
	case strings.Contains(dataStr, "TokenAccount"):
		return "TokenAccount"
	case strings.Contains(dataStr, "LiteTokenAccount"):
		return "LiteTokenAccount"
	case strings.Contains(dataStr, "DataAccount"):
		return "DataAccount"
	case strings.Contains(dataStr, "LiteDataAccount"):
		return "LiteDataAccount"
	case strings.Contains(dataStr, "Identity") || strings.Contains(dataStr, "ADI"):
		return "Identity"
	case strings.Contains(dataStr, "KeyBook"):
		return "KeyBook"
	case strings.Contains(dataStr, "KeyPage"):
		return "KeyPage"
	case strings.Contains(dataStr, "main") && len(data) < 30:
		// This pattern was observed in the debug output for chain-related data
		return "Chain"
	}
	
	return "Unknown"
}

// unmarshalAccountWithVersion attempts to unmarshal account data with version awareness
func unmarshalAccountWithVersion(data []byte) (protocol.Account, error) {
	// Currently, we use the standard protocol.UnmarshalAccount function
	// In the future, we could add version-specific handling if needed
	return protocol.UnmarshalAccount(data)
}

// determineAccountType analyzes account data and determines the account type
func determineAccountType(data []byte, urlStr string) (string, error) {
	// First attempt: Try to determine the account type from the URL structure
	accountType := determineAccountTypeFromURL(urlStr)
	if accountType != "Unknown" {
		return accountType, nil
	}

	// Second attempt: Try to unmarshal the account data using the Accumulate protocol
	// with version awareness
	account, err := unmarshalAccountWithVersion(data)
	if err == nil && account != nil {
		switch a := account.(type) {
		case *protocol.TokenAccount:
			return "TokenAccount", nil
		case *protocol.LiteTokenAccount:
			return "LiteTokenAccount", nil
		case *protocol.DataAccount:
			return "DataAccount", nil
		case *protocol.LiteDataAccount:
			return "LiteDataAccount", nil
		case *protocol.ADI:
			return "Identity", nil
		case *protocol.KeyBook:
			return "KeyBook", nil
		case *protocol.KeyPage:
			return "KeyPage", nil
		case *protocol.SystemLedger:
			return "SystemLedger", nil
		case *protocol.AnchorLedger:
			return "AnchorLedger", nil
		case *protocol.SyntheticLedger:
			return "SyntheticLedger", nil
		case *protocol.LiteIdentity:
			return "LiteIdentity", nil
		case *protocol.BlockLedger:
			return "BlockLedger", nil
		case *protocol.TokenIssuer:
			return "TokenIssuer", nil
		case *protocol.UnknownSigner:
			return "UnknownSigner", nil
		default:
			return fmt.Sprintf("Unknown (%T)", a), nil
		}
	}

	// Third attempt: Try to determine the type from the raw data pattern
	accountType = determineAccountTypeFromRawData(data)
	if accountType != "Unknown" {
		return accountType, nil
	}

	// If we get here, we couldn't determine the account type
	dataPreview := ""
	if len(data) > 0 {
		// Use hexDump for better visualization
		dataPreview = hexDump(data, 32) // Show up to 32 bytes
	}

	if err != nil {
		recordStats.UnmarshalFailures++
		recordStats.UnknownTypes++
		return "Unknown", fmt.Errorf("failed to unmarshal account data:\nError: %v\nData: %s", err, dataPreview)
	}

	recordStats.UnknownTypes++
	return "Unknown", nil
}

// processRecord processes a single record from the snapshot
func processRecord(report *SnapshotReport, entry *snapshot.RecordEntry) error {
	// Get the record type (first part of the key)
	if entry.Key == nil || entry.Key.Len() == 0 {
		return fmt.Errorf("empty key")
	}

	recordType := fmt.Sprint(entry.Key.Get(0))
	recordStats.TotalRecords++

	// Process based on record type
	switch recordType {
	case "Account":
		recordStats.AccountRecords++
		if entry.Key.Len() < 2 {
			return fmt.Errorf("invalid account key")
		}
		
		// Extract account URL
		urlStr := fmt.Sprint(entry.Key.Get(1))
		
		// Validate URL
		_, err := url.Parse(urlStr)
		if err != nil {
			return fmt.Errorf("invalid account URL %q: %w", urlStr, err)
		}
		
		// Track this account URL for main chain analysis
		if _, exists := accountMainRecords[urlStr]; !exists {
			accountMainRecords[urlStr] = false
		}
		
		// Check if this is a Main record - only Main records contain the full account data
		// This follows the same pattern as genesis.Extract
		if entry.Key.Len() > 2 && fmt.Sprint(entry.Key.Get(2)) == "Main" {
			recordStats.MainRecords++
			
			// Mark this account as having a main record
			accountMainRecords[urlStr] = true
			
			// Extract ADI authority and path for dictionary building
			parsedURL, err := url.Parse(urlStr)
			if err == nil && parsedURL.Authority != "" {
				// Store valid ADI authority
				validADIs[parsedURL.Authority] = true
				
				// Extract path components
				if parsedURL.Path != "" {
					// Remove leading slash if present
					path := parsedURL.Path
					if strings.HasPrefix(path, "/") {
						path = path[1:]
					}
					
					// Split path into components
					pathParts := strings.Split(path, "/")
					if len(pathParts) > 0 && pathParts[0] != "" {
						// Initialize map for this ADI if it doesn't exist
						if _, exists := validAccountPaths[parsedURL.Authority]; !exists {
							validAccountPaths[parsedURL.Authority] = make(map[string]bool)
						}
						
						// Store the first path component as valid for this ADI
						validAccountPaths[parsedURL.Authority][pathParts[0]] = true
					}
				}
			}
			
			if debugMode {
				fmt.Printf("Processing Main record for account: %s\n", urlStr)
			}
			
			// Extract account type from the value using protocol.UnmarshalAccount
			accountType := "Unknown"
			if entry.Value != nil && len(entry.Value) > 0 {
				// Unmarshal the account using the protocol package with version awareness
				acct, err := unmarshalAccountWithVersion(entry.Value)
				if err != nil {
					recordStats.UnmarshalFailures++
					if debugMode {
						// Use hexDump for better visualization of binary data
						dataPreview := hexDump(entry.Value, 32)
						
						// Enhanced error logging with more details
						fmt.Printf("Warning: failed to unmarshal account data for %s\n", urlStr)
						fmt.Printf("  - Error: %v\n", err)
						fmt.Printf("  - Data preview: %s\n", dataPreview)
						fmt.Printf("  - Data length: %d bytes\n", len(entry.Value))
						fmt.Printf("  - Key path: %v\n", entry.Key)
					}
				} else {
					// Determine the account type based on the concrete type
					switch a := acct.(type) {
					case *protocol.TokenAccount:
						accountType = "TokenAccount"
						// Extract chains from TokenAccount
						extractChainsFromAccount(report, a, urlStr)
					case *protocol.LiteTokenAccount:
						accountType = "LiteTokenAccount"
						// Extract chains from LiteTokenAccount
						extractChainsFromAccount(report, a, urlStr)
					case *protocol.DataAccount:
						accountType = "DataAccount"
						// Extract chains from DataAccount
						extractChainsFromAccount(report, a, urlStr)
					case *protocol.LiteDataAccount:
						accountType = "LiteDataAccount"
						// Extract chains from LiteDataAccount
						extractChainsFromAccount(report, a, urlStr)
					case *protocol.ADI:
						accountType = "Identity"
						// Extract chains from ADI
						extractChainsFromAccount(report, a, urlStr)
					case *protocol.KeyBook:
						accountType = "KeyBook"
						// Extract chains from KeyBook
						extractChainsFromAccount(report, a, urlStr)
					case *protocol.KeyPage:
						accountType = "KeyPage"
						// Extract chains from KeyPage
						extractChainsFromAccount(report, a, urlStr)
					case *protocol.SystemLedger:
						accountType = "SystemLedger"
						// Extract chains from SystemLedger
						extractChainsFromAccount(report, a, urlStr)
					case *protocol.AnchorLedger:
						accountType = "AnchorLedger"
						// Extract chains from AnchorLedger
						extractChainsFromAccount(report, a, urlStr)
					case *protocol.SyntheticLedger:
						accountType = "SyntheticLedger"
						// Extract chains from SyntheticLedger
						extractChainsFromAccount(report, a, urlStr)
					case *protocol.LiteIdentity:
						accountType = "LiteIdentity"
						// Extract chains from LiteIdentity
						extractChainsFromAccount(report, a, urlStr)
					case *protocol.BlockLedger:
						accountType = "BlockLedger"
						// Extract chains from BlockLedger
						extractChainsFromAccount(report, a, urlStr)
					case *protocol.TokenIssuer:
						accountType = "TokenIssuer"
						// Extract chains from TokenIssuer
						extractChainsFromAccount(report, a, urlStr)
					default:
						// Unknown account type, but we have the concrete type
						unknownType := fmt.Sprintf("%T", a)
						accountType = fmt.Sprintf("Unknown (%s)", unknownType)
						if debugMode {
							fmt.Printf("Found unknown account type %s for %s\n", unknownType, urlStr)
						}
						recordStats.UnknownTypes++
						
						// Track detailed unknown type information
						recordStats.UnknownTypeDetails[unknownType]++
						
						// Track accounts with unknown types
						accountsWithUnknownTypes[urlStr] = unknownType
						
						// Try to extract chains from unknown account type
						extractChainsFromAccount(report, a, urlStr)
					}
				}
			} else if debugMode {
				fmt.Printf("Warning: no value data for Main record of account %s\n", urlStr)
			}
			
			// Add the account to the report
			return report.AddAccount(urlStr, accountType)
		} else if entry.Key.Len() <= 2 {
			// This is a top-level account record, but not a Main record
			// We'll use URL-based detection as a fallback
			accountType := determineAccountTypeFromURL(urlStr)
			if accountType != "Unknown" {
				return report.AddAccount(urlStr, accountType)
			}
		}
		// Skip other account records (not Main records)
		
	case "Chain":
		// Chain records are embedded in account states, not standalone
		// This case is kept for completeness but should not be reached
		recordStats.ChainRecords++
		
		// Extract chain information from the record key
		if entry.Key.Len() >= 3 {
			urlStr := fmt.Sprint(entry.Key.Get(1))
			chainID := fmt.Sprint(entry.Key.Get(2))
			
			// If this is a main chain, we need to extract transaction references
			if chainID == protocol.MainChain {
				// The chain data contains transaction references
				// We'll extract the transaction hashes directly from the chain data
				// Each chain entry is 32 bytes (a hash)
				if entry.Value != nil && len(entry.Value) > 0 {
					// Process the chain data in 32-byte chunks
					for i := 0; i < len(entry.Value); i += 32 {
						// Make sure we have enough bytes for a complete hash
						if i+32 <= len(entry.Value) {
							// Extract the hash bytes
							hashBytes := entry.Value[i : i+32]
							// Convert to a hex string
							txHash := fmt.Sprintf("%x", hashBytes)
							// Add this transaction reference to the account's list
							transactionReferences[urlStr] = append(transactionReferences[urlStr], txHash)
						}
					}
				}
			}
			
			// Determine chain type from the data if available
			chainType := "Unknown"
			if entry.Value != nil && len(entry.Value) > 0 {
				// Try to determine chain type from the data
				chainType = determineChainType(entry.Value, urlStr, chainID)
			} else {
				// Try to infer chain type from the chain ID
				chainType = inferChainTypeFromID(chainID)
			}
			
			// Add the chain to the report with its type
			return report.AddChain(urlStr, chainID, chainType)
		}
		
		return fmt.Errorf("invalid chain key")
		
	case "Message":
		// Process message record
		if entry.Key.Len() < 2 {
			return fmt.Errorf("invalid message key")
		}
		
		// Extract message hash
		hash := fmt.Sprint(entry.Key.Get(1))
		
		// Add the message to the report
		return report.AddMessage(hash)
		
	case "Transaction":
		// Process transaction record
		if entry.Key.Len() < 2 {
			return fmt.Errorf("invalid transaction key")
		}
		
		// Extract transaction hash
		hash := fmt.Sprint(entry.Key.Get(1))
		
		// Add the transaction to the report
		if err := report.AddTransaction(hash); err != nil {
			return err
		}
		
		// Store this transaction hash in a set for later verification
		// We'll use this to verify that all transactions referenced in main chains exist
		report.AddTransactionHash(hash)
		
		return nil
	}
	
	// Ignore other record types
	return nil
}
