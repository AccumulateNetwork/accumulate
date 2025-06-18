// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// DEVELOPMENT TEST INFORMATION (to be removed when complete):
// Test snapshot file: /home/paul/work/acc1/bvn0.snap
// Test command: go run . analyze snap /home/paul/work/acc1/bvn0.snap

// Snapshot Analysis Implementation
//
// This file contains code for analyzing Accumulate snapshot files directly,
// without requiring a full database ingest. The analysis focuses on extracting
// and reporting account and chain data exactly as it exists in the snapshot,
// without fabricating any missing information.
//
// Implementation Plan:
//
// Phase 1: Basic Snapshot Processing
// 1. Open the snapshot file - If this fails, print the argument and help for scan
// 2. Determine the snapshot version - If version 1, print an error, help for scan, and exit
// 3. If version 2, print the arguments
// 4. Walk through the scan file without additional processing
//
// Phase 2: Data Extraction (to be implemented)
// 1. Extract accounts and their chains
// 2. Collect statistics on accounts and chains
// 3. Identify accounts with missing or incomplete data
//
// Phase 3: Analysis and Reporting (to be implemented)
// 1. Generate detailed reports on accounts and chains
// 2. Provide statistics on account types, chain types, and entry counts
// 3. Highlight potential issues like missing chains or incomplete data
//
// Following the critical rule for Accumulate data analysis, this implementation
// strictly reports only what is found in the snapshot without fabricating any
// missing data. This ensures accurate reporting of the snapshot state for
// debugging and monitoring purposes, preventing issues that could arise from
// working with fabricated data.

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
	snapshot "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

var cmdAnalyzeSnap = &cobra.Command{
	Use:   "snap [snapshot-path]",
	Short: "Scans a snapshot file",
	Long: `Scans a snapshot file and provides basic information:
- Snapshot version
- Basic file structure
- Record counts by type`,
	Args: cobra.ExactArgs(1),
	RunE: scanSnapshot,
}

var cmdAnalyzeSnapVersion = &cobra.Command{
	Use:   "snap-version [snapshot-path]",
	Short: "Shows the version of a snapshot file",
	Long:  `Shows the version of a snapshot file`,
	Args:  cobra.ExactArgs(1),
	RunE:  analyzeSnapshotVersion,
}

// scanSnapshot implements the basic snapshot scanning functionality.
// It follows the Phase 1 plan:
// 1. Open the snapshot file - If this fails, print the argument and help for scan
// 2. Determine the snapshot version - If version 1, print an error, help for scan, and exit
// 3. If version 2, print the arguments
// 4. Walk through the scan file without additional processing
func scanSnapshot(cmd *cobra.Command, args []string) error {
	fmt.Println("=== Starting scanSnapshot function ===")
	
	// Step 1: Open the snapshot file
	if len(args) < 1 {
		fmt.Println("Error: snapshot file path required")
		return cmd.Help()
	}

	snapshotPath := args[0]
	fmt.Printf("Opening snapshot file: %s\n", snapshotPath)

	// Open the snapshot file
	fmt.Println("Attempting to open the file...")
	file, err := os.Open(snapshotPath)
	if err != nil {
		fmt.Printf("Error opening snapshot file %s: %v\n", snapshotPath, err)
		return cmd.Help()
	}
	fmt.Println("File opened successfully")
	defer file.Close()

	// Step 2: Determine the snapshot version
	fmt.Println("Determining snapshot version...")
	version, err := snapshot.GetVersion(file)
	if err != nil {
		fmt.Printf("Error determining snapshot version: %v\n", err)
		return cmd.Help()
	}
	fmt.Printf("Snapshot version detected: %d\n", version)

	// If version 1, print an error and exit
	if version == 1 {
		fmt.Println("Error: Version 1 snapshots are not supported")
		return cmd.Help()
	}

	// Step 3: If version 2, print the arguments
	if version == 2 {
		fmt.Printf("Processing snapshot version %d\n", version)
		fmt.Printf("Snapshot path: %s\n", snapshotPath)
	}

	// Reset file position
	fmt.Println("Resetting file position...")
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		fmt.Printf("Error resetting file position: %v\n", err)
		return fmt.Errorf("failed to reset file position: %w", err)
	}

	// Step 4: Walk through the scan file
	fmt.Println("Walking through snapshot file...")
	
	// Open the snapshot for reading
	fmt.Println("Opening snapshot for reading...")
	reader, err := snapshot.Open(file)
	if err != nil {
		fmt.Printf("Error opening snapshot: %v\n", err)
		return fmt.Errorf("failed to open snapshot: %w", err)
	}

	// Print basic snapshot information
	fmt.Println("Printing snapshot information:")
	fmt.Printf("Snapshot header version: %d\n", reader.Header.Version)
	fmt.Printf("Root hash: %X\n", reader.Header.RootHash)

	// Phase 2: Data Extraction
	fmt.Println("\n=== Starting Phase 2: Data Extraction ===")
	
	// Reset file position
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		fmt.Printf("Error resetting file position: %v\n", err)
		return fmt.Errorf("failed to reset file position for data extraction: %w", err)
	}

	// Extract basic information from the snapshot
	fmt.Println("Extracting basic information...")
	
	// Count records by type
	fmt.Println("Counting records by type...")
	recordCounts, err := countRecordsByType(file)
	if err != nil {
		fmt.Printf("Error counting records: %v\n", err)
		return fmt.Errorf("failed to count records: %w", err)
	}

	// Print record counts
	fmt.Printf("\nFound %d different record types\n", len(recordCounts))
	fmt.Println("Record counts by type:")
	for recordType, count := range recordCounts {
		fmt.Printf("  %s: %d\n", recordType, count)
	}

	fmt.Println("\n=== Data extraction completed successfully ===")
	return nil
}

