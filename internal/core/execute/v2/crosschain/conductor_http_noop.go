// Copyright 2025 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

//go:build !testnet
// +build !testnet

package crosschain

import "net/http"

// RegisterPauseEndpoints does nothing in production builds
func (cc *CrossChainConductor) RegisterPauseEndpoints(mux *http.ServeMux) {
	// No-op - endpoints not registered in production
}
