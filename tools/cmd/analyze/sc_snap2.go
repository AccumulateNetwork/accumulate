// Copyright 2024 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Define section header structure - this matches the binary format in the snapshot file
type sectionHeader struct {
	Type       uint16
	Reserved1  [6]byte
	Size       uint64
	NextOffset uint64
	Reserved2  [40]byte
}

// sectionInfo represents a section in the snapshot
type sectionInfo struct {
	index    int    // Section index for ordering
	type_    uint16 // Section type
	subtype  string // Subtype for section 7 ("account" or "other")
	size     int64  // Size of section data
	filePath string // Path to the section data file
}

// processSection analyzes a section file and returns section information
func processSection(tmpDir string, file os.DirEntry, index int) (sectionInfo, error) {
	// Initialize section info
	info := sectionInfo{
		index:    index,
		filePath: filepath.Join(tmpDir, file.Name()),
	}

	// Get file size
	fileInfo, err := os.Stat(info.filePath)
	if err != nil {
		return sectionInfo{}, fmt.Errorf("failed to stat file %s: %w", info.filePath, err)
	}
	info.size = fileInfo.Size()

	// Determine section type and subtype from filename
	if strings.Contains(file.Name(), "accounts") {
		// Only matters for section 7 for accounts; ignored otherwise
		info.subtype = "account" // Only specific option is account
	} else {
		info.subtype = "other" // Anything else whatsoever is other
	}

	// Extract type from filename using the Order_xx_Section_Type_y pattern
	var typeVal uint64
	
	// Pattern: Order_xx_Section_Type_y.bin or Order_xx_Section_Type_y_account.bin or Order_xx_Section_Type_y_other.bin
	typeRegex := regexp.MustCompile(`Order_\d+_Section_Type_(\d+)`)
	typeMatches := typeRegex.FindStringSubmatch(file.Name())
	if len(typeMatches) >= 2 {
		typeVal, err = strconv.ParseUint(typeMatches[1], 10, 16)
		if err != nil {
			return sectionInfo{}, fmt.Errorf("invalid section type in filename %s: %w", file.Name(), err)
		}
		info.type_ = uint16(typeVal)
		return info, nil
	}
	
	// If we get here, we couldn't determine the type
	return sectionInfo{}, fmt.Errorf("could not determine section type from filename: %s", file.Name())
}

