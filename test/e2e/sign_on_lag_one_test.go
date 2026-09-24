// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"context"
	"crypto/ed25519"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/api/private"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/execute"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/database/record"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	. "gitlab.com/accumulatenetwork/accumulate/protocol"
	. "gitlab.com/accumulatenetwork/accumulate/test/harness"
	. "gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// These are executor spec, "Sync", §6, "Two things do not wait for ACTIVE"
// (#4443): a node answers for every anchor it executed and signed whatever
// its state, and signs and dispatches block N's anchor and synthetics only if
// the newest root it could check matched (lag one).
//
// WHAT IS DONE HERE BY HAND, AND WHY. Two things production does, the tests
// do for it:
//
//   - The root check. Whoever compares this node's root with the partition's
//     signed one reports to the node's execute.RootCheck; that is the join's
//     root watch, and it does not report yet (DIFFERENCES E20, and this
//     builder may not touch internal/node/join). The tests report the
//     outcome the watch would, through NodeRootCheck -- the very value the
//     node's conductor, executor and sequencer sign by.
//   - BOOTING while executing. A join that hands off unproven leaves the node
//     BOOTING and executing; a restarted simulator node collects instead, so
//     SetNodeMachine stands a running node in a fresh BOOTING machine, which
//     its production sequencer then refuses by.
//
// Everything else is production: the simulator's real conductors, executors,
// dispatchers and sequencers, reached through the per-peer routing.

// bvn0Signatures counts, per key, the anchors and synthetic messages from BVN0
// in a dispatched envelope.
type bvn0Signatures struct {
	mu        sync.Mutex
	anchors   map[string]int
	synthetic map[string]int
}

func (s *bvn0Signatures) capture(_ context.Context, env *messaging.Envelope) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	msgs, err := env.Normalize()
	if err != nil {
		return true, nil
	}
	fromBVN0 := func(u *url.URL) bool { return u != nil && PartitionUrl("BVN0").Equal(u.RootIdentity()) }
	for _, m := range msgs {
		switch m := m.(type) {
		case *messaging.BlockAnchor:
			if seq, ok := m.Anchor.(*messaging.SequencedMessage); ok && m.Signature != nil && fromBVN0(seq.Source) {
				s.anchors[string(m.Signature.GetPublicKey())]++
			}
		case *messaging.SyntheticMessage:
			if seq, ok := m.Message.(*messaging.SequencedMessage); ok && m.Signature != nil && fromBVN0(seq.Source) {
				s.synthetic[string(m.Signature.GetPublicKey())]++
			}
		}
	}
	return true, nil
}

func (s *bvn0Signatures) reset() (anchors, synthetic map[string]int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	anchors, synthetic = s.anchors, s.synthetic
	s.anchors, s.synthetic = map[string]int{}, map[string]int{}
	return anchors, synthetic
}

