// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// The phantom-leaf question of #4397, from the review of
// issue-4397-mainless-leaf (#4397 note_3896123320, F3).

package pull

import (
	"context"
	"crypto/sha256"
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/require"
	v3impl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// chainSwap answers the chain list for one account with another account's
// chain list (or none, when other is nil).
type chainSwap struct {
	Source
	for_  *url.URL
	other *url.URL
}

func (c chainSwap) QueryAccountChains(ctx context.Context, u *url.URL, q *api.ChainQuery) (*api.RecordRange[*api.ChainRecord], error) {
	if u.Equal(c.for_) {
		if c.other == nil {
			return &api.RecordRange[*api.ChainRecord]{}, nil
		}
		u = c.other
	}
	return c.Source.QueryAccountChains(ctx, u, q)
}

func (c chainSwap) QueryChainEntries(ctx context.Context, u *url.URL, q *api.ChainQuery) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
	if u.Equal(c.for_) && c.other != nil {
		u = c.other
	}
	return c.Source.QueryChainEntries(ctx, u, q)
}

func mainlessFixture(t *testing.T) (src *database.Database, root [32]byte, block uint64, part *url.URL, ghost, void, bodied *url.URL, partitionID string) {
	partitionID = "PhantomLeaf"
	part = protocol.PartitionUrl(partitionID)
	sysLedger := part.JoinPath(protocol.Ledger)
	ghost = url.MustParse("alice/ghostdata1")
	void = url.MustParse("void-9aac09e22e861b50/tokens")
	bodied = url.MustParse("alice/data")

	src = newObservedDB(t)
	sigHash := sha256.Sum256([]byte("a signature on a principal that does not exist"))
	b := src.Begin(true)
	ledger := &protocol.SystemLedger{Url: sysLedger, Index: 9}
	require.NoError(t, b.Account(sysLedger).Main().Put(ledger))
	require.NoError(t, b.Account(ghost).SignatureChain().Inner().AddEntry(sigHash[:], false))
	require.NoError(t, b.Account(void).MarkDirty())
	require.NoError(t, b.Account(bodied).Main().Put(&protocol.DataAccount{Url: bodied}))
	otherHash := sha256.Sum256([]byte("a different entry"))
	require.NoError(t, b.Account(bodied).MainChain().Inner().AddEntry(otherHash[:], false))
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
	root, err = b.GetBptRootHash()
	require.NoError(t, err)
	b.Discard()
	return src, root, ledger.Index, part, ghost, void, bodied, partitionID
}

func rvLeaf(db *database.Database, u *url.URL) ([]byte, error) {
	b := db.Begin(false)
	defer b.Discard()
	return b.BPT().Get(b.Account(u).Key())
}

// TestALiarAmongHonestPeersCannotPlantAPhantomLeaf (review F3). A leaf with
// no body is not bound to its account's name -- the tree hashes values, and
// every empty account's leaf is one hash -- so a peer can answer a name the
// tree holds no leaf for with an empty account's receipt and pass the leaf
// check. Before the unanimity rule the honest peer's NotFound did not stop
// it: FetchFrom moved on, the liar answered, the phantom leaf was written,
// and the name was later dropped on the honest peer's NotFound with the leaf
// still there -- the join wedged until the store was wiped. Now a body-less
// leaf is kept only when every source serves the same one, so one liar in
// either position gets the name asked again, and nothing is written.
func TestALiarAmongHonestPeersCannotPlantAPhantomLeaf(t *testing.T) {
	src, root, block, part, _, void, _, partitionID := mainlessFixture(t)
	honest := api.Querier2{Querier: v3impl.NewQuerier(v3impl.QuerierParams{Database: src, Partition: partitionID})}
	liar := hideBody{Source: honest, swap: void}
	opts := Options{Mode: ModeStateOnly, Verify: anchored{root: root, block: block}, Partition: part}
	ctx := context.Background()
	phantom := url.MustParse("nobody/tokens")

	for _, c := range []struct {
		name string
		srcs []Source
	}{
		{"honest first", []Source{honest, liar}},
		{"liar first", []Source{liar, honest}},
		{"liar between two honest peers", []Source{honest, liar, honest}},
	} {
		t.Run(c.name, func(t *testing.T) {
			dst := newObservedDB(t)
			batch := dst.Begin(true)
			p, _, err := FetchFrom(ctx, c.srcs, batch, phantom, opts)
			if err == nil {
				require.NoError(t, p.Settle(root))
			}
			require.Error(t, err, "a body-less leaf one peer served and another denied was kept")
			require.True(t, stderrors.Is(err, ErrDissent), "a dissent must be retried, not dropped: %v", err)
			require.False(t, stderrors.Is(err, ErrNoLeaf), "a dissent must not drop the name")
			require.NoError(t, batch.UpdateBPT())
			require.NoError(t, batch.Commit())
			_, err = rvLeaf(dst, phantom)
			require.Error(t, err, "a phantom leaf was written")
		})
	}

	// And the honest case the rule must not break: every peer serves the
	// same body-less leaf, and it is kept.
	t.Run("every peer serves the same body-less leaf", func(t *testing.T) {
		dst := newObservedDB(t)
		batch := dst.Begin(true)
		p, _, err := FetchFrom(ctx, []Source{honest, honest, honest}, batch, void, opts)
		require.NoError(t, err)
		require.NoError(t, p.Settle(root))
		require.NoError(t, batch.UpdateBPT())
		require.NoError(t, batch.Commit())
		_, err = rvLeaf(dst, void)
		require.NoError(t, err)
	})
}

