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
	// Globals contains network-wide configuration
	Globals struct {
		Network struct {
			Partitions []struct {
				ID   string `json:"id"`
				Type string `json:"type"`
			} `json:"partitions"`
		} `json:"network"`
	} `json:"globals"`
}
