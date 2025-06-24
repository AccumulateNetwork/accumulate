package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

// sc_StartReconstruction begins the snapshot reconstruction process
// It validates temporary files and prepares for reconstruction
func sc_StartReconstruction(scState *sc_State) error {
	// Record start time for reporting
	scState.StartTime = time.Now()

	fmt.Printf("Starting snapshot reconstruction...\n")

	// Validate that we have temporary files to work with
	if scState.SectionFiles.Count() == 0 {
		return fmt.Errorf("no section files found for reconstruction")
	}

	// Print a table of temporary files and their sizes
	fmt.Printf("\nTemporary files for reconstruction:\n")
	fmt.Printf("%-20s %-15s %-15s\n", "File", "Section Type", "Size (bytes)")
	fmt.Printf("%-20s %-15s %-15s\n", "--------------------", "---------------", "---------------")

	// Get a sorted list of section types for consistent output
	keys := scState.SectionFiles.Keys()
	sort.Strings(keys)

	// Track total size of all section data
	var totalSize int64

	// Print each file and its size
	for _, key := range keys {
		section := scState.SectionFiles.Get(key)
		if section == nil || section.TmpFile == nil {
			return fmt.Errorf("section file not found for key %s", key)
		}

		// Get file info to determine size
		fileInfo, err := section.TmpFile.Stat()
		if err != nil {
			return fmt.Errorf("failed to get file info for %s: %w", key, err)
		}

		// Parse section type from the key (format is "type_index")
		parts := strings.Split(key, "_")
		if len(parts) != 2 {
			return fmt.Errorf("invalid section file key format: %s", key)
		}

		// Print file size and section type
		fmt.Printf("Section %s: %d bytes (type %s)\n", key, fileInfo.Size(), parts[0])

		// Add to total size
		totalSize += fileInfo.Size()

		// Warn if file is empty
		if fileInfo.Size() == 0 {
			fmt.Printf("WARNING: Section file %s is empty\n", key)
		}

		// Reset file position to beginning for reading
		_, err = section.TmpFile.Seek(0, io.SeekStart)
		if err != nil {
			return fmt.Errorf("failed to reset file position for %s: %w", key, err)
		}
	}

	fmt.Printf("\nTotal section data size: %d bytes\n", totalSize)

	// Validate that we have the required header section (type 1)
	headerSection := scState.SectionFiles.Get("1_1")
	if headerSection == nil || headerSection.TmpFile == nil {
		return fmt.Errorf("missing required header section file (1_1)")
	}

	// Read the format version from the header section
	formatVersionBytes := make([]byte, 4)
	_, err := headerSection.TmpFile.Read(formatVersionBytes)
	if err != nil {
		return fmt.Errorf("failed to read format version from header file: %w", err)
	}

	fmt.Printf("Reconstruction preparation complete. Ready to write sections.\n")
	return nil
}

// sc_StartReconstructionTest is a unit test for sc_StartReconstruction
func sc_StartReconstructionTest() error {
	// Create a test scState
	scState := &sc_State{
		SectionFiles: NewSections(),
	}

	// Create temporary test files
	tempDir, err := os.MkdirTemp("", "sc_test")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test section files
	testFile1, err := os.CreateTemp("", "test-section-1-*")
	if err != nil {
		return fmt.Errorf("failed to create test file: %w", err)
	}
	scState.SectionFiles.Add("1_1", testFile1)

	// Write test header data
	_, err = testFile1.Write([]byte{0, 0, 0, 2}) // Format version 2
	if err != nil {
		return fmt.Errorf("failed to write test data: %w", err)
	}

	// Reset file position
	_, err = testFile1.Seek(0, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to reset file position: %w", err)
	}
	defer testFile1.Close()

	// Create another section file
	dataFile, err := os.CreateTemp(tempDir, "section_2_1")
	if err != nil {
		return fmt.Errorf("failed to create data file: %w", err)
	}
	defer dataFile.Close()

	// Write test data
	_, err = dataFile.Write([]byte("test data"))
	if err != nil {
		return fmt.Errorf("failed to write to data file: %w", err)
	}

	// Add files to scState - testFile1 is already added as "1_1" above
	scState.SectionFiles.Add("2_1", dataFile)

	// Run the reconstruction start function
	err = sc_StartReconstruction(scState)
	if err != nil {
		return fmt.Errorf("sc_StartReconstruction failed: %w", err)
	}

	fmt.Printf("sc_StartReconstructionTest: PASSED\n")
	return nil
}