// TestUnanimousLiarsPlantAPhantomLeaf is THE LIMIT of the unanimity rule,
// pinned so that it is stated rather than discovered: when every source asked
// is a liar, the phantom leaf is kept, and nothing at the pull can tell. It is
// trust in unsigned peers for the existence of an empty leaf, a departure from
// "proven against the anchored root" recorded in DIFFERENCES.md E11; the
// whole-root match still refuses the state, and the structural closing is a
// two-way page diff that removes local-only leaves, or a key-binding hash.
func TestUnanimousLiarsPlantAPhantomLeaf(t *testing.T) {
	src, root, block, part, _, void, _, partitionID := mainlessFixture(t)
	honest := api.Querier2{Querier: v3impl.NewQuerier(v3impl.QuerierParams{Database: src, Partition: partitionID})}
	liar := hideBody{Source: honest, swap: void}
	opts := Options{Mode: ModeStateOnly, Verify: anchored{root: root, block: block}, Partition: part}
	phantom := url.MustParse("nobody/tokens")

	dst := newObservedDB(t)
	batch := dst.Begin(true)
	p, _, err := FetchFrom(context.Background(), []Source{liar, liar}, batch, phantom, opts)
	require.NoError(t, err, "if this now fails, the limit is closed: update DIFFERENCES.md E11 and flip this test")
	require.NoError(t, p.Settle(root))
	require.NoError(t, batch.UpdateBPT())
	require.NoError(t, batch.Commit())
	_, err = rvLeaf(dst, phantom)
	require.NoError(t, err)
}

// TestTheGhostFamilyChainSetIsWhatTheLeafCheckCatches: mutations of the
// served chain set for a signature-chain-only leaf. An empty chain set and
// another account's chain set must both be refused by the leaf check.
func TestTheGhostFamilyChainSetIsWhatTheLeafCheckCatches(t *testing.T) {
	src, root, block, part, ghost, _, bodied, partitionID := mainlessFixture(t)
	honest := api.Querier2{Querier: v3impl.NewQuerier(v3impl.QuerierParams{Database: src, Partition: partitionID})}
	opts := Options{Mode: ModeStateOnly, Verify: anchored{root: root, block: block}, Partition: part}
	ctx := context.Background()

	for _, c := range []struct {
		name string
		peer Source
	}{
		{"empty chain set", chainSwap{Source: honest, for_: ghost}},
		{"another account's chain set", chainSwap{Source: honest, for_: ghost, other: bodied}},
	} {
		t.Run(c.name, func(t *testing.T) {
			dst := newObservedDB(t)
			batch := dst.Begin(true)
			defer batch.Discard()
			err := Account(ctx, c.peer, batch, ghost, opts)
			require.Error(t, err, "a wrong chain set for a body-less leaf was kept")
			t.Log(err)
		})
	}
}
