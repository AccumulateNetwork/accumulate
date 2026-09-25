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
	noValidators
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
// holds no leaf for it -- is dropped, not owed: what is owed is asked again
// every round for the life of the process, and on the live run
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
		p := newSyncing()

		require.Equal(t, dropped, s.pullOne(context.Background(), p, nobody))
		require.Empty(t, p.retry,
			"a name every source says it holds no leaf for is dropped, not asked again every round")
	})

	t.Run("a source failed to answer", func(t *testing.T) {
		s := quietState(t, here, &fixedSources{partition: here, srcs: []pull.Source{peer, silent{peer}}})
		p := newSyncing()

		require.Equal(t, owed, s.pullOne(context.Background(), p, nobody))
		require.Len(t, p.retry, 1,
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
