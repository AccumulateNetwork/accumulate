package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// sc_StartReconstruction begins the snapshot reconstruction process
// It validates temporary files and prepares for reconstruction
func sc_StartReconstruction(scState *sc_State, outputPath string) error {
	// Record start time for reporting
	scState.StartTime = time.Now()

	fmt.Printf("Starting snapshot reconstruction...\n")

	// Validate that we have temporary files to work with
	if len(scState.SectionFiles) == 0 {
		return fmt.Errorf("no section files found for reconstruction")
	}

	// Create output file and store it in the scState
	fmt.Printf("Creating output file: %s\n", outputPath)
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	scState.OutFile = outFile

	// Print a table of temporary files and their sizes
	fmt.Printf("\nTemporary files for reconstruction:\n")
	fmt.Printf("%-20s %-15s %-15s\n", "File", "Section Type", "Size (bytes)")
	fmt.Printf("%-20s %-15s %-15s\n", "--------------------", "---------------", "---------------")

	// Get a sorted list of keys for consistent output
	keys := make([]string, 0, len(scState.SectionFiles))
	for k := range scState.SectionFiles {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Track total size of all section data
	var totalSize int64

	// Print each file and its size
	for _, key := range keys {
		file := scState.SectionFiles[key]

		// Get file info to determine size
		fileInfo, err := file.Stat()
		if err != nil {
			return fmt.Errorf("failed to get file info for %s: %w", key, err)
		}

		// Parse section type from the key (format is "type_index")
		parts := strings.Split(key, "_")
		if len(parts) != 2 {
			return fmt.Errorf("invalid section file key format: %s", key)
		}

		sectionType, err := strconv.Atoi(parts[0])
		if err != nil {
			return fmt.Errorf("invalid section type in key %s: %w", key, err)
		}

		// Print file details
		fmt.Printf("%-20s %-15d %-15d\n", filepath.Base(file.Name()), sectionType, fileInfo.Size())

		// Add to total size
		totalSize += fileInfo.Size()

		// Validate that the file exists and has content (except for section type 7 which can be empty)
		if fileInfo.Size() == 0 && sectionType != 7 {
			fmt.Printf("WARNING: Section file %s is empty\n", key)
		}

		// Reset file position to beginning for subsequent operations
		_, err = file.Seek(0, 0)
		if err != nil {
			return fmt.Errorf("failed to reset file position for %s: %w", key, err)
		}
	}

	fmt.Printf("\nTotal section data size: %d bytes\n", totalSize)

	// Validate that we have the required header section (type 1)
	headerFile := scState.SectionFiles["1_1"]
	if headerFile == nil {
		return fmt.Errorf("missing required header section file (1_1)")
	}

	// Get header file info
	headerInfo, err := headerFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get header file info: %w", err)
	}

	// Validate header file size
	if headerInfo.Size() < 4 {
		return fmt.Errorf("header file is too small (%d bytes), must be at least 4 bytes", headerInfo.Size())
	}

	fmt.Printf("Reconstruction preparation complete. Ready to write sections.\n")
	return nil
}

// sc_StartReconstructionTest is a unit test for sc_StartReconstruction
func sc_StartReconstructionTest() error {
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

	// Add files to scState
	scState.SectionFiles["1_1"] = headerFile
	scState.SectionFiles["2_1"] = dataFile

	// Create a temporary output path for testing
	testOutputPath := filepath.Join(scState.TempDir, "test_output.snap")

	// Run the function with output path
	err = sc_StartReconstruction(scState, testOutputPath)
	if err != nil {
		return fmt.Errorf("sc_StartReconstruction failed: %w", err)
	}

	fmt.Printf("sc_StartReconstructionTest: PASSED\n")
	return nil
}
