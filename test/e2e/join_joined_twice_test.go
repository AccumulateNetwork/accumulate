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
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

func TestASecondRestartAfterAJoinStillOpens(t *testing.T) {
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
		sim.StepUntilN(200, Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
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
	stepping.State = p.NodeJoinState(joiner)
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

	q1 := partitionBlock(t, p.NodeDatabase(joiner), bvn)
	t.Logf("process A: stopped at R=%d, joined at %d, and produced the buffered blocks to Q1=%d", r, matched.block, q1)
	require.Greater(t, q1, r)

	// Process A opens its first block: the span (R, Q1] is skipped.
	_, err = restarted.Begin(coreexec.BlockParams{Context: context.Background(), Index: q1 + 1, Time: time.Now()})
	require.NoError(t, err, "process A opens its first block after the join")

	// The node runs on for a few blocks (fewer than the in-flight window),
	// executing them itself, with traffic. Then the process dies again.
	for i := 0; i < 3; i++ {
		ts++
		submit(ts)
		sim.StepN(1)
	}
	h := partitionBlock(t, p.NodeDatabase(joiner), bvn)
	t.Logf("process A executed on to H=%d, then died", h)
	require.Greater(t, h, q1)

	// Process B: the daemon restarts, joins, and this time fell nothing
	// behind: its join matches its own root at H and settles there. Its
	// executor's ExecutedBlock record says H, so the span it skips is empty
	// had it been recorded in memory. Process A's span (R, Q1] died with
	// process A; the store is what still says which blocks it lacks.
	cacheB := synthcache.New(0)
	processB := fresh(joiner, cacheB)
	require.NoError(t, processB.(join.Settler).SettleStagingAt(h))
	require.Equal(t, h, cacheB.Joined(), "precondition: process B joined at its own height, with no memory of process A's gap")
	_, err = processB.Begin(coreexec.BlockParams{Context: context.Background(), Index: h + 1, Time: time.Now()})
	if err != nil {
		b, _ := json.Marshal(err)
		t.Fatalf("a node that joined at %d, executed to %d and restarted cannot open block %d: %s", q1, h, h+1, b)
	}
}