// countRecordsByType counts the number of records by type in a snapshot file
func countRecordsByType(file io.ReadSeeker) (map[string]int, error) {
	// Reset file position
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to reset file position: %w", err)
	}

	// Convert to SectionReader which is required by snapshot.Open
	osFile, ok := file.(*os.File)
	if !ok {
		return nil, fmt.Errorf("file is not an os.File")
	}
	
	// Get file stats to determine size
	stat, err := osFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file stats: %w", err)
	}
	
	// Create a SectionReader
	sectionReader, err := ioutil2.NewSectionReader(osFile, 0, stat.Size())
	if err != nil {
		return nil, fmt.Errorf("failed to create section reader: %w", err)
	}
	
	// Open the snapshot file
	reader, err := snapshot.Open(sectionReader)
	if err != nil {
		return nil, fmt.Errorf("failed to open snapshot: %w", err)
	}

	// Count records by type
	counts := make(map[string]int)
	
	// Process each section
	fmt.Printf("Found %d sections in the snapshot\n", len(reader.Sections))
	for i := 0; i < len(reader.Sections); i++ {
		section := reader.Sections[i]
		
		// Print section information
		fmt.Printf("Section %d: Type=%d, Size=%d\n", i, section.Type(), section.Size())
		
		// Only process record sections
		if section.Type() != snapshot.SectionTypeRecords {
			fmt.Printf("Skipping non-record section %d\n", i)
			continue
		}
		
		// Open the record section
		fmt.Printf("Opening record section %d...\n", i)
		records, err := reader.OpenRecords(i)
		if err != nil {
			return nil, fmt.Errorf("failed to open record section %d: %w", i, err)
		}
		
		// Read each record
		recordCount := 0
		for {
			entry, err := records.Read()
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, fmt.Errorf("failed to read record: %w", err)
			}
			
			// Get the record type (first part of the key)
			recordType := fmt.Sprint(entry.Key.Get(0))
			
			// Count by record type
			counts[recordType]++
			recordCount++
			
			// Print progress every 10000 records
			if recordCount % 10000 == 0 {
				fmt.Printf("Processed %d records in section %d\n", recordCount, i)
			}
		}
		
		fmt.Printf("Completed section %d: processed %d records\n", i, recordCount)
	}

	return counts, nil
}

// analyzeSnapshotVersion shows the version of a snapshot file
// This function is used by the snap-version command
func analyzeSnapshotVersion(cmd *cobra.Command, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("snapshot file path required")
	}

	snapshotPath := args[0]
	fmt.Printf("Opening snapshot file: %s\n", snapshotPath)

	// Open the snapshot file
	f, err := os.Open(snapshotPath)
	if err != nil {
		return fmt.Errorf("failed to open snapshot file: %w", err)
	}
	defer f.Close()

	// Get the snapshot version
	v, err := snapshot.GetVersion(f)
	if err != nil {
		return fmt.Errorf("error getting snapshot version: %w", err)
	}

	fmt.Printf("Snapshot version: %d\n", v)
	return nil
}
