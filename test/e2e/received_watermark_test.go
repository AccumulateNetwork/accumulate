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

// streamLedgers reads, from one node's store, the synthetic and anchor
// ledgers of a partition: source -> the stream's ledger entry.
func streamLedgers(t *testing.T, db *database.Database, partition string) (synth, anchors map[string]PartitionSyntheticLedger) {
	t.Helper()
	synth, anchors = map[string]PartitionSyntheticLedger{}, map[string]PartitionSyntheticLedger{}
	View(t, db, func(batch *database.Batch) {
		var sl *SyntheticLedger
		require.NoError(t, batch.Account(PartitionUrl(partition).JoinPath(Synthetic)).Main().GetAs(&sl))
		for _, p := range sl.Sequence {
			synth[p.Url.String()] = *p
		}
		var al *AnchorLedger
		require.NoError(t, batch.Account(PartitionUrl(partition).JoinPath(AnchorPool)).Main().GetAs(&al))
		for _, p := range al.Sequence {
			anchors[p.Url.String()] = *p
		}
	})
	return synth, anchors
}

// sharedHole is a two-BVN network in which the BVN0 -> BVN1 synthetic stream
// has a hole every node of BVN1 shares, with entries held behind it on every
// node -- the shape of run 20260924T074702Z's BVN1 -> Directory stream, and of
// the #4412 repro (branch repro-4412, restart_prelisten_hole_test.go).
type sharedHole struct {
	sim     *Sim
	p       *simulator.Partition
	stream  execute.StreamID
	d0      uint64 // Delivered before the hole
	want    uint64 // the highest number held behind it
	release func()
	// suppress discards, from here on, every copy of the numbers behind the
	// hole: what a restarted node's own heal has not yet brought it
	suppress func()
	ledger   func(i int) PartitionSyntheticLedger
}

func newSharedHole(t *testing.T) *sharedHole {
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

	// Every copy of the hole's number (the original and every heal of it) is
	// held back on every node until release, as the run's 740 was. The hook
	// filters messages, not envelopes: a heal envelope carries a span.
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

	h := &sharedHole{
		sim:    sim,
		p:      sim.S.Partition("BVN1"),
		stream: execute.StreamID{Ledger: PartitionUrl("BVN1").JoinPath(Synthetic), Source: PartitionUrl("BVN0")},
	}
	h.ledger = func(i int) PartitionSyntheticLedger {
		synth, _ := streamLedgers(t, h.p.NodeDatabase(i), "BVN1")
		return synth[PartitionUrl("BVN0").String()]
	}

	// Traffic delivered: the stream has no hole, and what arrived is what
	// was delivered.
	for i := uint64(1); i <= 5; i++ {
		send(i)
		sim.Step()
	}
	for _, st := range sts {
		sim.StepUntil(
			Txn(st.TxID).Succeeds(),
			Txn(st.TxID).Produced().Succeeds())
	}
	h.d0 = h.ledger(0).Delivered
	require.NotZero(t, h.d0)
	for i := 0; i < h.p.NodeCount(); i++ {
		l := h.ledger(i)
		require.Equal(t, h.d0, l.Delivered, "node %d", i)
		require.Equal(t, h.d0, l.Received, "node %d: with no hole, Received is Delivered", i)
	}

	// A hole every node shares at Delivered+1, and entries behind it.
	const behind = 6
	hole.Store(h.d0 + 1)
	for i := uint64(6); i <= 6+behind; i++ {
		send(i)
		sim.Step()
	}
	sim.StepN(40)
	h.want = h.d0 + 1 + behind
	for i := 0; i < h.p.NodeCount(); i++ {
		l := h.ledger(i)
		require.Equal(t, h.d0, l.Delivered, "precondition: node %d's stream waits on the hole", i)
		require.Equal(t, h.want, h.p.NodeStaging(i).SightedOn(h.stream), "precondition: node %d holds through %d", i, h.want)
	}

	h.release = func() { release.Store(true) }
	h.suppress = func() {
		suppressLo.Store(h.d0 + 2)
		suppressHi.Store(h.want)
	}
	return h
}

