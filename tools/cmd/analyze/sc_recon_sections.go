package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

// sc_WriteSections writes all sections to the destination file
func sc_WriteSections(scState *sc_State) error {
	fmt.Printf("Writing sections to output file...\n")

	// Get all sections
	sections := scState.SectionFiles.List()
	if len(sections) == 0 {
		return fmt.Errorf("no sections to write")
	}

	// Check if we have input files
	if len(scState.InputFiles) == 0 {
		return fmt.Errorf("no input snapshot files available")
	}

	// Use the first input snapshot file as the source for the header and section 1_1
	firstSnapshotFile := scState.InputFiles[0]

	// Initialize output position counter
	var currentPos int64 = 0

	// Step 1: Use snapshot.Open to read the snapshot file
	// This properly handles the file header and section headers
	_, err := firstSnapshotFile.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek to beginning of first snapshot file: %w", err)
	}

	// Open the snapshot using the snapshot package
	snapshotReader, err := snapshot.Open(firstSnapshotFile)
	if err != nil {
		return fmt.Errorf("failed to open snapshot file: %w", err)
	}

	// Get the snapshot header
	header := snapshotReader.Header
	fmt.Printf("Snapshot version: %d\n", header.Version)
	fmt.Printf("Root hash: %x\n", header.RootHash)

	// Seek back to the beginning of the file to copy the raw header bytes
	_, err = firstSnapshotFile.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek to beginning of first snapshot file: %w", err)
	}

	// Read the 64-byte snapshot header
	headerBuf := make([]byte, 64)
	_, err = io.ReadFull(firstSnapshotFile, headerBuf)
	if err != nil {
		return fmt.Errorf("failed to read snapshot header from first snapshot file: %w", err)
	}

	// Write the header to the output file
	_, err = scState.OutFile.Write(headerBuf)
	if err != nil {
		return fmt.Errorf("failed to write snapshot header to output file: %w", err)
	}

	fmt.Printf("Copied 64-byte snapshot header from first input file\n")
	currentPos = 64

	// Step 2: Process the header section (section 1_1) using the snapshot package
	// Find the header section in the snapshot
	var headerSectionReader io.Reader
	for i, s := range snapshotReader.Sections {
		if s.Type() == snapshot.SectionTypeHeader {
			// Open the header section
			reader, err := snapshotReader.Open(snapshot.SectionTypeHeader)
			if err != nil {
				return fmt.Errorf("failed to open header section: %w", err)
			}
			headerSectionReader = reader
			fmt.Printf("Found header section at index %d\n", i)
			break
		}
	}

	if headerSectionReader == nil {
		return fmt.Errorf("header section not found in snapshot")
	}

	// Get the header section from our temporary files
	headerSection := scState.SectionFiles.Get("1_1")
	if headerSection == nil {
		return fmt.Errorf("section 1_1 not found in temporary files")
	}

	// Get the size of the header section
	tmpFileInfo, err := headerSection.TmpFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get section 1_1 file info: %w", err)
	}
	section1Size := tmpFileInfo.Size()
	fmt.Printf("Header section size from temp files: %d bytes\n", section1Size)

	// Sanity check on section size - if it's unreasonably large, something is wrong
	if section1Size > 100*1024*1024 { // 100 MB limit as a reasonable maximum
		return fmt.Errorf("section 1_1 size is unreasonably large (%d bytes), possible format error", section1Size)
	}

	// Prepare to write the section header
	sectionType := uint16(snapshot.SectionTypeHeader)
	fmt.Printf("Writing header section (type %d)\n", sectionType)

	// Create a new section header (64 bytes)
	sectionHeaderBuf := make([]byte, 64)

	// Set the section type (2 bytes, big-endian)
	binary.BigEndian.PutUint16(sectionHeaderBuf[0:2], sectionType)

	// Set the section size (8 bytes, big-endian)
	binary.BigEndian.PutUint64(sectionHeaderBuf[8:16], uint64(section1Size))

	// Calculate and set the next section offset (8 bytes, big-endian)
	// Next section starts after this header (64 bytes) + section data + padding
	nextSectionOffset := currentPos + 64 + section1Size
	// Add padding to align to 64-byte boundary if needed
	padding := (64 - (nextSectionOffset % 64)) % 64
	nextSectionOffset += padding
	binary.BigEndian.PutUint64(sectionHeaderBuf[16:24], uint64(nextSectionOffset))

	// Debug: Print the section header
	fmt.Printf("Created section header (64 bytes):\n")
	printHexDump(sectionHeaderBuf, 0, 64, 0, 2, 8, 16, 24)

	// Write the section header to the output file
	_, err = scState.OutFile.Write(sectionHeaderBuf)
	if err != nil {
		return fmt.Errorf("failed to write section 1_1 header to output file: %w", err)
	}
	currentPos += 64

	// For the header section, we'll use our own section data from the temporary file
	// This ensures we're using our reconstructed data rather than the original
	// Reset the temporary file position to the beginning
	_, err = headerSection.TmpFile.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek to beginning of temporary file for section 1_1: %w", err)
	}

	// Read the section data from the temporary file
	section1Data := make([]byte, section1Size)
	_, err = io.ReadFull(headerSection.TmpFile, section1Data)
	if err != nil {
		return fmt.Errorf("failed to read section 1_1 data from temporary file: %w", err)
	}

	fmt.Printf("Read %d bytes of section 1_1 data from temporary file\n", len(section1Data))

	_, err = scState.OutFile.Write(section1Data)
	if err != nil {
		return fmt.Errorf("failed to write section 1_1 data to output file: %w", err)
	}

	currentPos += int64(section1Size)
	fmt.Printf("Copied section 1_1 from first input file, size: %d bytes\n", section1Size)

	// Update the section's offset in the Sections list if it exists in our sections
	if headerSection := scState.SectionFiles.Get("1_1"); headerSection != nil {
		scState.SectionFiles.UpdateOffset("1_1", 64) // 64 is the offset where section 1_1 starts
	}

	// Add padding after the header section if needed to align to 64-byte boundary
	if currentPos%64 != 0 {
		paddingSize := 64 - (currentPos % 64)
		padding := make([]byte, paddingSize)

		// Write padding
		_, err = scState.OutFile.Write(padding)
		if err != nil {
			return fmt.Errorf("failed to write alignment padding: %w", err)
		}

		currentPos += int64(paddingSize)
		fmt.Printf("Added %d bytes of padding to align to 64-byte boundary\n", paddingSize)
	}

	// Step 3: Process remaining sections
	for _, section := range sections {
		// Skip the header section (already processed)
		if section.Type == "1_1" {
			continue
		}
		
		// Special handling for BPT section (section 11) - use snapshot reader like we did for header
		if strings.HasPrefix(section.Type, "11_") {
			fmt.Printf("Using snapshot reader for BPT section %s\n", section.Type)
			
			// Find the BPT section in the snapshot
			var bptSectionReader io.Reader
			var bptSectionSize int64
			for i, s := range snapshotReader.Sections {
				if s.Type() == snapshot.SectionTypeBPT {
					// Open the BPT section
					reader, err := snapshotReader.Open(snapshot.SectionTypeBPT)
					if err != nil {
						return fmt.Errorf("failed to open BPT section: %w", err)
					}
					bptSectionReader = reader
					bptSectionSize = s.Size()
					fmt.Printf("Found BPT section at index %d, size: %d bytes\n", i, bptSectionSize)
					break
				}
			}
			
			if bptSectionReader == nil {
				return fmt.Errorf("BPT section not found in snapshot")
			}
			
			// Get the size of the BPT section from our temporary file
			tmpFileInfo, err := section.TmpFile.Stat()
			if err != nil {
				return fmt.Errorf("failed to get section %s file info: %w", section.Type, err)
			}
			tmpFileSize := tmpFileInfo.Size()
			
			// Use the exact size from the original BPT section
			if tmpFileSize != bptSectionSize {
				fmt.Printf("Warning: BPT section size mismatch. Using original size: %d bytes, Temporary file size: %d bytes\n", 
					bptSectionSize, tmpFileSize)
				tmpFileSize = bptSectionSize
			}
			
			// Prepare to write the section header
			sectionType := uint16(snapshot.SectionTypeBPT)
			fmt.Printf("Writing BPT section (type %d)\n", sectionType)
			
			// Create a new section header (64 bytes)
			sectionHeaderBuf := make([]byte, 64)
			
			// Set the section type (2 bytes, big-endian)
			binary.BigEndian.PutUint16(sectionHeaderBuf[0:2], sectionType)
			
			// Set the section size (8 bytes, big-endian)
			binary.BigEndian.PutUint64(sectionHeaderBuf[8:16], uint64(tmpFileSize))
			
			// Calculate and set the next section offset (8 bytes, big-endian) - offset 16
			nextSectionOffset := currentPos + 64 + tmpFileSize
			if nextSectionOffset%64 != 0 {
				padding := 64 - (nextSectionOffset % 64)
				nextSectionOffset += padding
			}
			binary.BigEndian.PutUint64(sectionHeaderBuf[16:24], uint64(nextSectionOffset))
			
			// Debug: Print the section header
			fmt.Printf("Created BPT section header (64 bytes):\n")
			printHexDump(sectionHeaderBuf, 0, 64, 0, 2, 8, 16, 24)
			
			// Write the section header to the output file
			_, err = scState.OutFile.Write(sectionHeaderBuf)
			if err != nil {
				return fmt.Errorf("failed to write BPT section header: %w", err)
			}
			
			// Update current position
			currentPos += 64 // Section header is 64 bytes
			
			// Seek to the beginning of the temporary file
			_, err = section.TmpFile.Seek(0, io.SeekStart)
			if err != nil {
				return fmt.Errorf("failed to seek to beginning of temporary file for %s: %w", section.Type, err)
			}
			
			// Read the section data from the temporary file
			sectionData := make([]byte, tmpFileSize)
			_, err = io.ReadFull(section.TmpFile, sectionData)
			if err != nil {
				return fmt.Errorf("failed to read section data for %s: %w", section.Type, err)
			}
			
			// Write the section data to the output file
			bytesWritten, err := scState.OutFile.Write(sectionData)
			if err != nil {
				return fmt.Errorf("failed to write section data for %s: %w", section.Type, err)
			}
			
			// Update current position after writing data
			currentPos += int64(bytesWritten)
			
			// Check if the correct number of bytes was written
			if int64(bytesWritten) != tmpFileSize {
				return fmt.Errorf("incorrect number of bytes written for %s: expected %d, got %d",
					section.Type, tmpFileSize, bytesWritten)
			}
			
			fmt.Printf("Successfully wrote %d bytes of data for BPT section %s\n", bytesWritten, section.Type)
			
			// Add padding to align to the next 64-byte boundary if needed
			if currentPos%64 != 0 {
				paddingSize := 64 - (currentPos % 64)
				padding := make([]byte, paddingSize)
				
				// Write padding
				_, err = scState.OutFile.Write(padding)
				if err != nil {
					return fmt.Errorf("failed to write alignment padding after BPT section %s: %w", section.Type, err)
				}
				
				currentPos += int64(paddingSize)
				fmt.Printf("Added %d bytes of padding after BPT section %s to align to 64-byte boundary\n",
					paddingSize, section.Type)
			}
			
			// Skip the normal section processing
			continue
		}

		// Ensure we're at a 64-byte boundary for each section
		if currentPos%64 != 0 {
			paddingSize := 64 - (currentPos % 64)
			padding := make([]byte, paddingSize)

			// Write padding
			_, err = scState.OutFile.Write(padding)
			if err != nil {
				return fmt.Errorf("failed to write alignment padding: %w", err)
			}

			currentPos += int64(paddingSize)
			fmt.Printf("Added %d bytes of padding to align to 64-byte boundary\n", paddingSize)
		}

		// Get section type as integer using the snapshot package's section types
		sectionType, err := parseSectionType(section.Type)
		if err != nil {
			return fmt.Errorf("failed to parse section type %s: %w", section.Type, err)
		}

		// Get the size of the section data
		tmpFileInfo, err := section.TmpFile.Stat()
		if err != nil {
			return fmt.Errorf("failed to get section file info for %s: %w", section.Type, err)
		}
		tmpFileSize := tmpFileInfo.Size()

		// Validate section size
		if tmpFileSize <= 0 {
			return fmt.Errorf("invalid section size for %s: %d bytes", section.Type, tmpFileSize)
		}

		// Sanity check on section size - if it's unreasonably large, something is wrong
		if tmpFileSize > 100*1024*1024 { // 100 MB limit as a reasonable maximum
			return fmt.Errorf("section %s size is unreasonably large (%d bytes), possible format error",
				section.Type, tmpFileSize)
		}

		// Prepare section header (64 bytes)
		sectionHeader := make([]byte, 64)

		// Set the section type at bytes 0-1 (2-byte big-endian)
		binary.BigEndian.PutUint16(sectionHeader[0:2], uint16(sectionType))

		// Bytes 2-7 are reserved (leave as zeros)

		// Set the section size at bytes 8-15 (8-byte big-endian)
		binary.BigEndian.PutUint64(sectionHeader[8:16], uint64(tmpFileSize))

		// Calculate next section offset (current position + header size + data size + padding)
		padding := (64 - ((currentPos + 64 + tmpFileSize) % 64)) % 64
		nextOffset := currentPos + 64 + tmpFileSize + padding

		// Set the next section offset at bytes 16-23 (8-byte big-endian)
		binary.BigEndian.PutUint64(sectionHeader[16:24], uint64(nextOffset))

		// Bytes 24-63 are additional metadata (leave as zeros for now)

		// Store the current position as the section's offset
		sectionOffset := currentPos
		scState.SectionFiles.UpdateOffset(section.Type, sectionOffset)

		// Write the section header (64 bytes)
		_, err = scState.OutFile.Write(sectionHeader)
		if err != nil {
			return fmt.Errorf("failed to write section header for %s: %w", section.Type, err)
		}

		currentPos += 64 // Section header is 64 bytes
		fmt.Printf("Writing section %s, type: %d, size: %d bytes at offset %d\n",
			section.Type, sectionType, tmpFileSize, sectionOffset)

		// Debug: Print the section header
		fmt.Printf("Section %s header (64 bytes):\n", section.Type)
		printHexDump(sectionHeader, 0, 64, 0, 2, 8, 16, 24)

		// Reset the temporary file position to the beginning
		_, err = section.TmpFile.Seek(0, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to seek to beginning of temporary file for %s: %w", section.Type, err)
		}

		// Read the section data from the temporary file
		sectionData := make([]byte, tmpFileSize)
		_, err = io.ReadFull(section.TmpFile, sectionData)
		if err != nil {
			return fmt.Errorf("failed to read section data for %s: %w", section.Type, err)
		}

		// Debug: Print the first 64 bytes of the section data (or less if smaller)
		debugSize := int64(64)
		if tmpFileSize < debugSize {
			debugSize = tmpFileSize
		}
		fmt.Printf("First %d bytes of section %s data:\n", debugSize, section.Type)
		printHexDump(sectionData[:debugSize], 0, int(debugSize), 0, 0, 0, 0, 0)

		// Write the section data to the output file
		bytesWritten, err := scState.OutFile.Write(sectionData)
		if err != nil {
			return fmt.Errorf("failed to write section data for %s: %w", section.Type, err)
		}

		// Update current position after writing data
		currentPos += int64(bytesWritten)

		// Check if the correct number of bytes was written
		if int64(bytesWritten) != tmpFileSize {
			return fmt.Errorf("incorrect number of bytes written for %s: expected %d, got %d",
				section.Type, tmpFileSize, bytesWritten)
		}

		fmt.Printf("Successfully wrote %d bytes of data for section %s\n", bytesWritten, section.Type)

		// Add padding to align to the next 64-byte boundary if needed
		if currentPos%64 != 0 {
			paddingSize := 64 - (currentPos % 64)
			padding := make([]byte, paddingSize)

			// Write padding
			_, err = scState.OutFile.Write(padding)
			if err != nil {
				return fmt.Errorf("failed to write alignment padding after section %s: %w", section.Type, err)
			}

			currentPos += int64(paddingSize)
			fmt.Printf("Added %d bytes of padding after section %s to align to 64-byte boundary\n",
				paddingSize, section.Type)
		}
	}

	fmt.Printf("All sections written successfully\n")

	// Print a summary of the reconstructed snapshot
	printSnapshotSummary(scState.OutFile, currentPos)

	return nil
}

