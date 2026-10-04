// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package join

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	apiimpl "gitlab.com/accumulatenetwork/accumulate/internal/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// countingQuerier counts every call a peer is asked, by the query's type:
// every read the join makes of a peer is one Query, whatever the Querier2
// method that made it.
type countingQuerier struct {
	api.Querier
	calls map[string]int
}

func (c *countingQuerier) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	c.calls[fmt.Sprintf("%T", query)]++
	return c.Querier.Query(ctx, scope, query)
}

func (c *countingQuerier) total() int {
	n := 0
	for _, v := range c.calls {
		n += v
	}
	return n
}

func (c *countingQuerier) String() string {
	var keys []string
	for k := range c.calls {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	s := ""
	for _, k := range keys {
		s += fmt.Sprintf(" %s=%d", k, c.calls[k])
	}
	return s
}

// TestARoundCostsCallsPerPageNotPerBlockOrAccount — #4453, executor spec,
// "Sync", §3: "A pull costs calls, not accounts". A round that processes the
// records of K blocks naming U accounts asked a call per block and about
// eight per account, K + 8U; ChangedAccountStates answers every account the
// records name in pages, so the round costs its pages and a constant.
//
// The peer is the production querier over a store holding the records. The
// first round walks the tree at S; K blocks then change every one of U
// accounts, and the second round's calls to the peer are counted.
func TestARoundCostsCallsPerPageNotPerBlockOrAccount(t *testing.T) {
	const K, U = 300, 100
	here := protocol.PartitionUrl("BVN0")
	peerDB := peerStore(t)
	putLedger(t, peerDB, here, 10)
	var accounts []*url.URL
	for i := 0; i < U; i++ {
		u := protocol.AccountUrl(fmt.Sprintf("account%03d", i), "tokens")
		accounts = append(accounts, u)
		putTokens(t, peerDB, u, 5)
	}

	counting := &countingQuerier{Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peerDB, Partition: "BVN0"}), calls: map[string]int{}}
	db := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = db.Close() })
	values, _ := genesisValues(t, 4)
	putNetwork(t, db, here, values)
	s, err := NewState(StateOptions{Partition: here, Database: db, Sources: &onePeer{partition: here, querier: counting, block: 10}, ExecutedBlock: 1})
	require.NoError(t, err)
	ctx := context.Background()

	// Round one: the pull starts at 10 and takes the top of the tree. It
	// takes no account nothing names (executor spec, "Sync", §3a).
	require.NoError(t, s.Pull(ctx))
	none := db.Begin(false)
	_, err = none.Account(accounts[U-1]).Main().Get()
	none.Discard()
	require.Error(t, err, "the top of the tree took an account nothing named")

	// Blocks 11 through 10+K: each names three accounts, and every account
	// ends at a new balance.
	for i, u := range accounts {
		putTokens(t, peerDB, u, int64(100+i))
	}
	batch := peerDB.Begin(true)
	ledger := batch.Account(here.JoinPath(protocol.Ledger))
	for n := uint64(11); n <= 10+K; n++ {
		var entries []*protocol.BlockEntry
		for j := 0; j < 3; j++ {
			u := accounts[(int(n)*3+j)%U]
			entries = append(entries, &protocol.BlockEntry{Account: u, Chain: "main", Index: n})
		}
		require.NoError(t, ledger.BlockLedger(n).Put(&database.BlockLedger{Index: n, Entries: entries}))
	}
	require.NoError(t, batch.Commit())
	putLedger(t, peerDB, here, 10+K)
	s.sources.(*onePeer).block = 10 + K

	// Round two, counted.
	counting.calls = map[string]int{}
	require.NoError(t, s.Pull(ctx))
	require.Equal(t, uint64(10+K), s.sync.last, "precondition: the round processed the records through block %d", 10+K)
	for i, u := range accounts {
		require.Equal(t, int64(100+i), balanceIn(t, db, u), "%v was not brought to the state the records name", u)
	}

	t.Logf("a round over %d blocks naming %d accounts asked the peer %d calls:%v", K, U, counting.total(), counting)
	require.LessOrEqual(t, counting.total(), 40,
		"a round over %d blocks naming %d accounts made %d calls (K + 8U would be %d): the calls grow with blocks or accounts, not pages",
		K, U, counting.total(), K+8*U)
}

