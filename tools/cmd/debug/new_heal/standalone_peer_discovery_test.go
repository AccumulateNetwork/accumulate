// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// StandalonePeerDiscovery provides a self-contained implementation for testing
type StandalonePeerDiscovery struct {
	logger *log.Logger
}

// NewStandalonePeerDiscovery creates a new StandalonePeerDiscovery instance
func NewStandalonePeerDiscovery(logger *log.Logger) *StandalonePeerDiscovery {
	return &StandalonePeerDiscovery{
		logger: logger,
	}
}

// ExtractHostFromMultiaddr extracts a host from a multiaddr string
func (p *StandalonePeerDiscovery) ExtractHostFromMultiaddr(addrStr string) (string, error) {
	if p.logger != nil {
		p.logger.Printf("Attempting to extract host from: %s", addrStr)
	}

	// Parse the multiaddress
	maddr, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		if p.logger != nil {
			p.logger.Printf("Failed to parse multiaddr: %v", err)
		}
		return "", fmt.Errorf("failed to parse multiaddr: %w", err)
	}

	// Extract the host component
	var host string
	multiaddr.ForEach(maddr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case multiaddr.P_DNS, multiaddr.P_DNS4, multiaddr.P_DNS6, multiaddr.P_IP4, multiaddr.P_IP6:
			host = c.Value()
			if p.logger != nil {
				p.logger.Printf("Found %s component: %s", c.Protocol().Name, host)
			}
			return false
		}
		return true
	})

	if host == "" {
		if p.logger != nil {
			p.logger.Printf("No host component found in multiaddr")
		}
		return "", fmt.Errorf("no host component found in multiaddr")
	}

	return host, nil
}

// ExtractHostFromURL extracts a host from a URL string
func (p *StandalonePeerDiscovery) ExtractHostFromURL(urlStr string) (string, error) {
	if p.logger != nil {
		p.logger.Printf("Attempting to extract host from URL: %s", urlStr)
	}

	// Handle URLs that don't have a scheme
	if !strings.Contains(urlStr, "://") {
		urlStr = "http://" + urlStr
	}

	// Parse the URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		if p.logger != nil {
			p.logger.Printf("Failed to parse URL: %v", err)
		}
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	host := parsedURL.Hostname()
	if host == "" {
		if p.logger != nil {
			p.logger.Printf("No host found in URL")
		}
		return "", fmt.Errorf("no host found in URL")
	}

	if p.logger != nil {
		p.logger.Printf("Extracted host from URL: %s", host)
	}
	return host, nil
}

// GetPeerRPCEndpoint constructs an RPC endpoint from a host
func (p *StandalonePeerDiscovery) GetPeerRPCEndpoint(host string) string {
	endpoint := fmt.Sprintf("http://%s:16592", host)
	if p.logger != nil {
		p.logger.Printf("Constructed RPC endpoint: %s", endpoint)
	}
	return endpoint
}

// LookupValidatorHost returns a known host for a validator ID
func (p *StandalonePeerDiscovery) LookupValidatorHost(validatorID string) (string, bool) {
	// Known validator hosts
	validatorHostMap := map[string]string{
		"defidevs.acme":    "65.108.73.121",
		"lunanova.acme":    "65.108.4.175",
		"tfa.acme":         "65.108.201.154",
		"factoshi.acme":    "135.181.114.121",
		"compumatrix.acme": "65.21.231.58",
		"ertai.acme":       "65.108.201.154",
	}

	host, ok := validatorHostMap[validatorID]
	if ok && p.logger != nil {
		p.logger.Printf("Found known host %s for validator %s", host, validatorID)
	}
	return host, ok
}

// ExtractHost attempts to extract a host using all available methods
func (p *StandalonePeerDiscovery) ExtractHost(addr string) (string, string) {
	// Try multiaddr parsing first
	host, err := p.ExtractHostFromMultiaddr(addr)
	if err == nil && host != "" {
		return host, "multiaddr"
	}

	// Try URL parsing as fallback
	host, err = p.ExtractHostFromURL(addr)
	if err == nil && host != "" {
		return host, "url"
	}

	// Check validator ID mapping
	host, ok := p.LookupValidatorHost(addr)
	if ok && host != "" {
		return host, "validator_map"
	}

	if p.logger != nil {
		p.logger.Printf("Failed to extract host from %s using any method", addr)
	}
	return "", "failed"
}

// TestStandalonePeerDiscovery tests the standalone peer discovery implementation
func TestStandalonePeerDiscovery(t *testing.T) {
	// Create a log directory if it doesn't exist
	logDir := "logs"
	err := os.MkdirAll(logDir, 0755)
	require.NoError(t, err)

	// Create a log file
	logFile, err := os.Create(fmt.Sprintf("%s/standalone_peer_discovery_test.log", logDir))
	require.NoError(t, err)
	defer logFile.Close()

	// Create a logger
	logger := log.New(logFile, "[StandalonePeerDiscoveryTest] ", log.LstdFlags)
	logger.Printf("======= STANDALONE PEER DISCOVERY TEST STARTED =======")

	// Create a new peer discovery instance
	peerDiscovery := NewStandalonePeerDiscovery(logger)

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
		
		// Test with standalone implementation
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
	
	logger.Printf("\n======= STANDALONE PEER DISCOVERY TEST COMPLETED =======")
}
