// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// createMockNetworkStatusForTest creates a mock network status for test purposes
// This is different from the CreateMockNetworkService function in discovery_mocks_test.go

// createMockNetworkStatus creates a mock network status response
func createMockNetworkStatus() *api.NetworkStatus {
	// Create a mock network definition
	network := &protocol.NetworkDefinition{
		NetworkName: "testnet",
		Partitions: []*protocol.PartitionInfo{
			{
				ID:   "dn",
				Type: protocol.PartitionTypeDirectory,
			},
			{
				ID:   "bvn-apollo",
				Type: protocol.PartitionTypeBlockValidator,
			},
		},
		Validators: []*protocol.ValidatorInfo{
			{
				// Validator 1 - active in DN
				Operator: protocol.AccountUrl("operator1.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{
						ID:     "dn",
						Active: true,
					},
				},
			},
			{
				// Validator 2 - active in BVN
				Operator: protocol.AccountUrl("operator2.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{
						ID:     "bvn-apollo",
						Active: true,
					},
				},
			},
			{
				// Validator 3 - active in both
				Operator: protocol.AccountUrl("operator3.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{
						ID:     "dn",
						Active: true,
					},
					{
						ID:     "bvn-apollo",
						Active: true,
					},
				},
			},
		},
	}

	// Create the network status response
	return &api.NetworkStatus{
		Network: network,
	}
}

// TestNetworkDiscovery tests the network discovery functionality
func TestNetworkDiscovery(t *testing.T) {
	// Create a logger
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)

	// Create an address directory
	addressDir := &AddressDir{
		NetworkPeers:   make(map[string]NetworkPeer),
		URLHelpers:     make(map[string]string),
		DiscoveryStats: NewDiscoveryStats(),
	}
	addressDir.Logger = logger

	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)

	// Initialize the network
	err := discovery.InitializeNetwork("testnet")
	require.NoError(t, err, "Failed to initialize network")

	// Verify network information
	network := addressDir.GetNetwork()
	assert.Equal(t, "testnet", network.Name, "Network name mismatch")
	assert.Equal(t, "acme", network.ID, "Network ID mismatch")
	assert.False(t, network.IsMainnet, "Network should not be mainnet")
	assert.NotEmpty(t, network.APIEndpoint, "API endpoint should not be empty")

	// Verify partitions
	assert.NotEmpty(t, network.Partitions, "Partitions should not be empty")
	assert.NotEmpty(t, network.PartitionMap, "Partition map should not be empty")

	// Create a mock network service
	mockService := CreateMockNetworkService()

	// Discover network peers
	stats, err := discovery.DiscoverNetworkPeers(context.Background(), mockService)
	require.NoError(t, err, "Failed to discover network peers")

	// Verify discovery statistics
	assert.True(t, stats.TotalPeers > 0, "Should discover at least one peer")
	assert.True(t, stats.DNValidators > 0, "Should discover at least one DN validator")

	// Verify address directory was updated
	assert.NotEmpty(t, addressDir.DNValidators, "DN validators should not be empty")

	// Generate and print a report of the discovered network
	if testing.Verbose() {
		t.Log("Generating network report...")
		var buf bytes.Buffer
		GenerateAddressDirReport(addressDir, &buf)
		t.Log("\n" + buf.String())
	}
}

