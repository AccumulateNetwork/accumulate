// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"context"
	"fmt"
	"math/big"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/bcdb"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/keyvalue/memory"
	recordpkg "gitlab.com/accumulatenetwork/accumulate/pkg/types/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// timedQuerier counts and times every call a peer is asked, by the query's
// type.
type timedQuerier struct {
	api.Querier
	mu    sync.Mutex
	calls map[string]int
	spent map[string]time.Duration
}

func newTimedQuerier(q api.Querier) *timedQuerier {
	return &timedQuerier{Querier: q, calls: map[string]int{}, spent: map[string]time.Duration{}}
}

func (c *timedQuerier) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	start := time.Now()
	r, err := c.Querier.Query(ctx, scope, query)
	c.mu.Lock()
	k := fmt.Sprintf("%T", query)
	c.calls[k]++
	c.spent[k] += time.Since(start)
	c.mu.Unlock()
	return r, err
}

func (c *timedQuerier) reset() {
	c.calls, c.spent = map[string]int{}, map[string]time.Duration{}
}

func (c *timedQuerier) total() (int, time.Duration) {
	n, d := 0, time.Duration(0)
	for k, v := range c.calls {
		n += v
		d += c.spent[k]
	}
	return n, d
}

func (c *timedQuerier) String() string {
	s := ""
	for k, v := range c.calls {
		s += fmt.Sprintf(" %s=%d(%v)", k, v, c.spent[k].Round(time.Millisecond))
	}
	return s
}

// bcdbStore opens a BlockchainDB store, the store the network runs.
func bcdbStore(t *testing.T, name string) *database.Database {
	t.Helper()
	kv, err := bcdb.Open(filepath.Join(t.TempDir(), name))
	require.NoError(t, err)
	kv.StatsEvery = 0
	require.NoError(t, kv.SetMergeLag(20))
	db := database.New(kv, nil)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// putManyTokens writes n token accounts, each with a main-chain entry, in one
// commit.
func putManyTokens(t *testing.T, db *database.Database, n int, balance int64) []*url.URL {
	t.Helper()
	b := db.Begin(true)
	defer b.Discard()
	var out []*url.URL
	for i := 0; i < n; i++ {
		u := protocol.AccountUrl(fmt.Sprintf("account%05d", i), "tokens")
		out = append(out, u)
		require.NoError(t, b.Account(u).Main().Put(&protocol.TokenAccount{Url: u, TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(balance)}))
		chain, err := b.Account(u).MainChain().Get()
		require.NoError(t, err)
		require.NoError(t, chain.AddEntry([]byte{1, 29: byte(i >> 8), 30: byte(i), 31: 1}, false))
	}
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())
	return out
}

// TestMeasureTheTopAndTheRecords — #4461, #4485, measurement. A join at a
// realistic size, both stores BlockchainDB: the top of a tree of a few
// thousand accounts, then the records of many blocks naming them, and the
// sections they are in. It logs the calls and where the time goes: in the
// peer's answers, or in the joining node's writes.
func TestMeasureTheTopAndTheRecords(t *testing.T) {
	const U, K = 3000, 1000
	here := protocol.PartitionUrl("BVN0")
	peerDB := bcdbStore(t, "peer")
	putLedger(t, peerDB, here, 10)
	accounts := putManyTokens(t, peerDB, U, 5)
	timed := newTimedQuerier(apiimpl.NewQuerier(apiimpl.QuerierParams{NodeState: nodestate.Always{}, Database: peerDB, Partition: "BVN0"}))

	db := bcdbStore(t, "joiner")
	values, _ := genesisValues(t, 4)
	putNetwork(t, db, here, values)
	s, err := NewState(StateOptions{Partition: here, Database: db, Sources: &onePeer{partition: here, querier: timed, block: 10}, ExecutedBlock: 1})
	require.NoError(t, err)
	ctx := context.Background()

	for round := 0; round == 0 || !s.sync.covered(); round++ {
		timed.reset()
		pages := 0
		if s.sync != nil {
			pages = s.sync.sectionPages
		}
		start := time.Now()
		require.NoError(t, s.Pull(ctx))
		n, spent := timed.total()
		t.Logf("top round %d: %d section pages, %d calls, %v total, %v in the peer's answers:%v",
			round, s.sync.sectionPages-pages, n, time.Since(start).Round(time.Millisecond), spent.Round(time.Millisecond), timed)
		require.Less(t, round, 100)
	}

	// K blocks, each naming three accounts.
	b := peerDB.Begin(true)
	ledger := b.Account(here.JoinPath(protocol.Ledger))
	for n := uint64(11); n <= 10+K; n++ {
		var entries []*protocol.BlockEntry
		for j := 0; j < 3; j++ {
			u := accounts[(int(n)*3+j)%U]
			require.NoError(t, b.Account(u).Main().Put(&protocol.TokenAccount{Url: u, TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(int64(n))}))
			entries = append(entries, &protocol.BlockEntry{Account: u, Chain: "main", Index: n})
		}
		require.NoError(t, ledger.BlockLedger(n).Put(&database.BlockLedger{Index: n, Entries: entries}))
	}
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())
	putLedger(t, peerDB, here, 10+K)
	s.sources.(*onePeer).block = 10 + K

	timed.reset()
	start := time.Now()
	require.NoError(t, s.Pull(ctx))
	n, spent := timed.total()
	t.Logf("records round: %d blocks naming %d accounts, through %d: %d calls, %v total, %v in the peer's answers:%v",
		K, 3*K, s.sync.last, n, time.Since(start).Round(time.Millisecond), spent.Round(time.Millisecond), timed)
}

