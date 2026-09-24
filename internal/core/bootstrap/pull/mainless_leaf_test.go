// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package pull

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	v3impl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// hideBody is a peer that answers an account with the receipt the real
// querier built and no body -- the answer an honest peer gives for a leaf
// with no main state, given for an account that has one. swap, when set,
// serves another account's receipt instead.
type hideBody struct {
	Source
	swap *url.URL
}

func (h hideBody) QueryAccount(ctx context.Context, u *url.URL, q *api.DefaultQuery) (*api.AccountRecord, error) {
	if h.swap != nil {
		u = h.swap
	}
	rec, err := h.Source.QueryAccount(ctx, u, q)
	if err != nil {
		return nil, err
	}
	rec.Account = nil
	return rec, nil
}

// TestALeafWithNoBodyIsPulledAndVerified drives the pull through the querier
// that ships (#4397). A leaf with no main state -- both families the load
// generator's failed work leaves, a principal given signature chains and a
// failed deposit's empty account -- is pulled, verified against the anchored
// root, and written, so the local leaf is the peer's. And a peer that answers
// "no body" for an account that HAS one is refused by the leaf check, whether
// it serves that account's true receipt or another mainless leaf's.
func TestALeafWithNoBodyIsPulledAndVerified(t *testing.T) {
	const partitionID = "PullMainless"
	part := protocol.PartitionUrl(partitionID)
	sysLedger := part.JoinPath(protocol.Ledger)
	ghost := url.MustParse("alice/ghostdata1")
	void := url.MustParse("void-9aac09e22e861b50/tokens")
	bodied := url.MustParse("alice/data")

	src := newObservedDB(t)
	// A signature's hash, as RecordHistory appends it. Not zero: a chain whose
	// only entry is zero has the empty chain's anchor, and the leaf would be
	// the empty account's.
	sigHash := sha256.Sum256([]byte("a signature on a principal that does not exist"))
	b := src.Begin(true)
	ledger := &protocol.SystemLedger{Url: sysLedger, Index: 9}
	require.NoError(t, b.Account(sysLedger).Main().Put(ledger))
	require.NoError(t, b.Account(ghost).SignatureChain().Inner().AddEntry(sigHash[:], false))
	require.NoError(t, b.Account(void).MarkDirty())
	require.NoError(t, b.Account(bodied).Main().Put(&protocol.DataAccount{Url: bodied}))
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())

	b = src.Begin(true)
	idx, err := b.Account(sysLedger).RootChain().Index().Get()
	require.NoError(t, err)
	entry, err := (&protocol.IndexEntry{BlockIndex: ledger.Index}).MarshalBinary()
	require.NoError(t, err)
	require.NoError(t, idx.AddEntry(entry, false))
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())

	b = src.Begin(false)
	root, err := b.GetBptRootHash()
	require.NoError(t, err)
	b.Discard()

	leaf := func(db *database.Database, u *url.URL) ([]byte, error) {
		b := db.Begin(false)
		defer b.Discard()
		return b.BPT().Get(b.Account(u).Key())
	}

	// The two families have different leaves; were they the same, the test
	// below would be one case twice.
	gl, err := leaf(src, ghost)
	require.NoError(t, err)
	vl, err := leaf(src, void)
	require.NoError(t, err)
	require.NotEqual(t, gl, vl)

	honest := api.Querier2{Querier: v3impl.NewQuerier(v3impl.QuerierParams{Database: src, Partition: partitionID})}
	opts := Options{Mode: ModeStateOnly, Verify: anchored{root: root, block: ledger.Index}, Partition: part}
	ctx := context.Background()

	for _, u := range []*url.URL{ghost, void} {
		t.Run(u.String(), func(t *testing.T) {
			dst := newObservedDB(t)
			batch := dst.Begin(true)
			require.NoError(t, Account(ctx, honest, batch, u, opts), "a leaf with no body was not pulled")
			require.NoError(t, batch.UpdateBPT())
			require.NoError(t, batch.Commit())

			want, err := leaf(src, u)
			require.NoError(t, err)
			got, err := leaf(dst, u)
			require.NoError(t, err, "the pulled account has no leaf locally")
			require.Equal(t, want, got, "the pulled leaf is not the peer's")
		})
	}

	for _, c := range []struct {
		name string
		peer Source
	}{
		{"its own receipt", hideBody{Source: honest}},
		{"another mainless leaf's receipt", hideBody{Source: honest, swap: ghost}},
	} {
		t.Run("a peer hiding a body is refused/"+c.name, func(t *testing.T) {
			dst := newObservedDB(t)
			batch := dst.Begin(true)
			require.Error(t, Account(ctx, c.peer, batch, bodied, opts),
				"a peer that served no body for an account with one was believed")
			require.NoError(t, batch.UpdateBPT())
			require.NoError(t, batch.Commit())
			_, err := leaf(dst, bodied)
			require.Error(t, err, "the refused answer put a leaf in the local tree")
		})
	}

	// THE LIMIT of one source, pinned so that it is stated rather than
	// discovered; with several, FetchFrom's unanimity rule refuses a lone liar
	// (TestALiarAmongHonestPeersCannotPlantAPhantomLeaf) and this is the case
	// where every source lies (TestUnanimousLiarsPlantAPhantomLeaf). A BPT
	// leaf is the account's state hash and nothing else -- the key is not
	// hashed into the tree -- and a leaf with no body carries no URL, so every
	// empty account's leaf is the same hash. A peer that names an account the
	// tree holds no leaf for, serves it as "no body" with an empty account's
	// receipt, and serves no chains, directory or pending for it, is believed:
	// the receipt passes through the leaf that state hashes to, and it ends at
	// the anchored root. The whole-root match is what refuses the result --
	// the local tree then holds a leaf no peer's does -- so this is a join
	// that does not finish, not a node that executes from a wrong state.
	// DIFFERENCES.md E11 records it.
	t.Run("the limit: an empty account's receipt proves any empty account", func(t *testing.T) {
		phantom := url.MustParse("nobody/tokens")
		dst := newObservedDB(t)
		batch := dst.Begin(true)
		require.NoError(t, Account(ctx, hideBody{Source: honest, swap: void}, batch, phantom, opts),
			"if this now fails, the limit is closed: update DIFFERENCES.md E11 and flip this test")
		require.NoError(t, batch.UpdateBPT())
		require.NoError(t, batch.Commit())
		_, err := leaf(dst, phantom)
		require.NoError(t, err)
	})
}
