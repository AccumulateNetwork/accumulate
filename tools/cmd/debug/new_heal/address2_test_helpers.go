// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// This file contains test helpers for address2.go tests

package new_heal

import (
	"log"
	"os"
	"sync"
	"time"
)

// NewTestAddressDir creates a new AddressDir instance for testing
func NewTestAddressDir() *AddressDir {
	logger := log.New(os.Stdout, "AddressDir: ", log.LstdFlags)
	return &AddressDir{
		Validators:   make([]*Validator, 0),
		NetworkPeers:  make(map[string]NetworkPeer),
		URLHelpers:    make(map[string]string),
		NetworkInfo: &NetworkInfo{
			Name:        "acme",
			ID:          "acme",
			IsMainnet:   false,
			Partitions:  make([]*PartitionInfo, 0),
			PartitionMap: make(map[string]*PartitionInfo),
			APIEndpoint: "https://mainnet.accumulatenetwork.io/v2",
		},
		Logger:        logger,
		peerDiscovery: NewSimplePeerDiscovery(logger),
		DiscoveryStats: NewDiscoveryStats(),
	}
}

// TestValidator creates a validator for testing
func TestValidator(peerID, name, partitionID, partitionType string) Validator {
	return Validator{
		PeerID:        peerID,
		Name:          name,
		PartitionID:   partitionID,
		PartitionType: partitionType,
		Status:        "active",
		P2PAddress:    "/ip4/127.0.0.1/tcp/16593/p2p/" + peerID,
		URLs:          make(map[string]string),
		LastUpdated:   time.Now(),
	}
}

// TestNetworkPeer creates a network peer for testing
func TestNetworkPeer(id string, isValidator bool, validatorID, partitionID string) NetworkPeer {
	return NetworkPeer{
		ID:           id,
		IsValidator:  isValidator,
		ValidatorID:  validatorID,
		PartitionID:  partitionID,
		Addresses:    []ValidatorAddress{{Address: "/ip4/127.0.0.1/tcp/16593/p2p/" + id, Validated: true, IP: "127.0.0.1", Port: "16593", PeerID: id}},
		Status:       "active",
		LastSeen:     time.Now(),
		FirstSeen:    time.Now(),
		PartitionURL: "acc://" + partitionID + ".acme",
	}
}

// MockAddressDir is a simplified version of AddressDir for testing
type MockAddressDir struct {
	mu            sync.RWMutex
	DNValidators  []Validator
	BVNValidators [][]Validator
	NetworkPeers  map[string]NetworkPeer
	NetworkName   string
}

// NewMockAddressDir creates a new MockAddressDir for testing
func NewMockAddressDir() *MockAddressDir {
	return &MockAddressDir{
		DNValidators:  make([]Validator, 0),
		BVNValidators: make([][]Validator, 3), // 3 BVNs for mainnet
		NetworkPeers:  make(map[string]NetworkPeer),
		NetworkName:   "acme",
	}
}

// ConstructPartitionURL standardizes URL construction for partitions
func (a *MockAddressDir) ConstructPartitionURL(partitionID string) string {
	return "acc://" + partitionID + "." + a.NetworkName
}

// ConstructAnchorURL standardizes URL construction for anchors
func (a *MockAddressDir) ConstructAnchorURL(partitionID string) string {
	return "acc://dn." + a.NetworkName + "/anchors/" + partitionID
}