// reconstructSnapshot reconstructs a snapshot file from temporary files
// NOTE: Temp files must be processed in order. This function does NOT use maps for ordered data.
func reconstructSnapshot(tmpDir string, outputPath string) error {
	// Read all files in the temporary directory
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		return fmt.Errorf("failed to read temporary directory: %w", err)
	}

	// Find the header file and collect section files
	var headerFile os.DirEntry
	var sectionFiles []os.DirEntry

	for _, file := range files {
		if file.Name() == "Order_00_Section_Type_1.bin" {
			// This is the header file (Type 1 = Header section)
			headerFile = file
		} else if strings.HasPrefix(file.Name(), "Order_") && file.Name() != "Order_00_Section_Type_1.bin" {
			// Collect files with the Order_xx_Section_Type_y format, excluding the header
			sectionFiles = append(sectionFiles, file)
		}
	}

	if headerFile == nil {
		return fmt.Errorf("header file not found in temporary directory")
	}

	// Sort section files by their section index
	// Extract the index from filenames like "section_1_type_11.bin"
	sort.Slice(sectionFiles, func(i, j int) bool {
		// Extract index from names
		indexI := extractSectionIndex(sectionFiles[i].Name())
		indexJ := extractSectionIndex(sectionFiles[j].Name())
		return indexI < indexJ
	})

	// Process all section files
	var sections []sectionInfo
	for _, file := range sectionFiles {
		// Extract index from filename
		index := extractSectionIndex(file.Name())
		
		// Process this section file
		section, err := processSection(tmpDir, file, index)
		if err != nil {
			return fmt.Errorf("failed to process section file %s: %w", file.Name(), err)
		}

		// Add to our sections list
		sections = append(sections, section)
	}

	// Create the output file
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFile.Close()

	// Process the header file
	headerPath := filepath.Join(tmpDir, headerFile.Name())
	headerData, err := os.ReadFile(headerPath)
	if err != nil {
		return fmt.Errorf("failed to read header file: %w", err)
	}

	// Reserve space for the header section header (64 bytes)
	// This exactly matches how SegmentedWriter.Open works
	_, err = outputFile.Write(make([]byte, 64))
	if err != nil {
		return fmt.Errorf("failed to allocate space for header section header: %w", err)
	}

	// Write the header data
	// In the official code, this is done via writeValue which writes:
	// 1. An 8-byte length prefix
	// 2. The actual data
	
	// Write the length prefix (8 bytes)
	var lengthBytes [8]byte
	binary.BigEndian.PutUint64(lengthBytes[:], uint64(len(headerData)))
	_, err = outputFile.Write(lengthBytes[:])
	if err != nil {
		return fmt.Errorf("failed to write header length prefix: %w", err)
	}
	
	// Write the actual header data
	_, err = outputFile.Write(headerData)
	if err != nil {
		return fmt.Errorf("failed to write header data: %w", err)
	}
	
	// Get current position after writing header data
	currentPos, err := outputFile.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("failed to get current position: %w", err)
	}
	
	// Calculate header size (excluding the header section header)
	headerSize := currentPos - 64 // 64 bytes for header section header
	
	// Go back to write the header section header
	_, err = outputFile.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek to header section header: %w", err)
	}
	
	// Write the header section header
	// This exactly matches how SegmentedWriter.closeSegment works
	var headerSectionHeaderBytes [64]byte
	binary.BigEndian.PutUint16(headerSectionHeaderBytes[0:], 1) // Type 1 = Header section
	binary.BigEndian.PutUint64(headerSectionHeaderBytes[8:], uint64(headerSize)) // Size
	
	// Calculate the offset to the next section (after padding)
	// This is critical - the original snapshot stores the next section offset at bytes 16-23
	nextSectionOffset := currentPos
	if nextSectionOffset%64 > 0 {
		// Add padding to align to 64-byte boundary
		nextSectionOffset += (64 - (nextSectionOffset % 64))
	}
	
	// Set the next section offset in the header
	binary.BigEndian.PutUint64(headerSectionHeaderBytes[16:], uint64(nextSectionOffset))
	
	_, err = outputFile.Write(headerSectionHeaderBytes[:])
	if err != nil {
		return fmt.Errorf("failed to write header section header: %w", err)
	}
	
	// Return to the end of the header data
	_, err = outputFile.Seek(currentPos, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek back to end of header: %w", err)
	}
	
	// Pad to 64-byte alignment
	if currentPos%64 > 0 {
		padding := make([]byte, 64-(currentPos%64))
		_, err = outputFile.Write(padding)
		if err != nil {
			return fmt.Errorf("failed to write padding after header: %w", err)
		}
	}

	// Get the position after padding (this is where the first section will start)
	_, err = outputFile.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("failed to seek to first section offset: %w", err)
	}

	// Now write each section in order
	for i, section := range sections {
		// Get current position (start of this section)
		sectionStart, err := outputFile.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("failed to get current position: %w", err)
		}
		
		// This matches exactly how SegmentedWriter.Open works:
		// 1. Reserve 64 bytes for the section header
		// 2. Write the section data
		// 3. Go back and update the header
		
		// Reserve space for the section header (64 bytes)
		_, err = outputFile.Write(make([]byte, 64))
		if err != nil {
			return fmt.Errorf("failed to allocate space for section header: %w", err)
		}
		
		// Write the section data
		err = writeSectionDataFromFile(section.filePath, outputFile)
		if err != nil {
			return fmt.Errorf("failed to write section data: %w", err)
		}
		
		// Get current position after writing section data
		currentPos, err := outputFile.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("failed to get current position: %w", err)
		}
		
		// Go back to write the section header
		_, err = outputFile.Seek(sectionStart, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to seek to section header: %w", err)
		}
		
		// Write the section header
		// This exactly matches how SegmentedWriter.closeSegment works
		var sectionHeaderBytes [64]byte
		binary.BigEndian.PutUint16(sectionHeaderBytes[0:], uint16(section.type_)) // Type
		binary.BigEndian.PutUint64(sectionHeaderBytes[8:], uint64(currentPos - sectionStart - 64)) // Size
		
		// Calculate the next section offset (0 if this is the last section)
		if i < len(sections)-1 {
			// If there's a next section, we need to update the NextOffset field
			// The next section will be after padding the current section to 64-byte alignment
			nextOffset := currentPos
			if nextOffset%64 > 0 {
				nextOffset += 64 - (nextOffset % 64)
			}
			binary.BigEndian.PutUint64(sectionHeaderBytes[16:], uint64(nextOffset))
		}
		
		_, err = outputFile.Write(sectionHeaderBytes[:])
		if err != nil {
			return fmt.Errorf("failed to write section header: %w", err)
		}
		
		// Return to the end of the section data
		_, err = outputFile.Seek(currentPos, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to seek back to end of section: %w", err)
		}

		// Align to 64-byte boundary for the next section
		if err := alignTo64(outputFile); err != nil {
			return fmt.Errorf("failed to align after section %d: %w", i, err)
		}
	}

	return nil
}

