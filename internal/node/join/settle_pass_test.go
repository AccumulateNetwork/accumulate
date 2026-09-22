// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// servedAt is the production querier with a receipt stapled on that ends at
// the root the test names for the account, zero for an account it names no
// root for. The peer here is a store with no block index, so it cannot build a
// proof to its BPT root; the receipt is the account's own state-tree receipt,
// which ends at the account's leaf, with the named root written over its end.
// What these tests are about is what settlePass does with the roots a pass
// ends at, which is decided before any receipt is verified; a test that names
// the leaf itself as the root (leafOf) gets a receipt that verifies.
type servedAt struct {
	pull.Source
	db    *database.Database
	roots map[string][32]byte
}

func (s servedAt) QueryAccount(ctx context.Context, u *url.URL, _ *api.DefaultQuery) (*api.AccountRecord, error) {
	r, err := s.Source.QueryAccount(ctx, u, nil)
	if err != nil {
		return nil, err
	}
	r.Receipt = &api.Receipt{Partition: "BVN0", LocalBlock: 7}
	root, ok := s.roots[strings.ToLower(u.String())]
	if !ok {
		return r, nil // Served at no root
	}
	batch := s.db.Begin(false)
	defer batch.Discard()
	leaf, err := batch.Account(u).StateTreeReceipt()
	if err != nil {
		return nil, err
	}
	r.Receipt.Receipt = *leaf
	r.Receipt.Anchor = root[:]
	return r, nil
}

// leafOf is the leaf the account heldPass writes into the peer hashes to,
// computed over a store of its own: the one root a receipt from the peer
// here can be verified against.
func leafOf(t *testing.T, u *url.URL) [32]byte {
	t.Helper()
	db := database.OpenInMemory(nil)
	db.SetObserver(database.NewDatabaseObserver())
	defer db.Close()
	writeAccount(t, db, u, 3)
	batch := db.Begin(false)
	defer batch.Discard()
	leaf, err := batch.Account(u).Hash()
	require.NoError(t, err)
	return leaf
}

// heldPass fetches the named accounts as one pass, each ending at the root the
// test gives it, and returns the state holding it. anchors is where the roots
// a pass ends at are proven; nil is a Directory that has anchored nothing,
// so no root is proven and none is passed.
func heldPass(t *testing.T, anchors *anchorsrc.Source, roots map[string][32]byte, accounts ...*url.URL) *PulledState {
	t.Helper()
	here := protocol.PartitionUrl("BVN0")

	peer := database.OpenInMemory(nil)
	peer.SetObserver(database.NewDatabaseObserver())
	t.Cleanup(func() { _ = peer.Close() })
	for _, u := range accounts {
		writeAccount(t, peer, u, 3)
	}

	keyed := map[string][32]byte{}
	for u, r := range roots {
		keyed[strings.ToLower(u)] = r
	}
	s := quietState(t, here, &oneSource{part: here, src: servedAt{
		Source: api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peer, Partition: "BVN0"})},
		db:     peer,
		roots:  keyed,
	}})
	s.db.SetObserver(database.NewDatabaseObserver())
	if anchors == nil {
		values, _ := genesisValues(t, 4)
		anchors = noAnchorSource(t, here, values)
	}
	s.anchors = anchors
	s.spine = true // the spine is not what these are about

	s.fetchPass(context.Background(), accounts)
	require.NotNil(t, s.pass, "nothing was fetched, so there is no pass to settle")
	require.Len(t, s.pass.accounts, len(accounts))
	require.Empty(t, s.refused)
	return s
}

func urlsOf(us []*url.URL) []string {
	var out []string
	for _, u := range us {
		out = append(out, u.String())
	}
	return out
}

