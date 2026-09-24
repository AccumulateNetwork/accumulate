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
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// cadencePeer is a partition's peers as the join's reads see them: a ledger
// a few blocks ahead of this node, blocks whose ledger records name nothing
// beyond the system accounts every block changes, and a BPT whose pages are
// counted. It answers nothing else.
type cadencePeer struct {
	partition *url.URL
	block     uint64
	pages     int
}

func (p *cadencePeer) Query(_ context.Context, scope *url.URL, q api.Query) (api.Record, error) {
	switch q.(type) {
	case *api.DefaultQuery:
		if scope.Equal(p.partition.JoinPath(protocol.Ledger)) {
			ledger := &protocol.SystemLedger{Url: scope, Index: p.block}
			return &api.AccountRecord{Account: ledger}, nil
		}
	case *api.BptPageQuery:
		p.pages++
		return &api.BptPageRecord{Done: true}, nil
	}
	return nil, errors.NotFound.WithFormat("no such record")
}

// cadenceSources pulls every account from one peer and reads the partition
// through cadencePeer.
type cadenceSources struct {
	part *url.URL
	src  pull.Source
	peer *cadencePeer
}

func (c *cadenceSources) For(context.Context, *url.URL) ([]pull.Source, *url.URL, error) {
	return []pull.Source{c.src}, c.part, nil
}
func (c *cadenceSources) Querier(*url.URL) api.Querier { return c.peer }

// TestPull_ThePageDiffCadenceCountsFetchingRounds.
//
// The page diff is the backstop, and the spec requires it to run on a cadence
// whatever the block ledger named (executor spec, "Sync" §3; #4306). A round
// that is still waiting on an earlier pass settles it and returns before the
// page-diff decision is reached, so a cadence counted over every round is
// decided only on the rounds that happen to fetch. Here every pass settles on
// the round after it was fetched -- the next anchor arrives one round later --
// so the join fetches on rounds 1, 3, 5, ..., and a cadence of every eighth
// round is never decided on a multiple of eight: the backstop ran on the
// first round and never again (#4395).
//
// The ledger walk names the system accounts every block changes, so the set
// it returns is never empty and the span is never too wide: the cadence is the
// only way the page diff can run here.
func TestPull_ThePageDiffCadenceCountsFetchingRounds(t *testing.T) {
	ctx := context.Background()
	here := protocol.PartitionUrl("BVN0")
	named := []*url.URL{here.JoinPath(protocol.Ledger), here.JoinPath(protocol.Synthetic)}

	// A peer that holds what the walk names, each served at a root no
	// verified anchor carries yet.
	store := database.OpenInMemory(nil)
	store.SetObserver(database.NewDatabaseObserver())
	t.Cleanup(func() { _ = store.Close() })
	roots := map[string][32]byte{}
	for _, u := range named {
		// No chain entries: both are taken whole (pull.WholeAccounts), and a
		// made-up entry has no message behind it for a whole pull to take.
		writeAccount(t, store, u, 0)
		roots[strings.ToLower(u.String())] = [32]byte{1}
	}
	peer := &cadencePeer{partition: here, block: 5}
	s := quietState(t, here, &cadenceSources{part: here, peer: peer, src: servedAt{
		Source: api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: store, Partition: "BVN0"})},
		db:     store,
		roots:  roots,
	}})
	s.db.SetObserver(database.NewDatabaseObserver())
	s.spine = true // the spine is not what this is about

	values, keys := genesisValues(t, 4)
	authority, err := anchorsrc.FromValues(values)
	require.NoError(t, err)
	s.authority = authority

	// Two Directories: one that has anchored nothing, so a pass fetched
	// against it is held, and one whose next anchor (block 8) carries
	// another root than the pass was served at (block 7), so the held pass
	// settles -- it is dropped and fetched again.
	waiting := noAnchorSource(t, here, values)
	arrived := anchoredSource(t, values, signedAnchor(t, values, keys, 8, [32]byte{2}))
	s.anchors = waiting

	const want = 2 * staleEvery
	var fetching int
	var pagesAfterFirst int
	for round := 0; fetching < want+1 && round < 10*want; round++ {
		held := s.pass != nil
		before := peer.pages
		require.NoError(t, s.Pull(ctx))
		if !held {
			fetching++
			require.NotNil(t, s.pass, "a fetching round must hold its pass, or it did not settle on the next round")
			if fetching == 1 {
				require.NotZero(t, peer.pages-before, "the first round pages, so a page query is seen when one is made")
			} else {
				pagesAfterFirst += peer.pages - before
			}
			// The anchor that settles it arrives before the next round.
			s.anchors = arrived
		} else {
			require.Nil(t, s.pass, "a held pass must settle on the round after it was fetched")
			require.Equal(t, before, peer.pages, "a settling round does not fetch, so it does not page")
			s.anchors = waiting
		}
	}
	require.Equal(t, want+1, fetching)
	require.NotZero(t, pagesAfterFirst,
		"%d fetching rounds after the first and the page diff never ran: the backstop is unreachable (#4395)", want)
}
