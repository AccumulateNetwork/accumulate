// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
)

// #4419. A joining BVN whose Directory peers cannot serve an anchor signed --
// the rolling restart of #4413: one peer joined by pull and refuses its pulled
// range, the other is itself joining and refuses everything -- is held at the
// FIRST entry no peer serves, not at the start of the page that holds it, and
// says so: which entry, and which peers it asked.
//
// Through the production wiring: join.QueryPeers finds the Directory's query
// service and addresses each peer by name over the simulator's client; each
// node answers with its registered querier behind its join's gate.
func TestAJoinHeldAtAnAnchorNoPeerServesSaysWhereAndWhom(t *testing.T) {
	const joiner, rebooted = 1, 2
	sim, p, _, _ := joinADirectoryNodeByPull(t)
	ctx := context.Background()
	dnPool := DnUrl().JoinPath(AnchorPool)
	bvn := PartitionUrl("BVN0")
	authority, err := anchorsrc.FromStore(sim.S.Partition("BVN0").NodeDatabase(0), bvn)
	require.NoError(t, err)

	// The first entry the joined node will not serve, asked one at a time of
	// that node alone.
	addr := api.ServiceTypeQuery.AddressFor(Directory).Multiaddr()
	joined := api.Querier2{Querier: sim.S.Services().ForPeer(p.NodePeerID(joiner)).ForAddress(addr)}
	chain, err := joined.QueryChain(ctx, dnPool, &api.ChainQuery{Name: "main"})
	require.NoError(t, err)
	first := chain.Count
	for i := uint64(0); i < chain.Count; i++ {
		one, expand := uint64(1), true
		_, err := joined.QueryChainEntries(ctx, dnPool, &api.ChainQuery{
			Name: "main", Range: &api.RangeOptions{Start: i, Count: &one, Expand: &expand},
		})
		if err != nil {
			require.True(t, errors.Is(err, errors.NotReady), "entry %d: %v", i, err)
			first = i
			break
		}
	}
	require.Less(t, first, chain.Count, "precondition: the joined node refuses some of its pulled range")
	require.NotZero(t, first%anchorsrc.DefaultPageSize,
		"precondition: the first refused entry (%d) is not at a page boundary, or the page boundary and the entry cannot be told apart", first)
	t.Logf("dn.acme/anchors: %d entries; the joined node refuses from entry %d", chain.Count, first)

	// The only other Directory node restarts, and refuses everything while it
	// joins. The reader is node 0, which never asks itself.
	p.RestartNode(rebooted)
	peers := &join.QueryPeers{Client: sim.S.Services(), Network: t.Name(), Self: p.NodePeerID(0)}
	querier := peers.Querier(DnUrl())
	ring, ok := querier.(anchorsrc.Peers)
	require.True(t, ok, "the join's querier says how many peers it asks and which")
	require.Equal(t, 2, ring.PeerCount(ctx))

	src, err := anchorsrc.New(querier, dnPool, bvn, authority)
	require.NoError(t, err)
	src.Backfill = chain.Count // the whole chain: the window is not the question here
	var verified int
	src.OnAnchor = func(*url.URL, uint64, [32]byte) { verified++ }

	for read := 1; read <= 2; read++ {
		err = src.Read(ctx)
		require.Error(t, err)
		require.True(t, errors.Is(err, errors.NotReady), "%v", err)
		st, held := src.Stalled()
		require.True(t, held, "read %d: the read is held and does not say so", read)
		t.Logf("read %d: %d BVN0 anchors verified; held at entry %d, asked %v: %v", read, verified, st.Entry, st.Asked, st.Err)
		require.Equal(t, first, st.Entry, "read %d: held at the page's start, not at the first entry no peer serves", read)
		require.ElementsMatch(t, []string{p.NodePeerID(joiner).String(), p.NodePeerID(rebooted).String()}, st.Asked,
			"read %d: the peers asked are named", read)
	}
}
