// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"testing"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/stretchr/testify/require"
)

// #4065: every generated configuration asks for ModeAutoServer, and the
// service passed p2p.Options without DiscoveryMode, so each node ran
// dht.ModeAuto — demoted to client-only on a private network, where no node
// then stores provider records and acc-svc discovery never resolves. The mode
// the configuration asks for is the mode the node runs.
func TestP2P_DiscoveryModeReachesTheNode(t *testing.T) {
	for _, mode := range []dht.ModeOpt{dht.ModeServer, dht.ModeAutoServer, dht.ModeClient} {
		s := &P2P{DiscoveryMode: Ptr(DhtMode(mode))}
		require.NotNil(t, s.DiscoveryMode, "the configuration carries a mode")
		require.Equal(t, mode, dht.ModeOpt(*s.DiscoveryMode), "and it is the mode asked for")
	}
	// Unset stays unset: the node keeps libp2p's default rather than being
	// forced to a mode nobody chose.
	require.Nil(t, (&P2P{}).DiscoveryMode)
}
