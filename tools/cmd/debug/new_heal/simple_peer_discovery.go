// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import (
	"fmt"
	"log"
	"strings"

	"github.com/multiformats/go-multiaddr"
)

// SimplePeerDiscovery provides utilities for extracting host information from peer addresses
type SimplePeerDiscovery struct {
	logger *log.Logger
}

// NewSimplePeerDiscovery creates a new SimplePeerDiscovery instance
func NewSimplePeerDiscovery(logger *log.Logger) *SimplePeerDiscovery {
	return &SimplePeerDiscovery{
		logger: logger,
	}
}

// ExtractHostFromMultiaddr extracts the host from a multiaddr
func (d *SimplePeerDiscovery) ExtractHostFromMultiaddr(addrStr string) (string, error) {
	d.log("Attempting to extract host from: %s", addrStr)
	
	// Try to parse as multiaddr
	maddr, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		d.log("Error parsing multiaddr: %v", err)
		return "", fmt.Errorf("error parsing multiaddr: %w", err)
	}
	
	// Extract host from multiaddr
	var host string
	multiaddr.ForEach(maddr, func(c multiaddr.Component) bool {
		switch c.Protocol().Code {
		case multiaddr.P_IP4, multiaddr.P_IP6:
			// IP address
			host = c.Value()
			d.log("Found IP component: %s", host)
			return false
		case multiaddr.P_DNS, multiaddr.P_DNS4, multiaddr.P_DNS6:
			// Domain name
			host = c.Value()
			d.log("Found DNS component: %s", host)
			return false
		}
		return true
	})
	
	if host == "" {
		d.log("No host component found in multiaddr")
		return "", fmt.Errorf("no host component found in multiaddr")
	}
	
	return host, nil
}

// ExtractHostFromURL attempts to extract a host from a URL-like string
func (d *SimplePeerDiscovery) ExtractHostFromURL(urlStr string) (string, error) {
	d.log("Attempting to extract host from URL: %s", urlStr)
	
	// Check if it's a URL with scheme
	if strings.Contains(urlStr, "://") {
		parts := strings.Split(urlStr, "://")
		if len(parts) > 1 {
			hostPort := strings.Split(parts[1], ":")
			if len(hostPort) > 0 {
				host := hostPort[0]
				// If there's a path, remove it
				if strings.Contains(host, "/") {
					host = strings.Split(host, "/")[0]
				}
				d.log("Extracted host from URL: %s", host)
				return host, nil
			}
		}
	}
	
	return "", fmt.Errorf("could not extract host from URL")
}

// GetKnownAddressForValidator returns a known IP address for a validator ID
func (d *SimplePeerDiscovery) GetKnownAddressForValidator(validatorID string) string {
	// Hardcoded mappings for known validators
	knownAddresses := map[string]string{
		"defidevs.acme":         "65.108.73.121",
		"LunaNova.acme":         "65.108.73.121",
		"Sphereon.acme":         "65.108.4.175",
		"ConsensusNetworks.acme": "65.21.231.58",
		"tfa.acme":              "65.108.201.154",
		"HighStakes.acme":       "65.109.33.17",
		"TurtleBoat.acme":       "135.181.114.121",
	}
	
	if addr, ok := knownAddresses[validatorID]; ok {
		d.log("Found known address for validator %s: %s", validatorID, addr)
		return addr
	}
	
	d.log("No known address for validator %s", validatorID)
	return ""
}

// GetPeerRPCEndpoint constructs an RPC endpoint from a host
func (d *SimplePeerDiscovery) GetPeerRPCEndpoint(host string) string {
	if host == "" {
		return ""
	}
	
	// Use the standard Tendermint RPC port
	endpoint := fmt.Sprintf("http://%s:16592", host)
	d.log("Constructed RPC endpoint: %s", endpoint)
	return endpoint
}

// log logs a message if the logger is available
func (d *SimplePeerDiscovery) log(format string, args ...interface{}) {
	if d.logger != nil {
		d.logger.Printf(format, args...)
		// Also print to stdout for immediate visibility
		fmt.Printf(format+"\n", args...)
	}
}
