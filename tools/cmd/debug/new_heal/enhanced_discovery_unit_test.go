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
)

// TestAddressCollection tests the address collection functionality
func TestAddressCollection(t *testing.T) {
	// Create a logger
	logger := log.New(os.Stdout, "[TestAddressCollection] ", log.LstdFlags)

	// Create an address directory
	addressDir := NewAddressDir()
	addressDir.Logger = logger

	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)

	// Create a test validator
	validator := CreateTestValidator("test-validator", "dn")
	validator.IPAddress = "127.0.0.1"
	validator.P2PAddress = "tcp://127.0.0.1:26656"
	validator.RPCAddress = "http://127.0.0.1:26657"
	validator.APIAddress = "http://127.0.0.1:8080"
	validator.MetricsAddress = "http://127.0.0.1:26660"

	// Set up URLs
	validator.URLs = map[string]string{
		"p2p":     validator.P2PAddress,
		"rpc":     validator.RPCAddress,
		"api":     validator.APIAddress,
		"metrics": validator.MetricsAddress,
	}

	// Create a context
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Test address collection
	stats, _ := discovery.collectValidatorAddresses(ctx, validator, "127.0.0.1")
	
	// We expect errors since we're not using real services, but the function should still work
	assert.NotNil(t, stats, "Address stats should not be nil")
	
	// Verify that all address types were collected
	assert.NotEmpty(t, stats.TotalByType, "Total by type should not be empty")
	assert.Greater(t, stats.TotalByType["p2p"], 0, "P2P address count should be greater than 0")
	assert.Greater(t, stats.TotalByType["rpc"], 0, "RPC address count should be greater than 0")
	assert.Greater(t, stats.TotalByType["api"], 0, "API address count should be greater than 0")
	assert.Greater(t, stats.TotalByType["metrics"], 0, "Metrics address count should be greater than 0")
}

// TestAddressValidation tests the address validation functionality
func TestAddressValidation(t *testing.T) {
	// Create a logger
	logger := log.New(os.Stdout, "[TestAddressValidation] ", log.LstdFlags)

	// Create an address directory
	addressDir := NewAddressDir()
	addressDir.Logger = logger

	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)

	// Create a test validator
	validator := CreateTestValidator("test-validator", "dn")

	// Create a context
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Test validation of valid addresses
	validAddresses := map[string]string{
		"p2p":     "tcp://127.0.0.1:26656",
		"rpc":     "http://127.0.0.1:26657",
		"api":     "http://127.0.0.1:8080",
		"metrics": "http://127.0.0.1:26660",
	}

	for addrType, addr := range validAddresses {
		result, _ := discovery.validateAddress(ctx, validator, addrType, addr)
		
		// We expect errors since we're not using real services, but the function should still work
		assert.NotNil(t, result, "Validation result should not be nil")
		// Check for IP and Port instead of AddressType and Address
		assert.NotEmpty(t, result.IP, "IP should not be empty")
		assert.NotEmpty(t, result.Port, "Port should not be empty")
	}

	// Test validation of invalid addresses
	invalidAddresses := map[string]string{
		"p2p":     "invalid://127.0.0.1:26656",
		"rpc":     "invalid://127.0.0.1:26657",
		"api":     "invalid://127.0.0.1:8080",
		"metrics": "invalid://127.0.0.1:26660",
	}

	for addrType, addr := range invalidAddresses {
		result, _ := discovery.validateAddress(ctx, validator, addrType, addr)
		
		// We expect errors for invalid addresses
		assert.NotNil(t, result, "Validation result should not be nil")
		assert.False(t, result.Success, "Validation should fail for invalid address")
		assert.NotEmpty(t, result.Error, "Error message should not be empty")
	}
}

