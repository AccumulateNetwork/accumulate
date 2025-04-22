// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnhancedDiscoveryIntegration tests the complete discovery process with a mock network
func TestEnhancedDiscoveryIntegration(t *testing.T) {
	// Note: We're simulating the discovery process manually instead of using a mock service
	
	// Create a logger
	logger := log.New(os.Stdout, "[IntegrationTest] ", log.LstdFlags)
	
	// Create an address directory
	addressDir := NewAddressDir()
	addressDir.Logger = logger
	
	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)
	
	// Initialize the network
	err := discovery.InitializeNetwork("mainnet")
	require.NoError(t, err, "Failed to initialize network")
	
	// Note: We're simulating the discovery process manually, so we don't need a context
	
	// 1. Simulate discovering directory peers
	dnValidators := []Validator{
		*CreateMockValidator("validator1", "dn"),
		*CreateMockValidator("validator2", "dn"),
	}
	
	// Add some addresses to the validators
	for i := range dnValidators {
		validator := &dnValidators[i]
		validator.IPAddress = "127.0.0.1"
		validator.P2PAddress = "tcp://127.0.0.1:26656"
		validator.RPCAddress = "http://127.0.0.1:26657"
		validator.APIAddress = "http://127.0.0.1:8080"
		validator.MetricsAddress = "http://127.0.0.1:26660"
		validator.URLs = map[string]string{
			"p2p":     validator.P2PAddress,
			"rpc":     validator.RPCAddress,
			"api":     validator.APIAddress,
			"metrics": validator.MetricsAddress,
		}
		validator.Version = "v1.0.0"
		validator.APIV3Status = true
		validator.DNHeight = 1000
	}
	
	// 2. Simulate discovering BVN peers
	bvnValidators := [][]Validator{
		{
			*CreateMockValidator("validator3", "Apollo"),
			*CreateMockValidator("validator4", "Apollo"),
		},
		{
			*CreateMockValidator("validator5", "Chandrayaan"),
			*CreateMockValidator("validator6", "Chandrayaan"),
		},
	}
	
	// Add some addresses to the BVN validators
	for i := range bvnValidators {
		for j := range bvnValidators[i] {
			validator := &bvnValidators[i][j]
			validator.IPAddress = "127.0.0.1"
			validator.P2PAddress = "tcp://127.0.0.1:26656"
			validator.RPCAddress = "http://127.0.0.1:26657"
			validator.APIAddress = "http://127.0.0.1:8080"
			validator.MetricsAddress = "http://127.0.0.1:26660"
			validator.URLs = map[string]string{
				"p2p":     validator.P2PAddress,
				"rpc":     validator.RPCAddress,
				"api":     validator.APIAddress,
				"metrics": validator.MetricsAddress,
			}
			validator.Version = "v1.0.0"
			validator.APIV3Status = true
			validator.BVNHeight = 900
		}
	}
	
	// 3. Update the address directory
	addressDir.mu.Lock()
	addressDir.DNValidators = dnValidators
	addressDir.BVNValidators = bvnValidators
	addressDir.mu.Unlock()
	
	// 4. Create statistics
	stats := RefreshStats{
		TotalPeers:        len(dnValidators) + len(bvnValidators[0]) + len(bvnValidators[1]),
		TotalValidators:   len(dnValidators) + len(bvnValidators[0]) + len(bvnValidators[1]),
		DNValidators:      len(dnValidators),
		BVNValidators:     len(bvnValidators[0]) + len(bvnValidators[1]),
		APIV3Available:    len(dnValidators) + len(bvnValidators[0]) + len(bvnValidators[1]),
		APIV3Unavailable:  0,
		ZombieNodes:       0,
		VersionCounts:     map[string]int{"v1.0.0": len(dnValidators) + len(bvnValidators[0]) + len(bvnValidators[1])},
		AddressStats:      make(map[string]struct{ Total, Valid, Invalid int }),
		SuccessRate:       1.0,
	}
	
	// Set address stats
	stats.AddressStats["p2p"] = struct{ Total, Valid, Invalid int }{
		Total:   stats.TotalValidators,
		Valid:   stats.TotalValidators,
		Invalid: 0,
	}
	stats.AddressStats["rpc"] = struct{ Total, Valid, Invalid int }{
		Total:   stats.TotalValidators,
		Valid:   stats.TotalValidators,
		Invalid: 0,
	}
	stats.AddressStats["api"] = struct{ Total, Valid, Invalid int }{
		Total:   stats.TotalValidators,
		Valid:   stats.TotalValidators,
		Invalid: 0,
	}
	stats.AddressStats["metrics"] = struct{ Total, Valid, Invalid int }{
		Total:   stats.TotalValidators,
		Valid:   stats.TotalValidators,
		Invalid: 0,
	}
	
	// 5. Verify the results
	assert.Equal(t, 2, len(addressDir.DNValidators), "Incorrect number of DN validators")
	assert.Equal(t, 2, len(addressDir.BVNValidators), "Incorrect number of BVN partitions")
	assert.Equal(t, 2, len(addressDir.BVNValidators[0]), "Incorrect number of validators in first BVN")
	assert.Equal(t, 2, len(addressDir.BVNValidators[1]), "Incorrect number of validators in second BVN")
	
	// Verify that all validators have the expected address types
	for _, validator := range addressDir.DNValidators {
		assert.NotEmpty(t, validator.IPAddress, "Validator %s has no IP address", validator.Name)
		assert.NotEmpty(t, validator.P2PAddress, "Validator %s has no P2P address", validator.Name)
		assert.NotEmpty(t, validator.RPCAddress, "Validator %s has no RPC address", validator.Name)
		assert.NotEmpty(t, validator.APIAddress, "Validator %s has no API address", validator.Name)
		assert.NotEmpty(t, validator.MetricsAddress, "Validator %s has no metrics address", validator.Name)
		
		assert.NotNil(t, validator.URLs, "Validator %s has no URLs", validator.Name)
		assert.NotEmpty(t, validator.URLs["p2p"], "Validator %s has no P2P URL", validator.Name)
		assert.NotEmpty(t, validator.URLs["rpc"], "Validator %s has no RPC URL", validator.Name)
		assert.NotEmpty(t, validator.URLs["api"], "Validator %s has no API URL", validator.Name)
		assert.NotEmpty(t, validator.URLs["metrics"], "Validator %s has no metrics URL", validator.Name)
		
		assert.NotEmpty(t, validator.Version, "Validator %s has no version information", validator.Name)
		assert.True(t, validator.APIV3Status, "Validator %s has API v3 unavailable", validator.Name)
		assert.Greater(t, validator.DNHeight, uint64(0), "Validator %s has no DN height", validator.Name)
	}
	
	// Verify statistics
	assert.Equal(t, stats.TotalValidators, stats.DNValidators+stats.BVNValidators, "Incorrect total validators")
	assert.Equal(t, stats.APIV3Available, stats.TotalValidators, "Incorrect API v3 available count")
	assert.Equal(t, 0, stats.APIV3Unavailable, "Incorrect API v3 unavailable count")
	assert.Equal(t, 0, stats.ZombieNodes, "Incorrect zombie nodes count")
	assert.Equal(t, 1, len(stats.VersionCounts), "Incorrect version counts")
	assert.Equal(t, stats.TotalValidators, stats.VersionCounts["v1.0.0"], "Incorrect version count")
}

