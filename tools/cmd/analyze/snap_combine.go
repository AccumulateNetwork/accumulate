// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// snap-combine command
//
// This command merges multiple snapshot files into a single consolidated snapshot file.
// Usage: snap-combine <output snapshot> <input snapshot> <input snapshot> ...
//
// Implementation Strategy:
// 1. Create a temporary directory for working files and database
// 2. Read all input snapshots into a temporary database
// 3. Track account URLs and metadata in temporary files to minimize memory usage
// 4. Process records by section type and write to separate temporary files
// 5. Combine all temporary section files into the final output snapshot
// 6. Clean up temporary files and database when complete
//
// Memory Optimization:
// - Use temporary files instead of in-memory structures for large datasets
// - Stream records from database to output files without loading everything into memory
// - Process snapshots sequentially to maintain constant memory usage regardless of input size

// Path:  /home/paul/work/acc1/
// Files: bvn0.snap bvn1.snap bvn2.snap dn.snap

package main

import (
	"fmt"
	"os"

	blockchainDB "github.com/AccumulateNetwork/BlockchainDB/database"
	"github.com/spf13/cobra"
)

// RecordKey represents a key for a record in the database
// This struct is used to track records as they are processed from input snapshots
type RecordKey struct {
	Hash       [32]byte // SHA-256 hash of the key path, used as the database key
	KeyPath    string   // Original key path from the snapshot (e.g., "Account/acc://example/")
	RecordType string   // Type of record (first part of the key, e.g., "Account", "Transaction")
	AccountURL string   // Account URL if applicable (e.g., "acc://example/")
	ChainID    string   // Chain ID if applicable (e.g., "main", "scratch")
}

// SnapCombine represents the state for the snapshot combine operation
// This struct tracks all the state needed during the snapshot combining process
type SnapCombine struct {
	// Input and output paths
	OutputPath string   // Path where the combined output snapshot will be written
	InputPaths []string // List of input snapshot paths to be combined

	// Database information
	dbPath string           // Path to the temporary database directory
	db     *blockchainDB.KV2 // Temporary database for storing all records from input snapshots

	// Record tracking - only used for testing and small datasets
	RecordKeys     []RecordKey       // All record keys in order (for testing only)
	RecordsByType  map[string][]int  // Map of record type to indices in RecordKeys
	AccountRecords map[string][]int  // Map of account URL to indices in RecordKeys
	
	// URL hash mapping - for efficient URL storage and retrieval
	urlHashFile     *os.File            // File for storing URL hash mappings
	urlHashFilePath string              // Path to the URL hash mapping file
	urlHashMap      map[[32]byte]string // In-memory URL hash map (only used when useMemory is true)
	
	// Temporary files for large datasets
	keysFile     *os.File // File handle for storing record keys to reduce memory usage
	keysFilePath string   // Path to the temporary keys file
	useMemory    bool     // Whether to use in-memory storage (for testing) or file storage

	// Statistics
	RecordsRead    int            // Total number of records read from all input snapshots
	RecordsWritten int            // Total number of records written to the output snapshot
	SnapshotsRead  int            // Number of snapshots successfully processed
	RecordTypes    map[string]int // Map of record type to count for statistics

	// Temporary section files
	sectionFiles map[string]*os.File // Map of section type to temporary file for that section
	sectionPaths map[string]string   // Map of section type to file path
}

var cmdAnalyzeSnapCombine = &cobra.Command{
	Use:   "snap-combine [output-snapshot-path] [input-snapshot-paths...]",
	Short: "Combines multiple snapshots into a single snapshot file",
	Long: `Combines multiple snapshots into a single snapshot file with optimized memory usage.

Usage: snap-combine <output snapshot> <input snapshot> <input snapshot> ...

This command:
1. Creates a temporary directory for working files and database
2. Reads all input snapshots into the temporary database
3. Tracks account URLs and metadata in temporary files to minimize memory usage
4. Processes records by section type and writes to separate temporary files
5. Combines all temporary section files into the final output snapshot
6. Cleans up temporary files and database when complete

The command processes all snapshots and creates a consolidated view without
fabricating any data, ensuring accurate representation of the combined state.

Memory Optimization:
- Uses temporary files instead of in-memory structures for large datasets
- Streams records from database to output files without loading everything into memory
- Processes snapshots sequentially to maintain constant memory usage regardless of input size`,
	Args: cobra.MinimumNArgs(2), // At least output path and one input path
	RunE: combineSnapshots,
}

