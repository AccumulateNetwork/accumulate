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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkDiscoveryInitialization(t *testing.T) {
	// Create an address directory
	addressDir := NewAddressDir()
	
	// Create a logger
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	
	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)
	
	// Initialize for testnet
	err := discovery.InitializeNetwork("testnet")
	require.NoError(t, err)
	
	// Verify network information
	assert.Equal(t, "testnet", discovery.Network.Name)
	assert.Equal(t, "acme", discovery.Network.ID)
	assert.False(t, discovery.Network.IsMainnet)
	assert.Equal(t, "https://testnet.accumulatenetwork.io/v3", discovery.Network.APIEndpoint)
	
	// Verify partitions
	require.Len(t, discovery.Network.Partitions, 2)
	assert.Equal(t, "dn", discovery.Network.Partitions[0].ID)
	assert.Equal(t, "bvn-Apollo", discovery.Network.Partitions[1].ID)
	
	// Verify partition map
	require.Len(t, discovery.Network.PartitionMap, 2)
	assert.Equal(t, "dn", discovery.Network.PartitionMap["dn"].ID)
	assert.Equal(t, "bvn-Apollo", discovery.Network.PartitionMap["bvn-Apollo"].ID)
}

func TestNetworkDiscoveryWithMockService(t *testing.T) {
	// Create an address directory
	addressDir := NewAddressDir()
	
	// Create a logger
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	
	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)
	
	// Initialize for testnet
	err := discovery.InitializeNetwork("testnet")
	require.NoError(t, err)
	
	// Create a mock network service
	mockService := NewMockNetworkService()
	
	// Replace the client with our mock
	discovery.Client = mockService
	
	// Test network status retrieval
	ctx := context.Background()
	networkStatus, err := discovery.GetNetworkStatus(ctx)
	require.NoError(t, err)
	require.NotNil(t, networkStatus)
	
	// Verify network status
	assert.Equal(t, "testnet", networkStatus.Network.NetworkName)
	require.Len(t, networkStatus.Network.Validators, 3)
}

func TestNetworkDiscoveryNetworkDiscovery(t *testing.T) {
	// Create an address directory
	addressDir := NewAddressDir()
	
	// Create a logger
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	
	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)
	
	// Initialize for testnet
	err := discovery.InitializeNetwork("testnet")
	require.NoError(t, err)
	
	// Create a mock network service
	mockService := NewMockNetworkService()
	
	// Test network discovery
	ctx := context.Background()
	stats, err := discovery.DiscoverNetworkPeers(ctx, mockService)
	require.NoError(t, err)
	
	// Verify discovery statistics
	assert.Greater(t, stats.TotalValidators, 0)
	assert.Greater(t, stats.DNValidators, 0)
	
	// Verify that validators were added to the address directory
	assert.Greater(t, len(addressDir.DNValidators), 0)
	
	// Log the statistics
	t.Logf("Discovery Statistics:\n%s", FormatDiscoveryStats(stats))
}

func TestNetworkDiscoveryNetworkValidation(t *testing.T) {
	// Create a test address directory with sample data
	addressDir := CreateTestAddressDir()
	
	// Create a logger
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	
	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)
	
	// Initialize for testnet
	err := discovery.InitializeNetwork("testnet")
	require.NoError(t, err)
	
	// Create a mock network service
	mockService := NewMockNetworkService()
	
	// Replace the client with our mock
	discovery.Client = mockService
	
	// Test network validation
	ctx := context.Background()
	stats, err := discovery.ValidateNetwork(ctx)
	require.NoError(t, err)
	
	// Verify validation statistics
	assert.Greater(t, stats.TotalValidators, 0)
	assert.Greater(t, stats.DNValidators, 0)
	
	// Log the statistics
	t.Logf("Validation Statistics:\n%s", FormatDiscoveryStats(stats))
}

