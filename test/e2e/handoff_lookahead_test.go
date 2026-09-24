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
	"time"

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

// Run 20260924T052134Z: acc-bvn1-val1's BVN1 node restarted, joined at 203 and
// handed off with eight collected groups (blocks 204-211). Its block 204 had
// the same round, the same 32 arrivals and the same five batches as its
// peers', and a different root. It delivered MORE than they did: at 204 its
// staging already held the arrivals of blocks 205-211 (Directory anchors
// held=10 sighted=169 against the peers' held=1 sighted=161), because every
// buffered group is taken into staging as it is collected, and the handoff
// then executes Q+1 from that staging. buildRun (stream_run.go) walks past
// the block's own arrivals into held, runnable entries the peers only
// receive in Q+2 and later.
//
// The existing restart tests send nothing while the node joins, so the groups
// after Q+1 carry no stream entries and the lookahead is empty. This keeps
// cross-partition traffic flowing through the join. The control arm drains the
// traffic before the restart and passes; the traffic arm fails, every run.
//
// Proven by a flip: with the simulator's joinState changed to take a buffered
// block into staging only if it is at or below the block handed off at (the
// later ones reach staging only by being executed, as on the peers), the
// traffic arm passes 3/3 and TestOneValidatorRestartDoesNotDiverge and
// TestRestartedNodeWithAPopulatedDatabaseResyncs still pass. Without it the
// traffic arm fails 5/5.
func TestHandoffExecutesQPlusOneWithLaterBlocksStaged(t *testing.T) {
	for _, c := range []struct {
		name          string
		sendWhileJoin bool
	}{
		{"quiet join (control)", false},
		{"traffic through the join", true},
	} {
		t.Run(c.name, func(t *testing.T) { handoffLookahead(t, c.sendWhileJoin) })
	}
}

func handoffLookahead(t *testing.T, sendWhileJoin bool) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e6))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	var ts uint64
	var sts []*TransactionStatus
	send := func() {
		ts++
		sts = append(sts, sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey)))
	}
	// Two sends a block: BVN0 produces synthetics to BVN1 every block, so
	// every block BVN1 commits carries entries on the BVN0 stream.
	step := func(n int, traffic bool) {
		for i := 0; i < n; i++ {
			if traffic {
				send()
				send()
			}
			sim.Step()
		}
	}

	step(30, true)
	for _, st := range sts {
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	if sendWhileJoin {
		// In flight when the node restarts, and for the whole join.
		step(10, true)
	} else {
		// Nothing in flight: every synthetic produced so far is delivered
		// and anchored before the node restarts, so the blocks it collects
		// carry no stream entries.
		step(30, false)
	}

	p := sim.S.Partition("BVN1")
	p.RestartNode(1)
	require.True(t, p.Joining(1))
	step(5, sendWhileJoin)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Three blocks a pull round, as TestOneValidatorRestartDoesNotDiverge
	// steps: the Directory anchors a BVN block some blocks after it, so the
	// block the pull matches is always behind the last block collected, and
	// the handoff produces several buffered groups -- eight in the run.
	const maxRounds = 200
	stepping := &steppingState{step: func(round int) {
		step(3, sendWhileJoin)
		if round >= maxRounds {
			cancel()
		}
	}}
	stepping.State = p.NodeJoinState(1)
	settler, ok := p.NodeExecutor(1).(join.Settler)
	require.True(t, ok)
	rec := &handoffRecorder{Buffer: p.NodeJoin(1)}
	_, err := join.Run(ctx, join.Options{
		Partition: "BVN1",
		Buffer:    rec,
		Stage:     &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(1), Database: p.NodeDatabase(1)},
		State:     stepping,
		Peers:     &join.APIPeers{Partition: "BVN1", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err, "the join did not complete within %d pull rounds", maxRounds)
	require.False(t, p.Joining(1))
	joinedAt := rec.q
	t.Logf("handed off at %d; the network is at %d", joinedAt, partitionBlock(t, p.NodeDatabase(0), PartitionUrl("BVN1")))

	// Same block on every node, then the root chain anchor: a Merkle root
	// over every block root, so any block that differed shows here.
	step(5, sendWhileJoin)
	for i := 0; i < 20 && partitionBlock(t, p.NodeDatabase(0), PartitionUrl("BVN1")) != partitionBlock(t, p.NodeDatabase(1), PartitionUrl("BVN1")); i++ {
		sim.Step()
	}
	anchor := func(i int) []byte {
		var a []byte
		View(t, p.NodeDatabase(i), func(batch *database.Batch) {
			var err error
			a, err = batch.Account(PartitionUrl("BVN1").JoinPath(Ledger)).RootChain().Anchor()
			require.NoError(t, err)
		})
		return a
	}
	require.Equal(t, anchor(0), anchor(2), "precondition: the two nodes that never restarted agree")
	require.Equal(t, anchor(0), anchor(1),
		"BVN1 node 1 diverged after handing off at block %d with later blocks already in its staging", joinedAt)
}

// handoffRecorder records the block the join handed off at.
type handoffRecorder struct {
	join.Buffer
	q uint64
}

func (r *handoffRecorder) Handoff(q uint64) error {
	err := r.Buffer.Handoff(q)
	if err == nil {
		r.q = q
	}
	return err
}
