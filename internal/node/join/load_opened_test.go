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
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
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
	served := api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{NodeState: nodestate.Always{}, Database: peerDB, Partition: "BVN0"})}
	a := &pageSource{Source: served, inflight: &inflight, most: &most}
	b := &pageSource{Source: served, inflight: &inflight, most: &most}

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

// peerLoad is one peer that counts the BptPage and AccountStates calls it has
// in flight at once and the most it had, each call taking pause.
type peerLoad struct {
	pull.Source
	pause    time.Duration
	inflight atomic.Int64
	most     atomic.Int64
}

func (s *peerLoad) enter() func() {
	n := s.inflight.Add(1)
	for {
		m := s.most.Load()
		if n <= m || s.most.CompareAndSwap(m, n) {
			break
		}
	}
	time.Sleep(s.pause)
	return func() { s.inflight.Add(-1) }
}

func (s *peerLoad) QueryBptPage(ctx context.Context, u *url.URL, q *api.BptPageQuery) (*api.BptPageRecord, error) {
	defer s.enter()()
	return s.Source.QueryBptPage(ctx, u, q)
}

func (s *peerLoad) QueryAccountStates(ctx context.Context, u *url.URL, q *api.AccountStatesQuery) (*api.AccountStatesRecord, error) {
	defer s.enter()()
	return s.Source.QueryAccountStates(ctx, u, q)
}

// TestTheOpenedSectionsLoadHoldsStatesInFlightPerPeer — #4604: the load ran
// statesInFlight workers for every peer and picked a span's peer by its
// index, and every span asked its stale leaves' states across the peers at
// once, so a peer slower than the other was asked for more than it admits; in
// a run of two peers of four slots each, 63 queries were refused. Each peer
// has at most statesInFlight of the load's queries in flight at once.
func TestTheOpenedSectionsLoadHoldsStatesInFlightPerPeer(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	peerDB := peerStore(t)
	putLedger(t, peerDB, here, 10)
	const n = 4000
	accounts := make([]*url.URL, n)
	for i := range accounts {
		accounts[i] = protocol.AccountUrl(fmt.Sprintf("spread-%04d", i), "data")
		writeAccount(t, peerDB, accounts[i], 2)
	}
	served := api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{NodeState: nodestate.Always{}, Database: peerDB, Partition: "BVN0"})}
	slow := &peerLoad{Source: served, pause: 20 * time.Millisecond}
	fast := &peerLoad{Source: served, pause: time.Millisecond}

	db := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = db.Close() })
	values, _ := genesisValues(t, 4)
	putNetwork(t, db, here, values)
	s, err := NewState(StateOptions{Partition: here, Database: db, Sources: &fixedSources{partition: here, srcs: []pull.Source{slow, fast}}, ExecutedBlock: 1})
	require.NoError(t, err)

	loadSectionOf(t, s, accounts...)

	require.Empty(t, s.sync.retry, "accounts were left owed")
	for sec, done := range s.sync.opened {
		require.True(t, done, "section %d was opened and not loaded", sec)
	}
	require.LessOrEqual(t, slow.most.Load(), int64(statesInFlight), "the slow peer had more than statesInFlight queries in flight")
	require.LessOrEqual(t, fast.most.Load(), int64(statesInFlight), "the fast peer had more than statesInFlight queries in flight")
	require.GreaterOrEqual(t, slow.most.Load()+fast.most.Load(), int64(3), "the load did not run at once")
}