func TestAddressValidation(t *testing.T) {
	// Create an address directory
	addressDir := NewAddressDir()
	
	// Create a logger
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	
	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)
	
	// Create a validator for testing
	validator := CreateMockValidator("test-validator", "dn")
	
	// Test cases
	testCases := []struct {
		name        string
		addressType string
		address     string
		expectValid bool
	}{
		{
			name:        "Valid P2P Address",
			addressType: "p2p",
			address:     "tcp://127.0.0.1:26656",
			expectValid: true,
		},
		{
			name:        "Invalid P2P Address",
			addressType: "p2p",
			address:     "http://127.0.0.1:26656",
			expectValid: false,
		},
		{
			name:        "Valid RPC Address",
			addressType: "rpc",
			address:     "http://127.0.0.1:26657",
			expectValid: true,
		},
		{
			name:        "Valid HTTPS RPC Address",
			addressType: "rpc",
			address:     "https://127.0.0.1:26657",
			expectValid: true,
		},
		{
			name:        "Invalid RPC Address",
			addressType: "rpc",
			address:     "tcp://127.0.0.1:26657",
			expectValid: false,
		},
		{
			name:        "Valid API Address",
			addressType: "api",
			address:     "http://127.0.0.1:8080",
			expectValid: true,
		},
		{
			name:        "Valid Metrics Address",
			addressType: "metrics",
			address:     "http://127.0.0.1:26660",
			expectValid: true,
		},
		{
			name:        "Unknown Address Type",
			addressType: "unknown",
			address:     "http://127.0.0.1:8080",
			expectValid: false,
		},
	}
	
	// Run test cases
	ctx := context.Background()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := discovery.validateAddress(ctx, validator, tc.addressType, tc.address)
			require.NoError(t, err)
			assert.Equal(t, tc.expectValid, result.Success)
			
			if !result.Success {
				t.Logf("Validation error: %s", result.Error)
			}
		})
	}
}

func TestURLStandardization(t *testing.T) {
	// Create an address directory
	addressDir := NewAddressDir()
	
	// Create a logger
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	
	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)
	
	// Test cases
	testCases := []struct {
		name        string
		urlType     string
		partitionID string
		baseURL     string
		expected    string
	}{
		{
			name:        "DN Partition URL",
			urlType:     "partition",
			partitionID: "dn",
			baseURL:     "",
			expected:    "acc://dn.acme",
		},
		{
			name:        "BVN Partition URL",
			urlType:     "partition",
			partitionID: "Apollo",
			baseURL:     "",
			expected:    "acc://bvn-Apollo.acme",
		},
		{
			name:        "DN Anchor URL",
			urlType:     "anchor",
			partitionID: "dn",
			baseURL:     "",
			expected:    "acc://dn.acme/anchors",
		},
		{
			name:        "BVN Anchor URL",
			urlType:     "anchor",
			partitionID: "Apollo",
			baseURL:     "",
			expected:    "acc://dn.acme/anchors/Apollo",
		},
		{
			name:        "API URL",
			urlType:     "api",
			partitionID: "",
			baseURL:     "http://127.0.0.1:8080",
			expected:    "http://127.0.0.1:8080",
		},
	}
	
	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := discovery.standardizeURL(tc.urlType, tc.partitionID, tc.baseURL)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestWellKnownEndpoints(t *testing.T) {
	// Test cases
	testCases := []struct {
		name     string
		network  string
		service  string
		expected string
	}{
		{
			name:     "Mainnet v3",
			network:  "mainnet",
			service:  "v3",
			expected: "https://mainnet.accumulatenetwork.io/v3",
		},
		{
			name:     "Testnet v3",
			network:  "testnet",
			service:  "v3",
			expected: "https://testnet.accumulatenetwork.io/v3",
		},
		{
			name:     "Devnet v3",
			network:  "devnet",
			service:  "v3",
			expected: "https://devnet.accumulatenetwork.io/v3",
		},
		{
			name:     "Custom Network v3",
			network:  "custom",
			service:  "v3",
			expected: "https://custom.accumulatenetwork.io/v3",
		},
	}
	
	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ResolveWellKnownEndpoint(tc.network, tc.service)
			assert.Equal(t, tc.expected, result)
		})
	}
}