// Note: The previous writeSectionData and writeType7SectionData functions have been replaced
// by the more generic writeSectionDataFromFile function that handles all section types

// compareSnapshots compares two snapshot files to check if they are identical
func compareSnapshots(originalPath, reconstructedPath string) (bool, error) {
	// Get file sizes
	originalInfo, err := os.Stat(originalPath)
	if err != nil {
		return false, fmt.Errorf("failed to stat original file: %w", err)
	}
	reconstructedInfo, err := os.Stat(reconstructedPath)
	if err != nil {
		return false, fmt.Errorf("failed to stat reconstructed file: %w", err)
	}

	// Open the original file
	originalFile, err := os.Open(originalPath)
	if err != nil {
		return false, fmt.Errorf("failed to open original file: %w", err)
	}
	defer originalFile.Close()

	// Open the reconstructed file
	reconstructedFile, err := os.Open(reconstructedPath)
	if err != nil {
		return false, fmt.Errorf("failed to open reconstructed file: %w", err)
	}
	defer reconstructedFile.Close()

	// Compare file sizes
	if originalInfo.Size() != reconstructedInfo.Size() {
		fmt.Printf("File sizes differ: original=%d bytes, reconstructed=%d bytes (diff=%d bytes)\n",
			originalInfo.Size(), reconstructedInfo.Size(),
			abs(originalInfo.Size()-reconstructedInfo.Size()))
		
		// Show a hex dump of the first few bytes of each file for comparison
		originalBytes := make([]byte, min(256, int(originalInfo.Size())))
		reconstructedBytes := make([]byte, min(256, int(reconstructedInfo.Size())))
		
		// Reset file positions
		_, err = originalFile.Seek(0, io.SeekStart)
		if err != nil {
			return false, fmt.Errorf("failed to seek in original file: %w", err)
		}
		_, err = reconstructedFile.Seek(0, io.SeekStart)
		if err != nil {
			return false, fmt.Errorf("failed to seek in reconstructed file: %w", err)
		}
		
		// Read the bytes
		_, err = io.ReadFull(originalFile, originalBytes)
		if err != nil && err != io.ErrUnexpectedEOF {
			return false, fmt.Errorf("failed to read from original file: %w", err)
		}
		_, err = io.ReadFull(reconstructedFile, reconstructedBytes)
		if err != nil && err != io.ErrUnexpectedEOF {
			return false, fmt.Errorf("failed to read from reconstructed file: %w", err)
		}
		
		// Print hex dumps
		fmt.Printf("Original file first bytes:\n")
		printHexDump(originalBytes, 0, -1) // -1 means no specific diff position to highlight
		
		fmt.Printf("Reconstructed file first bytes:\n")
		printHexDump(reconstructedBytes, 0, -1)
		
		// Return false to indicate files are different
		return false, nil
	}

	// Find actual section headers in the reconstructed file
	// We'll scan the file for section headers by looking for valid section types
	var sectionOffsets []int

	// Add the first section header at offset 0
	sectionOffsets = append(sectionOffsets, 0)

	// Scan for section headers by following the next section offset pointers
	currentOffset := int64(0)
	for {
		// Read the section header
		header := make([]byte, 64)
		_, err = reconstructedFile.Seek(currentOffset, io.SeekStart)
		if err != nil {
			break
		}

		n, err := reconstructedFile.Read(header)
		if err != nil || n < 64 {
			break
		}

		// Extract the next section offset from the header
		nextOffset := uint64(0)
		for i := 0; i < 8; i++ {
			nextOffset = (nextOffset << 8) | uint64(header[23-i])
		}

		// Add this section header to our list
		sectionOffsets = append(sectionOffsets, int(currentOffset))

		// If next offset is 0, we've reached the end
		if nextOffset == 0 {
			break
		}

		// Move to the next section
		currentOffset = int64(nextOffset)
		if currentOffset >= reconstructedInfo.Size() {
			break
		}
	}

	// Reset file position
	_, err = reconstructedFile.Seek(0, io.SeekStart)
	if err != nil {
		return false, fmt.Errorf("failed to reset file position: %w", err)
	}

	// Compare the files byte by byte
	const bufferSize = 64 * 1024 // 64KB buffer
	originalBuffer := make([]byte, bufferSize)
	reconstructedBuffer := make([]byte, bufferSize)

	offset := int64(0)
	for {
		originalN, originalErr := originalFile.Read(originalBuffer)
		reconstructedN, reconstructedErr := reconstructedFile.Read(reconstructedBuffer)

		// Check if the number of bytes read is different
		if originalN != reconstructedN {
			fmt.Printf("Different number of bytes read at offset 0x%x: original=%d, reconstructed=%d\n",
				offset, originalN, reconstructedN)
			return false, nil
		}

		// Check if the content is different
		for i := 0; i < originalN; i++ {
			if originalBuffer[i] != reconstructedBuffer[i] {
				fmt.Printf("First difference at offset 0x%x: original=0x%02x, reconstructed=0x%02x\n",
					offset+int64(i), originalBuffer[i], reconstructedBuffer[i])

				// Show a larger context around the difference (256 bytes)
				const contextSize = 256

				// Calculate start and end positions for the context
				// Handle edge cases properly
				startPos := offset + int64(i) - contextSize/2
				if startPos < 0 {
					startPos = 0
				}

				endPos := startPos + contextSize
				if endPos > originalInfo.Size() {
					endPos = originalInfo.Size()
					// Adjust startPos if we're near the end of the file
					if endPos-startPos < contextSize && startPos > 0 {
						startPos = endPos - contextSize
						if startPos < 0 {
							startPos = 0
						}
					}
				}

				// Read the context bytes from both files
				originalContext := make([]byte, endPos-startPos)
				reconstructedContext := make([]byte, endPos-startPos)

				// Seek to the start position in both files
				_, err = originalFile.Seek(startPos, io.SeekStart)
				if err != nil {
					return false, fmt.Errorf("failed to seek in original file: %w", err)
				}
				_, err = reconstructedFile.Seek(startPos, io.SeekStart)
				if err != nil {
					return false, fmt.Errorf("failed to seek in reconstructed file: %w", err)
				}

				// Read the context bytes
				_, err = io.ReadFull(originalFile, originalContext)
				if err != nil && err != io.ErrUnexpectedEOF {
					return false, fmt.Errorf("failed to read context from original file: %w", err)
				}
				_, err = io.ReadFull(reconstructedFile, reconstructedContext)
				if err != nil && err != io.ErrUnexpectedEOF {
					return false, fmt.Errorf("failed to read context from reconstructed file: %w", err)
				}

				// Calculate relevant section offsets for this region
				var relevantOffsets []int
				for _, secOffset := range sectionOffsets {
					relOffset := secOffset - int(startPos)
					if relOffset >= 0 && relOffset < len(originalContext) {
						relevantOffsets = append(relevantOffsets, relOffset)
					}
				}

				// Calculate the position of the differing byte within our context
				diffPos := int(offset + int64(i) - startPos)

				// Print the context with the difference highlighted
				fmt.Printf("Original bytes around difference:\n")
				printHexDump(originalContext, int(startPos), diffPos, relevantOffsets...)
				fmt.Printf("Total: %d bytes\n", len(originalContext))

				fmt.Printf("Reconstructed bytes around difference:\n")
				printHexDump(reconstructedContext, int(startPos), diffPos, relevantOffsets...)

				// Reset file positions to continue from where we left off
				_, err = originalFile.Seek(offset+int64(originalN), io.SeekStart)
				if err != nil {
					return false, fmt.Errorf("failed to reset position in original file: %w", err)
				}
				_, err = reconstructedFile.Seek(offset+int64(reconstructedN), io.SeekStart)
				if err != nil {
					return false, fmt.Errorf("failed to reset position in reconstructed file: %w", err)
				}

				return false, nil
			}
		}

		// Update offset
		offset += int64(originalN)

		// Check for end of file
		if originalErr == io.EOF && reconstructedErr == io.EOF {
			break
		}

		// Check for other errors
		if originalErr != nil && originalErr != io.EOF {
			return false, fmt.Errorf("error reading original file: %w", originalErr)
		}
		if reconstructedErr != nil && reconstructedErr != io.EOF {
			return false, fmt.Errorf("error reading reconstructed file: %w", reconstructedErr)
		}
	}

	return true, nil
}

