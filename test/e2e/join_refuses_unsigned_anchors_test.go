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
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
)

// #4413. A Directory node that joined by a pull from before #4416 holds the
// anchors in its pulled range without their signatures: that pull did not
// bring them. It used to serve them anyway, and every node reading its roots
// from it refused them as "the anchor carries no signatures" -- 22 BVN3
// anchors in run 20260924T074702Z. It now refuses them with NotReady, and a
// joining BVN reading the Directory's pool through its peers, as the daemon
// does, takes every one of them from a node that executed them.
//
// Since #4416 the pull brings them, so the joined node is made into such a
// peer -- one that joined with an older binary -- by forgetting what the pull
// wrote beside each signature of an anchor it did not execute
// (forgetPulledSignatures). TestAJoinedNodeServesTheAnchorsItPulledWithTheirSignatures
// holds the pull itself.
//
// Everything here goes through the production wiring: each Directory node's
// registered querier behind its join's gate (the simulator registers it as the
// daemon does), reached by join.QueryPeers, which finds the query service's
// peers and addresses each one by name.
func TestAJoinedNodeRefusesTheAnchorsItHoldsWithoutSignatures(t *testing.T) {
	const joiner = 1
	sim, p, r, q := joinADirectoryNodeByPull(t)
	forgetPulledSignatures(t, p.NodeDatabase(joiner))
	require.True(t, p.NodeJoinState(joiner).Machine().CanServeCurrent(),
		"precondition: the joined node reads ACTIVE, so its gate lets it serve (#4368)")

	ctx := context.Background()
	dnPool := DnUrl().JoinPath(AnchorPool)
	bvn := PartitionUrl("BVN0")
	authority, err := anchorsrc.FromStore(sim.S.Partition("BVN0").NodeDatabase(0), bvn)
	require.NoError(t, err)
	pool, err := anchorsrc.PoolFor(bvn, authority.BvnNames())
	require.NoError(t, err)
	require.True(t, pool.Equal(dnPool))

	peers := &join.QueryPeers{Client: sim.S.Services(), Network: t.Name()}

	// One Directory node, addressed by name, as QueryPeers addresses each peer.
	nodeQuerier := func(node int) api.Querier {
		addr := api.ServiceTypeQuery.AddressFor(Directory).Multiaddr()
		return sim.S.Services().ForPeer(p.NodePeerID(node)).ForAddress(addr)
	}

	read := func(name string, querier api.Querier, reads int) (verified map[uint64]bool, refused map[uint64]string, errs []error) {
		src, err := anchorsrc.New(querier, pool, bvn, authority)
		require.NoError(t, err)
		verified, refused = map[uint64]bool{}, map[uint64]string{}
		src.OnRefused = func(block uint64, err error) { refused[block] = err.Error() }
		src.OnAnchor = func(_ *url.URL, block uint64, _ [32]byte) { verified[block] = true }
		for i := 0; i < reads; i++ {
			// A fresh read of the whole window each time, so every read asks
			// for the pulled range again.
			src.Rewind()
			if err := src.Read(ctx); err != nil {
				errs = append(errs, err)
			}
		}
		t.Logf("%s: %d BVN0 anchors verified, %d refused, %d reads failed", name, len(verified), len(refused), len(errs))
		return verified, refused, errs
	}

	// The control: a Directory node that executed every block.
	want, refused, errs := read("node 0", nodeQuerier(0), 1)
	require.Empty(t, errs)
	require.Empty(t, refused, "control: a node that executed every block serves every anchor signed")
	require.NotEmpty(t, want)
	var inPulledRange int
	for block := range want {
		if block > r {
			inPulledRange++
		}
	}
	require.NotZero(t, inPulledRange, "precondition: BVN0 anchored blocks the joined node holds only by pull (R=%d..Q=%d)", r, q)

	// The joined node, asked directly for its pool: NotReady, never a record
	// with no signatures.
	count, expand := uint64(64), true
	_, err = api.Querier2{Querier: nodeQuerier(joiner)}.QueryMainChainEntries(ctx, dnPool, &api.ChainQuery{
		Name:  "main",
		Range: &api.RangeOptions{Start: 0, Count: &count, Expand: &expand},
	})
	require.Error(t, err, "the joined node served its pulled anchors")
	require.True(t, errors.Is(err, errors.NotReady), "the joined node refuses with NotReady so the caller asks another node: %v", err)

	// Read through it alone, nothing is refused as unsigned: nothing unsigned
	// is served.
	_, refused, errs = read("node 1 alone", nodeQuerier(joiner), 1)
	require.Empty(t, refused, "the joined node served anchors without their signatures")
	require.NotEmpty(t, errs)
	require.True(t, errors.Is(errs[0], errors.NotReady), "%v", errs[0])

	// Through the Directory's peers, as a joining BVN0 node reads them. The
	// peers rotate per call and there are two calls per read, so over six
	// reads the page call starts at every peer, the joined one included.
	got, refused, errs := read("the Directory's peers", peers.Querier(DnUrl()), 6)
	require.Empty(t, errs)
	require.Empty(t, refused, "an anchor reached the reader without its signatures")
	require.Equal(t, want, got, "every anchor the control serves is taken through the peers, and no root is skipped")
}

// forgetPulledSignatures makes a node that joined by pull hold its pulled
// anchors as a pull from before #4416 left them: the entries and their
// messages, and nothing an anchor's signatures are read from. An anchor the
// node executed has a delivered status; one it holds only by pull has none,
// because the pull does not bring a status.
func forgetPulledSignatures(t *testing.T, db *database.Database) {
	t.Helper()
	pool := DnUrl().JoinPath(AnchorPool)
	var forgot int
	Update(t, db, func(batch *database.Batch) {
		head, err := batch.Account(pool).MainChain().Head().Get()
		require.NoError(t, err)
		for i := int64(0); i < head.Count; i++ {
			h, err := batch.Account(pool).MainChain().Entry(i)
			require.NoError(t, err)
			st, err := batch.Transaction(h).Status().Get()
			require.NoError(t, err)
			if st.Delivered() {
				continue
			}
			txn := batch.Account(pool).Transaction(*(*[32]byte)(h))
			hist, err := txn.History().Get()
			require.NoError(t, err)
			for _, j := range hist {
				require.NoError(t, txn.History().Remove(j))
			}
			if len(hist) > 0 {
				forgot++
			}
		}
	})
	require.NotZero(t, forgot, "precondition: the node holds anchors it did not execute")
	t.Logf("forgot the signature history of %d pulled anchors", forgot)
}
