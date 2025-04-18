// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"testing"
	"time"
)

func TestNetworkPeerMethods(t *testing.T) {
	// Create a new AddressDir
	addr := NewAddressDir()
	
	// Add some network peers
	addr.AddNetworkPeer("peer1", true, "validator1", "bvn1")
	addr.AddNetworkPeer("peer2", false, "", "")
	addr.AddNetworkPeer("peer3", true, "validator2", "bvn2")
	
	// Test GetNetworkPeer
	retrievedPeer, exists := addr.GetNetworkPeer("peer1")
	if !exists {
		t.Errorf("Expected network peer 'peer1' to exist")
	}
	if retrievedPeer.ID != "peer1" {
		t.Errorf("Expected network peer ID to be 'peer1', got '%s'", retrievedPeer.ID)
	}
	
	// Test GetNetworkPeers
	peers := addr.GetNetworkPeers()
	if len(peers) != 3 {
		t.Errorf("Expected 3 network peers, got %d", len(peers))
	}
	
	// Test GetValidatorPeers
	validatorPeers := addr.GetValidatorPeers()
	if len(validatorPeers) != 2 {
		t.Errorf("Expected 2 validator peers, got %d", len(validatorPeers))
	}
	
	// Test GetNonValidatorPeers
	nonValidatorPeers := addr.GetNonValidatorPeers()
	if len(nonValidatorPeers) != 1 {
		t.Errorf("Expected 1 non-validator peer, got %d", len(nonValidatorPeers))
	}
	
	// Test AddPeerAddress
	err := addr.AddPeerAddress("peer1", "/ip4/127.0.0.1/tcp/8080/p2p/QmPeer1")
	if err != nil {
		t.Errorf("Expected no error when adding address to peer, got: %v", err)
	}
	
	// Verify the address was added
	retrievedPeer, _ = addr.GetNetworkPeer("peer1")
	if len(retrievedPeer.Addresses) != 1 {
		t.Errorf("Expected peer to have 1 address, got %d", len(retrievedPeer.Addresses))
	}
	if retrievedPeer.Addresses[0].Address != "/ip4/127.0.0.1/tcp/8080/p2p/QmPeer1" {
		t.Errorf("Expected address to be '/ip4/127.0.0.1/tcp/8080/p2p/QmPeer1', got '%s'", retrievedPeer.Addresses[0].Address)
	}
	
	// Test RemoveNetworkPeer
	removed := addr.RemoveNetworkPeer("peer2")
	if !removed {
		t.Errorf("Expected peer2 to be removed successfully")
	}
	
	// Verify the peer was removed
	_, exists = addr.GetNetworkPeer("peer2")
	if exists {
		t.Errorf("Expected peer2 to not exist after removal")
	}
	
	// Test UpdateNetworkPeerStatus
	updated := addr.UpdateNetworkPeerStatus("peer3", "inactive")
	if !updated {
		t.Errorf("Expected peer3 status to be updated successfully")
	}
	
	// Verify the status was updated
	retrievedPeer, _ = addr.GetNetworkPeer("peer3")
	if retrievedPeer.Status != "inactive" {
		t.Errorf("Expected peer3 status to be 'inactive', got '%s'", retrievedPeer.Status)
	}
}

func TestNetworkPeerAddAddress(t *testing.T) {
	// Create a new network peer
	peer := NetworkPeer{
		ID:          "peer1",
		IsValidator: true,
		ValidatorID: "validator1",
		PartitionID: "bvn1",
		Status:      "active",
		LastSeen:    time.Now(),
		FirstSeen:   time.Now(),
		Addresses:   make([]ValidatorAddress, 0),
	}
	
	// Add an address
	peer.AddAddress("/ip4/127.0.0.1/tcp/8080/p2p/QmPeer1")
	
	// Verify the address was added
	if len(peer.Addresses) != 1 {
		t.Errorf("Expected peer to have 1 address, got %d", len(peer.Addresses))
	}
	if peer.Addresses[0].Address != "/ip4/127.0.0.1/tcp/8080/p2p/QmPeer1" {
		t.Errorf("Expected address to be '/ip4/127.0.0.1/tcp/8080/p2p/QmPeer1', got '%s'", peer.Addresses[0].Address)
	}
	
	// Add the same address again
	peer.AddAddress("/ip4/127.0.0.1/tcp/8080/p2p/QmPeer1")
	
	// Verify the address was not duplicated
	if len(peer.Addresses) != 1 {
		t.Errorf("Expected peer to still have 1 address after adding duplicate, got %d", len(peer.Addresses))
	}
	
	// Add a different address
	peer.AddAddress("/ip4/192.168.1.1/tcp/9090/p2p/QmPeer1")
	
	// Verify the new address was added
	if len(peer.Addresses) != 2 {
		t.Errorf("Expected peer to have 2 addresses, got %d", len(peer.Addresses))
	}
}