// combineSnapshots reads multiple snapshot files into a database and creates a combined snapshot
// This is the main entry point for the snap-combine command
func combineSnapshots(cmd *cobra.Command, args []string) error {
	// Step 1: Create a new SnapCombine instance with the output and input paths
	combiner := &SnapCombine{
		OutputPath:     args[0],                 // First argument is the output path
		InputPaths:     args[1:],                // Remaining arguments are input paths
		RecordTypes:    make(map[string]int),    // Initialize statistics tracking
		RecordsByType:  make(map[string][]int),  // Initialize record type index
		AccountRecords: make(map[string][]int),  // Initialize account records index
		sectionFiles:   make(map[string]*os.File), // Initialize section files map
		sectionPaths:   make(map[string]string),   // Initialize section paths map
		useMemory:      false,                   // Default to file-based storage for production use
	}

	// Step 2: Initialize the combiner - creates temp directory, database, and files
	fmt.Println("Initializing snapshot combiner...")
	err := combiner.Initialize()
	if err != nil {
		return fmt.Errorf("failed to initialize combiner: %w", err)
	}
	// Ensure cleanup of temporary resources when done
	defer combiner.Cleanup()

	// Step 3: Process each input snapshot and load into the database
	fmt.Printf("Processing %d input snapshots...\n", len(combiner.InputPaths))
	for i, inputPath := range combiner.InputPaths {
		fmt.Printf("Reading snapshot %d/%d: %s\n", i+1, len(combiner.InputPaths), inputPath)
		// ReadSnapshot loads all records from the snapshot into the database
		// and tracks account URLs in the temporary file
		err := combiner.ReadSnapshot(inputPath)
		if err != nil {
			return fmt.Errorf("failed to read snapshot %s: %w", inputPath, err)
		}
	}

	// Step 4: Write the combined snapshot from the database to the output file
	fmt.Printf("Writing combined snapshot to %s...\n", combiner.OutputPath)
	// WriteSnapshot reads records from the database and writes them to the output file
	// It uses the temporary files to minimize memory usage
	err = combiner.WriteSnapshot(combiner.OutputPath)
	if err != nil {
		return fmt.Errorf("failed to write combined snapshot: %w", err)
	}

	// Step 5: Print statistics about the operation
	combiner.PrintStatistics()

	return nil
}

// Initialize prepares the SnapCombine instance for operation by setting up all necessary 
// temporary resources for processing snapshots with minimal memory usage
func (sc *SnapCombine) Initialize() error {
	// Step 1: Create a temporary directory for all working files and database
	// This directory will hold the database files and all temporary files
	tempDir, err := os.MkdirTemp("", "acc-snapshot-combine-")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	sc.dbPath = tempDir

	// Step 2: Initialize the BlockchainDB database in the temporary directory
	// This database will store all records from all input snapshots
	// Parameters: path, cache size, max file size, max open files
	db, err := blockchainDB.NewKV2(tempDir, 1024, 1024*1024, 100)
	if err != nil {
		// Clean up the directory if database creation fails
		os.RemoveAll(tempDir)
		return fmt.Errorf("failed to open database: %w", err)
	}
	sc.db = db
	
	// Step 3: Create a temporary file for storing record keys
	// This file will contain all record keys and metadata to avoid keeping them in memory
	// Format: hashHex,keyPath,recordType,accountURL,chainID
	keysFile, err := os.CreateTemp(tempDir, "record-keys-")
	if err != nil {
		// Clean up if file creation fails
		sc.db.Close()
		os.RemoveAll(tempDir)
		return fmt.Errorf("failed to create temp file for keys: %w", err)
	}
	sc.keysFile = keysFile
	sc.keysFilePath = keysFile.Name()
	
	// Step 4: Create a temporary file for URL hash mapping
	// This file will store mappings between URL hashes and original URLs
	// Format: hashHex,url
	urlHashFile, err := os.CreateTemp(tempDir, "url-hashes-")
	if err != nil {
		// Clean up if file creation fails
		sc.keysFile.Close()
		sc.db.Close()
		os.RemoveAll(tempDir)
		return fmt.Errorf("failed to create temp file for URL hashes: %w", err)
	}
	sc.urlHashFile = urlHashFile
	sc.urlHashFilePath = urlHashFile.Name()
	
	// Step 5: Initialize section files map for different section types
	// Each section type will have its own temporary file
	sc.sectionFiles = make(map[string]*os.File)
	sc.sectionPaths = make(map[string]string)
	
	// Step 6: Configure memory usage strategy
	// Use file storage for large datasets, memory for tests
	sc.useMemory = false // Default to file storage for production, true for tests

	// Step 7: Initialize record tracking maps - used primarily for testing
	// These are only populated when useMemory is true
	sc.RecordKeys = make([]RecordKey, 0, 100) // Small initial capacity for tests
	sc.RecordsByType = make(map[string][]int)
	sc.AccountRecords = make(map[string][]int)
	
	// Initialize URL hash map if using memory
	if sc.useMemory {
		sc.urlHashMap = make(map[[32]byte]string)
	}

	// Step 8: Initialize record type statistics
	if sc.RecordTypes == nil {
		sc.RecordTypes = make(map[string]int)
	}

	fmt.Printf("Created temporary database at %s\n", sc.dbPath)
	return nil
}

