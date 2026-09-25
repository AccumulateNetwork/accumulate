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
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
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
// (#4443): a node answers for every anchor it signed whatever its state, and signs and dispatches block N's anchor and synthetics only if
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
//   - Two handoffs. Where join.Run cannot finish because the proof it waits
//     for is the signatures under test, the tests make the two calls it
//     makes at a handoff. The single-restart test runs join.Run itself.
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

// A staggered fault forms no quorum: a node already mismatched when another
// goes wrong the same way signs nothing, so the one wrong signature lag one
// lets through is no quorum.
//
// What lag one can do and what it cannot (executor spec, "Sync", §6). A node
// signs block N's anchor under the check of N-1, so the first wrong anchor of
// every wrongly executing node is signed before any check can say it is
// wrong. Nodes that go wrong the same way in the same block and are enough for
// a quorum sign it together, and their check compares against the root they
// formed, so they match; that is outside the BFT bound, and no gate on a check
// can stop it. What the gate stops is a node that ALREADY knows its state is
// wrong adding its key to a quorum another wrong node started.
//
// BVN0 has three validators, so two signatures are its threshold. Node 2 is
// mismatched before the fault (reported by hand at an earlier block; its state
// is left right until the fault, so that from the fault on both wrong nodes
// compute the same roots -- a node's roots carry its whole history, so two
// nodes that went wrong in different blocks never agree). Nodes 1 and 2 then
// take the same stray account. Node 1 signs the first wrong anchor, as lag one
// allows, and its own mismatch is reported one block later, the earliest a
// check could. Without node 2's gate the two sign that anchor together and the
// Directory executes a root BVN0 never had.
func TestAStaggeredFaultFormsNoQuorum(t *testing.T) {
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

	// Node 2 is already mismatched.
	const first, already = 1, 2
	_, before := rootOf(already)
	bvn.NodeRootCheck(already).Mismatched(before - 1)
	sim.StepN(3)

	// Nodes 1 and 2 go wrong the same way: one account neither the network
	// nor node 0 holds.
	stray := &LiteIdentity{Url: acctesting.AcmeLiteAddressStdPriv(acctesting.GenerateKey("stray")).RootIdentity()}
	for _, i := range []int{first, already} {
		require.NoError(t, bvn.NodeDatabase(i).Update(func(batch *database.Batch) error {
			return batch.Account(stray.Url).Main().Put(stray)
		}))
	}
	right, from := rootOf(0)
	w1, _ := rootOf(first)
	w2, _ := rootOf(already)
	require.NotEqual(t, right, w1, "premise: node %d's root is wrong", first)
	require.Equal(t, w1, w2, "premise: the two wrong nodes' roots are the same")
	wrongRoots := map[[32]byte]bool{w1: true}

	// Node 1 signs the first wrong anchor (block from's, at the next block's
	// open); then its check of that block mismatches.
	sim.StepN(1)
	bvn.NodeRootCheck(first).Mismatched(from)

	// Every root the wrong nodes compute from here, block by block.
	for i := 0; i < 40; i++ {
		sim.StepN(1)
		r1, _ := rootOf(first)
		r2, _ := rootOf(already)
		r0, _ := rootOf(0)
		require.Equal(t, r1, r2)
		require.NotEqual(t, r0, r1)
		wrongRoots[r1] = true
	}

	// Premise: the wrong nodes produced anchors for blocks after the stray
	// write, so there was something for a quorum to form on.
	for _, i := range []int{first, already} {
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

	for _, pa := range directoryAnchors(t, sim, "BVN0") {
		if pa.MinorBlockIndex < from {
			continue
		}
		require.Falsef(t, wrongRoots[pa.StateTreeAnchor],
			"the Directory executed BVN0's anchor for block %d under a root only the two wrong nodes computed", pa.MinorBlockIndex)
	}
}

// directoryAnchors is every anchor from the partition the Directory executed.
func directoryAnchors(t *testing.T, sim *Sim, part string) []*PartitionAnchor {
	t.Helper()
	q := api.Querier2{Querier: sim.S.Services()}
	count, expand := uint64(1000), true
	rec, err := q.QueryMainChainEntries(context.Background(), DnUrl().JoinPath(AnchorPool), &api.ChainQuery{
		Name:  "main",
		Range: &api.RangeOptions{Start: 0, Count: &count, Expand: &expand},
	})
	require.NoError(t, err)
	var out []*PartitionAnchor
	for _, entry := range rec.Records {
		body, ok := entry.Value.Message.Transaction.Body.(AnchorBody)
		if !ok {
			continue
		}
		if pa := body.GetPartitionAnchor(); PartitionUrl(part).Equal(pa.Source) {
			out = append(out, pa)
		}
	}
	return out
}

// handOff hands a node off at block q with the two calls join.Run makes at a
// handoff (the executor's SettleStagingAt, then the buffer's Handoff), for a
// node whose state the test set by hand.
func handOff(t *testing.T, bvn *simulator.Partition, i int, q uint64) {
	t.Helper()
	settler, ok := bvn.NodeExecutor(i).(interface{ SettleStagingAt(uint64) error })
	require.True(t, ok)
	require.NoError(t, settler.SettleStagingAt(q))
	require.NoError(t, bvn.NodeJoin(i).Handoff(q))
}

// rejoin runs node i's join as the daemon runs it (join.Run over the node's
// buffer, executor stage and join state), stepping the network after every
// pull round, and returns the block it handed off at.
func rejoin(t *testing.T, sim *Sim, p *simulator.Partition, i int) uint64 {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stepping := &steppingState{State: p.NodeJoinState(i), step: func(round int) {
		sim.StepN(1)
		if round >= 200 {
			cancel()
		}
	}}
	settler, ok := p.NodeExecutor(i).(join.Settler)
	require.True(t, ok)
	outcome, err := join.Run(ctx, join.Options{
		Partition: p.ID,
		Buffer:    p.NodeJoin(i),
		Stage:     &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(i), Database: p.NodeDatabase(i)},
		State:     stepping,
		Peers:     &join.APIPeers{Partition: p.ID, Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err, "%s node %d did not join", p.ID, i)
	require.Equal(t, join.Joined, outcome)
	require.False(t, p.Joining(i))
	return p.NodeSynthCache(i).Joined()
}

// executedBlock is the block node i's ledger stands at.
func executedBlock(t *testing.T, bvn *simulator.Partition, i int) uint64 {
	t.Helper()
	var index uint64
	View(t, bvn.NodeDatabase(i), func(batch *database.Batch) {
		var ledger *SystemLedger
		require.NoError(t, batch.Account(PartitionUrl(bvn.ID).JoinPath(Ledger)).Main().GetAs(&ledger))
		index = ledger.Index
	})
	return index
}

// A validator that signed an anchor, restarted and handed off still answers
// for it with its own signature (#4443 review F1). What it signed is in
// SystemData, which a restart and a handoff leave as they were. Before, it
// refused every anchor at or below the block it handed off at -- which after
// a restart is its current block, not the boundary of what it executed.
func TestARestartedValidatorAnswersForTheAnchorsItSigned(t *testing.T) {
	sim := NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 4),
		simulator.Genesis(GenesisTime),
		// A collecting node's results are not the executing nodes'.
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
	)
	sim.StepN(20)

	bvn := sim.S.Partition("BVN0")
	const node = 1
	n, ok := bvn.SynthCache().LastAnchorNumber()
	require.True(t, ok, "premise: BVN0 produced an anchor")

	bvn.RestartNode(node)
	sim.StepN(3)
	q := rejoin(t, sim, bvn, node)
	sim.StepN(3 + 16) // past the in-flight window
	_, block, ok := bvn.NodeSynthCache(node).Anchor(n)
	require.True(t, ok)
	require.LessOrEqual(t, block, q, "premise: anchor %d is for a block at or below the handoff", n)
	require.Greater(t, executedBlock(t, bvn, node), q, "premise: node %d executes again", node)

	key := bvn.NodeConductor(node).ValidatorKey.Public().(ed25519.PublicKey)
	r, err := bvn.NodePrivate(node).Sequence(context.Background(), PartitionUrl("BVN0").JoinPath(AnchorPool), DnUrl(), n, private.SequenceOptions{})
	require.NoError(t, err, "node %d signed anchor %d before it restarted and must still answer for it", node, n)
	var own bool
	for _, set := range r.Signatures.Records {
		for _, m := range set.Signatures.Records {
			if sm, ok := m.Message.(*messaging.SignatureMessage); ok {
				if ks, ok := sm.Signature.(KeySignature); ok && string(ks.GetPublicKey()) == string(key) {
					own = true
				}
			}
		}
	}
	require.True(t, own, "node %d's answer for anchor %d carries no signature of its own", node, n)
}

