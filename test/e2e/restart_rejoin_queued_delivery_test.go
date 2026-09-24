// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
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

// TestRestartedNodeResyncsUnderSamePartitionLoad is
// TestRestartedNodeWithAPopulatedDatabaseResyncs with the one thing the live
// network has and that test does not: traffic still flowing while the node
// pulls, so the block a peer serves has a same-partition delivery queued on
// <partition>/synthetic (run 20260924T052134Z, acc-bvn1-val1 and acc-bvn3-val1:
// "acc://bvn-BVN1.acme/synthetic: the state served does not hash into the
// anchored root" on every pass).
//
// The original sends and then steps until the deposit is PRODUCED and
// executed, and steps idle blocks between pull rounds, so every block a peer
// serves has an empty LocalDeliveryQueue and the synthetic ledger verifies by
// accident.
//
// "drained" is the control: a send per round, each waited on until its deposit
// has executed, so every block a peer serves has an empty queue. "queued" keeps
// a send executing in every block, so every block a peer serves has a deposit
// queued for the next — which is what every block of a loaded network looks
// like. The control settles; "queued" is refused on every pass, for every
// round, deterministically.
func TestRestartedNodeResyncsUnderSamePartitionLoad(t *testing.T) {
	t.Run("drained", func(t *testing.T) { restartResyncUnderLoad(t, false) })
	t.Run("queued", func(t *testing.T) { restartResyncUnderLoad(t, true) })
}

func restartResyncUnderLoad(t *testing.T, pullWhileQueued bool) {
	const joiner = 2

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

	var ts uint64
	// send submits alice -> bob and steps until the deposit has executed.
	// Both are on BVN0, so the deposit is a LOCAL delivery: queued on
	// <partition>/synthetic at the end of the block the send executes in, and
	// drained at the start of the next.
	send := func() {
		ts++
		st := sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}

	// flow keeps a send executing in every block for n blocks: one is
	// submitted before each step, so every block ends with a deposit queued
	// for the next. That is a loaded network; one send per round is not,
	// because a send that takes two blocks leaves a block with an empty
	// queue, and the pass served there settles.
	flow := func(n int) {
		for i := 0; i < n; i++ {
			ts++
			sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(alice, "tokens").
					SendTokens(1, 0).To(bob, "tokens").
					SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
			sim.StepN(1)
		}
	}

	for i := 0; i < 5; i++ {
		send()
	}
	sim.StepN(10)

	part := PartitionUrl("BVN0")
	p := sim.S.Partition("BVN0")
	r := partitionBlock(t, p.NodeDatabase(joiner), part)

	p.RestartNode(joiner)
	require.True(t, p.Joining(joiner))
	for i := 0; i < 10; i++ {
		send()
	}
	sim.StepN(30)

	ctx := context.Background()
	sources := &join.QueryPeers{
		Client:  sim.S.Services(),
		Network: t.Name(),
		Router:  sim.S.Router(),
		Self:    p.NodePeerID(joiner),
	}
	state, err := join.NewState(join.StateOptions{
		Partition: part,
		Database:  p.NodeDatabase(joiner),
		Sources:   sources,
	})
	require.NoError(t, err)

	synth := part.JoinPath(Synthetic)
	var matched bool
	var queuedAtPull, rounds int
	for round := 0; round < 60 && !matched; round++ {
		// The network keeps working while the node pulls.
		if pullWhileQueued {
			flow(3)
		} else {
			send()
		}
		rounds++
		if localQueueLen(t, p.NodeDatabase(0), synth) > 0 {
			queuedAtPull++
		}

		require.NoError(t, state.Pull(ctx), "pull round %d", round)
		_, matched, err = state.Matched(ctx)
		require.NoError(t, err)
	}

	if pullWhileQueued {
		require.Equal(t, rounds, queuedAtPull, "precondition: every pull must meet a queued local delivery")
	} else {
		require.Zero(t, queuedAtPull, "precondition: the control must never pull with a delivery queued")
	}
	t.Logf("stopped at R=%d; pulled %d times with a delivery queued on the peer", r, queuedAtPull)

	require.True(t, matched,
		"the restarted node never reached a root the Directory anchored while same-partition\n"+
			"deliveries were queued at the blocks it was served (%d of its pulls): %v's leaf commits\n"+
			"to its LocalDeliveryQueue, which the querier does not serve and the pull does not write",
		queuedAtPull, synth)

	// Matching is not enough: the node executes the next block from here, and
	// that block starts by draining the queue it pulled, which loads each
	// queued delivery's message by hash (block.drainDeliveryQueues). A queue
	// pulled without its messages matches and then fails at Q+1 (#4399).
	batch := p.NodeDatabase(joiner).Begin(false)
	defer batch.Discard()
	queue, err := batch.Account(synth).LocalDeliveryQueue().Get()
	require.NoError(t, err)
	if pullWhileQueued {
		require.NotEmpty(t, queue, "precondition: the root the node matched holds a queued delivery")
	}
	for _, id := range queue {
		msg, err := batch.Message(id.Hash()).Main().Get()
		require.NoError(t, err, "the queued delivery %v was pulled without its message", id)
		require.Equal(t, id.Hash(), msg.Hash())
	}
}

func localQueueLen(t *testing.T, db *database.Database, synth *url.URL) int {
	t.Helper()
	batch := db.Begin(false)
	defer batch.Discard()
	q, err := batch.Account(synth).LocalDeliveryQueue().Get()
	require.NoError(t, err)
	return len(q)
}
