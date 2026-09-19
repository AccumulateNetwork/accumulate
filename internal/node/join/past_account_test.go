// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// withReceipt is the production querier with a receipt stapled on. The peer
// here is a store that holds the account but no block index, so it cannot
// build a real proof; the pull needs a receipt to accept the body at all, and
// what this test is about is what the FETCH LOOP does with an account the node
// is past — which is decided before any receipt is looked at.
type withReceipt struct{ pull.Source }

func (s withReceipt) QueryAccount(ctx context.Context, u *url.URL, _ *api.DefaultQuery) (*api.AccountRecord, error) {
	r, err := s.Source.QueryAccount(ctx, u, nil)
	if err != nil {
		return nil, err
	}
	r.Receipt = &api.Receipt{Partition: "BVN0", LocalBlock: 7}
	return r, nil
}

// oneSource hands the fetch loop a single peer for every account.
type oneSource struct {
	part *url.URL
	src  pull.Source
}

func (o *oneSource) For(context.Context, *url.URL) ([]pull.Source, *url.URL, error) {
	return []pull.Source{o.src}, o.part, nil
}
func (o *oneSource) Querier(*url.URL) api.Querier { return nil }

func writeAccount(t *testing.T, db *database.Database, u *url.URL, entries int) {
	t.Helper()
	b := db.Begin(true)
	defer b.Discard()
	require.NoError(t, b.Account(u).Main().Put(&protocol.DataAccount{Url: u}))
	c, err := b.Account(u).ChainByName("main")
	require.NoError(t, err)
	_, err = c.Get()
	require.NoError(t, err)
	for i := 0; i < entries; i++ {
		e := make([]byte, 32)
		e[0], e[1] = byte(i), byte(i>>8)
		require.NoError(t, c.Inner().AddEntry(e, false))
	}
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())
}

// TestFetch_AnAccountTheNodeIsPastIsNeitherHeldNorRefused — the fetch loop's
// half of #4348.
//
// An account the node is past took nothing from the peer: there is no state to
// verify and no block to wait for an anchor for. Holding it would keep a round
// batch open across four settle rounds and a place under pull.MaxHeld, for
// state the node is never going to take — and in the case this arises in, a
// node ahead of its peer, it is not one account but every account the node is
// ahead on, every round (enumerate.Stale names a difference in either
// direction).
//
// Refusing it would be wrong for the other reason: a refusal is asked for
// again next round for the life of the process, and nothing failed here.
func TestFetch_AnAccountTheNodeIsPastIsNeitherHeldNorRefused(t *testing.T) {
	ctx := context.Background()
	here := protocol.PartitionUrl("BVN0")
	u := protocol.AccountUrl("alice", "tokens")

	// The peer holds the node's own first 20 entries; the node holds 50.
	peer := database.OpenInMemory(nil)
	peer.SetObserver(database.NewDatabaseObserver())
	t.Cleanup(func() { _ = peer.Close() })
	writeAccount(t, peer, u, 20)

	s := quietState(t, here, &oneSource{part: here, src: withReceipt{api.Querier2{
		Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peer, Partition: "BVN0"}),
	}}})
	s.db.SetObserver(database.NewDatabaseObserver())
	writeAccount(t, s.db, u, 50)

	// s.anchors must be non-nil for the fetch to ask for a receipt at all,
	// which is what the long tail does (join.fetch passes Verify: s.anchors).
	// It is never consulted: a past account has nothing to settle.
	values, _ := genesisValues(t, 4)
	s.anchors = noAnchorSource(t, here, values)

	before := pull.Held()
	pulled, refused := s.fetch(ctx, []*url.URL{u})

	require.Zero(t, pulled, "an account the node is past was counted as pulled")
	require.Empty(t, refused, "an account the node is past was refused, so it is asked for again for ever")
	require.Empty(t, s.held, "an account that took nothing is waiting for an anchor")
	require.Equal(t, before, pull.Held(), "the fetch left a held account outstanding")

	// And the node's own state is untouched.
	b := s.db.Begin(false)
	defer b.Discard()
	c, err := b.Account(u).ChainByName("main")
	require.NoError(t, err)
	head, err := c.Inner().Head().Get()
	require.NoError(t, err)
	require.Equal(t, int64(50), head.Count, "the fetch shortened the node's own chain")
}
