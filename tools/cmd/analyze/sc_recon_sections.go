package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"
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

	// Track current position in the output file
	var currentPos int64 = 0

	// Step 1: Copy the 64-byte snapshot header from the first input file
	// Seek to the beginning of the first snapshot file
	_, err := firstSnapshotFile.Seek(0, io.SeekStart)
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

	// Step 2: Copy section 1_1 (header section) from the first input file
	// Read the section header (16 bytes)
	sectionHeaderBuf := make([]byte, 16)
	_, err = io.ReadFull(firstSnapshotFile, sectionHeaderBuf)
	if err != nil {
		return fmt.Errorf("failed to read section 1_1 header from first snapshot file: %w", err)
	}

	// Extract the section size from the header (bytes 8-15)
	section1Size := binary.BigEndian.Uint64(sectionHeaderBuf[8:16])
	fmt.Printf("Section 1_1 size from first snapshot: %d bytes\n", section1Size)

	// Write the section header to the output file
	_, err = scState.OutFile.Write(sectionHeaderBuf)
	if err != nil {
		return fmt.Errorf("failed to write section 1_1 header to output file: %w", err)
	}
	currentPos += 16

	// Read and write the section 1_1 data
	section1Data := make([]byte, section1Size)
	_, err = io.ReadFull(firstSnapshotFile, section1Data)
	if err != nil {
		return fmt.Errorf("failed to read section 1_1 data from first snapshot file: %w", err)
	}

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

	// Process each section except 1_1 which we've already handled
	for _, section := range sections {
		// Skip section 1_1 as we've already written it from the first snapshot file
		if section.Type == "1_1" {
			continue
		}

		// For all other sections, ensure we're at a 64-byte boundary
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

		// Get section type as integer
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

		// Prepare section header (16 bytes)
		sectionHeader := make([]byte, 16)

		// Set the section type at bytes 0-1
		binary.BigEndian.PutUint16(sectionHeader[0:2], uint16(sectionType))

		// Set the section size at bytes 8-15
		binary.BigEndian.PutUint64(sectionHeader[8:16], uint64(tmpFileSize))

		// Store the current position as the section's offset
		sectionOffset := currentPos
		scState.SectionFiles.UpdateOffset(section.Type, sectionOffset)

		// Write the section header
		_, err = scState.OutFile.Write(sectionHeader)
		if err != nil {
			return fmt.Errorf("failed to write section header for %s: %w", section.Type, err)
		}

		currentPos += 16
		fmt.Printf("Writing section %s, size: %d bytes at offset %d\n", section.Type, tmpFileSize, sectionOffset)

		// Reset the temporary file position to the beginning
		_, err = section.TmpFile.Seek(0, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to seek to beginning of temporary file for %s: %w", section.Type, err)
		}

		// Copy the section data from the temporary file to the output file
		bytesWritten, err := io.Copy(scState.OutFile, section.TmpFile)
		if err != nil {
			return fmt.Errorf("failed to copy section data for %s: %w", section.Type, err)
		}

		// Update current position after writing data
		currentPos += bytesWritten

		// Check if the correct number of bytes was written
		if bytesWritten != tmpFileSize {
			return fmt.Errorf("incorrect number of bytes written for %s: expected %d, got %d",
				section.Type, tmpFileSize, bytesWritten)
		}
	}

	fmt.Printf("All sections written successfully\n")
	return nil
}

// alignToBlockBoundary aligns a position to the next 64-byte boundary
func alignToBlockBoundary(pos int64) int64 {
	if pos%64 == 0 {
		return pos
	}
	return ((pos / 64) + 1) * 64
}

// parseSectionType extracts the section type number from a section type string (e.g., "1_1" -> 1)
func parseSectionType(sectionTypeString string) (int, error) {
	parts := strings.Split(sectionTypeString, "_")
	if len(parts) < 1 {
		return 0, fmt.Errorf("invalid section type format: %s", sectionTypeString)
	}

	sectionType, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid section type number: %w", err)
	}

	return sectionType, nil
}