// committingStore counts the commits made to a store.
type committingStore struct {
	keyvalue.Beginner
	commits *int
}

func (s committingStore) Begin(prefix *recordpkg.Key, writable bool) keyvalue.ChangeSet {
	return countedChangeSet{s.Beginner.Begin(prefix, writable), s.commits}
}

type countedChangeSet struct {
	keyvalue.ChangeSet
	commits *int
}

func (c countedChangeSet) Commit() error {
	*c.commits++
	return c.ChangeSet.Commit()
}

// catchUpCost is what a join's walk, and then its records round, cost when the
// peer holds u accounts and k blocks after the walk's start that name them:
// the calls it asks the peer, and the commits it makes to its own store.
func catchUpCost(t *testing.T, u, k int) (walkCalls, walkCommits, recordCalls, recordCommits int) {
	t.Helper()
	here := protocol.PartitionUrl("BVN0")
	peerDB := peerStore(t)
	putLedger(t, peerDB, here, 10)
	accounts := putManyTokens(t, peerDB, u, 5)
	timed := newTimedQuerier(apiimpl.NewQuerier(apiimpl.QuerierParams{NodeState: nodestate.Always{}, Database: peerDB, Partition: "BVN0"}))

	commits := 0
	db := database.New(committingStore{memory.New(nil), &commits}, nil)
	t.Cleanup(func() { _ = db.Close() })
	values, _ := genesisValues(t, 4)
	putNetwork(t, db, here, values)
	s, err := NewState(StateOptions{Partition: here, Database: db, Sources: &onePeer{partition: here, querier: timed, block: 10}, ExecutedBlock: 1})
	require.NoError(t, err)
	ctx := context.Background()

	commits = 0
	for round := 0; round == 0 || !s.sync.covered(); round++ {
		require.NoError(t, s.Pull(ctx))
		require.Less(t, round, 100)
	}
	walkCalls, _ = timed.total()
	walkCommits = commits

	b := peerDB.Begin(true)
	ledger := b.Account(here.JoinPath(protocol.Ledger))
	for n := uint64(11); n <= uint64(10+k); n++ {
		var entries []*protocol.BlockEntry
		for j := 0; j < 3; j++ {
			a := accounts[(int(n)*3+j)%u]
			require.NoError(t, b.Account(a).Main().Put(&protocol.TokenAccount{Url: a, TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(int64(n))}))
			entries = append(entries, &protocol.BlockEntry{Account: a, Chain: "main", Index: n})
		}
		require.NoError(t, ledger.BlockLedger(n).Put(&database.BlockLedger{Index: n, Entries: entries}))
	}
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())
	putLedger(t, peerDB, here, uint64(10+k))
	s.sources.(*onePeer).block = uint64(10 + k)

	timed.reset()
	commits = 0
	require.NoError(t, s.Pull(ctx))
	require.Equal(t, uint64(10+k), s.sync.last, "precondition: the round processed the records through block %d", 10+k)
	recordCalls, _ = timed.total()
	recordCommits = commits
	t.Logf("%d accounts, %d blocks: the walk asked %d calls and made %d commits; the records round asked %d calls:%v and made %d commits",
		u, k, walkCalls, walkCommits, recordCalls, timed, recordCommits)
	return walkCalls, walkCommits, recordCalls, recordCommits
}

// TestACatchUpCostsCallsAndCommitsPerAnswerNotPerAccountOrRecord — #4461. At
// ~6,000 blocks of history a BVN's join took 9 minutes: 24 s a walk page, 2.5
// minutes of records, nearly all of it the joining node committing each
// account it took in a batch of its own. The records after the node's index I
// are one answer up to the peer's cap of blocks per answer, and each answer's
// accounts are written in one commit. Ten times the accounts and the records cost the
// same calls, and commits that grow with pages, not accounts.
func TestACatchUpCostsCallsAndCommitsPerAnswerNotPerAccountOrRecord(t *testing.T) {
	// Records within the peer's cap of blocks per answer (1,024): the
	// catch-up after a fast walk is a few blocks (#4461).
	wc1, wm1, rc1, rm1 := catchUpCost(t, 200, 100)
	wc2, wm2, rc2, rm2 := catchUpCost(t, 2000, 1000)

	// The walk asks a BPT page and an AccountStates answer per 256 accounts:
	// ten times the accounts is ten times the pages, and no more.
	require.LessOrEqual(t, wc2, 10*wc1,
		"the walk's calls grew faster than its pages: %d for 200 accounts, %d for 2,000", wc1, wc2)
	require.LessOrEqual(t, wm2, 10*wm1,
		"the walk's commits grew faster than its pages: %d for 200 accounts, %d for 2,000", wm1, wm2)
	require.Less(t, wm2, 2000/10,
		"the walk made %d commits for 2,000 accounts: its commits grow with the accounts, not the pages", wm2)

	// The records: one answer, whatever the blocks and accounts; a call per
	// statesChunk accounts they name, asked at once (#4552); and a page per
	// 256 leaves of the sections their accounts are in (executor spec,
	// "Sync", §3a): calls that grow with chunks and pages, not blocks, and
	// never per account. The opened sections are loaded in spans sized by
	// the round's first page (#4549); a span whose sections hold more leaves
	// than that page's takes a second page, so at most twice the pages.
	require.LessOrEqual(t, rc2, rc1+2000/statesChunk+2*(2000/256)+1,
		"the records round asked %d calls for 100 blocks naming 200 accounts and %d for 1,000 blocks naming 2,000", rc1, rc2)
	require.Equal(t, rm1, rm2,
		"the records round made %d commits for 200 accounts and %d for 2,000", rm1, rm2)
}
