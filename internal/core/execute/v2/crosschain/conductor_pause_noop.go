// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !testnet
// +build !testnet

// Production build - pause functions do nothing

package crosschain

// Pause does nothing in production builds
func (cc *CrossChainConductor) Pause() {
	// No-op
}

// Resume does nothing in production builds
func (cc *CrossChainConductor) Resume() {
	// No-op
}

// IsPaused always returns false in production builds
func (cc *CrossChainConductor) IsPaused() bool {
	return false
}
