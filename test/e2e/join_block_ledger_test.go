// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"fmt"
	"math/big"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/anchorsrc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/pull"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// pagelessSources is a peer set that can be told to stop paging its BPT. It
// is not a fake: every read still goes to the simulator's nodes through
// join.QueryPeers, and only the BPT page query is refused, the way a peer too
// busy to finish a paged read refuses it (state.go logs that case and carries
// on).
type pagelessSources struct {
	inner join.Sources

	refuse atomic.Bool
	asked  atomic.Int64 // BPT page queries seen
	served atomic.Int64 // BPT page queries answered
}

func (s *pagelessSources) For(ctx context.Context, account *url.URL) ([]pull.Source, *url.URL, error) {
	srcs, partition, err := s.inner.For(ctx, account)
	for i, src := range srcs {
		srcs[i] = pagelessSource{Source: src, owner: s}
	}
	return srcs, partition, err
}

// pagelessSource is one named peer, whose BPT pages are counted and refused
// as the owner says: the walk reads each page from a named peer (#4438).
type pagelessSource struct {
	pull.Source
	owner *pagelessSources
}

func (s pagelessSource) String() string { return fmt.Sprint(s.Source) }

func (s pagelessSource) QueryBptPage(ctx context.Context, scope *url.URL, q *api.BptPageQuery) (*api.BptPageRecord, error) {
	s.owner.asked.Add(1)
	if s.owner.refuse.Load() {
		return nil, errors.NotReady.WithFormat("%v: no peer will page its BPT for this node", scope)
	}
	s.owner.served.Add(1)
	return s.Source.(interface {
		QueryBptPage(context.Context, *url.URL, *api.BptPageQuery) (*api.BptPageRecord, error)
	}).QueryBptPage(ctx, scope, q)
}

func (s *pagelessSources) ValidatorsOf(ctx context.Context, partition *url.URL) ([]anchorsrc.Validator, error) {
	return s.inner.ValidatorsOf(ctx, partition)
}

func (s *pagelessSources) Querier(partition *url.URL) api.Querier {
	return &pagelessQuerier{inner: s.inner.Querier(partition), owner: s}
}

type pagelessQuerier struct {
	inner api.Querier
	owner *pagelessSources
}

func (q *pagelessQuerier) Query(ctx context.Context, scope *url.URL, query api.Query) (api.Record, error) {
	if _, ok := query.(*api.BptPageQuery); ok {
		q.owner.asked.Add(1)
		if q.owner.refuse.Load() {
			return nil, errors.NotReady.WithFormat("%v: no peer will page its BPT for this node", scope)
		}
		q.owner.served.Add(1)
	}
	return q.inner.Query(ctx, scope, query)
}

// TestJoinKeepsUpFromTheBlockLedgerAlone is the guard on the #4306 fix that had
// none (#4317): THE ACCOUNTS A BLOCK CHANGED ARE NAMED BY THE BLOCK LEDGER.
//
// The join walks the peer's whole BPT once and processes every block-ledger
// record from the start of its pull, in order (executor spec, "Sync", "The
// algorithm", steps 1-2; #4438). Once the walk has covered the tree, a block's
// changes reach the node only through that block's record. So this test runs
// the join in two phases. The first lets the walk cover the tree. The second
// takes the peers' BPT pages away, has the network create an account the node
// has never heard of, and requires the node to pull it: only the record of the
// block that created it can name it.
//
// A NEW account is the discriminating case: an account the join already
// asked for could be re-pulled for another reason (a retry), a name that has
// never been seen can only come from the record.
func TestJoinKeepsUpFromTheBlockLedgerAlone(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	ts := uint64(0)
	send := func() {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	for i := 0; i < 3; i++ {
		send()
	}
	sim.StepN(20)

	ctx := context.Background()
	part := PartitionUrl("BVN0")
	tokens := alice.JoinPath("tokens")

	sources := &pagelessSources{inner: &join.QueryPeers{
		Client:  sim.S.Services(),
		Network: t.Name(),
		Router:  sim.S.Router(),
	}}

	// What a joining node really holds before it asks anybody anything: its
	// own genesis network accounts. They are the keys it verifies anchors
	// against, and they are the one thing it must not take from a peer
	// (#4301). An empty store is not a case: every node loads genesis before
	// it joins, and a node with no key to start from cannot verify a root.
	local := genesisOf(t, sim, "BVN0")
	state, err := join.NewState(join.StateOptions{
		Partition: part,
		Database:  local,
		Sources:   sources,
	})
	require.NoError(t, err)

	// Phase one: the walk covers the tree. The partition is small, so one
	// round's pages do; a few more rounds let the records run on.
	for round := 0; round < 5; round++ {
		require.NoError(t, state.Pull(ctx), "pull round %d", round)
		sim.StepN(3)
	}
	require.NotZero(t, sources.served.Load(), "the walk was never served a page in phase one")
	require.NotNil(t, balanceOf(t, local, tokens), "the join did not pull %v at all", tokens)

	// Phase two: no peer will page its BPT from here on, and the network
	// creates an account this node has never heard of.
	sources.refuse.Store(true)
	servedBefore := sources.served.Load()

	savings := alice.JoinPath("savings")
	ts++
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice).
			CreateTokenAccount(alice, "savings").ForToken(AcmeUrl()).
			SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())
	send()
	sim.StepN(20)

	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		_, err := batch.Account(savings).Main().Get()
		require.NoError(t, err, "the partition did not create %v, so there is nothing to follow", savings)
	})

	var held bool
	for round := 0; round < 40 && !held; round++ {
		require.NoError(t, state.Pull(ctx), "phase two pull round %d", round)
		sim.StepN(1)
		View(t, local, func(batch *database.Batch) {
			_, err := batch.Account(savings).Main().Get()
			held = err == nil
		})
	}

	require.Equal(t, servedBefore, sources.served.Load(), "a BPT page was served after the pages were taken away")
	require.True(t, held,
		"the node never pulled %v. With no BPT page and no earlier refusal to retry, "+
			"only the block ledger can say that a block created it (#4306)", savings)
}

