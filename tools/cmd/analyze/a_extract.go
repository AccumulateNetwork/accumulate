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
	"encoding/json"
	"fmt"
	"os"
)

// DoExtract is the main entry point for the extraction process
func DoExtract(snapshotFile, networkFile string) error {
	// Create a new ExtractState
	state := NewExtractState()
	state.SnapshotFile = snapshotFile
	state.NetworkFile = networkFile

	// Call the Run method to execute the extraction
	return state.Run()
}

// ParseNetworkJson parses the network.json file
func ParseNetworkJson(networkFile string) (*NetworkConfig, error) {
	// Read network.json file
	data, err := os.ReadFile(networkFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read network.json: %w", err)
	}

	// Parse JSON
	var config NetworkConfig
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse network.json: %w", err)
	}

	return &config, nil
}

// PrintRoutingInfo prints information about the routing configuration
func PrintRoutingInfo(config *NetworkConfig) {
	fmt.Println("Network Configuration:")
	fmt.Printf("  Partitions: %d\n", len(config.Globals.Network.Partitions))
	for i, partition := range config.Globals.Network.Partitions {
		fmt.Printf("    %d: %s (Type: %s)\n", i+1, partition.ID, partition.Type)
	}
}

// InitializeRouting initializes the routing configuration
func InitializeRouting(config *NetworkConfig) (interface{}, error) {
	// In our simplified version, we'll just return a placeholder
	// This would normally create a routing.Router and add partitions
	fmt.Println("Initializing routing with", len(config.Globals.Network.Partitions), "partitions")
	
	// Return a simple placeholder
	return "Router initialized", nil
}