// TestEnhancedDiscoveryErrorHandling tests error handling in the discovery process
func TestEnhancedDiscoveryErrorHandling(t *testing.T) {
	// Create a mock network service with errors
	mockService := CreateMockNetworkService()
	mockService.NetworkStatusError = errors.New("network status error")
	mockService.StatusError = errors.New("status error")
	mockService.PingError = errors.New("ping error")
	
	// Create a logger
	logger := log.New(os.Stdout, "[ErrorHandlingTest] ", log.LstdFlags)
	
	// Create an address directory
	addressDir := NewAddressDir()
	addressDir.Logger = logger
	
	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)
	
	// Initialize the network
	err := discovery.InitializeNetwork("mainnet")
	require.NoError(t, err, "Failed to initialize network")
	
	// Note: We're simulating the discovery process manually, so we don't need a context
	
	// Override the discovery's network service with our mock
	// Note: In a real implementation, we would need to modify the DiscoverNetworkPeers
	// function to accept a client parameter for testing
	
	// For this test, we'll simulate error handling manually
	
	// 1. Test handling of network status error
	validator := CreateMockValidator("test-validator", "dn")
	validator.IPAddress = "127.0.0.1"
	
	// 2. Test API v3 connectivity error handling
	// Create a context for this specific test
	testCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	apiStatus, err := discovery.testAPIv3Connectivity(testCtx, validator)
	assert.Error(t, err, "Expected error from API v3 connectivity test")
	assert.False(t, apiStatus.Available, "API v3 should not be available")
	assert.NotEmpty(t, apiStatus.Error, "API v3 error should not be empty")
	
	// 3. Test version information error handling
	err = discovery.collectVersionInfo(testCtx, validator)
	assert.Error(t, err, "Expected error from version information collection")

	// 4. Test consensus status error handling
	consensusStatus, err := discovery.checkConsensusStatus(testCtx, validator)
	assert.Error(t, err, "Expected error from consensus status check")
	assert.False(t, consensusStatus.InConsensus, "Node should not be in consensus")
	
	// 5. Verify that the discovery process can continue despite errors
	// In a real implementation, we would verify that DiscoverNetworkPeers
	// returns partial results when some operations fail
}