// TestJoinFollowsTheRecordsAcrossAnyNumberOfBlocks: the records are processed
// from the last one processed through the peer's block, however many blocks
// that is. Before #4438 a round measured a span and, past 128 blocks
// (MaxLedgerSpan), walked no block ledger at all and fell back to the page
// diff for the rest of the join (#4356). There is no span now: this is
// TestJoinKeepsUpFromTheBlockLedgerAlone with the join paused for more than
// 128 blocks between its rounds, and the account created in them still
// reaches the node from the records alone.
func TestJoinFollowsTheRecordsAcrossAnyNumberOfBlocks(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN0")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	ts := uint64(0)
	send := func() {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	for i := 0; i < 3; i++ {
		send()
	}
	sim.StepN(20)

	ctx := context.Background()
	part := PartitionUrl("BVN0")
	tokens := alice.JoinPath("tokens")

	sources := &pagelessSources{inner: &join.QueryPeers{
		Client:  sim.S.Services(),
		Network: t.Name(),
		Router:  sim.S.Router(),
	}}

	local := genesisOf(t, sim, "BVN0")
	state, err := join.NewState(join.StateOptions{
		Partition: part,
		Database:  local,
		Sources:   sources,
	})
	require.NoError(t, err)

	// Phase one: the walk covers the tree.
	for round := 0; round < 5; round++ {
		require.NoError(t, state.Pull(ctx), "pull round %d", round)
		sim.StepN(3)
	}
	require.NotZero(t, sources.served.Load(), "the walk was never served a page in phase one")
	require.NotNil(t, balanceOf(t, local, tokens), "the join did not pull %v at all", tokens)

	// The join pauses while the network runs on well past what the old
	// span allowed.
	const paused = 150
	before := sim.S.BlockIndex("BVN0")
	sim.StepN(paused)
	require.Greater(t, sim.S.BlockIndex("BVN0")-before, uint64(128), "the network did not run 128 blocks past the join")

	// Phase two: no peer will page its BPT, and the network creates an
	// account this node has never heard of. Only the block ledger's record of
	// the block that created it can name it.
	sources.refuse.Store(true)
	servedBefore := sources.served.Load()

	savings := alice.JoinPath("savings")
	ts++
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(alice).
			CreateTokenAccount(alice, "savings").ForToken(AcmeUrl()).
			SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	sim.StepUntil(Txn(st.TxID).Succeeds())
	send()
	sim.StepN(20)

	View(t, sim.DatabaseFor(alice), func(batch *database.Batch) {
		_, err := batch.Account(savings).Main().Get()
		require.NoError(t, err, "the partition did not create %v, so there is nothing to follow", savings)
	})

	var held bool
	for round := 0; round < 40 && !held; round++ {
		require.NoError(t, state.Pull(ctx), "phase two pull round %d", round)
		sim.StepN(1)
		View(t, local, func(batch *database.Batch) {
			_, err := batch.Account(savings).Main().Get()
			held = err == nil
		})
	}

	require.Equal(t, servedBefore, sources.served.Load(), "a BPT page was served after the pages were taken away")
	require.True(t, held,
		"the node never pulled %v: the records after a pause of %d blocks were not all processed (#4356)",
		savings, paused)
}

// balanceOf is the balance a store holds for a token account, or nil if it
// does not hold the account at all.
func balanceOf(t *testing.T, db database.Updater, u *url.URL) *big.Int {
	t.Helper()
	var bal *big.Int
	View(t, db, func(batch *database.Batch) {
		var acct *TokenAccount
		err := batch.Account(u).Main().GetAs(&acct)
		if errors.Is(err, errors.NotFound) {
			return
		}
		require.NoError(t, err)
		bal = new(big.Int).Set(&acct.Balance)
	})
	return bal
}
