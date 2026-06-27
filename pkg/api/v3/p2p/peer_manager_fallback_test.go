// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package p2p

import (
	"context"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func TestMergeServiceFallback(t *testing.T) {
	ai := func(id string) peer.AddrInfo { return peer.AddrInfo{ID: peer.ID(id)} }
	collect := func(ch <-chan peer.AddrInfo) []string {
		var ids []string
		for p := range ch {
			ids = append(ids, string(p.ID))
		}
		return ids
	}

	// DHT yields a,b; fallback adds b (dup) and c -> a,b,c (DHT first, deduped).
	dht := make(chan peer.AddrInfo, 2)
	dht <- ai("a")
	dht <- ai("b")
	close(dht)
	out := mergeServiceFallback(context.Background(), dht, 0, []peer.AddrInfo{ai("b"), ai("c")})
	require.Equal(t, []string{"a", "b", "c"}, collect(out))

	// DHT empty -> only the fallback (the backstop).
	empty := make(chan peer.AddrInfo)
	close(empty)
	out2 := mergeServiceFallback(context.Background(), empty, 0, []peer.AddrInfo{ai("x")})
	require.Equal(t, []string{"x"}, collect(out2))

	// No fallback peers -> DHT only.
	d3 := make(chan peer.AddrInfo, 1)
	d3 <- ai("z")
	close(d3)
	out3 := mergeServiceFallback(context.Background(), d3, 0, nil)
	require.Equal(t, []string{"z"}, collect(out3))
}
