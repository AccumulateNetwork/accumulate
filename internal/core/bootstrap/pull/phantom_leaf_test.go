// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

// The phantom-leaf question of #4397 and #4406, closed by #4437: the state
// tree holds a leaf only for an account with main state (executor spec,
// invariant 13), so a leaf with no body does not exist, and a peer that
// serves one is refused as a failure of that source.

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
	apierrors "gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

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

// notReady is a peer that is itself joining: its querier refuses every read
// (querier.servingFor).
type notReady struct{ Source }

func (notReady) QueryAccount(context.Context, *url.URL, *api.DefaultQuery) (*api.AccountRecord, error) {
	return nil, apierrors.NotReady.With("the peer is joining and cannot answer for state it has not executed")
}

// swapAll answers every query for one name with another account's answers:
// its body or none, its receipt, its chains.
type swapAll struct {
	Source
	for_, other *url.URL
}

func (s swapAll) u(u *url.URL) *url.URL {
	if u.Equal(s.for_) {
		return s.other
	}
	return u
}

func (s swapAll) QueryAccount(ctx context.Context, u *url.URL, q *api.DefaultQuery) (*api.AccountRecord, error) {
	return s.Source.QueryAccount(ctx, s.u(u), q)
}

func (s swapAll) QueryAccountChains(ctx context.Context, u *url.URL, q *api.ChainQuery) (*api.RecordRange[*api.ChainRecord], error) {
	return s.Source.QueryAccountChains(ctx, s.u(u), q)
}

func (s swapAll) QueryChainEntries(ctx context.Context, u *url.URL, q *api.ChainQuery) (*api.RecordRange[*api.ChainEntryRecord[api.Record]], error) {
	return s.Source.QueryChainEntries(ctx, s.u(u), q)
}

// bodyless answers every account query with no body beside the receipt the
// honest peer gives for another account that does exist: what a peer serving
// "a leaf with no body" looked like, and passed the leaf check with, before
// #4437.
type bodyless struct {
	Source
	receiptOf *url.URL
}

func (b bodyless) QueryAccount(ctx context.Context, u *url.URL, q *api.DefaultQuery) (*api.AccountRecord, error) {
	rec, err := b.Source.QueryAccount(ctx, b.receiptOf, q)
	if err != nil {
		return nil, err
	}
	rec.Account = nil
	return rec, nil
}

// TestTheWritesThatMadeEmptyLeavesMakeNone: dirtying a missing account's
// bookkeeping (void) and recording a signature chain on a principal with no
// main state (ghost) are the two writes that used to give an account with no
// state a leaf. Neither does (#4437).
func TestTheWritesThatMadeEmptyLeavesMakeNone(t *testing.T) {
	src, _, _, _, ghost, void, bodied, _ := mainlessFixture(t)
	for _, u := range []*url.URL{ghost, void} {
		_, err := rvLeaf(src, u)
		require.Error(t, err, "%v has no main state and got a state-tree leaf", u)
	}
	_, err := rvLeaf(src, bodied)
	require.NoError(t, err, "precondition: an account with main state has a leaf")
}

// TestAPeerServingALeafWithNoBodyIsRefused: such a leaf does not exist, so an
// answer carrying one is a lie. It is refused as that source's failure — never
// kept, never a reason to drop the name — and the next source is asked.
func TestAPeerServingALeafWithNoBodyIsRefused(t *testing.T) {
	src, root, block, part, _, void, bodied, partitionID := mainlessFixture(t)
	honest := api.Querier2{Querier: v3impl.NewQuerier(v3impl.QuerierParams{Database: src, Partition: partitionID})}
	liar := bodyless{Source: honest, receiptOf: bodied}
	opts := Options{Mode: ModeStateOnly, Verify: anchored{root: root, block: block}, Partition: part}

	fetch := func(t *testing.T, srcs []Source, u *url.URL) (int, error) {
		t.Helper()
		dst := newObservedDB(t)
		batch := dst.Begin(true)
		p, i, err := FetchFrom(context.Background(), srcs, batch, u, opts)
		if err == nil {
			err = p.Settle(root)
		}
		require.NoError(t, batch.UpdateBPT())
		require.NoError(t, batch.Commit())
		if err != nil {
			_, lerr := rvLeaf(dst, u)
			require.Error(t, lerr, "refused, but a leaf was written for %v", u)
		}
		return i, err
	}

	t.Run("a body-less answer for an account that exists", func(t *testing.T) {
		i, err := fetch(t, []Source{liar, honest}, bodied)
		require.NoError(t, err, "the honest peer's body was not taken after the liar's answer")
		require.Equal(t, 1, i, "the body-less answer was kept")
	})

	t.Run("a body-less answer alone is refused", func(t *testing.T) {
		_, err := fetch(t, []Source{liar}, bodied)
		require.Error(t, err, "a leaf with no body was kept")
		require.False(t, stderrors.Is(err, ErrNoLeaf), "a lie is a failure to ask again, not a reason to drop the name")
	})

	t.Run("a body-less answer does not drop a name the others do not hold", func(t *testing.T) {
		_, err := fetch(t, []Source{honest, liar}, void)
		require.Error(t, err)
		require.False(t, stderrors.Is(err, ErrNoLeaf), "one lying source must leave the name to be asked again")
	})

	t.Run("every source answering NotFound drops the name", func(t *testing.T) {
		_, err := fetch(t, []Source{honest, honest}, void)
		require.True(t, stderrors.Is(err, ErrNoLeaf), "want ErrNoLeaf, got %v", err)
	})

	t.Run("a joining peer neither blocks nor counts", func(t *testing.T) {
		i, err := fetch(t, []Source{notReady{honest}, honest}, bodied)
		require.NoError(t, err)
		require.Equal(t, 1, i)
	})
}

// TestABodyServedUnderAnotherNameIsRefused (#4408, review R4). A liar serves
// alice/data's body, receipt and chains under nobody/tokens. The pulled state
// carries alice/data's URL, so it hashes to alice/data's true leaf and passes
// the leaf check against the anchored root; the store's own URL check then
// fired at commit, where it is a panic, and took the joining node down. A body
// that does not name the account asked for is refused at the pull, and the
// next source is asked.
func TestABodyServedUnderAnotherNameIsRefused(t *testing.T) {
	src, root, block, part, _, _, bodied, partitionID := mainlessFixture(t)
	honest := api.Querier2{Querier: v3impl.NewQuerier(v3impl.QuerierParams{Database: src, Partition: partitionID})}
	opts := Options{Mode: ModeStateOnly, Verify: anchored{root: root, block: block}, Partition: part}
	phantom := url.MustParse("nobody/tokens")
	liar := swapAll{Source: honest, for_: phantom, other: bodied}

	for _, c := range []struct {
		name string
		srcs []Source
	}{
		{"the liar alone", []Source{liar}},
		{"the liar, then an honest peer", []Source{liar, honest}},
	} {
		t.Run(c.name, func(t *testing.T) {
			dst := newObservedDB(t)
			batch := dst.Begin(true)
			p, _, err := FetchFrom(context.Background(), c.srcs, batch, phantom, opts)
			if err == nil {
				err = p.Settle(root)
			}
			require.Error(t, err, "a body served under a name it does not carry was kept")
			require.NoError(t, batch.UpdateBPT())
			require.NoError(t, batch.Commit())
			_, lerr := rvLeaf(dst, phantom)
			require.Error(t, lerr, "a leaf was written under the phantom name")
		})
	}
}