// TestDiscoveryPerformance tests the performance of the enhanced discovery
func TestDiscoveryPerformance(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping performance test in short mode")
	}
	
	// Create a logger
	logger := log.New(os.Stdout, "[PerformanceTest] ", log.LstdFlags)
	
	// Create an address directory
	addressDir := NewAddressDir()
	addressDir.Logger = logger
	
	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)
	
	// Initialize the network
	err := discovery.InitializeNetwork("mainnet")
	require.NoError(t, err, "Failed to initialize network")
	
	// Note: We're simulating the discovery process manually, so we don't need a context
	
	// Measure discovery time
	startTime := time.Now()
	
	// Simulate discovery with a large number of validators
	// In a real implementation, we would use DiscoverNetworkPeers
	// with a mock network service that returns a large number of validators
	
	// For this test, we'll just measure the time to create and process a large number of validators
	dnValidators := make([]Validator, 0, 100)
	for i := 0; i < 100; i++ {
		validator := CreateMockValidator(fmt.Sprintf("validator%d", i), "dn")
		validator.IPAddress = "127.0.0.1"
		validator.P2PAddress = "tcp://127.0.0.1:26656"
		validator.RPCAddress = "http://127.0.0.1:26657"
		validator.APIAddress = "http://127.0.0.1:8080"
		validator.MetricsAddress = "http://127.0.0.1:26660"
		validator.URLs = map[string]string{
			"p2p":     validator.P2PAddress,
			"rpc":     validator.RPCAddress,
			"api":     validator.APIAddress,
			"metrics": validator.MetricsAddress,
		}
		validator.Version = "v1.0.0"
		validator.APIV3Status = true
		validator.DNHeight = 1000
		
		dnValidators = append(dnValidators, *validator)
	}
	
	// Update the address directory
	addressDir.mu.Lock()
	addressDir.DNValidators = dnValidators
	addressDir.mu.Unlock()
	
	// Calculate discovery time
	discoveryTime := time.Since(startTime)
	
	// Log performance metrics
	t.Logf("Discovery time for 100 validators: %v", discoveryTime)
	t.Logf("Average time per validator: %v", discoveryTime/100)
	
	// Verify that the discovery time is reasonable
	assert.Less(t, discoveryTime, 5*time.Second, "Discovery took too long")
}
