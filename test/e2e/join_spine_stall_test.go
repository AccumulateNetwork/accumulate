// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
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
//
// Since #4421 a join takes the pool whole in every pass, with the signatures
// behind each anchor (#4416), so the joined node no longer refuses any of its
// pulled range by itself. Its store is written here as a join before them
// left it: the anchors of the pool's main chain past a point held without
// their signature history, which is what its querier refuses by (#4413).
func TestAJoinHeldAtAnAnchorNoPeerServesSaysWhereAndWhom(t *testing.T) {
	const joiner, rebooted = 1, 2
	sim, p, _, _ := joinADirectoryNodeByPull(t)
	ctx := context.Background()
	dnPool := DnUrl().JoinPath(AnchorPool)
	unsignedFrom(t, p.NodeDatabase(joiner), dnPool)
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

	// The production join reads it the same way, and says so on the gauge
	// (review F2): a BVN0 node restarts, and the join the simulator builds
	// for it, as the daemon does, runs its round. Directory node 0 restarts
	// too, so no Directory peer the BVN0 join can ask serves that entry signed.
	p.RestartNode(0)
	b := sim.S.Partition("BVN0")
	const bvnJoiner = 1

	// Diverged, the root watch's read after the handoff.
	b.RestartNode(bvnJoiner)
	require.Equal(t, float64(-1), spineStalledGauge(t, "BVN0"), "a join built fresh is not stalled")
	_, _, err = b.NodeJoinState(bvnJoiner).Diverged(ctx)
	require.Error(t, err)
	require.Equal(t, float64(first), spineStalledGauge(t, "BVN0"), "the root watch's read does not report the stall")

	// Pull, the join's round.
	b.RestartNode(bvnJoiner)
	require.Equal(t, float64(-1), spineStalledGauge(t, "BVN0"), "a join built fresh is not stalled")
	_ = b.NodeJoinState(bvnJoiner).Pull(ctx)
	require.Equal(t, float64(first), spineStalledGauge(t, "BVN0"), "the join's round does not report the stall")
}

// unsignedFrom drops the signature history of the anchors of pool's main
// chain from a position that is not a page boundary to its head, as a join
// that took them state-only left them (#4421).
func unsignedFrom(t *testing.T, db *database.Database, pool *url.URL) {
	t.Helper()
	require.NoError(t, db.Update(func(batch *database.Batch) error {
		c := batch.Account(pool).MainChain()
		head, err := c.Head().Get()
		if err != nil {
			return err
		}
		from := head.Count - 3
		if from%int64(anchorsrc.DefaultPageSize) == 0 {
			from--
		}
		require.Positive(t, from, "precondition: the pool holds anchors to drop")
		for i := from; i < head.Count; i++ {
			h, err := c.Entry(i)
			if err != nil {
				return err
			}
			if err := batch.Account(pool).Transaction(*(*[32]byte)(h)).History().Put(nil); err != nil {
				return err
			}
		}
		return nil
	}))
}

// spineStalledGauge is accumulate_join_spine_stalled_entry for a partition,
// as the node's metrics endpoint serves it.
func spineStalledGauge(t *testing.T, partition string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() != "accumulate_join_spine_stalled_entry" {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "partition" && l.GetValue() == partition {
					return m.GetGauge().GetValue()
				}
			}
		}
	}
	t.Fatalf("no accumulate_join_spine_stalled_entry series for %s", partition)
	return 0
}
