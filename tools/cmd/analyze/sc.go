// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// The following Rules and directives:
// - using the sc_ prefix for functions, structs, and constants used in sc
// - Keep files short (less than 300 lines)
// - Keep methods short; factor into helper functions and more methods to keep things short
// - Keep methods independent enough to easy test
// - Keep logging and printing out of long inter loops
// - Keep summaries like counts of record types or files processed, error types, etc. in the sc_state
// - At the end of execution produce a report using the summaries collected
//
// sc implements the 'sc' command for the analyze tool.
//
// The sc command is a streaming version of the snap-combine tool.
// It reads snapshot files and streams the sections to temporary files,
// maintaining state and summaries in memory.
//
// This approach is more memory-efficient than the original snap-combine
// tool, which loads entire sections into memory.
//
// IMPORTANT NOTE: The snap_combine code is BROKEN and should NOT be used as a pattern
// or reference for this implementation. This implementation must correctly parse
// the snapshot format directly based on the documented binary format.
//
// Snapshot ext:  .snap
//
// IMPORTANT FILE PATHS AND DIRECTORIES:
// ===================================
// - Test snapshot files location:     ~/work/acc1/
//   (Example: ~/work/acc1/dn.snap)
// - Output files directory:           ~/work/acc1/output/
// - Compile analyze binary to:        ~/work/acc1/
// - Run analyze command from:         ~/work/acc1/
//
// EXAMPLE USAGE:
// cd ~/work/acc1
//
// Process a single snapshot (automatically validates reconstruction accuracy):
// ./analyze sc combined.snap dn.snap
//
// Combine multiple snapshots:
// ./analyze sc combined.snap dn.snap bvn0.snap bvn1.snap bvn2.snap

package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// sc_State holds state for the sc command
// SectionInfo tracks detailed information about a section during reconstruction
type SectionInfo struct {
	Type         uint32
	StartOffset  int64  // Offset of the section in the file
	HeaderOffset int64  // Offset of the section header
	DataOffset   int64  // Offset of the section data
	Size         uint64 // Size of the section data
	EndOffset    int64  // End offset of the section
	Order        int    // Original order in the snapshot file
	Instance     int    // Instance number for sections with the same type
}

type sc_State struct {
	// File paths and handles
	SnapshotPath string              // Path to the input snapshot file
	InputFiles   []*os.File          // List of input snapshot files
	OutFile      *os.File            // The destination file
	TempDir      string              // Directory for temporary files
	SectionFiles map[string]*os.File // Map of section type and instance to temporary file

	// Snapshot metadata
	FormatVersion uint32 // Detected snapshot format version

	// Track original section information during parsing
	OriginalSections   []SectionInfo
	FirstSectionOffset uint64 // Offset to the first section in the original file

	// Reconstruction tracking
	ReconstructionInfo []SectionInfo // Information about reconstructed sections

	// Record type tracking
	AccountRecordCount int // Count of account records
	MessageRecordCount int // Count of message records
	OtherRecordCount   int // Count of other record types

	// Summary statistics
	SectionCounts  map[uint32]int   // Count of records by section type
	SectionSizes   map[uint32]int64 // Size of each section in bytes
	TotalRecords   int              // Total number of records processed
	TotalSections  int              // Total number of sections found
	ErrorCounts    map[string]int   // Count of errors by type
	StartTime      time.Time        // When processing started
	ProcessingTime time.Duration    // Total processing time
}

