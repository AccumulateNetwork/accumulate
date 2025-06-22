// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package test_reader

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	ioutil2 "gitlab.com/accumulatenetwork/accumulate/internal/util/io"
)

func main() {
	// Check if a snapshot file path was provided
	if len(os.Args) < 2 {
		fmt.Println("Usage: test_snapshot_reader <snapshot-file>")
		os.Exit(1)
	}

	// Get the snapshot file path
	snapshotPath := os.Args[1]
	fmt.Printf("Testing snapshot reader with file: %s\n", snapshotPath)

	// Open the snapshot file
	file, err := os.Open(snapshotPath)
	if err != nil {
		fmt.Printf("Failed to open snapshot file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	// Get file info for size
	fileInfo, err := file.Stat()
	if err != nil {
		fmt.Printf("Failed to get file info: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Snapshot file size: %d bytes\n", fileInfo.Size())

	// Create a section reader for the file
	sectionReader, err := ioutil2.NewSectionReader(file, 0, fileInfo.Size())
	if err != nil {
		fmt.Printf("Failed to create section reader: %v\n", err)
		os.Exit(1)
	}

	// Open the snapshot using the snapshot package
	startTime := time.Now()
	reader, err := snapshot.Open(sectionReader)
	if err != nil {
		fmt.Printf("Failed to open snapshot: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Opened snapshot in %v\n", time.Since(startTime))

	// Print snapshot information
	fmt.Printf("Number of sections: %d\n", len(reader.Sections))

	// Process each section
	totalRecords := 0
	recordsByType := make(map[string]int)

	for i := 0; i < len(reader.Sections); i++ {
		section := reader.Sections[i]
		sectionType := section.Type()

		// Determine section type name for logging
		sectionTypeStr := "Unknown"
		switch sectionType {
		case snapshot.SectionTypeHeader:
			sectionTypeStr = "Header"
		case snapshot.SectionTypeSnapshot:
			sectionTypeStr = "Snapshot"
		case snapshot.SectionTypeRecords:
			sectionTypeStr = "Records"
		case snapshot.SectionTypeRecordIndex:
			sectionTypeStr = "RecordIndex"
		case snapshot.SectionTypeBPT:
			sectionTypeStr = "BPT"
		case snapshot.SectionTypeRawBPT:
			sectionTypeStr = "RawBPT"
		case snapshot.SectionTypeConsensus:
			sectionTypeStr = "Consensus"
		}

		fmt.Printf("Processing section %d: type=%s\n", i, sectionTypeStr)

		// Skip non-record sections
		if sectionType != snapshot.SectionTypeRecords {
			fmt.Printf("  Skipping non-record section\n")
			continue
		}

		// Open the record section
		sectionStartTime := time.Now()
		records, err := reader.OpenRecords(i)
		if err != nil {
			fmt.Printf("  Failed to open record section: %v\n", err)
			continue
		}

		// Process records
		sectionRecords := 0
		fmt.Printf("  Reading records from section %d\n", i)

		for {
			// Read the next record
			recordStartTime := time.Now()
			recordEntry, err := records.Read()
			if err != nil {
				if err == io.EOF {
					fmt.Printf("  Reached EOF after reading %d records\n", sectionRecords)
					break
				}
				fmt.Printf("  Failed to read record: %v\n", err)
				break
			}

			// Extract key and value
			keyStr := recordEntry.Key.String()
			valueBytes := recordEntry.Value

			// Extract record type from key
			recordType := "Unknown"
			parts := strings.Split(keyStr, "/")
			if len(parts) > 0 {
				recordType = parts[0]
			}

			// Print record information for every 10000th record
			if sectionRecords%10000 == 0 {
				fmt.Printf("  Record %d: type=%s, key=%s, value_len=%d, time=%v\n",
					sectionRecords, recordType, keyStr, len(valueBytes), time.Since(recordStartTime))
			}

			// Update statistics
			sectionRecords++
			recordsByType[recordType]++
		}

		// Log section completion
		fmt.Printf("  Processed %d records in %v\n", sectionRecords, time.Since(sectionStartTime))
		totalRecords += sectionRecords
	}

	// Print summary
	fmt.Printf("\nSummary:\n")
	fmt.Printf("Total records processed: %d\n", totalRecords)
	fmt.Printf("Records by type:\n")
	for recordType, count := range recordsByType {
		fmt.Printf("  %s: %d\n", recordType, count)
	}
	fmt.Printf("Total processing time: %v\n", time.Since(startTime))
}
