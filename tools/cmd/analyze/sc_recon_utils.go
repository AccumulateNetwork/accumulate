package main

import (
	"fmt"
	"os"
)

// sc_getOrCreateSectionFile is implemented in sc_parse.go

// sc_writeSnapshotHeader writes the snapshot format version header to the file
func sc_writeSnapshotHeader(file *os.File, formatVersion uint32) error {
	// Write the magic number "SNAP"
	_, err := file.Write([]byte("SNAP"))
	if err != nil {
		return fmt.Errorf("failed to write magic number: %w", err)
	}

	// Write the format version (4 bytes, little-endian)
	versionBytes := make([]byte, 4)
	versionBytes[0] = byte(formatVersion)
	versionBytes[1] = byte(formatVersion >> 8)
	versionBytes[2] = byte(formatVersion >> 16)
	versionBytes[3] = byte(formatVersion >> 24)

	_, err = file.Write(versionBytes)
	if err != nil {
		return fmt.Errorf("failed to write format version: %w", err)
	}

	return nil
}

// sc_writeSectionFromFile writes a section from a temporary file to the destination file
func sc_writeSectionFromFile(destFile *os.File, sourceFile *os.File, key string) error {
	// Get the file size
	fileInfo, err := sourceFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	sectionSize := fileInfo.Size()
	if sectionSize == 0 {
		// Skip empty sections
		return nil
	}

	// Parse the section type and instance from the key
	var sectionType uint32
	var instance int
	_, err = fmt.Sscanf(key, "%d_%d", &sectionType, &instance)
	if err != nil {
		return fmt.Errorf("failed to parse section key %s: %w", key, err)
	}

	// Write the section header
	// Type (4 bytes, little-endian)
	typeBytes := make([]byte, 4)
	typeBytes[0] = byte(sectionType)
	typeBytes[1] = byte(sectionType >> 8)
	typeBytes[2] = byte(sectionType >> 16)
	typeBytes[3] = byte(sectionType >> 24)

	_, err = destFile.Write(typeBytes)
	if err != nil {
		return fmt.Errorf("failed to write section type: %w", err)
	}

	// Size (8 bytes, little-endian)
	sizeBytes := make([]byte, 8)
	size := uint64(sectionSize)
	sizeBytes[0] = byte(size)
	sizeBytes[1] = byte(size >> 8)
	sizeBytes[2] = byte(size >> 16)
	sizeBytes[3] = byte(size >> 24)
	sizeBytes[4] = byte(size >> 32)
	sizeBytes[5] = byte(size >> 40)
	sizeBytes[6] = byte(size >> 48)
	sizeBytes[7] = byte(size >> 56)

	_, err = destFile.Write(sizeBytes)
	if err != nil {
		return fmt.Errorf("failed to write section size: %w", err)
	}

	// Copy the section data
	buffer := make([]byte, 1024*1024) // 1MB buffer
	remaining := sectionSize

	for remaining > 0 {
		// Read a chunk
		readSize := int64(len(buffer))
		if remaining < readSize {
			readSize = remaining
		}

		n, err := sourceFile.Read(buffer[:readSize])
		if err != nil {
			return fmt.Errorf("failed to read section data: %w", err)
		}

		// Write the chunk
		_, err = destFile.Write(buffer[:n])
		if err != nil {
			return fmt.Errorf("failed to write section data: %w", err)
		}

		remaining -= int64(n)
	}

	return nil
}

// TestCompareHeaderEncoding compares different approaches to encoding the header section (section type 1)
// This test helps identify differences between:
// 1. The original snapshot file header section data
// 2. Our reconstruction approach using encoding.NewWriter
// 3. The debug snap collect approach using snapshot.Header.MarshalBinary
//
// The test also compares:
// - Section headers (64-byte headers)
// - Padding calculations for alignment
// - Next section offset calculations
//
// This test is critical for ensuring our snapshot reconstruction process produces
// a byte-for-byte identical header section compared to the original snapshot format.
// printHexDump prints a hexadecimal representation of data for debugging
// offset is the starting offset for the address column
// diffPos is the position of the differing byte (relative to the data slice)
// indexes is an optional slice of indexes to highlight in the output (section headers)
//
// The diffPos is highlighted in bright red background with white text.
// Section headers (indexes) are highlighted in green.
func printHexDump(data []byte, offset int, diffPos int, indexes ...int) {
	// ANSI color codes
	const (
		diffColor  = "\033[97;41m" // White text on red background for the differing byte
		greenColor = "\033[32m"    // Green for section headers (64-byte ranges)
		resetColor = "\033[0m"     // Reset to default color
	)

	// Create a map to track which indexes should be highlighted and with what color
	highlightMap := make(map[int]string)

	// Mark the differing byte position with bright highlighting
	if diffPos >= 0 && diffPos < len(data) {
		highlightMap[offset+diffPos] = diffColor
	}

	// Mark section headers with green (64 bytes each)
	for _, sectionOffset := range indexes {
		// For each section header, highlight all 64 bytes
		for i := 0; i < 64; i++ {
			byteOffset := sectionOffset + i
			// Only highlight if within our display range and not already highlighted as diff
			if byteOffset >= offset && byteOffset < offset+len(data) && 
			   highlightMap[byteOffset] != diffColor {
				highlightMap[byteOffset] = greenColor
			}
		}
	}

	// Print 16 bytes per line
	for i := 0; i < len(data); i += 16 {
		// Print the offset
		fmt.Printf("%08x  ", offset+i)

		// Print the hex values
		chunk := data[i:]
		if len(chunk) > 16 {
			chunk = chunk[:16]
		}

		// Print hex representation
		for j := 0; j < len(chunk); j++ {
			globalIdx := offset + i + j
			color, hasColor := highlightMap[globalIdx]

			if hasColor {
				fmt.Printf("%s%02x%s ", color, chunk[j], resetColor)
			} else {
				fmt.Printf("%02x ", chunk[j])
			}

			if j == 7 {
				fmt.Print(" ") // Extra space after 8 bytes
			}
		}

		// Pad if less than 16 bytes
		for j := len(chunk); j < 16; j++ {
			fmt.Print("   ")
			if j == 7 {
				fmt.Print(" ") // Extra space after 8 bytes
			}
		}

		// Print ASCII representation
		fmt.Print(" |")
		for j := 0; j < len(chunk); j++ {
			globalIdx := offset + i + j
			color, hasColor := highlightMap[globalIdx]

			if chunk[j] >= 32 && chunk[j] <= 126 { // Printable ASCII
				if hasColor {
					fmt.Printf("%s%c%s", color, chunk[j], resetColor)
				} else {
					fmt.Printf("%c", chunk[j])
				}
			} else { // Non-printable ASCII
				if hasColor {
					fmt.Printf("%s.%s", color, resetColor)
				} else {
					fmt.Print(".")
				}
			}
		}
		fmt.Println("|")
	}

	// Print total size
	fmt.Printf("Total: %d bytes\n", len(data))
}
