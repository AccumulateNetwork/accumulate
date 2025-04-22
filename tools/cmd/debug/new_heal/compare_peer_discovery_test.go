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
	"sort"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

// TestComparePeerDiscovery compares our simplified peer discovery utility with the original code
// to verify we've successfully extracted the peer discovery logic.
func TestComparePeerDiscovery(t *testing.T) {
	// Create a log directory if it doesn't exist
	logDir := "logs"
	err := os.MkdirAll(logDir, 0755)
	require.NoError(t, err)

	// Create a log file with timestamp
	timestamp := time.Now().Format("20060102-150405")
	logFilePath := fmt.Sprintf("%s/compare_peer_discovery_%s.log", logDir, timestamp)
	logFile, err := os.Create(logFilePath)
	require.NoError(t, err)
	defer logFile.Close()

	// Create a logger
	logger := log.New(logFile, "[CompareDiscovery] ", log.LstdFlags)
	logger.Printf("======= PEER DISCOVERY COMPARISON STARTED =======")
	logger.Printf("Test started at: %s", time.Now().Format(time.RFC3339))

	// 1. Test addresses from the original code
	logger.Printf("\n===== TESTING ORIGINAL CODE ADDRESSES =====")

	// These are the multiaddrs that would be processed by the original code
	originalAddrs := []string{
		"/ip4/65.108.73.121/tcp/16593/p2p/12D3KooWBSEYCpvBcQUL8KPSXUQrQEAhQNFCT7fKZ44WarJkQskY",
		"/ip4/65.108.4.175/tcp/16593/p2p/12D3KooWJ4bTHirdTFNZpCS72TAzwtdmavTBkkEXtzo6wHL25CtE",
		"/dns4/validator.lunanova.acme/tcp/16593/p2p/12D3KooWJ4bTHirdTFNZpCS72TAzwtdmavTBkkEXtzo6wHL25CtE",
		"/ip4/135.181.114.121/tcp/16593/p2p/12D3KooWBSEYCpvBcQUL8KPSXUQrQEAhQNFCT7fKZ44WarJkQskY",
		"/ip4/65.21.231.58/tcp/26656/p2p/12D3KooWHHzSeKaY8xuZVzkLbKFfvNgPPeKhFBGrMbNzbm5akpqu",
		"/ip4/65.108.201.154/udp/16593/quic/p2p/12D3KooWRBnwN3UX8Vr7P7qN7zYAX7eCzrNLP9g5phL9jGmSvCTp",
		"/ip6/2a01:4f9:3a:2c26::2/tcp/16593/p2p/12D3KooWRBnwN3UX8Vr7P7qN7zYAX7eCzrNLP9g5phL9jGmSvCTp",
		"/dns/validator.tfa.acme/tcp/16593/p2p/12D3KooWRBnwN3UX8Vr7P7qN7zYAX7eCzrNLP9g5phL9jGmSvCTp",
	}

	// 2. Extract hosts using original code logic
	logger.Printf("Extracting hosts using original code logic...")
	originalResults := make(map[string]string)

	for i, addr := range originalAddrs {
		logger.Printf("Original address %d: %s", i+1, addr)

		// Parse the multiaddr
		maddr, err := multiaddr.NewMultiaddr(addr)
		if err != nil {
			logger.Printf("  Error parsing multiaddr: %v", err)
			continue
		}

		// Extract host using the original code logic
		var host string
		multiaddr.ForEach(maddr, func(c multiaddr.Component) bool {
			switch c.Protocol().Code {
			case multiaddr.P_DNS,
				multiaddr.P_DNS4,
				multiaddr.P_DNS6,
				multiaddr.P_IP4,
				multiaddr.P_IP6:
				host = c.Value()
				return false
			}
			return true
		})

		if host != "" {
			endpoint := fmt.Sprintf("http://%s:16592", host)
			originalResults[addr] = endpoint
			logger.Printf("  Original code extracted: %s → %s", host, endpoint)
		} else {
			logger.Printf("  Original code failed to extract host")
		}
	}

	// 3. Extract hosts using our simplified utility
	logger.Printf("\n===== TESTING SIMPLIFIED UTILITY =====")
	logger.Printf("Extracting hosts using simplified utility...")

	discovery := NewSimplePeerDiscovery(logger)
	simplifiedResults := make(map[string]string)

	for i, addr := range originalAddrs {
		logger.Printf("Testing address %d: %s", i+1, addr)

		host, err := discovery.ExtractHostFromMultiaddr(addr)
		if err != nil {
			logger.Printf("  Simplified utility error: %v", err)
			continue
		}

		if host != "" {
			endpoint := discovery.GetPeerRPCEndpoint(host)
			simplifiedResults[addr] = endpoint
			logger.Printf("  Simplified utility extracted: %s → %s", host, endpoint)
		} else {
			logger.Printf("  Simplified utility failed to extract host")
		}
	}

	// 4. Compare results
	logger.Printf("\n===== COMPARING RESULTS =====")

	// Sort addresses for consistent output
	var addresses []string
	for addr := range originalResults {
		addresses = append(addresses, addr)
	}
	for addr := range simplifiedResults {
		if _, exists := originalResults[addr]; !exists {
			addresses = append(addresses, addr)
		}
	}
	sort.Strings(addresses)

	matchCount := 0
	mismatchCount := 0
	onlyInOriginal := 0
	onlyInSimplified := 0

	for _, addr := range addresses {
		originalEndpoint, inOriginal := originalResults[addr]
		simplifiedEndpoint, inSimplified := simplifiedResults[addr]

		switch {
		case inOriginal && inSimplified:
			if originalEndpoint == simplifiedEndpoint {
				matchCount++
				logger.Printf("✅ MATCH: %s", addr)
				logger.Printf("  Both extracted: %s", originalEndpoint)
			} else {
				mismatchCount++
				logger.Printf("❌ MISMATCH: %s", addr)
				logger.Printf("  Original: %s", originalEndpoint)
				logger.Printf("  Simplified: %s", simplifiedEndpoint)
			}
		case inOriginal && !inSimplified:
			onlyInOriginal++
			logger.Printf("⚠️ ONLY IN ORIGINAL: %s", addr)
			logger.Printf("  Endpoint: %s", originalEndpoint)
		case !inOriginal && inSimplified:
			onlyInSimplified++
			logger.Printf("⚠️ ONLY IN SIMPLIFIED: %s", addr)
			logger.Printf("  Endpoint: %s", simplifiedEndpoint)
		}
	}

	// 5. Print summary
	totalAddresses := len(addresses)
	logger.Printf("\n===== SUMMARY =====")
	logger.Printf("Total addresses tested: %d", totalAddresses)
	logger.Printf("Matches: %d (%.1f%%)", matchCount, float64(matchCount)/float64(totalAddresses)*100)
	logger.Printf("Mismatches: %d (%.1f%%)", mismatchCount, float64(mismatchCount)/float64(totalAddresses)*100)
	logger.Printf("Only in original: %d (%.1f%%)", onlyInOriginal, float64(onlyInOriginal)/float64(totalAddresses)*100)
	logger.Printf("Only in simplified: %d (%.1f%%)", onlyInSimplified, float64(onlyInSimplified)/float64(totalAddresses)*100)

	// 6. Verify success
	successRate := float64(matchCount) / float64(matchCount+mismatchCount) * 100
	logger.Printf("Success rate: %.1f%%", successRate)

	// Test passes if we have at least 80% match rate
	require.GreaterOrEqual(t, successRate, 80.0, "Simplified utility should match original code at least 80% of the time")

	fmt.Printf("\nComparison test completed. See log file for details: %s\n", logFilePath)
}