// Cleanup releases all resources used by the SnapCombine instance
// This includes closing and removing all temporary files and the database
func (sc *SnapCombine) Cleanup() error {
	// Step 1: Close the keys file if it's open
	if sc.keysFile != nil {
		err := sc.keysFile.Close()
		if err != nil {
			fmt.Printf("Warning: failed to close keys file: %v\n", err)
		}
		sc.keysFile = nil
	}

	// Step 2: Close the URL hash file if it's open
	if sc.urlHashFile != nil {
		err := sc.urlHashFile.Close()
		if err != nil {
			fmt.Printf("Warning: failed to close URL hash file: %v\n", err)
		}
		sc.urlHashFile = nil
	}

	// Step 3: Close all section files
	for sectionType, file := range sc.sectionFiles {
		if file != nil {
			err := file.Close()
			if err != nil {
				fmt.Printf("Warning: failed to close section file for %s: %v\n", sectionType, err)
			}
		}
	}
	sc.sectionFiles = nil
	sc.sectionPaths = nil

	// Step 4: Close the database if it's open
	if sc.db != nil {
		err := sc.db.Close()
		if err != nil {
			fmt.Printf("Warning: failed to close database: %v\n", err)
		}
		sc.db = nil
	}

	// Step 5: Remove the temporary directory (which includes all temporary files)
	// This will clean up all files created in the temporary directory
	if sc.dbPath != "" {
		err := os.RemoveAll(sc.dbPath)
		if err != nil {
			fmt.Printf("Warning: failed to remove temp directory: %v\n", err)
		} else {
			fmt.Printf("Removed temporary directory at %s\n", sc.dbPath)
		}
		sc.dbPath = ""
		sc.keysFilePath = ""
		sc.urlHashFilePath = ""
	}

	// Step 6: Clear any remaining in-memory data structures
	sc.RecordKeys = nil
	sc.RecordsByType = nil
	sc.AccountRecords = nil
	sc.urlHashMap = nil
	
	return nil
}

// ReadSnapshot and WriteSnapshot functions are defined in snap_read.go and snap_write.go

// PrintStatistics prints statistics about the combine operation
func (sc *SnapCombine) PrintStatistics() {
	fmt.Println("\n=== Snapshot Combine Statistics ===")
	fmt.Printf("Input snapshots processed: %d\n", sc.SnapshotsRead)
	fmt.Printf("Records read: %d\n", sc.RecordsRead)
	fmt.Printf("Records written: %d\n", sc.RecordsWritten)
	
	// Print record type statistics if available
	if len(sc.RecordTypes) > 0 {
		fmt.Println("\nRecord types:")
		printSortedStats(sc.RecordTypes)
	}
}

