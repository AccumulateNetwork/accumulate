// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/stretchr/testify/require"
	v3impl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// fixedSources hands every account the same sources, all of this partition.
type fixedSources struct {
	partition *url.URL
	srcs      []pull.Source
}

func (f *fixedSources) For(context.Context, *url.URL) ([]pull.Source, *url.URL, error) {
	return f.srcs, f.partition, nil
}

func (f *fixedSources) Querier(*url.URL) api.Querier { return nil }

// silent is a source that does not answer: a peer restarting, or unreachable.
type silent struct{ pull.Source }

func (silent) QueryAccount(context.Context, *url.URL, *api.DefaultQuery) (*api.AccountRecord, error) {
	return nil, errors.NoPeer.With("the peer did not answer")
}

// TestFetch_DropsANameNoPeerHoldsALeafFor drives the production fetch loop
// against the querier that ships (#4397). A name every source answers
// NotFound for -- which, asked with a receipt, a peer says only when its tree
// holds no leaf for it -- is dropped, not refused: a refusal is asked again at
// the front of every pass for the life of the process, and on the live run
// that was 76,752 lines in seven minutes. A name some source failed to answer
// is refused and asked again, because that source may hold it.
func TestFetch_DropsANameNoPeerHoldsALeafFor(t *testing.T) {
	const partitionID = "NoLeaf"
	here := protocol.PartitionUrl(partitionID)
	nobody := protocol.AccountUrl("nobody", "tokens")

	db := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = db.Close() })
	b := db.Begin(true)
	require.NoError(t, b.Account(here.JoinPath(protocol.Ledger)).Main().Put(
		&protocol.SystemLedger{Url: here.JoinPath(protocol.Ledger), Index: 3}))
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())

	peer := api.Querier2{Querier: v3impl.NewQuerier(v3impl.QuerierParams{Database: db, Partition: partitionID})}

	t.Run("every source answers NotFound", func(t *testing.T) {
		s := quietState(t, here, &fixedSources{partition: here, srcs: []pull.Source{peer, peer}})
		s.spine = true // the spine is not what this is about
		s.fetchPass(context.Background(), []*url.URL{nobody})

		require.Nil(t, s.pass)
		require.Empty(t, s.refused,
			"a name every source says it holds no leaf for is dropped, not asked again every pass")
	})

	t.Run("a source failed to answer", func(t *testing.T) {
		s := quietState(t, here, &fixedSources{partition: here, srcs: []pull.Source{peer, silent{peer}}})
		s.spine = true
		s.fetchPass(context.Background(), []*url.URL{nobody})

		require.Nil(t, s.pass)
		require.Len(t, s.refused, 1,
			"a name a source did not answer for may be held by that source, so it is asked again")
	})
}

// joiningPeer is a peer that is itself BOOTING: its querier refuses every
// read as NotReady (querier.servingFor), exactly as a second node joining the
// same partition does.
type joiningPeer struct{ pull.Source }

func (joiningPeer) QueryAccount(context.Context, *url.URL, *api.DefaultQuery) (*api.AccountRecord, error) {
	return nil, errors.NotReady.With("the peer is joining and cannot answer for state it has not executed")
}

// TestAJoiningPeerDoesNotBlockALeafWithNoBody drives the production fetch
// loop (#4397 review R1). A peer that is itself joining answers every read
// NotReady; counted as dissent, it kept a node from keeping ANY body-less
// leaf while it joined -- and it could not either, so two nodes joining one
// partition under load, where failed work leaves such a leaf in every block,
// blocked each other for ever. A source that does not answer does not vote.
func TestAJoiningPeerDoesNotBlockALeafWithNoBody(t *testing.T) {
	const partitionID = "TwoJoiners"
	here := protocol.PartitionUrl(partitionID)
	void := url.MustParse("void-9aac09e22e861b50/tokens")
	ghost := url.MustParse("alice/ghostdata1")

	db := database.OpenInMemory(nil)
	db.SetObserver(database.NewDatabaseObserver())
	t.Cleanup(func() { _ = db.Close() })
	b := db.Begin(true)
	require.NoError(t, b.Account(here.JoinPath(protocol.Ledger)).Main().Put(
		&protocol.SystemLedger{Url: here.JoinPath(protocol.Ledger), Index: 3}))
	sigHash := sha256.Sum256([]byte("a signature on a principal that does not exist"))
	require.NoError(t, b.Account(ghost).SignatureChain().Inner().AddEntry(sigHash[:], false))
	require.NoError(t, b.Account(void).MarkDirty())
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())
	b = db.Begin(true)
	idx, err := b.Account(here.JoinPath(protocol.Ledger)).RootChain().Index().Get()
	require.NoError(t, err)
	entry, err := (&protocol.IndexEntry{BlockIndex: 3}).MarshalBinary()
	require.NoError(t, err)
	require.NoError(t, idx.AddEntry(entry, false))
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())

	peer := api.Querier2{Querier: v3impl.NewQuerier(v3impl.QuerierParams{Database: db, Partition: partitionID})}

	t.Run("three active peers", func(t *testing.T) {
		s := quietState(t, here, &fixedSources{partition: here, srcs: []pull.Source{peer, peer, peer}})
		s.spine = true
		s.fetchPass(context.Background(), []*url.URL{void, ghost})
		require.Empty(t, s.refused)
		require.NotNil(t, s.pass)
		require.Len(t, s.pass.accounts, 2)
		s.pass.batch.Discard()
	})

	t.Run("two active peers and one that is joining", func(t *testing.T) {
		s := quietState(t, here, &fixedSources{partition: here, srcs: []pull.Source{peer, peer, joiningPeer{peer}}})
		s.spine = true
		s.fetchPass(context.Background(), []*url.URL{void, ghost})
		if s.pass != nil {
			defer s.pass.batch.Discard()
		}
		require.Empty(t, s.refused,
			"a real body-less leaf two active peers serve alike was refused because a third peer is joining")
		require.NotNil(t, s.pass)
		require.Len(t, s.pass.accounts, 2)
	})
}
