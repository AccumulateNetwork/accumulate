// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// pageSource is one peer that counts the BptPage calls asked of it and how
// many are in flight at once across every pageSource sharing the gauge, each
// call taking long enough for calls to overlap.
type pageSource struct {
	pull.Source
	calls    atomic.Int64
	inflight *atomic.Int64
	most     *atomic.Int64
}

func (s *pageSource) QueryBptPage(ctx context.Context, u *url.URL, q *api.BptPageQuery) (*api.BptPageRecord, error) {
	s.calls.Add(1)
	n := s.inflight.Add(1)
	defer s.inflight.Add(-1)
	for {
		m := s.most.Load()
		if n <= m || s.most.CompareAndSwap(m, n) {
			break
		}
	}
	time.Sleep(2 * time.Millisecond)
	return s.Source.QueryBptPage(ctx, u, q)
}

// TestTheSectionsARoundOpenedAreLoadedInParallel — #4549, run
// 20261007T141334Z: a round loaded the sections it opened one page at a
// time, 1.4 queries in flight of the 16 four validators admit, about 2.6 s
// and 100 accounts a page, so a BVN of 96k leaves was 40 minutes of pulling
// against a 30-minute deadline. The opened sections are cut into spans and
// the spans are loaded at once, statesInFlight against each peer, every
// span's answers kept on the round's goroutine.
func TestTheSectionsARoundOpenedAreLoadedInParallel(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	peerDB := peerStore(t)
	putLedger(t, peerDB, here, 10)
	// Eight pages of leaves: the first is the probe that sizes the spans,
	// the rest are fetched at once.
	const n = 2000
	accounts := make([]*url.URL, n)
	for i := range accounts {
		accounts[i] = protocol.AccountUrl(fmt.Sprintf("spread-%03d", i), "data")
		writeAccount(t, peerDB, accounts[i], 2)
	}
	var inflight, most atomic.Int64
	// Each peer is one a joining node holds: its own query gate, as another
	// node has, behind the node's gate for that peer (QueryPeers.clientFor).
	// Sharing the process's gate, the round's eight loads met four slots, the
	// slow ones were refused by both peers, and their accounts were owed
	// (#4602).
	served := func() pull.Source {
		q := apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peerDB, Partition: "BVN0", QueryGate: apiimpl.NewQueryGate()})
		return api.Querier2{Querier: pull.Gated(q, pull.NewGate())}
	}
	a := &pageSource{Source: served(), inflight: &inflight, most: &most}
	b := &pageSource{Source: served(), inflight: &inflight, most: &most}

	db := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = db.Close() })
	values, _ := genesisValues(t, 4)
	putNetwork(t, db, here, values)
	s, err := NewState(StateOptions{Partition: here, Database: db, Sources: &fixedSources{partition: here, srcs: []pull.Source{a, b}}, ExecutedBlock: 1})
	require.NoError(t, err)

	loadSectionOf(t, s, accounts...)

	require.Empty(t, s.sync.retry, "accounts were left owed")
	batch := db.Begin(false)
	defer batch.Discard()
	for _, u := range accounts {
		_, err := batch.Account(u).Main().Get()
		require.NoError(t, err, "%v was not kept", u)
	}
	for sec, done := range s.sync.opened {
		require.True(t, done, "section %d was opened and not loaded", sec)
	}
	require.Positive(t, a.calls.Load())
	require.Positive(t, b.calls.Load(), "the second peer was never asked for a page")
	require.GreaterOrEqual(t, most.Load(), int64(2), "the sections were paged one page at a time")
}
