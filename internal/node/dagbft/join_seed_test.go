// Copyright 2026 The Accumulate Authors
//
// Use of this source code is governed by an MIT-style
// license that can be found in the LICENSE file or at
// https://opensource.org/licenses/MIT.

package dagbft

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"gitlab.com/accumulatenetwork/accumulate/internal/core/events"
	"gitlab.com/accumulatenetwork/accumulate/internal/database"
	"gitlab.com/accumulatenetwork/accumulate/internal/node/join"
	"gitlab.com/accumulatenetwork/accumulate/pkg/api/v3"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus"
	"gitlab.com/accumulatenetwork/accumulate/pkg/consensus/adapter"
	"gitlab.com/accumulatenetwork/accumulate/pkg/errors"
	"gitlab.com/accumulatenetwork/accumulate/protocol"
)

// A node added to a running network whose DAG has been collected far past
// its first rounds joins and hands off (#4405; Docker run 20260925T011332Z,
// fol2-stall). With no consensus checkpoint it used to order from round zero,
// whose certificates and batches no peer holds any more: the block production
// loop waited for them for ever and the join's StageThrough waited behind it.
//
// Four validators run the production Service over libp2p with a DAG GC depth
// of 20 rounds and traffic in every round. Once they are well past that, a
// fifth node starts collecting and runs join.Run with its own Service as the
// buffer, as cmd/accumulated/run/dagbft.go wires it. The pulled state is a
// validator's own record of the block it produced and the leader round that
// committed it — what the pull leaves in the system ledger. The join must hand
// off, and the blocks the new node then produces must be the validators'
// blocks at the same indexes, batch for batch.
func TestAJoinerAddedPastTheDAGsCollectedRoundsHandsOff(t *testing.T) {
	if testing.Short() {
		t.Skip("five consensus nodes over libp2p")
	}

	const (
		nVals   = 4
		part    = "BVN5seed"
		gcDepth = 20
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	keys := make([]ed25519.PrivateKey, nVals+1)
	initial := make([]adapter.ValidatorInfo, nVals)
	for i := range keys {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		keys[i] = priv
		if i < nVals {
			initial[i] = adapter.ValidatorInfo{PublicKey: [32]byte(pub), Stake: 100, Active: true}
		}
	}

	hosts := make([]host.Host, nVals+1)
	gossips := make([]*pubsub.PubSub, nVals+1)
	for i := range hosts {
		h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
		require.NoError(t, err)
		hosts[i] = h
		t.Cleanup(func() { _ = h.Close() })
		gossips[i], err = pubsub.NewGossipSub(ctx, h)
		require.NoError(t, err)
	}
	for i := range hosts {
		for j := i + 1; j < len(hosts); j++ {
			require.NoError(t, hosts[i].Connect(ctx, peer.AddrInfo{ID: hosts[j].ID(), Addrs: hosts[j].Addrs()}))
		}
	}

	newService := func(i int, a adapter.ConsensusAdapter) *Service {
		svc, err := NewService(ServiceConfig{
			Partition: &protocol.PartitionInfo{ID: part, Type: protocol.PartitionTypeBlockValidator},
			NodeConfig: consensus.NodeConfig{
				Partition:           part,
				KeyPair:             keys[i],
				NumWorkers:          1,
				DAGGCDepth:          gcDepth,
				MinRoundInterval:    50 * time.Millisecond,
				BatchCollectTimeout: time.Minute,
			},
			Adapter:           a,
			EventBus:          events.NewBus(nil),
			Host:              hosts[i],
			PubSub:            gossips[i],
			InitialValidators: initial,
			Database:          database.OpenInMemory(nil),
		})
		require.NoError(t, err)
		return svc
	}

	vals := make([]*Service, nVals)
	recorded := &commitAdapter{}
	for i := range vals {
		a := adapter.ConsensusAdapter(&commitAdapter{})
		if i == 0 {
			a = recorded
		}
		vals[i] = newService(i, a)
		require.NoError(t, vals[i].Start(ctx))
		t.Cleanup(func() { _ = vals[i].Stop() })
	}

	// Traffic in every round, so every certificate names a batch and a node
	// ordering from round zero needs batches no peer still holds.
	go func() {
		for n := 0; ctx.Err() == nil; n++ {
			_ = vals[n%nVals].SubmitTransaction([]byte(fmt.Sprintf("tx-%d", n)))
			time.Sleep(10 * time.Millisecond)
		}
	}()

	require.Eventually(t, func() bool { return vals[0].LastCommitRound() > 5*gcDepth },
		time.Minute, 100*time.Millisecond, "the network never got past its DAG's GC depth")

	// The new node: collecting before it starts, with no checkpoint, as the
	// daemon starts a node configured to join a running network.
	joinerAdapter := &collectingAdapter{}
	joiner := newService(nVals, joinerAdapter)
	joiner.StartCollecting()
	require.NoError(t, joiner.Start(ctx))
	t.Cleanup(func() { _ = joiner.Stop() })

	runCtx, stop := context.WithTimeout(ctx, time.Minute)
	defer stop()
	state := &validatorLedgerState{from: recorded, into: joiner}
	outcome, err := join.Run(runCtx, join.Options{
		Partition: part,
		Buffer:    joiner,
		Stage:     noGapStage{},
		State:     state,
		Peers:     onePeer{},
		Logger:    slog.Default(),
		Retry:     200 * time.Millisecond,
	})
	require.NoError(t, err, "the join did not hand off; buffered %d groups, consensus at round %d",
		joiner.BufferedCount(), joiner.LastCommitRound())
	require.Equal(t, join.Joined, outcome)
	q := state.promoted()
	require.NotZero(t, q)

	// What the new node produces from q + 1 is what the validators produced.
	require.Eventually(t, func() bool { return len(joinerAdapter.produced()) >= 3 },
		30*time.Second, 100*time.Millisecond, "the new node produced nothing after the handoff")
	for _, got := range joinerAdapter.produced()[:3] {
		want, ok := recorded.block(got.Index)
		require.True(t, ok, "validator has no block %d", got.Index)
		require.Equal(t, want.LeaderRound, got.LeaderRound, "block %d", got.Index)
		require.Equal(t, txsOf(want), txsOf(got), "block %d", got.Index)
	}
	require.Equal(t, q+1, joinerAdapter.produced()[0].Index)
}

func (a *commitAdapter) block(index uint64) (adapter.BlockParams, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, b := range a.blocks {
		if b.Index == index {
			return b, true
		}
	}
	return adapter.BlockParams{}, false
}

func (a *commitAdapter) latest() (adapter.BlockParams, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.blocks) == 0 {
		return adapter.BlockParams{}, false
	}
	return a.blocks[len(a.blocks)-1], true
}

