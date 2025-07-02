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
	"strings"

	"gitlab.com/accumulatenetwork/accumulate/exp/ioutil"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
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
	Key           []byte       // Serialized key
	Value         []byte       // Value data
	AccountURL    string       // Account URL as string
	URL           string       // Chain URL as string
	ChainType     string       // MainChain, AnchorChain, etc.
	EntryCount    int64        // Number of entries in the chain
	FoundEntries  int64        // Number of entries found in the snapshot
	HasMerkleData bool         // Whether the chain has Merkle tree data
	Entries       []ChainEntry // Chain entries
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

// ChainEntry represents an entry in a chain
type ChainEntry struct {
	Hash []byte // Entry hash
}

// RecordEntry represents a unified record entry from the snapshot
// This can be an account, transaction, message, or any other record type
type RecordEntry struct {
	Key       []byte       // Serialized key
	Value     []byte       // Value data
	KeyHash   [32]byte     // Hash of the key for indexing
	Type      string       // Type of record ("account", "chain", "transaction", "message", "other")
	URL       string       // Account URL (for account records)
	Partition string       // Partition name (for account records)
	Chain     *ChainRecord // Chain record (for chain records)
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

// SnapshotSectionInfo contains information about a snapshot section
type SnapshotSectionInfo struct {
	Index       int                  // Section index in the snapshot
	Type        snapshot.SectionType // Section type (Records, BPT, etc.)
	Offset      int64                // File offset to the beginning of this section
	Size        int64                // Size of the section in bytes
	RecordCount int64                // Number of records in this section (for record sections)
	Description string               // Human-readable description of section content
}

// SectionTypeMap maps section types to their indices for quick lookup
type SectionTypeMap struct {
	BPTSections   []int // Indices of BPT sections (type 11) - to be skipped
	OtherSections []int // Indices of other sections (type 7, etc.) - messages/transactions
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
	Router routing.Router // Router for partition membership checks

	// Partition information
	Partitions []PartitionInfo // List of partitions from network.json

	// Snapshot file and section information
	SnapshotFileHandle *os.File              // Open file handle to the snapshot file
	SnapshotReader     *snapshot.Reader      // Snapshot reader for header access
	SnapshotHeader     *SnapshotHeader       // Header information from the snapshot file
	Sections           []SnapshotSectionInfo // Information about each section in the snapshot
	SectionInfos       []SectionAnalysisInfo // Detailed section analysis with sizes and types
	SectionTypeMap     *SectionTypeMap       // Quick lookup map for sections by type

	// Unified record collection - using a streaming approach to minimize memory usage
	Records        []RecordEntry    // All records (accounts, transactions, messages, etc.)
	KeyHashToIndex map[[32]byte]int // Maps record key hash to index in Records slice

	// Chain entry cache to avoid repeated extraction
	ChainEntryCache *ChainEntryCache // Cache of chain entries by chain URL

	// Bloom filters for partition filtering
	BloomFilters map[string]*Bloom // Maps partition ID to bloom filter (legacy)

	// Report data
	Report *ExtractReport // Statistics and analysis results
}

// NewExtractState creates a new ExtractState with initialized fields
func NewExtractState() *ExtractState {
	return &ExtractState{
		// Initialize unified record collections
		Records:        make([]RecordEntry, 0),
		KeyHashToIndex: make(map[[32]byte]int),

		// Initialize chain entry cache
		ChainEntryCache: NewChainEntryCache(),

		// Initialize partition bloom filters
		BloomFilters: make(map[string]*Bloom),

		// Initialize partition information
		Partitions: make([]PartitionInfo, 0),

		// Initialize section information
		Sections: make([]SnapshotSectionInfo, 0),
		SectionTypeMap: &SectionTypeMap{
			BPTSections:   make([]int, 0),
			OtherSections: make([]int, 0),
		},

		// Initialize report
		Report: NewExtractReport(),
	}
}

// Close closes the snapshot file handle if it's open
func (s *ExtractState) Close() error {
	if s.SnapshotFileHandle != nil {
		err := s.SnapshotFileHandle.Close()
		s.SnapshotFileHandle = nil
		s.SnapshotReader = nil
		return err
	}
	return nil
}

// InitializeSnapshot opens the snapshot file and parses section headers
func (s *ExtractState) InitializeSnapshot() error {
	// Close any existing file handle
	if err := s.Close(); err != nil {
		return fmt.Errorf("failed to close existing snapshot file: %w", err)
	}

	// Open the snapshot file
	file, err := os.Open(s.SnapshotFile)
	if err != nil {
		return fmt.Errorf("failed to open snapshot file: %w", err)
	}
	s.SnapshotFileHandle = file

	// Create snapshot reader
	reader, err := snapshot.Open(file)
	if err != nil {
		return fmt.Errorf("failed to create snapshot reader: %w", err)
	}
	s.SnapshotReader = reader

	// Store snapshot header information
	s.SnapshotHeader = &SnapshotHeader{
		Version:  reader.Header.Version,
		RootHash: reader.Header.RootHash,
	}

	// Store system ledger info if available
	if reader.Header.SystemLedger != nil {
		s.SnapshotHeader.SystemLedger.URL = reader.Header.SystemLedger.Url.String()
		s.SnapshotHeader.SystemLedger.Index = reader.Header.SystemLedger.Index
		s.SnapshotHeader.SystemLedger.Timestamp = reader.Header.SystemLedger.Timestamp.UnixNano()
	}

	fmt.Printf("Initialized snapshot file: %s\n", s.SnapshotFile)
	fmt.Printf("  Snapshot Version: %d\n", reader.Header.Version)
	fmt.Printf("  Root Hash: %x\n", reader.Header.RootHash)
	fmt.Printf("  Sections: %d\n\n", len(reader.Sections))

	// Scan and report section information
	sectionInfos, err := ScanSnapshotSections(reader)
	if err != nil {
		return fmt.Errorf("failed to scan snapshot sections: %v", err)
	}
	s.SectionInfos = sectionInfos

	// Parse section information and categorize by type
	s.Sections = make([]SnapshotSectionInfo, len(reader.Sections))

	return nil
}

// getSectionDescription returns a human-readable description for a section type
func (s *ExtractState) getSectionDescription(sectionType snapshot.SectionType) string {
	switch sectionType {
	case snapshot.SectionTypeRecords:
		return "accounts and records"
	case snapshot.SectionTypeBPT:
		return "BPT index (skip)"
	default:
		return "messages/transactions"
	}
}

// GetRecordSections returns indices of record sections (type 7)
func (s *ExtractState) GetRecordSections() []int {
	var recordSections []int
	for i, section := range s.SnapshotReader.Sections {
		if section.Type() == snapshot.SectionTypeRecords {
			recordSections = append(recordSections, i)
		}
	}
	return recordSections
}

// GetOtherSections returns indices of non-record, non-BPT sections
func (s *ExtractState) GetOtherSections() []int {
	var otherSections []int
	for i, section := range s.SnapshotReader.Sections {
		if section.Type() != snapshot.SectionTypeRecords && section.Type() != snapshot.SectionTypeBPT {
			otherSections = append(otherSections, i)
		}
	}
	return otherSections
}

// ProcessRecordSectionsOnly processes only record sections, skipping BPT and other types
func (s *ExtractState) ProcessRecordSectionsOnly(processor func(sectionIndex int, section *ioutil.Segment[snapshot.SectionType, *snapshot.SectionType]) error) error {
	if s.SnapshotReader == nil {
		return fmt.Errorf("snapshot not initialized")
	}

	for _, sectionIndex := range s.GetRecordSections() {
		if sectionIndex >= len(s.SnapshotReader.Sections) {
			continue
		}
		section := s.SnapshotReader.Sections[sectionIndex]
		if err := processor(sectionIndex, section); err != nil {
			return fmt.Errorf("error processing record section %d: %w", sectionIndex, err)
		}
	}
	return nil
}

// ProcessOtherSectionsOnly processes only non-record, non-BPT sections (messages/transactions)
func (s *ExtractState) ProcessOtherSectionsOnly(processor func(sectionIndex int, section *ioutil.Segment[snapshot.SectionType, *snapshot.SectionType]) error) error {
	if s.SnapshotReader == nil {
		return fmt.Errorf("snapshot not initialized")
	}

	for _, sectionIndex := range s.GetOtherSections() {
		if sectionIndex >= len(s.SnapshotReader.Sections) {
			continue
		}
		section := s.SnapshotReader.Sections[sectionIndex]
		if err := processor(sectionIndex, section); err != nil {
			return fmt.Errorf("error processing other section %d: %w", sectionIndex, err)
		}
	}
	return nil
}

// Run executes the extraction process
func (s *ExtractState) Run() error {
	// Ensure cleanup on exit
	defer s.Close()

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

	// Initialize snapshot file and parse section headers
	err = s.InitializeSnapshot()
	if err != nil {
		return fmt.Errorf("failed to initialize snapshot: %w", err)
	}

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

	return nil
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

	// Write snapshots for all partitions (both DN and BVN)
	for _, partition := range s.Partitions {
		// Process both directory (DN) and validator (BVN) partitions
		if strings.EqualFold(partition.Type, "directory") {
			fmt.Printf("Writing snapshot for DN partition: %s (type: %s)\n", partition.ID, partition.Type)
		} else if strings.EqualFold(partition.Type, "validator") {
			fmt.Printf("Writing snapshot for BVN partition: %s (type: %s)\n", partition.ID, partition.Type)
		} else {
			fmt.Printf("Writing snapshot for partition: %s (type: %s)\n", partition.ID, partition.Type)
		}

		// Process accounts for this partition first
		fmt.Printf("Processing accounts for partition: %s\n", partition.ID)
		stats, err := ProcessPartitionAccounts(s, partition.ID)
		if err != nil {
			return fmt.Errorf("failed to process accounts for partition %s: %w", partition.ID, err)
		}

		// Print account statistics
		fmt.Printf("Partition %s account statistics:\n", partition.ID)
		fmt.Printf("  Total accounts: %d\n", stats.TotalAccounts)
		fmt.Printf("  Total chains: %d\n", stats.TotalChains)
		for accountType, count := range stats.AccountsByType {
			fmt.Printf("  %s accounts: %d\n", accountType, count)
		}
		for chainType, count := range stats.ChainsByType {
			fmt.Printf("  %s chains: %d\n", chainType, count)
		}

		// Create output filename
		outputFile := fmt.Sprintf("%s/%s-partition.snap", outputDir, partition.ID)

		// Write the partition snapshot
		err = WritePartitionSnapshot(s, outputFile, partition.ID)
		if err != nil {
			return fmt.Errorf("failed to write snapshot for partition %s: %w", partition.ID, err)
		}
	}

	fmt.Println("Partition snapshot writing completed.")
	return nil
}
