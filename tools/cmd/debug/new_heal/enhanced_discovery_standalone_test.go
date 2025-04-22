// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// +build enhanced_discovery

package new_heal

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestEnhancedDiscoveryStandalone tests the enhanced discovery implementation
func TestEnhancedDiscoveryStandalone(t *testing.T) {
	// Create a logger
	logger := log.New(os.Stdout, "[EnhancedDiscovery] ", log.LstdFlags)

	// Create an address directory
	addressDir := NewAddressDir()
	addressDir.Logger = logger

	// Create an enhanced discovery instance
	discovery := NewEnhancedDiscovery(addressDir, logger)

	// Initialize the network
	err := discovery.InitializeNetwork("testnet")
	assert.NoError(t, err, "Failed to initialize network")

	// Create a mock client
	discovery.Client = CreateMockNetworkService()

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Discover the network
	stats, err := discovery.DiscoverNetwork(ctx)
	assert.NoError(t, err, "Failed to discover network")

	// Verify that we discovered some validators
	assert.Greater(t, stats.TotalValidators, 0, "No validators discovered")
	assert.Greater(t, stats.DNValidators, 0, "No DN validators discovered")

	// Verify that we collected address information
	for addrType, typeStats := range stats.AddressTypeStats {
		assert.Greater(t, typeStats.Total, 0, "No %s addresses collected", addrType)
	}

	// Verify that we collected version information
	assert.Greater(t, len(stats.VersionCounts), 0, "No version information collected")

	// Validate the network
	validationStats, err := discovery.ValidateNetwork(ctx)
	assert.NoError(t, err, "Failed to validate network")

	// Verify that we validated some validators
	assert.Greater(t, len(validationStats.ValidatorStats), 0, "No validators validated")
}

// TestEnhancedDiscoveryMock tests the enhanced discovery with mock data
func TestEnhancedDiscoveryMock(t *testing.T) {
	// Create a logger
	logger := log.New(os.Stdout, "[MockTest] ", log.LstdFlags)

	// Create an address directory
	addressDir := NewAddressDir()
	addressDir.Logger = logger

	// Add some test validators
	dnValidator := CreateTestValidator("dn-validator", "dn")
	dnValidator.IPAddress = "127.0.0.1"
	dnValidator.P2PAddress = "tcp://127.0.0.1:26656"
	dnValidator.RPCAddress = "http://127.0.0.1:26657"
	dnValidator.APIAddress = "http://127.0.0.1:8080"
	dnValidator.MetricsAddress = "http://127.0.0.1:26660"
	addressDir.AddDNValidator(*dnValidator)

	bvnValidator := CreateTestValidator("bvn-validator", "bvn-apollo")
	bvnValidator.IPAddress = "127.0.0.2"
	bvnValidator.P2PAddress = "tcp://127.0.0.2:26656"
	bvnValidator.RPCAddress = "http://127.0.0.2:26657"
	bvnValidator.APIAddress = "http://127.0.0.2:8080"
	bvnValidator.MetricsAddress = "http://127.0.0.2:26660"
	addressDir.AddBVNValidator(0, *bvnValidator)

	// Create an enhanced discovery instance
	discovery := NewEnhancedDiscovery(addressDir, logger)

	// Initialize the network
	err := discovery.InitializeNetwork("testnet")
	assert.NoError(t, err, "Failed to initialize network")

	// Create a mock client
	discovery.Client = CreateMockNetworkService()

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Validate the network
	validationStats, err := discovery.ValidateNetwork(ctx)
	assert.NoError(t, err, "Failed to validate network")

	// Verify that we validated our test validators
	assert.Contains(t, validationStats.ValidatorStats, "dn-validator", "DN validator not validated")
	assert.Contains(t, validationStats.ValidatorStats, "bvn-validator", "BVN validator not validated")

	// Verify address validation
	dnStats := validationStats.ValidatorStats["dn-validator"]
	assert.Contains(t, dnStats.AddressStats, "p2p", "P2P address not validated")
	assert.Contains(t, dnStats.AddressStats, "rpc", "RPC address not validated")
	assert.Contains(t, dnStats.AddressStats, "api", "API address not validated")
	assert.Contains(t, dnStats.AddressStats, "metrics", "Metrics address not validated")
}
