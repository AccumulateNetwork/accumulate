package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// sc_reconstruct.go implements snapshot reconstruction functionality
// SectionInfo is defined in sc.go

// abs returns the absolute value of an int64
func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// sc_ReconstructSnapshotImpl rebuilds a snapshot file from its parsed components
// This is the implementation of the stub function defined in sc.go
//
// The reconstruction process preserves the exact structure of the original snapshot file,
// including multiple sections of the same type. Multiple sections of the same type are created
// during snapshot collection when a section's data exceeds the maximum size limit. When
// reconstructing, we maintain these separate sections exactly as they appeared in the original
// file, preserving their order and content.
func sc_ReconstructSnapshotImpl(state *sc_State, outputPath string) error {
	fmt.Printf("Reconstructing snapshot to %s\n", outputPath)

	// Create the output directory if it doesn't exist
	outputDir := filepath.Dir(outputPath)
	err := os.MkdirAll(outputDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create output directory: %v", err)
	}

	// Create the output file
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %v", err)
	}
	defer outFile.Close()

	// Write the snapshot header (64 bytes)
	// Copy the header from the original file
	state.File.Seek(0, 0)
	headerBuf := make([]byte, 64)
	_, err = io.ReadFull(state.File, headerBuf)
	if err != nil {
		return fmt.Errorf("failed to read snapshot header: %v", err)
	}
	
	// Update the first section offset in the snapshot header (bytes 16-23)
	// This should match the original file's first section offset
	if state.FirstSectionOffset > 0 {
		binary.BigEndian.PutUint64(headerBuf[16:24], state.FirstSectionOffset)
	}
	
	// Write the updated header
	_, err = outFile.Write(headerBuf)
	if err != nil {
		return fmt.Errorf("failed to write snapshot header: %v", err)
	}

	// Track the current file position
	currentPos := int64(64) // Start after the header
	
	// Get the header data size from the snapshot header (bytes 8-15)
	headerDataSize := binary.BigEndian.Uint64(headerBuf[8:16])
	fmt.Printf("Header data size: %d bytes\n", headerDataSize)
	
	// Write the header data directly from the original file
	// The header data follows the main header in the original file
	// First, seek to the position right after the main header in the original file
	_, err = state.File.Seek(64, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek to header data in original file: %v", err)
	}
	
	// Create a buffer for the header data
	headerDataBuf := make([]byte, headerDataSize)
	
	// Read the header data directly from the original file
	n, err := io.ReadFull(state.File, headerDataBuf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return fmt.Errorf("failed to read header data from original file: %v", err)
	}
	
	// Debug the header data bytes around offset 0xD7
	if headerDataSize > 160 {
		fmt.Printf("Header data bytes around offset 0xD7 (before fix):\n")
		for i := 140; i < 160; i++ {
			fmt.Printf("%02x ", headerDataBuf[i])
			if (i+1) % 8 == 0 {
				fmt.Printf("| ")
			}
		}
		fmt.Printf("\n")
	}

	// Print debug information about the header data
	if headerDataSize > 151 {
		fmt.Printf("Header data bytes around offset 0xD7:\n")
		for i := 140; i < 160; i++ {
			fmt.Printf("%02x ", headerDataBuf[i])
			if (i+1) % 8 == 0 {
				fmt.Printf("| ")
			}
		}
		fmt.Printf("\n")
		
		fmt.Printf("Value at offset 0xD7 (151 in header data): 0x%02x, FirstSectionOffset: %d (0x%x)\n", 
			headerDataBuf[151], state.FirstSectionOffset, state.FirstSectionOffset)
		
		// We preserve the original value from the snapshot file
		// No modifications to headerDataBuf[151]
	}
	
	// Write the header data to the output file
	_, err = outFile.Write(headerDataBuf[:n])
	if err != nil {
		return fmt.Errorf("failed to write header data: %v", err)
	}
	
	currentPos += int64(n)
	fmt.Printf("Wrote header data (%d bytes)\n", n)
	
	// Verify the header data was written correctly by reading it back
	// This is for debugging purposes
	verifyBuf := make([]byte, n)
	_, err = outFile.Seek(64, io.SeekStart) // Seek to after the main header
	if err != nil {
		fmt.Printf("Warning: Failed to seek to verify header data: %v\n", err)
	} else {
		_, err = io.ReadFull(outFile, verifyBuf)
		if err != nil {
			fmt.Printf("Warning: Failed to read back header data: %v\n", err)
		} else if headerDataSize > 151 {
			fmt.Printf("Verification - Value at offset 0xD7 after writing: 0x%02x\n", verifyBuf[151])
			// Simply report the value we read back, without making assumptions about what it should be
			// This ensures we're not artificially expecting a specific value
		}
		// Seek back to the end of the header data
		_, err = outFile.Seek(int64(64+n), io.SeekStart)
		if err != nil {
			fmt.Printf("Warning: Failed to seek back after verification: %v\n", err)
		}
	}
	
	// After writing header data, we need to align to a 64-byte boundary
	// This is exactly what the segmented writer does
	if currentPos % 64 > 0 {
		// Calculate padding size using the segmented writer formula
		paddingSize := 64 - (currentPos % 64)
		fmt.Printf("Adding %d bytes of padding to align to 64-byte boundary after header data\n", paddingSize)
		
		// Create and write padding
		padding := make([]byte, paddingSize)
		_, err = outFile.Write(padding)
		if err != nil {
			return fmt.Errorf("failed to write padding after header data: %v", err)
		}
		currentPos += paddingSize
	}
	
	// Verify that we're now at the expected first section offset
	if state.FirstSectionOffset > 0 && uint64(currentPos) != state.FirstSectionOffset {
		fmt.Printf("WARNING: Current position %d does not match expected first section offset %d\n", 
			currentPos, state.FirstSectionOffset)
		fmt.Printf("This suggests the original file may not have followed the segmented writer algorithm exactly\n")
	}
	
	// We'll track section offsets in the sectionInfos slice

	// Create a slice to store section information for debugging
	sectionInfos := make([]SectionInfo, 0)
	
	// Process all sections in the exact order they appear in the original snapshot
	// This preserves multiple instances of the same section type
	for _, section := range state.OriginalSections {
		sectionType := section.Type
		sectionInstance := section.Instance
		
		// Skip header section (type 1) as we've already handled it separately
		if sectionType == 1 {
			continue
		}
		
		// Print the section info for debugging
		fmt.Printf("Processing section type %d (instance %d)\n", sectionType, sectionInstance)
		// We already have the section from the loop, no need to search for it
		
		// Create a unique key for this section type and instance
		sectionKey := fmt.Sprintf("%d_%d", sectionType, sectionInstance)
		
		tmpFile := state.SectionFiles[sectionKey]
		if tmpFile == nil {
			fmt.Printf("Warning: No temp file for section type %d (instance %d), skipping\n", sectionType, sectionInstance)
			continue // Skip if no temp file for this section type and instance
		}
		
		// CRITICAL: Position the output file exactly at the original section's start offset
		// This ensures we maintain the exact byte layout of the original file
		if currentPos != section.StartOffset {
			// Verify the current file position matches our calculated position before adding padding
			actualPos, err := outFile.Seek(0, io.SeekCurrent)
			if err != nil {
				return fmt.Errorf("failed to get current position before adding padding: %v", err)
			}
			
			if int64(currentPos) != actualPos {
				fmt.Printf("ERROR: Position mismatch before adding padding for section type %d\n", sectionType)
				fmt.Printf("  Calculated position: %d, Actual file position: %d, Difference: %d\n", 
					currentPos, actualPos, int64(currentPos) - actualPos)
				return fmt.Errorf("position mismatch before adding padding")
			}
			
			paddingSize := section.StartOffset - currentPos
			padding := make([]byte, paddingSize)
			// Fill padding with zeros
			for i := range padding {
				padding[i] = 0
			}
			
			// Write padding
			_, err = outFile.Write(padding)
			if err != nil {
				return fmt.Errorf("failed to write padding before section %d: %v", sectionType, err)
			}
			
			// Verify the position after adding padding
			actualPos, err = outFile.Seek(0, io.SeekCurrent)
			if err != nil {
				return fmt.Errorf("failed to get current position after adding padding: %v", err)
			}
			
			expectedPos := currentPos + paddingSize
			if int64(expectedPos) != actualPos {
				fmt.Printf("ERROR: Position mismatch after adding padding for section type %d\n", sectionType)
				fmt.Printf("  Expected position: %d, Actual file position: %d, Difference: %d\n", 
					expectedPos, actualPos, int64(expectedPos) - actualPos)
				return fmt.Errorf("position mismatch after adding padding")
			}
			
			fmt.Printf("Added %d bytes of padding to align to original section offset %d\n", paddingSize, section.StartOffset)
			currentPos += paddingSize
		} else if section.StartOffset < currentPos {
			return fmt.Errorf("current position %d is past the expected section offset %d for section type %d", 
				currentPos, section.StartOffset, sectionType)
		}
		
		// We already have the temp file from the state
		// No need to open it again, just rewind it
		_, err = tmpFile.Seek(0, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to seek to beginning of temporary file for section %d: %v", sectionType, err)
		}
		
		// Get section data size from the temporary file
		tmpStat, err := tmpFile.Stat()
		if err != nil {
			return fmt.Errorf("failed to stat temporary file for section %d: %v", sectionType, err)
		}
		sectionDataSize := uint64(tmpStat.Size())

		// Create section info for tracking
		sectionInfo := SectionInfo{
			Type:        sectionType,
			Instance:    section.Instance,
			StartOffset: currentPos,
			HeaderOffset: currentPos,
			Size:        sectionDataSize, // Will be updated after writing data
		}

		// To match the exact format of the original file, we need to read the original section header
		// and then update only the necessary fields
		sectionHeader := make([]byte, 64)
		
		// Save current position in the output file
		currentOutPos, err := outFile.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("failed to get current position in output file: %v", err)
		}
		
		// Seek to the original section header in the input file
		_, err = state.File.Seek(section.HeaderOffset, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to seek to original section header: %v", err)
		}
		
		// Read the original section header
		_, err = io.ReadFull(state.File, sectionHeader)
		if err != nil {
			return fmt.Errorf("failed to read original section header: %v", err)
		}
		
		// Restore position in the output file
		_, err = outFile.Seek(currentOutPos, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to restore position in output file: %v", err)
		}
		
		// Print the original section header bytes for debugging
		fmt.Printf("Original section header for type %d:\n", sectionType)
		for i := 0; i < 64; i += 16 {
			fmt.Printf("%04x: ", i)
			for j := 0; j < 16 && i+j < 64; j++ {
				fmt.Printf("%02x ", sectionHeader[i+j])
				if j == 7 {
					fmt.Printf("| ")
				}
			}
			fmt.Printf("\n")
		}
		
		// IMPORTANT: We GENERATE the headers exactly as the segmented writer does.
		// We do NOT reuse headers from the original snapshot.
		// This is critical because ultimately we will be folding multiple snapshots
		// together into these segments, and there will be NO HEADERS to copy.
		
		// Generate a new section header (64 bytes of zeros)
		for i := range sectionHeader {
			sectionHeader[i] = 0
		}
		
		// Set the section type (bytes 0-1) - EXACTLY as in segmented writer
		// From segment_writer.go: binary.BigEndian.PutUint16(header[0:], uint16(s.typ.GetEnumValue()))
		binary.BigEndian.PutUint16(sectionHeader[0:2], uint16(sectionType))
		
		// Set the section size (bytes 8-15)
		// The segmented writer calculates size as: current - offset - 64
		// Where current is the end position after writing the data
		// offset is the start of the section header
		// and 64 is the size of the header
		
		// Initially set the section size to 0
		// We'll update it after writing the data, exactly like the segmented writer does
		binary.BigEndian.PutUint64(sectionHeader[8:16], 0)
		
		// Next section offset (bytes 16-23) will be updated later
		// Leave it as zeros for now
		
		fmt.Printf("Generated new section header for type %d following segmented writer algorithm\n", sectionType)
		
		// Print key header values for verification
		fmt.Printf("  Section Type: %d\n", binary.BigEndian.Uint32(sectionHeader[0:4]))
		fmt.Printf("  Instance: %d\n", binary.BigEndian.Uint32(sectionHeader[4:8]))
		fmt.Printf("  Size: %d\n", binary.BigEndian.Uint64(sectionHeader[8:16]))
		fmt.Printf("  Next Offset: %d\n", binary.BigEndian.Uint64(sectionHeader[16:24]))
		
		// Verify the current file position matches our calculated position
		actualPos, err := outFile.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("failed to get current position in output file: %v", err)
		}
		
		if int64(currentPos) != actualPos {
			fmt.Printf("ERROR: Position mismatch before writing section header for type %d\n", sectionType)
			fmt.Printf("  Calculated position: %d, Actual file position: %d, Difference: %d\n", 
				currentPos, actualPos, int64(currentPos) - actualPos)
			return fmt.Errorf("position mismatch before writing section header")
		}
		
		// Write the section header
		_, err = outFile.Write(sectionHeader)
		if err != nil {
			return fmt.Errorf("failed to write section header: %v", err)
		}
		
		// Verify the position after writing header
		actualPos, err = outFile.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("failed to get current position after writing header: %v", err)
		}
		
		expectedPos := currentPos + 64 // Header size is 64 bytes
		if int64(expectedPos) != actualPos {
			fmt.Printf("ERROR: Position mismatch after writing section header for type %d\n", sectionType)
			fmt.Printf("  Expected position: %d, Actual file position: %d, Difference: %d\n", 
				expectedPos, actualPos, int64(expectedPos) - actualPos)
			return fmt.Errorf("position mismatch after writing section header")
		}
		
		// Save the header position for later size calculation
		headerPos := currentPos
		
		// Move past the header to the data position
		currentPos += 64 // Header size is 64 bytes
		sectionInfo.DataOffset = currentPos
		
		// Write the section data
		_, err = io.Copy(outFile, tmpFile)
		if err != nil {
			return fmt.Errorf("failed to write section data for section %d: %v", sectionType, err)
		}
		
		// Get current position after writing data
		endDataPos, err := outFile.Seek(0, io.SeekCurrent)
		if err != nil {
			return fmt.Errorf("failed to get current position after writing section data: %v", err)
		}
		
		// Update current position
		currentPos = endDataPos
		
		// Calculate section size EXACTLY as the segmented writer does:
		// From segment_writer.go: binary.BigEndian.PutUint64(header[8:], uint64(current-s.offset-64))
		// Where current is the position after writing data
		// offset is the start of the section header
		// and 64 is the header size
		actualSectionSize := uint64(endDataPos - headerPos - 64)
		
		// Update the section info with the actual size
		sectionInfo.Size = actualSectionSize
		
		// Now go back and update the size in the header
		_, err = outFile.Seek(headerPos+8, io.SeekStart) // Seek to size field in header
		if err != nil {
			return fmt.Errorf("failed to seek to size field in header: %v", err)
		}
		
		// Write the actual section size
		sizeBuf := make([]byte, 8)
		binary.BigEndian.PutUint64(sizeBuf, actualSectionSize)
		_, err = outFile.Write(sizeBuf)
		if err != nil {
			return fmt.Errorf("failed to update section size in header: %v", err)
		}
		
		// Return to the end of the data
		_, err = outFile.Seek(currentPos, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to seek back to end of data: %v", err)
		}
		
		// Print debug info about the section size
		fmt.Printf("Section type %d: calculated size = %d bytes\n", sectionType, actualSectionSize)
		
		// Current position is already updated after writing data, no need to verify or update it further
		
		// Add padding to align to 64-byte boundary, exactly as the segmented writer does
		if currentPos % 64 > 0 {
			padSize := 64 - (currentPos % 64)
			padding := make([]byte, padSize)
			_, err = outFile.Write(padding)
			if err != nil {
				return fmt.Errorf("failed to write padding after section data: %v", err)
			}
			fmt.Printf("Added %d bytes of padding to align to 64-byte boundary\n", padSize)
			currentPos += padSize
		}

		// Update section info with end offset
		sectionInfo.EndOffset = currentPos
		sectionInfos = append(sectionInfos, sectionInfo)

		fmt.Printf("Wrote section type %d (instance %d) (%d bytes) to output file\n", sectionType, section.Instance, actualSectionSize)
	}

	// Update the next section offset for all sections
	// EXACTLY as the segmented writer does in Open() method
	if len(sectionInfos) > 0 {
		fmt.Printf("Updating next section offsets for all sections...\n")
		
		for i := 0; i < len(sectionInfos)-1; i++ {
			// Get the current and next section
			currentSection := sectionInfos[i]
			nextSection := sectionInfos[i+1]
			
			// Calculate the next section offset
			// From segment_writer.go: binary.BigEndian.PutUint64(headerPart[:], uint64(offset))
			nextOffset := nextSection.HeaderOffset
			
			// Seek to the next offset field in the current section header
			// This is at bytes 16-23 in the section header (exactly as in segmented writer)
			// From segment_writer.go: _, err = w.file.Seek(w.prevSegment+16, io.SeekStart)
			nextOffsetPos := currentSection.HeaderOffset + 16
			_, err = outFile.Seek(nextOffsetPos, io.SeekStart)
			if err != nil {
				return fmt.Errorf("failed to seek to next offset field in section header: %v", err)
			}
			
			// Write the next section offset
			nextOffsetBuf := make([]byte, 8)
			binary.BigEndian.PutUint64(nextOffsetBuf, uint64(nextOffset))
			_, err = outFile.Write(nextOffsetBuf)
			if err != nil {
				return fmt.Errorf("failed to write next section offset: %v", err)
			}
			
			fmt.Printf("Updated section %d next offset to %d (0x%x)\n", 
				currentSection.Type, nextOffset, nextOffset)
		}
		
		// The last section's next offset should be 0
		lastSection := sectionInfos[len(sectionInfos)-1]
		lastOffsetPos := lastSection.HeaderOffset + 16
		_, err = outFile.Seek(lastOffsetPos, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to seek to next offset field in last section header: %v", err)
		}
		
		// Write 0 as the next section offset for the last section
		zeroOffsetBuf := make([]byte, 8)
		_, err = outFile.Write(zeroOffsetBuf)
		if err != nil {
			return fmt.Errorf("failed to write zero next section offset for last section: %v", err)
		}
		
		fmt.Printf("Set last section %d next offset to 0\n", lastSection.Type)
	}
	
	fmt.Printf("Reconstruction complete. First section offset preserved at %d bytes.\n", state.FirstSectionOffset)

	// Print section information for debugging
	fmt.Printf("\nSection Information for Debugging:\n")
	fmt.Printf("%-6s %-12s %-12s %-12s %-12s %-12s\n", "Type", "Start", "Header", "Data", "Size", "End")
	fmt.Printf("%-6s %-12s %-12s %-12s %-12s %-12s\n", "------", "------------", "------------", "------------", "------------", "------------")
	
	// Track original section sizes for comparison
	originalSectionSizes := make(map[uint32]uint64)
	for _, section := range state.OriginalSections {
		originalSectionSizes[section.Type] += section.Size
	}
	
	// Track reconstructed section sizes
	reconstructedSectionSizes := make(map[uint32]uint64)
	for _, info := range sectionInfos {
		fmt.Printf("%-6d %-12d %-12d %-12d %-12d %-12d\n", 
			info.Type, info.StartOffset, info.HeaderOffset, info.DataOffset, info.Size, info.EndOffset)
		reconstructedSectionSizes[info.Type] += info.Size
	}
	
	// Compare original and reconstructed section sizes
	fmt.Printf("\nSection Size Comparison (Original vs Reconstructed):\n")
	fmt.Printf("%-6s %-15s %-15s %-15s\n", "Type", "Original", "Reconstructed", "Difference")
	fmt.Printf("%-6s %-15s %-15s %-15s\n", "------", "---------------", "---------------", "---------------")
	
	// Collect all section types
	sectionTypes := make(map[uint32]bool)
	for t := range originalSectionSizes {
		sectionTypes[t] = true
	}
	for t := range reconstructedSectionSizes {
		sectionTypes[t] = true
	}
	
	// Print size comparison for each section type
	totalOriginal := uint64(0)
	totalReconstructed := uint64(0)
	for t := range sectionTypes {
		orig := originalSectionSizes[t]
		recon := reconstructedSectionSizes[t]
		diff := int64(recon) - int64(orig)
		fmt.Printf("%-6d %-15d %-15d %-15d\n", t, orig, recon, diff)
		totalOriginal += orig
		totalReconstructed += recon
	}
	
	// Print totals
	fmt.Printf("%-6s %-15d %-15d %-15d\n", "Total", totalOriginal, totalReconstructed, 
		int64(totalReconstructed) - int64(totalOriginal))
	
	// Store the section information in the state for use during validation
	state.ReconstructionInfo = sectionInfos
	
	fmt.Printf("Snapshot reconstruction completed, wrote %d bytes\n", currentPos)
	return nil
}

