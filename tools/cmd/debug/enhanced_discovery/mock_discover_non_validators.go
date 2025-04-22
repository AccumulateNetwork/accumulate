// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package enhanced_discovery

import (
	"context"
	"time"
)

// MockDiscoverNonValidatorPeers is a mock implementation of the DiscoverNonValidatorPeers function
// for testing purposes. It simulates the discovery of non-validator peers without requiring
// actual network connections.
func (nd *NetworkDiscoveryImpl) MockDiscoverNonValidatorPeers(ctx context.Context, mockService *MockNetworkService) error {
	// Skip if no validators have been discovered yet
	if len(nd.AddressDir.DNValidators) == 0 && len(nd.AddressDir.BVNValidators) == 0 {
		nd.Logger.Printf("No validators found, skipping non-validator peer discovery")
		return nil
	}

	nd.Logger.Printf("Starting mock non-validator peer discovery")
	
	// Get network peers directly from the mock service's Peers map
	// This avoids the complexity of dealing with interface{} types
	peersList := make([]struct {
		ID        string
		Addresses []string
	}, 0, len(mockService.Peers))
	
	// Convert the peers map to a slice
	for _, peer := range mockService.Peers {
		peersList = append(peersList, struct {
			ID        string
			Addresses []string
		}{
			ID:        peer.ID,
			Addresses: peer.Addresses,
		})
	}
	
	// Process each peer
	nonValidatorCount := 0
	for _, peer := range peersList {
		peerID := peer.ID
		addrStrings := peer.Addresses
		
		// Check if this is a validator
		isValidator := false
		
		// Check DN validators
		for _, validator := range nd.AddressDir.DNValidators {
			if validator.PeerID == peerID || validator.Name == peerID {
				isValidator = true
				break
			}
		}
		
		// Check BVN validators
		if !isValidator {
			for _, validators := range nd.AddressDir.BVNValidators {
				for _, validator := range validators {
					if validator.PeerID == peerID || validator.Name == peerID {
						isValidator = true
						break
					}
				}
				if isValidator {
					break
				}
			}
		}
		
		// If not a validator, add as a non-validator peer
		if !isValidator {
			nonValidatorCount++
			nd.Logger.Printf("Found non-validator peer %d: %s", nonValidatorCount, peerID)
			
			// Create a network peer
			networkPeer := NetworkPeer{
				ID:          peerID,
				IsValidator: false,
				Addresses:   addrStrings,
				LastSeen:    time.Now(),
				Active:      true,
			}
			
			// Add to network peers
			nd.AddressDir.AddNetworkPeer(networkPeer)
		}
	}
	
	nd.Logger.Printf("Mock non-validator peer discovery completed. Found %d non-validator peers.", nonValidatorCount)
	return nil
}