// Two of four validators restarting and handing off at the same block do not
// stop the partition's anchoring (#4443 review F1): each signs the anchor of
// the block it hands off at, which it executed, so the Directory still gets
// three signatures for every BVN0 anchor and keeps executing them.
//
// The two stop right after block q, whose anchor is signed at q+1's open, so
// neither has signed it. The simulator is deterministic, so a first run finds
// such a q and a second, identical one stops there.
func TestTwoOfFourHandingOffAtOneBlockKeepAnchoring(t *testing.T) {
	anchored := map[uint64]bool{}
	var mu sync.Mutex
	newSim := func() *Sim {
		return NewSim(t,
			simulator.SimpleNetwork(t.Name(), 1, 4),
			simulator.Genesis(GenesisTime),
			// A collecting node's results are not the executing nodes'.
			simulator.IgnoreDeliverResults(),
			simulator.IgnoreCommitResults(),
			simulator.CaptureDispatchedMessages(func(_ context.Context, env *messaging.Envelope) (bool, error) {
				mu.Lock()
				defer mu.Unlock()
				for _, m := range env.Messages {
					if a, ok := m.(*messaging.BlockAnchor); ok {
						seq := a.Anchor.(*messaging.SequencedMessage)
						if PartitionUrl("BVN0").Equal(seq.Source.RootIdentity()) {
							anchored[seq.Message.(*messaging.TransactionMessage).Transaction.Body.(AnchorBody).GetPartitionAnchor().MinorBlockIndex] = true
						}
					}
				}
				return true, nil
			}),
		)
	}

	// The first run: the first anchored block from 20 on.
	newSim().StepN(40)
	q := uint64(0)
	mu.Lock()
	for x := range anchored {
		if x >= 20 && (q == 0 || x < q) {
			q = x
		}
	}
	anchored = map[uint64]bool{}
	mu.Unlock()
	require.NotZero(t, q, "premise: BVN0 anchored a block from 20 on")

	// The second: stop at q.
	sim := newSim()
	bvn := sim.S.Partition("BVN0")
	require.Equal(t, uint64(3), bvn.NodeConductor(0).Globals.Load().ValidatorThreshold("BVN0"),
		"premise: three of four sign a BVN0 anchor")
	const a, b = 2, 3
	for i := 0; executedBlock(t, bvn, a) < q; i++ {
		require.Less(t, i, 100)
		sim.StepN(1)
	}
	require.Equal(t, q, executedBlock(t, bvn, a), "premise: node %d stands at block %d", a, q)
	require.Equal(t, q, executedBlock(t, bvn, b), "premise: node %d stands at block %d", b, q)
	mu.Lock()
	require.False(t, anchored[q], "premise: block %d's anchor is not signed yet", q)
	mu.Unlock()

	bvn.RestartNode(a)
	bvn.RestartNode(b)
	sim.StepN(3)
	// Neither can prove a root by itself here: q's anchor has only the two
	// running nodes' signatures until these two sign it, so the join's proof
	// waits on the very signatures this test is about. Both hand off at q
	// with join.Run's two calls, as the reviewer's scenario does.
	handOff(t, bvn, a, q)
	handOff(t, bvn, b, q)
	sim.StepN(40)

	top := executedBlock(t, bvn, 0)
	var highest uint64
	for _, pa := range directoryAnchors(t, sim, "BVN0") {
		if pa.MinorBlockIndex > highest {
			highest = pa.MinorBlockIndex
		}
	}
	require.Greaterf(t, highest, q+20,
		"the Directory executed no BVN0 anchor past block %d while BVN0 ran on to %d; both nodes handed off at %d", highest, top, q)
}

