// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"compress/gzip"
	"fmt"
	"io"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

// SectionAnalysisInfo holds information about a snapshot section
type SectionAnalysisInfo struct {
	Index       int
	Type        snapshot.SectionType
	TypeName    string
	Size        int64
	Description string
}

// ScanSnapshotSections scans all sections in a snapshot and reports their sizes and types
func ScanSnapshotSections(reader *snapshot.Reader) ([]SectionAnalysisInfo, error) {
	var sections []SectionAnalysisInfo

	fmt.Printf("\n=== Snapshot Section Analysis ===\n")
	fmt.Printf("Total sections: %d\n\n", len(reader.Sections))

	for i, section := range reader.Sections {
		sectionInfo := SectionAnalysisInfo{
			Index:    i,
			Type:     section.Type(),
			TypeName: getSectionTypeNameForAnalysis(section.Type()),
			Size:     section.Size(),
		}

		// Add description based on section type
		switch section.Type() {
		case snapshot.SectionTypeHeader:
			sectionInfo.Description = "Snapshot metadata and configuration"
		case snapshot.SectionTypeAccountsV1:
			sectionInfo.Description = "Account records (main data)"
		case snapshot.SectionTypeTransactionsV1:
			sectionInfo.Description = "Transactions (v1 format, uncompressed)"
		case snapshot.SectionTypeSignaturesV1:
			sectionInfo.Description = "Signatures (v1 format)"
		case snapshot.SectionTypeGzTransactionsV1:
			sectionInfo.Description = "Gzipped transactions (v1 format, compressed)"
		case snapshot.SectionTypeSnapshot:
			sectionInfo.Description = "Nested snapshots"
		case snapshot.SectionTypeRecords:
			sectionInfo.Description = "Records (transactions/messages, v2 format)"
		case snapshot.SectionTypeRecordIndex:
			sectionInfo.Description = "Record index for fast lookups"
		case snapshot.SectionTypeRawBPT:
			sectionInfo.Description = "Binary Patricia Tree (raw format, deprecated)"
		case snapshot.SectionTypeConsensus:
			sectionInfo.Description = "Consensus parameters"
		case snapshot.SectionTypeBPT:
			sectionInfo.Description = "Binary Patricia Tree (current format)"
		default:
			sectionInfo.Description = "Unknown section type"
		}

		sections = append(sections, sectionInfo)

		// Print section info
		fmt.Printf("Section %d: %s (type %d)\n", i, sectionInfo.TypeName, int(section.Type()))
		fmt.Printf("  Size: %d bytes (%.2f KB, %.2f MB)\n", 
			sectionInfo.Size, 
			float64(sectionInfo.Size)/1024, 
			float64(sectionInfo.Size)/(1024*1024))
		fmt.Printf("  Description: %s\n", sectionInfo.Description)

		// Special handling for gzipped sections
		if section.Type() == snapshot.SectionTypeGzTransactionsV1 {
			uncompressedSize, err := getUncompressedSize(section)
			if err != nil {
				fmt.Printf("  Compression: Error reading uncompressed size: %v\n", err)
			} else {
				compressionRatio := float64(sectionInfo.Size) / float64(uncompressedSize) * 100
				fmt.Printf("  Uncompressed size: %d bytes (%.2f KB, %.2f MB)\n", 
					uncompressedSize,
					float64(uncompressedSize)/1024, 
					float64(uncompressedSize)/(1024*1024))
				fmt.Printf("  Compression ratio: %.1f%% (%.1fx reduction)\n", 
					compressionRatio, 
					float64(uncompressedSize)/float64(sectionInfo.Size))
			}
		}
		fmt.Println()
	}

	// Print summary
	var totalSize int64
	for _, section := range sections {
		totalSize += section.Size
	}

	fmt.Printf("=== Summary ===\n")
	fmt.Printf("Total snapshot size: %d bytes (%.2f KB, %.2f MB)\n", 
		totalSize, 
		float64(totalSize)/1024, 
		float64(totalSize)/(1024*1024))
	fmt.Printf("Average section size: %.2f KB\n", float64(totalSize)/float64(len(sections))/1024)

	return sections, nil
}

// getSectionTypeNameForAnalysis returns a human-readable name for a section type
func getSectionTypeNameForAnalysis(sectionType snapshot.SectionType) string {
	switch sectionType {
	case snapshot.SectionTypeHeader:
		return "Header"
	case snapshot.SectionTypeAccountsV1:
		return "AccountsV1"
	case snapshot.SectionTypeTransactionsV1:
		return "TransactionsV1"
	case snapshot.SectionTypeSignaturesV1:
		return "SignaturesV1"
	case snapshot.SectionTypeGzTransactionsV1:
		return "GzTransactionsV1"
	case snapshot.SectionTypeSnapshot:
		return "Snapshot"
	case snapshot.SectionTypeRecords:
		return "Records"
	case snapshot.SectionTypeRecordIndex:
		return "RecordIndex"
	case snapshot.SectionTypeRawBPT:
		return "RawBPT"
	case snapshot.SectionTypeConsensus:
		return "Consensus"
	case snapshot.SectionTypeBPT:
		return "BPT"
	default:
		return fmt.Sprintf("Unknown(%d)", int(sectionType))
	}
}

// getUncompressedSize reads a gzipped section and returns its uncompressed size
func getUncompressedSize(section *ioutil.Segment[snapshot.SectionType, *snapshot.SectionType]) (int64, error) {
	// Open the section
	sr, err := section.Open()
	if err != nil {
		return 0, fmt.Errorf("failed to open section: %v", err)
	}

	// Create gzip reader
	gz, err := gzip.NewReader(sr)
	if err != nil {
		return 0, fmt.Errorf("failed to create gzip reader: %v", err)
	}
	defer gz.Close()

	// Count bytes by reading through the entire stream
	var uncompressedSize int64
	buffer := make([]byte, 32*1024) // 32KB buffer
	for {
		n, err := gz.Read(buffer)
		uncompressedSize += int64(n)
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("error reading gzipped data: %v", err)
		}
	}

	return uncompressedSize, nil
}

// ProcessGzTransactionsSection processes a gzipped transactions section (type 5)
func ProcessGzTransactionsSection(reader *snapshot.Reader, sectionIndex int) error {
	section := reader.Sections[sectionIndex]
	if section.Type() != snapshot.SectionTypeGzTransactionsV1 {
		return fmt.Errorf("section %d is not a gzipped transactions section, got type: %v", 
			sectionIndex, section.Type())
	}

	fmt.Printf("Processing gzipped transactions section %d...\n", sectionIndex)

	// Open the section
	sr, err := section.Open()
	if err != nil {
		return fmt.Errorf("failed to open section: %v", err)
	}

	// Create gzip reader
	gz, err := gzip.NewReader(sr)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %v", err)
	}
	defer gz.Close()

	// Note: We would need to define txnSection struct to unmarshal
	// For now, just demonstrate the decompression process
	fmt.Printf("Successfully opened and decompressed section %d\n", sectionIndex)
	fmt.Printf("Section can be read as normal binary data after gzip decompression\n")

	return nil
}