func (a *collectingAdapter) produced() []adapter.BlockParams {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]adapter.BlockParams(nil), a.blocks...)
}

func txsOf(b adapter.BlockParams) []string {
	var out []string
	for _, batch := range b.Batches {
		for _, tx := range batch.Transactions {
			out = append(out, string(tx))
		}
	}
	sort.Strings(out)
	return out
}

// validatorLedgerState is the pull reduced to what the handoff reads: each
// Pull puts in the new node's database the system ledger of the latest block
// a validator produced, index and leader round, and that block is matched.
type validatorLedgerState struct {
	from *commitAdapter
	into *Service

	mu      sync.Mutex
	at      uint64
	promote uint64
}

func (s *validatorLedgerState) Pull(context.Context) error {
	b, ok := s.from.latest()
	if !ok {
		return nil
	}
	u := protocol.PartitionUrl(s.into.config.Partition.ID).JoinPath(protocol.Ledger)
	err := s.into.config.Database.Update(func(batch *database.Batch) error {
		return batch.Account(u).Main().Put(&protocol.SystemLedger{Url: u, Index: b.Index, LeaderRound: uint64(b.LeaderRound)})
	})
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.at = b.Index
	s.mu.Unlock()
	return nil
}

func (s *validatorLedgerState) Matched(context.Context) (uint64, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.at, s.at > 0, nil
}

func (s *validatorLedgerState) Ready() (uint64, bool) { return 0, false }
func (s *validatorLedgerState) Demote(uint64)         {}
func (s *validatorLedgerState) Promote(q uint64) {
	s.mu.Lock()
	s.promote = q
	s.mu.Unlock()
}
func (s *validatorLedgerState) promoted() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.promote
}

type noGapStage struct{}

func (noGapStage) SettleStagingAt(uint64) error            { return nil }
func (noGapStage) HasGap(uint64) ([]join.StreamGap, error) { return nil, nil }

type onePeer struct{}

func (onePeer) Validators(context.Context) ([]*api.FindServiceResult, error) {
	return []*api.FindServiceResult{{}}, nil
}

// StageThrough returns when the block production loop cannot take its request
// — here, a loop stuck producing the group committed at round 4, as the
// fol2-stall loop was — and says why, instead of waiting with it (#4405).
func TestStageThrough_ReturnsWhenTheProductionLoopIsStuck(t *testing.T) {
	svc, _, _ := newJoiningService(t)
	svc.StartCollecting()
	defer func(d time.Duration) { stageThroughWait = d }(stageThroughWait)
	stageThroughWait = 100 * time.Millisecond

	// No loop reads the channel: the loop is inside a group.
	svc.producing.Store(4)
	done := make(chan error, 1)
	go func() { done <- svc.StageThrough(10) }()
	select {
	case err := <-done:
		require.True(t, errors.Is(err, errors.NotReady), "got %v", err)
		require.ErrorContains(t, err, "leader round 4")
	case <-time.After(5 * time.Second):
		t.Fatal("StageThrough is still waiting on a production loop that cannot serve it")
	}
}
