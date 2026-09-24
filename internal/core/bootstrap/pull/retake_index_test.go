// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute/v2/chain"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
)

// TestFullSpine_ARetakenChainAppendsWhatOnlyItsWrongGrowthHeld — #4444,
// through the production pull and the executor's append. A node that
// executed from a wrong state appended X of its own; the pull finds its chain
// is not the peer's and takes it whole. Then X arrives honestly and every
// node appends it through ChainUpdates.AddChainEntry, which asks for a unique
// append. The retaken node's element index still named the position X had
// held, and the append was skipped on it: the node ended one entry short of
// its peers, on another anchor.
func TestFullSpine_ARetakenChainAppendsWhatOnlyItsWrongGrowthHeld(t *testing.T) {
	peer, u, _ := spineWithMessages(t)
	node := newObservedDB(t)
	b := node.Begin(true)
	require.NoError(t, Account(context.Background(), &dbSource{db: peer}, b, u, Options{Mode: ModeFullSpine}))
	require.NoError(t, b.Commit())

	// The node's wrong growth ends in X; the peer appends its own.
	var x [32]byte
	for _, side := range []struct {
		db   *database.Database
		salt byte
		n    int
	}{{peer, 0x42, 3}, {node, 0x99, 2}} {
		b := side.db.Begin(true)
		for i := 0; i < side.n; i++ {
			h := addTransactionEntry(t, b, u, 200+i, side.salt)
			if side.db == node {
				x = h
			}
		}
		require.NoError(t, b.UpdateBPT())
		require.NoError(t, b.Commit())
	}

	b = node.Begin(true)
	p, _, err := FetchFrom(context.Background(), []Source{&dbSource{db: peer}}, b, u, Options{Mode: ModeFullSpine, CheckHeld: true})
	require.NoError(t, err, "precondition: the diverged chain is taken whole")
	require.NoError(t, p.Keep())
	require.NoError(t, b.Commit())

	// X arrives honestly, and each node appends it as execution does.
	for _, db := range []*database.Database{peer, node} {
		b := db.Begin(true)
		require.NoError(t, new(chain.ChainUpdates).AddChainEntry(b, b.Account(u).MainChain(), x[:], 0, 0))
		require.NoError(t, b.Commit())
	}

	s := peer.Begin(false)
	defer s.Discard()
	d := node.Begin(false)
	defer d.Discard()
	want, err := s.Account(u).MainChain().Head().Get()
	require.NoError(t, err)
	got, err := d.Account(u).MainChain().Head().Get()
	require.NoError(t, err)
	require.Equal(t, want.Count, got.Count, "the retaken node skipped an honest append of a hash only its wrong chain held")
	require.Equal(t, want.Anchor(), got.Anchor())
}