// Helper functions for min and max
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

// extractSectionIndex extracts the section index from a filename
// Example: "Order_01_Section_Type_11.bin" -> 1
func extractSectionIndex(filename string) int {
	// Use regex to extract the index from Order_xx_Section_Type_y format
	regex := regexp.MustCompile(`Order_([0-9]+)_`)
	matches := regex.FindStringSubmatch(filename)
	
	if len(matches) < 2 {
		// If no match found, return a high number to sort it last
		return 9999
	}
	
	index, err := strconv.Atoi(matches[1])
	if err != nil {
		// If conversion fails, return a high number
		return 9999
	}
	
	return index
}

// alignTo64 ensures the file position is aligned to a 64-byte boundary
func alignTo64(file *os.File) error {
	// Get current position
	pos, err := file.Seek(0, io.SeekCurrent)
	if err != nil {
		return fmt.Errorf("failed to get current position: %w", err)
	}

	// Calculate padding needed
	const alignment = 64
	if remainder := pos % alignment; remainder > 0 {
		padding := alignment - remainder
		// Write padding bytes
		padBytes := make([]byte, padding)
		_, err = file.Write(padBytes)
		if err != nil {
			return fmt.Errorf("failed to write padding bytes: %w", err)
		}
	}

	return nil
}

// writeSectionHeaderStruct writes a section header structure to the file
func writeSectionHeaderStruct(file *os.File, header sectionHeader) error {
	// Create a 64-byte header buffer
	var headerBytes [64]byte

	// Write section type (first 2 bytes)
	binary.BigEndian.PutUint16(headerBytes[0:2], header.Type)

	// Write section size (bytes 8-15)
	binary.BigEndian.PutUint64(headerBytes[8:16], header.Size)

	// Write next section offset (bytes 16-23)
	binary.BigEndian.PutUint64(headerBytes[16:24], header.NextOffset)

	// Write the header to the file
	_, err := file.Write(headerBytes[:])
	if err != nil {
		return fmt.Errorf("failed to write section header: %w", err)
	}

	return nil
}

// writeSectionDataFromFile reads data from a section file and writes it to the output file
func writeSectionDataFromFile(filePath string, outputFile *os.File) error {
	// Open the section file
	sectionFile, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open section file %s: %w", filePath, err)
	}
	defer sectionFile.Close()

	// Copy the section data to the output file
	_, err = io.Copy(outputFile, sectionFile)
	if err != nil {
		return fmt.Errorf("failed to copy section data: %w", err)
	}

	return nil
}
