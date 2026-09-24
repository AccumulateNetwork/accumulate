// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package e2e

import (
	"bytes"
	"context"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
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

// A NODE IS ACTIVE ONLY WHILE IT IS EXECUTING IN AGREEMENT (executor spec,
// "Sync", steps 4 and 6; #4385).
//
// Run 20260924T074702Z (#4205 note_3896688665): a Directory node froze at
// block 661 with its gauge reading ACTIVE, served 693 pulls at that block to
// four joining nodes, and was handed deliveries it accepted and never
// certified or relayed. The machine only moved forward, so a node that went
// ACTIVE at its first match stayed ACTIVE through every re-sync after it.
//
// Here a joined node's state is corrupted after the handoff — the fault a
// missed gap leaves (#4362d, TestJoin_AnExecutedBlockWhoseRootDiverges…) —
// and the root watch catches it. From the divergence until its root matches
// again the node must:
//
//   - refuse a read addressed to it with NotReady, by the production querier
//     over the machine of the join RestartNode built, as the daemon builds it;
//   - sign and dispatch no anchor: the positive control is that it signed
//     them while it executed in agreement;
//   - report BOOTING on accumulate_node_state;
//
// and once it hands off again it answers, reports ACTIVE, and signs anchors
// again. A match alone does not promote it: every reading is taken while the
// node is collecting, matched or not (#4385).
//
// The join is join.Run over the node's own buffer, the executor's stage, and
// the PulledState RestartNode built (join.QueryPeers, own peer excluded) —
// the state whose machine the node's querier refuses by. The test does not
// demote anything: it only steps the simulator and reads.
//
// What this does NOT prove: that a submission is relayed. A simulator node
// hands every submission to the whole partition through the consensus hub
// (DIFFERENCES E11), so a submission addressed to it succeeds whatever its
// state. The relay decision on a demoted machine is proven through the
// daemon's own submitter in cmd/accumulated/run
// (TestADemotedJoinIsRefusedByTheDaemonsServicesAndGauge).
func TestAReSyncingNodeIsBootingUntilItMatchesAgain(t *testing.T) {
	const joiner = 2

	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)

	net := simulator.NewSimpleNetwork(t.Name(), 1, 3)
	joinerKey := net.Bvns[0].Nodes[joiner].PrivValKey[32:]

	// Every BVN0 anchor the joiner signs, counted by the phase of the test it
	// was dispatched in: the phase is the test's own record of what the join
	// did, not the machine's word, so the count does not depend on the thing
	// under test.
	const (
		phaseBefore   = "joined, in agreement"
		phaseResync   = "syncing again"
		phaseRejoined = "joined again"
	)
	var mu sync.Mutex
	phase := ""
	signed := map[string]int{}
	capture := simulator.CaptureDispatchedMessages(func(_ context.Context, env *messaging.Envelope) (bool, error) {
		mu.Lock()
		defer mu.Unlock()
		if phase == "" {
			return true, nil
		}
		for _, m := range env.Messages {
			blk, ok := m.(*messaging.BlockAnchor)
			if !ok || !bytes.Equal(blk.Signature.GetPublicKey(), joinerKey) {
				continue
			}
			// The joiner's Directory node signs with the same validator key;
			// only BVN0's own anchors are this node's.
			seq, ok := blk.Anchor.(*messaging.SequencedMessage)
			if !ok {
				continue
			}
			txn, ok := seq.Message.(*messaging.TransactionMessage)
			if !ok {
				continue
			}
			body, ok := txn.Transaction.Body.(AnchorBody)
			if ok && body.GetPartitionAnchor().Source.Equal(PartitionUrl("BVN0")) {
				signed[phase]++

			}
		}
		return true, nil
	})

	sim := NewSim(t,
		simulator.WithNetwork(net),
		simulator.Genesis(GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
		simulator.BPTHistoryDepth(1024),
		capture,
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
	submit := func() {
		ts++
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	}
	for i := 0; i < 5; i++ {
		submit()
		sim.StepN(3)
	}
	sim.StepN(10)

	p := sim.S.Partition("BVN0")
	p.RestartNode(joiner)
	for i := 0; i < 3; i++ {
		submit()
		sim.StepN(3)
	}

	// The join's state as the daemon builds it, and the machine the node's
	// production querier refuses by.
	pulled := p.NodeJoinState(joiner)
	require.NotNil(t, pulled)
	machine := pulled.Machine()
	setPhase := func(to string) {
		mu.Lock()
		defer mu.Unlock()
		phase = to
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	query := func() error {
		_, err := sim.S.Services().ForPeer(p.NodePeerID(joiner)).
			ForAddress(api.ServiceTypeQuery.AddressFor("BVN0").Multiaddr()).
			Query(ctx, PartitionUrl("BVN0").JoinPath(Ledger), &api.DefaultQuery{})
		return err
	}
	gauge := func() float64 { return nodeStateGauge(t, "bvn0") }

	type reading struct {
		state    nodestate.State
		queryErr error
		gauge    float64
		joining  bool
	}
	var resyncing []reading // taken on every pull between the divergence and the second handoff
	var activeBefore, activeAfter *reading

	buf := &countingBuffer{Buffer: p.NodeJoin(joiner)}
	var corruptedAt, checksAfterSecond int
	// A handoff produces the buffered blocks inside the call, and each one
	// sends its anchors, so the second handoff's blocks are "joined again"
	// from the moment the call starts; a handoff that is refused changes
	// nothing and the phase goes back.
	buf.handingOff = func(refused bool) {
		switch {
		case corruptedAt == 0 || len(buf.handoffs) != 1:
		case !refused:
			setPhase(phaseRejoined)
		default:
			setPhase(phaseResync)
		}
	}
	watches := 0
	state := &steppingPulledState{PulledState: pulled}
	state.step = func() {
		if state.steps%4 == 0 {
			submit()
		}
		state.steps++
		sim.Step()
	}
	state.onPull = func() {
		// Between the divergence and the second handoff: the node is
		// syncing again.
		if corruptedAt == 0 || len(buf.handoffs) != 1 || !buf.Collecting() {
			return
		}
		setPhase(phaseResync)
		r := reading{state: machine.State(), queryErr: query(), gauge: gauge(), joining: p.Joining(joiner)}
		resyncing = append(resyncing, r)
	}
	state.watch = func() {
		watches++
		switch {
		case len(buf.handoffs) == 1 && corruptedAt == 0 && watches == 20:
			require.Equal(t, 1, buf.starts, "precondition: the node was not stopped while its state was right")
			activeBefore = &reading{state: machine.State(), queryErr: query(), gauge: gauge()}
			corrupt(t, p.NodeDatabase(joiner), bob.JoinPath("tokens"))
			corruptedAt = watches
		case len(buf.handoffs) == 1 && corruptedAt == 0:
			setPhase(phaseBefore)
		case len(buf.handoffs) > 1:
			setPhase(phaseRejoined)
			checksAfterSecond++
			if checksAfterSecond == 20 {
				activeAfter = &reading{state: machine.State(), queryErr: query(), gauge: gauge()}
				cancel()
			}
		case watches > 400:
			cancel()
		}
	}

	settler, ok := p.NodeExecutor(joiner).(join.Settler)
	require.True(t, ok, "the executor must settle staging")
	_, err := join.Run(ctx, join.Options{
		Partition: "BVN0",
		Buffer:    buf,
		Stage:     &join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(joiner), Database: p.NodeDatabase(joiner)},
		State:     state,
		Peers:     &join.APIPeers{Partition: "BVN0", Client: sim.S.Services(), Network: t.Name()},
		Retry:     time.Millisecond,
	})
	require.NoError(t, err)

	// The shape: joined, corrupted, caught, synced again, joined again.
	require.NotZero(t, corruptedAt, "precondition: the joined node's state was corrupted after the handoff")
	require.Equal(t, 2, buf.starts, "precondition: the root check stopped the node and it collected again")
	require.Len(t, buf.handoffs, 2, "precondition: it handed off a second time")

	// While it executed in agreement it was ACTIVE, answered, and signed.
	require.NotNil(t, activeBefore)
	require.Equal(t, nodestate.StateActive, activeBefore.state, "precondition: the joined node is ACTIVE")
	require.NoError(t, activeBefore.queryErr, "precondition: an ACTIVE node answers a read addressed to it")
	require.Equal(t, 2.0, activeBefore.gauge, "precondition: the gauge reads ACTIVE")
	mu.Lock()
	signedBefore, signedResync, signedAfter := signed[phaseBefore], signed[phaseResync], signed[phaseRejoined]
	mu.Unlock()
	require.NotZero(t, signedBefore, "precondition: the joined node signed anchors while it executed in agreement, "+
		"so 'signed none while syncing' below is a statement about something it does")

	// While it synced again it was BOOTING, refused, and signed nothing.
	require.NotEmpty(t, resyncing, "no reading was taken while the node synced again")
	for i, r := range resyncing {
		require.Equal(t, nodestate.StateBooting, r.state,
			"reading %d of %d: a node syncing again after its root diverged still says %v", i+1, len(resyncing), r.state)
		require.Error(t, r.queryErr,
			"reading %d of %d: a node syncing again answered a read for the state it knows is wrong", i+1, len(resyncing))
		require.True(t, errors.Is(r.queryErr, errors.NotReady),
			"reading %d of %d: a syncing node must refuse as NotReady, got %v", i+1, len(resyncing), r.queryErr)
		require.Equal(t, 0.0, r.gauge,
			"reading %d of %d: accumulate_node_state reads %v while the node syncs again", i+1, len(resyncing), r.gauge)
		require.True(t, r.joining, "reading %d of %d: the node is executing while it syncs", i+1, len(resyncing))
	}
	require.Zero(t, signedResync, "the node signed %d anchor(s) while it was syncing again", signedResync)
	t.Logf("%d readings while syncing again; BVN0 anchors signed by the joiner: %d in agreement, %d syncing again, %d joined again",
		len(resyncing), signedBefore, signedResync, signedAfter)

	// And once it matched and handed off again it is ACTIVE by the same
	// promotion, and answers.
	require.NotNil(t, activeAfter)
	require.Equal(t, nodestate.StateActive, activeAfter.state, "a node that handed off again is ACTIVE again")
	require.NoError(t, activeAfter.queryErr, "a node that handed off again answers again")
	require.Equal(t, 2.0, activeAfter.gauge, "the gauge reads ACTIVE again")
	require.NotZero(t, signedAfter, "a node that handed off again signs anchors again")

	// And it is on its peers' root chain.
	sim.StepN(10)
	part := PartitionUrl("BVN0")
	var anchors [][]byte
	for i := 0; i < p.NodeCount(); i++ {
		View(t, p.NodeDatabase(i), func(batch *database.Batch) {
			a, err := batch.Account(part.JoinPath(Ledger)).RootChain().Anchor()
			require.NoError(t, err)
			anchors = append(anchors, a)
		})
	}
	for i := 1; i < len(anchors); i++ {
		require.Equal(t, anchors[0], anchors[i], "node %d's root chain is not node 0's after the re-sync", i)
	}
}

// nodeStateGauge reads accumulate_node_state for one partition label from the
// process's registry — what the node's /metrics serves.
func nodeStateGauge(t *testing.T, partition string) float64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != "accumulate_node_state" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == "partition" && l.GetValue() == partition {
					return m.GetGauge().GetValue()
				}
			}
		}
	}
	t.Fatalf("no accumulate_node_state series for %s", partition)
	return 0
}

