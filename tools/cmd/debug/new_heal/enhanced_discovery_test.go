// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

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

func TestEnhancedNetworkDiscovery(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	// Create a logger
	logger := log.New(os.Stdout, "[EnhancedDiscovery] ", log.LstdFlags)

	// Create an address directory
	addressDir := NewAddressDir()
	addressDir.Logger = logger

	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)

	// Initialize the network
	err := discovery.InitializeNetwork("mainnet")
	require.NoError(t, err, "Failed to initialize network")

	// Create a client using our utility function
	client := jsonrpc.NewClient(ResolveWellKnownEndpoint("mainnet", "v3"))

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Discover network peers
	stats, err := discovery.DiscoverNetworkPeers(ctx, client)
	require.NoError(t, err, "Failed to discover network peers")

	// Verify that we discovered some peers
	assert.Greater(t, stats.TotalPeers, 0, "No peers discovered")
	assert.Greater(t, stats.TotalValidators, 0, "No validators discovered")

	// Verify that we have DN validators
	assert.Greater(t, stats.DNValidators, 0, "No DN validators discovered")
	assert.Greater(t, len(addressDir.DNValidators), 0, "No DN validators in address directory")

	// Verify that we have BVN validators
	assert.Greater(t, stats.BVNValidators, 0, "No BVN validators discovered")
	assert.Greater(t, len(addressDir.BVNValidators), 0, "No BVN validators in address directory")

	// Verify that we have collected all address types
	for _, validator := range addressDir.DNValidators {
		assert.NotEmpty(t, validator.IPAddress, "Validator %s has no IP address", validator.Name)
		assert.NotEmpty(t, validator.P2PAddress, "Validator %s has no P2P address", validator.Name)
		assert.NotEmpty(t, validator.RPCAddress, "Validator %s has no RPC address", validator.Name)
		assert.NotEmpty(t, validator.APIAddress, "Validator %s has no API address", validator.Name)
		assert.NotEmpty(t, validator.MetricsAddress, "Validator %s has no metrics address", validator.Name)

		// Verify that we have URLs
		assert.NotNil(t, validator.URLs, "Validator %s has no URLs", validator.Name)
		assert.NotEmpty(t, validator.URLs["p2p"], "Validator %s has no P2P URL", validator.Name)
		assert.NotEmpty(t, validator.URLs["rpc"], "Validator %s has no RPC URL", validator.Name)
		assert.NotEmpty(t, validator.URLs["api"], "Validator %s has no API URL", validator.Name)
		assert.NotEmpty(t, validator.URLs["metrics"], "Validator %s has no metrics URL", validator.Name)
	}

	// Verify that we have collected version information
	versionCount := 0
	for _, validator := range addressDir.DNValidators {
		if validator.Version != "" {
			versionCount++
		}
	}
	assert.Greater(t, versionCount, 0, "No version information collected")

	// Verify that we have tested API v3 connectivity
	apiV3Count := 0
	for _, validator := range addressDir.DNValidators {
		if validator.APIV3Status {
			apiV3Count++
		}
	}
	assert.Greater(t, apiV3Count, 0, "No API v3 connectivity tested")

	// Verify that we have collected consensus information
	heightCount := 0
	for _, validator := range addressDir.DNValidators {
		if validator.DNHeight > 0 {
			heightCount++
		}
	}
	assert.Greater(t, heightCount, 0, "No consensus information collected")

	// Verify that we have detected zombie nodes
	// Note: This might not always be true, so we don't assert it
	t.Logf("Detected %d zombie nodes", stats.ZombieNodes)

	// Log detailed statistics
	t.Logf("Total peers: %d", stats.TotalPeers)
	t.Logf("Total validators: %d", stats.TotalValidators)
	t.Logf("DN validators: %d", stats.DNValidators)
	t.Logf("BVN validators: %d", stats.BVNValidators)
	t.Logf("API v3 available: %d", stats.APIV3Available)
	t.Logf("API v3 unavailable: %d", stats.APIV3Unavailable)
}

func TestCompareNetworkStatusAndDiscovery(t *testing.T) {
	// Skip in short mode
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	// Create a logger
	logger := log.New(os.Stdout, "[ComparisonTest] ", log.LstdFlags)

	// Create an address directory
	addressDir := NewAddressDir()
	addressDir.Logger = logger

	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)

	// Initialize the network
	err := discovery.InitializeNetwork("mainnet")
	require.NoError(t, err, "Failed to initialize network")

	// Create a client using our utility function
	client := jsonrpc.NewClient(ResolveWellKnownEndpoint("mainnet", "v3"))

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Discover network peers
	stats, err := discovery.DiscoverNetworkPeers(ctx, client)
	require.NoError(t, err, "Failed to discover network peers")

	// Get network status directly
	networkStatus, err := client.NetworkStatus(ctx, api.NetworkStatusOptions{})
	require.NoError(t, err, "Failed to get network status")

	// Compare validator counts
	discoveredValidators := stats.TotalValidators
	statusValidators := len(networkStatus.Network.Validators)
	
	// The counts might not match exactly due to different filtering criteria,
	// but they should be reasonably close
	t.Logf("Discovered validators: %d, Status validators: %d", 
		discoveredValidators, statusValidators)
	
	// Verify that we discovered at least 80% of the validators from network status
	minExpectedValidators := int(float64(statusValidators) * 0.8)
	assert.GreaterOrEqual(t, discoveredValidators, minExpectedValidators, 
		"Discovered too few validators compared to network status")

	// Compare validator identities
	// Create a map of validator IDs from network status
	statusValidatorMap := make(map[string]bool)
	for _, validator := range networkStatus.Network.Validators {
		if validator.Operator != nil {
			statusValidatorMap[validator.Operator.Authority] = true
		}
	}

	// Count how many validators from our discovery are also in network status
	matchCount := 0
	for _, validator := range addressDir.DNValidators {
		if statusValidatorMap[validator.Name] {
			matchCount++
		}
	}

	// Verify that we have a reasonable match rate
	matchRate := float64(matchCount) / float64(len(addressDir.DNValidators))
	t.Logf("Validator match rate: %.2f%%", matchRate*100)
	assert.GreaterOrEqual(t, matchRate, 0.7, 
		"Too few validators match between discovery and network status")
}
