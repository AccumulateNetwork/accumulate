// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"context"
	"testing"
	"time"
)

func TestAddressDir_AddValidator(t *testing.T) {
	// Create a new AddressDir instance
	dir := NewAddressDir()
	
	// Add validators
	_ = dir.AddValidator("val1", "Validator 1", "dn", "dn")
	dir.AddBVNValidator(0, Validator{
		ID:            "val2",
		PeerID:        "val2",
		Name:          "Validator 2",
		PartitionID:   "Apollo",
		PartitionType: "bvn",
		BVN:           0,
		Status:        "active",
		LastUpdated:   time.Now(),
	})
	dir.AddDNValidator(Validator{
		ID:            "val3",
		PeerID:        "val3",
		Name:          "Validator 3",
		PartitionID:   "dn",
		PartitionType: "dn",
		Status:        "active",
		LastUpdated:   time.Now(),
	})
	
	// Test FindValidator
	foundValidator1, _, found1 := dir.FindValidator("val1")
	foundValidator2, _, found2 := dir.FindValidator("val2")
	foundValidator3, _, found3 := dir.FindValidator("val3")
	
	// Assertions
	if !found1 || foundValidator1 == nil || foundValidator1.ID != "val1" {
		t.Errorf("Failed to find validator1, found: %v", foundValidator1)
	}
	if !found2 || foundValidator2 == nil || foundValidator2.ID != "val2" {
		t.Errorf("Failed to find validator2, found: %v", foundValidator2)
	}
	if !found3 || foundValidator3 == nil || foundValidator3.ID != "val3" {
		t.Errorf("Failed to find validator3, found: %v", foundValidator3)
	}
	
	// Test FindValidatorsByPartition
	dnValidators := dir.FindValidatorsByPartition("dn")
	bvnValidators := dir.FindValidatorsByPartition("Apollo")
	
	if len(dnValidators) != 2 {
		t.Errorf("Expected 2 DN validators, got %d", len(dnValidators))
	}
	if len(bvnValidators) != 1 {
		t.Errorf("Expected 1 BVN validator, got %d", len(bvnValidators))
	}
	
	// Test FindValidatorsByBVN
	bvn0Validators := dir.FindValidatorsByBVN(0)
	
	if len(bvn0Validators) != 1 {
		t.Errorf("Expected 1 BVN 0 validator, got %d", len(bvn0Validators))
	}
	
	// Test address management
	dir.SetValidatorP2PAddress("val1", "/ip4/127.0.0.1/tcp/26656/p2p/12D3KooWJN9BjpUXjxJFrHdXjwSYgm97oTgH7L7iVJ7DTBuxE7u5")
	dir.SetValidatorRPCAddress("val1", "http://127.0.0.1:26657")
	dir.SetValidatorAPIAddress("val1", "http://127.0.0.1:8080")
	
	// Test GetKnownAddressesForValidator
	addresses := dir.GetKnownAddressesForValidator("val1")
	if len(addresses) != 3 {
		t.Errorf("Expected 3 addresses for validator1, got %d", len(addresses))
	}
	
	// Test problem node management
	dir.MarkNodeProblematic("val2", "test reason")
	if !dir.IsNodeProblematic("val2") {
		t.Errorf("Expected val2 to be problematic")
	}
	
	problemNodes := dir.GetProblemNodes()
	if len(problemNodes) != 1 {
		t.Errorf("Expected 1 problem node, got %d", len(problemNodes))
	}
	
	dir.AddRequestTypeToAvoid("val2", "query")
	if !dir.ShouldAvoidForRequestType("val2", "query") {
		t.Errorf("Expected val2 to be avoided for query requests")
	}
	
	dir.ClearProblematicStatus("val2")
	if dir.IsNodeProblematic("val2") {
		t.Errorf("Expected val2 to no longer be problematic")
	}
}

func TestAddressDir_DiscoverNetworkPeers(t *testing.T) {
	// Create a new AddressDir instance
	dir := NewAddressDir()
	
	// Create a mock network service
	mockService := CreateMockNetworkService()
	ctx := context.Background()
	
	// Call the method with our mock service
	peerCount, err := dir.DiscoverNetworkPeers(ctx, mockService)
	
	// Verify that the method works with our mock service
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	
	// We should discover at least some peers with our mock service
	if peerCount == 0 {
		t.Errorf("Expected to discover at least one peer, got %d", peerCount)
	}
}

