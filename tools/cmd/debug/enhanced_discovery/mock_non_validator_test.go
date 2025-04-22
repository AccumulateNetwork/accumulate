// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package enhanced_discovery

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMockNonValidatorDiscovery tests the discovery of non-validator nodes
// using a mock setup to demonstrate the functionality
func TestMockNonValidatorDiscovery(t *testing.T) {
	// Create a logger
	logger := log.New(os.Stdout, "[MOCK-NON-VALIDATOR-TEST] ", log.LstdFlags)
	logger.Printf("Starting mock non-validator discovery test")

	// Create an address directory
	addressDir := NewAddressDir()
	addressDir.Logger = logger

	// Create a network discovery instance
	discovery := &NetworkDiscoveryImpl{
		AddressDir: addressDir,
		Logger:     logger,
	}

	// Initialize the network for testnet
	logger.Printf("Initializing network for testnet")
	err := discovery.InitializeNetwork("testnet")
	require.NoError(t, err, "Failed to initialize network")

	// Set the NetworkInfo in the AddressDir
	addressDir.NetworkInfo = &discovery.Network

	// Create mock validators
	logger.Printf("Creating mock validators")
	createMockValidators(addressDir, 5)

	// Create mock non-validator peers
	logger.Printf("Creating mock non-validator peers")
	createMockNonValidatorPeers(addressDir, 10)

	// Log initial counts
	logger.Printf("Initial peer counts:")
	logger.Printf("  Validators: %d", len(addressDir.DNValidators))
	logger.Printf("  Total peers: %d", len(addressDir.NetworkPeers))
	
	// Count initial non-validator peers
	initialNonValidatorCount := 0
	for _, peer := range addressDir.NetworkPeers {
		if !peer.IsValidator {
			initialNonValidatorCount++
		}
	}
	logger.Printf("  Non-validator peers: %d", initialNonValidatorCount)

	// Create a mock service that will simulate finding non-validator nodes
	mockService := NewMockNetworkService()
	
	// Add some additional non-validator peers that will be "discovered"
	logger.Printf("Adding additional non-validator peers to be discovered")
	for i := 0; i < 5; i++ {
		peerID := fmt.Sprintf("discovered-non-validator-%d", i+1)
		peer := NetworkPeer{
			ID:          peerID,
			IsValidator: false,
			Addresses:   []string{fmt.Sprintf("/ip4/10.0.1.%d/tcp/16593", i+1)},
			LastSeen:    time.Now(),
			Active:      true,
		}
		
		// Add these to our mock service but not to the AddressDir
		// They will be discovered during the discovery process
		mockService.AddPeer(peer)
	}

	// Set up a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Run the discovery process with our mock implementation
	logger.Printf("Running network discovery with mock implementation")
	
	// First run the regular network discovery to process validators
	stats, err := discovery.DiscoverNetworkPeers(ctx, mockService)
	require.NoError(t, err, "Failed to discover network peers")
	
	// Then use our mock implementation to discover non-validator peers
	err = discovery.MockDiscoverNonValidatorPeers(ctx, mockService)
	require.NoError(t, err, "Failed to discover non-validator peers")

	// Log discovery statistics
	logger.Printf("Discovery Statistics:")
	logger.Printf("  Total Validators: %d", stats.TotalValidators)
	logger.Printf("  Total Peers: %d", stats.TotalPeers)
	logger.Printf("  New Peers: %d", stats.NewPeers)

	// Count non-validator peers after discovery
	finalNonValidatorCount := 0
	for _, peer := range addressDir.NetworkPeers {
		if !peer.IsValidator {
			finalNonValidatorCount++
		}
	}
	logger.Printf("  Non-validator peers after discovery: %d", finalNonValidatorCount)

	// Verify that we found the additional non-validator peers
	assert.Equal(t, initialNonValidatorCount+5, finalNonValidatorCount, 
		"Should have found 5 additional non-validator peers")

	// Log all non-validator peers
	logger.Printf("\nNon-Validator Nodes:")
	nonValidatorCount := 0
	for id, peer := range addressDir.NetworkPeers {
		if !peer.IsValidator {
			nonValidatorCount++
			logger.Printf("  Peer %d: %s", nonValidatorCount, id)
			if len(peer.Addresses) > 0 {
				logger.Printf("    Addresses: %v", peer.Addresses)
			}
			if !peer.LastSeen.IsZero() {
				logger.Printf("    Last Seen: %s", peer.LastSeen.Format(time.RFC3339))
			}
		}
	}

	logger.Printf("\nFound %d non-validator nodes out of %d total peers", 
		nonValidatorCount, len(addressDir.NetworkPeers))
	
	// Test completed successfully
	logger.Printf("Mock non-validator discovery test completed successfully")
}

// createMockValidators creates mock validators and adds them to the address directory
func createMockValidators(addressDir *AddressDir, count int) {
	for i := 0; i < count; i++ {
		validatorID := fmt.Sprintf("acc://validator-%d.acme", i+1)
		validator := NewValidator(validatorID, validatorID, "dn")
		validator.IPAddress = fmt.Sprintf("192.168.1.%d", i+1)
		validator.P2PAddress = fmt.Sprintf("tcp://192.168.1.%d:26656", i+1)
		validator.RPCAddress = fmt.Sprintf("http://192.168.1.%d:26657", i+1)
		validator.APIAddress = fmt.Sprintf("http://192.168.1.%d:8080", i+1)
		
		// Add to DN validators
		addressDir.AddDNValidator(validator)
		
		// Add as a network peer
		peer := NetworkPeer{
			ID:          validatorID,
			IsValidator: true,
			ValidatorID: validatorID,
			Addresses:   []string{fmt.Sprintf("/ip4/192.168.1.%d/tcp/16593", i+1)},
			LastSeen:    time.Now(),
			Active:      true,
		}
		addressDir.AddNetworkPeer(peer)
	}
}

// createMockNonValidatorPeers creates mock non-validator peers and adds them to the address directory
func createMockNonValidatorPeers(addressDir *AddressDir, count int) {
	for i := 0; i < count; i++ {
		peerID := fmt.Sprintf("non-validator-%d", i+1)
		peer := NetworkPeer{
			ID:          peerID,
			IsValidator: false,
			Addresses:   []string{fmt.Sprintf("/ip4/10.0.0.%d/tcp/16593", i+1)},
			LastSeen:    time.Now(),
			Active:      true,
		}
		addressDir.AddNetworkPeer(peer)
	}
}

// AddPeer adds a peer to the mock service's internal peer list
func (m *MockNetworkService) AddPeer(peer NetworkPeer) {
	// This is a simple extension to the mock service to support our test
	if m.Peers == nil {
		m.Peers = make(map[string]NetworkPeer)
	}
	m.Peers[peer.ID] = peer
}
