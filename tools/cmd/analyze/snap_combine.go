// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// snap-combine command
//
// This command merges multiple snapshots into a single consolidated snapshot file.
// Usage: snap-combine <output snapshot> <input snapshot> <input snapshot> ...
//
// Implementation Strategy:
// 1. Process snapshots in batches to minimize memory usage
// 2. Use file-backed bucket sorting to efficiently merge records
// 3. Sort records in descending key hash order as required by the snapshot format
// 4. Maintain proper record indexing for efficient lookups
// 5. Skip BPT section during combining (can be regenerated if needed)
//
// Memory Optimization:
// - Process accounts in batches (default 10,000) to limit memory usage
// - Use 256 bucket files based on first byte of key hash for efficient sorting
// - Only load one bucket at a time into memory for sorting
// - Write sorted records directly to output without keeping them in memory

package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// Default configuration values
var defaultConfig = SnapCombineConfig{
	BatchSize:  10000, // Default batch size of 10,000 accounts
	NumBuckets: 256,   // Default 256 buckets (one for each possible first byte)
	Verbose:    false, // Default to minimal output
}

// Command for the snap-combine functionality
var cmdAnalyzeSnapCombine = &cobra.Command{
	Use:   "snap-combine [output-snapshot-path] [input-snapshot-paths...]",
	Short: "Combines multiple snapshots into a single snapshot file",
	Long: `Combines multiple snapshots into a single snapshot file with memory-efficient bucket sorting.

Usage: snap-combine <output snapshot> <input snapshot> <input snapshot> ...

This command:
1. Processes accounts in batches to minimize memory usage
2. Uses file-backed bucket sorting for efficient merging of records
3. Sorts records in descending key hash order as required by the snapshot format
4. Maintains proper record indexing for efficient lookups
5. Skips BPT section during combining (can be regenerated if needed)

Memory Optimization:
- Processes accounts in batches (default 10,000) to limit memory usage
- Uses 256 bucket files based on first byte of key hash for efficient sorting
- Only loads one bucket at a time into memory for sorting
- Writes sorted records directly to output without keeping them in memory`,
	Args: cobra.MinimumNArgs(2), // At least output path and one input path
	RunE: combineSnapshots,
}

// Command flags
var config SnapCombineConfig

func init() {
	// Add command-specific flags
	cmdAnalyzeSnapCombine.Flags().IntVar(&config.BatchSize, "batch-size", defaultConfig.BatchSize, 
		"Number of accounts to process in each batch")
	cmdAnalyzeSnapCombine.Flags().IntVar(&config.NumBuckets, "num-buckets", defaultConfig.NumBuckets, 
		"Number of bucket files to use for sorting")
	cmdAnalyzeSnapCombine.Flags().BoolVar(&config.Verbose, "verbose", defaultConfig.Verbose, 
		"Show detailed progress information")
}

// combineSnapshots combines multiple snapshots into a single snapshot using a memory-efficient bucket sort
// This is the main entry point for the snap-combine command
func combineSnapshots(cmd *cobra.Command, args []string) error {
	// Parse command line arguments
	outputPath := args[0]    // First argument is the output path
	inputPaths := args[1:]   // Remaining arguments are input paths

	// Validate input parameters
	if len(inputPaths) == 0 {
		return fmt.Errorf("no input snapshots provided")
	}
	
	// Special case: if only one snapshot, just copy it
	if len(inputPaths) == 1 {
		fmt.Printf("Only one snapshot provided, copying %s to %s\n", inputPaths[0], outputPath)
		return copySingleSnapshot(inputPaths[0], outputPath)
	}
	
	// Log the start of the operation
	fmt.Printf("Combining %d snapshots into %s\n", len(inputPaths), outputPath)
	fmt.Printf("Using batch size: %d, bucket count: %d\n", config.BatchSize, config.NumBuckets)
	
	// Create a SnapCombiner instance with the provided configuration
	combiner := &SnapCombiner{
		Config: config,
		InputPaths: inputPaths,
		OutputPath: outputPath,
		Stats: CombineStats{
			InputSnapshots: len(inputPaths),
			RecordsByType: make(map[string]int),
		},
	}
	
	// Execute the snapshot combining algorithm
	err := combiner.Execute()
	if err != nil {
		return fmt.Errorf("failed to combine snapshots: %w", err)
	}
	
	// Print statistics
	combiner.PrintStats()
	
	return nil
}

// copySingleSnapshot copies a single snapshot file to the output path
// This is a special case handler when only one input snapshot is provided
func copySingleSnapshot(inputPath, outputPath string) error {
	// Open the input file for reading
	inputFile, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open input snapshot %s: %w", inputPath, err)
	}
	defer inputFile.Close()
	
	// Create the output file for writing
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output snapshot %s: %w", outputPath, err)
	}
	defer outputFile.Close()
	
	// Copy the contents
	_, err = io.Copy(outputFile, inputFile)
	if err != nil {
		return fmt.Errorf("failed to copy snapshot: %w", err)
	}
	
	fmt.Printf("Successfully copied snapshot to %s\n", outputPath)
	return nil
}