// A root no verified anchor carries yet is the one wait: the next anchor may
// carry it. The pass stays held, whole, and nothing is asked for again.
func TestSettlePass_ARootNotProvenYetIsHeld(t *testing.T) {
	a, b := protocol.AccountUrl("alice", "tokens"), protocol.AccountUrl("bob", "tokens")
	before := pull.Held()
	// No anchor is verified, so ProveRoot answers "not yet" without an error.
	s := heldPass(t, nil, map[string][32]byte{a.String(): {1}, b.String(): {1}}, a, b)

	for i := 0; i < 50; i++ {
		s.settlePass(context.Background())
	}

	require.NotNil(t, s.pass, "a pass waiting for an anchor was given up on: there is no bound on rounds")
	require.Len(t, s.pass.accounts, 2)
	require.Empty(t, s.refused)
	require.Equal(t, before+2, pull.Held())
	s.dropPass()
	require.Equal(t, before, pull.Held())
}

// An error from ProveRoot is not a wait -- no anchor to come changes it -- so
// the pass is dropped and all of it is asked for again. The error here is the
// one anchorsrc pins: the pass was served at block 7, the anchor of block 8 is
// verified and carries another root, so block 7's anchor would be verified
// too if there were one, and no verified anchor carries this root.
func TestSettlePass_ARootThatWillNotProveIsDroppedAndFetchedAgain(t *testing.T) {
	a, b := protocol.AccountUrl("alice", "tokens"), protocol.AccountUrl("bob", "tokens")
	before := pull.Held()
	values, keys := genesisValues(t, 4)
	passed := anchoredSource(t, values, signedAnchor(t, values, keys, 8, [32]byte{2}))
	s := heldPass(t, passed, map[string][32]byte{a.String(): {1}, b.String(): {1}}, a, b)

	s.settlePass(context.Background())

	require.Nil(t, s.pass, "a pass nothing will ever prove is still held, and nothing new is fetched while it is")
	require.ElementsMatch(t, []string{a.String(), b.String()}, urlsOf(s.refused))
	require.Equal(t, before, pull.Held(), "the dropped pass left held accounts outstanding")
}

// An account served with no receipt has no root to be proven against. It is
// fetched again and the rest of the pass is not held up by it.
func TestSettlePass_AnAccountServedAtNoRootIsFetchedAgain(t *testing.T) {
	a, b := protocol.AccountUrl("alice", "tokens"), protocol.AccountUrl("bob", "tokens")
	s := heldPass(t, nil, map[string][32]byte{a.String(): {1}}, a, b)

	s.settlePass(context.Background())

	require.Equal(t, []string{b.String()}, urlsOf(s.refused))
	require.NotNil(t, s.pass)
	require.Len(t, s.pass.accounts, 1)
	require.Equal(t, a.String(), s.pass.accounts[0].url.String())
	s.dropPass()
}

// A pass whose accounts were all served with no receipt is no pass at all.
func TestSettlePass_APassAtNoRootIsNotHeld(t *testing.T) {
	a := protocol.AccountUrl("alice", "tokens")
	before := pull.Held()
	s := heldPass(t, nil, nil, a)

	s.settlePass(context.Background())

	require.Nil(t, s.pass)
	require.Equal(t, []string{a.String()}, urlsOf(s.refused))
	require.Equal(t, before, pull.Held())
}

// The peers move while a pass is fetched, so its accounts can end at different
// roots. Written together they are a state no block had. The root most of the
// pass ends at is kept and the minority is fetched again.
func TestSettlePass_TheMinorityRootIsFetchedAgain(t *testing.T) {
	a, b, c := protocol.AccountUrl("alice", "tokens"), protocol.AccountUrl("bob", "tokens"), protocol.AccountUrl("carol", "tokens")
	s := heldPass(t, nil, map[string][32]byte{a.String(): {1}, b.String(): {2}, c.String(): {1}}, a, b, c)

	s.settlePass(context.Background())

	require.Equal(t, []string{b.String()}, urlsOf(s.refused), "the account served at another root is asked for again")
	require.NotNil(t, s.pass)
	var kept []string
	for _, h := range s.pass.accounts {
		kept = append(kept, h.url.String())
		require.Equal(t, [32]byte{1}, h.pending.Root(), "one pass is one root")
	}
	require.ElementsMatch(t, []string{a.String(), c.String()}, kept)
	s.dropPass()
}
