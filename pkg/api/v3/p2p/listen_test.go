// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"strings"
	"testing"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

// A node listens where it is told and nowhere else. Told nothing at
// construction, libp2p listened on its own defaults as well -- a TCP, a QUIC
// and a WebTransport port on each of IPv4 and IPv6, advertised to every peer
// -- and every WebTransport session a peer's dial began and dropped stayed in
// the library's session map for good: 114 MiB on a validator after forty
// hours of followers joining and leaving (#4542).
func TestANodeListensOnlyWhereItIsTold(t *testing.T) {
	n, err := New(Options{Network: t.Name(), Listen: []multiaddr.Multiaddr{
		multiaddr.StringCast("/ip4/127.0.0.1/tcp/0"),
		multiaddr.StringCast("/ip4/127.0.0.1/udp/0/quic"),
	}})
	require.NoError(t, err)
	defer func() { _ = n.Close() }()

	addrs := n.Host().Network().ListenAddresses()
	var tcp, quic int
	for _, a := range addrs {
		s := a.String()
		if s == "/p2p-circuit" {
			continue // The relay client: reachable through a relay, no socket
		}
		require.True(t, strings.HasPrefix(s, "/ip4/127.0.0.1/"), "listening where it was not told to: %v", s)
		require.NotContains(t, s, "webtransport")
		require.NotContains(t, s, "/ws")
		switch {
		case strings.Contains(s, "/tcp/"):
			tcp++
		case strings.HasSuffix(s, "/quic-v1"):
			quic++
		default:
			t.Fatalf("listening on %v", s)
		}
	}
	require.Equal(t, 1, tcp, "addresses: %v", addrs)
	require.Equal(t, 1, quic, "the address written /quic is not listened on as /quic-v1: %v", addrs)
}

// A node told to listen nowhere listens nowhere: a client of the network
// dials out and accepts nothing.
func TestANodeToldNothingListensOnNothing(t *testing.T) {
	n, err := New(Options{Network: t.Name()})
	require.NoError(t, err)
	defer func() { _ = n.Close() }()
	for _, a := range n.Host().Network().ListenAddresses() {
		require.Equal(t, "/p2p-circuit", a.String(), "a socket nobody asked for")
	}
}
