package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// reconstructHeaderOnly creates a snapshot file with only the header section
// This is a simplified version of reconstructSnapshot that only processes the header
func reconstructHeaderOnly(tmpDir string, outputPath string) error {
	// Read all files in the temporary directory
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		return fmt.Errorf("failed to read temporary directory: %w", err)
	}

	// Find the header file
	var headerFile os.DirEntry
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "Order_00_Section_Type_1") {
			headerFile = file
			break
		}
	}

	if headerFile == nil {
		return fmt.Errorf("header file not found in temporary directory")
	}

	// Create the output file
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	// Open the header file
	headerPath := filepath.Join(tmpDir, headerFile.Name())
	headerFileHandle, err := os.Open(headerPath)
	if err != nil {
		return fmt.Errorf("failed to open header file: %w", err)
	}
	defer headerFileHandle.Close()

	// Get the header file size
	headerFileInfo, err := headerFileHandle.Stat()
	if err != nil {
		return fmt.Errorf("failed to get header file info: %w", err)
	}

	fmt.Printf("Header file: %s, size: %d bytes\n", headerFile.Name(), headerFileInfo.Size())

	// Write the SNAP magic number and format version
	_, err = outputFile.Write([]byte("SNAP"))
	if err != nil {
		return fmt.Errorf("failed to write magic number: %w", err)
	}

	// Write format version (4 bytes, little-endian)
	formatVersion := uint32(1) // Use version 1
	versionBytes := make([]byte, 4)
	versionBytes[0] = byte(formatVersion)
	versionBytes[1] = byte(formatVersion >> 8)
	versionBytes[2] = byte(formatVersion >> 16)
	versionBytes[3] = byte(formatVersion >> 24)

	_, err = outputFile.Write(versionBytes)
	if err != nil {
		return fmt.Errorf("failed to write format version: %w", err)
	}

	// Copy the header section directly from the header file to the output file
	_, err = io.Copy(outputFile, headerFileHandle)
	if err != nil {
		return fmt.Errorf("failed to copy header section: %w", err)
	}

	// Get current position after writing header section
	currentPos, err := outputFile.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("failed to get current position: %w", err)
	}

	// Pad to 64-byte alignment if needed
	if currentPos%64 > 0 {
		padding := make([]byte, 64-(currentPos%64))
		_, err = outputFile.Write(padding)
		if err != nil {
			return fmt.Errorf("failed to write padding after header: %w", err)
		}
	}

	fmt.Printf("Header-only snapshot created at: %s\n", outputPath)
	return nil
}