// A node whose last checked root did not match signs and dispatches nothing:
// no anchor, no synthetic (as the leader), and no answer from its sequencer.
// Before #4443 the conductor was gated on the committee alone and the leader's
// dispatch on nothing, so a node that knew its state was wrong went on
// putting its key on every root it computed.
func TestAMismatchedNodeSignsAndDispatchesNothing(t *testing.T) {
	captured := &bvn0Signatures{anchors: map[string]int{}, synthetic: map[string]int{}}
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 2, 4),
		simulator.Genesis(GenesisTime),
		simulator.CaptureDispatchedMessages(captured.capture),
	)

	// Alice on BVN0 sends to Bob on BVN1: BVN0's leader dispatches a
	// synthetic deposit for every send.
	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	sim.SetRoute(aliceUrl.RootIdentity(), "BVN0")
	bobUrl := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("Bob"))
	sim.SetRoute(bobUrl.RootIdentity(), "BVN1")
	MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(aliceUrl), aliceUrl.RootIdentity(), 1e9)
	CreditTokens(t, sim.DatabaseFor(aliceUrl), aliceUrl, big.NewInt(1e12))

	var ts uint64
	traffic := func(sends int) {
		for i := 0; i < sends; i++ {
			ts++
			st := sim.BuildAndSubmitTxnSuccessfully(
				build.Transaction().For(aliceUrl).
					SendTokens(1, 0).To(bobUrl).
					SignWith(aliceUrl).Version(1).Timestamp(ts).PrivateKey(alice))
			sim.StepUntil(Txn(st.TxID).Succeeds(), Txn(st.TxID).Produced().Succeeds())
			// A different number of blocks each time, so the dispatching
			// block falls to each validator in turn as leader.
			sim.StepN(1 + i%4)
		}
		sim.StepN(10)
	}

	bvn := sim.S.Partition("BVN0")
	const wrong = 1
	key := string(bvn.NodeConductor(wrong).ValidatorKey.Public().(ed25519.PublicKey))

	// Premise: before its check mismatched, this node signs BVN0 anchors and,
	// on the blocks it leads, dispatches BVN0 synthetics.
	traffic(12)
	anchors, synthetic := captured.reset()
	require.NotZero(t, anchors[key], "premise: node %d signed no BVN0 anchor", wrong)
	require.NotZero(t, synthetic[key], "premise: node %d dispatched no BVN0 synthetic", wrong)

	// Its root after the block it stands at did not match (reported by
	// hand; see the note above).
	var at uint64
	View(t, bvn.NodeDatabase(wrong), func(batch *database.Batch) {
		var ledger *SystemLedger
		require.NoError(t, batch.Account(PartitionUrl("BVN0").JoinPath(Ledger)).Main().GetAs(&ledger))
		at = ledger.Index
	})
	bvn.NodeRootCheck(wrong).Mismatched(at)

	traffic(12)
	anchors, synthetic = captured.reset()
	require.Zero(t, anchors[key], "node %d signed %d BVN0 anchors after its root mismatched", wrong, anchors[key])
	require.Zero(t, synthetic[key], "node %d dispatched %d BVN0 synthetics after its root mismatched", wrong, synthetic[key])

	// The others are unaffected: this is a gate on one node, not a switch.
	require.GreaterOrEqual(t, len(anchors), 3, "the other validators stopped anchoring")
	require.NotEmpty(t, synthetic, "no BVN0 synthetic was dispatched at all")

	// And its sequencer answers for nothing: every answer carries its key.
	n, ok := bvn.SynthCache().LastAnchorNumber()
	require.True(t, ok)
	_, err := bvn.NodePrivate(wrong).Sequence(context.Background(), PartitionUrl("BVN0").JoinPath(AnchorPool), DnUrl(), n, private.SequenceOptions{})
	require.ErrorIs(t, err, errors.NotReady)
	_, err = bvn.NodePrivate(0).Sequence(context.Background(), PartitionUrl("BVN0").JoinPath(AnchorPool), DnUrl(), n, private.SequenceOptions{})
	require.NoError(t, err, "control: a node whose root matched answers")
}

