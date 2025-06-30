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
	"path/filepath"
	
	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

// References to important snapshot-related structures and functions:
//
// Header struct:
// - Defined in: pkg/database/snapshot/types_gen.go
// - Type: snapshot.Header
// - Fields:
//   - Version uint64
//   - RootHash [32]byte
//   - SystemLedger *protocol.SystemLedger
//
// Reader:
// - Defined in: pkg/database/snapshot/format.go
// - Type: snapshot.Reader
// - Created via: snapshot.Open(io.ReadSeeker) (*Reader, error)
// - Key methods:
//   - OpenRecords(int) (RecordReader, error)
//   - OpenIndex(int) (IndexReader, error)
//
// Writer:
// - Defined in: pkg/database/snapshot/format.go
// - Type: snapshot.Writer
// - Created via: snapshot.Create(io.WriteSeeker) (*Writer, error)
// - Key methods:
//   - WriteHeader(*Header) error
//   - OpenRaw(SectionType) (io.WriteCloser, error)
//   - OpenRecords() (*RecordWriter, error)
//   - OpenIndex() (*IndexWriter, error)
//
// Section Types:
// - Defined in: pkg/database/snapshot/enums_gen.go
// - Key types:
//   - SectionTypeHeader = 1
//   - SectionTypeSnapshot = 6
//   - SectionTypeRecords = 7
//   - SectionTypeRecordIndex = 8
//   - SectionTypeBPT = 11
//
// Record Entry:
// - Defined in: pkg/database/snapshot/types_gen.go
// - Type: snapshot.RecordEntry
// - Fields:
//   - Key *record.Key
//   - Value []byte
//   - Receipt *merkle.Receipt

// sectionScan parses and dumps all section headers from a snapshot file
// It takes an sc_State with an initialized open snapshot file (inFile)
// and prints information about each section header
func sectionScan(scState *sc_State) error {
	if len(scState.InputFiles) == 0 || scState.InputFiles[0] == nil {
		return fmt.Errorf("no input snapshot file available")
	}

	// Use the first input file
	inFile := scState.InputFiles[0]

	// Seek to the beginning of the file
	_, err := inFile.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek to beginning of snapshot file: %w", err)
	}

	// Open the snapshot using the snapshot package
	snapshotReader, err := snapshot.Open(inFile)
	if err != nil {
		return fmt.Errorf("failed to open snapshot file: %w", err)
	}

	// Print snapshot header information
	fmt.Printf("=== Snapshot Header ===\n")
	fmt.Printf("Version: %d\n", snapshotReader.Header.Version)
	fmt.Printf("Root Hash: %x\n", snapshotReader.Header.RootHash)

	// Print system ledger information if available
	if snapshotReader.Header.SystemLedger != nil {
		fmt.Printf("System Ledger URL: %v\n", snapshotReader.Header.SystemLedger.Url)
		fmt.Printf("System Ledger Index: %d\n", snapshotReader.Header.SystemLedger.Index)
		fmt.Printf("System Ledger Timestamp: %v\n", snapshotReader.Header.SystemLedger.Timestamp)
		fmt.Printf("Executor Version: %d\n", snapshotReader.Header.SystemLedger.ExecutorVersion)
	}

	// Print information about each section
	fmt.Printf("\n=== Sections (%d total) ===\n", len(snapshotReader.Sections))
	fmt.Printf("\n=== Processing Sections in Original Order ===\n")
	
	// Create a map to track the count of each section type
	typeCounts := make(map[snapshot.SectionType]int)

	for i, section := range snapshotReader.Sections {
		// Get section type name
		typeName := getSectionTypeName(section.Type())
		
		// Increment the count for this section type
		typeCounts[section.Type()]++
		
		// Print section information
		fmt.Printf("Section %d:\n", i)
		fmt.Printf("  Type: %d (%s)\n", section.Type(), typeName)
		fmt.Printf("  Size: %d bytes\n", section.Size())
		fmt.Printf("  Offset: 0x%x\n", section.Offset())
	}
	
	// Print section type counts
	fmt.Printf("\n=== Section Type Counts ===\n")
	for sectionType, count := range typeCounts {
		fmt.Printf("  %s: %d\n", getSectionTypeName(sectionType), count)
	}
	
	// Create a temporary directory for section files if it doesn't exist
	tmpDir := "/tmp/accumulate-snapshot-sections"
	err = os.MkdirAll(tmpDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	
	// Extract the header section to a temporary file
	headerFilePath, headerSize, err := writeHeaderToTempFile(snapshotReader, inFile, tmpDir)
	if err != nil {
		return fmt.Errorf("failed to write header to temporary file: %w", err)
	}
	
	fmt.Printf("\nHeader section extracted to: %s (%d bytes)\n", headerFilePath, headerSize)
	
	// Extract all remaining sections to temporary files
	for i := 1; i < len(snapshotReader.Sections); i++ {
		sectionFilePath, sectionSize, err := writeNextSectionToTempFile(snapshotReader, inFile, tmpDir, i)
		if err != nil {
			return fmt.Errorf("failed to write section %d to temporary file: %w", i, err)
		}
		
		sectionType := snapshotReader.Sections[i].Type()
		sectionTypeName := getSectionTypeName(sectionType)
		fmt.Printf("Section %d (%s) extracted to: %s (%d bytes)\n", i, sectionTypeName, sectionFilePath, sectionSize)
	}

	return nil
}

