package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// sc_WriteSections writes all sections to the destination file
func sc_WriteSections(scState *sc_State) error {
	fmt.Printf("Writing sections to output file...\n")

	// Track the current file position
	currentPos := int64(64) // Start after the 64-byte snapshot header

	// Get a sorted list of section keys for consistent processing
	sectionKeys := sc_getSortedSectionKeys(scState.SectionFiles)

	// First, write the snapshot header (64 bytes)
	headerBuf := make([]byte, 64)

	// Set the section type to 1 (header section) at bytes 0-1
	binary.BigEndian.PutUint16(headerBuf[0:2], 1)

	// Get the header section file to determine its size
	headerFile := scState.SectionFiles["1_1"]
	if headerFile == nil {
		return fmt.Errorf("header section file not found")
	}

	headerInfo, err := headerFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get header file info: %w", err)
	}

	// Set the section size to the header file size at bytes 8-15
	binary.BigEndian.PutUint64(headerBuf[8:16], uint64(headerInfo.Size()))

	// Set the first section offset at bytes 16-23
	// This will be 64 (header) + header data size, aligned to 64-byte boundary
	// The first section offset points to where the first section begins in the file
	firstSectionOffset := alignToBlockBoundary(64 + headerInfo.Size())
	binary.BigEndian.PutUint64(headerBuf[16:24], uint64(firstSectionOffset))

	// This is where we calculate the section offset - it points to the beginning of the next section
	fmt.Printf("First section will start at offset: %d\n", firstSectionOffset)

	// Write the generated header
	_, err = scState.OutFile.Write(headerBuf)
	if err != nil {
		return fmt.Errorf("failed to write snapshot header: %w", err)
	}

	fmt.Printf("Wrote 64-byte snapshot header\n")

	// Now write the header section data (from the header section file)
	err = sc_WriteSection(scState, 1, 1, &currentPos)
	if err != nil {
		return fmt.Errorf("failed to write header section: %w", err)
	}

	// Write all other sections
	for _, key := range sectionKeys {
		// Skip the header section as we've already written it
		if key == "1_1" {
			continue
		}

		// Parse section type and index from the key
		sectionType, sectionIndex, err := sc_parseSectionKey(key)
		if err != nil {
			return fmt.Errorf("invalid section key %s: %w", key, err)
		}

		// Write the section
		err = sc_WriteSection(scState, sectionType, sectionIndex, &currentPos)
		if err != nil {
			return fmt.Errorf("failed to write section %s: %w", key, err)
		}
	}

	fmt.Printf("All sections written successfully\n")
	return nil
}

// sc_WriteSection writes a single section to the output file
func sc_WriteSection(scState *sc_State, sectionType, sectionIndex int, currentPos *int64) error {
	outFile := scState.OutFile
	// Generate the section key
	sectionKey := fmt.Sprintf("%d_%d", sectionType, sectionIndex)

	// Get the section file
	sectionFile := scState.SectionFiles[sectionKey]
	if sectionFile == nil {
		return fmt.Errorf("section file %s not found", sectionKey)
	}

	// Get file info to determine size
	fileInfo, err := sectionFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info for %s: %w", sectionKey, err)
	}

	// Reset file position to beginning
	_, err = sectionFile.Seek(0, 0)
	if err != nil {
		return fmt.Errorf("failed to reset file position for %s: %w", sectionKey, err)
	}

	// Special handling for section type 7 (empty section)
	if sectionType == 7 {
		// For section type 7, we write an empty section with just the header
		fmt.Printf("Writing empty section type 7 (index %d)\n", sectionIndex)

		// Write section header (64 bytes)
		sectionHeader := make([]byte, 64)

		// Set section type
		binary.BigEndian.PutUint16(sectionHeader[0:2], uint16(sectionType))

		// Set section size to 0
		binary.BigEndian.PutUint64(sectionHeader[8:16], 0)

		// Write the section header
		_, err = outFile.Write(sectionHeader)
		if err != nil {
			return fmt.Errorf("failed to write section header for %s: %w", sectionKey, err)
		}

		// Update current position
		*currentPos += 64

		// Align to 64-byte boundary
		newPos := alignToBlockBoundary(*currentPos)
		if newPos > *currentPos {
			paddingSize := newPos - *currentPos
			padding := make([]byte, paddingSize)
			_, err = outFile.Write(padding)
			if err != nil {
				return fmt.Errorf("failed to write padding after section %s: %w", sectionKey, err)
			}
			*currentPos = newPos
		}

		return nil
	}

	// Regular section handling
	fmt.Printf("Writing section type %d (index %d), size: %d bytes\n", sectionType, sectionIndex, fileInfo.Size())

	// Write section header (64 bytes)
	sectionHeader := make([]byte, 64)

	// Set section type
	binary.BigEndian.PutUint16(sectionHeader[0:2], uint16(sectionType))

	// Set section size
	binary.BigEndian.PutUint64(sectionHeader[8:16], uint64(fileInfo.Size()))

	// Calculate the next section offset
	// This is the position where the next section will start
	// Current position + 64 (section header) + section data size, aligned to boundary
	nextSectionOffset := alignToBlockBoundary(*currentPos + 64 + fileInfo.Size())

	// Store the position where we're writing the next section offset
	// This is for tracking purposes only, as we're writing it now
	nextSectionOffsetPos := *currentPos + 16

	// Write the next section offset into the header (bytes 16-23)
	// This points to where the next section will begin
	binary.BigEndian.PutUint64(sectionHeader[16:24], uint64(nextSectionOffset))

	fmt.Printf("  Section offset: %d -> %d (next section at %d)\n", *currentPos, nextSectionOffsetPos, nextSectionOffset)

	// Write the section header
	_, err = outFile.Write(sectionHeader)
	if err != nil {
		return fmt.Errorf("failed to write section header for %s: %w", sectionKey, err)
	}

	// Section offset information is now written directly to the section header
	// No need to store for later updates

	// Update current position after writing header
	*currentPos += 64

	// Copy section data from temporary file to output file
	buffer := make([]byte, 32*1024) // 32KB buffer for copying
	var totalCopied int64

	for {
		n, err := sectionFile.Read(buffer)
		if err != nil && err != io.EOF {
			return fmt.Errorf("failed to read from section file %s: %w", sectionKey, err)
		}

		if n == 0 {
			break
		}

		_, err = outFile.Write(buffer[:n])
		if err != nil {
			return fmt.Errorf("failed to write section data for %s: %w", sectionKey, err)
		}

		totalCopied += int64(n)

		if err == io.EOF {
			break
		}
	}

	// Verify we copied the expected amount of data
	if totalCopied != fileInfo.Size() {
		return fmt.Errorf("copied %d bytes for section %s, but expected %d bytes", totalCopied, sectionKey, fileInfo.Size())
	}

	// Update current position after writing section data
	*currentPos += totalCopied

	// Align to 64-byte boundary
	newPos := alignToBlockBoundary(*currentPos)
	if newPos > *currentPos {
		paddingSize := newPos - *currentPos
		padding := make([]byte, paddingSize)
		_, err = outFile.Write(padding)
		if err != nil {
			return fmt.Errorf("failed to write padding after section %s: %w", sectionKey, err)
		}
		fmt.Printf("Added %d bytes of padding to align to 64-byte boundary after section %s\n", paddingSize, sectionKey)
		*currentPos = newPos
	}

	return nil
}