// onePeer is one peer: the production querier over its store, reached as a
// named source whose account answers carry a receipt at the peer's block.
type onePeer struct {
	noValidators
	partition *url.URL
	querier   api.Querier
	block     uint64
}

func (p *onePeer) For(context.Context, *url.URL) ([]pull.Source, *url.URL, error) {
	return []pull.Source{servedAt{Source: api.Querier2{Querier: p.querier}, block: p.block}}, p.partition, nil
}

func (p *onePeer) Querier(*url.URL) api.Querier { return p.querier }

// tearingQuerier changes an account on the peer, once, right after the first
// call that reads it: between two calls, where a peer's store moves.
type tearingQuerier struct {
	api.Querier
	account *url.URL
	change  func()
	done    bool
}

func (q *tearingQuerier) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	r, err := q.Querier.Query(ctx, scope, query)
	if !q.done && q.reads(scope, query) {
		q.done = true
		q.change()
	}
	return r, err
}

func (q *tearingQuerier) reads(scope *url.URL, query api.Query) bool {
	if scope.Equal(q.account) {
		return true
	}
	// A batch read names the account in its body; its encoding carries the
	// URL, whatever the query's type.
	b, err := query.MarshalBinary()
	return err == nil && bytes.Contains(b, []byte(q.account.String()))
}

// TestAnAccountIsNotTakenTorn — #4453, executor spec, "Sync", §3: "each of
// those calls read the peer's store separately, so one account's parts could
// come from different blocks" (#4411, #4442). alice's body and her main chain
// change together, in one block, right after the join's first read of her. A
// pull that takes her in a call per part takes the body before the block and
// the chain after it: a leaf no block ever had. The batch reads her in one
// view, and the leaf the join holds is one the peer held.
func TestAnAccountIsNotTakenTorn(t *testing.T) {
	here := protocol.PartitionUrl("BVN0")
	alice := protocol.AccountUrl("alice", "tokens")
	peerDB := peerStore(t)
	putLedger(t, peerDB, here, 10)
	putTokens(t, peerDB, alice, 5)
	appendMain(t, peerDB, alice, 1)
	before := leafIn(t, peerDB, alice)

	tearing := &tearingQuerier{
		Querier: apiimpl.NewQuerier(apiimpl.QuerierParams{Database: peerDB, Partition: "BVN0"}),
		account: alice,
		change: func() {
			// One block: the body and the chain move together.
			b := peerDB.Begin(true)
			defer b.Discard()
			require.NoError(t, b.Account(alice).Main().Put(&protocol.TokenAccount{Url: alice, TokenUrl: protocol.AcmeUrl(), Balance: *big.NewInt(7)}))
			chain, err := b.Account(alice).MainChain().Get()
			require.NoError(t, err)
			require.NoError(t, chain.AddEntry([]byte{2, 31: 2}, false))
			require.NoError(t, b.UpdateBPT())
			require.NoError(t, b.Commit())
		},
	}
	db := database.OpenInMemory(nil)
	t.Cleanup(func() { _ = db.Close() })
	values, _ := genesisValues(t, 4)
	putNetwork(t, db, here, values)
	s, err := NewState(StateOptions{Partition: here, Database: db, Sources: &onePeer{partition: here, querier: tearing, block: 10}, ExecutedBlock: 1})
	require.NoError(t, err)

	loadSectionOf(t, s, alice)
	require.True(t, tearing.done, "precondition: alice was read, and changed after the read")
	after := leafIn(t, peerDB, alice)
	require.NotEqual(t, before.ValueHash, after.ValueHash)

	got := leafIn(t, db, alice)
	require.Contains(t, [][32]byte{before.ValueHash, after.ValueHash}, got.ValueHash,
		"alice was taken torn: her leaf %x is neither the peer's before the block (%x) nor after it (%x)",
		got.ValueHash[:4], before.ValueHash[:4], after.ValueHash[:4])
}

// appendMain appends n entries to an account's main chain, with the BPT.
func appendMain(t *testing.T, db *database.Database, u *url.URL, n int) {
	t.Helper()
	b := db.Begin(true)
	defer b.Discard()
	chain, err := b.Account(u).MainChain().Get()
	require.NoError(t, err)
	for i := 0; i < n; i++ {
		require.NoError(t, chain.AddEntry([]byte{1, 30: byte(i), 31: 1}, false))
	}
	require.NoError(t, b.UpdateBPT())
	require.NoError(t, b.Commit())
}