// writeHeaderToTempFile writes the header section to a temporary file with the new naming format
func writeHeaderToTempFile(reader *snapshot.Reader, inFile *os.File, tmpDir string) (string, int64, error) {
	// Find the header section in the snapshot
	var headerSection *ioutil.Segment[snapshot.SectionType, *snapshot.SectionType]
	for _, section := range reader.Sections {
		if section.Type() == snapshot.SectionTypeHeader {
			headerSection = section
			break
		}
	}
	
	if headerSection == nil {
		return "", 0, fmt.Errorf("header section not found")
	}
	
	// Create a temporary file for the header section
	headerFilePath := filepath.Join(tmpDir, "Order_00_Section_Type_1.bin")
	headerFile, err := os.Create(headerFilePath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create temporary file for header section: %w", err)
	}
	defer headerFile.Close()
	
	// Get the section offset and size
	sectionOffset := headerSection.Offset()
	sectionSize := headerSection.Size()
	
	// Include the 64-byte section header in the extraction
	// The section offset points to the section data, so we need to go back 64 bytes
	headerOffset := sectionOffset - 64
	totalSize := sectionSize + 64
	
	// Seek to the section header
	_, err = inFile.Seek(int64(headerOffset), io.SeekStart)
	if err != nil {
		return "", 0, fmt.Errorf("failed to seek to header section: %w", err)
	}
	
	// Read the entire section including its header
	sectionData := make([]byte, totalSize)
	_, err = io.ReadFull(inFile, sectionData)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read header section: %w", err)
	}
	
	// Write the section data to the temporary file
	_, err = headerFile.Write(sectionData)
	if err != nil {
		return "", 0, fmt.Errorf("failed to write header section to temporary file: %w", err)
	}
	
	// Print some information about the extracted section
	fmt.Printf("  Complete header section written to: %s (%d bytes)\n", 
		filepath.Base(headerFilePath), totalSize)
	fmt.Printf("  Section offset: 0x%x, Section size: %d bytes (including 64-byte header)\n", 
		headerOffset, totalSize)
	
	return headerFilePath, int64(totalSize), nil
}

// writeNextSectionToTempFile extracts the specified section from a snapshot file and writes it to a temporary file
func writeNextSectionToTempFile(snapshotReader *snapshot.Reader, inFile *os.File, tmpDir string, sectionIndex int) (string, int64, error) {
	// Make sure the section index is valid
	if sectionIndex < 0 || sectionIndex >= len(snapshotReader.Sections) {
		return "", 0, fmt.Errorf("invalid section index: %d", sectionIndex)
	}
	
	// Get the section
	section := snapshotReader.Sections[sectionIndex]
	
	// Create a temporary file for the section
	// Use the section index (1-based for the filename) and type for the filename
	sectionFilePath := filepath.Join(tmpDir, fmt.Sprintf("Order_%02d_Section_Type_%d.bin", sectionIndex, section.Type()))
	sectionFile, err := os.Create(sectionFilePath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create temporary file for section: %w", err)
	}
	defer sectionFile.Close()
	
	// Get the section offset and size
	sectionOffset := section.Offset()
	sectionSize := section.Size()
	
	// Include the 64-byte section header in the extraction
	// The section offset points to the section data, so we need to go back 64 bytes
	headerOffset := sectionOffset - 64
	totalSize := sectionSize + 64
	
	// Seek to the section header
	_, err = inFile.Seek(int64(headerOffset), io.SeekStart)
	if err != nil {
		return "", 0, fmt.Errorf("failed to seek to section: %w", err)
	}
	
	// Read the entire section including its header
	sectionData := make([]byte, totalSize)
	_, err = io.ReadFull(inFile, sectionData)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read section: %w", err)
	}
	
	// Write the section data to the temporary file
	_, err = sectionFile.Write(sectionData)
	if err != nil {
		return "", 0, fmt.Errorf("failed to write section to temporary file: %w", err)
	}
	
	// Print some information about the extracted section
	fmt.Printf("  Complete section written to: %s (%d bytes)\n", 
		filepath.Base(sectionFilePath), totalSize)
	fmt.Printf("  Section offset: 0x%x, Section size: %d bytes (including 64-byte header)\n", 
		headerOffset, totalSize)
	
	return sectionFilePath, int64(totalSize), nil
}

// getSectionTypeName returns a human-readable name for a section type
func getSectionTypeName(sectionType snapshot.SectionType) string {
	switch sectionType {
	case snapshot.SectionTypeHeader:
		return "Header"
	case snapshot.SectionTypeSnapshot:
		return "Nested Snapshot"
	case snapshot.SectionTypeRecords:
		return "Records"
	case snapshot.SectionTypeRecordIndex:
		return "Record Index"
	case snapshot.SectionTypeBPT:
		return "BPT"
	case snapshot.SectionTypeRawBPT:
		return "Raw BPT (Deprecated)"
	case snapshot.SectionTypeConsensus:
		return "Consensus Parameters"
	case snapshot.SectionTypeAccountsV1:
		return "Accounts (v1)"
	case snapshot.SectionTypeTransactionsV1:
		return "Transactions (v1)"
	case snapshot.SectionTypeSignaturesV1:
		return "Signatures (v1)"
	case snapshot.SectionTypeGzTransactionsV1:
		return "Gzipped Transactions (v1)"
	default:
		return "Unknown"
	}
}
