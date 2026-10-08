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

// TestTheFillLoadsSectionsInSpans — #4554 fix 2, run 20261008T180957Z: the
// fill loaded one section per unit of work, a page and a states call for
// ~23 leaves, and at 3.5 sections a second under the follower's own gate it
// ran thirteen minutes for 3,500 sections -- and the overlap settled every
// block for as long. The fill pages 256 leaves from the highest section not
// loaded, as the pull's rounds do (#4549), takes every section the page
// covers whole that hashes to its marker, asks one states call for all of
// their leaves, and hands each section to the next block as before: a page
// and a states call per ~eleven sections.
func TestTheFillLoadsSectionsInSpans(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	peerDB := peerStore(t)
	putLedger(t, peerDB, here, 10)
	const n = 2000
	accounts := make([]*url.URL, n)
	for i := range accounts {
		accounts[i] = protocol.AccountUrl(fmt.Sprintf("fill-%04d", i), "tokens")
		putTokens(t, peerDB, accounts[i], int64(i))
	}
	var pagesInFlight, pagesMost, statesInFlightN, statesMost atomic.Int64
	served := api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peerDB, Partition: "BVN0"})}
	stA := &statesSource{Source: served, inflight: &statesInFlightN, most: &statesMost}
	stB := &statesSource{Source: served, inflight: &statesInFlightN, most: &statesMost}
	a := &pageSource{Source: stA, inflight: &pagesInFlight, most: &pagesMost}
	b := &pageSource{Source: stB, inflight: &pagesInFlight, most: &pagesMost}

	db := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = db.Close() })
	values, _ := genesisValues(t, 4)
	putNetwork(t, db, here, values)
	s, err := NewState(StateOptions{Partition: here, Database: db, Sources: &fixedSources{partition: here, srcs: []pull.Source{a, b}}, ExecutedBlock: 1})
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, s.Pull(ctx)) // the top of the tree: every section the node does not hold is not loaded
	left := s.SectionsNotLoaded()
	require.Greater(t, left, 40, "precondition: the peer's %d accounts span few sections (%d)", n, left)
	require.True(t, s.machine.PromoteToActive([32]byte{1}, 10), "precondition: ACTIVE, so the fill runs")

	a.calls.Store(0)
	b.calls.Store(0)
	stA.calls.Store(0)
	stB.calls.Store(0)
	fillCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); s.Fill(fillCtx) }()

	// The blocks: each writes what the fill has made ready.
	deadline := time.Now().Add(90 * time.Second)
	for s.SectionsNotLoaded() > 0 && time.Now().Before(deadline) {
		batch := db.Begin(true)
		s.secs.Opened(batch)
		require.NoError(t, batch.Commit())
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	require.Zero(t, s.SectionsNotLoaded(), "the fill did not load every section")

	batch := db.Begin(false)
	defer batch.Discard()
	for i, u := range accounts {
		require.Equal(t, int64(i), balanceIn(t, db, u), "%v was not loaded", u)
	}
	pages := a.calls.Load() + b.calls.Load()
	states := stA.calls.Load() + stB.calls.Load()
	t.Logf("%d sections, %d accounts: %d pages, %d states calls, %d pages and %d states calls in flight at most", left, n, pages, states, pagesMost.Load(), statesMost.Load())
	require.LessOrEqual(t, pages, int64(left/4+4), "the fill paged the tree a section at a time: %d pages for %d sections", pages, left)
	require.LessOrEqual(t, states, int64(n/64+left/4+4), "the fill asked states a section at a time: %d calls for %d accounts", states, n)
	require.Positive(t, a.calls.Load())
	require.Positive(t, b.calls.Load(), "the second peer was never asked")
	require.GreaterOrEqual(t, pagesMost.Load()+statesMost.Load(), int64(2), "the fill loaded one span at a time")
}