// A node that executed block q wrongly and signed its anchor, then repaired
// and handed off again at q, does not sign q's anchor a second time: one key
// never goes on two roots under one sequence number (#4443, DIFFERENCES E20).
//
// At the first block after a handoff at q the conductor builds the anchor for
// q from the pulled ledger -- the partition's sequence number for q -- and the
// node's repaired roots, a body other than the one it signed under that
// number. It refuses, because SystemData.SignedAnchor records the first body;
// without that record it signed and sent it. Afterwards it agrees with the
// partition and signs its next anchors.
//
// Production throughout except where marked: the node's own executor opens
// block q+1 (the conductor's hook is the executor's WillBeginBlock), the
// handoff is the two calls join.Run makes at a handoff (the executor's
// SettleStagingAt, then the buffer's Handoff), and the dispatch is captured
// on its way out of the simulator's dispatcher. By hand: the wrong state (a
// stray account), its repair (node 0's store copied over this node's, but
// for this node's SystemData, which is what a pull leaves), and the node's first opening of q+1 before it
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

	// It repairs, and hands off again at q, as join.Run does. The repair is
	// the pull's: node 0's state at q, every key but this node's own
	// SystemData, which no pull reaches. Deleting the stray account alone
	// would not do -- block q was executed over it, so its roots are in the
	// chains q wrote.
	pullFrom(t, bvn.NodeDatabase(0), bvn.NodeDatabase(node))
	_, repaired := ledgerOf()
	require.Equal(t, rightRoot, repaired, "premise: the repair restores the right state at block %d", q)
	handOff(t, bvn, node, q)

	// The network's block q+1 reaches it, and it executes it.
	mu.Lock()
	byNode1 = nil
	mu.Unlock()
	sim.StepN(1)
	n, _ = ledgerOf()
	require.Greater(t, n, q, "premise: node %d executed a block after the handoff", node)

	mu.Lock()
	for _, s := range byNode1 {
		require.NotEqualf(t, first[0].seq, s.seq,
			"node %d signed anchor %d a second time, with root %x after %x", node, s.seq, s.root[:4], first[0].root[:4])
	}
	mu.Unlock()

	// And the repaired node agrees with the partition, and signs its next
	// anchors with the partition's roots.
	sim.StepN(3)
	_, mine := ledgerOf()
	View(t, bvn.NodeDatabase(0), func(batch *database.Batch) {
		var err error
		rightRoot, err = batch.GetBptRootHash()
		require.NoError(t, err)
	})
	require.Equal(t, rightRoot, mine, "the repaired node disagrees with node 0")
	mu.Lock()
	defer mu.Unlock()
	require.NotEmpty(t, byNode1, "the repaired node signs no later anchor")
}

// pullFrom makes dst's store src's, but for dst's own SystemData: what a
// join's pull of every account and chain leaves.
func pullFrom(t *testing.T, src, dst *database.Database) {
	t.Helper()
	own := func(k *record.Key) bool { return k.Len() > 0 && k.Get(0) == "SystemData" }
	from, err := src.Store()
	require.NoError(t, err)
	to, err := dst.Store()
	require.NoError(t, err)
	r := from.Begin(nil, false)
	defer r.Discard()
	w := to.Begin(nil, true)
	defer w.Discard()
	keep := map[[32]byte]bool{}
	require.NoError(t, r.ForEach(func(k *record.Key, v []byte) error {
		if own(k) {
			return nil
		}
		keep[k.Hash()] = true
		return w.Put(k, v)
	}))
	var drop []*record.Key
	require.NoError(t, w.ForEach(func(k *record.Key, _ []byte) error {
		if !own(k) && !keep[k.Hash()] {
			drop = append(drop, k)
		}
		return nil
	}))
	for _, k := range drop {
		require.NoError(t, w.Delete(k))
	}
	require.NoError(t, w.Commit())
}
