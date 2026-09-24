// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"math/big"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/database/merkle"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// proofBudgetNet is a two-BVN network sending BVN0 -> BVN1 synthetics as
// packages (a shared SyntheticProof and proof-less members), with every
// Directory anchor kept from BVN1 while `holding` is set, so each package's
// proof waits in anchor staging under the budget (executor spec, "Anchor
// staging").
type proofBudgetNet struct {
	sim     *Sim
	p       *simulator.Partition
	stream  execute.StreamID
	holding atomic.Bool
	sent    []*TransactionStatus
	send    func() // two deposits to BVN1 in the next block: one package
	ledger  func(i int) PartitionSyntheticLedger
	root    func(i int) [32]byte
}

func newProofBudgetNet(t *testing.T) *proofBudgetNet {
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

	n := &proofBudgetNet{
		sim:    sim,
		p:      sim.S.Partition("BVN1"),
		stream: execute.StreamID{Ledger: PartitionUrl("BVN1").JoinPath(Synthetic), Source: PartitionUrl("BVN0")},
	}

	// Every Directory anchor addressed to BVN1 -- first copies and heals
	// alike -- is kept from every node of BVN1 while holding. Every node
	// loses the same messages, so what differs between nodes is only what
	// the test plants in one node's staging.
	sim.S.SetNodeBlockHook("BVN1", func(_ int, _ execute.BlockParams, envelopes []*messaging.Envelope) ([]*messaging.Envelope, bool) {
		if !n.holding.Load() {
			return envelopes, true
		}
		var kept []*messaging.Envelope
		for _, env := range envelopes {
			var rest []messaging.Message
			touched := false
			for _, m := range env.Messages {
				if a, ok := m.(*messaging.BlockAnchor); ok {
					if seq, ok := a.Anchor.(*messaging.SequencedMessage); ok && seq.Source != nil && seq.Source.Equal(DnUrl()) {
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
	CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	MakeAccount(t, sim.DatabaseFor(alice), &TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: AcmeUrl()})
	CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(1000))
	MakeIdentity(t, sim.DatabaseFor(bob), bob, bobKey[32:])
	MakeAccount(t, sim.DatabaseFor(bob), &TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: AcmeUrl()})

	var ts uint64
	n.send = func() {
		for k := 0; k < 2; k++ {
			ts++
			n.sent = append(n.sent, sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(alice, "tokens").
					SendTokens(1, 0).To(bob, "tokens").
					SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey)))
		}
		sim.Step()
	}
	n.ledger = func(i int) PartitionSyntheticLedger {
		synth, _ := streamLedgers(t, n.p.NodeDatabase(i), "BVN1")
		return synth[PartitionUrl("BVN0").String()]
	}
	n.root = func(i int) [32]byte {
		var r [32]byte
		View(t, n.p.NodeDatabase(i), func(b *database.Batch) {
			var err error
			r, err = b.GetBptRootHash()
			require.NoError(t, err)
		})
		return r
	}

	// Traffic delivered first, so the stream exists on every node and the
	// packages below are its continuation.
	n.send()
	for _, st := range n.sent {
		sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
	}
	return n
}

// requireAgree checks that every node of BVN1 records the same Received and
// Delivered on the stream and has the same state root.
func (n *proofBudgetNet) requireAgree(t *testing.T, when string) PartitionSyntheticLedger {
	t.Helper()
	l0, r0 := n.ledger(0), n.root(0)
	for i := 1; i < n.p.NodeCount(); i++ {
		l := n.ledger(i)
		require.Equal(t, l0.Received, l.Received, "%s: node %d's Received differs from node 0's", when, i)
		require.Equal(t, l0.Delivered, l.Delivered, "%s: node %d's Delivered differs from node 0's", when, i)
		require.Equal(t, r0, n.root(i), "%s: node %d's state root differs from node 0's", when, i)
	}
	return l0
}

// #4439. The budget that bounds anchor staging is read from staging's memory,
// and a restarted or joining node's staged proofs are not its peers'. Here one
// node's staging holds proof bytes its peers' does not -- planted, standing
// for whatever a restart left it with -- so that node alone is over budget
// when the next packages' proofs arrive. It drops their proofs; its peers
// stage them. Received is hashed state (#4412), so it must still count every
// entry consensus delivered on that node exactly as on its peers: the budget
// bounds memory, it does not decide what was received (Paul, 2026-09-24,
// note_3900507570).
func TestProofBudgetDoesNotDecideReceived(t *testing.T) {
	restore := execute.MaxStagedProofBytes
	t.Cleanup(func() { execute.MaxStagedProofBytes = restore })

	n := newProofBudgetNet(t)
	d0 := n.requireAgree(t, "before").Delivered
	require.NotZero(t, d0)

	// Node 1's staging holds a megabyte of proof from BVN0 its peers do not,
	// waiting on a Directory block that never executes in this test. It
	// covers no number (no merkle state), so nothing but the budget reads it.
	const budget = 1 << 20
	planted := &AnnotatedReceipt{
		ReceiptList: &merkle.ReceiptList{},
		Anchor:      &AnchorMetadata{Account: DnUrl(), SourceBlock: 1 << 40},
	}
	for i := 0; i < budget/32; i++ {
		planted.ReceiptList.Elements = append(planted.ReceiptList.Elements, make([]byte, 32))
	}
	txn := n.p.NodeStaging(1).Begin()
	require.True(t, txn.StageProof(n.stream.Source, planted.Anchor.SourceBlock, planted))
	txn.Commit()
	execute.MaxStagedProofBytes = budget
	require.GreaterOrEqual(t, n.p.NodeStaging(1).Begin().StagedProofBytes(n.stream.Source), budget,
		"precondition: node 1 is over the budget")
	require.Less(t, n.p.NodeStaging(0).Begin().StagedProofBytes(n.stream.Source), budget,
		"precondition: node 0 is not")

	n.holding.Store(true)
	const packages = 3
	for k := 0; k < packages; k++ {
		n.send()
		n.requireAgree(t, "while the packages arrive")
	}
	n.sim.StepN(20)
	l := n.requireAgree(t, "with the packages held")

	want := d0 + 2*packages
	require.Equal(t, d0, l.Delivered, "precondition: nothing is delivered while the anchors are kept away")
	for i := 0; i < n.p.NodeCount(); i++ {
		require.Equal(t, want, n.ledger(i).Received,
			"node %d: Received counts every entry consensus delivered, whatever the node's proof budget", i)
		require.Equal(t, want, n.p.NodeStaging(i).SightedOn(n.stream), "node %d holds every entry", i)
	}
	require.Equal(t, 1, len(n.p.NodeStaging(1).Begin().ProofBlocks(n.stream.Source)),
		"precondition: node 1 dropped the packages' proofs (only the planted one waits)")
	require.NotEmpty(t, n.p.NodeStaging(0).Begin().ProofBlocks(n.stream.Source),
		"precondition: node 0 staged them")
}

// #4439, #4282. When the budget binds, a package's proof is dropped and its
// entries are held and counted. Nothing re-sends a proof on its own, so the
// entries must not be stranded waiting for the one that was dropped: once
// their stream stops at them they are a gap of proof, the requester asks the
// source for the span, the answer brings the proof back through consensus,
// and the entries execute (healing spec, "Deciding, in staging"). The budget
// binds identically on every node here, so they must also agree throughout.
func TestADroppedProofIsFetchedAndTheEntryExecutes(t *testing.T) {
	restore := execute.MaxStagedProofBytes
	t.Cleanup(func() { execute.MaxStagedProofBytes = restore })

	n := newProofBudgetNet(t)
	d0 := n.requireAgree(t, "before").Delivered

	// One staged proof already costs the whole budget: the first package's
	// proof is staged (the budget is measured before a proof, not after),
	// every later one is dropped.
	execute.MaxStagedProofBytes = 1
	n.holding.Store(true)
	const packages = 3
	for k := 0; k < packages; k++ {
		n.send()
		n.requireAgree(t, "while the packages arrive")
	}
	n.sim.StepN(10)
	want := d0 + 2*packages
	for i := 0; i < n.p.NodeCount(); i++ {
		require.Equal(t, want, n.ledger(i).Received, "node %d: the entries of a package whose proof was dropped are held and counted", i)
		require.Len(t, n.p.NodeStaging(i).Begin().ProofBlocks(n.stream.Source), 1,
			"precondition: node %d staged the first package's proof and dropped the rest", i)
	}

	// The anchors come back. The first package executes on its staged
	// proof; the others wait for a proof, and the requester fetches it.
	healed0 := healEntriesTo(t, "BVN1")
	n.holding.Store(false)
	for _, st := range n.sent {
		n.sim.StepUntilN(600, Txn(st.TxID).Produced().Succeeds())
	}
	for i := 0; i < n.p.NodeCount(); i++ {
		require.Equal(t, want, n.ledger(i).Delivered, "node %d: every entry of every package executed", i)
	}
	n.requireAgree(t, "after the fetch")
	require.Greater(t, healEntriesTo(t, "BVN1"), healed0,
		"the entries whose proof was dropped executed on a proof the requester fetched")
}

// healEntriesTo is how many entries span requests have brought to a
// partition, whatever became of them (accumulate_conductor_heal_entries_total).
func healEntriesTo(t *testing.T, partition string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	var n float64
	for _, f := range families {
		if f.GetName() != "accumulate_conductor_heal_entries_total" {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "destination" && strings.EqualFold(l.GetValue(), partition) {
					n += m.GetCounter().GetValue()
				}
			}
		}
	}
	return n
}
