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
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// Run 20260924T052134Z, acc-bvn3-val1's Directory: restarted at block 613,
// joined at 698, and the handoff's first block failed to open --
//
//	produce buffered group 1 of 22: produce block: begin block: seed synthetic
//	cache: load Directory receipts: load anchor pool main chain entry 2363:
//	Message.c06fcb7d….Main not found
//
// and every committed group after it failed the same way one read later
// (determine last anchored block: load anchor 623: Message.….Main not found).
//
// A restart is a new process: its executor opens its first block by seeding
// the producer cache from the store, and that seed walks dn.acme/anchors'
// main chain and the anchor sequence chain and loads the MESSAGE behind each
// entry (synth_cache_seed.go ownReceipts, block_begin.go lastAnchoredBlock).
// The join pulls those chains as HASHES -- pull.chainEntries asks with
// Expand=false and nothing else writes a body -- so every entry appended by a
// block the node did not execute names a body the node does not hold.
//
// The simulator's RestartNode is not a new process: the executor, its
// cacheSeedOnce and its in-memory cache survive, so the first block after a
// simulated join never seeds and never reads those bodies. This test joins
// with the production pull, then does what the daemon does after a restart:
// opens block Q+1 with a FRESH executor on the joined node's database.
func TestAJoinedNodeCanOpenItsFirstBlockAfterARestart(t *testing.T) {
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
	sim.SetRoute(bob, "BVN0")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

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

	dn := DnUrl()
	p := sim.S.Partition("Directory")
	r := partitionBlock(t, p.NodeDatabase(joiner), dn)

	// The Directory validator restarts and the network runs on without it:
	// anchors keep landing on dn.acme/anchors on its peers and not on it.
	p.RestartNode(joiner)
	require.True(t, p.Joining(joiner))
	for i := uint64(6); i <= 10; i++ {
		send(i)
	}
	sim.StepN(20)

	// The join, as the daemon runs it (see TestOneValidatorRestartDoesNotDiverge).
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stepping := &steppingState{step: func(round int) {
		sim.StepN(3)
		if round >= 200 {
			cancel()
		}
	}}
	stepping.State = pulledState(t, sim, p, joiner, "Directory")
	settler, ok := p.NodeExecutor(joiner).(join.Settler)
	require.True(t, ok)
	_, err := join.Run(ctx, join.Options{
		Partition: "Directory",
		Buffer:    p.NodeJoin(joiner),
		Stage:     &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(joiner), Database: p.NodeDatabase(joiner)},
		State:     stepping,
		Peers:     &join.APIPeers{Partition: "Directory", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err)
	require.False(t, p.Joining(joiner))

	// Stop where the join put the node's state, before it executes anything
	// more, and look at what the store holds.
	q := partitionBlock(t, p.NodeDatabase(joiner), dn)
	t.Logf("the node stopped at R=%d and its state is now Q=%d", r, q)
	require.Greater(t, q, r, "precondition: the join carried the node past blocks it did not execute")

	// What a fresh executor's first block reads: the body behind each entry
	// of the chains it walks by position. Every entry the node did not
	// execute itself -- those appended by blocks R+1 through the block the
	// join handed off at -- is a hash with nothing behind it.
	View(t, p.NodeDatabase(joiner), func(batch *database.Batch) {
		for _, c := range []*database.Chain2{
			batch.Account(dn.JoinPath(AnchorPool)).MainChain(),
			batch.Account(dn.JoinPath(AnchorPool)).AnchorSequenceChain(),
		} {
			head, err := c.Head().Get()
			require.NoError(t, err)
			var missing []int64
			for i := int64(0); i < head.Count; i++ {
				hash, err := c.Entry(i)
				require.NoError(t, err)
				var msg messaging.Message
				if batch.Message2(hash).Main().GetAs(&msg) != nil {
					missing = append(missing, i)
				}
			}
			if len(missing) > 0 {
				t.Errorf("dn.acme/anchors %s chain: %d of %d entries name a message this node does not hold (entries %d..%d): the join pulled the chain's hashes and not the messages they name",
					c.Name(), len(missing), head.Count, missing[0], missing[len(missing)-1])
			}
		}
	})

	// And the read itself: a new process opens block Q+1. The same read on a
	// peer that executed every block is the control -- it opens -- so what
	// fails below is the joined node's store, not a fresh executor.
	fresh := func(node int) coreexec.Executor {
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
			Describe:      coreexec.DescribeShim{NetworkType: PartitionTypeDirectory, PartitionId: Directory},
			Staging:       coreexec.NewStaging(),
			SynthCache:    synthcache.New(0),
		})
		require.NoError(t, err)
		return x
	}
	peerBlock := partitionBlock(t, p.NodeDatabase(0), dn)
	_, err = fresh(0).Begin(coreexec.BlockParams{Context: context.Background(), Index: peerBlock + 1, Time: time.Now()})
	require.NoError(t, err, "control: a restarted peer that executed every block opens its next block")

	_, err = fresh(joiner).Begin(coreexec.BlockParams{Context: context.Background(), Index: q + 1, Time: time.Now()})
	if err != nil {
		b, _ := json.Marshal(err)
		t.Fatalf("a restarted node that joined cannot open the first block after the state it joined at: %s", b)
	}
}

type nopDispatcher struct{}

func (nopDispatcher) Submit(context.Context, *url.URL, *messaging.Envelope) error { return nil }
func (nopDispatcher) Send(context.Context) <-chan error {
	ch := make(chan error)
	close(ch)
	return ch
}
func (nopDispatcher) Close() {}