// #4412, option B (executor.md, "Sync", "The algorithm", step 6). Entries
// that arrive behind a hole every node shares are held, not delivered, so
// Delivered stays below the hole. The ledger's Received must still say how
// far the stream has arrived -- the highest number held -- and say it
// identically on every validator, because it is hashed state and it is what a
// joining node's staging is checked against.
func TestReceivedRecordsArrivalsBehindASharedHole(t *testing.T) {
	h := newSharedHole(t)
	for i := 0; i < h.p.NodeCount(); i++ {
		l := h.ledger(i)
		t.Logf("node %d: delivered=%d received=%d sighted=%d", i, l.Delivered, l.Received, h.p.NodeStaging(i).SightedOn(h.stream))
		require.Equal(t, h.want, l.Received, "node %d: the ledger's Received is the highest number that arrived", i)
	}

	// The hole fills; the stream drains; Received stays where it was and
	// Delivered reaches it.
	h.release()
	h.sim.StepN(10)
	for i := 0; i < h.p.NodeCount(); i++ {
		l := h.ledger(i)
		require.Equal(t, h.want, l.Delivered, "node %d delivered the run", i)
		require.Equal(t, h.want, l.Received, "node %d", i)
	}
}

// #4412: a validator that restarts behind the shared hole comes back with an
// empty staging -- it never sees the entries its peers hold, because they
// were received before it was listening. Its ledger's Received is the
// peers', because it is state it pulled, and every block after the handoff
// writes the same value on it as on its peers, because the value is counted
// from the block and never read from staging. That is what the join's
// consistency check (step 6) stands on.
func TestReceivedIsThePeersOnARejoinedNodeWhoseStagingIsNot(t *testing.T) {
	h := newSharedHole(t)
	p := h.p

	h.suppress()
	p.RestartNode(1)
	require.True(t, p.Joining(1))
	h.sim.StepN(5)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const maxRounds = 100
	stepping := &steppingState{cancel: cancel, step: func(round int) {
		h.sim.StepN(3)
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
		Peers:     &join.APIPeers{Partition: "BVN1", Client: h.sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err, "the join did not complete within %d pull rounds", maxRounds)
	require.False(t, p.Joining(1), "the joined node executes")
	h.sim.StepN(5)

	require.Less(t, p.NodeStaging(1).SightedOn(h.stream), h.want,
		"precondition: the rejoined node's staging never saw what its peers hold")
	for i := 0; i < p.NodeCount(); i++ {
		l := h.ledger(i)
		t.Logf("node %d after the handoff: delivered=%d received=%d sighted=%d", i, l.Delivered, l.Received, p.NodeStaging(i).SightedOn(h.stream))
		require.Equal(t, h.want, l.Received, "node %d: Received is the stream's, not this node's staging's", i)
	}

	// The hole fills. Whatever the rejoined node's run does (the join side of
	// #4412 is what makes it hold the peers' entries), Received is the same
	// on every node at every block.
	h.release()
	for b := 0; b < 10; b++ {
		h.sim.Step()
		r0 := h.ledger(0).Received
		for i := 1; i < p.NodeCount(); i++ {
			require.Equal(t, r0, h.ledger(i).Received, "step %d: node %d's Received differs from node 0's", b, i)
		}
	}
}

// #4412: Received never falls below Delivered and never decreases, on any
// stream -- synthetic or anchor -- of any partition, at any block of a
// traffic run, and every validator of a partition records the same value.
func TestReceivedIsAtLeastDeliveredAtEveryBlock(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 3),
		simulator.Genesis(GenesisTime),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")

	MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	CreditCredits(t, sim.DatabaseFor(bob), bob.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(bob), bob.JoinPath("tokens"), big.NewInt(1000))

	type key struct{ partition, ledger, source string }
	last := map[key]uint64{}
	sawNonZero := map[string]bool{}
	check := func() {
		t.Helper()
		for _, info := range sim.S.Partitions() {
			part := sim.S.Partition(info.ID)
			var first [2]map[string]PartitionSyntheticLedger
			for i := 0; i < part.NodeCount(); i++ {
				synth, anchors := streamLedgers(t, part.NodeDatabase(i), info.ID)
				for li, m := range []map[string]PartitionSyntheticLedger{synth, anchors} {
					name := [2]string{"synthetic", "anchors"}[li]
					for src, l := range m {
						require.GreaterOrEqual(t, l.Received, l.Delivered,
							"%s node %d %s from %s: Received below Delivered", info.ID, i, name, src)
						if l.Received > 0 {
							sawNonZero[name] = true
						}
						if i == 0 {
							k := key{info.ID, name, src}
							require.GreaterOrEqual(t, l.Received, last[k],
								"%s %s from %s: Received decreased", info.ID, name, src)
							last[k] = l.Received
						}
					}
					if i == 0 {
						first[li] = m
						continue
					}
					for src, l := range m {
						require.Equal(t, first[li][src].Received, l.Received,
							"%s node %d %s from %s: Received differs from node 0's", info.ID, i, name, src)
					}
				}
			}
		}
	}

	for i := uint64(1); i <= 20; i++ {
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(i).PrivateKey(aliceKey))
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(bob, "tokens").
				SendTokens(1, 0).To(alice, "tokens").
				SignWith(bob, "book", "1").Version(1).Timestamp(i).PrivateKey(bobKey))
		sim.Step()
		check()
	}
	for i := 0; i < 50; i++ {
		sim.Step()
		check()
	}
	require.True(t, sawNonZero["synthetic"], "the run delivered no synthetic stream")
	require.True(t, sawNonZero["anchors"], "the run delivered no anchor stream")
}

