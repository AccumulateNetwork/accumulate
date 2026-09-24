// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	coreexec "gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	multi "gitlab.com/accumulatenetwork/accumulate/internal/core/execute/multi"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/synthcache"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// TestAJoinedBVNNodeCanOpenItsFirstBlockAfterARestart is
// TestAJoinedNodeCanOpenItsFirstBlockAfterARestart on a BVN carrying traffic
// while the node is away. A BVN's seed reads more than the anchor pool: for
// each of its own blocks still in flight it rebuilds the producer cache from
// <partition>/synthetic's chains and the messages behind them
// (synth_cache_seed.go rebuildCacheBlock), and the join pulls that account
// state-only -- its chains' heads and open mark sets, and no messages
// (#4400, the debugger's "not verified" list).
func TestAJoinedBVNNodeCanOpenItsFirstBlockAfterARestart(t *testing.T) {
	const joiner = 1

	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
		simulator.BPTHistoryDepth(1024),
	)
	sim.SetRoute(alice, "BVN0")
	// bob is on another partition: a transaction whose principal is on its
	// own partition is executed there through the local queue, and only one
	// that crosses a partition goes out on the synthetic chain.
	sim.SetRoute(bob, "Directory")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	// Each send produces a synthetic deposit to the Directory, so BVN0's own
	// synthetic chain grows in the blocks the node does not execute.
	send := func(ts uint64) {
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	for i := uint64(1); i <= 5; i++ {
		send(i)
	}
	sim.StepN(10)

	bvn := PartitionUrl("BVN0")
	p := sim.S.Partition("BVN0")
	r := partitionBlock(t, p.NodeDatabase(joiner), bvn)

	p.RestartNode(joiner)
	require.True(t, p.Joining(joiner))
	for i := uint64(6); i <= 15; i++ {
		send(i)
	}

	// And a burst just before the join starts: executed in the blocks the
	// first pass is served at, so their synthetics are in blocks the node
	// does not execute and still in flight when it hands off.
	submit := func(ts uint64) {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	}
	for i := uint64(16); i <= 20; i++ {
		submit(i)
	}
	sim.StepN(2)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts := uint64(100)
	stepping := &steppingState{step: func(round int) {
		// Traffic continues while the node pulls, so the blocks it hands
		// off at carry synthetics still in flight.
		ts++
		submit(ts)
		sim.StepN(3)
		if round >= 200 {
			cancel()
		}
	}}
	stepping.State = pulledState(t, sim, p, joiner, "BVN0")
	matched := &matchedAt{steppingState: stepping}
	fresh := func(node int, cache *synthcache.Cache) coreexec.Executor {
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		x, err := multi.NewExecutor(coreexec.Options{
			Logger:        acctesting.NewTestLogger(t),
			Database:      p.NodeDatabase(node),
			Key:           priv,
			Router:        sim.S.Router(),
			EventBus:      events.NewBus(nil),
			NewDispatcher: func() coreexec.Dispatcher { return nopDispatcher{} },
			Sequencer:     sim.S.Services().Private(),
			Querier:       sim.S.Services(),
			Describe:      coreexec.DescribeShim{NetworkType: PartitionTypeBlockValidator, PartitionId: "BVN0"},
			Staging:       coreexec.NewStaging(),
			SynthCache:    cache,
		})
		require.NoError(t, err)
		return x
	}
	// The process that opens the first block after the join is the process
	// that joined: its executor settles staging at the block the join lands
	// on, and that is what tells it which blocks it did not execute. The
	// simulator's node executor produces the buffered blocks, so both settle
	// -- together they are the one executor a daemon has.
	cache := synthcache.New(0)
	restarted := fresh(joiner, cache)
	settler := bothSettle{p.NodeExecutor(joiner).(join.Settler), restarted.(join.Settler)}
	_, err := join.Run(ctx, join.Options{
		Partition: "BVN0",
		Buffer:    p.NodeJoin(joiner),
		Stage:     &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(joiner), Database: p.NodeDatabase(joiner)},
		State:     matched,
		Peers:     &join.APIPeers{Partition: "BVN0", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err)
	require.False(t, p.Joining(joiner))

	q := partitionBlock(t, p.NodeDatabase(joiner), bvn)
	t.Logf("the node stopped at R=%d and its state is now Q=%d", r, q)
	require.Greater(t, q, r, "precondition: the join carried the node past blocks it did not execute")

	// What the seed used to read and fail on: the synthetic chain entries of
	// blocks the node did not execute are in its store -- the open mark set
	// comes with the chain's head -- and the messages behind them are not.
	View(t, p.NodeDatabase(joiner), func(batch *database.Batch) {
		c := batch.Account(bvn.JoinPath(Synthetic)).SyntheticChain(Directory)
		head, err := c.Head().Get()
		require.NoError(t, err)
		var missing int
		for i := head.Count - 1; i >= 0; i-- {
			hash, err := c.Entry(i)
			if err != nil {
				break // below the open mark set: not held
			}
			if _, err := batch.Message2(hash).Main().Get(); err != nil {
				missing++
			}
		}
		t.Logf("%d of the synthetic chain entries this node holds name a message it does not", missing)
		require.NotZero(t, missing, "precondition: the node holds synthetic chain entries of blocks it did not execute, with no message behind them")
	})

	peerBlock := partitionBlock(t, p.NodeDatabase(0), bvn)
	peerCache := synthcache.New(0)
	_, err = fresh(0, peerCache).Begin(coreexec.BlockParams{Context: context.Background(), Index: peerBlock + 1, Time: time.Now()})
	require.NoError(t, err, "control: a restarted peer that executed every block opens its next block")

	_, err = restarted.Begin(coreexec.BlockParams{Context: context.Background(), Index: q + 1, Time: time.Now()})
	if err != nil {
		b, _ := json.Marshal(err)
		t.Fatalf("a restarted BVN node that joined cannot open the first block after the state it joined at: %s", b)
	}

	// What the seed read: the synthetics of blocks after R, which this node
	// did not execute. Without any, the test says nothing about them.
	// The seed opened on what it could read: the blocks after the one the
	// node joined at, which it executed. Nothing at or below it was this
	// node's to produce, and the process's own join is what said where that
	// is (SettleStaging, synthcache.JoinedAt).
	require.Equal(t, matched.block, cache.Joined(), "the join told the process's cache the block it joined at")
	entries, blocks := cache.Len()
	t.Logf("the seed rebuilt %d synthetic entries in %d blocks, all above %d", entries, blocks, matched.block)
}

// matchedAt records the block the join last matched, which is the block it
// hands off at: the node executes the blocks after it and none before.
type matchedAt struct {
	*steppingState
	block uint64
}

func (m *matchedAt) Matched(ctx context.Context) (uint64, bool, error) {
	b, ok, err := m.steppingState.Matched(ctx)
	if ok && err == nil {
		m.block = b
	}
	return b, ok, err
}

// bothSettle settles staging on two executors that stand in for one.
type bothSettle [2]join.Settler

func (b bothSettle) SettleStagingAt(q uint64) error {
	for _, s := range b {
		if err := s.SettleStagingAt(q); err != nil {
			return err
		}
	}
	return nil
}