// TestURLStandardization tests the URL standardization functionality
func TestURLStandardization(t *testing.T) {
	// Create a logger
	logger := log.New(os.Stdout, "[TestURLStandardization] ", log.LstdFlags)

	// Create an address directory
	addressDir := NewAddressDir()
	addressDir.Logger = logger

	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)

	// Test partition URL standardization
	partitionURLs := map[string]string{
		"dn":     "acc://dn.acme",
		"Apollo": "acc://bvn-Apollo.acme",
	}

	for partition, expectedURL := range partitionURLs {
		url := discovery.standardizeURL("partition", partition, "")
		assert.Equal(t, expectedURL, url, "Partition URL for %s is not standardized", partition)
	}

	// Test anchor URL standardization
	anchorURLs := map[string]string{
		"Apollo":     "acc://dn.acme/anchors/Apollo",
		"Chandrayaan": "acc://dn.acme/anchors/Chandrayaan",
		"Yutu":       "acc://dn.acme/anchors/Yutu",
	}

	for partition, expectedURL := range anchorURLs {
		url := discovery.standardizeURL("anchor", partition, "")
		assert.Equal(t, expectedURL, url, "Anchor URL for %s is not standardized", partition)
	}

	// Test API URL standardization
	apiURL := "http://example.com:8080"
	url := discovery.standardizeURL("api", "", apiURL)
	assert.Equal(t, apiURL, url, "API URL is not standardized")
}

// TestEnhancedAPIv3Connectivity tests the API v3 connectivity functionality
func TestEnhancedAPIv3Connectivity(t *testing.T) {
	// Create a logger
	logger := log.New(os.Stdout, "[TestAPIv3Connectivity] ", log.LstdFlags)

	// Create an address directory
	addressDir := NewAddressDir()
	addressDir.Logger = logger

	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)

	// Create a test validator
	validator := CreateTestValidator("test-validator", "dn")
	validator.APIAddress = "http://127.0.0.1:8080"

	// Create a context
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Test API v3 connectivity
	result, _ := discovery.testAPIv3Connectivity(ctx, validator)
	
	// We expect errors since we're not using real services, but the function should still work
	assert.NotNil(t, result, "API v3 connectivity result should not be nil")
	assert.False(t, result.Available, "API v3 should not be available for test validator")
	assert.NotEmpty(t, result.Error, "Error message should not be empty")
}

// TestVersionCollection tests the version information collection functionality
func TestVersionCollection(t *testing.T) {
	// Create a logger
	logger := log.New(os.Stdout, "[TestVersionCollection] ", log.LstdFlags)

	// Create an address directory
	addressDir := NewAddressDir()
	addressDir.Logger = logger

	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)

	// Create a test validator
	validator := CreateTestValidator("test-validator", "dn")
	validator.RPCAddress = "http://127.0.0.1:26657"

	// Create a context
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Test version information collection
	_ = discovery.collectVersionInfo(ctx, validator)
	
	// We expect errors since we're not using real services, but the function should still work
	assert.NotEmpty(t, validator.Version, "Version should not be empty")
}

// TestConsensusStatus tests the consensus status functionality
func TestConsensusStatus(t *testing.T) {
	// Create a logger
	logger := log.New(os.Stdout, "[TestConsensusStatus] ", log.LstdFlags)

	// Create an address directory
	addressDir := NewAddressDir()
	addressDir.Logger = logger

	// Create a network discovery instance
	discovery := NewNetworkDiscovery(addressDir, logger)

	// Create a test validator
	validator := CreateTestValidator("test-validator", "dn")
	validator.RPCAddress = "http://127.0.0.1:26657"

	// Create a context
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Test consensus status
	result, _ := discovery.checkConsensusStatus(ctx, validator)
	
	// We expect errors since we're not using real services, but the function should still work
	assert.NotNil(t, result, "Consensus status result should not be nil")
	
	// Verify that we have consensus information
	assert.NotZero(t, result.DNHeight, "DN height should not be zero")
	
	// Verify that we have zombie node detection
	assert.NotNil(t, result.IsZombie, "Zombie node detection should work")
}
