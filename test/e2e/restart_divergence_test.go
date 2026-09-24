// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// One validator restarts while its peers continue. Run 20260917T223150Z:
// every restarted validator signed a different anchor body from its first
// anchor after the restart and on every anchor after, so each restart took
// one validator out of the anchor quorum, and the fifth Directory restart
// took the majority below the threshold and nothing executed anywhere
// again. A pause did not do this (#4290).
//
// The suspect is staging: memory by design, not rebuilt from the store. A
// restarted node loses the entries it held unexecuted -- received, waiting
// on a proof -- and its peers execute them at block N when the proof lands
// while it executes them only once healing has brought them back. Different
// state at N, different root at N, and the root chain differs forever after.
//
// This stands one BVN1 validator where a restarted one stands: its staging
// emptied while its peers keep what they hold, with entries held on all of
// them at that moment. Then the anchors that prove those entries arrive, and
// the three nodes' root chains are compared.
func itoa(v uint64) string { return fmt.Sprint(v) }

// The proving anchor lands AFTER the join: the node takes its peers' staging
// and their state while they still hold the entries and their proofs wait, and
// when the anchor arrives every node — the one that joined included — executes
// what it holds. A join that took no staging holds nothing then, executes a
// block its peers do not, and its root chain never matches again (#4290, run
// 20260918T023054Z).
func TestOneValidatorRestartDoesNotDiverge(t *testing.T) {
	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	bobKey := acctesting.GenerateKey(bob)

	// A node that is joining executes nothing, so it reports no results for
	// the blocks it collects, and the simulator's per-block result comparison
	// would call that a consensus failure. It is not: the node is not
	// executing on purpose. What matters is the state it reaches, and that is
	// what this test ends by comparing — every node's root chain, which is a
	// Merkle root over the history of block roots and so compares every block
	// any of them executed.
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.SetRoute(alice, "BVN0")
	sim.SetRoute(bob, "BVN1")

	// Directory anchors are held back from BVN1 while the switch is thrown --
	// per node, so every node sees the same thing -- and released all at once
	// afterwards. Delayed, not dropped: the anchors that prove the held
	// entries then land on every node in one block, so the peers execute
	// what they hold right then, and the restarted node, holding nothing,
	// cannot. (Dropping them instead made every node heal them back on the
	// same activation, which coincided with the restarted node healing its
	// entries, and hid the divergence.)
	var hookMu sync.Mutex
	var dropAnchors, release atomic.Bool
	delayed := map[int][]*messaging.Envelope{}
	sim.S.SetNodeBlockHook("BVN1", func(node int, _ execute.BlockParams, envelopes []*messaging.Envelope) ([]*messaging.Envelope, bool) {
		hookMu.Lock()
		defer hookMu.Unlock()
		var kept []*messaging.Envelope
		for _, env := range envelopes {
			isAnchor := false
			for _, m := range env.Messages {
				blk, ok := m.(*messaging.BlockAnchor)
				if !ok {
					continue
				}
				if seq, ok := blk.Anchor.(*messaging.SequencedMessage); ok {
					if txn, ok := seq.Message.(*messaging.TransactionMessage); ok && txn.Transaction.Body.Type() == TransactionTypeDirectoryAnchor {
						isAnchor = true
					}
				}
			}
			if isAnchor && dropAnchors.Load() {
				delayed[node] = append(delayed[node], env)
				continue
			}
			kept = append(kept, env)
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

	// Traffic with the Directory answering, delivered and confirmed, so the
	// stream has no hole. Then without: what BVN1 receives from here is held
	// unproven on every one of its nodes, contiguous above Delivered.
	for i := uint64(1); i <= 5; i++ {
		send(i)
		sim.Step()
	}
	for _, st := range sts {
		sim.StepUntil(
			Txn(st.TxID).Succeeds(),
			Txn(st.TxID).Produced().Succeeds())
	}
	dropAnchors.Store(true)
	for i := uint64(6); i <= 17; i++ {
		send(i)
		sim.Step()
	}
	// Keep the anchors held back until the Directory anchor that proves the
	// held entries is among them: released together, the peers validate the
	// proofs and execute what they hold in the very next block, long before
	// healing's first probe (probeAfter activations of healCadence blocks).
	// That is the soak's timing -- a validator restarted under load meets the
	// proving anchor within seconds (#4290, run 20260917T223150Z).
	sim.StepN(40)
	p := sim.S.Partition("BVN1")
	require.Equal(t, 3, p.NodeCount())
	held := func(i int) int {
		tx := p.NodeStaging(i).Begin()
		defer tx.Discard()
		n := 0
		for _, st := range tx.Streams() {
			n += int(st.Held)
		}
		return n
	}
	before := []int{held(0), held(1), held(2)}
	t.Logf("held before the restart: %v", before)
	require.Greater(t, before[1], 0, "precondition: BVN1's nodes hold unproven entries when one of them restarts")
	{
		tx := p.NodeStaging(0).Begin()
		blocks := tx.ProofBlocks(PartitionUrl("BVN0"))
		tx.Discard()
		hookMu.Lock()
		var queued []uint64
		for _, env := range delayed[0] {
			for _, m := range env.Messages {
				if blk, ok := m.(*messaging.BlockAnchor); ok {
					if seq, ok := blk.Anchor.(*messaging.SequencedMessage); ok {
						if txn, ok := seq.Message.(*messaging.TransactionMessage); ok {
							queued = append(queued, txn.Transaction.Body.(*DirectoryAnchor).MinorBlockIndex)
						}
					}
				}
			}
		}
		hookMu.Unlock()
		t.Logf("proofs wait for Directory blocks %v; Directory anchors held back: %v", blocks, queued)
		require.NotEmpty(t, blocks, "precondition: the peers hold proofs waiting for their anchor")
		require.NotEmpty(t, queued)
		require.GreaterOrEqual(t, queued[len(queued)-1], blocks[len(blocks)-1], "precondition: the anchor that proves the held entries is among those held back")
	}

	var delivered uint64
	View(t, p.NodeDatabase(0), func(batch *database.Batch) {
		var ledger *SyntheticLedger
		require.NoError(t, batch.Account(PartitionUrl("BVN1").JoinPath(Synthetic)).Main().GetAs(&ledger))
		delivered = ledger.Partition(PartitionUrl("BVN0")).Delivered
	})
	t.Logf("BVN1's Delivered from BVN0 before the restart: %d", delivered)
	require.Equal(t, uint64(5), delivered, "precondition: the first batch was delivered before anchors were dropped")
	{
		tx := p.NodeStaging(0).Begin()
		stream := execute.StreamID{Ledger: PartitionUrl("BVN1").JoinPath(Synthetic), Source: PartitionUrl("BVN0")}
		for n := delivered + 1; n <= delivered+uint64(before[0]); n++ {
			_, ok := tx.IDOf(stream, n)
			require.True(t, ok, "precondition: entry %d is held on the peers, contiguous above Delivered -- no hole", n)
		}
		tx.Discard()
	}

	// One validator restarts: its staging is gone, its peers' is not. From
	// here it collects the blocks it is handed and executes none of them, and
	// it comes back by the join path — never by replaying what it missed or
	// by rebuilding staging from a source's cache (executor spec, "Sync";
	// #4294).
	p.RestartNode(1)
	require.True(t, p.Joining(1))
	t.Logf("held after the restart:  %v", []int{held(0), held(1), held(2)})

	// The join happens FIRST, while the peers still hold the entries and
	// their proofs still wait: the node takes their staging and their state,
	// and only then do the anchors land. That ordering is what this test is
	// for — with it, a join that skipped the staging (or collected nothing)
	// holds nothing when the proving anchor arrives, executes a block its
	// peers do not, and its root chain never matches again (#4290).
	require.NoError(t, p.TakeStaging(1, 0), "take a peer's staging")
	{
		tx := p.NodeStaging(1).Begin()
		n := 0
		for _, st := range tx.Streams() {
			n += int(st.Held)
		}
		tx.Discard()
		require.Greater(t, n, 0, "the staging taken holds what the peers hold")
	}
	// Blocks pass between the two halves of the join, and they carry new
	// entries: sent now, they reach BVN1 after the snapshot was taken, so
	// they are in nobody's snapshot and the joining node can only have them
	// by COLLECTING them. Their anchors are still held back, so the peers
	// hold them unexecuted — which is what makes the difference visible when
	// the anchors land.
	for i := uint64(18); i <= 20; i++ {
		send(i)
		sim.Step()
	}
	sim.StepN(10)
	t.Logf("held while joining:      %v", []int{held(0), held(1), held(2)})
	require.Greater(t, held(1), 0, "the joining node collected what arrived after the snapshot")

	require.NoError(t, p.CompleteJoin(1, 0), "take the state and execute from the next block")
	require.False(t, p.Joining(1))
	t.Logf("held after the join:     %v", []int{held(0), held(1), held(2)})
	require.Equal(t, held(0), held(1), "the joined node holds what its peers hold")

	// The held-back anchors land on every node in the next block: every node,
	// the one that joined included, executes what it holds.
	dropAnchors.Store(false)
	release.Store(true)
	proofs := func() string {
		mfs, err := prometheus.DefaultGatherer.Gather()
		require.NoError(t, err)
		out := ""
		for _, mf := range mfs {
			if mf.GetName() != "accumulate_exec_staged_proofs_total" {
				continue
			}
			for _, m := range mf.GetMetric() {
				for _, l := range m.GetLabel() {
					if l.GetName() == "outcome" {
						out += fmt.Sprintf(" %s=%.0f", l.GetValue(), m.GetCounter().GetValue())
					}
				}
			}
		}
		return out
	}
	for step := 0; step < 30; step++ {
		require.NoError(t, sim.S.Step())

		var line string
		for i := 0; i < p.NodeCount(); i++ {
			tx := p.NodeStaging(i).Begin()
			for _, st := range tx.Streams() {
				if st.ID.Source.Equal(PartitionUrl("BVN0")) && st.ID.Ledger.Equal(PartitionUrl("BVN1").JoinPath(Synthetic)) {
					line += fmt.Sprintf(" n%d:syn(held=%d)", i, st.Held)
				}
				if st.ID.Source.Equal(DnUrl()) && st.ID.Ledger.Equal(PartitionUrl("BVN1").JoinPath(AnchorPool)) {
					line += fmt.Sprintf(" n%d:anc(held=%d,sighted=%d)", i, st.Held, st.Sighted)
				}
			}
			tx.Discard()
		}
		t.Logf("block %d:%s | proofs%s", sim.S.BlockIndex("BVN1"), line, proofs())
	}
	for _, st := range sts {
		sim.StepUntil(
			Txn(st.TxID).Succeeds(),
			Txn(st.TxID).Produced().Succeeds())
	}
	sim.StepN(20)
	t.Logf("held at the end:         %v", []int{held(0), held(1), held(2)})

	// Every node of BVN1 must stand on the same root chain, or the anchors
	// it signs from here on will never gather a quorum.
	var anchors [][]byte
	for i := 0; i < p.NodeCount(); i++ {
		View(t, p.NodeDatabase(i), func(batch *database.Batch) {
			a, err := batch.Account(PartitionUrl("BVN1").JoinPath(Ledger)).RootChain().Anchor()
			require.NoError(t, err)
			anchors = append(anchors, a)
		})
	}
	for i := 1; i < len(anchors); i++ {
		require.Equal(t, anchors[0], anchors[i], "BVN1 node %d's root chain diverged from node 0's after a restart", i)
	}
}
