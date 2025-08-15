// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build testnet
// +build testnet

// This file is only compiled when the testnet build tag is set.
// This prevents the pause functionality from being available in production builds.
//
// To enable: go build -tags testnet

package crosschain

import (
	"sync/atomic"
)

// paused is a simple atomic flag to pause all CCC message processing
// 1 = paused, 0 = running
var paused uint32

// Pause pauses all CCC message processing (inbound and outbound)
func (cc *CrossChainConductor) Pause() {
	atomic.StoreUint32(&paused, 1)
	cc.logger.Info("CCC PAUSED - dropping all inbound and outbound messages (testnet feature)")
}

// Resume resumes CCC message processing
func (cc *CrossChainConductor) Resume() {
	atomic.StoreUint32(&paused, 0)
	cc.logger.Info("CCC RESUMED - processing messages normally")
}

// IsPaused returns true if CCC is paused
func (cc *CrossChainConductor) IsPaused() bool {
	return atomic.LoadUint32(&paused) == 1
}