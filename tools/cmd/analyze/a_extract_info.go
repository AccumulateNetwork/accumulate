package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/cometbft/cometbft/types"
	"github.com/spf13/cobra"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

// InfoCommand creates a command to display information about a snapshot file
func InfoCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info [snapshot-file]",
		Short: "Display information about a snapshot file",
		Long:  "Display detailed information about a snapshot file, including consensus section data",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			snapshotFile := args[0]
			if err := displaySnapshotInfo(snapshotFile); err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
		},
	}

	return cmd
}

// displaySnapshotInfo opens a snapshot file and displays information about it
func displaySnapshotInfo(snapshotFile string) error {
	// Open the snapshot file
	file, err := os.Open(snapshotFile)
	if err != nil {
		return fmt.Errorf("failed to open snapshot file: %w", err)
	}
	defer file.Close()

	// Open the snapshot
	reader, err := snapshot.Open(file)
	if err != nil {
		return fmt.Errorf("failed to open snapshot: %w", err)
	}

	// Display section information
	return displaySectionInfo(reader)
}

// displaySectionInfo displays information about all sections in the snapshot
func displaySectionInfo(reader *snapshot.Reader) error {
	// Display header information
	fmt.Println("Snapshot Header Information:")
	if reader.Header != nil {
		fmt.Printf("  Version: %d\n", reader.Header.Version)
		fmt.Printf("  Root Hash: %x\n", reader.Header.RootHash)
		if reader.Header.SystemLedger != nil && reader.Header.SystemLedger.Url != nil {
			fmt.Printf("  System Ledger URL: %s\n", reader.Header.SystemLedger.Url)
		} else {
			fmt.Printf("  System Ledger URL: <not set>\n")
		}
	} else {
		fmt.Println("  <Header not available>")
	}
	fmt.Println()

	// Display section information
	fmt.Println("Snapshot Sections:")
	for i, section := range reader.Sections {
		// Get section type and name
		sectionType := section.Type()
		typeName := getSectionTypeNameForAnalysis(sectionType)
		
		// Format size in a human-readable way
		size := section.Size()
		sizeStr := formatSize(size)
		
		fmt.Printf("  Section %d: Type %d (%s), Size %s\n", i, sectionType, typeName, sizeStr)
		
		// If this is a consensus section, display its contents
		if sectionType == snapshot.SectionTypeConsensus {
			reader, err := section.Open()
			if err != nil {
				fmt.Printf("    Error opening consensus section: %v\n", err)
				continue
			}
			if err := displayConsensusSection(reader); err != nil {
				fmt.Printf("    Error reading consensus data: %v\n", err)
			}
		}
	}
	
	// Print total section count
	fmt.Printf("\nTotal sections: %d\n", len(reader.Sections))
	return nil
}

// NOTE: getSectionTypeName function removed - using getSectionTypeNameForAnalysis from a_extract_section_scan.go instead

// displayConsensusSection reads and displays the contents of a consensus section
func displayConsensusSection(reader io.Reader) error {
	// Read the entire section content
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read consensus section: %w", err)
	}
	
	// First, print the raw JSON in a pretty format
	fmt.Println("\n    Consensus Section Raw JSON:")
	
	// Try to pretty-print the JSON
	var prettyJSON bytes.Buffer
	err = json.Indent(&prettyJSON, data, "      ", "  ")
	if err != nil {
		// If we can't pretty-print, show the raw data
		dataStr := string(data)
		if len(dataStr) > 1000 {
			dataStr = dataStr[:1000] + "... [truncated]"
		}
		fmt.Printf("      %s\n", dataStr)
	} else {
		// Print the pretty JSON
		fmt.Printf("      %s\n", prettyJSON.String())
	}
	
	// Now try to unmarshal the JSON data into a CometBFT GenesisDoc
	var doc types.GenesisDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		// If we can't unmarshal directly to GenesisDoc, print error details
		return fmt.Errorf("unmarshal consensus data: %w", err)
	}
	
	// Display the consensus information
	fmt.Println("\n    Consensus Information:")
	fmt.Printf("      Chain ID: %s\n", doc.ChainID)
	if !doc.GenesisTime.IsZero() {
		fmt.Printf("      Genesis Time: %s\n", doc.GenesisTime)
	}
	
	// Display consensus parameters if available
	if doc.ConsensusParams != nil {
		fmt.Println("      Consensus Parameters:")
		
		// Block parameters
		fmt.Printf("        Block Max Bytes: %d\n", doc.ConsensusParams.Block.MaxBytes)
		fmt.Printf("        Block Max Gas: %d\n", doc.ConsensusParams.Block.MaxGas)
		
		// Evidence parameters
		fmt.Printf("        Evidence Max Age Num Blocks: %d\n", doc.ConsensusParams.Evidence.MaxAgeNumBlocks)
		fmt.Printf("        Evidence Max Age Duration: %s\n", doc.ConsensusParams.Evidence.MaxAgeDuration)
	}
	
	// Display validator information
	if len(doc.Validators) > 0 {
		fmt.Printf("      Validators: %d\n", len(doc.Validators))
		for i, val := range doc.Validators {
			if i >= 5 { // Only show first 5 validators
				break
			}
			
			// Display validator information safely
			name := val.Name
			power := val.Power
			
			// Handle PubKey display
			pubKeyType := "<unknown>"
			pubKeyValue := "<unknown>"
			if val.PubKey != nil {
				pubKeyType = val.PubKey.Type()
				
				// Safely display truncated public key
				pubKeyBytes := val.PubKey.Bytes()
				if len(pubKeyBytes) >= 8 {
					pubKeyValue = fmt.Sprintf("%X", pubKeyBytes[:8]) + "..."
				} else if len(pubKeyBytes) > 0 {
					pubKeyValue = fmt.Sprintf("%X", pubKeyBytes)
				}
			}
			
			fmt.Printf("        %d: %s (Power: %d, PubKey: %s/%s)\n", 
				i+1, name, power, pubKeyType, pubKeyValue)
		}
		
		if len(doc.Validators) > 5 {
			fmt.Printf("        ... and %d more validators\n", len(doc.Validators)-5)
		}
	} else {
		fmt.Println("      No validators defined")
	}
	
	return nil
}

// formatSize formats a byte size into a human-readable string
func formatSize(bytes int64) string {
	const (
		_          = iota
		KB float64 = 1 << (10 * iota)
		MB
		GB
	)

	var size float64
	var unit string

	switch {
	case bytes >= int64(GB):
		size = float64(bytes) / GB
		unit = "GB"
	case bytes >= int64(MB):
		size = float64(bytes) / MB
		unit = "MB"
	case bytes >= int64(KB):
		size = float64(bytes) / KB
		unit = "KB"
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}

	return fmt.Sprintf("%.2f %s (%d bytes)", size, unit, bytes)
}