// corrupt changes an account in one node's store, and its BPT leaf with it,
// without a block: the fault a missed gap leaves.
func corrupt(t *testing.T, db *database.Database, account *url.URL) {
	t.Helper()
	Update(t, db, func(batch *database.Batch) {
		var tokens *TokenAccount
		require.NoError(t, batch.Account(account).Main().GetAs(&tokens))
		tokens.Balance.Add(&tokens.Balance, big.NewInt(7))
		require.NoError(t, batch.Account(account).Main().Put(tokens))
		require.NoError(t, batch.UpdateBPT())
	})
}

// steppingPulledState is the production state with the simulator stepped on
// every pull and every root check. It embeds the concrete *join.PulledState,
// so Run sees its Diverged, HandedOff and Demote as the daemon's join does.
type steppingPulledState struct {
	*join.PulledState
	steps  int
	step   func()
	watch  func()
	onPull func()
}

func (s *steppingPulledState) Pull(ctx context.Context) error {
	s.onPull()
	err := s.PulledState.Pull(ctx)
	s.step()
	return err
}

func (s *steppingPulledState) Diverged(ctx context.Context) (uint64, bool, error) {
	s.step()
	s.watch()
	return s.PulledState.Diverged(ctx)
}

// countingBuffer is the simulator's buffer, counted.
type countingBuffer struct {
	join.Buffer
	starts   int
	handoffs []uint64

	// handingOff, if set, is called as a handoff starts (false) and when
	// one is refused (true).
	handingOff func(refused bool)
}

func (b *countingBuffer) StartCollecting() {
	b.starts++
	b.Buffer.StartCollecting()
}

func (b *countingBuffer) Handoff(q uint64) error {
	if b.handingOff != nil {
		b.handingOff(false)
	}
	err := b.Buffer.Handoff(q)
	if err == nil {
		b.handoffs = append(b.handoffs, q)
	} else if b.handingOff != nil {
		b.handingOff(true)
	}
	return err
}
