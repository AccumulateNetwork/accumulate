// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

import "strings"

// ExtractHost attempts to extract a host from an address string using multiple methods
// Returns the host and a string indicating which method was successful
func (a *AddressDir) extractHost(addr string) (string, string) {
	// Update statistics
	a.mu.Lock()
	a.DiscoveryStats.TotalAttempts++
	a.mu.Unlock()

	// Try multiaddr extraction first
	host, err := a.peerDiscovery.ExtractHostFromMultiaddr(addr)
	if err == nil && host != "" {
		// Update statistics
		a.mu.Lock()
		defer a.mu.Unlock()
		
		method := "multiaddr"
		a.DiscoveryStats.MethodStats[method]++
		a.DiscoveryStats.MultiaddrSuccess++
		return host, method
	}

	// Try URL extraction next
	host, err = a.peerDiscovery.ExtractHostFromURL(addr)
	if err == nil && host != "" {
		// Update statistics
		a.mu.Lock()
		defer a.mu.Unlock()
		
		method := "url"
		a.DiscoveryStats.MethodStats[method]++
		a.DiscoveryStats.URLSuccess++
		return host, method
	}

	// Try validator map lookup
	validatorID := ""
	// Extract validator ID from addr if possible
	parts := strings.Split(addr, "/")
	if len(parts) > 2 {
		validatorID = parts[len(parts)-1]
	}
	
	host = a.peerDiscovery.GetKnownAddressForValidator(validatorID)
	if host != "" {
		// Update statistics
		a.mu.Lock()
		defer a.mu.Unlock()
		
		method := "validator_map"
		a.DiscoveryStats.MethodStats[method]++
		a.DiscoveryStats.ValidatorMapSuccess++
		return host, method
	}

	// All methods failed
	a.mu.Lock()
	defer a.mu.Unlock()
	
	method := "failed"
	a.DiscoveryStats.MethodStats[method]++
	a.DiscoveryStats.Failures++
	return "", method
}
