// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"testing"

	"github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr/net"
	"github.com/stretchr/testify/require"
)

func addrs(t *testing.T, s ...string) []multiaddr.Multiaddr {
	t.Helper()
	out := make([]multiaddr.Multiaddr, len(s))
	for i, v := range s {
		a, err := multiaddr.NewMultiaddr(v)
		require.NoError(t, err)
		out[i] = a
	}
	return out
}

func strs(a []multiaddr.Multiaddr) []string {
	out := make([]string, len(a))
	for i, v := range a {
		out[i] = v.String()
	}
	return out
}

func TestDialableAddrs(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			// The mainnet case this exists for: the peerstore holds both the
			// routable address and the loopback one this node uses to reach
			// services on itself. Only the former is any use to a caller.
			name: "drops loopback when a routable address exists",
			in:   []string{"/ip4/127.0.0.1/tcp/16593", "/ip4/206.191.154.166/tcp/16593"},
			want: []string{"/ip4/206.191.154.166/tcp/16593"},
		},
		{
			// A devnet separates its nodes by loopback address rather than by
			// port (#4060). Filtering here would return an empty list and
			// break discovery outright, so the whole set passes through.
			name: "passes loopback through when there is nothing else",
			in:   []string{"/ip4/127.0.1.2/tcp/26656", "/ip4/127.0.1.2/tcp/26657"},
			want: []string{"/ip4/127.0.1.2/tcp/26656", "/ip4/127.0.1.2/tcp/26657"},
		},
		{
			// Same reasoning for RFC1918: a node reachable only on a LAN is a
			// legitimate deployment, not a broken one.
			name: "passes private ranges through when there is nothing else",
			in:   []string{"/ip4/10.0.0.4/tcp/16593", "/ip4/192.168.1.7/tcp/16593"},
			want: []string{"/ip4/10.0.0.4/tcp/16593", "/ip4/192.168.1.7/tcp/16593"},
		},
		{
			name: "drops private ranges when a routable address exists",
			in:   []string{"/ip4/10.0.0.4/tcp/16593", "/ip4/162.217.96.197/tcp/16593"},
			want: []string{"/ip4/162.217.96.197/tcp/16593"},
		},
		{
			name: "keeps every routable address",
			in:   []string{"/ip4/206.191.154.166/tcp/16593", "/ip4/206.191.154.166/tcp/16693"},
			want: []string{"/ip4/206.191.154.166/tcp/16593", "/ip4/206.191.154.166/tcp/16693"},
		},
		{
			name: "empty stays empty",
			in:   []string{},
			want: []string{},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := dialableAddrs(addrs(t, c.in...))
			require.Equal(t, c.want, strs(got))
		})
	}
}

// TestDialableAddrs_MainnetSample replays the exact response that motivated
// the fix: find-service for query:Directory on 2026-08-17. Four of the ten
// peers were undialable from outside the network.
//
// The point of the assertion is narrow and worth stating: filtering must not
// reduce the number of USABLE peers. It removes noise from peers that are
// reachable and leaves untouched the ones that are not, so the count of
// peers a client can actually dial is identical before and after.
func TestDialableAddrs_MainnetSample(t *testing.T) {
	sample := [][]string{
		{"/ip4/206.191.154.166/tcp/16593", "/ip4/206.191.154.166/tcp/16693", "/ip4/127.0.0.1/tcp/16593", "/ip4/127.0.0.1/tcp/16693"},
		{"/ip4/127.0.0.1/tcp/16593", "/ip4/127.0.0.1/tcp/16693"},
		{"/ip4/127.0.0.1/tcp/16593", "/ip4/127.0.0.1/tcp/16693", "/ip4/162.217.96.197/tcp/16593", "/ip4/162.217.96.197/tcp/16693"},
		{"/ip4/63.251.238.156/tcp/16593", "/ip4/63.251.238.156/tcp/16693", "/ip4/127.0.0.1/tcp/16593", "/ip4/127.0.0.1/tcp/16693"},
	}

	var routableBefore, routableAfter, noise int
	for _, s := range sample {
		in := addrs(t, s...)
		out := dialableAddrs(in)

		var hadRoutable bool
		for _, a := range in {
			if isRoutableForTest(a) {
				hadRoutable = true
				routableBefore++
			}
		}
		for _, a := range out {
			if isRoutableForTest(a) {
				routableAfter++
			} else {
				noise++
			}
		}

		if hadRoutable {
			// Every address handed back for a reachable peer is dialable.
			for _, a := range out {
				require.True(t, isRoutableForTest(a),
					"peer with a routable address should not advertise %s", a)
			}
		} else {
			// Nothing routable: unchanged, so no peer is ever made worse off.
			require.Equal(t, strs(in), strs(out))
		}
	}

	require.Equal(t, routableBefore, routableAfter, "no routable address may be lost")
	require.Equal(t, 2, noise, "only the loopback-only peer's own addresses remain")
}

func isRoutableForTest(a multiaddr.Multiaddr) bool {
	return manet.IsPublicAddr(a)
}