// TestNetworkDiscoveryMainnet tests the network discovery functionality for mainnet
func TestNetworkDiscoveryMainnet(t *testing.T) {
	// Create a logger
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)

	// Create an address directory
	addressDir := &AddressDir{
		NetworkPeers:   make(map[string]NetworkPeer),
		URLHelpers:     make(map[string]string),
		DiscoveryStats: NewDiscoveryStats(),
	}
	addressDir.Logger = logger

	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)

	// Initialize the network
	err := discovery.InitializeNetwork("mainnet")
	require.NoError(t, err, "Failed to initialize network")

	// Verify network information
	network := addressDir.GetNetwork()
	assert.Equal(t, "mainnet", network.Name, "Network name mismatch")
	assert.Equal(t, "acme", network.ID, "Network ID mismatch")
	assert.True(t, network.IsMainnet, "Network should be mainnet")
	assert.NotEmpty(t, network.APIEndpoint, "API endpoint should not be empty")

	// Verify partitions
	assert.Equal(t, 4, len(network.Partitions), "Should have 4 partitions (DN + 3 BVNs)")
	assert.Equal(t, 4, len(network.PartitionMap), "Should have 4 partitions in map")

	// We already have the network information from earlier
	
	// Verify DN partition
	dnPartition, ok := network.PartitionMap["dn"]
	assert.True(t, ok, "DN partition should exist")
	assert.Equal(t, "dn", dnPartition.ID, "DN partition ID mismatch")
	assert.Equal(t, "dn", dnPartition.Type, "DN partition type mismatch")
	assert.Equal(t, "acc://dn.acme", dnPartition.URL, "DN partition URL mismatch")
	assert.True(t, dnPartition.Active, "DN partition should be active")
	assert.Equal(t, -1, dnPartition.BVNIndex, "DN partition BVN index mismatch")

	// Verify BVN partitions
	bvnNames := []string{"Apollo", "Chandrayaan", "Yutu"}
	for i, name := range bvnNames {
		bvnPartition, ok := network.PartitionMap[name]
		assert.True(t, ok, "%s partition should exist", name)
		assert.Equal(t, name, bvnPartition.ID, "%s partition ID mismatch", name)
		assert.Equal(t, "bvn", bvnPartition.Type, "%s partition type mismatch", name)
		assert.Equal(t, fmt.Sprintf("acc://bvn-%s.acme", name), bvnPartition.URL, "%s partition URL mismatch", name)
		assert.True(t, bvnPartition.Active, "%s partition should be active", name)
		assert.Equal(t, i, bvnPartition.BVNIndex, "%s partition BVN index mismatch", name)
	}

	// Generate and print a report of the discovered network
	if testing.Verbose() {
		t.Log("Generating mainnet network report...")
		var buf bytes.Buffer
		GenerateAddressDirReport(addressDir, &buf)
		t.Log("\n" + buf.String())
	}
}

// TestUpdateNetworkState tests the UpdateNetworkState function
func TestUpdateNetworkState(t *testing.T) {
	// Create a logger
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)

	// Create an address directory
	addressDir := &AddressDir{
		NetworkPeers:   make(map[string]NetworkPeer),
		URLHelpers:     make(map[string]string),
		DiscoveryStats: NewDiscoveryStats(),
	}
	addressDir.Logger = logger

	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)

	// Initialize the network
	err := discovery.InitializeNetwork("testnet")
	require.NoError(t, err, "Failed to initialize network")

	// Create a mock network service
	mockService := CreateMockNetworkService()

	// Update network state
	stats, err := discovery.UpdateNetworkState(context.Background(), mockService)
	require.NoError(t, err, "Failed to update network state")

	// Verify update statistics
	assert.True(t, stats.TotalPeers > 0, "Should discover at least one peer")
	assert.True(t, stats.DNValidators > 0, "Should discover at least one DN validator")

	// Verify discovery stats were updated
	assert.NotEqual(t, time.Time{}, addressDir.DiscoveryStats.LastDiscovery, "Last discovery time should be set")
	assert.True(t, addressDir.DiscoveryStats.DiscoveryAttempts > 0, "Discovery attempts should be incremented")
	assert.True(t, addressDir.DiscoveryStats.SuccessfulDiscoveries > 0, "Successful discoveries should be incremented")

	// Generate and print a report of the updated network state
	if testing.Verbose() {
		t.Log("Generating network state report...")
		var buf bytes.Buffer
		GenerateAddressDirReport(addressDir, &buf)
		t.Log("\n" + buf.String())
	}
}

// TestErrorHandling tests error handling during network discovery
func TestErrorHandling(t *testing.T) {
	// Create a logger
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)

	// Create an address directory
	addressDir := &AddressDir{
		NetworkPeers:   make(map[string]NetworkPeer),
		URLHelpers:     make(map[string]string),
		DiscoveryStats: NewDiscoveryStats(),
	}
	addressDir.Logger = logger

	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)

	// Initialize the network
	err := discovery.InitializeNetwork("testnet")
	require.NoError(t, err, "Failed to initialize network")

	// Create a mock network service with error
	mockService := CreateMockNetworkService()
	mockService.NetworkStatusError = fmt.Errorf("mock error")

	// Discover network peers
	stats, err := discovery.DiscoverNetworkPeers(context.Background(), mockService)
	// We expect an error but the function should still return stats
	assert.Error(t, err, "Should return an error")
	assert.Equal(t, 0, stats.TotalPeers, "Should not discover any peers")

	// Verify discovery stats were updated
	assert.NotEqual(t, time.Time{}, addressDir.DiscoveryStats.LastDiscovery, "Last discovery time should be set")
	assert.True(t, addressDir.DiscoveryStats.DiscoveryAttempts > 0, "Discovery attempts should be incremented")
	assert.True(t, addressDir.DiscoveryStats.FailedDiscoveries > 0, "Failed discoveries should be incremented")

	// Generate and print a report of the error state
	if testing.Verbose() {
		t.Log("Generating error handling report...")
		var buf bytes.Buffer
		GenerateAddressDirReport(addressDir, &buf)
		t.Log("\n" + buf.String())
	}
}