func TestPerformanceMetrics(t *testing.T) {
	// Create a new AddressDir
	addr := NewAddressDir()
	
	// Add a network peer
	addr.AddNetworkPeer("peer1", true, "validator1", "bvn1")
	
	// Update performance metrics with successful operation
	err := addr.UpdatePeerPerformance("peer1", 100*time.Millisecond, true, "")
	if err != nil {
		t.Errorf("Expected no error when updating peer performance, got: %v", err)
	}
	
	// Verify the metrics were updated
	peer, _ := addr.GetNetworkPeer("peer1")
	if peer.ResponseTime != 100*time.Millisecond {
		t.Errorf("Expected response time to be 100ms, got %v", peer.ResponseTime)
	}
	if peer.SuccessRate != 1.0 {
		t.Errorf("Expected success rate to be 1.0, got %f", peer.SuccessRate)
	}
	if peer.SuccessfulConnections != 1 {
		t.Errorf("Expected successful connections to be 1, got %d", peer.SuccessfulConnections)
	}
	if peer.LastSuccess.IsZero() {
		t.Errorf("Expected last success time to be set")
	}
	
	// Update performance metrics with failed operation
	err = addr.UpdatePeerPerformance("peer1", 500*time.Millisecond, false, "connection timeout")
	if err != nil {
		t.Errorf("Expected no error when updating peer performance, got: %v", err)
	}
	
	// Verify the metrics were updated
	peer, _ = addr.GetNetworkPeer("peer1")
	// Response time should be weighted average: 0.2*500ms + 0.8*100ms = 180ms
	expectedResponseTime := time.Duration(float64(500*time.Millisecond)*0.2 + float64(100*time.Millisecond)*0.8)
	if peer.ResponseTime != expectedResponseTime {
		t.Errorf("Expected response time to be %v, got %v", expectedResponseTime, peer.ResponseTime)
	}
	// Success rate should be weighted average: 0.1*0.0 + 0.9*1.0 = 0.9
	if peer.SuccessRate != 0.9 {
		t.Errorf("Expected success rate to be 0.9, got %f", peer.SuccessRate)
	}
	if peer.FailedConnections != 1 {
		t.Errorf("Expected failed connections to be 1, got %d", peer.FailedConnections)
	}
	if peer.LastError != "connection timeout" {
		t.Errorf("Expected last error to be 'connection timeout', got '%s'", peer.LastError)
	}
	if peer.LastErrorTime.IsZero() {
		t.Errorf("Expected last error time to be set")
	}
}

func TestBestPeerSelection(t *testing.T) {
	// Create a new AddressDir
	addr := NewAddressDir()
	
	// Add several network peers
	addr.AddNetworkPeer("peer1", true, "validator1", "bvn1")
	addr.AddNetworkPeer("peer2", true, "validator2", "bvn1")
	addr.AddNetworkPeer("peer3", true, "validator3", "bvn2")
	
	// Update performance metrics for each peer
	addr.UpdatePeerPerformance("peer1", 200*time.Millisecond, true, "")
	addr.UpdatePeerPerformance("peer2", 100*time.Millisecond, true, "")
	addr.UpdatePeerPerformance("peer3", 300*time.Millisecond, true, "")
	
	// Add some failures to peer3
	addr.UpdatePeerPerformance("peer3", 300*time.Millisecond, false, "timeout")
	addr.UpdatePeerPerformance("peer3", 300*time.Millisecond, false, "timeout")
	
	// Mark peer1 as preferred
	addr.MarkPeerPreferred("peer1")
	
	// Test GetBestNetworkPeer
	bestPeer := addr.GetBestNetworkPeer()
	if bestPeer == nil {
		t.Fatalf("Expected to find a best peer, got nil")
	}
	if bestPeer.ID != "peer1" {
		t.Errorf("Expected best peer to be peer1 (preferred), got %s", bestPeer.ID)
	}
	
	// Test GetBestNetworkPeerForPartition
	bestPeerBVN1 := addr.GetBestNetworkPeerForPartition("bvn1")
	if bestPeerBVN1 == nil {
		t.Fatalf("Expected to find a best peer for bvn1, got nil")
	}
	if bestPeerBVN1.ID != "peer1" {
		t.Errorf("Expected best peer for bvn1 to be peer1 (preferred), got %s", bestPeerBVN1.ID)
	}
	
	bestPeerBVN2 := addr.GetBestNetworkPeerForPartition("bvn2")
	if bestPeerBVN2 == nil {
		t.Fatalf("Expected to find a best peer for bvn2, got nil")
	}
	if bestPeerBVN2.ID != "peer3" {
		t.Errorf("Expected best peer for bvn2 to be peer3, got %s", bestPeerBVN2.ID)
	}
	
	// Test GetPreferredPeers
	preferredPeers := addr.GetPreferredPeers()
	if len(preferredPeers) != 1 {
		t.Errorf("Expected 1 preferred peer, got %d", len(preferredPeers))
	}
	if len(preferredPeers) > 0 && preferredPeers[0].ID != "peer1" {
		t.Errorf("Expected preferred peer to be peer1, got %s", preferredPeers[0].ID)
	}
}
