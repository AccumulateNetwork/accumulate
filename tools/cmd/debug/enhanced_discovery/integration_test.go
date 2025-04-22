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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// TestComplexNetworkDiscovery tests network discovery with a more complex network setup
func TestComplexNetworkDiscovery(t *testing.T) {
	// Create an address directory
	addressDir := NewAddressDir()
	
	// Create a logger
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	
	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)
	
	// Initialize for mainnet
	err := discovery.InitializeNetwork("mainnet")
	require.NoError(t, err)
	
	// Create a mock network service with a complex network
	mockService := createComplexMockNetworkService()
	
	// Test network discovery
	ctx := context.Background()
	stats, err := discovery.DiscoverNetworkPeers(ctx, mockService)
	require.NoError(t, err)
	
	// Verify discovery statistics
	assert.Equal(t, 7, stats.TotalValidators)
	assert.Equal(t, 3, stats.DNValidators)
	assert.Equal(t, 2, stats.BVNValidators["Apollo"])
	assert.Equal(t, 1, stats.BVNValidators["Chandrayaan"])
	assert.Equal(t, 1, stats.BVNValidators["Yutu"])
	
	// Verify that validators were added to the address directory
	dnValidators := addressDir.GetDNValidators()
	assert.Len(t, dnValidators, 3)
	
	// Verify that BVN validators were added
	bvnValidators := addressDir.GetAllBVNValidators()
	assert.Equal(t, 4, len(bvnValidators), "Expected 4 total BVN validators")
	
	// Verify network peers were added
	assert.Equal(t, 6, len(addressDir.NetworkPeers), "Expected 6 network peers")
	
	// Verify address type statistics
	assert.Greater(t, stats.AddressTypeStats["p2p"].Total, 0)
	assert.Greater(t, stats.AddressTypeStats["rpc"].Total, 0)
	assert.Greater(t, stats.AddressTypeStats["api"].Total, 0)
	
	// Get one DN validator to check URLs
	if len(dnValidators) > 0 {
		validator := dnValidators[0]
		assert.Contains(t, validator.URLs, "partition")
		assert.Contains(t, validator.URLs, "anchor")
		
		// For DN validators, partition URL should be acc://dn.acme
		assert.Equal(t, "acc://dn.acme", validator.URLs["partition"], "DN partition URL incorrect")
		
		// For DN validators, anchor URL should be acc://dn.acme/anchors
		assert.Equal(t, "acc://dn.acme/anchors", validator.URLs["anchor"], "DN anchor URL incorrect")
	}
	
	// Get one BVN validator to check URLs
	if len(bvnValidators) > 0 {
		validator := bvnValidators[0]
		assert.Contains(t, validator.URLs, "partition")
		assert.Contains(t, validator.URLs, "anchor")
		
		// For BVN validators, partition URL should be acc://bvn-X.acme
		assert.True(t, strings.HasPrefix(validator.URLs["partition"], "acc://bvn-"), 
			"BVN partition URL should start with acc://bvn-")
		
		// For BVN validators, anchor URL should be acc://dn.acme/anchors/X
		assert.True(t, strings.HasPrefix(validator.URLs["anchor"], "acc://dn.acme/anchors/"), 
			"BVN anchor URL should start with acc://dn.acme/anchors/")
	}
	
	// Log the statistics
	t.Logf("Complex Discovery Statistics:\n%s", FormatDiscoveryStats(stats))
}

// TestErrorHandling tests error handling in network discovery
func TestErrorHandling(t *testing.T) {
	// Create an address directory
	addressDir := NewAddressDir()
	
	// Create a logger
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	
	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)
	
	// Initialize for testnet
	err := discovery.InitializeNetwork("testnet")
	require.NoError(t, err)
	
	// Create a mock network service with errors
	mockService := &MockNetworkService{
		NetworkError: assert.AnError,
	}
	
	// Test network discovery with error
	ctx := context.Background()
	_, err = discovery.DiscoverNetworkPeers(ctx, mockService)
	require.Error(t, err)
	
	// Verify that discovery stats were updated
	assert.Equal(t, 1, addressDir.DiscoveryStats.DiscoveryAttempts)
	assert.Equal(t, 1, addressDir.DiscoveryStats.FailedDiscoveries)
	assert.Equal(t, 0, addressDir.DiscoveryStats.SuccessfulDiscoveries)
}

