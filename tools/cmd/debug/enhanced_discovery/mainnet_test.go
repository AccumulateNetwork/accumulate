// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package enhanced_discovery

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

// TestMainnetBasicConnectivity tests basic connectivity to mainnet
// This test is skipped by default when running with -short flag
// Run with: go test -v -run TestMainnetBasicConnectivity
// Skip with: go test -v -short -run TestMainnetBasicConnectivity
func TestMainnetBasicConnectivity(t *testing.T) {
	// Skip in short mode (for quick test runs)
	if testing.Short() {
		t.Skip("Skipping mainnet test in short mode. Run without -short flag to test against mainnet.")
	}

	// Create a logger
	logger := log.New(os.Stdout, "[MAINNET-TEST] ", log.LstdFlags)
	logger.Printf("Starting mainnet basic connectivity test")

	// Create a client for mainnet
	endpoint := "https://mainnet.accumulatenetwork.io/v3"
	logger.Printf("Using API endpoint: %s", endpoint)
	client := jsonrpc.NewClient(endpoint)
	
	// Set a reasonable timeout for the context
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Query network status
	logger.Printf("Querying network status")
	networkStatus, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	require.NoError(t, err, "Failed to query network status")

	// Verify network information
	assert.NotNil(t, networkStatus, "Network status should not be nil")
	assert.NotNil(t, networkStatus.Network, "Network definition should not be nil")
	logger.Printf("Network name: %s", networkStatus.Network.NetworkName)

	// Check partitions
	assert.NotEmpty(t, networkStatus.Network.Partitions, "Partitions should not be empty")
	logger.Printf("Found %d partitions", len(networkStatus.Network.Partitions))
	for i, partition := range networkStatus.Network.Partitions {
		logger.Printf("Partition %d: %s (Type: %s)", i+1, partition.ID, partition.Type)
	}

	// Check validators
	assert.NotEmpty(t, networkStatus.Network.Validators, "Validators should not be empty")
	logger.Printf("Found %d validators", len(networkStatus.Network.Validators))
	for i, validator := range networkStatus.Network.Validators {
		logger.Printf("Validator %d: %s", i+1, validator.Operator.String())
		logger.Printf("  Partitions: %d", len(validator.Partitions))
		for j, partition := range validator.Partitions {
			logger.Printf("    Partition %d: %s (Active: %v)", j+1, partition.ID, partition.Active)
		}
	}

	// Compare with our enhanced discovery implementation
	logger.Printf("\nComparing with enhanced discovery implementation")

	// Create an address directory
	addressDir := NewAddressDir()
	addressDir.Logger = logger

	// Create a network discovery instance
	discovery := &NetworkDiscoveryImpl{
		AddressDir: addressDir,
		Logger:     logger,
	}

	// Initialize the network for mainnet
	logger.Printf("Initializing network for mainnet")
	err = discovery.InitializeNetwork("mainnet")
	require.NoError(t, err, "Failed to initialize network")

	// Verify network information
	assert.Equal(t, "mainnet", discovery.Network.Name, "Network name should be mainnet")
	assert.Equal(t, "acme", discovery.Network.ID, "Network ID should be acme")
	assert.True(t, discovery.Network.IsMainnet, "IsMainnet should be true")
	assert.Equal(t, endpoint, discovery.Network.APIEndpoint, "API endpoint should match")

	// Verify partitions
	assert.NotEmpty(t, discovery.Network.Partitions, "Partitions should not be empty")
	logger.Printf("Enhanced discovery found %d partitions", len(discovery.Network.Partitions))
	for i, partition := range discovery.Network.Partitions {
		logger.Printf("Partition %d: %s (Type: %s, URL: %s)", 
			i+1, partition.ID, partition.Type, partition.URL)
	}

	// Log test completion
	logger.Printf("Mainnet basic connectivity test completed successfully")
}