// Two nodes executing the same wrong block form no quorum on it.
//
// BVN0 has three validators, so two signatures are its threshold, and two
// nodes whose state is wrong in the same way compute the same wrong root:
// without lag one they sign it, the Directory holds a quorum for it, and
// executes a BVN0 anchor for a root BVN0 never had. With it, a node whose
// last checked root did not match signs nothing, and the partition stalls
// instead: the one right signature is no quorum either.
func TestTwoNodesExecutingTheSameWrongBlockFormNoQuorum(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
		// The two wrong nodes' results differ from the right one's; that is
		// the premise, not a failure of the simulator's consensus.
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.StepN(10)

	bvn := sim.S.Partition("BVN0")
	require.Equal(t, 3, bvn.NodeCount())
	require.Equal(t, uint64(2), bvn.NodeConductor(0).Globals.Load().ValidatorThreshold("BVN0"),
		"premise: two of three sign a BVN0 anchor")

	rootOf := func(i int) (root [32]byte, block uint64) {
		View(t, bvn.NodeDatabase(i), func(batch *database.Batch) {
			var ledger *SystemLedger
			require.NoError(t, batch.Account(PartitionUrl("BVN0").JoinPath(Ledger)).Main().GetAs(&ledger))
			r, err := batch.GetBptRootHash()
			require.NoError(t, err)
			root, block = r, ledger.Index
		})
		return root, block
	}

	// Nodes 1 and 2 go wrong the same way: one account neither the network
	// nor node 0 holds. Their check of the block they stand at mismatched.
	const wrong1, wrong2 = 1, 2
	stray := &LiteIdentity{Url: acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("stray")).RootIdentity()}
	for _, i := range []int{wrong1, wrong2} {
		require.NoError(t, bvn.NodeDatabase(i).Update(func(batch *database.Batch) error {
			return batch.Account(stray.Url).Main().Put(stray)
		}))
		_, at := rootOf(i)
		bvn.NodeRootCheck(i).Mismatched(at)
	}
	right, from := rootOf(0)
	w1, _ := rootOf(wrong1)
	w2, _ := rootOf(wrong2)
	require.NotEqual(t, right, w1, "premise: node %d's root is wrong", wrong1)
	require.Equal(t, w1, w2, "premise: the two wrong nodes' roots are the same")

	// Every root the wrong nodes compute from here, block by block.
	wrongRoots := map[[32]byte]bool{}
	for i := 0; i < 40; i++ {
		sim.StepN(1)
		r1, _ := rootOf(wrong1)
		r2, _ := rootOf(wrong2)
		r0, _ := rootOf(0)
		require.Equal(t, r1, r2)
		require.NotEqual(t, r0, r1)
		wrongRoots[r1] = true
	}

	// Premise: the wrong nodes produced anchors for blocks after the stray
	// write, so there was something for a quorum to form on.
	for _, i := range []int{wrong1, wrong2} {
		View(t, bvn.NodeDatabase(i), func(batch *database.Batch) {
			chain, err := batch.Account(PartitionUrl("BVN0").JoinPath(AnchorPool)).AnchorSequenceChain().Get()
			require.NoError(t, err)
			hash, err := chain.Entry(chain.Height() - 1)
			require.NoError(t, err)
			var last *messaging.TransactionMessage
			require.NoError(t, batch.Message2(hash).Main().GetAs(&last))
			pa := last.Transaction.Body.(AnchorBody).GetPartitionAnchor()
			require.Greater(t, pa.MinorBlockIndex, from, "premise: node %d produced no anchor after block %d", i, from)
			require.True(t, wrongRoots[pa.StateTreeAnchor], "premise: node %d's newest anchor carries a wrong root", i)
		})
	}

	// What the Directory executed of BVN0's anchors after the stray write.
	q := api.Querier2{Querier: sim.S.Services()}
	count, expand := uint64(1000), true
	rec, err := q.QueryMainChainEntries(context.Background(), DnUrl().JoinPath(AnchorPool), &api.ChainQuery{
		Name:  "main",
		Range: &api.RangeOptions{Start: 0, Count: &count, Expand: &expand},
	})
	require.NoError(t, err)
	var executed int
	for _, entry := range rec.Records {
		body, ok := entry.Value.Message.Transaction.Body.(AnchorBody)
		if !ok {
			continue
		}
		pa := body.GetPartitionAnchor()
		if !PartitionUrl("BVN0").Equal(pa.Source) || pa.MinorBlockIndex <= from {
			continue
		}
		executed++
		require.Falsef(t, wrongRoots[pa.StateTreeAnchor],
			"the Directory executed BVN0's anchor for block %d under a root only the two wrong nodes computed", pa.MinorBlockIndex)
	}
	t.Logf("the Directory executed %d BVN0 anchors after block %d", executed, from)
}