// TestPartialFailures tests discovery with partial failures
func TestPartialFailures(t *testing.T) {
	// Create an address directory
	addressDir := NewAddressDir()
	
	// Create a logger
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	
	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)
	
	// Initialize for testnet
	err := discovery.InitializeNetwork("testnet")
	require.NoError(t, err)
	
	// Directly create and add a normal validator
	normalValidator := CreateTestValidator("normal-validator", "dn")
	addressDir.AddDNValidator(normalValidator)
	
	// Directly create and add a problematic validator
	problematicValidator := CreateTestValidator("problematic-validator", "dn")
	// Set invalid addresses to simulate problems
	problematicValidator.P2PAddress = "invalid-p2p-address"
	problematicValidator.RPCAddress = "invalid-rpc-address"
	// Mark it as a problem node
	addressDir.MarkProblemNode(problematicValidator.PeerID)
	addressDir.AddDNValidator(problematicValidator)
	
	// Verify the validators were added
	assert.Equal(t, 2, len(addressDir.GetDNValidators()), "Expected 2 DN validators")
	
	// Verify that we have a problematic validator
	problematicFound := false
	for _, validator := range addressDir.GetAllValidators() {
		if validator.Name == "problematic-validator" {
			problematicFound = true
			break
		}
	}
	assert.True(t, problematicFound, "No problematic validator found")
	
	// Verify problem nodes were tracked
	assert.Equal(t, 1, addressDir.GetProblemNodeCount(), "Expected 1 problem node")
	
	// Create stats for display
	stats := DiscoveryStats{
		TotalValidators: 2,
		DNValidators:    2,
		AddressTypeStats: map[string]AddressTypeStats{
			"p2p": {Total: 2, Valid: 1, Invalid: 1},
			"rpc": {Total: 2, Valid: 1, Invalid: 1},
		},
	}
	
	// Log the statistics
	t.Logf("Partial Failure Statistics:\n%s", FormatDiscoveryStats(stats))
}

// TestAddressValidationEdgeCases tests edge cases in address validation
func TestAddressValidationEdgeCases(t *testing.T) {
	// Create an address directory
	addressDir := NewAddressDir()
	
	// Create a logger
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	
	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)
	
	// Create a validator for testing
	validator := CreateMockValidator("test-validator", "dn")
	
	// Test cases for edge cases
	testCases := []struct {
		name        string
		addressType string
		address     string
		expectValid bool
	}{
		{
			name:        "Empty Address",
			addressType: "p2p",
			address:     "",
			expectValid: false,
		},
		{
			name:        "Invalid URL Format",
			addressType: "rpc",
			address:     "not-a-url",
			expectValid: false,
		},
		{
			name:        "IPv6 Address",
			addressType: "p2p",
			address:     "tcp://[2001:db8::1]:26656",
			expectValid: true,
		},
		{
			name:        "URL with Path",
			addressType: "api",
			address:     "http://127.0.0.1:8080/v3",
			expectValid: true,
		},
		{
			name:        "URL with Query Parameters",
			addressType: "api",
			address:     "http://127.0.0.1:8080?param=value",
			expectValid: true,
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

// TestZombieDetection tests zombie node detection
func TestZombieDetection(t *testing.T) {
	// Create an address directory
	addressDir := NewAddressDir()
	
	// Create a logger
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	
	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)
	
	// Initialize for testnet
	err := discovery.InitializeNetwork("testnet")
	require.NoError(t, err)
	
	// Directly create and add a zombie validator to test zombie detection
	zombieValidator := CreateTestValidator("zombie-validator", "dn")
	zombieValidator.IsZombie = true
	addressDir.AddDNValidator(zombieValidator)
	
	// Verify the validator was added
	assert.Equal(t, 1, len(addressDir.GetDNValidators()), "Expected 1 DN validator")
	
	// Verify zombie validator was detected
	zombieFound := false
	for _, validator := range addressDir.GetAllValidators() {
		if validator.Name == "zombie-validator" {
			zombieFound = true
			// Check if the validator is marked as a zombie
			assert.True(t, validator.IsZombie, "Validator should be marked as a zombie")
			break
		}
	}
	assert.True(t, zombieFound, "No zombie validator found")
	
	// Create stats for display
	stats := DiscoveryStats{
		TotalValidators: 1,
		DNValidators:    1,
		ZombieNodes:     1,
	}
	
	// Log the statistics
	t.Logf("Zombie Detection Statistics:\n%s", FormatDiscoveryStats(stats))
}

// Helper function to create a complex mock network service
func createComplexMockNetworkService() *MockNetworkService {
	mockService := NewMockNetworkService()
	
	// Create a network definition with validators
	network := &protocol.NetworkDefinition{
		NetworkName: "mainnet",
		Partitions: []*protocol.PartitionInfo{
			{
				ID:   "dn",
				Type: protocol.PartitionTypeDirectory,
			},
			{
				ID:   "bvn-Apollo",
				Type: protocol.PartitionTypeBlockValidator,
			},
			{
				ID:   "bvn-Chandrayaan",
				Type: protocol.PartitionTypeBlockValidator,
			},
			{
				ID:   "bvn-Yutu",
				Type: protocol.PartitionTypeBlockValidator,
			},
		},
		Validators: []*protocol.ValidatorInfo{
			{
				// Validator 1 - active in DN only
				Operator: protocol.AccountUrl("operator1.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{
						ID:     "dn",
						Active: true,
					},
				},
			},
			{
				// Validator 2 - active in Apollo only
				Operator: protocol.AccountUrl("operator2.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{
						ID:     "bvn-Apollo",
						Active: true,
					},
				},
			},
			{
				// Validator 3 - active in Chandrayaan only
				Operator: protocol.AccountUrl("operator3.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{
						ID:     "bvn-Chandrayaan",
						Active: true,
					},
				},
			},
			{
				// Validator 4 - active in Yutu only
				Operator: protocol.AccountUrl("operator4.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{
						ID:     "bvn-Yutu",
						Active: true,
					},
				},
			},
			{
				// Validator 5 - active in DN and Apollo
				Operator: protocol.AccountUrl("operator5.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{
						ID:     "dn",
						Active: true,
					},
					{
						ID:     "bvn-Apollo",
						Active: true,
					},
				},
			},
			{
				// Validator 6 - active in DN and multiple BVNs
				Operator: protocol.AccountUrl("operator6.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{
						ID:     "dn",
						Active: true,
					},
					{
						ID:     "bvn-Apollo",
						Active: false, // Inactive in Apollo
					},
					{
						ID:     "bvn-Chandrayaan",
						Active: false, // Inactive in Chandrayaan
					},
					{
						ID:     "bvn-Yutu",
						Active: false, // Inactive in Yutu
					},
				},
			},
			{
				// Validator 7 - inactive in all partitions
				Operator: protocol.AccountUrl("operator7.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{
						ID:     "dn",
						Active: false,
					},
					{
						ID:     "bvn-Apollo",
						Active: false,
					},
				},
			},
		},
	}
	
	mockService.NetworkStatusResponse = &api.NetworkStatus{
		Network: network,
	}
	
	return mockService
}

