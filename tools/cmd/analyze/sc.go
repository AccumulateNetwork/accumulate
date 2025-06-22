// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// The following Rules and directives:
// - using the sc_ prefix for functions, structs, and constants used in sc
// - Keep files short (less than 300 lines)
// - Keep methods short; factor into helper functions and more methods to keep things short
// - Keep methods independent enough to easly test
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
// ./analyze sc dn.snap

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// sc_State holds state for the sc command
// SectionInfo tracks detailed information about a section during reconstruction
type SectionInfo struct {
	Type        uint32
	StartOffset int64  // Offset of the section in the file
	HeaderOffset int64 // Offset of the section header
	DataOffset  int64  // Offset of the section data
	Size        uint64 // Size of the section data
	EndOffset   int64  // End offset of the section
	Order       int    // Original order in the snapshot file
	Instance    int    // Instance number for sections with the same type
}

type sc_State struct {
	// File paths and handles
	SnapshotPath string            // Path to the snapshot file
	File         *os.File          // File handle for the snapshot
	TempDir      string            // Directory for temporary files
	SectionFiles map[uint32]*os.File // Map of section type to temporary file
	
	// Snapshot metadata
	FormatVersion uint32          // Detected snapshot format version
	
	// Track original section information during parsing
	OriginalSections []SectionInfo
	FirstSectionOffset uint64 // Offset to the first section in the original file
	
	// Reconstruction tracking
	ReconstructionInfo []SectionInfo // Information about reconstructed sections
	
	// Summary statistics
	SectionCounts   map[uint32]int // Count of records by section type
	SectionSizes    map[uint32]int64 // Size of each section in bytes
	TotalRecords    int            // Total number of records processed
	TotalSections   int            // Total number of sections found
	ErrorCounts     map[string]int // Count of errors by type
	StartTime       time.Time      // When processing started
	ProcessingTime  time.Duration  // Total processing time
}

// Init initializes the sc_State by opening the snapshot file
func (s *sc_State) Init(snapshotPath string) error {
	// Store the snapshot path
	s.SnapshotPath = snapshotPath

	// Open the snapshot file
	file, err := os.Open(snapshotPath)
	if err != nil {
		return fmt.Errorf("snapshot file not found: %w", err)
	}

	// Store the file handle
	s.File = file
	
	// Create a temporary directory for section files
	tempDir, err := os.MkdirTemp("", "sc-sections-*")
	if err != nil {
		s.File.Close()
		return fmt.Errorf("failed to create temporary directory: %w", err)
	}
	s.TempDir = tempDir
	
	// Initialize maps
	s.SectionFiles = make(map[uint32]*os.File)
	s.SectionCounts = make(map[uint32]int)
	s.SectionSizes = make(map[uint32]int64)
	s.ErrorCounts = make(map[string]int)
	
	// Record start time
	s.StartTime = time.Now()
	
	return nil
}

// Command for the sc functionality
var sc_Cmd = &cobra.Command{
	Use:   "sc [snapshot-path...] [destination-path]",
	Short: "Process snapshot files",
	Long:  `Process snapshot files, optionally testing parsing accuracy with --test-parse.`,
	Args:  cobra.MinimumNArgs(1), // At least one snapshot path required
	RunE:  sc_Run,
}

func init() {
	// Add flags to the sc command
	sc_Cmd.Flags().Bool("test-parse", false, "Test parsing by reconstructing and comparing snapshots")
}

// Cleanup closes all open files and removes temporary files
func (s *sc_State) Cleanup() {
	// Close the snapshot file if it's open
	if s.File != nil {
		s.File.Close()
		s.File = nil
	}
	
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
	fmt.Printf("Snapshot file: %s\n", s.SnapshotPath)
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
var sc_ReconstructSnapshot = func(state *sc_State, outputPath string) error {
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
	// Check for test-parse flag
	testParse, _ := cmd.Flags().GetBool("test-parse")
	
	if testParse {
		// Test parse mode - process each input snapshot independently
		fmt.Println("Running in test-parse mode")
		
		// Process each input snapshot file
		for _, snapshotPath := range args {
			// Skip the destination path if it's the last argument and we have more than one argument
			if len(args) > 1 && snapshotPath == args[len(args)-1] {
				fmt.Println("Skipping destination path in test-parse mode:", snapshotPath)
				continue
			}
			
			fmt.Printf("Processing snapshot file: %s\n", snapshotPath)
			
			// Create a new sc_State for this snapshot
			state := &sc_State{}
			
			// Initialize the state with the snapshot path
			err := state.Init(snapshotPath)
			if err != nil {
				fmt.Printf("Error initializing state for %s: %v\n", snapshotPath, err)
				continue
			}
			
			// Ensure cleanup happens when we're done with this snapshot
			defer state.Cleanup()
			
			// Parse the snapshot file
			err = sc_ParseSnapshot(state)
			if err != nil {
				fmt.Printf("Error parsing %s: %v\n", snapshotPath, err)
				continue
			}
			
			// Generate output path for the reconstructed snapshot
			baseName := filepath.Base(snapshotPath)
			extension := filepath.Ext(baseName)
			fileNameWithoutExt := strings.TrimSuffix(baseName, extension)
			outputPath := filepath.Join("/home/paul/work/acc1/output", fileNameWithoutExt + "-parsed" + extension)
			
			// Reconstruct the snapshot
			err = sc_ReconstructSnapshot(state, outputPath)
			if err != nil {
				fmt.Printf("Error reconstructing %s: %v\n", snapshotPath, err)
				continue
			}
			
			// Validate the reconstruction
			match, err := sc_ValidateReconstruction(snapshotPath, outputPath)
			if err != nil {
				fmt.Printf("Error validating reconstruction of %s: %v\n", snapshotPath, err)
			} else if match {
				fmt.Printf("✓ Reconstruction of %s matches the original\n", snapshotPath)
			} else {
				fmt.Printf("✗ Reconstruction of %s does NOT match the original\n", snapshotPath)
			}
			
			// Generate and print the summary report
			state.sc_GenerateReport()
		}
		
		return nil
	} else {
		// Original functionality - process a single snapshot
		// Create a new sc_State
		state := &sc_State{}

		// Initialize the state with the snapshot path
		snapshotPath := args[0]
		err := state.Init(snapshotPath)
		if err != nil {
			return err
		}

		// Ensure cleanup happens when we're done
		defer state.Cleanup()
		
		// Parse the snapshot file
		err = sc_ParseSnapshot(state)
		if err != nil {
			return err
		}

		// Generate and print the summary report
		state.sc_GenerateReport()
		
		// Print success message
		fmt.Printf("Successfully parsed snapshot file: %s\n", snapshotPath)

		return nil
	}
}
