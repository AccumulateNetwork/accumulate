// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/accumulatenetwork/accumulate/pkg/database/snapshot"
)

// TestConsensusSection validates that a consensus section exists in a partition snapshot
// and prints its size. Note: Consensus sections are only present in partition snapshots,
// not in unified snapshots.
func TestConsensusSection(t *testing.T) {
	// Define file paths - use environment variable to find home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get user home directory: %v", err)
	}
	
	// First check if the partition snapshot directory exists
	partitionDir := filepath.Join("/tmp", "partition-snapshots")
	if _, err := os.Stat(partitionDir); os.IsNotExist(err) {
		fmt.Printf("Partition snapshot directory not found at %s\n", partitionDir)
		fmt.Println("Creating directory and generating partition snapshots first...")
		
		// Create the directory
		err = os.MkdirAll(partitionDir, 0755)
		if err != nil {
			t.Fatalf("Failed to create partition snapshot directory: %v", err)
		}
		
		// Generate partition snapshots using the existing code
		networkFile := filepath.Join(homeDir, "accumulate-network/artifacts/cyclops-network.json")
		snapshotFile := filepath.Join(homeDir, "accumulate-network/artifacts/cyclops-genesis.snap")
		
		// Create extract state for generating partition snapshots
		state := NewExtractState()
		state.SnapshotFile = snapshotFile
		state.NetworkFile = networkFile
		
		// Initialize and run extraction to generate partition snapshots
		err = state.InitializeSnapshot()
		if err != nil {
			t.Fatalf("Failed to initialize snapshot: %v", err)
		}
		
		// Generate partition snapshots
		fmt.Println("Generating partition snapshots...")
		err = state.writePartitionSnapshots()
		if err != nil {
			t.Fatalf("Failed to generate partition snapshots: %v", err)
		}
		
		state.Close()
	}
	
	// Find a partition snapshot file
	files, err := os.ReadDir(partitionDir)
	if err != nil {
		t.Fatalf("Failed to read partition snapshot directory: %v", err)
	}
	
	if len(files) == 0 {
		t.Fatalf("No partition snapshot files found in %s", partitionDir)
	}
	
	// Use the first partition snapshot file found
	partitionFile := filepath.Join(partitionDir, files[0].Name())
	fmt.Printf("Testing consensus section in partition snapshot: %s\n", partitionFile)
	
	// Create extract state for the partition snapshot
	state := NewExtractState()
	state.SnapshotFile = partitionFile
	
	// Initialize snapshot
	err = state.InitializeSnapshot()
	if err != nil {
		t.Fatalf("Failed to initialize partition snapshot: %v", err)
	}
	defer state.Close()

	// Check for consensus section
	foundConsensus := false
	var consensusSectionIndex int
	var consensusSectionSize int64

	for i, section := range state.SnapshotReader.Sections {
		fmt.Printf("Section %d: Type %d\n", i, section.Type())
		if section.Type() == snapshot.SectionTypeConsensus {
			foundConsensus = true
			consensusSectionIndex = i
			consensusSectionSize = section.Size()
			break
		}
	}

	// Fail the test if no consensus section is found
	if !foundConsensus {
		fmt.Println("No consensus section found in partition snapshot")
		t.Fail()
		return
	}

	// Print consensus section details using fmt
	fmt.Printf("\nConsensus section validation successful!\n")
	fmt.Printf("Found consensus section at index %d\n", consensusSectionIndex)
	fmt.Printf("Consensus section size: %d bytes (%.2f KB, %.2f MB)\n",
		consensusSectionSize,
		float64(consensusSectionSize)/1024,
		float64(consensusSectionSize)/(1024*1024))

	// Print additional information about the section
	sectionTypeName := getSectionTypeNameForAnalysis(snapshot.SectionTypeConsensus)
	fmt.Printf("Section type: %s (type %d)\n", sectionTypeName, int(snapshot.SectionTypeConsensus))
}
