package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

// sc_UpdateOffsets updates the next section offset fields in all section headers
// It uses the section offsets collected during the write phase
func sc_UpdateOffsets(outFile *os.File, sectionOffsets []SectionOffset) error {
	fmt.Printf("Updating section offsets in output file...\n")
	
	// Ensure we have at least one section
	if len(sectionOffsets) == 0 {
		return fmt.Errorf("no section offsets to update")
	}
	
	// Sort section offsets by file position to ensure consistent processing
	sc_sortSectionOffsetsByPosition(sectionOffsets)
	
	// For each section, update its next section offset field
	for i := 0; i < len(sectionOffsets); i++ {
		currentSection := sectionOffsets[i]
		
		// Calculate the next section offset
		var nextSectionOffset uint64
		if i < len(sectionOffsets)-1 {
			// If there's a next section, use its file offset
			nextSectionOffset = sectionOffsets[i+1].FileOffset
		} else {
			// For the last section, set next section offset to 0
			nextSectionOffset = 0
		}
		
		// Seek to the position where the next section offset needs to be written
		_, err := outFile.Seek(currentSection.NextSectionPos, 0)
		if err != nil {
			return fmt.Errorf("failed to seek to offset position for section type %d index %d: %w", 
				currentSection.SectionType, currentSection.SectionIndex, err)
		}
		
		// Write the next section offset (8 bytes, big-endian)
		offsetBuf := make([]byte, 8)
		binary.BigEndian.PutUint64(offsetBuf, nextSectionOffset)
		_, err = outFile.Write(offsetBuf)
		if err != nil {
			return fmt.Errorf("failed to write next section offset for section type %d index %d: %w", 
				currentSection.SectionType, currentSection.SectionIndex, err)
		}
		
		fmt.Printf("Updated section type %d index %d: next section offset = %d\n", 
			currentSection.SectionType, currentSection.SectionIndex, nextSectionOffset)
	}
	
	// Update the snapshot header's next section offset field (at position 16)
	if len(sectionOffsets) > 0 {
		// Seek to the next section offset field in the snapshot header
		_, err := outFile.Seek(16, 0)
		if err != nil {
			return fmt.Errorf("failed to seek to snapshot header next section offset: %w", err)
		}
		
		// Write the offset of the first section
		offsetBuf := make([]byte, 8)
		binary.BigEndian.PutUint64(offsetBuf, sectionOffsets[0].FileOffset)
		_, err = outFile.Write(offsetBuf)
		if err != nil {
			return fmt.Errorf("failed to write snapshot header next section offset: %w", err)
		}
		
		fmt.Printf("Updated snapshot header: next section offset = %d\n", sectionOffsets[0].FileOffset)
	}
	
	// Ensure all writes are flushed to disk
	err := outFile.Sync()
	if err != nil {
		return fmt.Errorf("failed to sync output file: %w", err)
	}
	
	fmt.Printf("All section offsets updated successfully\n")
	return nil
}

// This function has been moved to sc_utils.go to avoid redeclaration
// Using sc_sortSectionOffsetsByPosition instead

// sc_UpdateOffsetsTest is a unit test for sc_UpdateOffsets
func sc_UpdateOffsetsTest() error {
	// Create temporary test file
	tempDir, err := os.MkdirTemp("", "sc_test")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	
	// Create output file with placeholder data
	outFile, err := os.CreateTemp(tempDir, "test_output")
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()
	
	// Write placeholder data for snapshot header and three section headers
	// Each header is 64 bytes
	headerData := make([]byte, 64*4) // Snapshot header + 3 section headers
	_, err = outFile.Write(headerData)
	if err != nil {
		return fmt.Errorf("failed to write placeholder data: %w", err)
	}
	
	// Create test section offsets
	sectionOffsets := []SectionOffset{
		{
			SectionType:    1,
			SectionIndex:   1,
			FileOffset:     64,  // Header section starts at byte 64
			SectionSize:    4,
			NextSectionPos: 16,  // Position in snapshot header for next section offset
		},
		{
			SectionType:    2,
			SectionIndex:   1,
			FileOffset:     128, // Data section starts at byte 128
			SectionSize:    20,
			NextSectionPos: 80,  // Position in header section header for next section offset
		},
		{
			SectionType:    7,
			SectionIndex:   1,
			FileOffset:     212, // Empty section starts at byte 212
			SectionSize:    0,
			NextSectionPos: 144, // Position in data section header for next section offset
		},
	}
	
	// Reset file position
	_, err = outFile.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("failed to reset file position: %w", err)
	}
	
	// Update offsets
	err = sc_UpdateOffsets(outFile, sectionOffsets)
	if err != nil {
		return fmt.Errorf("sc_UpdateOffsets failed: %w", err)
	}
	
	// Verify offsets were written correctly
	// Reset file position
	_, err = outFile.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("failed to reset file position: %w", err)
	}
	
	// Read the file data
	fileData := make([]byte, 64*4)
	_, err = outFile.Read(fileData)
	if err != nil {
		return fmt.Errorf("failed to read file data: %w", err)
	}
	
	// Check snapshot header next section offset (at position 16)
	snapshotNextOffset := binary.BigEndian.Uint64(fileData[16:24])
	if snapshotNextOffset != 64 {
		return fmt.Errorf("snapshot header next section offset is %d, expected 64", snapshotNextOffset)
	}
	
	// Check header section next section offset (at position 80)
	headerNextOffset := binary.BigEndian.Uint64(fileData[80:88])
	if headerNextOffset != 128 {
		return fmt.Errorf("header section next section offset is %d, expected 128", headerNextOffset)
	}
	
	// Check data section next section offset (at position 144)
	dataNextOffset := binary.BigEndian.Uint64(fileData[144:152])
	if dataNextOffset != 212 {
		return fmt.Errorf("data section next section offset is %d, expected 212", dataNextOffset)
	}
	
	// Check empty section next section offset (at position 208)
	emptyNextOffset := binary.BigEndian.Uint64(fileData[208:216])
	if emptyNextOffset != 0 {
		return fmt.Errorf("empty section next section offset is %d, expected 0", emptyNextOffset)
	}
	
	fmt.Printf("sc_UpdateOffsetsTest: PASSED\n")
	return nil
}
