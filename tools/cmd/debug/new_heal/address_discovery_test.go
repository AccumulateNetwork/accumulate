// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEnhancedPeerDiscovery tests the enhanced peer discovery implementation
// in the AddressDir struct, comparing it to our standalone implementation.
func TestEnhancedPeerDiscovery(t *testing.T) {
	// Create a log directory if it doesn't exist
	logDir := "logs"
	err := os.MkdirAll(logDir, 0755)
	require.NoError(t, err)

	// Create a log file
	logFile, err := os.Create(fmt.Sprintf("%s/address_discovery_test.log", logDir))
	require.NoError(t, err)
	defer logFile.Close()

	// Create a logger
	logger := log.New(logFile, "[AddressDiscoveryTest] ", log.LstdFlags)
	logger.Printf("======= ADDRESS DISCOVERY TEST STARTED =======")

	// Create a new AddressDir instance
	addressDir := NewAddressDir()

	// Create a new peer discovery instance for comparison
	peerDiscovery := NewPeerDiscovery(logger)

	// Test addresses to check
	testAddresses := []string{
		"/ip4/65.108.73.121/tcp/16593/p2p/12D3KooWBSEYCpvBcQUL8KPSXUQrQEAhQNFCT7fKZ44WarJkQskY",
		"/ip4/65.108.4.175/tcp/16593/p2p/12D3KooWJ4bTHirdTFNZpCS72TAzwtdmavTBkkEXtzo6wHL25CtE",
		"/dns4/validator.lunanova.acme/tcp/16593/p2p/12D3KooWJ4bTHirdTFNZpCS72TAzwtdmavTBkkEXtzo6wHL25CtE",
		"/ip4/135.181.114.121/tcp/16593/p2p/12D3KooWBSEYCpvBcQUL8KPSXUQrQEAhQNFCT7fKZ44WarJkQskY",
		"/ip4/65.21.231.58/tcp/26656/p2p/12D3KooWHHzSeKaY8xuZVzkLbKFfvNgPPeKhFBGrMbNzbm5akpqu",
		"/ip4/65.108.201.154/udp/16593/quic/p2p/12D3KooWRBnwN3UX8Vr7P7qN7zYAX7eCzrNLP9g5phL9jGmSvCTp",
		"/ip6/2a01:4f9:3a:2c26::2/tcp/16593/p2p/12D3KooWRBnwN3UX8Vr7P7qN7zYAX7eCzrNLP9g5phL9jGmSvCTp",
		"/dns/validator.tfa.acme/tcp/16593/p2p/12D3KooWRBnwN3UX8Vr7P7qN7zYAX7eCzrNLP9g5phL9jGmSvCTp",
		// Add problematic addresses that should be handled by fallback mechanisms
		"/p2p/defidevs.acme",
		"validator.lunanova.acme:16592",
		"http://65.108.73.121:16592",
		"65.108.4.175",
	}

	// Create a NetworkPeer for testing
	testPeer := NetworkPeer{
		ID:        "test-peer",
		Addresses: testAddresses,
	}

	// Test each address individually with both implementations
	logger.Printf("\n===== TESTING INDIVIDUAL ADDRESS EXTRACTION =====")

	successCount := 0
	totalCount := len(testAddresses)

	for _, addr := range testAddresses {
		logger.Printf("Testing address: %s", addr)

		// Test with standalone implementation
		host, method := peerDiscovery.ExtractHost(addr)
		logger.Printf("Standalone implementation: host=%s, method=%s", host, method)

		// Test with AddressDir implementation
		addrHost, addrMethod := addressDir.extractHost(addr)
		logger.Printf("AddressDir implementation: host=%s, method=%s", addrHost, addrMethod)

		// Compare results
		if host == addrHost {
			logger.Printf("✅ MATCH: Both implementations extracted the same host: %s", host)
			successCount++
		} else if host == "" && addrHost != "" {
			logger.Printf("✅ IMPROVED: AddressDir extracted a host (%s) while standalone failed", addrHost)
			successCount++
		} else if host != "" && addrHost == "" {
			logger.Printf("❌ REGRESSION: Standalone extracted a host (%s) while AddressDir failed", host)
		} else {
			logger.Printf("❓ DIFFERENT: Standalone extracted '%s', AddressDir extracted '%s'", host, addrHost)
		}
	}

	// Test GetPeerRPCEndpoint with the test peer
	logger.Printf("\n===== TESTING GetPeerRPCEndpoint =====")

	endpoint := addressDir.GetPeerRPCEndpoint(testPeer)
	logger.Printf("GetPeerRPCEndpoint result: %s", endpoint)

	// Verify that we got a valid endpoint
	assert.NotEmpty(t, endpoint, "Should have extracted a valid endpoint")

	// Log statistics
	logger.Printf("\n===== DISCOVERY STATISTICS =====")
	logger.Printf("Total addresses tested: %d", totalCount)
	logger.Printf("Successful extractions: %d (%.1f%%)",
		successCount, float64(successCount)/float64(totalCount)*100)

	addressDir.mu.RLock()
	logger.Printf("AddressDir stats - Total attempts: %d", addressDir.discoveryStats.TotalAttempts)
	logger.Printf("AddressDir stats - Multiaddr successes: %d", addressDir.discoveryStats.MultiaddrSuccess)
	logger.Printf("AddressDir stats - URL successes: %d", addressDir.discoveryStats.URLSuccess)
	logger.Printf("AddressDir stats - Validator map successes: %d", addressDir.discoveryStats.ValidatorMapSuccess)
	logger.Printf("AddressDir stats - Failures: %d", addressDir.discoveryStats.Failures)

	logger.Printf("\nMethod statistics:")
	for method, count := range addressDir.discoveryStats.MethodStats {
		logger.Printf("  %s: %d", method, count)
	}
	addressDir.mu.RUnlock()

	// Verify that we have a reasonable success rate
	successRate := float64(successCount) / float64(totalCount) * 100
	assert.GreaterOrEqual(t, successRate, 75.0, "Should have at least 75%% success rate")

	logger.Printf("\n======= ADDRESS DISCOVERY TEST COMPLETED =======")
}
