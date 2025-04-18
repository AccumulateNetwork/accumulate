// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package new_heal

// ExtractHost attempts to extract a host from an address string using multiple methods
// Returns the host and a string indicating which method was successful
func (a *AddressDir) extractHost(addr string) (string, string) {
	// Update statistics
	a.mu.Lock()
	a.DiscoveryStats.TotalAttempts++
	a.mu.Unlock()

	// Try all extraction methods
	host, method := a.peerDiscovery.ExtractHost(addr)

	// Update statistics
	a.mu.Lock()
	defer a.mu.Unlock()

	a.DiscoveryStats.MethodStats[method]++

	switch method {
	case "multiaddr":
		a.DiscoveryStats.MultiaddrSuccess++
	case "url":
		a.DiscoveryStats.URLSuccess++
	case "validator_map":
		a.DiscoveryStats.ValidatorMapSuccess++
	case "failed":
		a.DiscoveryStats.Failures++
	}

	return host, method
}
