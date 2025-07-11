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
	"os"
	"sort"

	"path/filepath"

	"github.com/spf13/cobra"
	sv2 "gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// MergeState holds the state for merging multiple snapshots
type MergeState struct {
	InputSnapshots []*SnapshotInfo
	OutputFile     string
	AccountRecords []*sv2.RecordEntry // All account records for first section 7
	OtherRecords   []*sv2.RecordEntry // All non-account records for second section 7
	BloomFilter    *Bloom            // Bloom filter for account verification
}

// SnapshotInfo holds information about a snapshot file
type SnapshotInfo struct {
	FilePath string
	File     *os.File
	Reader   *sv2.Reader
	Size     int64
}

var cmdMerge = &cobra.Command{
	Use:   "merge <output> <snapshot1> <snapshot2> [snapshot3...]",
	Short: "Merge multiple snapshots into a single output",
	Long: `Merge multiple snapshots into a single output snapshot.

This command:
1. Reads all input snapshots
2. Loads all sections except BPT from the first snapshot
3. Loads section 7 (records) from all subsequent snapshots
4. Sorts section 7 records
5. Uses bloom filter to filter messages/transactions by accounts
6. Writes the combined snapshot

Usage:
  analyze merge output.snap snapshot1.snap snapshot2.snap`,
	Args: cobra.MinimumNArgs(3),
	RunE: runMerge,
}

func runMerge(cmd *cobra.Command, args []string) error {
	fmt.Println("=== Starting Snapshot Merge ===")

	// Validate arguments
	if len(args) < 3 {
		return fmt.Errorf("need at least 2 input snapshots and 1 output file")
	}

	// Split output file and input snapshots
	outputFile := args[0]
	inputFiles := args[1:]

	fmt.Printf("Input snapshots: %v\n", inputFiles)
	fmt.Printf("Output file: %s\n", outputFile)

	// Create merge state
	mergeState := &MergeState{
		InputSnapshots: make([]*SnapshotInfo, 0, len(inputFiles)),
		OutputFile:     outputFile,
		BloomFilter:    NewBloom("merged-snapshot"),
	}

	// Step 1: Read in all the snapshots
	err := loadAllSnapshots(mergeState, inputFiles)
	if err != nil {
		return fmt.Errorf("failed to load snapshots: %w", err)
	}

	// Step 2: Load and segregate all section 7 records
	err = loadAndSegregateRecords(mergeState)
	if err != nil {
		return fmt.Errorf("failed to load and segregate records: %w", err)
	}

	// Step 3: Populate bloom filter with account hashes
	err = populateBloomFilter(mergeState)
	if err != nil {
		return fmt.Errorf("failed to populate bloom filter: %w", err)
	}

	// Step 4: Sort section 7 records
	fmt.Println("\n=== Step 3: Sorting section 7 records ===")
	fmt.Println("Sorting account records...")
	sortMergeRecords(mergeState.AccountRecords)
	fmt.Println("Sorting other records...")
	sortMergeRecords(mergeState.OtherRecords)

	// Step 4: Write combined snapshot
	err = writeSnapshot(mergeState)
	if err != nil {
		return fmt.Errorf("failed to write combined snapshot: %w", err)
	}

	// Cleanup
	defer func() {
		for _, snapshot := range mergeState.InputSnapshots {
			if snapshot.File != nil {
				snapshot.File.Close()
			}
		}
	}()

	fmt.Println("=== Snapshot Merge Completed Successfully ===")
	return nil
}

// loadAndSegregateRecords loads all section 7 records from all snapshots
// and segregates them into account records vs other records
func loadAndSegregateRecords(mergeState *MergeState) error {
	fmt.Println("\n=== Step 2: Loading and segregating section 7 records ===")

	// Initialize record collections
	mergeState.AccountRecords = make([]*sv2.RecordEntry, 0)
	mergeState.OtherRecords = make([]*sv2.RecordEntry, 0)

	totalAccountRecords := 0
	totalOtherRecords := 0

	// Process each snapshot
	for i, snapshot := range mergeState.InputSnapshots {
		fmt.Printf("Processing snapshot %d: %s\n", i+1, snapshot.FilePath)

		// Find all section 7 (records) sections
		recordSections := make([]int, 0)
		for j, section := range snapshot.Reader.Sections {
			if section.Type() == sv2.SectionTypeRecords {
				recordSections = append(recordSections, j)
			}
		}

		fmt.Printf("  Found %d record sections: %v\n", len(recordSections), recordSections)

		// Process each record section
		for _, sectionIndex := range recordSections {
			fmt.Printf("  Processing section %d (records)...\n", sectionIndex)

			accountCount, otherCount, err := loadRecordsFromSection(snapshot.Reader, sectionIndex, mergeState)
			if err != nil {
				return fmt.Errorf("failed to load records from section %d: %w", sectionIndex, err)
			}

			totalAccountRecords += accountCount
			totalOtherRecords += otherCount
			fmt.Printf("    Loaded %d account records, %d other records\n", accountCount, otherCount)
		}
	}

	fmt.Printf("\n=== Record Segregation Summary ===\n")
	fmt.Printf("Total account records: %d (will go to first section 7)\n", totalAccountRecords)
	fmt.Printf("Total other records: %d (will go to second section 7)\n", totalOtherRecords)
	fmt.Printf("Total records loaded: %d\n", totalAccountRecords+totalOtherRecords)

	return nil
}

