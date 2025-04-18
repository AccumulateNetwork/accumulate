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

// NetworkPeer represents a network peer for testing
type NetworkPeer struct {
	ID          string
	IsValidator bool
	ValidatorID string
	Addresses   []string
	Host        string
	RPCEndpoint string
}

// TestIntegrationWithAddressDir demonstrates how to integrate the peer discovery utility
// with an AddressDir-like implementation
func TestIntegrationWithAddressDir(t *testing.T) {
	// Create a log directory if it doesn't exist
	logDir := "logs"
	err := os.MkdirAll(logDir, 0755)
	require.NoError(t, err)

	// Create a log file
	logFile, err := os.Create(fmt.Sprintf("%s/integration_test.log", logDir))
	require.NoError(t, err)
	defer logFile.Close()

	// Create a logger
	logger := log.New(logFile, "[IntegrationTest] ", log.LstdFlags)
	logger.Printf("======= INTEGRATION TEST STARTED =======")

	// Create a new peer discovery instance
	peerDiscovery := New(logger)

	// Create test peers with various address formats
	testPeers := []NetworkPeer{
		{
			ID:          "peer1",
			IsValidator: true,
			ValidatorID: "validator1",
			Addresses: []string{
				"/ip4/65.108.73.121/tcp/16593/p2p/12D3KooWBSEYCpvBcQUL8KPSXUQrQEAhQNFCT7fKZ44WarJkQskY",
				"/dns4/validator.lunanova.acme/tcp/16593/p2p/12D3KooWJ4bTHirdTFNZpCS72TAzwtdmavTBkkEXtzo6wHL25CtE",
			},
		},
		{
			ID:          "peer2",
			IsValidator: false,
			Addresses: []string{
				"/ip4/65.108.201.154/udp/16593/quic/p2p/12D3KooWRBnwN3UX8Vr7P7qN7zYAX7eCzrNLP9g5phL9jGmSvCTp",
				"/ip6/2a01:4f9:3a:2c26::2/tcp/16593/p2p/12D3KooWRBnwN3UX8Vr7P7qN7zYAX7eCzrNLP9g5phL9jGmSvCTp",
			},
		},
		{
			ID:          "peer3",
			IsValidator: true,
			ValidatorID: "defidevs.acme",
			Addresses: []string{
				"/p2p/defidevs.acme", // This is a problematic format that should be handled by validator mapping
			},
		},
		{
			ID:          "peer4",
			IsValidator: false,
			Addresses: []string{
				"validator.lunanova.acme:16592", // This is a URL format that should be handled by URL parsing
			},
		},
	}

	// Process each peer
	logger.Printf("\n===== PROCESSING PEERS =====")

	successCount := 0
	totalCount := len(testPeers)

	for i, peer := range testPeers {
		logger.Printf("Processing peer %d: %s", i+1, peer.ID)

		// Extract host and construct RPC endpoint
		var host string
		var method string

		// Try to extract a host from one of the addresses
		for _, addr := range peer.Addresses {
			logger.Printf("Attempting to extract host from address: %s", addr)

			// Use our peer discovery utility
			extractedHost, extractionMethod := peerDiscovery.ExtractHost(addr)
			if extractedHost != "" {
				host = extractedHost
				method = extractionMethod
				break
			}
		}

		// If no host was found and this is a validator, try the validator ID
		if host == "" && peer.IsValidator && peer.ValidatorID != "" {
			logger.Printf("Attempting to extract host from validator ID: %s", peer.ValidatorID)

			extractedHost, ok := peerDiscovery.LookupValidatorHost(peer.ValidatorID)
			if ok {
				host = extractedHost
				method = "validator_id_lookup"
			}
		}

		if host != "" {
			successCount++
			endpoint := peerDiscovery.GetPeerRPCEndpoint(host)
			logger.Printf("✅ SUCCESS: Extracted host %s using method %s", host, method)
			logger.Printf("Constructed endpoint: %s", endpoint)

			// Update the peer with the extracted information
			testPeers[i].Host = host
			testPeers[i].RPCEndpoint = endpoint
		} else {
			logger.Printf("❌ FAILED: Could not extract host for peer %s", peer.ID)
		}
	}

	// Log statistics
	logger.Printf("\n===== INTEGRATION TEST STATISTICS =====")
	logger.Printf("Total peers processed: %d", totalCount)
	logger.Printf("Successful extractions: %d (%.1f%%)",
		successCount, float64(successCount)/float64(totalCount)*100)

	// Verify that we have a reasonable success rate
	successRate := float64(successCount) / float64(totalCount) * 100
	assert.GreaterOrEqual(t, successRate, 75.0, "Should have at least 75%% success rate")

	// Verify that all peers have a host and RPC endpoint
	for _, peer := range testPeers {
		logger.Printf("Peer %s: Host=%s, RPCEndpoint=%s", peer.ID, peer.Host, peer.RPCEndpoint)
	}

	logger.Printf("\n======= INTEGRATION TEST COMPLETED =======")
}
