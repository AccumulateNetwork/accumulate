// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"testing"

	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

func TestEnforcePrivateConcealment(t *testing.T) {
	maddr := func(s string) multiaddr.Multiaddr { return multiaddr.StringCast(s) }
	tr := true

	// Public node: no-op, no error.
	require.NoError(t, (&P2P{}).enforcePrivateConcealment(nil))

	// Private + a publicly-reachable listen: refuse to start (#4047 §2).
	for _, h := range []string{"/ip4/0.0.0.0/tcp/16593", "/ip4/203.0.113.5/tcp/16593"} {
		p := &P2P{Private: &tr, Listen: []multiaddr.Multiaddr{maddr(h)}}
		require.Error(t, p.enforcePrivateConcealment(nil), h)
	}

	// Private + a guard-facing listen: allowed, and DHT forced to client mode.
	for _, h := range []string{"/ip4/127.0.0.1/tcp/16593", "/ip4/10.0.0.5/tcp/16593"} {
		p := &P2P{Private: &tr, Listen: []multiaddr.Multiaddr{maddr(h)}}
		require.NoError(t, p.enforcePrivateConcealment(nil), h)
		require.NotNil(t, p.DiscoveryMode)
		require.Equal(t, DhtMode(dht.ModeClient), *p.DiscoveryMode, "private node must use DHT client mode")
	}
}

func TestIsPublicHost(t *testing.T) {
	public := []string{"0.0.0.0", "203.0.113.5", "8.8.8.8", "example.com"}
	private := []string{"127.0.0.1", "10.0.0.5", "192.168.1.1", "172.16.0.1"}
	for _, h := range public {
		require.True(t, isPublicHost(h), h)
	}
	for _, h := range private {
		require.False(t, isPublicHost(h), h)
	}
}