// printSnapshotSummary prints a summary of the reconstructed snapshot
func printSnapshotSummary(file *os.File, totalSize int64) {
	fmt.Printf("\n=== Snapshot Reconstruction Summary ===\n")
	fmt.Printf("Total snapshot size: %d bytes (%.2f MB)\n", totalSize, float64(totalSize)/1024/1024)

	// Seek to the beginning of the file to read the header
	_, err := file.Seek(0, io.SeekStart)
	if err != nil {
		fmt.Printf("Error seeking to beginning of file: %v\n", err)
		return
	}

	// Try to open the snapshot using the snapshot package
	snapshotReader, err := snapshot.Open(file)
	if err != nil {
		fmt.Printf("Error opening snapshot: %v\n", err)
		return
	}

	// Print header information
	fmt.Printf("Snapshot version: %d\n", snapshotReader.Header.Version)
	fmt.Printf("Root hash: %x\n", snapshotReader.Header.RootHash)

	// Print section information
	fmt.Printf("Number of sections: %d\n", len(snapshotReader.Sections))
	for i, section := range snapshotReader.Sections {
		fmt.Printf("  Section %d: Type=%d, Size=%d bytes, Offset=0x%x\n",
			i, section.Type(), section.Size(), section.Offset())
	}

	fmt.Printf("=== End of Summary ===\n")
}

// alignToBlockBoundary aligns a position to the next 64-byte boundary
func alignToBlockBoundary(pos int64) int64 {
	if pos%64 == 0 {
		return pos
	}
	return ((pos / 64) + 1) * 64
}

// parseSectionType converts a section type string (e.g., "1_1") to a snapshot section type
func parseSectionType(sectionType string) (int, error) {
	parts := strings.Split(sectionType, "_")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid section type format: %s", sectionType)
	}

	// Parse the first part as the section type
	typeNum, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("failed to parse section type: %w", err)
	}

	// Map the section type to the snapshot package's section types
	switch typeNum {
	case 1:
		return int(snapshot.SectionTypeHeader), nil
	case 2:
		return int(snapshot.SectionTypeBPT), nil
	case 3:
		return int(snapshot.SectionTypeRecords), nil
	default:
		// For unknown types, return the parsed type as-is
		return typeNum, nil
	}
}
