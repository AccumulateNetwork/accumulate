// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package enhanced_discovery

import (
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSliceBasedValidatorStorage tests that validators are stored in slices as required by the design
func TestSliceBasedValidatorStorage(t *testing.T) {
	// Create an address directory
	addressDir := NewAddressDir()
	
	// Create validators
	val1 := NewValidator("validator1", "Validator 1", "dn")
	val2 := NewValidator("validator2", "Validator 2", "bvn-Apollo")
	val3 := NewValidator("validator3", "Validator 3", "bvn-Chandrayaan")
	
	// Add validators
	addressDir.AddDNValidator(val1)
	addressDir.AddBVNValidator(0, val2)
	addressDir.AddBVNValidator(1, val3)
	
	// Verify DN validators are stored in a slice
	assert.IsType(t, []Validator{}, addressDir.DNValidators)
	assert.Len(t, addressDir.DNValidators, 1)
	
	// Verify BVN validators are stored in a slice of slices
	assert.IsType(t, [][]Validator{}, addressDir.BVNValidators)
	assert.Len(t, addressDir.BVNValidators, 2)
	assert.Len(t, addressDir.BVNValidators[0], 1)
	assert.Len(t, addressDir.BVNValidators[1], 1)
	
	// Verify validator contents
	assert.Equal(t, "validator1", addressDir.DNValidators[0].PeerID)
	assert.Equal(t, "validator2", addressDir.BVNValidators[0][0].PeerID)
	assert.Equal(t, "validator3", addressDir.BVNValidators[1][0].PeerID)
}

// TestURLStandardizationWithDiscovery tests the URL standardization functionality
func TestURLStandardizationWithDiscovery(t *testing.T) {
	// Create an address directory
	addressDir := NewAddressDir()
	
	// Create a logger
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	
	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)
	
	// Initialize for testnet
	err := discovery.InitializeNetwork("testnet")
	require.NoError(t, err)
	
	// Test cases for URL standardization
	testCases := []struct {
		name        string
		partitionID string
		urlType     string
		expected    string
	}{
		{
			name:        "DN Partition URL",
			partitionID: "dn",
			urlType:     "partition",
			expected:    "acc://dn.acme",
		},
		{
			name:        "BVN Partition URL",
			partitionID: "bvn-Apollo",
			urlType:     "partition",
			expected:    "acc://bvn-Apollo.acme",
		},
		{
			name:        "BVN Name Only Partition URL",
			partitionID: "Apollo",
			urlType:     "partition",
			expected:    "acc://bvn-Apollo.acme",
		},
		{
			name:        "DN Anchor URL",
			partitionID: "dn",
			urlType:     "anchor",
			expected:    "acc://dn.acme/anchors",
		},
		{
			name:        "BVN Anchor URL",
			partitionID: "bvn-Apollo",
			urlType:     "anchor",
			expected:    "acc://dn.acme/anchors/Apollo",
		},
		{
			name:        "BVN Name Only Anchor URL",
			partitionID: "Apollo",
			urlType:     "anchor",
			expected:    "acc://dn.acme/anchors/Apollo",
		},
	}
	
	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := discovery.standardizeURL(tc.urlType, tc.partitionID, "")
			assert.Equal(t, tc.expected, result)
		})
	}
}
