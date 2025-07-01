// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// Package main implements snapshot extraction tools.
// IMPORTANT: This implementation uses a streaming architecture to process snapshots
// efficiently without loading the entire database into memory. The goal is to process
// a 2GB snapshot using less than 5GB of memory (compared to previous implementations
// that required 40GB for a 2GB snapshot).
package main

import (
	"fmt"
	"os"
)

// AccountRecord represents an account record from the snapshot
type AccountRecord struct {
	Key       []byte // Serialized key
	Value     []byte // Value data
	URL       string // Account URL as string
	Partition string // Partition name
}

// ChainRecord represents a chain sub-record from an account
type ChainRecord struct {
	Key           []byte // Serialized key
	Value         []byte // Value data
	AccountURL    string // Account URL as string
	ChainType     string // MainChain, AnchorChain, etc.
	EntryCount    int64  // Number of entries in the chain
	FoundEntries  int64  // Number of entries found in the snapshot
	HasMerkleData bool   // Whether the chain has Merkle tree data
}

// TransactionRecord represents a transaction record from the snapshot
type TransactionRecord struct {
	Key   []byte   // Serialized key
	Value []byte   // Value data
	Hash  [32]byte // Transaction hash
}

// MessageRecord represents a message record from the snapshot
type MessageRecord struct {
	Key   []byte   // Serialized key
	Value []byte   // Value data
	Hash  [32]byte // Message hash
}

// PartitionInfo contains information about a network partition
type PartitionInfo struct {
	ID   string // Partition ID
	Type string // Partition type (e.g., "bvn", "directory")
}

// SnapshotHeader contains header information from the snapshot file
type SnapshotHeader struct {
	Version      uint64   // Snapshot format version
	RootHash     [32]byte // Root hash of the snapshot
	SystemLedger struct { // System ledger information
		URL       string // System ledger URL
		Index     uint64 // System ledger index
		Timestamp int64  // System ledger timestamp
	}
}

// ExtractState encapsulates all state for the snapshot extraction process.
// NOTE: We are NOT using accessors. All fields are exported (capitalized) for direct access.
type ExtractState struct {
	// Input parameters
	SnapshotFile string // Path to the snapshot file
	NetworkFile  string // Path to the network.json file

	// Network configuration from network.json
	NetworkConfig *NetworkConfig // Parsed network.json structure

	// Routing information
	Router interface{} // Generic interface to avoid import dependency

	// Partition information
	Partitions []PartitionInfo // List of partitions from network.json

	// Snapshot information
	SnapshotHeader *SnapshotHeader // Header information from the snapshot file

	// Collection data structures - using a streaming approach to minimize memory usage
	Accounts               []AccountRecord     // Account records from the snapshot
	Transactions           []TransactionRecord // Transaction records from the snapshot
	Messages               []MessageRecord     // Message records from the snapshot
	TransactionHashToIndex map[[32]byte]int    // Maps transaction hash to index in Transactions slice
	MessageHashToIndex     map[[32]byte]int    // Maps message hash to index in Messages slice

	// Report data
	Report *ExtractReport // Statistics and analysis results
}

// NewExtractState creates a new ExtractState with initialized fields
func NewExtractState() *ExtractState {
	return &ExtractState{
		// Initialize collections
		Accounts:               make([]AccountRecord, 0),
		Transactions:           make([]TransactionRecord, 0),
		Messages:               make([]MessageRecord, 0),
		TransactionHashToIndex: make(map[[32]byte]int),
		MessageHashToIndex:     make(map[[32]byte]int),

		// Initialize partition information
		Partitions: make([]PartitionInfo, 0),

		// Initialize report
		Report: NewExtractReport(),
	}
}

// Run executes the extraction process
func (s *ExtractState) Run() error {
	// Parse network.json file
	config, err := ParseNetworkJson(s.NetworkFile)
	if err != nil {
		return fmt.Errorf("failed to parse network.json: %w", err)
	}

	// Store network configuration in state
	s.NetworkConfig = config

	// Extract partition information from network config
	if config.Globals.Network.Partitions != nil {
		for _, partition := range config.Globals.Network.Partitions {
			partitionInfo := PartitionInfo{
				ID:   partition.ID,
				Type: partition.Type,
			}
			s.Partitions = append(s.Partitions, partitionInfo)
		}
	}

	// Print routing information
	PrintRoutingInfo(config)

	// Initialize routing
	router, err := InitializeRouting(config)
	if err != nil {
		return fmt.Errorf("failed to initialize routing: %w", err)
	}

	// Store router in state
	s.Router = router

	// Load up the transaction slice/map and message slice/map
	err = Load(s)
	if err != nil {
		return fmt.Errorf("failed to load snapshot data: %w", err)
	}

	// Write partition-specific snapshots for DN partitions
	err = s.writePartitionSnapshots()
	if err != nil {
		return fmt.Errorf("failed to write partition snapshots: %w", err)
	}

	// Print report (comes last)
	s.PrintReport()

	return nil
}

// PrintReport prints a summary of the extraction process
func (s *ExtractState) PrintReport() {
	fmt.Println("\nSnapshot Extraction Summary:")
	fmt.Printf("  Accounts processed: %d\n", len(s.Accounts))
	fmt.Printf("  Transactions collected: %d\n", len(s.Transactions))
	fmt.Printf("  Messages collected: %d\n", len(s.Messages))

	// If report is available, print detailed report
	if s.Report != nil {
		// Update report counts from our collections
		s.Report.AccountCount = int64(len(s.Accounts))
		s.Report.TransactionCount = int64(len(s.Transactions))
		s.Report.MessageCount = int64(len(s.Messages))
		s.Report.PrintReport()
	}
}

// writePartitionSnapshots writes partition-specific snapshots for DN partitions
func (s *ExtractState) writePartitionSnapshots() error {
	fmt.Println("\nWriting partition-specific snapshots...")
	
	// Create output directory if it doesn't exist
	outputDir := "/tmp/partition-snapshots"
	err := os.MkdirAll(outputDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}
	
	// Find DN partitions and write snapshots for each
	for _, partition := range s.Partitions {
		if partition.Type == "directory" {
			fmt.Printf("Writing snapshot for DN partition: %s\n", partition.ID)
			
			// Create output filename
			outputFile := fmt.Sprintf("%s/%s-partition.snap", outputDir, partition.ID)
			
			// Write the partition snapshot
			err := WritePartitionSnapshot(s, outputFile, partition.ID)
			if err != nil {
				return fmt.Errorf("failed to write snapshot for partition %s: %w", partition.ID, err)
			}
			
			fmt.Printf("Successfully wrote partition snapshot: %s\n", outputFile)
		}
	}
	
	fmt.Println("Partition snapshot writing completed.")
	return nil
}
