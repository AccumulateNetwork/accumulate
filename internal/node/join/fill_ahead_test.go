// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/bpt"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// recordsAhead is a peer whose block ledger names the given accounts in the
// three blocks after whatever block it is asked after, and whose pages are
// slow: a validator under load.
type recordsAhead struct {
	pull.Source
	named []*url.URL
	delay time.Duration
}

func (s *recordsAhead) QueryChangedAccountStates(ctx context.Context, u *url.URL, q *api.ChangedAccountStatesQuery) (*api.AccountStatesRecord, error) {
	rec := &api.AccountStatesRecord{Through: q.After + 3, Block: q.After + 3}
	for i, a := range s.named {
		rec.Records = append(rec.Records, &api.BlockLedgerRecord{
			Index:   q.After + 1 + uint64(i%3),
			Entries: []*protocol.BlockEntry{{Account: a, Chain: "main"}},
		})
	}
	return rec, nil
}

func (s *recordsAhead) QueryBptPage(ctx context.Context, u *url.URL, q *api.BptPageQuery) (*api.BptPageRecord, error) {
	time.Sleep(s.delay)
	return s.Source.QueryBptPage(ctx, u, q)
}

// TestTheSectionsTheNextBlocksTouchAreLoadedAheadOfThem — Paul, 2026-10-10,
// run 20261009T175728Z: a follower executed 1-6 blocks a minute while the
// fill ran and 290 once every section was loaded, because each block loaded
// the sections it touched itself, one at a time, and the key-order fill,
// highest first, reached them last or never. The fill reads the records of
// the blocks after the node's and loads the sections they name first, so
// they are ready for the next block's batch while the key-order fill is
// still at the top of the tree.
func TestTheSectionsTheNextBlocksTouchAreLoadedAheadOfThem(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	peerDB := peerStore(t)
	putLedger(t, peerDB, here, 10)
	// Dense sections, as a live tree's are (~25 leaves each): a page of the
	// tree covers ~20 sections, so the key-order fill, highest first, holds
	// fillReady sections long before it reaches the lowest.
	const sections, perSection = 600, 12
	var accounts []*url.URL
	for i := 0; len(accounts) < sections*perSection; i++ {
		u := protocol.AccountUrl(fmt.Sprintf("ahead-%06d", i), "tokens")
		if database.SectionOfAccount(u) < sections {
			accounts = append(accounts, u)
			putTokens(t, peerDB, u, int64(i))
		}
	}

	// The next blocks touch accounts in the lowest sections: the last the
	// key-order fill would reach.
	sorted := append([]*url.URL(nil), accounts...)
	sort.Slice(sorted, func(i, j int) bool {
		return database.SectionOfAccount(sorted[i]) < database.SectionOfAccount(sorted[j])
	})
	var named []*url.URL
	seen := map[bpt.Section]bool{}
	for _, u := range sorted {
		if s := database.SectionOfAccount(u); !seen[s] {
			seen[s] = true
			named = append(named, u)
		}
		if len(named) == 6 {
			break
		}
	}

	served := api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peerDB, Partition: "BVN0"})}
	a := &recordsAhead{Source: served, named: named, delay: 100 * time.Millisecond}
	b := &recordsAhead{Source: served, named: named, delay: 100 * time.Millisecond}

	db := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = db.Close() })
	values, _ := genesisValues(t, 4)
	putNetwork(t, db, here, values)
	s, err := NewState(StateOptions{Partition: here, Database: db, Sources: &fixedSources{partition: here, srcs: []pull.Source{a, b}}, ExecutedBlock: 1})
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, s.Pull(ctx))
	left := s.SectionsNotLoaded()
	require.Greater(t, left, 2*fillReady, "precondition: more sections not loaded (%d) than the fill holds ready", left)
	for _, u := range named {
		require.True(t, s.secs.NotLoaded(database.SectionOfAccount(u)), "precondition: %v is not loaded", u)
	}
	require.True(t, s.machine.PromoteToActive([32]byte{1}, 10), "precondition: ACTIVE, so the fill runs")

	fillCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); s.Fill(fillCtx) }()
	defer func() { cancel(); <-done }()

	// No block runs: what is ready stays ready, and the key-order fill stops
	// at fillReady sections from the top of the tree.
	ready := func() int {
		s.secs.mu.Lock()
		defer s.secs.mu.Unlock()
		k := 0
		for _, u := range named {
			if _, ok := s.secs.ready[database.SectionOfAccount(u)]; ok {
				k++
			}
		}
		return k
	}
	deadline := time.Now().Add(15 * time.Second)
	for ready() < len(named) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	t.Logf("%d sections not loaded; %d of the %d the next blocks touch ready", left, ready(), len(named))
	require.Equal(t, len(named), ready(), "the sections the next blocks touch were not loaded ahead of them")

	// The block writes them, and their accounts read as the peer holds them.
	batch := db.Begin(true)
	s.secs.Opened(batch)
	require.NoError(t, batch.Commit())
	for _, u := range named {
		require.False(t, s.secs.NotLoaded(database.SectionOfAccount(u)), "%v was not written by the block", u)
		var i int
		_, err := fmt.Sscanf(u.Authority, "ahead-%06d", &i)
		require.NoError(t, err)
		require.Equal(t, int64(i), balanceIn(t, db, u))
	}
}
