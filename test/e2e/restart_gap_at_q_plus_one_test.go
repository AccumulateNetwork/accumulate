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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	coreexec "gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
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

// Run 20260924T111811Z: every BVN restart found a gap at Q + 1 -- 3 of 3 --
// where the BVN restarts of runs 2 and 3 found none (0 of 3), and the gap line
// named neither stream nor number (#4432).
//
// A restart loses staging: it is memory by design. Whatever the node held
// unexecuted when it stopped -- a synthetic waiting on the anchor that proves
// it -- is gone, and it arrived before the node was listening again, so the
// node does not collect it again. Q + 1 then carries a later number on that
// stream and the first check finds a gap (executor spec, "Sync", step 4). A
// node restarted with nothing in flight held nothing and loses nothing.
//
// Every arm restarts BVN1 node 1 level with its peers, as the Docker restarts
// were, and joins through join.Run with the executor's stage over the node's
// own staging -- the production wiring. Every gap any arm finds at Q + 1 is on
// a stream BVN1 receives, at a number the restarted node itself held before
// it stopped and its peers held at the same moment: an entry from before the
// node was listening, which the gap check exists to catch.
//
//   - quiet: the traffic is drained first. Nothing held, no gap.
//   - traffic, anchors prompt: BVN0 sends to BVN1 through the restart and the
//     join, and the Directory anchor that proves each synthetic reaches BVN1
//     in the block the synthetic does, so it executes on arrival. Nothing is
//     held at the restart, and there is no gap -- traffic alone is not what
//     makes the gap.
//   - traffic, anchors lagging: the same, with every Directory anchor reaching
//     BVN1 anchorLag blocks after it was sent, as in Docker, where a synthetic
//     reaches its destination before the anchor that proves it. Entries are
//     held at every moment, so the restart loses some, and Q + 1 has a gap at
//     exactly those numbers. The lag is ten blocks: at three, the anchor that
//     proves a synthetic still lands no later than the synthetic, nothing is
//     held, and this arm finds no gap either.
//
// So whether a restart finds a gap at Q + 1 is whether it stopped holding an
// unexecuted entry, and that is the traffic's in-flight state at the moment
// of the restart. No message status enters it: HasGap reads staging and the
// pulled ledgers only (join/stage.go), and collecting writes and reads none
// (collect_block.go).
func TestRestartGapAtQPlusOneIsAnEntryHeldBeforeTheRestart(t *testing.T) {
	for _, c := range []struct {
		name      string
		traffic   bool
		anchorLag int
		wantGap   bool
	}{
		{"quiet restart", false, 0, false},
		{"traffic, anchors prompt", true, 0, false},
		{"traffic, anchors lagging", true, 10, true},
	} {
		t.Run(c.name, func(t *testing.T) { restartGapAtQPlusOne(t, c.traffic, c.anchorLag, c.wantGap) })
	}
}

// heldBefore is, per stream, the numbers a node's staging held and had not
// delivered.
type heldBefore map[string]map[uint64]bool

func heldNumbers(staging *coreexec.Staging) heldBefore {
	tx := staging.Begin()
	defer tx.Discard()
	held := heldBefore{}
	for _, st := range tx.Streams() {
		k := coreexec.StreamName(st.ID)
		held[k] = map[uint64]bool{}
		for n := st.Delivered + 1; n <= st.Sighted; n++ {
			if _, ok := tx.IDOf(st.ID, n); ok {
				held[k][n] = true
			}
		}
	}
	return held
}

// gapRecorder is the executor's stage, recording every gap check's answer.
type gapRecorder struct {
	join.Stage
	checks []gapCheck
}

type gapCheck struct {
	block uint64
	gaps  []join.StreamGap
}

func (r *gapRecorder) HasGap(block uint64) ([]join.StreamGap, error) {
	gaps, err := r.Stage.HasGap(block)
	if err == nil {
		r.checks = append(r.checks, gapCheck{block, gaps})
	}
	return gaps, err
}

