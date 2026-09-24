// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"math/big"
	"sync"
	"sync/atomic"
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

// #4412, run 20260924T074702Z. A Directory validator restarted, joined at its
// own block 633 and matched its peers for 24 blocks, then executed block 658
// with the same batches as its peers and a different root.
//
// The stream BVN1 -> Directory had a hole every node shared: entry 740 was
// missing everywhere (BVN1 had lost a dispatcher and fell to healing). Behind
// it every peer held 742-751 and 754-758, received and validated BEFORE the
// restart. The restarted node's staging is memory and came back empty; the
// handoff's gap check (join.ExecutorStage.HasGap) asks only about what the
// node's own staging holds, and it held nothing on that stream at Q+1, so it
// saw no gap and handed off. The node then ran 24 blocks identical to its
// peers', because every node's run on that stream stopped at the shared hole.
// At 658 the heal of 740 landed on every node: the peers ran 740..812, the
// restarted node ran 740..741 and stopped at 742 -- a validated number whose
// body it never had (buildRun's "Missing" stop, stream_run.go). Different
// Delivered, different root.
//
// This stands the same thing up on BVN1: a hole every node shares at
// Delivered+1, entries held behind it on every node, one node restarted and
// rejoined, and then the hole's entry lands. (In the simulator the held
// entries carry their own proofs, so staging's validated list is empty; in
// the soak it reached 770 on every node. The divergence does not depend on
// it: the run stops at the first number nothing holds either way.)
//
// It fails on demand at the Delivered comparison: 12 on the peers, 6 on the
// rejoined node, in the same block.
func TestARejoinedNodeRunsThePeersRunWhenASharedHoleFills(t *testing.T) {
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

	// seqOf is the BVN0 -> BVN1 synthetic sequence number a message carries.
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

	// The hook is per node and deterministic, so every node sees the same
	// thing, and it filters messages, not envelopes: a heal envelope carries
	// a span. hole: every copy of that number is held back (the original and
	// every heal of it) until release. suppress: copies of these numbers
	// that arrive after the restart are discarded -- the restarted node's
	// own heal of what its peers hold has not landed when the hole fills,
	// which is the soak's timing (the peers' heal of 740 was requested at
	// 656 and landed at 658; the restarted node noticed at 661).
	var hookMu sync.Mutex
	var hole atomic.Uint64
	var suppressLo, suppressHi atomic.Uint64
	var release atomic.Bool
	delayed := map[int][]*messaging.Envelope{}
	sim.S.SetNodeBlockHook("BVN1", func(node int, _ execute.BlockParams, envelopes []*messaging.Envelope) ([]*messaging.Envelope, bool) {
		hookMu.Lock()
		defer hookMu.Unlock()
		var kept []*messaging.Envelope
		for _, env := range envelopes {
			touched := false
			var rest []messaging.Message
			for _, m := range env.Messages {
				n, ok := seqOf(m)
				switch {
				case ok && hole.Load() != 0 && n == hole.Load() && !release.Load():
					delayed[node] = append(delayed[node], &messaging.Envelope{Messages: []messaging.Message{m}})
					touched = true
				case ok && suppressLo.Load() != 0 && n >= suppressLo.Load() && n <= suppressHi.Load():
					touched = true
				default:
					rest = append(rest, m)
				}
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
		if release.Load() && len(delayed[node]) > 0 {
			kept = append(delayed[node], kept...)
			delayed[node] = nil
		}
		return kept, true
	})

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	var sts []*TransactionStatus
	send := func(ts uint64) {
		sts = append(sts, sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey)))
	}

	p := sim.S.Partition("BVN1")
	stream := execute.StreamID{Ledger: PartitionUrl("BVN1").JoinPath(Synthetic), Source: PartitionUrl("BVN0")}
	status := func(i int) execute.StreamStatus {
		tx := p.NodeStaging(i).Begin()
		defer tx.Discard()
		for _, st := range tx.Streams() {
			if st.ID.Ledger.Equal(stream.Ledger) && st.ID.Source.Equal(stream.Source) {
				return st
			}
		}
		return execute.StreamStatus{}
	}
	delivered := func(i int) uint64 {
		var d uint64
		View(t, p.NodeDatabase(i), func(batch *database.Batch) {
			var ledger *SyntheticLedger
			require.NoError(t, batch.Account(PartitionUrl("BVN1").JoinPath(Synthetic)).Main().GetAs(&ledger))
			d = ledger.Partition(PartitionUrl("BVN0")).Delivered
		})
		return d
	}

	// Traffic delivered and confirmed: the stream has no hole.
	for i := uint64(1); i <= 5; i++ {
		send(i)
		sim.Step()
	}
	for _, st := range sts {
		sim.StepUntil(
			Txn(st.TxID).Succeeds(),
			Txn(st.TxID).Produced().Succeeds())
	}
	d0 := delivered(0)
	t.Logf("BVN1's Delivered from BVN0 before the hole: %d", d0)

	// A hole every node shares at Delivered+1, and entries behind it that
	// every node receives and validates.
	const behind = 6
	hole.Store(d0 + 1)
	for i := uint64(6); i <= 6+behind; i++ {
		send(i)
		sim.Step()
	}
	sim.StepN(40)
	for i := 0; i < p.NodeCount(); i++ {
		st := status(i)
		t.Logf("node %d before the restart: %+v", i, st)
		require.Equal(t, d0, st.Delivered, "precondition: the hole holds every node's stream")
		require.Equal(t, d0+1, st.Waiting, "precondition: every node waits on the same hole")
		require.GreaterOrEqual(t, st.Held, behind, "precondition: entries are held behind the hole")
	}

	// One validator restarts and rejoins by the production join. Its staging
	// is memory and comes back empty.
	suppressLo.Store(d0 + 2)
	suppressHi.Store(d0 + 1 + behind)
	p.RestartNode(1)
	require.True(t, p.Joining(1))
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
	stepping.State = p.NodeJoinState(1)
	settler, ok := p.NodeExecutor(1).(join.Settler)
	require.True(t, ok)
	_, err := join.Run(ctx, join.Options{
		Partition: "BVN1",
		Buffer:    p.NodeJoin(1),
		Stage:     &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(1), Database: p.NodeDatabase(1)},
		State:     stepping,
		Peers:     &join.APIPeers{Partition: "BVN1", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err, "the join did not complete within %d pull rounds", maxRounds)
	require.False(t, p.Joining(1), "the joined node executes")
	sim.StepN(5)
	for i := 0; i < p.NodeCount(); i++ {
		t.Logf("node %d after the handoff: %+v", i, status(i))
	}

	// The handoff was good: every node stands on the same root chain while
	// the shared hole holds every node's stream.
	rootChains := func() [][]byte {
		var anchors [][]byte
		for i := 0; i < p.NodeCount(); i++ {
			View(t, p.NodeDatabase(i), func(batch *database.Batch) {
				a, err := batch.Account(PartitionUrl("BVN1").JoinPath(Ledger)).RootChain().Anchor()
				require.NoError(t, err)
				anchors = append(anchors, a)
			})
		}
		return anchors
	}
	a := rootChains()
	require.Equal(t, a[0], a[1], "precondition: the rejoined node matches its peers after the handoff")

	// The hole fills on every node in the same block.
	release.Store(true)
	for b := 0; b < 10; b++ {
		sim.Step()
		t.Logf("step %d: block %d Delivered %d %d %d, held %d %d %d, heals(1) %+v", b, sim.S.BlockIndex("BVN1"),
			delivered(0), delivered(1), delivered(2), status(0).Held, status(1).Held, status(2).Held, p.NodeHeals(1))
	}

	a = rootChains()
	t.Logf("root chain anchors: %x / %x / %x", a[0][:4], a[1][:4], a[2][:4])
	require.Equal(t, delivered(0), delivered(1),
		"the rejoined node delivered a different run from BVN0 than its peers when the shared hole filled")
	for i := 1; i < len(a); i++ {
		require.Equal(t, a[0], a[i], "BVN1 node %d's root chain diverged from node 0's", i)
	}
}
