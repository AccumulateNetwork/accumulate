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
	"strings"

	"github.com/multiformats/go-multiaddr"
)

// PeerDiscovery provides enhanced methods for extracting hosts from various address formats
type PeerDiscovery struct {
	logger *log.Logger
}

// NewPeerDiscovery creates a new PeerDiscovery instance
func NewPeerDiscovery(logger *log.Logger) *PeerDiscovery {
	return &PeerDiscovery{
		logger: logger,
	}
}

// ExtractHostFromMultiaddr extracts a host from a multiaddr string
// This implements the same logic as the original code but with better error handling
func (p *PeerDiscovery) ExtractHostFromMultiaddr(addrStr string) (string, error) {
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
// This provides a fallback when multiaddr parsing fails
func (p *PeerDiscovery) ExtractHostFromURL(urlStr string) (string, error) {
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
func (p *PeerDiscovery) GetPeerRPCEndpoint(host string) string {
	endpoint := fmt.Sprintf("http://%s:16592", host)
	if p.logger != nil {
		p.logger.Printf("Constructed RPC endpoint: %s", endpoint)
	}
	return endpoint
}

// LookupValidatorHost returns a known host for a validator ID
// This provides a fallback when other methods fail
func (p *PeerDiscovery) LookupValidatorHost(validatorID string) (string, bool) {
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
// Returns the host and a string indicating which method was successful
func (p *PeerDiscovery) ExtractHost(addr string) (string, string) {
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