func TestAddressDir_RefreshNetworkPeers(t *testing.T) {
	// Create a new AddressDir instance
	dir := NewAddressDir()
	
	// Add some test validators and peers
	dir.AddValidator("val1", "Validator 1", "dn", "dn")
	dir.AddBVNValidator(0, Validator{
		ID:            "val2",
		PeerID:        "val2",
		Name:          "Validator 2",
		PartitionID:   "Apollo",
		PartitionType: "bvn",
		BVN:           0,
		Status:        "active",
		LastUpdated:   time.Now(),
	})
	
	// Add network peers
	peer1 := NetworkPeer{
		ID:              "val1",
		IsValidator:     true,
		ValidatorID:     "val1",
		PartitionID:     "dn",
		Status:          "active",
		LastSeen:        time.Now(),
		FirstSeen:       time.Now(),
		DiscoveryMethod: "test",
		DiscoveredAt:    time.Now(),
	}
	dir.NetworkPeers["val1"] = peer1
	
	// Test RefreshNetworkPeers
	ctx := context.Background()
	stats, err := dir.RefreshNetworkPeers(ctx, nil)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	
	// Check stats
	if stats.TotalPeers != 1 {
		t.Errorf("Expected 1 total peer, got %d", stats.TotalPeers)
	}
	if stats.ActiveValidators != 1 {
		t.Errorf("Expected 1 active validator, got %d", stats.ActiveValidators)
	}
}

func TestConstructPartitionURL(t *testing.T) {
	tests := []struct {
		name        string
		partitionID string
		want        string
	}{
		{
			name:        "Directory Network",
			partitionID: "dn",
			want:        "acc://dn.acme",
		},
		{
			name:        "BVN Apollo",
			partitionID: "bvn-Apollo",
			want:        "acc://bvn-Apollo.acme",
		},
		{
			name:        "BVN Artemis",
			partitionID: "bvn-Artemis",
			want:        "acc://bvn-Artemis.acme",
		},
		{
			name:        "Empty partition",
			partitionID: "",
			want:        "acc://.acme", // Edge case, should probably handle this better
		},
	}

	addrDir := NewTestAddressDir()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addrDir.constructPartitionURL(tt.partitionID)
			if got != tt.want {
				t.Errorf("constructPartitionURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConstructAnchorURL(t *testing.T) {
	tests := []struct {
		name        string
		partitionID string
		want        string
	}{
		{
			name:        "Directory Network",
			partitionID: "dn",
			want:        "acc://dn.acme",
		},
		{
			name:        "BVN Apollo",
			partitionID: "bvn-Apollo",
			want:        "acc://bvn-Apollo.acme",
		},
		{
			name:        "BVN Artemis",
			partitionID: "bvn-Artemis",
			want:        "acc://bvn-Artemis.acme",
		},
		{
			name:        "Empty partition",
			partitionID: "",
			want:        "acc://.acme", // Edge case, should probably handle this better
		},
	}

	addrDir := NewTestAddressDir()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := addrDir.constructAnchorURL(tt.partitionID)
			if got != tt.want {
				t.Errorf("constructAnchorURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseMultiaddress(t *testing.T) {
	tests := []struct {
		name      string
		address   string
		wantIP    string
		wantPort  string
		wantPeerID string
		wantErr   bool
	}{
		{
			name:      "Valid multiaddress with IPv4",
			address:   "/ip4/65.108.73.121/tcp/16593/p2p/QmHash123",
			wantIP:    "65.108.73.121",
			wantPort:  "16593",
			wantPeerID: "QmHash123",
			wantErr:   false,
		},
		{
			name:      "Valid multiaddress with IPv6",
			address:   "/ip6/2001:db8::1/tcp/16593/p2p/QmHash456",
			wantIP:    "2001:db8::1",
			wantPort:  "16593",
			wantPeerID: "QmHash456",
			wantErr:   false,
		},
		{
			name:      "Invalid multiaddress",
			address:   "not-a-multiaddress",
			wantIP:    "",
			wantPort:  "",
			wantPeerID: "",
			wantErr:   true,
		},
		{
			name:      "Multiaddress without IP",
			address:   "/tcp/16593/p2p/QmHash789",
			wantIP:    "",
			wantPort:  "",
			wantPeerID: "",
			wantErr:   true,
		},
	}

	addrDir := NewTestAddressDir()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIP, gotPort, gotPeerID, err := addrDir.parseMultiaddress(tt.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseMultiaddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if gotIP != tt.wantIP {
					t.Errorf("parseMultiaddress() gotIP = %v, want %v", gotIP, tt.wantIP)
				}
				if gotPort != tt.wantPort {
					t.Errorf("parseMultiaddress() gotPort = %v, want %v", gotPort, tt.wantPort)
				}
				if gotPeerID != tt.wantPeerID {
					t.Errorf("parseMultiaddress() gotPeerID = %v, want %v", gotPeerID, tt.wantPeerID)
				}
			}
		})
	}
}

func TestValidateMultiaddress(t *testing.T) {
	tests := []struct {
		name      string
		address   string
		wantValid bool
	}{
		{
			name:      "Valid multiaddress",
			address:   "/ip4/65.108.73.121/tcp/16593/p2p/QmHash123",
			wantValid: true,
		},
		{
			name:      "Invalid multiaddress",
			address:   "not-a-multiaddress",
			wantValid: false,
		},
		{
			name:      "Multiaddress without IP",
			address:   "/tcp/16593/p2p/QmHash789",
			wantValid: false,
		},
	}

	addrDir := NewTestAddressDir()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, _, valid := addrDir.ValidateMultiaddress(tt.address)
			if valid != tt.wantValid {
				t.Errorf("ValidateMultiaddress() valid = %v, want %v", valid, tt.wantValid)
			}
		})
	}
}
