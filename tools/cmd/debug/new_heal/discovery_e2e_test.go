// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/jsonrpc"
)

// TestMainnetEnhancedDiscovery tests the enhanced discovery against the real mainnet
func TestMainnetEnhancedDiscovery(t *testing.T) {
	// Skip in short mode or if not running on CI
	if testing.Short() || os.Getenv("CI") == "" {
		t.Skip("skipping mainnet test in short mode or when not on CI")
	}

	// Create a logger
	logger := log.New(os.Stdout, "[MainnetTest] ", log.LstdFlags)

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
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Measure discovery time
	startTime := time.Now()

	// Discover network peers
	stats, err := discovery.DiscoverNetworkPeers(ctx, client)
	require.NoError(t, err, "Failed to discover network peers")

	// Calculate discovery time
	discoveryTime := time.Since(startTime)

	// Log performance metrics
	t.Logf("Discovery time: %v", discoveryTime)
	t.Logf("Total validators: %d", stats.TotalValidators)
	t.Logf("DN validators: %d", stats.DNValidators)
	t.Logf("BVN validators: %d", stats.BVNValidators)
	t.Logf("API v3 available: %d", stats.APIV3Available)
	t.Logf("API v3 unavailable: %d", stats.APIV3Unavailable)
	t.Logf("Zombie nodes: %d", stats.ZombieNodes)

	// Verify that we discovered some validators
	assert.Greater(t, stats.TotalValidators, 0, "No validators discovered")
	assert.Greater(t, stats.DNValidators, 0, "No DN validators discovered")
	assert.Greater(t, stats.BVNValidators, 0, "No BVN validators discovered")

	// Verify that we have collected all address types
	addressTypesCollected := true
	for _, validator := range addressDir.DNValidators {
		if validator.IPAddress == "" || validator.P2PAddress == "" ||
			validator.RPCAddress == "" || validator.APIAddress == "" ||
			validator.MetricsAddress == "" {
			addressTypesCollected = false
			t.Logf("Validator %s is missing some address types", validator.Name)
		}
	}
	assert.True(t, addressTypesCollected, "Not all address types were collected")

	// Verify URL standardization
	urlsStandardized := true
	for _, validator := range addressDir.DNValidators {
		if validator.PartitionID == "dn" {
			partitionURL := discovery.standardizeURL("partition", "dn", "")
			if validator.PartitionID != "dn" || partitionURL != "acc://dn.acme" {
				urlsStandardized = false
				t.Logf("Validator %s has non-standardized URL: %s", validator.Name, partitionURL)
			}
		} else {
			partitionURL := discovery.standardizeURL("partition", validator.PartitionID, "")
			expectedURL := fmt.Sprintf("acc://bvn-%s.acme", validator.PartitionID)
			if partitionURL != expectedURL {
				urlsStandardized = false
				t.Logf("Validator %s has non-standardized URL: %s, expected: %s", 
					validator.Name, partitionURL, expectedURL)
			}
		}
	}
	assert.True(t, urlsStandardized, "URLs are not standardized")
}

// TestCompareWithNetworkStatus tests the comparison between network discovery and network status
func TestCompareWithNetworkStatus(t *testing.T) {
	// Skip in short mode or if not running on CI
	if testing.Short() || os.Getenv("CI") == "" {
		t.Skip("skipping comparison test in short mode or when not on CI")
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
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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

	// Compare address types
	// Count how many validators from network status have all address types
	addressTypesCount := 0
	for _, validator := range addressDir.DNValidators {
		if validator.IPAddress != "" && validator.P2PAddress != "" &&
			validator.RPCAddress != "" && validator.APIAddress != "" &&
			validator.MetricsAddress != "" {
			addressTypesCount++
		}
	}
	addressTypeRate := float64(addressTypesCount) / float64(len(addressDir.DNValidators))
	t.Logf("Address types collection rate: %.2f%%", addressTypeRate*100)
	assert.GreaterOrEqual(t, addressTypeRate, 0.7,
		"Too few validators have all address types")

	// Compare URL standardization
	// Verify that all partition URLs follow the standardized format
	for _, validator := range addressDir.DNValidators {
		if validator.PartitionID == "dn" {
			expectedURL := "acc://dn.acme"
			assert.Equal(t, expectedURL, discovery.standardizeURL("partition", "dn", ""),
				"DN partition URL is not standardized")
		} else {
			expectedURL := fmt.Sprintf("acc://bvn-%s.acme", validator.PartitionID)
			assert.Equal(t, expectedURL, discovery.standardizeURL("partition", validator.PartitionID, ""),
				"BVN partition URL is not standardized")
		}
	}

	// Compare anchor URLs
	// Verify that all anchor URLs follow the standardized format
	for _, partition := range []string{"Apollo", "Chandrayaan", "Yutu"} {
		expectedURL := fmt.Sprintf("acc://dn.acme/anchors/%s", partition)
		assert.Equal(t, expectedURL, discovery.standardizeURL("anchor", partition, ""),
			"Anchor URL is not standardized")
	}
}

// TestNetworkStatusCommand runs the network status command and compares results
func TestNetworkStatusCommand(t *testing.T) {
	// Skip in short mode or if not running on CI
	if testing.Short() || os.Getenv("CI") == "" {
		t.Skip("skipping network status command test in short mode or when not on CI")
	}

	// Run the network status command
	cmd := exec.Command("go", "run", "main.go", "network", "status", "mainnet", "--json")
	output, err := cmd.Output()
	require.NoError(t, err, "Failed to run network status command")

	// Parse the output
	// In a real implementation, we would parse the JSON output and compare it
	// with the results from our enhanced discovery
	assert.NotEmpty(t, output, "Network status command returned empty output")

	// For now, just log the output
	t.Logf("Network status command output: %d bytes", len(output))
}

// TestURLConsistency tests URL consistency across components
func TestURLConsistency(t *testing.T) {
	// Create a network discovery instance
	addressDir := NewAddressDir()
	discovery := NewNetworkDiscovery(addressDir, nil)

	// Initialize the network
	err := discovery.InitializeNetwork("mainnet")
	require.NoError(t, err, "Failed to initialize network")

	// Test partition URLs
	partitionURLs := map[string]string{
		"dn":     "acc://dn.acme",
		"Apollo": "acc://bvn-Apollo.acme",
	}

	for partition, expectedURL := range partitionURLs {
		url := discovery.standardizeURL("partition", partition, "")
		assert.Equal(t, expectedURL, url, "Partition URL for %s is not consistent", partition)
	}

	// Test anchor URLs
	anchorURLs := map[string]string{
		"Apollo":     "acc://dn.acme/anchors/Apollo",
		"Chandrayaan": "acc://dn.acme/anchors/Chandrayaan",
		"Yutu":       "acc://dn.acme/anchors/Yutu",
	}

	for partition, expectedURL := range anchorURLs {
		url := discovery.standardizeURL("anchor", partition, "")
		assert.Equal(t, expectedURL, url, "Anchor URL for %s is not consistent", partition)
	}

	// Test API URLs
	apiURL := "http://example.com:8080"
	url := discovery.standardizeURL("api", "", apiURL)
	assert.Equal(t, apiURL, url, "API URL is not consistent")
}
