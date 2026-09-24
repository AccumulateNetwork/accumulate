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
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/pkg/types/messaging"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
	"gitlab.com/accumulatenetwork/accumulate/test/harness"
	"gitlab.com/accumulatenetwork/accumulate/test/simulator"
)

// A DEMOTION REACHES THE DAEMON'S SERVICES AND ITS GAUGE (#4385).
//
// The join demotes its machine when a handoff fails (executor.md, "Sync",
// step 5), and that only stops the node serving if the services the daemon
// built ask THAT machine. This effort's first pass went green while the
// daemon omitted a seam of exactly this kind, so nothing here is built by
// hand that the daemon builds: the join's options are joinOptions and the
// services' node state is nodeStateOf, the two functions the start path
// calls with its one join state; the querier is the one (*Querier).start
// registers; the submitter is newSubmitterService's; the gauge is read from
// the process's Prometheus registry, which is what /metrics serves.
//
// What the daemon does NOT do here and a simulator does: the join state is
// the one test/simulator's RestartNode builds over a simulated node's store
// (join.NewState over join.QueryPeers, own peer excluded — what dagbft.go
// builds), because a node that joins needs peers that are running, and one
// netsim process cannot restart one of its nodes. The buffer's first
// handoff is made to fail, as a buffered group that cannot be produced makes
// it fail (#4401); everything else is the production join.
func TestADemotedJoinIsRefusedByTheDaemonsServicesAndGauge(t *testing.T) {
	const joiner = 2
	const partition = "BVN0"
	partUrl := protocol.PartitionUrl(partition)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sim := harness.NewSim(t,
		simulator.SimpleNetwork(t.Name(), 1, 3),
		simulator.Genesis(harness.GenesisTime),
		simulator.IgnoreDeliverResults(),
		simulator.IgnoreCommitResults(),
		simulator.BPTHistoryDepth(1024),
	)
	sim.StepN(20)
	p := sim.S.Partition(partition)
	p.RestartNode(joiner)
	sim.StepN(10)

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

	// --- one reading of everything the demotion must reach -----------------

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

	var atFailure, demoted, rejoined *reading
	buf.failing = func() { r := read(); atFailure = &r }
	state := &steppedJoinState{PulledState: pulled, step: sim.Step}
	state.pulled = func() {
		if buf.failed && buf.handedOff == 0 && demoted == nil {
			r := read()
			demoted = &r
		}
	}
	state.watched = func() {
		if buf.handedOff > 0 && rejoined == nil {
			r := read()
			rejoined = &r
			cancel()
		}
	}
	opts.State = state
	opts.Retry = time.Millisecond

	_, err = join.Run(ctx, opts)
	require.NoError(t, err)

	// Matched and about to hand off: ACTIVE, and every service answers.
	require.NotNil(t, atFailure, "precondition: the join never reached a handoff")
	require.Equal(t, nodestate.StateActive, atFailure.state, "precondition: a node that matched is ACTIVE")
	require.False(t, errors.Is(atFailure.queryErr, errors.NotReady),
		"precondition: an ACTIVE node's querier refused: %v", atFailure.queryErr)
	require.ErrorContains(t, atFailure.submitErr, "node not started",
		"precondition: an ACTIVE committee member proposes (this fixture's consensus node is not started)")
	require.Equal(t, 2.0, atFailure.gauge, "precondition: the gauge reads ACTIVE")

	// The handoff failed: BOOTING, and the daemon's services say so.
	require.NotNil(t, demoted, "no reading was taken after the handoff failed")
	require.Equal(t, nodestate.StateBooting, demoted.state, "a node whose handoff failed is still %v", demoted.state)
	require.Error(t, demoted.queryErr, "the daemon's querier answered for a node whose handoff failed")
	require.True(t, errors.Is(demoted.queryErr, errors.NotReady), "got %v", demoted.queryErr)
	require.Error(t, demoted.submitErr)
	require.True(t, errors.Is(demoted.submitErr, errors.NoPeer),
		"a demoted node must relay, not propose: the relay finds nobody here, got %v", demoted.submitErr)
	require.Equal(t, 0.0, demoted.gauge, "accumulate_node_state reads %v for a node whose handoff failed", demoted.gauge)

	// Matched again and handed off: ACTIVE again by the same promotion.
	require.NotNil(t, rejoined, "the join never handed off after the failure")
	require.Equal(t, nodestate.StateActive, rejoined.state)
	require.False(t, errors.Is(rejoined.queryErr, errors.NotReady), "a rejoined node's querier refused: %v", rejoined.queryErr)
	require.ErrorContains(t, rejoined.submitErr, "node not started", "a rejoined committee member proposes again")
	require.Equal(t, 2.0, rejoined.gauge, "the gauge reads ACTIVE again")
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
// *join.PulledState, so Run sees its Demote, HandedOff and Diverged.
type steppedJoinState struct {
	*join.PulledState
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
