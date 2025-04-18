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

// TestURLConstructionConsistency tests that URL construction is consistent with sequence.go approach
func TestURLConstructionConsistency(t *testing.T) {
	addrDir := NewTestAddressDir()
	
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
		
		// Verify we're using the heal_anchor.go approach (appending to anchor pool URL)
		if tc.partitionID != "dn" {
			incorrectFormat := "acc://" + tc.partitionID + ".acme" // The old sequence.go approach
			if anchorURL == incorrectFormat {
				t.Errorf("Using incorrect URL construction approach for %s. Got %s, which matches sequence.go approach", 
					tc.partitionID, anchorURL)
			}
		}
	}
}

// TestMultiaddressHandling tests the multiaddress parsing and handling functions
func TestMultiaddressHandling(t *testing.T) {
	addrDir := NewTestAddressDir()
	
	// Test valid multiaddresses
	validAddrs := []string{
		"/ip4/1.2.3.4/tcp/16593/p2p/12D3KooWJbJFaZ1dwWzsJYJ1HWcwrZRQkYEnAHEAr8v9fTJm5LX7",
		"/ip4/5.6.7.8/tcp/26656",
		"/dns4/example.com/tcp/16593/p2p/12D3KooWJbJFaZ1dwWzsJYJ1HWcwrZRQkYEnAHEAr8v9fTJm5LX7",
	}
	
	for _, addr := range validAddrs {
		isValid, err := addrDir.isValidMultiaddress(addr)
		if !isValid || err != nil {
			t.Errorf("Expected multiaddress %s to be valid, got isValid=%v, err=%v", 
				addr, isValid, err)
		}
	}
	
	// Test invalid multiaddresses
	invalidAddrs := []string{
		"not-a-multiaddress",
		"http://example.com",
		"/ip4/1.2.3.4/wrong/16593",
	}
	
	for _, addr := range invalidAddrs {
		isValid, _ := addrDir.isValidMultiaddress(addr)
		if isValid {
			t.Errorf("Expected multiaddress %s to be invalid, but it was reported as valid", addr)
		}
	}
	
	// Test extracting peer ID from multiaddress
	addrWithPeerID := "/ip4/1.2.3.4/tcp/16593/p2p/12D3KooWJbJFaZ1dwWzsJYJ1HWcwrZRQkYEnAHEAr8v9fTJm5LX7"
	peerID, err := addrDir.extractPeerIDFromMultiaddress(addrWithPeerID)
	if err != nil {
		t.Errorf("Failed to extract peer ID from %s: %v", addrWithPeerID, err)
	}
	if peerID != "12D3KooWJbJFaZ1dwWzsJYJ1HWcwrZRQkYEnAHEAr8v9fTJm5LX7" {
		t.Errorf("Expected peer ID to be 12D3KooWJbJFaZ1dwWzsJYJ1HWcwrZRQkYEnAHEAr8v9fTJm5LX7, got %s", peerID)
	}
	
	// Test extracting peer ID from multiaddress without p2p component
	addrWithoutPeerID := "/ip4/5.6.7.8/tcp/26656"
	_, err = addrDir.extractPeerIDFromMultiaddress(addrWithoutPeerID)
	if err == nil {
		t.Errorf("Expected error when extracting peer ID from %s, but got none", addrWithoutPeerID)
	}
}

// TestCacheConsistency tests that the URL construction is consistent with the caching system
func TestCacheConsistency(t *testing.T) {
	addrDir := NewTestAddressDir()
	
	// Create a validator
	validator := TestValidator("peer1", "validator1", "bvn-Apollo", "bvn")
	
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
	
	// Add a URL to the validator
	updatedValidator.URLs["partition"] = addrDir.constructPartitionURL("bvn-Apollo")
	success := addrDir.UpdateValidator(updatedValidator)
	if !success {
		t.Fatalf("Failed to update validator")
	}
	
	// Verify the URL is stored correctly
	retrievedValidator := addrDir.GetValidator("peer1")
	if retrievedValidator == nil {
		t.Fatalf("Expected to find validator with PeerID peer1, got nil")
	}
	
	expectedURL := "acc://bvn-Apollo.acme"
	if retrievedValidator.URLs["partition"] != expectedURL {
		t.Errorf("Expected partition URL to be %s, got %s", 
			expectedURL, retrievedValidator.URLs["partition"])
	}
	
	// Add an anchor URL to the validator
	retrievedValidator.URLs["anchor"] = addrDir.constructAnchorURL("bvn-Apollo")
	success = addrDir.UpdateValidator(retrievedValidator)
	if !success {
		t.Fatalf("Failed to update validator")
	}
	
	// Verify the anchor URL is stored correctly
	retrievedValidator = addrDir.GetValidator("peer1")
	if retrievedValidator == nil {
		t.Fatalf("Expected to find validator with PeerID peer1, got nil")
	}
	
	expectedAnchorURL := fmt.Sprintf("acc://dn.%s/anchors/bvn-Apollo", addrDir.GetNetworkName())
	if retrievedValidator.URLs["anchor"] != expectedAnchorURL {
		t.Errorf("Expected anchor URL to be %s, got %s", 
			expectedAnchorURL, retrievedValidator.URLs["anchor"])
	}
}