// Init initializes the sc_State by opening the snapshot file
func (s *sc_State) Init(snapshotPath string) error {
	// Open the snapshot file
	file, err := os.Open(snapshotPath)
	if err != nil {
		return fmt.Errorf("snapshot file not found: %w", err)
	}

	// Store the file handle
	s.InputFiles = append(s.InputFiles, file)

	// Create a temporary directory for section files
	tempDir, err := os.MkdirTemp("", "sc-sections-*")
	if err != nil {
		// Close the file we just opened
		file.Close()
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	s.TempDir = tempDir

	// Initialize maps and slices
	s.SectionFiles = make(map[string]*os.File)
	s.SectionCounts = make(map[uint32]int)
	s.SectionSizes = make(map[uint32]int64)
	s.ErrorCounts = make(map[string]int)
	s.OriginalSections = make([]SectionInfo, 0)

	// Create a special header section file (section 1) with format version
	// This ensures we always have a header section, even if parsing fails
	headerFileName := filepath.Join(s.TempDir, "section_1_1.tmp")
	headerFile, err := os.Create(headerFileName)
	if err != nil {
		return fmt.Errorf("failed to create header section file: %w", err)
	}

	// Store the file handle in the map
	s.SectionFiles["1_1"] = headerFile

	// Initialize with format version 2
	formatVersionBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(formatVersionBytes, 2) // Use format version 2
	_, err = headerFile.Write(formatVersionBytes)
	if err != nil {
		return fmt.Errorf("failed to write format version to header file: %w", err)
	}

	// Flush the file to ensure the data is written
	err = headerFile.Sync()
	if err != nil {
		return fmt.Errorf("failed to sync header file: %w", err)
	}

	// Set the format version in the state
	s.FormatVersion = 2

	// Debug: Check the file size
	fileInfo, err := headerFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to get header file info: %w", err)
	}
	fmt.Printf("Initialized header section file with format version 2 (file size: %d bytes)\n", fileInfo.Size())

	// Record start time
	s.StartTime = time.Now()

	return nil
}

// Command for the sc functionality
var sc_Cmd = &cobra.Command{
	Use:   "sc <destination-snapshot> <input-snapshot-1> [input-snapshot-2...]",
	Short: "Process and combine snapshot files",
	Long: `Process and combine multiple snapshot files into a destination snapshot.

When processing a single input snapshot, the command will automatically validate
that the reconstructed snapshot matches the original byte-for-byte.

When processing multiple input snapshots, the command will combine them into a
single destination snapshot, maintaining the separation of account and message
records in type 7 sections.`,
	Args: cobra.MinimumNArgs(2), // At least destination and one input snapshot required
	RunE: sc_Run,
}

func init() {
	// Register the sc command with the root command
	rootCmd.AddCommand(sc_Cmd)
}

// Cleanup closes all open files and removes temporary files
func (s *sc_State) Cleanup() {
	// Close all input files if they're open
	for _, file := range s.InputFiles {
		if file != nil {
			file.Close()
		}
	}
	// Clear the slice
	s.InputFiles = nil

	// Close all section files
	for _, file := range s.SectionFiles {
		if file != nil {
			file.Close()
		}
	}

	// Clear the section files map
	s.SectionFiles = nil

	// Remove the temporary directory and all its contents
	if s.TempDir != "" {
		os.RemoveAll(s.TempDir)
		s.TempDir = ""
	}
}

// sc_GenerateReport generates a summary report of the processing
func (s *sc_State) sc_GenerateReport() {
	// Calculate processing time
	s.ProcessingTime = time.Since(s.StartTime)

	// Print report header
	fmt.Println("\n===== Snapshot Processing Report =====")
	// Print information about all input files
	fmt.Printf("Input files: %d\n", len(s.InputFiles))
	for i, file := range s.InputFiles {
		fmt.Printf("  [%d] %s\n", i+1, file.Name())
	}
	fmt.Printf("Processing time: %v\n", s.ProcessingTime)

	// Print section statistics
	fmt.Println("\nSection Statistics:")
	fmt.Printf("Total sections: %d\n", s.TotalSections)

	// Print section details
	if len(s.SectionCounts) > 0 {
		fmt.Println("Section details:")
		for sectionType, count := range s.SectionCounts {
			sectionSize := s.SectionSizes[sectionType]
			fmt.Printf("  Section type %d: %d records, %d bytes\n", sectionType, count, sectionSize)
		}
	}

	// Print record statistics
	fmt.Printf("\nTotal records: %d\n", s.TotalRecords)

	// Print error statistics if any
	if len(s.ErrorCounts) > 0 {
		fmt.Println("\nError Statistics:")
		for errorType, count := range s.ErrorCounts {
			fmt.Printf("  %s: %d occurrences\n", errorType, count)
		}
	}

	fmt.Println("====================================")
}

// sc_Run is the main entry point for the sc command
// Function variables for reconstruction and validation
// These will be replaced by the actual implementations in sc_reconstruct.go
var sc_ReconstructSnapshot = func(scState *sc_State, outputPath string) error {
	// This is just a stub that will be replaced
	fmt.Printf("Reconstructing snapshot to %s (stub)\n", outputPath)
	return nil
}

var sc_ValidateReconstruction = func(originalPath, reconstructedPath string) (bool, error) {
	// This is just a stub that will be replaced
	fmt.Printf("Validating reconstruction: %s vs %s (stub)\n", originalPath, reconstructedPath)
	return true, nil
}

