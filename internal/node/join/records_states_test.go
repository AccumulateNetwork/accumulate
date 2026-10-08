// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// asOfRecorder is a peer that records the block every AccountStates call
// asks for.
type asOfRecorder struct {
	pull.Source
	mu   *sync.Mutex
	asOf map[uint64]int
}

func (s *asOfRecorder) QueryAccountStates(ctx context.Context, u *url.URL, q *api.AccountStatesQuery) (*api.AccountStatesRecord, error) {
	s.mu.Lock()
	s.asOf[q.ForHeight]++
	s.mu.Unlock()
	return s.Source.QueryAccountStates(ctx, u, q)
}

// TestTheRecordsStatesAreAskedAtOnceAsOfT — #4552, run 20261007T211137Z: a
// ChangedAccountStates answer carried the states of every account its
// records named, a page at a time, the validator reading each at ~30 ms, so
// a round's records took 10–12 s for ~17 blocks against a head moving a
// block a second, and a round was complete only when the answer fit one
// page (one view): the eleventh follower never matched. The answer carries
// the records; the states of the accounts they name are asked as the
// sections' are -- statesChunk a call, statesInFlight against each peer, at
// once -- as of T, the records' end, so the states raise no block the
// records must catch up to.
func TestTheRecordsStatesAreAskedAtOnceAsOfT(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	peerDB := peerStore(t)
	putLedger(t, peerDB, here, 10)
	const U, K = 600, 50
	accounts := make([]*url.URL, U)
	for i := range accounts {
		accounts[i] = protocol.AccountUrl(fmt.Sprintf("named-%03d", i), "tokens")
		putTokens(t, peerDB, accounts[i], 5)
	}
	var inflight, most atomic.Int64
	var mu sync.Mutex
	asOf := map[uint64]int{}
	served := api.Querier2{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peerDB, Partition: "BVN0"})}
	recorded := &asOfRecorder{Source: served, mu: &mu, asOf: asOf}
	a := &statesSource{Source: recorded, inflight: &inflight, most: &most}
	b := &statesSource{Source: recorded, inflight: &inflight, most: &most}

	db := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = db.Close() })
	values, _ := genesisValues(t, 4)
	putNetwork(t, db, here, values)
	s, err := NewState(StateOptions{Partition: here, Database: db, Sources: &fixedSources{partition: here, srcs: []pull.Source{a, b}}, ExecutedBlock: 1})
	require.NoError(t, err)
	loadSectionOf(t, s, accounts...)
	for _, u := range accounts {
		require.Equal(t, int64(5), balanceIn(t, db, u), "precondition: %v was loaded with its section", u)
	}

	// Blocks 11 through 10+K change every account, U/K a block.
	for i, u := range accounts {
		putTokens(t, peerDB, u, int64(100+i))
	}
	batch := peerDB.Begin(true)
	ledger := batch.Account(here.JoinPath(protocol.Ledger))
	for n := uint64(11); n <= 10+K; n++ {
		var entries []*protocol.BlockEntry
		for j := 0; j < U/K; j++ {
			entries = append(entries, &protocol.BlockEntry{Account: accounts[(int(n)-11)*(U/K)+j], Chain: "main", Index: n})
		}
		require.NoError(t, ledger.BlockLedger(n).Put(&database.BlockLedger{Index: n, Entries: entries}))
	}
	require.NoError(t, batch.Commit())
	putLedger(t, peerDB, here, 10+K)

	a.calls.Store(0)
	b.calls.Store(0)
	most.Store(0)
	mu.Lock()
	clear(asOf)
	mu.Unlock()
	require.NoError(t, s.Pull(context.Background()))

	require.Equal(t, uint64(10+K), s.sync.last, "the round did not process the records through block %d", 10+K)
	require.Empty(t, s.sync.retry, "accounts the records name were left owed")
	for i, u := range accounts {
		require.Equal(t, int64(100+i), balanceIn(t, db, u), "%v was not brought to the state the records name", u)
	}
	require.True(t, s.sync.ready, "the round was not complete with every state as of T")
	require.Positive(t, a.calls.Load(), "the records' states were not asked by AccountStates")
	require.Positive(t, b.calls.Load(), "the second peer was never asked for the records' states")
	require.GreaterOrEqual(t, most.Load(), int64(2), "the records' states were asked one call at a time")
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, map[uint64]int{10 + K: int(a.calls.Load() + b.calls.Load())}, asOf, "the records' states were not all asked as of T")
}

// omittingQuerier leaves one account out of every AccountStates answer while
// omit is set.
type omittingQuerier struct {
	api.Querier
	victim *url.URL
	omit   bool
}

func (q *omittingQuerier) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	r, err := q.Querier.Query(ctx, scope, query)
	if _, ok := query.(*api.AccountStatesQuery); !ok || err != nil || !q.omit {
		return r, err
	}
	rec := r.(*api.AccountStatesRecord)
	var kept []*api.AccountState
	for _, st := range rec.States {
		if !st.Url.Equal(q.victim) {
			kept = append(kept, st)
		}
	}
	rec.States = kept
	return rec, nil
}

// TestAnAccountARecordNamesThatNoPeerServesIsOwed — #4552. The records
// answer names; the states are asked by name. An account a record names
// that no peer's answer serves is owed to the next round, like any account
// a batch did not answer: the rest of the records' accounts are taken, L
// moves to T, and the state is not complete until it is taken.
func TestAnAccountARecordNamesThatNoPeerServesIsOwed(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	alice := protocol.AccountUrl("alice", "tokens")
	bob := protocol.AccountUrl("bob", "tokens")
	peerDB := peerStore(t)
	putLedger(t, peerDB, here, 10)
	putTokens(t, peerDB, alice, 5)
	putTokens(t, peerDB, bob, 5)
	q := &omittingQuerier{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peerDB, Partition: "BVN0"}), victim: bob}

	db := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = db.Close() })
	values, _ := genesisValues(t, 4)
	putNetwork(t, db, here, values)
	s, err := NewState(StateOptions{Partition: here, Database: db, Sources: &onePeer{partition: here, querier: q, block: 10}, ExecutedBlock: 1})
	require.NoError(t, err)
	ctx := context.Background()
	loadSectionOf(t, s, alice, bob)

	// Block 11 changes alice and bob.
	putTokens(t, peerDB, alice, 11)
	putTokens(t, peerDB, bob, 12)
	b := peerDB.Begin(true)
	require.NoError(t, b.Account(here.JoinPath(protocol.Ledger)).BlockLedger(11).Put(&database.BlockLedger{Index: 11, Entries: []*protocol.BlockEntry{
		{Account: alice, Chain: "main", Index: 1},
		{Account: bob, Chain: "main", Index: 1},
	}}))
	require.NoError(t, b.Commit())
	putLedger(t, peerDB, here, 11)
	s.sources.(*onePeer).block = 11

	q.omit = true
	require.NoError(t, s.Pull(ctx))
	require.Equal(t, uint64(11), s.sync.last, "the records were not processed because one account was not served")
	require.Equal(t, int64(11), balanceIn(t, db, alice), "alice, served, was not taken")
	require.Equal(t, int64(5), balanceIn(t, db, bob), "bob was written from an answer that did not serve him")
	require.Contains(t, s.sync.retry, accountKey(bob), "bob, named and not served, is not owed")
	require.False(t, s.sync.ready, "the state was complete with an account owed")

	q.omit = false
	require.NoError(t, s.Pull(ctx))
	require.Equal(t, int64(12), balanceIn(t, db, bob), "bob was not taken once served")
	require.Empty(t, s.sync.retry)
}
