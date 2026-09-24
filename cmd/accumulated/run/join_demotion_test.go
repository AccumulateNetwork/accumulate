// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package run

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/exp/ioc"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/bootstrap/nodestate"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/dagbft"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	apiv3 "gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3/p2p"
	"gitlab.com/accumulatenetwork/accumulate/pkg/build"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/pkg/url"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/helpers"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
	acctesting "gitlab.com/accumulatenetwork/accumulate/test/testing"
)

// PROMOTION AT THE HANDOFF AND DEMOTION REACH THE DAEMON'S SERVICES AND ITS
// GAUGE (#4385).
//
// A node is ACTIVE only while it is executing in agreement (executor.md,
// "Sync", step 6): the join promotes it when a handoff succeeds — never at a
// match — and demotes it when a re-sync starts or a handoff fails. That only
// stops the node serving if the services the daemon built ask THAT machine.
// This effort's first pass went green while the daemon omitted a seam of
// exactly this kind, so nothing here is built by hand that the daemon builds:
// the join's options are joinOptions and the services' node state is
// nodeStateOf, the two functions the start path calls with its one join
// state; the querier is the one (*Querier).start registers; the submitter is
// newSubmitterService's; the gauge is read from the process's Prometheus
// registry, which is what /metrics serves.
//
// The join runs through four phases, read at each:
//
//  1. It matches and its first handoff fails, as a buffered group that
//     cannot be produced makes it fail (#4401): BOOTING throughout — the
//     match did not promote, so a retried handoff never flips the gauge.
//  2. The retry hands off: ACTIVE.
//  3. Its state is corrupted as a missed gap corrupts it (#4362d) and the
//     root watch catches it: BOOTING while it re-syncs.
//  4. It hands off again: ACTIVE.
//
// What the daemon does NOT do here and a simulator does: the join state is
// the one test/simulator's RestartNode builds over a simulated node's store
// (join.NewState over join.QueryPeers, own peer excluded — what dagbft.go
// builds), because a node that joins needs peers that are running, and one
// netsim process cannot restart one of its nodes.
func TestADemotedJoinIsRefusedByTheDaemonsServicesAndGauge(t *testing.T) {
	const joiner = 2
	const partition = "BVN0"
	partUrl := protocol.PartitionUrl(partition)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	alice := url.MustParse("alice")
	bob := url.MustParse("bob")
	aliceKey := acctesting.GenerateKey(alice)
	sim := harness.NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(harness.GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
		simulator.BPTHistoryDepth(1024),
	)
	sim.SetRoute(alice, partition)
	sim.SetRoute(bob, partition)
	helpers.MakeIdentity(t, sim.DatabaseFor(alice), alice, aliceKey[32:])
	helpers.CreditCredits(t, sim.DatabaseFor(alice), alice.JoinPath("book", "1"), 1e9)
	helpers.MakeAccount(t, sim.DatabaseFor(alice), &protocol.TokenAccount{Url: alice.JoinPath("tokens"), TokenUrl: protocol.AcmeUrl()})
	helpers.CreditTokens(t, sim.DatabaseFor(alice), alice.JoinPath("tokens"), big.NewInt(100000))
	helpers.MakeIdentity(t, sim.DatabaseFor(bob), bob, acctesting.GenerateKey(bob)[32:])
	helpers.MakeAccount(t, sim.DatabaseFor(bob), &protocol.TokenAccount{Url: bob.JoinPath("tokens"), TokenUrl: protocol.AcmeUrl()})
	var ts uint64
	send := func() {
		ts++
		sim.BuildAndSubmitTxnSuccessfully(
			build.Transaction().For(alice, "tokens").
				SendTokens(1, 0).To(bob, "tokens").
				SignWith(alice, "book", "1").Version(1).Timestamp(ts).PrivateKey(aliceKey))
	}
	for i := 0; i < 5; i++ {
		send()
		sim.StepN(3)
	}
	sim.StepN(10)
	p := sim.S.Partition(partition)
	p.RestartNode(joiner)
	for i := 0; i < 3; i++ {
		send()
		sim.StepN(3)
	}

	pulled := p.NodeJoinState(joiner)
	require.NotNil(t, pulled)

	// --- what the daemon builds from that one join state -------------------

	settler, ok := p.NodeExecutor(joiner).(join.Settler)
	require.True(t, ok)
	buf := &failFirstHandoff{Buffer: p.NodeJoin(joiner)}
	opts := joinOptions(partition, pulled, buf,
		&join.ExecutorStage{Settler: settler, Staging: p.NodeStaging(joiner), Database: p.NodeDatabase(joiner)},
		&join.APIPeers{Partition: partition, Client: sim.S.Services(), Network: t.Name()})
	machine, serving := nodeStateOf(pulled)
	require.Equal(t, join.State(pulled), opts.State, "the join the daemon runs is not the join state it built")

	// The querier, through the daemon's own start.
	apiNode, err := p2p.New(p2p.Options{
		Network: "DemotionNet",
		Listen:  []multiaddr.Multiaddr{multiaddr.StringCast("/ip4/127.0.0.1/tcp/0")},
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = apiNode.Close() })
	inst := &Instance{
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		services: ioc.Registry{},
		p2p:      apiNode,
	}
	q := &Querier{Partition: partition, Storage: &StorageOrRef{value: &MemoryStorage{}}}
	require.NoError(t, querierWantsNodeState.Register(inst.services, q, serving))
	require.NoError(t, q.start(inst))
	querier, err := querierProvides.Get(inst.services, q)
	require.NoError(t, err)

	// The submitter, through the daemon's factory, for a node that IS in the
	// committee: only its node state decides whether it proposes or relays.
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	svc, err := dagbft.NewService(dagbft.ServiceConfig{
		Partition:  &protocol.PartitionInfo{ID: partition, Type: protocol.PartitionTypeBlockValidator},
		NodeConfig: consensus.NodeConfig{Partition: partition, KeyPair: priv, NumWorkers: 1},
		Adapter:    stubAdapter{},
		EventBus:   events.NewBus(nil),
		Database:   database.OpenInMemory(nil),
	})
	require.NoError(t, err)
	submitter, err := newSubmitterService(submitterParams{
		Partition:    partition,
		AuthorKey:    pub,
		ValidatorKey: priv,
		Globals:      definitionWith(partition, []ed25519.PublicKey{pub}),
		EventBus:     events.NewBus(nil),
		Service:      svc,
		NodeState:    serving,
		Node:         apiNode,
		Network:      "DemotionNet",
	})
	require.NoError(t, err)

	// --- one reading of everything the node state must reach ---------------

	type reading struct {
		state     nodestate.State
		queryErr  error
		submitErr error
		gauge     float64
	}
	read := func() reading {
		_, qerr := querier.Query(ctx, partUrl.JoinPath(protocol.Ledger), &apiv3.DefaultQuery{})
		verify := false
		_, serr := submitter.Submit(ctx,
			&messaging.Envelope{TxHash: []byte("0123456789abcdef0123456789abcdef")},
			apiv3.SubmitOptions{Verify: &verify})
		return reading{
			state:     machine.State(),
			queryErr:  qerr,
			submitErr: serr,
			gauge:     gaugeByPartition(t, "accumulate_node_state")["bvn0"],
		}
	}
	booting := func(phase string, r *reading) {
		t.Helper()
		require.NotNil(t, r, "%s: no reading was taken", phase)
		require.Equal(t, nodestate.StateBooting, r.state, "%s: the node reads %v", phase, r.state)
		require.Error(t, r.queryErr, "%s: the daemon's querier answered", phase)
		require.True(t, errors.Is(r.queryErr, errors.NotReady), "%s: got %v", phase, r.queryErr)
		require.Error(t, r.submitErr, "%s: the submitter accepted", phase)
		require.True(t, errors.Is(r.submitErr, errors.NoPeer),
			"%s: a BOOTING committee member must relay, not propose (the relay finds nobody here): %v", phase, r.submitErr)
		require.Equal(t, 0.0, r.gauge, "%s: accumulate_node_state reads %v", phase, r.gauge)
	}
	active := func(phase string, r *reading) {
		t.Helper()
		require.NotNil(t, r, "%s: no reading was taken", phase)
		require.Equal(t, nodestate.StateActive, r.state, "%s: the node reads %v", phase, r.state)
		require.False(t, errors.Is(r.queryErr, errors.NotReady), "%s: the querier refused: %v", phase, r.queryErr)
		require.ErrorContains(t, r.submitErr, "node not started",
			"%s: an ACTIVE committee member proposes (this fixture's consensus node is not started)", phase)
		require.Equal(t, 2.0, r.gauge, "%s: accumulate_node_state reads %v", phase, r.gauge)
	}

	// The node may hand off from a pulled state not yet proven, and it is
	// ACTIVE only from the first executed block whose root matches (executor
	// spec, "Sync", "Execute, and repair on a mismatch"). So the "handed off"
	// readings are taken at the first root check that finds it ACTIVE after a
	// handoff, and handoffs are counted from the one it was proven after.
	var matchedFailing, retried, handedOff, resyncing, rejoined *reading
	var provenAfter int
	buf.failing = func() { r := read(); matchedFailing = &r }
	state := &steppedJoinState{PulledState: pulled}
	watches := 0
	state.step = func() {
		if state.steps%4 == 0 {
			send()
		}
		state.steps++
		sim.Step()
	}
	state.pulled = func() {
		switch {
		case buf.failed && buf.handedOff == 0 && retried == nil:
			r := read()
			retried = &r
		case handedOff != nil && buf.Collecting() && resyncing == nil:
			r := read()
			resyncing = &r
		}
	}
	state.watched = func() {
		watches++
		switch {
		case buf.handedOff >= 1 && handedOff == nil && machine.State() == nodestate.StateActive:
			r := read()
			handedOff = &r
			provenAfter = buf.handedOff
			helpers.Update(t, p.NodeDatabase(joiner), func(batch *database.Batch) {
				var tokens *protocol.TokenAccount
				require.NoError(t, batch.Account(bob.JoinPath("tokens")).Main().GetAs(&tokens))
				tokens.Balance.Add(&tokens.Balance, big.NewInt(7))
				require.NoError(t, batch.Account(bob.JoinPath("tokens")).Main().Put(tokens))
				require.NoError(t, batch.UpdateBPT())
			})
		case handedOff != nil && buf.handedOff > provenAfter && rejoined == nil && machine.State() == nodestate.StateActive:
			r := read()
			rejoined = &r
			cancel()
		case watches > 400:
			cancel()
		}
	}
	opts.State = state
	opts.Retry = time.Millisecond

	_, err = join.Run(ctx, opts)
	require.NoError(t, err)

	booting("1. matched, handoff failing", matchedFailing)
	booting("1. retrying after the failed handoff", retried)
	active("2. handed off", handedOff)
	booting("3. re-syncing after the root diverged", resyncing)
	active("4. handed off again", rejoined)
}

// failFirstHandoff fails its first handoff as a buffered group that cannot be
// produced fails it (#4401): the buffer collects again, holding what it held.
type failFirstHandoff struct {
	join.Buffer
	failing   func()
	failed    bool
	handedOff int
}

func (b *failFirstHandoff) Handoff(q uint64) error {
	if !b.failed {
		b.failing()
		b.failed = true
		return errors.UnknownError.WithFormat("produce buffered group 1 of 1: %w", errors.NotFound.With("injected"))
	}
	err := b.Buffer.Handoff(q)
	if err == nil {
		b.handedOff++
	}
	return err
}

// steppedJoinState is the production join state with the simulator stepped
// on every pull and every root check. It embeds the concrete
// *join.PulledState, so Run sees its Promote, Demote, HandedOff and Diverged.
type steppedJoinState struct {
	*join.PulledState
	steps   int
	step    func()
	pulled  func()
	watched func()
}

func (s *steppedJoinState) Pull(ctx context.Context) error {
	s.pulled()
	err := s.PulledState.Pull(ctx)
	s.step()
	return err
}

func (s *steppedJoinState) Diverged(ctx context.Context) (uint64, bool, error) {
	s.step()
	s.watched()
	return s.PulledState.Diverged(ctx)
}
