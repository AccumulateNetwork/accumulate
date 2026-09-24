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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
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

// #4415's done-when, as written: "an in-process test restarts a node with a
// hole only it has and shows its requester asking within one patience
// window". This is that test, and it shows the criterion is not the one that
// matters.
//
// Each BVN1 node in turn restarts, rejoins by the production join, and then
// loses the first copy of one BVN0 -> BVN1 synthetic -- it alone; every later
// copy, which can only be its own heal, is let through. Traffic keeps
// flowing, so its peers deliver past the number.
//
// Every victim DOES ask, and heals, inside the window. But only by accident:
// its stream is stuck, so its blocks carry nothing to anchor, its stored
// ledger anchor is nil, and previousBlockSeed falls back to the block index --
// a different seed from its peers', whose anchored ledgers give the constant
// zero seed (see heal_pull_pair_test.go). Its own divergent state is what
// selects it.
//
// And asking is too late. The block in which the peers delivered the hole
// executed on the victim without it (stream_run.go buildRun stops at the
// first number nothing holds), so its state is not its peers' from that
// block on, and the heal landing afterwards does not make it so: its root
// chain differs for good. This fails there, for every node. A hole only one
// node has is a divergence in the block it is reached, whoever asks and
// however soon -- #4412's mechanism, reached here by one dropped message
// rather than by a restart's empty staging.
func TestAPrivateHoleDivergesTheNodeEvenWhenItAsks(t *testing.T) {
	const nodes = 4
	for i := 0; i < nodes; i++ {
		i := i
		t.Run(fmt.Sprintf("node%d", i), func(t *testing.T) {
			restartWithPrivateHole(t, nodes, i)
		})
	}
}

func restartWithPrivateHole(t *testing.T, nodes, victim int) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	sim := NewSim(t,
		simulator.SimpleNetwork("private-hole", 2, nodes),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")

	seqOf := func(m messaging.Message) (uint64, bool) {
		syn, ok := m.(*messaging.SyntheticMessage)
		if !ok {
			return 0, false
		}
		seq, ok := syn.Message.(*messaging.SequencedMessage)
		if !ok || seq.Source == nil || !seq.Source.Equal(PartitionUrl("BVN0")) {
			return 0, false
		}
		return seq.Number, true
	}

	// The victim loses the first copy of `hole` and nothing else. Any later
	// copy is an answer to a request -- no peer lacks the entry, so no peer
	// asks for it, and a source never pushes an entry twice.
	var mu sync.Mutex
	var hole uint64
	dropped := false
	copies := 0
	sim.S.SetNodeBlockHook("BVN1", func(node int, _ execute.BlockParams, envelopes []*messaging.Envelope) ([]*messaging.Envelope, bool) {
		mu.Lock()
		defer mu.Unlock()
		if node != victim || hole == 0 {
			return envelopes, true
		}
		var kept []*messaging.Envelope
		for _, env := range envelopes {
			var rest []messaging.Message
			touched := false
			for _, m := range env.Messages {
				if n, ok := seqOf(m); ok && n == hole {
					copies++
					if !dropped {
						dropped = true
						touched = true
						continue
					}
				}
				rest = append(rest, m)
			}
			switch {
			case !touched:
				kept = append(kept, env)
			case len(rest) > 0:
				e := env.Copy()
				e.Messages = rest
				kept = append(kept, e)
			}
		}
		return kept, true
	})

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e12)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1e9))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	ts := uint64(0)
	send := func() *TransactionStatus {
		ts++
		return sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	}

	p := sim.S.Partition("BVN1")
	delivered := func(i int) uint64 {
		var d uint64
		View(t, p.NodeDatabase(i), func(batch *database.Batch) {
			var ledger *SyntheticLedger
			require.NoError(t, batch.Account(PartitionUrl("BVN1").JoinPath(Synthetic)).Main().GetAs(&ledger))
			d = ledger.Partition(PartitionUrl("BVN0")).Delivered
		})
		return d
	}

	// A stream with traffic on it and no hole.
	var sts []*TransactionStatus
	for i := 0; i < 5; i++ {
		sts = append(sts, send())
		sim.Step()
	}
	for _, st := range sts {
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}

	// The victim restarts and rejoins by the production join.
	p.RestartNode(victim)
	require.True(t, p.Joining(victim))
	sim.StepN(5)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const maxRounds = 100
	stepping := &steppingState{step: func(round int) {
		sim.StepN(3)
		if round >= maxRounds {
			cancel()
		}
	}}
	stepping.State = p.NodeJoinState(victim)
	settler, ok := p.NodeExecutor(victim).(join.Settler)
	require.True(t, ok)
	_, err := join.Run(ctx, join.Options{
		Partition: "BVN1",
		Buffer:    p.NodeJoin(victim),
		Stage:     &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(victim), Database: p.NodeDatabase(victim)},
		State:     stepping,
		Peers:     &join.APIPeers{Partition: "BVN1", Client: sim.S.Services(), Network: "private-hole"},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err, "the join did not complete within %d pull rounds", maxRounds)
	require.False(t, p.Joining(victim))
	sim.StepN(5)

	// A hole only the victim has, right above where every node stands.
	d0 := delivered(victim)
	for i := 0; i < nodes; i++ {
		require.Equal(t, d0, delivered(i), "precondition: node %d stands where the victim does", i)
	}
	mu.Lock()
	hole = d0 + 1
	mu.Unlock()
	requestsBefore := p.NodeHeals(victim).Requests.Load()

	// Traffic every block, so the peers deliver past the hole and every
	// block has something to anchor. The window is one full probe wait plus
	// one patience window, for EVERY activation (not only the ones this node
	// happens to be selected on), with slack: 8*4 + 3*4 = 44 blocks, run 100.
	const window = 100
	for b := 0; b < window; b++ {
		send()
		sim.Step()
		if delivered(victim) > hole {
			break
		}
	}
	mu.Lock()
	sawCopies := copies
	mu.Unlock()
	peer := (victim + 1) % nodes
	t.Logf("victim %d: hole %d, Delivered %d (peer %d at %d), copies of the hole that reached it %d, its requests %d -> %d",
		victim, hole, delivered(victim), peer, delivered(peer), sawCopies, requestsBefore, p.NodeHeals(victim).Requests.Load())
	require.Greater(t, delivered(peer), hole, "precondition: the peers delivered past the victim's hole")
	require.Greater(t, delivered(victim), hole,
		"node %d had a hole only it had and never asked for it within %d blocks: its stream is stuck at %d while its peers are at %d",
		victim, window, delivered(victim), delivered(peer))

	// Asking was not enough. The victim executed the blocks its peers
	// executed with #hole in them without it, so its state is not theirs,
	// and it stays that way after the heal lands.
	sim.StepN(30)
	var roots [][]byte
	for i := 0; i < nodes; i++ {
		View(t, p.NodeDatabase(i), func(batch *database.Batch) {
			a, err := batch.Account(PartitionUrl("BVN1").JoinPath(Ledger)).RootChain().Anchor()
			require.NoError(t, err)
			roots = append(roots, a)
		})
		t.Logf("node %d: root chain anchor %x, Delivered %d", i, roots[i][:4], delivered(i))
	}
	require.Equal(t, roots[peer], roots[victim],
		"node %d asked for its private hole and healed it, and its root chain still differs from its peers'", victim)
}