// alignToBlockBoundary aligns a position to the next 64-byte boundary
func alignToBlockBoundary(pos int64) int64 {
	if pos%64 == 0 {
		return pos
	}
	return ((pos / 64) + 1) * 64
}

// This function has been moved to sc_utils.go to avoid redeclaration
// Using sc_getSortedSectionKeys instead

// This function has been moved to sc_utils.go to avoid redeclaration
// Using sc_parseSectionKey instead

// sc_WriteSectionTest is a unit test for sc_WriteSection
func sc_WriteSectionTest() error {
	// Create a test scState
	scState := &sc_State{
		SectionFiles: make(map[string]*os.File),
	}

	// Create temporary test files
	tempDir, err := os.MkdirTemp("", "sc_test")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Create header section file
	headerFile, err := os.CreateTemp(tempDir, "section_1_1")
	if err != nil {
		return fmt.Errorf("failed to create header file: %w", err)
	}
	defer headerFile.Close()

	// Write test format version (4 bytes)
	_, err = headerFile.Write([]byte{0x00, 0x00, 0x00, 0x02})
	if err != nil {
		return fmt.Errorf("failed to write to header file: %w", err)
	}

	// Create data section file
	dataFile, err := os.CreateTemp(tempDir, "section_2_1")
	if err != nil {
		return fmt.Errorf("failed to create data file: %w", err)
	}
	defer dataFile.Close()

	// Write test data
	testData := []byte("test data for section 2")
	_, err = dataFile.Write(testData)
	if err != nil {
		return fmt.Errorf("failed to write to data file: %w", err)
	}

	// Create empty section 7 file
	emptyFile, err := os.CreateTemp(tempDir, "section_7_1")
	if err != nil {
		return fmt.Errorf("failed to create empty file: %w", err)
	}
	defer emptyFile.Close()

	// Add files to scState
	scState.SectionFiles["1_1"] = headerFile
	scState.SectionFiles["2_1"] = dataFile
	scState.SectionFiles["7_1"] = emptyFile

	// Create output file
	outFile, err := os.CreateTemp(tempDir, "test_output")
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	// Write sections
	var currentPos int64 = 64 // Start after header

	// Write header section
	err = sc_WriteSection(scState, 1, 1, &currentPos)
	if err != nil {
		return fmt.Errorf("failed to write header section: %w", err)
	}

	// Write data section
	err = sc_WriteSection(scState, 2, 1, &currentPos)
	if err != nil {
		return fmt.Errorf("failed to write data section: %w", err)
	}

	// Write empty section
	err = sc_WriteSection(scState, 7, 1, &currentPos)
	if err != nil {
		return fmt.Errorf("failed to write empty section: %w", err)
	}

	// Verify we have the expected number of sections
	expectedSections := 3
	if len(scState.SectionFiles) != expectedSections {
		return fmt.Errorf("expected %d sections, got %d", expectedSections, len(scState.SectionFiles))
	}

	fmt.Printf("sc_WriteSectionTest: PASSED\n")
	return nil
}