// loadRecordsFromSection loads records from a specific section and segregates them
func loadRecordsFromSection(reader *sv2.Reader, sectionIndex int, mergeState *MergeState) (accountCount, otherCount int, err error) {
	// Open the record section using the proper API
	records, err := reader.OpenRecords(sectionIndex)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to open record section %d: %w", sectionIndex, err)
	}

	accountCount = 0
	otherCount = 0

	// Read all records from the section
	for {
		entry, err := records.Read()
		if err != nil {
			if err == io.EOF {
				break // End of section
			}
			return accountCount, otherCount, fmt.Errorf("failed to read record: %w", err)
		}

		// Determine if this is an account record or other record
		if isAccountRecord(entry) {
			mergeState.AccountRecords = append(mergeState.AccountRecords, entry)
			accountCount++
		} else {
			mergeState.OtherRecords = append(mergeState.OtherRecords, entry)
			otherCount++
		}
	}

	return accountCount, otherCount, nil
}

// isAccountRecord determines if a record entry represents an account
func isAccountRecord(entry *sv2.RecordEntry) bool {
	// Attempt to unmarshal the record's value as a protocol.Account
	account, err := protocol.UnmarshalAccountFrom(io.NewSectionReader(bytes.NewReader(entry.Value), 0, int64(len(entry.Value))))
	if err != nil || account == nil {
		return false
	}

	// Check if the account URL is valid
	accountURL := account.GetUrl()
	return accountURL != nil
}

// loadAllSnapshots loads all input snapshot files
func loadAllSnapshots(mergeState *MergeState, inputFiles []string) error {
	fmt.Printf("\n=== Step 1: Loading %d snapshots ===\n", len(inputFiles))

	for i, filePath := range inputFiles {
		fmt.Printf("Loading snapshot %d: %s\n", i+1, filePath)

		// Check if file exists
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return fmt.Errorf("snapshot file does not exist: %s", filePath)
		}

		// Open the file
		file, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open snapshot file %s: %w", filePath, err)
		}

		// Get file size
		stat, err := file.Stat()
		if err != nil {
			file.Close()
			return fmt.Errorf("failed to get file stats for %s: %w", filePath, err)
		}

		// Create section reader
		sectionReader := io.NewSectionReader(file, 0, stat.Size())

		// Open snapshot reader
		reader, err := sv2.Open(sectionReader)
		if err != nil {
			file.Close()
			return fmt.Errorf("failed to open snapshot reader for %s: %w", filePath, err)
		}

		// Create snapshot info
		snapshot := &SnapshotInfo{
			FilePath: filePath,
			File:     file,
			Reader:   reader,
			Size:     stat.Size(),
		}

		// Add to merge state
		mergeState.InputSnapshots = append(mergeState.InputSnapshots, snapshot)

		// Print snapshot information
		fmt.Printf("  File: %s\n", filepath.Base(filePath))
		fmt.Printf("  Size: %d bytes (%.2f MB)\n", stat.Size(), float64(stat.Size())/(1024*1024))
		fmt.Printf("  Sections: %d\n", len(reader.Sections))

		// Print section information
		var processedSections, skippedSections int
		for j, section := range reader.Sections {
			sectionType := section.Type()
			sectionTypeName := getMergeSectionTypeName(sectionType)

			if sectionType == sv2.SectionTypeRecordIndex {
				fmt.Printf("    Section %d: Type %d (%s) [SKIPPED - will be rebuilt]\n", j, int(sectionType), sectionTypeName)
				skippedSections++
			} else {
				fmt.Printf("    Section %d: Type %d (%s)\n", j, int(sectionType), sectionTypeName)
				processedSections++
			}
		}

		fmt.Printf("  Sections to process: %d, Sections to skip: %d\n", processedSections, skippedSections)

		fmt.Printf("  Successfully loaded\n\n")
	}

	fmt.Printf("Successfully loaded %d snapshots\n", len(mergeState.InputSnapshots))
	return nil
}

