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

// TestAddNetworkPeer tests the AddNetworkPeer function
func TestAddNetworkPeer(t *testing.T) {
	addrDir := NewTestAddressDir()
	
	// Add a network peer
	peer := addrDir.AddNetworkPeer("peer1", false, "", "bvn-Apollo", "/ip4/127.0.0.1/tcp/16593/p2p/peer1")
	
	// Verify the peer was added correctly
	if peer.ID != "peer1" {
		t.Errorf("Expected peer ID to be peer1, got %s", peer.ID)
	}
	
	// Verify the peer can be retrieved
	retrievedPeer, found := addrDir.GetNetworkPeer("peer1")
	if !found {
		t.Errorf("Expected to find peer with ID peer1, but it was not found")
	}
	
	if retrievedPeer.ID != "peer1" {
		t.Errorf("Expected retrieved peer ID to be peer1, got %s", retrievedPeer.ID)
	}
}

// TestGetNetworkPeer tests the GetNetworkPeer function
func TestGetNetworkPeer(t *testing.T) {
	addrDir := NewTestAddressDir()
	
	// Add a network peer
	addrDir.AddNetworkPeer("peer1", false, "", "bvn-Apollo", "/ip4/127.0.0.1/tcp/16593/p2p/peer1")
	
	// Get the peer
	peer, found := addrDir.GetNetworkPeer("peer1")
	if !found {
		t.Errorf("Expected to find peer with ID peer1, but it was not found")
	}
	
	if peer.ID != "peer1" {
		t.Errorf("Expected peer ID to be peer1, got %s", peer.ID)
	}
	
	// Try to get a non-existent peer
	_, found = addrDir.GetNetworkPeer("non-existent")
	if found {
		t.Errorf("Expected not to find peer with ID non-existent, but it was found")
	}
}

// TestRemoveNetworkPeer tests the RemoveNetworkPeer function
func TestRemoveNetworkPeer(t *testing.T) {
	addrDir := NewTestAddressDir()
	
	// Add a network peer
	addrDir.AddNetworkPeer("peer1", false, "", "bvn-Apollo", "/ip4/127.0.0.1/tcp/16593/p2p/peer1")
	
	// Verify the peer exists
	_, found := addrDir.GetNetworkPeer("peer1")
	if !found {
		t.Errorf("Expected to find peer with ID peer1, but it was not found")
	}
	
	// Remove the peer
	removed := addrDir.RemoveNetworkPeer("peer1")
	if !removed {
		t.Errorf("Expected RemoveNetworkPeer to return true, got false")
	}
	
	// Verify the peer was removed
	_, found = addrDir.GetNetworkPeer("peer1")
	if found {
		t.Errorf("Expected not to find peer with ID peer1 after removal, but it was found")
	}
	
	// Try to remove a non-existent peer
	removed = addrDir.RemoveNetworkPeer("non-existent")
	if removed {
		t.Errorf("Expected RemoveNetworkPeer to return false for non-existent peer, got true")
	}
}

// TestGetAllNetworkPeers tests the GetAllNetworkPeers function
func TestGetAllNetworkPeers(t *testing.T) {
	addrDir := NewTestAddressDir()
	
	// Add some network peers
	addrDir.AddNetworkPeer("peer1", false, "", "bvn-Apollo", "/ip4/127.0.0.1/tcp/16593/p2p/peer1")
	addrDir.AddNetworkPeer("peer2", false, "", "bvn-Apollo", "/ip4/127.0.0.1/tcp/16593/p2p/peer2")
	addrDir.AddNetworkPeer("peer3", true, "validator1", "dn", "/ip4/127.0.0.1/tcp/16593/p2p/peer3")
	
	// Get all peers
	peers := addrDir.GetAllNetworkPeers()
	
	// Verify the number of peers
	if len(peers) != 3 {
		t.Errorf("Expected 3 peers, got %d", len(peers))
	}
	
	// Verify the peers are correct
	peerIDs := make(map[string]bool)
	for _, peer := range peers {
		peerIDs[peer.ID] = true
	}
	
	if !peerIDs["peer1"] || !peerIDs["peer2"] || !peerIDs["peer3"] {
		t.Errorf("Not all expected peers were returned")
	}
}

// TestUpdateNetworkPeerStatus tests the UpdateNetworkPeerStatus function
func TestUpdateNetworkPeerStatus(t *testing.T) {
	addrDir := NewTestAddressDir()
	
	// Add a network peer
	addrDir.AddNetworkPeer("peer1", false, "", "bvn-Apollo", "/ip4/127.0.0.1/tcp/16593/p2p/peer1")
	
	// Update the peer status
	updated := addrDir.UpdateNetworkPeerStatus("peer1", "inactive")
	if !updated {
		t.Errorf("Expected UpdateNetworkPeerStatus to return true, got false")
	}
	
	// Verify the status was updated
	peer, found := addrDir.GetNetworkPeer("peer1")
	if !found {
		t.Errorf("Expected to find peer with ID peer1, but it was not found")
	}
	
	if peer.Status != "inactive" {
		t.Errorf("Expected peer status to be inactive, got %s", peer.Status)
	}
	
	// Try to update a non-existent peer
	updated = addrDir.UpdateNetworkPeerStatus("non-existent", "inactive")
	if updated {
		t.Errorf("Expected UpdateNetworkPeerStatus to return false for non-existent peer, got true")
	}
}

// TestURLConstruction tests the URL construction functions
func TestURLConstruction(t *testing.T) {
	addrDir := NewTestAddressDir()
	
	// Test partition URL construction
	partitionURL := addrDir.constructPartitionURL("bvn-Apollo")
	expectedURL := fmt.Sprintf("acc://bvn-Apollo.%s", addrDir.GetNetworkName())
	if partitionURL != expectedURL {
		t.Errorf("Expected partition URL to be %s, got %s", expectedURL, partitionURL)
	}
	
	// Test anchor URL construction
	anchorURL := addrDir.constructAnchorURL("bvn-Apollo")
	expectedAnchorURL := fmt.Sprintf("acc://dn.%s/anchors/bvn-Apollo", addrDir.GetNetworkName())
	if anchorURL != expectedAnchorURL {
		t.Errorf("Expected anchor URL to be %s, got %s", expectedAnchorURL, anchorURL)
	}
	
	// Test anchor URL construction for DN
	dnAnchorURL := addrDir.constructAnchorURL("dn")
	expectedDNAnchorURL := fmt.Sprintf("acc://dn.%s/anchors/dn", addrDir.GetNetworkName())
	if dnAnchorURL != expectedDNAnchorURL {
		t.Errorf("Expected DN anchor URL to be %s, got %s", expectedDNAnchorURL, dnAnchorURL)
	}
}