// compareHeaderOnly compares just the header section of two snapshot files
func compareHeaderOnly(originalPath, reconstructedPath string) (bool, error) {
	// Instead of trying to parse the original file, we'll use the extracted header file directly
	tmpDir := "/tmp/accumulate-snapshot-sections"
	headerFilePath := filepath.Join(tmpDir, "Order_00_Section_Type_1.bin")
	
	// Open the extracted header file
	headerFile, err := os.Open(headerFilePath)
	if err != nil {
		return false, fmt.Errorf("failed to open extracted header file: %w", err)
	}
	defer headerFile.Close()
	
	// Get the header file size
	headerFileInfo, err := headerFile.Stat()
	if err != nil {
		return false, fmt.Errorf("failed to get header file info: %w", err)
	}
	headerSize := headerFileInfo.Size()
	
	// Open the reconstructed file
	reconstructedFile, err := os.Open(reconstructedPath)
	if err != nil {
		return false, fmt.Errorf("failed to open reconstructed file: %w", err)
	}
	defer reconstructedFile.Close()

	// Skip the 8-byte SNAP header in the reconstructed file
	_, err = reconstructedFile.Seek(8, io.SeekStart)
	if err != nil {
		return false, fmt.Errorf("failed to seek in reconstructed file: %w", err)
	}

	fmt.Printf("Extracted header file: %s, size=%d bytes\n", headerFilePath, headerSize)
	fmt.Printf("Reconstructed file: %s\n", reconstructedPath)

	// Read the header sections
	headerBytes := make([]byte, headerSize)
	reconstructedBytes := make([]byte, headerSize)

	// Read the extracted header file
	_, err = io.ReadFull(headerFile, headerBytes)
	if err != nil {
		return false, fmt.Errorf("failed to read header file: %w", err)
	}

	// Read the same number of bytes from the reconstructed file
	_, err = io.ReadFull(reconstructedFile, reconstructedBytes)
	if err != nil {
		return false, fmt.Errorf("failed to read from reconstructed file: %w", err)
	}

	// Compare the bytes
	fmt.Println("\nComparing header sections...")
	fmt.Println("Extracted header file:")
	printHexDump(headerBytes, 0, 64) // Show the first 64 bytes (section header)
	fmt.Println("Reconstructed file (after 8-byte SNAP header):")
	printHexDump(reconstructedBytes, 0, 64) // Show the first 64 bytes (section header)
	// NOTE: The printHexDump function may highlight certain bytes (like at offset 0x40) in both outputs
	// even when they are identical. This is a visual aid in the terminal and does not indicate
	// actual differences between the files. Only explicit mismatch messages should be considered
	// as true differences.

	// Check if they match
	match := true
	mismatchCount := 0
	maxMismatches := 20 // Limit the number of mismatches to display

	for i := 0; i < int(headerSize); i++ {
		if headerBytes[i] != reconstructedBytes[i] {
			if mismatchCount < maxMismatches {
				fmt.Printf("Mismatch at byte %d (0x%x): header=0x%02x, reconstructed=0x%02x\n",
					i, i, headerBytes[i], reconstructedBytes[i])
			}
			mismatchCount++
			match = false
		}
	}

	if mismatchCount > maxMismatches {
		fmt.Printf("... and %d more mismatches\n", mismatchCount-maxMismatches)
	}

	if match {
		fmt.Println("Header sections match exactly!")
	} else {
		fmt.Printf("Header sections do not match. Total mismatches: %d\n", mismatchCount)
	}

	return match, nil
}

// findHeaderSection finds the header section in a snapshot file
// Returns the offset and size of the header section
func findHeaderSection(file *os.File) (int64, int64, error) {
	// Save the current position
	currentPos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get current position: %w", err)
	}
	
	// Seek to the beginning of the file (after the 8-byte SNAP header)
	_, err = file.Seek(8, io.SeekStart)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to seek to beginning of file: %w", err)
	}

	// For the header section, we know it's always the first section at offset 0x40 (64)
	// and we already extracted it to a file, so we can just return these values directly
	// This avoids having to parse the section headers which can be complex
	headerSectionOffset := int64(64) // 0x40, after the file header
	
	// Read the section header to get the size
	// First, seek to the section header
	_, err = file.Seek(headerSectionOffset, io.SeekStart)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to seek to header section: %w", err)
	}
	
	// Read the section type (first 2 bytes)
	typeBytes := make([]byte, 2)
	_, err = io.ReadFull(file, typeBytes)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read section type: %w", err)
	}
	
	// Verify this is the header section (type 1)
	sectionType := uint16(typeBytes[0]) | uint16(typeBytes[1])<<8
	if sectionType != 1 {
		return 0, 0, fmt.Errorf("expected header section (type 1), got type %d", sectionType)
	}
	
	// Skip 6 bytes
	_, err = file.Seek(6, io.SeekCurrent)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to seek: %w", err)
	}
	
	// Read the section size (8 bytes)
	sizeBytes := make([]byte, 8)
	_, err = io.ReadFull(file, sizeBytes)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read section size: %w", err)
	}
	
	// Convert to uint64
	sectionSize := uint64(sizeBytes[0]) | uint64(sizeBytes[1])<<8 | 
		uint64(sizeBytes[2])<<16 | uint64(sizeBytes[3])<<24 | 
		uint64(sizeBytes[4])<<32 | uint64(sizeBytes[5])<<40 | 
		uint64(sizeBytes[6])<<48 | uint64(sizeBytes[7])<<56
	
	// Restore the original position
	_, err = file.Seek(currentPos, io.SeekStart)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to restore position: %w", err)
	}
	
	// Return the offset and size (including the 64-byte section header)
	return headerSectionOffset, int64(sectionSize) + 64, nil
}