// getMergeSectionTypeName returns a human-readable name for a section type
func getMergeSectionTypeName(sectionType sv2.SectionType) string {
	switch sectionType {
	case sv2.SectionTypeHeader:
		return "header"
	case sv2.SectionTypeAccountsV1:
		return "accounts-v1"
	case sv2.SectionTypeTransactionsV1:
		return "transactions-v1"
	case sv2.SectionTypeSignaturesV1:
		return "signatures-v1"
	case sv2.SectionTypeGzTransactionsV1:
		return "gz-transactions-v1"
	case sv2.SectionTypeSnapshot:
		return "snapshot"
	case sv2.SectionTypeRecords:
		return "records"
	case sv2.SectionTypeRecordIndex:
		return "record-index"
	case sv2.SectionTypeRawBPT:
		return "raw-bpt"
	case sv2.SectionTypeConsensus:
		return "consensus"
	case sv2.SectionTypeBPT:
		return "bpt"
	default:
		return fmt.Sprintf("unknown-%d", int(sectionType))
	}
}

// sortMergeRecords sorts the records in place based on their keys
func sortMergeRecords(records []*sv2.RecordEntry) {
	sort.Slice(records, func(i, j int) bool {
		keyI, _ := records[i].Key.MarshalBinary()
		keyJ, _ := records[j].Key.MarshalBinary()
		return bytes.Compare(keyI, keyJ) < 0
	})
}

// writeSnapshot writes the merged snapshot to the output file
func writeSnapshot(mergeState *MergeState) error {
	fmt.Println("\n=== Step 4: Writing combined snapshot ===")
	fmt.Printf("Output file: %s\n", mergeState.OutputFile)
	
	// Create output file
	outFile, err := os.Create(mergeState.OutputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()
	
	// Create snapshot writer
	writer, err := sv2.Create(outFile)
	if err != nil {
		return fmt.Errorf("failed to create snapshot writer: %w", err)
	}
	
	// Use the first snapshot as the base for header
	firstSnapshot := mergeState.InputSnapshots[0]
	
	// Write header section from first snapshot
	fmt.Println("Writing header section...")
	err = writer.WriteHeader(firstSnapshot.Reader.Header)
	if err != nil {
		return fmt.Errorf("failed to write header section: %w", err)
	}
	
	// Copy BPT section from first snapshot
	fmt.Println("Writing BPT section...")
	err = copySection(writer, firstSnapshot.Reader, 1, sv2.SectionTypeBPT)
	if err != nil {
		return fmt.Errorf("failed to copy BPT section: %w", err)
	}
	
	// Write first section 7 (account records)
	fmt.Printf("Writing account records section (%d records)...\n", len(mergeState.AccountRecords))
	err = writeRecordSection(writer, mergeState.AccountRecords)
	if err != nil {
		return fmt.Errorf("failed to write account records section: %w", err)
	}
	
	// Write second section 7 (other records)
	fmt.Printf("Writing other records section (%d records)...\n", len(mergeState.OtherRecords))
	err = writeRecordSection(writer, mergeState.OtherRecords)
	if err != nil {
		return fmt.Errorf("failed to write other records section: %w", err)
	}
	
	fmt.Println("Successfully wrote combined snapshot")
	return nil
}

// copySection copies a section from the source snapshot to the destination writer
func copySection(writer *sv2.Writer, reader *sv2.Reader, sectionIndex int, sectionType sv2.SectionType) error {
	// Get the section from the reader
	if sectionIndex >= len(reader.Sections) {
		return fmt.Errorf("section index %d out of range", sectionIndex)
	}
	section := reader.Sections[sectionIndex]
	// Open the source section for reading
	sourceReader, err := section.Open()
	if err != nil {
		return fmt.Errorf("failed to open source section: %w", err)
	}
	
	// Open destination section for writing
	destSection, err := writer.OpenRaw(sectionType)
	if err != nil {
		return fmt.Errorf("failed to open destination section: %w", err)
	}
	defer destSection.Close()
	
	// Copy all data from source to destination
	_, err = io.Copy(destSection, sourceReader)
	if err != nil {
		return fmt.Errorf("failed to copy section data: %w", err)
	}
	
	return nil
}

// writeRecordSection writes a slice of records to a new records section
func writeRecordSection(writer *sv2.Writer, records []*sv2.RecordEntry) error {
	// Open a new records section
	collector, err := writer.OpenRecords()
	if err != nil {
		return fmt.Errorf("failed to open records section: %w", err)
	}
	defer collector.Close()
	
	// Write each record to the section
	for _, record := range records {
		err = collector.WriteRecord(record)
		if err != nil {
			return fmt.Errorf("failed to write record: %w", err)
		}
	}
	
	return nil
}