func restartGapAtQPlusOne(t *testing.T, traffic bool, anchorLag int, wantGap bool) {
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

	// Each BVN1 node receives every Directory anchor anchorLag of its own
	// blocks late. Per node, so every node, the restarted one included, sees
	// the same blocks.
	if anchorLag > 0 {
		var mu sync.Mutex
		type late struct {
			at  int
			env *messaging.Envelope
		}
		blocks := map[int]int{}
		pending := map[int][]late{}
		sim.S.SetNodeBlockHook("BVN1", func(node int, _ coreexec.BlockParams, envelopes []*messaging.Envelope) ([]*messaging.Envelope, bool) {
			mu.Lock()
			defer mu.Unlock()
			blocks[node]++
			now := blocks[node]
			var kept, due []*messaging.Envelope
			rest := pending[node][:0]
			for _, l := range pending[node] {
				if l.at <= now {
					due = append(due, l.env)
				} else {
					rest = append(rest, l)
				}
			}
			pending[node] = rest
			for _, env := range envelopes {
				if isDirectoryAnchor(env) {
					pending[node] = append(pending[node], late{now + anchorLag, env})
					continue
				}
				kept = append(kept, env)
			}
			return append(due, kept...), true
		})
	}

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
	// Two sends a block: BVN0 produces synthetics to BVN1 every block, and
	// each waits on BVN1 for the Directory anchor that proves it.
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
	if traffic {
		// Long enough that what BVN0 sends now has reached BVN1 by the
		// restart: a synthetic lands some blocks after its send.
		step(20, true)
	} else {
		step(30, false)
	}

	p := sim.S.Partition("BVN1")
	ownBefore := heldNumbers(p.NodeStaging(1))
	peersBefore := heldNumbers(p.NodeStaging(0))
	stoodAt := partitionBlock(t, p.NodeDatabase(1), PartitionUrl("BVN1"))
	require.Equal(t, partitionBlock(t, p.NodeDatabase(0), PartitionUrl("BVN1")), stoodAt,
		"precondition: the node restarts level with its peers, as in the run")

	p.RestartNode(1)
	require.True(t, p.Joining(1))
	step(5, traffic)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const maxRounds = 200
	stepping := &steppingState{cancel: cancel, step: func(round int) {
		step(3, traffic)
		if round >= maxRounds {
			cancel()
		}
	}}
	stepping.State = p.NodeJoinState(1)
	settler, ok := p.NodeExecutor(1).(join.Settler)
	require.True(t, ok)
	stage := &gapRecorder{Stage: &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(1), Database: p.NodeDatabase(1)}}
	rec := &handoffRecorder{Buffer: p.NodeJoin(1)}
	_, err := join.Run(ctx, join.Options{
		Partition: "BVN1",
		Buffer:    rec,
		Stage:     stage,
		State:     stepping,
		Peers:     &join.APIPeers{Partition: "BVN1", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err, "the join did not complete within %d pull rounds", maxRounds)
	require.NotEmpty(t, stage.checks)

	first := stage.checks[0]
	for _, c := range stage.checks {
		t.Logf("gap check at %d: %v", c.block, c.gaps)
	}
	t.Logf("stood at %d; first check at %d; handed off at %d", stoodAt, first.block, rec.q)
	for k, held := range ownBefore {
		t.Logf("held by the node before it stopped: %s %d entries", k, len(held))
	}

	lost := 0
	for _, held := range ownBefore {
		lost += len(held)
	}
	if !wantGap {
		require.Zero(t, lost, "precondition: nothing is held unexecuted at the restart")
		require.Empty(t, first.gaps, "nothing was held at the restart, so nothing was lost and Q + 1 has no gap")
		return
	}

	require.NotZero(t, lost, "precondition: entries are held unexecuted at the restart")
	require.Equal(t, stoodAt+1, first.block,
		"precondition, as in the run: the node matched its own state and the gap is at Q + 1")
	require.NotEmpty(t, first.gaps, "the node lost what it held, and Q + 1 carries a number above it")
	for _, g := range first.gaps {
		k := g.StreamName()
		require.True(t, g.Stream.Ledger.Equal(PartitionUrl("BVN1").JoinPath(Synthetic)) ||
			g.Stream.Ledger.Equal(PartitionUrl("BVN1").JoinPath(AnchorPool)),
			"%v is on a stream BVN1 receives", g)
		for n := g.Missing; n <= g.MissingTo; n++ {
			require.True(t, ownBefore[k][n],
				"%v: %d was held by the restarted node before it stopped, and lost with its staging", g, n)
			require.True(t, peersBefore[k][n],
				"%v: %d was held by its peers at the same moment: an entry from before the node was listening", g, n)
		}
	}
}

// isDirectoryAnchor is whether an envelope carries a Directory anchor.
func isDirectoryAnchor(env *messaging.Envelope) bool {
	for _, m := range env.Messages {
		blk, ok := m.(*messaging.BlockAnchor)
		if !ok {
			continue
		}
		seq, ok := blk.Anchor.(*messaging.SequencedMessage)
		if !ok {
			continue
		}
		txn, ok := seq.Message.(*messaging.TransactionMessage)
		if ok && txn.Transaction.Body.Type() == TransactionTypeDirectoryAnchor {
			return true
		}
	}
	return false
}