// #4412, review F1: the anchor stream. Directory anchors reach BVN1 with only
// one validator's copy each -- the other copies are withheld -- so every one
// of them stays below its signature quorum and is held, not executed. That is
// the hold #4432 found at every BVN restart of run 4. The anchor ledger's
// Received for the Directory must still say how far the anchors have arrived,
// above Delivered and identical on every node; only the anchor hold site
// (BlockAnchor.process) can count them, since nothing below quorum reaches
// the sequenced layer.
func TestReceivedCountsAnchorsHeldBelowQuorum(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)

	dn := DnUrl()
	anchorNumber := func(m messaging.Message) (*messaging.BlockAnchor, uint64, bool) {
		ba, ok := m.(*messaging.BlockAnchor)
		if !ok {
			return nil, 0, false
		}
		seq, ok := ba.Anchor.(*messaging.SequencedMessage)
		if !ok || seq.Source == nil || !seq.Source.Equal(dn) {
			return nil, 0, false
		}
		return ba, seq.Number, true
	}

	// From `from` on, one copy per anchor number is let through -- the copy
	// of whichever signer was seen first for that number, which every node
	// sees in the same block -- and every other copy, and every copy
	// authorized by a proof rather than a signature, is held back until
	// release.
	var hookMu sync.Mutex
	var from atomic.Uint64
	var release atomic.Bool
	keeper := map[uint64]string{}
	delayed := map[int][]*messaging.Envelope{}
	sim.S.SetNodeBlockHook("BVN1", func(node int, _ execute.BlockParams, envelopes []*messaging.Envelope) ([]*messaging.Envelope, bool) {
		hookMu.Lock()
		defer hookMu.Unlock()
		var kept []*messaging.Envelope
		for _, env := range envelopes {
			touched := false
			var rest []messaging.Message
			for _, m := range env.Messages {
				ba, n, ok := anchorNumber(m)
				if !ok || from.Load() == 0 || n < from.Load() || release.Load() {
					rest = append(rest, m)
					continue
				}
				var signer string
				if ba.Signature != nil && ba.Proof == nil {
					signer = string(ba.Signature.GetPublicKey())
					if _, seen := keeper[n]; !seen {
						keeper[n] = signer
					}
				}
				if signer != "" && keeper[n] == signer {
					rest = append(rest, m)
					continue
				}
				delayed[node] = append(delayed[node], &messaging.Envelope{Messages: []messaging.Message{m}})
				touched = true
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

	p := sim.S.Partition("BVN1")
	ledgerOf := func(i int) PartitionSyntheticLedger {
		_, anchors := streamLedgers(t, p.NodeDatabase(i), "BVN1")
		return anchors[dn.String()]
	}

	// Let the Directory anchor BVN1 a few times, then start withholding at
	// the next number.
	sim.StepN(20)
	d0 := ledgerOf(0).Delivered
	require.NotZero(t, d0, "precondition: BVN1 has executed Directory anchors")
	from.Store(d0 + 1)

	sim.StepN(20)
	var received uint64
	for i := 0; i < p.NodeCount(); i++ {
		l := ledgerOf(i)
		t.Logf("node %d: anchors from the Directory delivered=%d received=%d sighted=%d", i, l.Delivered, l.Received,
			p.NodeStaging(i).SightedOn(execute.StreamID{Ledger: PartitionUrl("BVN1").JoinPath(AnchorPool), Source: dn}))
		require.Equal(t, d0, l.Delivered, "precondition: node %d executed no anchor below its quorum", i)
		require.Greater(t, l.Received, l.Delivered, "node %d: anchors held below quorum are received, and Received says so", i)
		if i == 0 {
			received = l.Received
		}
		require.Equal(t, received, l.Received, "node %d's Received differs from node 0's", i)
	}

	// The withheld copies arrive; the anchors reach quorum and run.
	release.Store(true)
	sim.StepN(20)
	for i := 0; i < p.NodeCount(); i++ {
		l := ledgerOf(i)
		require.GreaterOrEqual(t, l.Delivered, received, "node %d ran the held anchors", i)
		require.GreaterOrEqual(t, l.Received, l.Delivered, "node %d", i)
	}
}
