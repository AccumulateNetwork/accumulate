// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pinned

// RegisterForTest installs a pinned hash for the given network name
// and returns a cleanup function that restores the prior state. Test
// helpers in other packages call this to exercise pin-aware code paths
// without editing the production table.
//
// Not safe for concurrent use across tests.
func RegisterForTest(network string, hash [32]byte) (restore func()) {
	prev, had := networkGenesis[network]
	networkGenesis[network] = hash
	return func() {
		if had {
			networkGenesis[network] = prev
		} else {
			delete(networkGenesis, network)
		}
	}
}
