// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pinned

// RegisterForTest installs a pin for the given network and returns a
// cleanup function that restores the prior state. Test helpers in
// other packages call this to exercise pin-aware code paths without
// editing the production table.
//
// Not safe for concurrent use across tests.
func RegisterForTest(network string, pin Pin) (restore func()) {
	prev, had := networkPins[network]
	networkPins[network] = pin
	return func() {
		if had {
			networkPins[network] = prev
		} else {
			delete(networkPins, network)
		}
	}
}