func sc_Run(cmd *cobra.Command, args []string) error {
	// Create a new sc_State for this snapshot
	scState := new(sc_State)

	// The first argument is always the destination snapshot
	destinationPath := args[0]

	// The remaining arguments are input snapshots
	inputSnapshots := args[1:]

	// Process all input snapshots and combine them
	fmt.Printf("Processing %d input snapshots to create: %s\n", len(inputSnapshots), destinationPath)

	// Create a combined state to hold all snapshot data
	scState.SectionCounts = make(map[uint32]int)
	scState.SectionSizes = make(map[uint32]int64)
	scState.ErrorCounts = make(map[string]int)
	scState.SectionFiles = make(map[string]*os.File)
	scState.OriginalSections = make([]SectionInfo, 0)

	// Create a temporary directory for the combined state
	tmpDir, err := os.MkdirTemp("", "sc_combine_")
	if err != nil {
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	scState.TempDir = tmpDir

	// Ensure cleanup of temporary files happens when we're done
	defer scState.Cleanup()

	// Process each input snapshot file
	for i, snapshotPath := range inputSnapshots {
		fmt.Printf("\nProcessing snapshot %d/%d: %s\n", i+1, len(inputSnapshots), snapshotPath)

		// Initialize the state with the snapshot path
		err := scState.Init(snapshotPath)
		if err != nil {
			fmt.Printf("Error initializing state for %s: %v\n", snapshotPath, err)
			continue
		}

		// Parse the snapshot file
		err = sc_ParseSnapshot(scState)
		if err != nil {
			fmt.Printf("Error parsing %s: %v\n", snapshotPath, err)
			scState.Cleanup()
			continue
		}

		// Open a new file handle that will remain open for reconstruction
		originalFile, err := os.Open(snapshotPath)
		if err != nil {
			fmt.Printf("Error opening file for reconstruction: %v\n", err)
			continue
		}

		// Add this file to the combined state
		// This is needed for reconstruction and validation
		scState.InputFiles = append(scState.InputFiles, originalFile)

		// Don't close this file handle yet - it will be needed for reconstruction
		// We'll defer its closing after reconstruction
		if len(inputSnapshots) == 1 {
			scState.FirstSectionOffset = scState.FirstSectionOffset
		}

		// Append the original sections from this state to the combined state
		originalLen := len(scState.OriginalSections)
		newSections := make([]SectionInfo, originalLen+len(scState.OriginalSections))
		copy(newSections, scState.OriginalSections)
		copy(newSections[originalLen:], scState.OriginalSections)
		scState.OriginalSections = newSections
		fmt.Printf("Copied %d sections from original state to combined state (total: %d)\n",
			len(scState.OriginalSections), len(scState.OriginalSections))

		// For single snapshot processing, copy all section files to the combined state
		// This ensures all sections are available during reconstruction
		for key, file := range scState.SectionFiles {
			// Seek to the beginning of the file to ensure it's ready for reading during reconstruction
			_, err := file.Seek(0, io.SeekStart)
			if err != nil {
				fmt.Printf("Warning: Failed to seek to beginning of section file %s: %v\n", key, err)
			}
			scState.SectionFiles[key] = file
		}
	}

	// Merge the section counts and sizes into the combined state
	for sectionType, count := range scState.SectionCounts {
		scState.SectionCounts[sectionType] += count
	}

	for sectionType, size := range scState.SectionSizes {
		scState.SectionSizes[sectionType] += size
	}

	// Copy the section files from this state to the combined state
	// For type 7 sections (records), we need to append to the existing files
	for key, file := range scState.SectionFiles {
		// Reset file position to beginning
		_, err := file.Seek(0, io.SeekStart)
		if err != nil {
			fmt.Printf("Error seeking to beginning of file: %v\n", err)
			continue
		}

		// Check if this is a type 7 section
		if strings.HasPrefix(key, "7_") {
			// For type 7 sections, append to the existing file
			var targetFile *os.File
			var exists bool

			// Determine if this is an account or message section
			if key == "7_1" { // Accounts
				targetFile, exists = scState.SectionFiles["7_1"]
				if !exists {
					// Create a new file for accounts
					targetFile, err = sc_getOrCreateSectionFile(scState, 7, 1)
					if err != nil {
						fmt.Printf("Error creating account file: %v\n", err)
						continue
					}
				}
			} else if key == "7_2" { // Messages
				targetFile, exists = scState.SectionFiles["7_2"]
				if !exists {
					// Create a new file for messages
					targetFile, err = sc_getOrCreateSectionFile(scState, 7, 2)
					if err != nil {
						fmt.Printf("Error creating message file: %v\n", err)
						continue
					}
				}
			} else {
				// Other type 7 sections
				targetFile, exists = scState.SectionFiles[key]
				if !exists {
					// Extract instance number from key
					parts := strings.Split(key, "_")
					if len(parts) != 2 {
						fmt.Printf("Invalid section key: %s\n", key)
						continue
					}

					instance := 0
					_, err := fmt.Sscanf(parts[1], "%d", &instance)
					if err != nil {
						fmt.Printf("Error parsing instance number: %v\n", err)
						continue
					}

					targetFile, err = sc_getOrCreateSectionFile(scState, 7, instance)
					if err != nil {
						fmt.Printf("Error creating section file: %v\n", err)
						continue
					}
				}
			}

			// Copy the contents of the file to the target file
			_, err = io.Copy(targetFile, file)
			if err != nil {
				fmt.Printf("Error copying file contents: %v\n", err)
				continue
			}
		} else {
			// For non-type 7 sections, just copy the file if it doesn't exist
			if _, exists := scState.SectionFiles[key]; !exists {
				// Create a new file in the combined state
				targetFile, err := os.Create(filepath.Join(scState.TempDir, filepath.Base(file.Name())))
				if err != nil {
					fmt.Printf("Error creating file: %v\n", err)
					continue
				}

				// Copy the contents
				_, err = io.Copy(targetFile, file)
				if err != nil {
					fmt.Printf("Error copying file contents: %v\n", err)
					targetFile.Close()
					continue
				}

				// Store the file in the combined state
				scState.SectionFiles[key] = targetFile
			}
		}
	}

	// Now reconstruct the combined snapshot
	fmt.Printf("\nCreating combined snapshot: %s\n", destinationPath)

	// Debug information about sections available for reconstruction
	fmt.Printf("\nDebug: Sections available for reconstruction:\n")
	fmt.Printf("Total sections in combinedState.OriginalSections: %d\n", len(scState.OriginalSections))
	for i, section := range scState.OriginalSections {
		fmt.Printf("  Section %d: Type=%d, Instance=%d, StartOffset=%d, Size=%d\n",
			i, section.Type, section.Instance, section.StartOffset, section.Size)
	}

	// Debug information about section files
	fmt.Printf("\n=== TEMPORARY FILES ANALYSIS ===\n")
	fmt.Printf("Number of temporary files: %d\n", len(scState.SectionFiles))
	var totalTmpFileSize int64
	for key, file := range scState.SectionFiles {
		// Get file size
		stat, err := file.Stat()
		if err != nil {
			fmt.Printf("  Section file: %s (error getting size: %v)\n", key, err)
			continue
		}
		fileSize := stat.Size()
		totalTmpFileSize += fileSize
		fmt.Printf("  Section file: %s (size: %d bytes)\n", key, fileSize)
	}
	fmt.Printf("Total size of all temporary files: %d bytes\n", totalTmpFileSize)
	fmt.Printf("=== END OF TEMPORARY FILES ANALYSIS ===\n")

	// Reconstruct the snapshot
	fmt.Printf("\nReconstructing snapshot...\n")
	err = sc_ReconstructSnapshot(scState, destinationPath)
	if err != nil {
		return fmt.Errorf("failed to reconstruct snapshot: %w", err)
	}

	// If we're processing a single snapshot, validate the reconstruction
	if len(inputSnapshots) == 1 {
		fmt.Printf("\nValidating reconstruction...\n")
		matches, err := sc_ValidateReconstruction(inputSnapshots[0], destinationPath)
		if err != nil {
			return fmt.Errorf("failed to validate reconstruction: %w", err)
		}
		if !matches {
			return fmt.Errorf("reconstruction validation failed: files do not match")
		}
		fmt.Printf("Validation successful: reconstructed snapshot matches original\n")
	}

	if err := sc_Cleanup(scState); err != nil {
		return fmt.Errorf("failed to cleanup: %w", err)
	}

	fmt.Printf("\nSnapshot processing completed successfully\n")
	return nil
}
