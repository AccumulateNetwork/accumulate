// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
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
	return s.inner.For(ctx, account)
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
// none (#4317): THE SET OF ACCOUNTS TO PULL COMES FROM THE BLOCK LEDGER.
//
// The auditor's mutation run established that making
// PulledState.changedAccounts return nil always leaves the whole suite green,
// while neutering staleAccounts turns TestJoinPullsFromPeersAndPromotes red.
// The single join test is carried entirely by the BPT page-diff backstop, so
// blockLedger, blockLedgerOf, ChangedAccounts-as-called and MaxLedgerSpan are
// unconstrained: any of them can be broken and nothing notices.
//
// The reason that test cannot see the walk is a construction mismatch. It
// starts the join on emptyDb(), so localBlock() is 0 and the round-one full
// scan answers every question. The daemon never creates that configuration:
// it joins a node that HAS state (cmd/accumulated/run/dagbft.go, joining :=
// lastBlock > 0).
//
// So this test runs the join in two phases. The first is the ordinary join,
// with the page diff available, and it exists only to produce the state the
// second phase starts from: a node holding the partition at a block. The
// second takes the backstop away, moves the network on, and requires the node
// to follow it. Only the block ledger can say what those blocks changed.
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

	// Phase one: the ordinary join, only so that phase two starts where a
	// daemon starts one -- from a node that holds the partition at a block.
	var promoted bool
	for round := 0; round < 40 && !promoted; round++ {
		require.NoError(t, state.Pull(ctx), "pull round %d", round)
		sim.StepN(3)
		_, promoted, err = state.Matched(ctx)
		require.NoError(t, err)
	}
	require.True(t, promoted, "the join never caught up, so phase two has no state to start from")
	require.NotZero(t, sources.served.Load(), "the page diff never ran in phase one")

	require.NotNil(t, balanceOf(t, local, tokens), "the join did not pull %v at all", tokens)

	// Phase two: no peer will page its BPT from here on, and the network
	// creates an account this node has never heard of.
	//
	// A NEW account is the discriminating case. The join retries what it
	// refused for the life of the process (PulledState.refused), so an account
	// it has already asked for once goes on being re-pulled at its newest
	// state whether or not anything names it — which is why following a
	// balance would prove nothing here. A name that has never been in that set
	// can only arrive from the block ledger's record of the block that created
	// it.
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

	// No page was served in phase two. Whether the page diff was ASKED is
	// not pinned: the spec runs it on the first round and on a cadence after
	// that, and phase two is shorter than the cadence, so requiring an ask
	// here pinned a cadence inside a window shorter than the cadence and was
	// red for that reason alone. The refusal is armed either way; what the
	// test proves is that with no page served, the block ledger alone named
	// the new account.
	require.Equal(t, servedBefore, sources.served.Load(), "a BPT page was served after the backstop was taken away")

	require.True(t, held,
		"the node never pulled %v. With no BPT page diff and no earlier refusal to retry, "+
			"only the block ledger can say that a block created it (#4306)", savings)
}

// TestJoinWalksTheBlockLedgerPast128Blocks is #4356: a join that lasts more
// than MaxLedgerSpan blocks still takes its changed set from the block ledger.
//
// PulledState.localBlock answers the block the executor stood at when the join
// STARTED, and nothing moves it while the join runs. changedAccounts measures
// the walk's span from there, so once the peers are more than 128 blocks past
// that block every round is wide and walks nothing: the join runs on the BPT
// page diff alone, every round, for the rest of its life. The peers move on
// about a block a second, so that is any join longer than two minutes.
//
// The span the walk wants is since the block this node's state is at now, not
// since the block it started from. This is TestJoinKeepsUpFromTheBlockLedgerAlone
// with the network run more than 128 blocks past where the join started. After
// phase one the node's state is a few blocks behind its peers, so a walk from
// there is short; a walk from where the join started is not taken at all.
//
// It does not require the page diff to be attempted in phase two: a join that
// walks the ledger has no reason to page on every round, and one that found
// what it wanted in the first round never gets to the backstop's cadence.
func TestJoinWalksTheBlockLedgerPast128Blocks(t *testing.T) {
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

	// The join starts where a node holding only its genesis network accounts
	// starts: its executor has executed nothing, so localBlock is zero.
	local := genesisOf(t, sim, "BVN0")
	state, err := join.NewState(join.StateOptions{
		Partition: part,
		Database:  local,
		Sources:   sources,
	})
	require.NoError(t, err)

	// The network runs on past one walk's reach of where the join started,
	// as it does under any join that lasts more than a couple of minutes.
	sim.StepN(join.MaxLedgerSpan + 20)
	require.Greater(t, sim.S.BlockIndex("BVN0"), uint64(join.MaxLedgerSpan),
		"the network is not %d blocks past where the join started", join.MaxLedgerSpan)

	// Phase one: the ordinary join, with the page diff available, so that
	// phase two starts from a node that holds the partition at a block.
	var promoted bool
	for round := 0; round < 40 && !promoted; round++ {
		require.NoError(t, state.Pull(ctx), "pull round %d", round)
		sim.StepN(3)
		_, promoted, err = state.Matched(ctx)
		require.NoError(t, err)
	}
	require.True(t, promoted, "the join never caught up, so phase two has no state to start from")
	require.NotZero(t, sources.served.Load(), "the page diff never ran in phase one")
	require.NotNil(t, balanceOf(t, local, tokens), "the join did not pull %v at all", tokens)

	// Phase two: no peer will page its BPT, and the network creates an
	// account this node has never heard of. Only the block ledger's record of
	// the block that created it can name it (see
	// TestJoinKeepsUpFromTheBlockLedgerAlone for why a new account is the
	// discriminating case).
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

	require.Equal(t, servedBefore, sources.served.Load(), "a BPT page was served after the backstop was taken away")
	require.True(t, held,
		"the node never pulled %v. The network is more than %d blocks past the block the join "+
			"started from, so every round measured from there is wide and walks no block ledger; "+
			"the walk must start from the block the node's state is at now (#4356)",
		savings, join.MaxLedgerSpan)
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
