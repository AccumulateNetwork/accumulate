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

// NetworkConfig only needs json tags for unmarshaling, no other imports required

// NetworkConfig represents the network configuration from network.json
type NetworkConfig struct {
	// ID is the network identifier
	ID string `json:"id"`
	
	// Globals contains network-wide configuration
	Globals struct {
		// Oracle contains oracle configuration
		Oracle struct {
			Price int `json:"price"`
		} `json:"oracle"`
		
		// Globals contains the nested globals configuration
		Globals struct {
			// Add other fields as needed
		} `json:"globals"`
		
		// Network contains the network configuration
		Network struct {
			// NetworkName is the name of the network
			NetworkName string `json:"networkName"`
			
			// Partitions defines the network partitions
			Partitions []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"partitions"`
			
			// Validators defines the network validators
			Validators []struct {
				// Operator is the validator operator name
				Operator string `json:"operator"`
				
				// PublicKey is the validator's public key (hex encoded)
				PublicKey string `json:"publicKey"`
				
				// Partitions defines which partitions this validator is active for
				Partitions []struct {
					// ID is the partition ID
					ID string `json:"id"`
					
					// Active indicates if the validator is active for this partition
					Active bool `json:"active"`
				} `json:"partitions"`
			} `json:"validators"`
		} `json:"network"`
	} `json:"globals"`
}
