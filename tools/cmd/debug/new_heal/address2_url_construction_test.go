// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"fmt"
	"testing"
)

// TestURLConstructionFunctions tests the URL construction functions in address2.go
func TestURLConstructionFunctions(t *testing.T) {
	// Create an AddressDir instance
	addrDir := NewAddressDir()
	
	// Test cases for different partition types
	testCases := []struct {
		partitionID     string
		expectedURL     string
		expectedAnchorURL string
	}{
		{"dn", "acc://dn.acme", fmt.Sprintf("acc://dn.%s/anchors/dn", addrDir.GetNetworkName())},
		{"bvn-Apollo", "acc://bvn-Apollo.acme", fmt.Sprintf("acc://dn.%s/anchors/bvn-Apollo", addrDir.GetNetworkName())},
		{"bvn-Artemis", "acc://bvn-Artemis.acme", fmt.Sprintf("acc://dn.%s/anchors/bvn-Artemis", addrDir.GetNetworkName())},
	}
	
	for _, tc := range testCases {
		t.Run(tc.partitionID, func(t *testing.T) {
			// Test partition URL construction
			partitionURL := addrDir.constructPartitionURL(tc.partitionID)
			if partitionURL != tc.expectedURL {
				t.Errorf("Expected partition URL for %s to be %s, got %s", 
					tc.partitionID, tc.expectedURL, partitionURL)
			}
			
			// Test anchor URL construction
			anchorURL := addrDir.constructAnchorURL(tc.partitionID)
			if anchorURL != tc.expectedAnchorURL {
				t.Errorf("Expected anchor URL for %s to be %s, got %s", 
					tc.partitionID, tc.expectedAnchorURL, anchorURL)
			}
		})
	}
}

// TestURLConsistencyWithCaching tests that the URL construction is consistent with the caching system
func TestURLConsistencyWithCaching(t *testing.T) {
	addrDir := NewAddressDir()
	
	// Create a validator
	validator := Validator{
		PeerID:        "peer1",
		Name:          "validator1",
		PartitionID:   "bvn-Apollo",
		PartitionType: "bvn",
		Status:        "active",
		URLs:          make(map[string]string),
	}
	
	// Add the validator
	err := addrDir.AddValidator(validator)
	if err != nil {
		t.Fatalf("Failed to add validator: %v", err)
	}
	
	// Get the validator
	updatedValidator := addrDir.GetValidator("peer1")
	if updatedValidator == nil {
		t.Fatalf("Expected to find validator with PeerID peer1, got nil")
	}
	
	// Add URLs to the validator
	updatedValidator.URLs["partition"] = addrDir.constructPartitionURL("bvn-Apollo")
	updatedValidator.URLs["anchor"] = addrDir.constructAnchorURL("bvn-Apollo")
	success := addrDir.UpdateValidator(updatedValidator)
	if !success {
		t.Fatalf("Failed to update validator")
	}
	
	// Verify the URLs are stored correctly
	retrievedValidator := addrDir.GetValidator("peer1")
	if retrievedValidator == nil {
		t.Fatalf("Expected to find validator with PeerID peer1, got nil")
	}
	
	expectedPartitionURL := "acc://bvn-Apollo.acme"
	if retrievedValidator.URLs["partition"] != expectedPartitionURL {
		t.Errorf("Expected partition URL to be %s, got %s", 
			expectedPartitionURL, retrievedValidator.URLs["partition"])
	}
	
	expectedAnchorURL := fmt.Sprintf("acc://dn.%s/anchors/bvn-Apollo", addrDir.GetNetworkName())
	if retrievedValidator.URLs["anchor"] != expectedAnchorURL {
		t.Errorf("Expected anchor URL to be %s, got %s", 
			expectedAnchorURL, retrievedValidator.URLs["anchor"])
	}
}
