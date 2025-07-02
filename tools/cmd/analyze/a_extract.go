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
	
	"gitlab.com/accumulatenetwork/accumulate/internal/api/routing"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
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
func InitializeRouting(config *NetworkConfig) (routing.Router, error) {
	fmt.Println("Initializing routing with", len(config.Globals.Network.Partitions), "partitions")
	
	// Create a routing table from the network configuration
	routingTable := &protocol.RoutingTable{}
	
	// Add routes for each partition
	for i, partition := range config.Globals.Network.Partitions {
		// Create a route for this partition
		// Use simple bit-based routing where each partition gets a range
		route := protocol.Route{
			Partition: partition.ID,
			Length:    1, // Use 1-bit routing for simplicity
			Value:     uint64(i),
		}
		routingTable.Routes = append(routingTable.Routes, route)
		fmt.Printf("  Added route: %s (value=%d, length=%d)\n", partition.ID, route.Value, route.Length)
	}
	
	// Create the router instance
	router := routing.NewRouter(routing.RouterOptions{
		Initial: routingTable,
	})
	
	fmt.Println("Router successfully initialized with", len(routingTable.Routes), "routes")
	return router, nil
}
