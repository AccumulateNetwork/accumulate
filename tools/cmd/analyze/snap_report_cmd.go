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

	"github.com/spf13/cobra"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

var (
	outputFile string
)

var (
	debugMode bool
	maxRecords int

	// Statistics for record types
	recordStats = struct {
		TotalRecords      int
		AccountRecords    int
		MainRecords       int
		ChainRecords      int
		UnknownTypes      int
		UnmarshalFailures int
	}{}
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
		return "empty"
	}
	
	if len(data) > maxBytes {
		data = data[:maxBytes]
	}
	
	var buf strings.Builder
	for i, b := range data {
		if i > 0 && i%16 == 0 {
			buf.WriteString("\n")
		} else if i > 0 {
			buf.WriteString(" ")
		}
		buf.WriteString(fmt.Sprintf("%02x", b))
	}
	
	return buf.String()
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
	fmt.Println("Creating snapshot report...")
	report, err := OpenReport()
	if err != nil {
		return fmt.Errorf("failed to create report: %w", err)
	}
	defer report.Close()

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
				fmt.Printf("Warning: failed to process record: %v\n", err)
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
	}
	
	return "Unknown"
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

// determineAccountType analyzes account data and determines the account type
func determineAccountType(data []byte, urlStr string) (string, error) {
	// Debug: Print the first few bytes of the account data
	dataPreview := ""
	if len(data) > 0 {
		previewLen := 16
		if len(data) < previewLen {
			previewLen = len(data)
		}
		dataPreview = fmt.Sprintf("%X", data[:previewLen])
	}

	// First attempt: Try to determine the account type from the URL structure
	accountType := determineAccountTypeFromURL(urlStr)
	if accountType != "Unknown" {
		return accountType, nil
	}

	// Second attempt: Try to unmarshal the account data using the Accumulate protocol
	account, err := protocol.UnmarshalAccount(data)
	if err == nil && account != nil {
		// Determine the account type based on the concrete type
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
		default:
			return fmt.Sprintf("Unknown (%T)", a), nil
		}
	}

	// Third attempt: Try to determine the type from the raw data pattern
	accountType = determineAccountTypeFromRawData(data)
	if accountType != "Unknown" {
		return accountType, nil
	}

	// If all attempts fail, return Unknown with debug info
	if err != nil {
		return "Unknown", fmt.Errorf("failed to unmarshal account data (bytes: %s): %w", dataPreview, err)
	}
	
	return "Unknown", nil
}

// processRecord processes a single record from the snapshot
func processRecord(report *SnapshotReport, entry *snapshot.RecordEntry) error {
	// Get the record type (first part of the key)
	if entry.Key == nil || entry.Key.Len() == 0 {
		return fmt.Errorf("empty key")
	}

	recordType := fmt.Sprint(entry.Key.Get(0))

	// Process based on record type
	switch recordType {
	case "Account":
		// Extract account URL
		urlStr := fmt.Sprint(entry.Key.Get(1))
		
		// Try to parse the URL to validate it
		parsedURL, err := url.Parse(urlStr)
		if err != nil {
			return fmt.Errorf("invalid account URL %q: %w", urlStr, err)
		}
		
		// Check if this is a Main record - only Main records contain the full account data
		// This follows the same pattern as genesis.Extract
		if entry.Key.Len() > 2 && fmt.Sprint(entry.Key.Get(2)) == "Main" {
			if debugMode {
				fmt.Printf("Found Main record for account %s\n", urlStr)
			}
			
			// Extract account type from the value using protocol.UnmarshalAccount
			accountType := "Unknown"
			if entry.Value != nil && len(entry.Value) > 0 {
				// Unmarshal the account using the protocol package
				acct, err := protocol.UnmarshalAccount(entry.Value)
				if err != nil {
					if debugMode {
						dataPreview := ""
						if len(entry.Value) > 0 {
							previewLen := 16
							if len(entry.Value) < previewLen {
								previewLen = len(entry.Value)
							}
							dataPreview = fmt.Sprintf("%X", entry.Value[:previewLen])
						}
						fmt.Printf("Warning: failed to unmarshal account data for %s (bytes: %s): %v\n", urlStr, dataPreview, err)
					}
				} else {
					// Determine the account type based on the concrete type
					switch a := acct.(type) {
					case *protocol.TokenAccount:
						accountType = "TokenAccount"
					case *protocol.LiteTokenAccount:
						accountType = "LiteTokenAccount"
					case *protocol.DataAccount:
						accountType = "DataAccount"
					case *protocol.LiteDataAccount:
						accountType = "LiteDataAccount"
					case *protocol.ADI:
						accountType = "Identity"
					case *protocol.KeyBook:
						accountType = "KeyBook"
					case *protocol.KeyPage:
						accountType = "KeyPage"
					case *protocol.SystemLedger:
						accountType = "SystemLedger"
					case *protocol.AnchorLedger:
						accountType = "AnchorLedger"
					case *protocol.SyntheticLedger:
						accountType = "SyntheticLedger"
					default:
						accountType = fmt.Sprintf("Unknown (%T)", a)
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
		// Process chain record
		if entry.Key.Len() < 3 {
			return fmt.Errorf("invalid chain key")
		}
		
		// Extract account URL and chain ID
		urlStr := fmt.Sprint(entry.Key.Get(1))
		chainID := fmt.Sprint(entry.Key.Get(2))
		
		// Add the chain to the report
		return report.AddChain(urlStr, chainID)
		
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
		return report.AddTransaction(hash)
	}
	
	// Ignore other record types
	return nil
}