// A node that executed block q wrongly and signed its anchor, then repaired
// and handed off again at q, does not sign q's anchor a second time: one key
// never goes on two roots under one sequence number (#4443, DIFFERENCES E20).
//
// At the first block after a handoff at q the conductor builds the anchor for
// q from the pulled ledger -- the partition's sequence number for q -- and the
// node's repaired roots. Before the fix it signed and sent it.
//
// Production throughout except where marked: the node's own executor opens
// block q+1 (the conductor's hook is the executor's WillBeginBlock), the
// handoff is the two calls join.Run makes at a handoff (the executor's
// SettleStagingAt, then the buffer's Handoff), and the dispatch is captured
// on its way out of the simulator's dispatcher. By hand: the wrong state (a
// stray account), its repair (deleting it, which is what re-pulling that
// account from a peer does), and the node's first opening of q+1 before it
// went down -- the simulator cannot re-deliver a block a node has begun, so
// that one Begin is made directly and discarded, as a crash before commit
// discards it.
func TestARepairedNodeDoesNotSignTheHandoffBlockAgain(t *testing.T) {
	type signed struct {
		seq  uint64
		root [32]byte
	}
	var mu sync.Mutex
	var byNode1 []signed
	var key1 []byte
	capture := func(_ context.Context, env *messaging.Envelope) (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		msgs, err := env.Normalize()
		if err != nil {
			return true, nil
		}
		for _, m := range msgs {
			a, ok := m.(*messaging.BlockAnchor)
			if !ok || a.Signature == nil || key1 == nil || string(a.Signature.GetPublicKey()) != string(key1) {
				continue
			}
			seq, ok := a.Anchor.(*messaging.SequencedMessage)
			if !ok || !PartitionUrl("BVN0").Equal(seq.Source.RootIdentity()) {
				continue
			}
			body := seq.Message.(*messaging.TransactionMessage).Transaction.Body.(AnchorBody).GetPartitionAnchor()
			byNode1 = append(byNode1, signed{seq.Number, body.StateTreeAnchor})
		}
		return true, nil
	}

	alice := acctesting.GenerateKey("Alice")
	aliceUrl := acctesting.AcmeLiteAddressStdPriv(alice)
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
		simulator.CaptureDispatchedMessages(capture),
	)
	sim.SetRoute(aliceUrl.RootIdentity(), "BVN0")
	bobUrl := acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("Bob"))
	sim.SetRoute(bobUrl.RootIdentity(), "BVN0")
	MakeLiteTokenAccount(t, sim.DatabaseFor(aliceUrl), alice[32:], AcmeUrl())
	CreditCredits(t, sim.DatabaseFor(aliceUrl), aliceUrl.RootIdentity(), 1e9)
	CreditTokens(t, sim.DatabaseFor(aliceUrl), aliceUrl, big.NewInt(1e12))
	sim.StepN(10)

	bvn := sim.S.Partition("BVN0")
	const node = 1
	mu.Lock()
	key1 = []byte(bvn.NodeConductor(node).ValidatorKey.Public().(ed25519.PublicKey))
	mu.Unlock()
	ledgerOf := func() (index uint64, root [32]byte) {
		View(t, bvn.NodeDatabase(node), func(batch *database.Batch) {
			var ledger *SystemLedger
			require.NoError(t, batch.Account(PartitionUrl("BVN0").JoinPath(Ledger)).Main().GetAs(&ledger))
			r, err := batch.GetBptRootHash()
			require.NoError(t, err)
			index, root = ledger.Index, r
		})
		return index, root
	}

	// The node goes wrong, and executes block q with the wrong state: q is a
	// block that changed state, so its ledger stands at q afterwards.
	stray := &LiteIdentity{Url: acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("stray")).RootIdentity()}
	require.NoError(t, bvn.NodeDatabase(node).Update(func(batch *database.Batch) error {
		return batch.Account(stray.Url).Main().Put(stray)
	}))
	st := sim.BuildAndSubmitTxnSuccessfully(
		build.Transaction().For(aliceUrl).
			SendTokens(1, 0).To(bobUrl).
			SignWith(aliceUrl).Version(1).Timestamp(1).PrivateKey(alice))
	sim.StepUntil(Txn(st.TxID).Succeeds())
	q, wrongRoot := ledgerOf()
	var rightRoot [32]byte
	View(t, bvn.NodeDatabase(0), func(batch *database.Batch) {
		var err error
		rightRoot, err = batch.GetBptRootHash()
		require.NoError(t, err)
	})
	require.NotEqual(t, rightRoot, wrongRoot, "premise: node %d's state after block %d is wrong", node, q)

	// It goes down with its next block begun: its conductor signed q's
	// anchor with the wrong root, and the block was never committed.
	bvn.RestartNode(node)
	mu.Lock()
	byNode1 = nil
	mu.Unlock()
	blk, err := bvn.NodeExecutor(node).Begin(execute.BlockParams{Context: context.Background(), Index: q + 1, Time: time.Now()})
	require.NoError(t, err)
	state, err := blk.Close()
	require.NoError(t, err)
	state.Discard()
	mu.Lock()
	first := append([]signed(nil), byNode1...)
	mu.Unlock()
	require.Len(t, first, 1, "premise: node %d signed q's anchor once when it first began block %d", node, q+1)
	require.Equal(t, wrongRoot, first[0].root, "premise: the first signature is on the wrong root")
	n, _ := ledgerOf()
	require.Equal(t, q, n, "premise: the begun block was not committed")

	// It repairs -- the stray account is re-pulled from a peer, which has
	// none -- and hands off again at q, as join.Run does.
	require.NoError(t, bvn.NodeDatabase(node).Update(func(batch *database.Batch) error {
		if err := batch.DeleteAccountState_TESTONLY(stray.Url); err != nil {
			return err
		}
		return batch.BPT().Delete(record.NewKey("Account", stray.Url))
	}))
	_, repaired := ledgerOf()
	require.NotEqual(t, wrongRoot, repaired, "premise: the repair changed the root")
	settler, ok := bvn.NodeExecutor(node).(interface{ SettleStagingAt(uint64) error })
	require.True(t, ok)
	require.NoError(t, settler.SettleStagingAt(q))
	require.NoError(t, bvn.NodeJoin(node).Handoff(q))

	// The network's block q+1 reaches it, and it executes it.
	mu.Lock()
	byNode1 = nil
	mu.Unlock()
	sim.StepN(1)
	n, _ = ledgerOf()
	require.Greater(t, n, q, "premise: node %d executed a block after the handoff", node)

	mu.Lock()
	defer mu.Unlock()
	for _, s := range byNode1 {
		require.NotEqualf(t, first[0].seq, s.seq,
			"node %d signed anchor %d a second time, with root %x after %x", node, s.seq, s.root[:4], first[0].root[:4])
	}
}