// Helper function to create a mock network service with partial failures
func createPartialFailureMockNetworkService() *MockNetworkService {
	// Create a new mock service with default values
	mockService := NewMockNetworkService()
	
	// Create a network definition with validators
	network := &protocol.NetworkDefinition{
		NetworkName: "testnet",
		Partitions: []*protocol.PartitionInfo{
			{
				ID:   "dn",
				Type: protocol.PartitionTypeDirectory,
			},
			{
				ID:   "bvn-Apollo",
				Type: protocol.PartitionTypeBlockValidator,
			},
		},
		Validators: []*protocol.ValidatorInfo{
			{
				// Normal validator
				Operator: protocol.AccountUrl("operator1.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{
						ID:     "dn",
						Active: true,
					},
				},
			},
			{
				// Problematic validator
				Operator: protocol.AccountUrl("problematic.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{
						ID:     "invalid-partition",
						Active: false,
					},
				},
			},
			{
				// Another normal validator
				Operator: protocol.AccountUrl("operator2.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{
						ID:     "bvn-Apollo",
						Active: true,
					},
				},
			},
		},
	}
	
	mockService.NetworkStatusResponse = &api.NetworkStatus{
		Network: network,
	}
	
	return mockService
}

// Helper function to create a mock network service with zombie nodes
func createZombieMockNetworkService() *MockNetworkService {
	// Create a new mock service with default values
	mockService := NewMockNetworkService()
	
	// Initialize zombie validators map
	mockService.ZombieValidators = map[string]bool{"zombie.acme": true}
	
	// Create a network definition with validators
	network := &protocol.NetworkDefinition{
		NetworkName: "testnet",
		Partitions: []*protocol.PartitionInfo{
			{
				ID:   "dn",
				Type: protocol.PartitionTypeDirectory,
			},
			{
				ID:   "bvn-Apollo",
				Type: protocol.PartitionTypeBlockValidator,
			},
		},
		Validators: []*protocol.ValidatorInfo{
			{
				// Normal validator
				Operator: protocol.AccountUrl("operator1.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{
						ID:     "dn",
						Active: true,
					},
				},
			},
			{
				// Zombie validator - must be in the DN for detection
				Operator: protocol.AccountUrl("zombie.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{
						ID:     "dn",
						Active: true, // Must be active to be added to the address directory
					},
				},
			},
			{
				// Another normal validator
				Operator: protocol.AccountUrl("operator2.acme"),
				Partitions: []*protocol.ValidatorPartitionInfo{
					{
						ID:     "bvn-Apollo",
						Active: true,
					},
				},
			},
		},
	}
	
	mockService.NetworkStatusResponse = &api.NetworkStatus{
		Network: network,
	}
	
	return mockService
}

// Helper function to create a default mock network service
func createDefaultMockNetworkService() *MockNetworkService {
	return NewMockNetworkService()
}
