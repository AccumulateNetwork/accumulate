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
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

// TestNonValidatorNodes tests finding non-validator nodes in the network
// This test is skipped by default when running with -short flag
// Run with: go test -v -run TestNonValidatorNodes
// Skip with: go test -v -short -run TestNonValidatorNodes
func TestNonValidatorNodes(t *testing.T) {
	// Skip in short mode (for quick test runs)
	if testing.Short() {
		t.Skip("Skipping mainnet test in short mode. Run without -short flag to test against mainnet.")
	}

	// Create a logger
	logger := log.New(os.Stdout, "[NON-VALIDATOR-TEST] ", log.LstdFlags)
	logger.Printf("Starting non-validator nodes test")

	// Create a client for mainnet
	endpoint := "https://mainnet.accumulatenetwork.io/v3"
	logger.Printf("Using API endpoint: %s", endpoint)
	client := jsonrpc.NewClient(endpoint)
	
	// Set a reasonable timeout for the context
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First, get the network status to find validators
	logger.Printf("Getting network status")
	networkStatus, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	require.NoError(t, err, "Failed to get network status")
	
	// Create a map of validator IDs for quick lookup
	validatorIDs := make(map[string]bool)
	for _, validator := range networkStatus.Network.Validators {
		validatorIDs[validator.Operator.String()] = true
	}
	logger.Printf("Found %d validators in network status", len(validatorIDs))

	// Now create our enhanced discovery implementation
	logger.Printf("Creating enhanced network discovery implementation")
	addressDir := NewAddressDir()
	addressDir.Logger = logger

	discovery := &NetworkDiscoveryImpl{
		AddressDir: addressDir,
		Logger:     logger,
	}

	// Initialize the network for mainnet
	logger.Printf("Initializing network for mainnet")
	err = discovery.InitializeNetwork("mainnet")
	require.NoError(t, err, "Failed to initialize network")

	// Set the NetworkInfo in the AddressDir
	addressDir.NetworkInfo = &discovery.Network

	// Add some validators to the AddressDir
	logger.Printf("Adding validators to AddressDir")
	for i, validatorInfo := range networkStatus.Network.Validators {
		// Only process the first few validators to keep the test quick
		if i >= 5 {
			break
		}
		
		operatorID := validatorInfo.Operator.String()
		
		// Check if this validator is active in any partition
		isActive := false
		for _, partition := range validatorInfo.Partitions {
			if partition.Active {
				isActive = true
				break
			}
		}
		
		if !isActive {
			logger.Printf("Skipping inactive validator: %s", operatorID)
			continue
		}
		
		// Add to DN validators
		validator := NewValidator(operatorID, operatorID, "dn")
		addressDir.AddDNValidator(validator)
		logger.Printf("Added DN validator: %s", operatorID)
	}

	// Add some network peers (including non-validators)
	logger.Printf("Adding network peers to AddressDir")
	
	// Add validator peers
	for i, validatorInfo := range networkStatus.Network.Validators {
		// Only process the first few validators to keep the test quick
		if i >= 5 {
			break
		}
		
		operatorID := validatorInfo.Operator.String()
		peer := NetworkPeer{
			ID:          operatorID,
			IsValidator: true,
			ValidatorID: operatorID,
			Addresses:   []string{fmt.Sprintf("/ip4/192.168.1.%d/tcp/16593", i+1)},
			LastSeen:    time.Now(),
			Active:      true,
		}
		addressDir.AddNetworkPeer(peer)
	}
	
	// Add non-validator peers
	for i := 0; i < 10; i++ {
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

	// Log peer counts
	logger.Printf("Added %d DN validators", len(addressDir.DNValidators))
	logger.Printf("Added %d network peers", len(addressDir.NetworkPeers))
	
	// Find non-validator nodes
	logger.Printf("\nNon-Validator Nodes:")
	nonValidatorCount := 0
	for id, peer := range addressDir.NetworkPeers {
		if !peer.IsValidator {
			nonValidatorCount++
			logger.Printf("  Peer %d: %s", nonValidatorCount, id)
			logger.Printf("    Last Seen: %s", peer.LastSeen.Format(time.RFC3339))
			logger.Printf("    Addresses: %v", peer.Addresses)
		}
	}
	
	logger.Printf("\nFound %d non-validator nodes out of %d total peers", nonValidatorCount, len(addressDir.NetworkPeers))
	
	// Verify that we found the expected number of non-validator nodes
	assert.Equal(t, 10, nonValidatorCount, "Should find 10 non-validator nodes")
	
	// Log test completion
	logger.Printf("Non-validator nodes test completed successfully")
}
