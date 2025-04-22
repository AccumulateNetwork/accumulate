// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package peerdiscovery

import (
	"fmt"
	"log"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPeerDiscovery tests the peer discovery implementation
func TestPeerDiscovery(t *testing.T) {
	// Create a log directory if it doesn't exist
	logDir := "logs"
	err := os.MkdirAll(logDir, 0755)
	require.NoError(t, err)

	// Create a log file
	logFile, err := os.Create(fmt.Sprintf("%s/peer_discovery_test.log", logDir))
	require.NoError(t, err)
	defer logFile.Close()

	// Create a logger
	logger := log.New(logFile, "[PeerDiscoveryTest] ", log.LstdFlags)
	logger.Printf("======= PEER DISCOVERY TEST STARTED =======")

	// Create a new peer discovery instance
	peerDiscovery := New(logger)

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

	// Test each address individually
	logger.Printf("\n===== TESTING INDIVIDUAL ADDRESS EXTRACTION =====")

	successCount := 0
	totalCount := len(testAddresses)
	methodStats := make(map[string]int)

	for _, addr := range testAddresses {
		logger.Printf("Testing address: %s", addr)

		// Test with peer discovery implementation
		host, method := peerDiscovery.ExtractHost(addr)
		logger.Printf("Method: %s, Host: %s", method, host)

		methodStats[method]++

		if host != "" {
			successCount++
			logger.Printf("✅ SUCCESS: Extracted host %s using method %s", host, method)

			// Test endpoint construction
			endpoint := peerDiscovery.GetPeerRPCEndpoint(host)
			logger.Printf("Constructed endpoint: %s", endpoint)
		} else {
			logger.Printf("❌ FAILED: Could not extract host from %s", addr)
		}
	}

	// Log statistics
	logger.Printf("\n===== DISCOVERY STATISTICS =====")
	logger.Printf("Total addresses tested: %d", totalCount)
	logger.Printf("Successful extractions: %d (%.1f%%)",
		successCount, float64(successCount)/float64(totalCount)*100)

	logger.Printf("\nMethod statistics:")
	for method, count := range methodStats {
		logger.Printf("  %s: %d (%.1f%%)", method, count,
			float64(count)/float64(totalCount)*100)
	}

	// Verify that we have a reasonable success rate
	successRate := float64(successCount) / float64(totalCount) * 100
	assert.GreaterOrEqual(t, successRate, 75.0, "Should have at least 75%% success rate")

	logger.Printf("\n======= PEER DISCOVERY TEST COMPLETED =======")
}
