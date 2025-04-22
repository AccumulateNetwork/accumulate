// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"fmt"
	"time"
)

// ResolveWellKnownEndpoint returns a well-known API endpoint for the given network and version
func ResolveWellKnownEndpoint(network, version string) string {
	switch network {
	case "mainnet":
		return fmt.Sprintf("https://mainnet.accumulatenetwork.io/%s", version)
	case "testnet":
		return fmt.Sprintf("https://testnet.accumulatenetwork.io/%s", version)
	case "devnet":
		return fmt.Sprintf("https://devnet.accumulatenetwork.io/%s", version)
	default:
		return fmt.Sprintf("https://%s.accumulatenetwork.io/%s", network, version)
	}
}

// CreateTestValidator creates a test validator with the given name and partition
func CreateTestValidator(name, partitionID string) *Validator {
	partitionType := "bvn"
	if partitionID == "dn" {
		partitionType = "directory"
	}
	
	return &Validator{
		Name:          name,
		PartitionID:   partitionID,
		PartitionType: partitionType,
		Status:        "active",
		LastUpdated:   time.Now(),
		Addresses:     []ValidatorAddress{},
		URLs:          make(map[string]string),
		BVNHeights:    make(map[string]uint64),
	}
}

// CreateTestAddressStats creates test address statistics
func CreateTestAddressStats() AddressStats {
	return AddressStats{
		TotalByType:        make(map[string]int),
		ValidByType:        make(map[string]int),
		ResponseTimeByType: make(map[string]time.Duration),
	}
}

// StandardizeTestURL standardizes URLs for testing
func StandardizeTestURL(urlType, partitionID, baseURL string) string {
	switch urlType {
	case "partition":
		if partitionID == "dn" {
			return "acc://dn.acme"
		}
		return fmt.Sprintf("acc://bvn-%s.acme", partitionID)
	case "anchor":
		return fmt.Sprintf("acc://dn.acme/anchors/%s", partitionID)
	case "api", "rpc", "p2p", "metrics":
		return baseURL
	default:
		return baseURL
	}
}