// sc_ValidateReconstructionImpl performs byte-by-byte comparison of two snapshot files
// This is the implementation of the stub function defined in sc.go
func printByteContext(buffer []byte, start, highlight, end, baseOffset int) {
	// Calculate how many lines we need to print (16 bytes per line)
	startLine := start / 16
	endLine := end / 16
	
	for line := startLine; line <= endLine; line++ {
		lineStart := line * 16
		lineEnd := lineStart + 15
		if lineEnd > end {
			lineEnd = end
		}
		
		// Print the absolute offset at the start of the line
		absOffset := baseOffset + (lineStart - start)
		fmt.Printf("%08x  ", absOffset)
		
		// Print hex values with 8-byte boundaries
		for i := lineStart; i <= lineEnd; i++ {
			if i < start || i > end {
				fmt.Printf("   ")
				continue
			}
			
			// Add visual separator every 8 bytes
			if i > lineStart && i % 8 == 0 {
				fmt.Printf(" |")
			}
			
			if i == highlight {
				// Highlight the mismatched byte in red and bold
				fmt.Printf(" \033[1;31m%02x\033[0m", buffer[i])
			} else {
				fmt.Printf(" %02x", buffer[i])
			}
		}
		
		// Pad the rest of the line if it's not complete
		for i := lineEnd + 1; i < lineStart + 16; i++ {
			// Add visual separator every 8 bytes
			if i > lineStart && i % 8 == 0 {
				fmt.Printf(" |")
			}
			fmt.Printf("   ")
		}
		
		// Print ASCII representation with 8-byte boundary marker
		fmt.Printf("  |")
		for i := lineStart; i <= lineEnd; i++ {
			if i < start || i > end {
				fmt.Printf(" ")
				continue
			}
			
			// Add visual separator every 8 bytes
			if i > lineStart && i % 8 == 0 {
				fmt.Printf("|")
			}
			
			b := buffer[i]
			if b >= 32 && b <= 126 { // Printable ASCII
				if i == highlight {
					// Highlight the mismatched byte
					fmt.Printf("\033[1;31m%c\033[0m", b)
				} else {
					fmt.Printf("%c", b)
				}
			} else {
				fmt.Printf(".")
			}
		}
		fmt.Printf("|\n")
	}
}
// sc_ValidateReconstructionImpl performs byte-by-byte comparison of two snapshot files
func sc_ValidateReconstructionImpl(originalPath, reconstructedPath string) (bool, error) {
	fmt.Printf("Validating reconstruction: %s vs %s\n", originalPath, reconstructedPath)
	
	// Open the original file
	originalFile, err := os.Open(originalPath)
	if err != nil {
		return false, fmt.Errorf("failed to open original file: %v", err)
	}
	defer originalFile.Close()
	
	// Open the reconstructed file
	reconstructedFile, err := os.Open(reconstructedPath)
	if err != nil {
		return false, fmt.Errorf("failed to open reconstructed file: %v", err)
	}
	defer reconstructedFile.Close()
	
	// Get file sizes
	originalInfo, err := originalFile.Stat()
	if err != nil {
		return false, fmt.Errorf("failed to get original file info: %v", err)
	}
	
	reconstructedInfo, err := reconstructedFile.Stat()
	if err != nil {
		return false, fmt.Errorf("failed to get reconstructed file info: %v", err)
	}
	
	originalSize := originalInfo.Size()
	reconstructedSize := reconstructedInfo.Size()
	
	fmt.Printf("Original file size: %d bytes\n", originalSize)
	fmt.Printf("Reconstructed file size: %d bytes\n", reconstructedSize)
	
	// No artificial byte modifications - we want to validate the actual reconstruction
	
	// Check if file sizes match
	if originalSize != reconstructedSize {
		fmt.Printf("File sizes do not match: original=%d, reconstructed=%d (difference: %d bytes)\n", 
			originalSize, reconstructedSize, abs(reconstructedSize - originalSize))
		fmt.Printf("Continuing with comparison to find first mismatch...\n\n")
	}
	
	// Perform byte-by-byte comparison in chunks
	bufferSize := 1024 * 1024 // 1MB buffer
	originalBuffer := make([]byte, bufferSize)
	reconstructedBuffer := make([]byte, bufferSize)
	
	offset := int64(0)
	
	// Determine the maximum size we can compare (the smaller of the two files)
	maxSize := originalInfo.Size()
	if reconstructedInfo.Size() < maxSize {
		maxSize = reconstructedInfo.Size()
	}
	
	for offset < maxSize {
		// Calculate how many bytes to read in this chunk
		readSize := bufferSize
		if offset + int64(readSize) > maxSize {
			readSize = int(maxSize - offset)
		}
		
		// Read chunk from original file
		n1, err := originalFile.Read(originalBuffer[:readSize])
		if err != nil && err != io.EOF {
			return false, fmt.Errorf("error reading original file at offset %d: %v", offset, err)
		}
		
		// Read chunk from reconstructed file
		n2, err := reconstructedFile.Read(reconstructedBuffer[:readSize])
		if err != nil && err != io.EOF {
			return false, fmt.Errorf("error reading reconstructed file at offset %d: %v", offset, err)
		}
		
		// Ensure we read the same number of bytes from both files
		if n1 != n2 {
			return false, fmt.Errorf("read size mismatch at offset %d: original=%d, reconstructed=%d", 
				offset, n1, n2)
		}
		
		// Compare the bytes
		for i := 0; i < n1; i++ {
			if originalBuffer[i] != reconstructedBuffer[i] {
				// Found first mismatch - print detailed information
				absoluteOffset := offset + int64(i)
				fmt.Printf("\nFIRST MISMATCH FOUND at offset %d (0x%x)\n", absoluteOffset, absoluteOffset)
				fmt.Printf("Original byte: 0x%02x, Reconstructed byte: 0x%02x\n", originalBuffer[i], reconstructedBuffer[i])
				
				// Print surrounding bytes for context (up to 128 bytes before and after = 256 bytes total)
				contextStart := i - 128
				if contextStart < 0 {
					contextStart = 0
				}
				contextEnd := i + 128
				if contextEnd >= n1 {
					contextEnd = n1 - 1
				}
				
				fmt.Println("\nContext bytes from original file (showing up to 256 bytes around mismatch):")
				printByteContext(originalBuffer, contextStart, i, contextEnd, int(absoluteOffset-int64(i-contextStart)))
				
				fmt.Println("\nContext bytes from reconstructed file (showing up to 256 bytes around mismatch):")
				printByteContext(reconstructedBuffer, contextStart, i, contextEnd, int(absoluteOffset-int64(i-contextStart)))
				
				// Stop after finding the first mismatch
				return false, nil
			}
		}
		
		// Move to the next chunk
		offset += int64(n1)
		
		// Progress indicator for large files
		if offset % int64(100 * bufferSize) == 0 {
			fmt.Printf("Validated %d/%d bytes (%.1f%%)\n", 
				offset, maxSize, float64(offset) * 100.0 / float64(maxSize))
		}
	}
	
	// If we got here, no mismatches were found in the common part
	if originalInfo.Size() != reconstructedInfo.Size() {
		// Files are different sizes but all common bytes match
		// Let's examine what's in the extra bytes of the larger file
		largerFile := originalFile
		largerSize := originalInfo.Size()
		isOriginal := true
		
		if reconstructedInfo.Size() > originalInfo.Size() {
			largerFile = reconstructedFile
			largerSize = reconstructedInfo.Size()
			isOriginal = false
		}
		
		// Seek to the end of the common part
		_, err := largerFile.Seek(maxSize, 0)
		if err != nil {
			return false, fmt.Errorf("failed to seek in larger file: %v", err)
		}
		
		// Read some of the extra bytes to analyze
		extraBytes := make([]byte, 64)
		if largerSize - maxSize < 64 {
			extraBytes = make([]byte, largerSize - maxSize)
		}
		
		n, err := largerFile.Read(extraBytes)
		if err != nil && err != io.EOF {
			return false, fmt.Errorf("failed to read extra bytes: %v", err)
		}
		
		fmt.Printf("\nFiles match for the first %d bytes, but differ in length.\n", maxSize)
		fmt.Printf("First %d extra bytes in the %s file:\n", n, 
			map[bool]string{true: "original", false: "reconstructed"}[isOriginal])
		
		// Print the extra bytes
		for i := 0; i < n; i++ {
			if i > 0 && i % 16 == 0 {
				fmt.Printf("\n")
			}
			fmt.Printf("%02x ", extraBytes[i])
		}
		fmt.Printf("\n")
		
		return false, nil
	}
	
	fmt.Println("Validation successful - Files match byte-by-byte")
	return true, nil
}

// Initialize the real implementations by replacing the stubs
func init() {
	// Replace the stub functions with the real implementations
	sc_ReconstructSnapshot = sc_ReconstructSnapshotImpl
	sc_ValidateReconstruction = sc_ValidateReconstructionImpl
}
